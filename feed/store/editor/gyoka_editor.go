package editor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	client "github.com/nus25/gyoka-client/go"
	gt "github.com/nus25/gyoka-client/go/types"
	"github.com/nus25/yuge/types"
)

var _ StoreEditor = (*GyokaEditor)(nil) //type check

const (
	maxBatchSize    = 40              // 1回のリクエストで処理できる最大投稿数
	defaultInterval = 1 * time.Minute //Add/Deleteの処理インターバル
)

type feedRequest struct {
	operation string
	feed      gt.FeedUri
	posts     []gt.Post
	count     int
	errCh     chan error
}

type GyokaEditor struct {
	client    *client.Client
	logger    *slog.Logger
	requestCh chan *feedRequest
	done      chan struct{} // 追加：終了通知用のチャネル
	mu        sync.RWMutex
	buffer    struct {
		sync.Mutex
		addPosts    []gt.Post
		deletePosts []gt.Post
	}
	processCh chan struct{} // 即時処理要求用のチャネル
	closeOnce sync.Once
	closeMu   sync.RWMutex
	closing   bool
}

type ClientOptionFunc func(*ClientOption)

type ClientOption struct {
	authType    AuthType
	credentials map[string]string
}

type AuthType int

const (
	NoAuth AuthType = iota
	CloudflareAccess
	BearerToken
	BasicAuth
)

func WithCfToken(clientID string, clientSecret string) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.authType = CloudflareAccess
		opt.credentials = map[string]string{
			"clientId":     clientID,
			"clientSecret": clientSecret,
		}
	}
}

func WithBearerToken(token string) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.authType = BearerToken
		opt.credentials = map[string]string{
			"token": token,
		}
	}
}

func WithBasicAuth(username string, password string) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.authType = BasicAuth
		opt.credentials = map[string]string{
			"username": username,
			"password": password,
		}
	}
}

func NewGyokaEditor(url string, logger *slog.Logger, opts ...ClientOptionFunc) (*GyokaEditor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "gyoka editor")
	if url == "" {
		logger.Info("feed editor url is not set. client will skip syncing")
		return &GyokaEditor{
			client:    nil,
			logger:    logger,
			requestCh: make(chan *feedRequest, 100),
			done:      make(chan struct{}),
			mu:        sync.RWMutex{},
		}, nil
	}

	// オプションの適用

	opt := &ClientOption{
		authType:    NoAuth,
		credentials: make(map[string]string),
	}

	for _, o := range opts {
		if o != nil {
			o(opt)
		}
	}

	// editor.ClientOptionの作成
	var clientOpts []client.ClientOption
	switch opt.authType {
	case CloudflareAccess:
		clientOpts = append(clientOpts, client.WithCloudflareAccess(
			opt.credentials["clientId"],
			opt.credentials["clientSecret"],
		))
	case BearerToken:
		clientOpts = append(clientOpts, client.WithToken(
			opt.credentials["token"],
		))
	case BasicAuth:
		clientOpts = append(clientOpts, client.WithBasicAuth(
			opt.credentials["username"],
			opt.credentials["password"],
		))
	}

	client, err := client.NewClient(url, logger, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create editor client: %w", err)
	}

	return &GyokaEditor{
		client:    client,
		logger:    logger,
		requestCh: make(chan *feedRequest, 100),
		done:      make(chan struct{}),
		mu:        sync.RWMutex{},
		processCh: make(chan struct{}, 1),
	}, nil
}

func (e *GyokaEditor) Open(ctx context.Context) error {
	if e.client == nil {
		return fmt.Errorf("failed to open gyoka. client is nil")
	}
	err := e.client.Ping(ctx)
	if err != nil {
		return err
	}
	go func() {
		if err := e.startWorker(); err != nil {
			e.logger.Error("worker error", "error", err)
		}
	}()
	return nil
}

