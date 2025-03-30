package subscriber

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nus25/yuge/feed/store/editor"
	_ "github.com/nus25/yuge/subscriber/customfeedlogic" //for register custom logic block
	jetstreamClient "github.com/nus25/yuge/subscriber/pkg/client"
	"github.com/nus25/yuge/subscriber/pkg/client/schedulers/sequential"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
)

//go:embed webcontent
var webContent embed.FS

func getLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func JetstreamSubscriber(cctx *cli.Context) error {
	ctx := cctx.Context
	//// Prepare
	logLevel := getLogLevel(cctx.String("log-level"))
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(log)
	logger := slog.Default()
	log.Info("log level", "level", logLevel)

	gin.SetMode(gin.ReleaseMode)

	u, err := url.Parse(cctx.String("jetstream-url"))
	if err != nil {
		return fmt.Errorf("failed to parse jetstream-url: %w", err)
	}

	//// setup store editor
	var se editor.StoreEditor
	//Gyoka Editor
	if cctx.String("feed-editor-endpoint") != "" {
		logger.Info("feed editor config", "endpoint", cctx.String("feed-editor-endpoint"))
		var opts []editor.ClientOptionFunc
		if cctx.String("feed-editor-cf-id") != "" {
			opts = append(opts, editor.WithCfToken(cctx.String("feed-editor-cf-id"), cctx.String("feed-editor-cf-secret")))
		} else if cctx.String("feed-editor-token") != "" {
			opts = append(opts, editor.WithBearerToken(cctx.String("feed-editor-token")))
		} else if cctx.String("feed-editor-username") != "" {
			opts = append(opts, editor.WithBasicAuth(cctx.String("feed-editor-username"), cctx.String("feed-editor-password")))
		}
		se, err = editor.NewGyokaEditor(cctx.String("feed-editor-endpoint"), logger, opts...)
		if err != nil {
			return fmt.Errorf("failed to create gyoka editor: %w", err)
		}
	}
	// if no feed editor endpoint, use file editor
	if se == nil {
		se, err = editor.NewFileEditor(cctx.String("data-directory-path"), logger)
		if err != nil {
			return fmt.Errorf("failed to create file editor: %w", err)
		}
	}

	// setup feed service
	var fs *FeedService
	var fdp FeedDefinitionProvider
	if p := cctx.String("config-directory-path"); p != "" {
		logger.Info("creating file feed definition provider", "config-directory-path", p)
		//load feed definition from file
		fdp, err = NewFileFeedDefinitionProvider(p)
		if err != nil {
			return fmt.Errorf("failed to create feed definition provider: %w", err)
		}
	}
	logger.Info("creating feed service", "config-directory-path", cctx.String("config-directory-path"), "data-directory-path", cctx.String("data-directory-path"))
	fs, err = NewFeedService(cctx.String("config-directory-path"), cctx.String("data-directory-path"), fdp, se, logger)
	if err != nil {
		return fmt.Errorf("failed to create feed service: %w", err)
	}
	logger.Info("loading feeds")
	if err := fs.LoadFeeds(context.Background()); err != nil {
		logger.Error("failed to load some feed", "error", err)
	}
	logger.Info("feed loaded", "feeds", fs.GetActiveFeedIDs())

	// handler
	h := NewHandler(logger, fs)

	//setup jetstream client
	config := jetstreamClient.DefaultClientConfig()
	config.WantedCollections = []string{"app.bsky.feed.post"}
	config.WebsocketURL = u.String()
	// compressはメモリ使用量が上がるので使用しない
	config.Compress = false

	sched := sequential.NewScheduler("jetstream_client", logger, h.HandlePostEvent)
	defer sched.Shutdown()
	jsc, err := jetstreamClient.NewClient(config, log, sched)
	if err != nil {
		log.Error("failed to create jetstream client", "error", err)
		return err
	}
	h.Jsc = jsc

	// Prometheusメトリクスエンドポイントの設定
	metricsServer := &http.Server{
		Addr:    cctx.String("metrics-listen-addr"),
		Handler: promhttp.Handler(),
	}
	go func() {
		mux := http.NewServeMux()
		// フィードの投稿数をメトリクスエンドポイントへのアクセス時に収集
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			for _, f := range fs.GetAllFeeds() {
				if f.Status.LastStatus != FeedStatusError && f.Feed != nil {
					updateMetrics(f.Feed)
				}
			}
			promhttp.Handler().ServeHTTP(w, r)
		})
		metricsServer.Handler = mux
		log.Info("starting metrics server", "addr", metricsServer.Addr)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("metrics server error", "error", err)
		}
	}()

	// APIエンドポイントの設定
	apiServer := &http.Server{
		Addr: cctx.String("api-listen-addr"),
		Handler: func() http.Handler {
			r := gin.Default()
			api := NewFeedApiHandler(fs)
			r.GET("", func(c *gin.Context) {
				c.String(200, fmt.Sprintf("hello yuge feed subscriber\njetstream-url: %s", u.String()))
			})
			r.GET("/api", func(c *gin.Context) {
				content, _ := webContent.ReadFile("webcontent/index.html")
				c.Data(200, "text/html", content)
			})
			r.GET("/api/feed", api.ListFeed)
			r.PUT("/api/feed/:feedid", api.RegisterFeed) // POSTからPUTに変更
			r.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
				GET("", api.GetFeedInfo).
				DELETE("", api.UnregisterFeed).
				GET("/status", api.GetFeedStatus).
				PATCH("/status", api.UpdateFeedStatus).
				POST("/clear", api.ClearFeed).
				POST("/reload", api.ReloadFeed).
				GET("/config", api.GetConfig).
				GET("/post", api.GetAllPosts).
				GET("/post/:did", api.GetPostsByDid).
				GET("/post/:did/:rkey", api.GetPostByRkey).
				POST("/post/:did/:rkey", api.AddPost).
				DELETE("/post/:did", api.DeletePostByDid).
				DELETE("/post/:did/:rkey", api.DeletePost).
				POST("/logicblock/:logicblockname/:command", api.ProcessLogicBlockCommand)

			return r
		}(),
	}
	go func() {
		log.Info("starting api server", "addr", apiServer.Addr)
		if err := apiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("api server error", "error", err)
		}
	}()

	//// Start
	log.Info("starting jetstream subscriber service")
	// when critical error occured, close this.
	eventsKill := make(chan struct{})

	//jetstream client
	shutdownJsc := make(chan struct{})
	jscShutdown := make(chan struct{})
	cursor := cctx.Int64("override-cursor")
	go func() {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		l := log.With("source", "jetstream client")
		go func() {
			for {
				//run jetstream client
				lastCursor, err := h.HandleJetstream(ctx, log, cursor)
				cursor = lastCursor
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						jetstreamErrorCount.Inc()
						l.Error("jetstream client returned unexpectedly, retrying in 5 seconds", "error", err)
						select {
						case <-ctx.Done():
							l.Info("jetstream client closed on context cancel")
							close(jscShutdown)
							return
						case <-time.After(5 * time.Second):
							continue //再接続
						}
					} else {
						l.Info("jetstream client closed on context cancel")
						close(jscShutdown)
						return
					}
				}
				close(jscShutdown)
				return
			}
		}()
		<-shutdownJsc
		cancel()
		l.Info("jetstream client shut down")
	}()

	// feed
	shutdownFeed := make(chan struct{})
	feedShutdown := make(chan struct{})
	go func() {
		l := log.With("source", "feed")
		<-shutdownFeed
		l.Info("shutting down feed")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := fs.Shutdown(shutdownCtx)
		if err != nil {
			l.Error("feed shutdown error", "error", err)
		}
		close(feedShutdown)
	}()

	//// Shutdown
	// Trap SIGINT to trigger a shutdown.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-signals:
		log.Info("shutting down on signal")
	case <-ctx.Done():
		log.Info("shutting down on context done")
	case <-eventsKill:
		log.Info("shutting down on events kill")
	}

	log.Info("shutting down, waiting for workers to clean up...")
	close(shutdownJsc)
	shutdownTimeout := time.After(10 * time.Second)
	select {
	case <-jscShutdown:
		log.Info("jetstream client shutdown completed")
	case <-shutdownTimeout:
		log.Warn("shutdown timeout at jetstream client")
	}
	close(shutdownFeed)
	select {
	case <-feedShutdown:
		log.Info("store shutdown completed")
	case <-shutdownTimeout:
		log.Warn("shutdown timeout at Store")
	}

	// メトリクスサーバーのシャットダウン
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Error("metrics server shutdown error", "error", err)
	}
	// APIサーバーのシャットダウン
	shutdownCtx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if err := apiServer.Shutdown(shutdownCtx2); err != nil {
		log.Error("api server shutdown error", "error", err)
	}

	log.Info("shut down successfully")
	return nil
}
