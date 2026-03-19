package subscriber

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"
)

var ErrJetstreamControllerUnavailable = errors.New("jetstream controller is not configured")

type JetstreamConnectRequest struct {
	URL    *string `json:"url,omitempty"`
	Cursor *int64  `json:"cursor,omitempty"`
}

type JetstreamStatusResponse struct {
	Connected    bool   `json:"connected"`
	WebsocketURL string `json:"websocketURL"`
	Cursor       int64  `json:"cursor"`
}

type JetstreamController interface {
	Connect(req JetstreamConnectRequest) (JetstreamStatusResponse, error)
	Disconnect() (JetstreamStatusResponse, error)
	Status() JetstreamStatusResponse
}

type UnavailableJetstreamController struct{}

func NewUnavailableJetstreamController() JetstreamController {
	return &UnavailableJetstreamController{}
}

func IsUnavailableJetstreamController(c JetstreamController) bool {
	_, ok := c.(*UnavailableJetstreamController)
	return ok
}

func (c *UnavailableJetstreamController) Connect(_ JetstreamConnectRequest) (JetstreamStatusResponse, error) {
	return JetstreamStatusResponse{}, ErrJetstreamControllerUnavailable
}

func (c *UnavailableJetstreamController) Disconnect() (JetstreamStatusResponse, error) {
	return JetstreamStatusResponse{}, ErrJetstreamControllerUnavailable
}

func (c *UnavailableJetstreamController) Status() JetstreamStatusResponse {
	return JetstreamStatusResponse{}
}

type RuntimeJetstreamController struct {
	logger *slog.Logger
	h      *Handler

	mu         sync.Mutex
	currentURL string
	cursor     int64
	cancel     context.CancelFunc
	done       chan struct{}
}

func NewRuntimeJetstreamController(logger *slog.Logger, h *Handler, defaultURL string, initialCursor int64) *RuntimeJetstreamController {
	return &RuntimeJetstreamController{
		logger:     logger.With("source", "jetstream-controller"),
		h:          h,
		currentURL: defaultURL,
		cursor:     initialCursor,
	}
}

func (c *RuntimeJetstreamController) Connect(req JetstreamConnectRequest) (JetstreamStatusResponse, error) {
	if req.URL != nil {
		u, err := url.Parse(*req.URL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return JetstreamStatusResponse{}, fmt.Errorf("invalid websocket url: %w", err)
		}
	}

	if req.Cursor != nil && *req.Cursor <= 0 {
		c.logger.Warn("invalid cursor specified in connect request", "cursor", *req.Cursor)
	}

	var waitCh chan struct{}
	var cancel context.CancelFunc
	var jscToClose interface{ Close() error }

	c.mu.Lock()
	if c.cancel != nil {
		cancel = c.cancel
		waitCh = c.done
		if c.h != nil && c.h.Jsc != nil {
			jscToClose = c.h.Jsc
		}
	}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
		if jscToClose != nil {
			_ = jscToClose.Close()
		}
	}

	if waitCh != nil {
		<-waitCh
	}

	c.mu.Lock()
	if req.URL != nil {
		c.currentURL = *req.URL
	}
	if req.Cursor != nil {
		c.cursor = *req.Cursor
	}
	defer c.mu.Unlock()

	if c.h == nil || c.h.Jsc == nil {
		return JetstreamStatusResponse{}, errors.New("jetstream client is not initialized")
	}
	if err := c.h.Jsc.SetWebsocketURL(c.currentURL); err != nil {
		return JetstreamStatusResponse{}, err
	}
	c.startLocked()
	return c.statusLocked(), nil
}

func (c *RuntimeJetstreamController) Disconnect() (JetstreamStatusResponse, error) {
	c.mu.Lock()
	waitCh := c.done
	cancel := c.cancel
	c.mu.Unlock()

	if cancel != nil {
		cancel()
		if c.h != nil && c.h.Jsc != nil {
			_ = c.h.Jsc.Close()
		}
		if waitCh != nil {
			<-waitCh
		}
	}

	return c.Status(), nil
}

func (c *RuntimeJetstreamController) Status() JetstreamStatusResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.statusLocked()
}

func (c *RuntimeJetstreamController) startLocked() {
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	startCursor := c.cursor
	c.cancel = cancel
	c.done = done

	go c.run(runCtx, done, startCursor)
}

func (c *RuntimeJetstreamController) run(ctx context.Context, done chan struct{}, cursor int64) {
	defer func() {
		c.mu.Lock()
		if c.h != nil && c.h.Jsc != nil {
			c.cursor = c.h.Jsc.Cursor
		}
		c.cancel = nil
		c.done = nil
		c.mu.Unlock()
		close(done)
	}()

	for {
		lastCursor, err := c.h.HandleJetstream(ctx, c.logger, cursor)
		c.mu.Lock()
		c.cursor = lastCursor
		c.mu.Unlock()
		cursor = lastCursor

		if err == nil {
			return
		}
		if errors.Is(err, context.Canceled) {
			return
		}

		jetstreamErrorCount.Inc()
		c.logger.Error("jetstream client returned unexpectedly, retrying in 5 seconds", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *RuntimeJetstreamController) statusLocked() JetstreamStatusResponse {
	resp := JetstreamStatusResponse{
		Connected:    c.cancel != nil,
		WebsocketURL: c.currentURL,
		Cursor:       c.cursor,
	}
	if c.h != nil && c.h.Jsc != nil {
		resp.Cursor = c.h.Jsc.Cursor
		if resp.WebsocketURL == "" {
			resp.WebsocketURL = c.h.Jsc.WebsocketURL()
		}
	}
	return resp
}