func (e *GyokaEditor) startWorker() error {
	if e.client == nil {
		return nil
	}
	e.logger.Info("starting worker")
	defer func() {
		e.closeMu.Lock()
		e.closing = true
		e.closeMu.Unlock()

		e.logger.Info("draining remaining requests in channel")
		e.processBufferedRequests()

		e.closeOnce.Do(func() {
			close(e.done)
			close(e.requestCh)
		})
		e.logger.Info("worker shutdown completed")
	}()

	ticker := time.NewTicker(defaultInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.done:
			return nil
		case req := <-e.requestCh:
			switch req.operation {
			case "add", "delete":
				e.bufferRequest(req)
				req.errCh <- nil
			case "trim":
				e.processBufferedRequests()
				err := e.processRequest(req)
				req.errCh <- err
			}
		case <-ticker.C:
			e.processBufferedRequests()
		case <-e.processCh:
			e.processBufferedRequests()
		}
	}
}

func (e *GyokaEditor) bufferRequest(req *feedRequest) {
	e.buffer.Lock()
	defer e.buffer.Unlock()

	switch req.operation {
	case "add":
		e.buffer.addPosts = append(e.buffer.addPosts, req.posts...)
		if len(e.buffer.addPosts) >= maxBatchSize {
			// バッファが閾値を超えたら即時処理を要求
			select {
			case e.processCh <- struct{}{}:
			default:
				// 既に処理要求がある場合は無視
			}
		}
	case "delete":
		e.buffer.deletePosts = append(e.buffer.deletePosts, req.posts...)
		if len(e.buffer.deletePosts) >= maxBatchSize {
			select {
			case e.processCh <- struct{}{}:
			default:
			}
		}
	}
}

func (e *GyokaEditor) processBufferedRequests() {
	e.buffer.Lock()
	defer e.buffer.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// addリクエストを最大40件ずつ処理
	for len(e.buffer.addPosts) > 0 {
		batchSize := min(len(e.buffer.addPosts), maxBatchSize)
		batch := e.buffer.addPosts[:batchSize]

		if resp, err := e.client.Add(ctx, batch); err != nil {
			e.logger.Error("failed to process buffered add requests", "error", err)
		} else if len(resp.FailedPosts) > 0 {
			e.logger.Error("failed to add some posts", "message", resp.Message)
		}

		e.buffer.addPosts = e.buffer.addPosts[batchSize:]
	}

	// deleteリクエストを最大40件ずつ処理
	for len(e.buffer.deletePosts) > 0 {
		batchSize := min(len(e.buffer.deletePosts), maxBatchSize)
		batch := e.buffer.deletePosts[:batchSize]

		if resp, err := e.client.Delete(ctx, batch); err != nil {
			e.logger.Error("failed to process buffered delete requests", "error", err)
		} else if len(resp.FailedPosts) > 0 {
			e.logger.Error("failed to delete some posts", "message", resp.Message)
		}

		e.buffer.deletePosts = e.buffer.deletePosts[batchSize:]
	}
}

func (e *GyokaEditor) processRequest(req *feedRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch req.operation {
	case "add":
		resp, err := e.client.Add(ctx, req.posts)
		if err != nil {
			return err
		}
		if len(resp.FailedPosts) > 0 {
			return fmt.Errorf("failed to add some posts: %s", resp.Message)
		}
		return nil
	case "delete":
		resp, err := e.client.Delete(ctx, req.posts)
		if err != nil {
			return err
		}
		if len(resp.FailedPosts) > 0 {
			return fmt.Errorf("failed to delete some posts: %s", resp.Message)
		}
		return nil
	case "trim":
		resp, err := e.client.TrimWithCount(ctx, req.feed, req.count)
		if err != nil {
			return err
		}
		if resp.DeletedCount == 0 {
			e.logger.Info("no posts were trimmed")
		}
		return nil
	default:
		return fmt.Errorf("unknown operation: %s", req.operation)
	}
}

