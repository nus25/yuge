package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
	"go.uber.org/atomic"
)

type ClientConfig struct {
	Compress          bool
	WebsocketURL      string
	WantedDids        []string
	WantedCollections []string
	MaxSize           uint32
	ExtraHeaders      map[string]string
}

type Scheduler interface {
	AddWork(ctx context.Context, repo string, evt *models.Event) error
	Shutdown()
}

type Client struct {
	Scheduler  Scheduler
	con        *websocket.Conn
	Cursor     int64
	config     *ClientConfig
	logger     *slog.Logger
	decoder    *zstd.Decoder
	BytesRead  atomic.Int64
	EventsRead atomic.Int64
	shutdown   chan chan struct{}
}

func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Compress:          true,
		WebsocketURL:      "ws://localhost:6008/subscribe",
		WantedDids:        []string{},
		WantedCollections: []string{},
		MaxSize:           0,
		ExtraHeaders: map[string]string{
			"User-Agent": "yuge-jetstream-client/v0.0.1",
		},
	}
}

func NewClient(config *ClientConfig, logger *slog.Logger, scheduler Scheduler) (*Client, error) {
	if config == nil {
		config = DefaultClientConfig()
	}

	logger = logger.With("component", "jetstream-client")
	c := Client{
		config:    config,
		shutdown:  make(chan chan struct{}),
		logger:    logger,
		Scheduler: scheduler,
	}

	if config.Compress {
		c.config.ExtraHeaders["Socket-Encoding"] = "zstd"
		dec, err := zstd.NewReader(nil, zstd.WithDecoderDicts(models.ZSTDDictionary))
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
		}
		c.decoder = dec
	}

	return &c, nil
}

func (c *Client) SendPing() error {
	if c.con == nil {
		return nil
	}
	if err := c.con.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second*10)); err != nil {
		return err
	}
	return nil
}

func (c *Client) ConnectAndRead(ctx context.Context, cursor int64) error {
	defer func() {
		if c.con != nil {
			err := c.con.Close() // 接続を明示的にクローズ
			if err != nil {
				c.logger.Error("failed to close connection", "error", err)
			}
			c.con = nil
			c.logger.Info("jetstream connection closed.", "last curosr", c.Cursor)
		}
	}()

	header := http.Header{}
	for k, v := range c.config.ExtraHeaders {
		header.Add(k, v)
	}

	fullURL := c.config.WebsocketURL
	c.logger.Info("fullurl: " + fullURL)
	params := []string{}
	if c.config.Compress {
		params = append(params, "compress=true")
	}
	c.Cursor = cursor
	if c.Cursor > 0 {
		params = append(params, fmt.Sprintf("cursor=%d", c.Cursor))
	} else {
		c.logger.Info("no valid cursor provided, starting from live stream")
	}

	for _, did := range c.config.WantedDids {
		params = append(params, fmt.Sprintf("wantedDids=%s", did))
	}

	for _, collection := range c.config.WantedCollections {
		params = append(params, fmt.Sprintf("wantedCollections=%s", collection))
	}

	if c.config.MaxSize > 0 {
		params = append(params, fmt.Sprintf("maxSize=%d", c.config.MaxSize))
	}

	if len(params) > 0 {
		fullURL += "?" + strings.Join(params, "&")
	}

	u, err := url.Parse(fullURL)
	if err != nil {
		return fmt.Errorf("failed to parse connection url %q: %w", c.config.WebsocketURL, err)
	}

	c.logger.Info("connecting to websocket", "url", u.String(), "cursor", c.Cursor)
	con, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		return err
	}

	//ホストjetstreamとのping&pong設定
	con.SetPingHandler(func(message string) error {
		err := c.con.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(time.Second*60))
		if err == websocket.ErrCloseSent {
			return nil
		} else if e, ok := err.(net.Error); ok && e.Temporary() {
			return nil
		}
		return err
	})

	con.SetPongHandler(func(_ string) error {
		if err := c.con.SetReadDeadline(time.Now().Add(time.Minute)); err != nil {
			return fmt.Errorf("failed to set read deadline: %s", err)
		}
		return nil
	})

	con.SetCloseHandler(func(code int, text string) error {
		c.logger.Info("connection closed", "code", code, "text", text)
		c.Scheduler.Shutdown()
		return nil
	})

	c.con = con

	if err := c.readLoop(ctx); err != nil {
		return fmt.Errorf("read loop failed: %w", err)
	}

	return nil
}

func (c *Client) readLoop(ctx context.Context) error {
	c.logger.Info("starting websocket read loop")

	bytesRead := clientBytesRead.WithLabelValues(c.config.WebsocketURL)
	eventsRead := clientEventsRead.WithLabelValues(c.config.WebsocketURL)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("shutting down read loop on context completion")
			return nil
		case s := <-c.shutdown:
			c.logger.Info("shutting down read loop on shutdown signal")
			s <- struct{}{}
			return nil
		default:
			_, msg, err := c.con.ReadMessage()
			if err != nil {
				c.logger.Error("failed to read message from websocket", "error", err)
				return fmt.Errorf("failed to read message from websocket: %w", err)
			}

			bytesRead.Add(float64(len(msg)))
			eventsRead.Inc()
			c.BytesRead.Add(int64(len(msg)))
			c.EventsRead.Inc()

			// Decompress the message if necessary
			if c.decoder != nil && c.config.Compress {
				m, err := c.decoder.DecodeAll(msg, nil)
				if err != nil {
					c.logger.Error("failed to decompress message", "error", err)
					return fmt.Errorf("failed to decompress message: %w", err)
				}
				msg = m
			}

			// Unpack the message and pass it to the handler
			var event models.Event
			if err := json.Unmarshal(msg, &event); err != nil {
				c.logger.Error("failed to unmarshal event", "error", err)
				return fmt.Errorf("failed to unmarshal event: %w", err)
			}

			if err := c.Scheduler.AddWork(ctx, "jetstream_repo", &event); err != nil {
				c.logger.Error("failed to add work to scheduler", "error", err)
				return fmt.Errorf("failed to add work to scheduler: %w", err)
			}
			c.Cursor = event.TimeUS
		}
	}
}