func (e *GyokaEditor) sendRequest(req *feedRequest) error {
	e.closeMu.RLock()
	if e.closing {
		e.closeMu.RUnlock()
		return fmt.Errorf("editor is shutting down")
	}
	e.closeMu.RUnlock()

	select {
	case e.requestCh <- req:
		return <-req.errCh
	case <-e.done:
		return fmt.Errorf("client is closed")
	}
}

func (e *GyokaEditor) Load(ctx context.Context, params LoadParams) ([]types.Post, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		e.mu.RLock()
		defer e.mu.RUnlock()
		resp, err := e.client.ListPost(ctx, gt.FeedUri(params.FeedUri), params.Limit)
		if err != nil {
			return nil, err
		}
		posts := make([]types.Post, len(resp.Posts))
		for i, p := range resp.Posts {
			posts[i] = types.Post{
				Uri:       types.PostUri(p.Uri),
				Cid:       p.Cid,
				IndexedAt: p.IndexedAt,
			}
		}
		return posts, nil
	}
}

func (e *GyokaEditor) Add(params PostParams) error {
	if e.client == nil {
		e.logger.Info("No feed editor url is set. Add request is skipped.")
		return nil
	}
	if err := params.FeedUri.Validate(); err != nil {
		e.logger.Error("invalid feed uri", "error", err)
		return fmt.Errorf("invalid feed uri: %w", err)
	}

	uri := "at://" + params.Did + "/app.bsky.feed.post/" + params.Rkey
	post := gt.Post{
		Feed:      gt.FeedUri(params.FeedUri),
		Uri:       gt.PostUri(uri),
		Cid:       params.Cid,
		IndexedAt: params.IndexedAt.UTC().Format(time.RFC3339Nano),
	}

	errCh := make(chan error, 1)
	e.requestCh <- &feedRequest{
		operation: "add",
		posts:     []gt.Post{post},
		errCh:     errCh,
	}
	return <-errCh
}

func (e *GyokaEditor) Delete(params DeleteParams) error {
	if e.client == nil {
		e.logger.Info("No feed editor url is set. Delete request is skipped.")
		return nil
	}
	if err := params.FeedUri.Validate(); err != nil {
		e.logger.Error("invalid feed uri", "error", err)
		return fmt.Errorf("invalid feed uri: %w", err)
	}

	uri := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", params.Did, params.Rkey)
	post := gt.Post{
		Feed: gt.FeedUri(params.FeedUri),
		Uri:  gt.PostUri(uri),
	}

	errCh := make(chan error, 1)
	return e.sendRequest(&feedRequest{
		operation: "delete",
		posts:     []gt.Post{post},
		errCh:     errCh,
	})
}
func (e *GyokaEditor) Trim(params TrimParams) error {
	f := params.FeedUri
	count := params.Count
	if e.client == nil {
		e.logger.Info("No feed editor url is set. Trim request is skipped.")
		return nil
	}
	if count < 0 {
		e.logger.Error("Invalid argument at Trim", "count", count)
		return fmt.Errorf("invalid count: %d", count)
	}
	if err := f.Validate(); err != nil {
		e.logger.Error("invalid feed uri", "error", err)
		return fmt.Errorf("invalid feed uri: %w", err)
	}

	errCh := make(chan error, 1)
	return e.sendRequest(&feedRequest{
		operation: "trim",
		feed:      gt.FeedUri(f),
		count:     count,
		errCh:     errCh,
	})
}

func (e *GyokaEditor) Save(ctx context.Context, params SaveParams) error {
	// nothing to save
	return nil
}

func (e *GyokaEditor) Close(ctx context.Context) error {
	if e.client != nil {
		e.closeMu.RLock()
		if !e.closing {
			e.closeOnce.Do(func() {
				close(e.done)
			})
		}
		e.closeMu.RUnlock()
		e.client.Close()
	}
	return nil
}
