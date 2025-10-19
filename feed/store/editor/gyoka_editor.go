package editor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	client "github.com/nus25/gyoka-client/go"
	"github.com/nus25/yuge/types"
)

var _ StoreEditor = (*GyokaEditor)(nil) //type check

const (
	defaultHttpTimeout         = 30 * time.Second
	defaultMaxIdleConns        = 10
	defaultMaxIdleConnsPerHost = 10
	defaultIdleConnTimeout     = 90 * time.Second
	defaultMaxRetries          = 3
	defaultRetryWaitTime       = 2 * time.Second
)

func isRetryableError(statusCode int) bool {
	return statusCode >= 500 || statusCode == 429 || statusCode == 408
}

func calculateBackoffDelay(attempt int, baseDelay time.Duration) time.Duration {
	if attempt == 0 {
		return 0
	}
	delay := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	jitter := delay * 0.1 * (2.0*float64(time.Now().UnixNano()%1000)/1000.0 - 1.0)
	return time.Duration(delay + jitter)
}

type feedRequest struct {
	operation         string
	AddParams         PostParams
	DeleteParams      DeleteParams
	DeleteByDidParams DeleteByDidParams
	TrimParams        TrimParams
	errCh             chan error
}

type GyokaEditor struct {
	client    *client.ClientWithResponses
	option    *ClientOption
	logger    *slog.Logger
	requestCh chan *feedRequest
	done      chan struct{} // 追加：終了通知用のチャネル
	mu        sync.RWMutex
	closeOnce sync.Once
	closeMu   sync.RWMutex
	requestMu sync.RWMutex
	closing   bool
}

type customHeaderTransport struct {
	customHeaders map[string]string
	transport     http.RoundTripper
}

func (c *customHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range c.customHeaders {
		req.Header.Set(key, value)
	}
	if c.transport == nil {
		c.transport = http.DefaultTransport
	}
	return c.transport.RoundTrip(req)
}

type ClientOptionFunc func(*ClientOption)

type ClientOption struct {
	authType            AuthType
	credentials         map[string]string
	httpTimeout         time.Duration
	maxIdleConns        int
	maxIdleConnsPerHost int
	idleConnTimeout     time.Duration
	maxRetries          int
	retryWaitTime       time.Duration
}

type AuthType int

const (
	NoAuth AuthType = iota
	CloudflareAccess
	GyokaApiKey
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

func WithApiKey(key string) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.authType = GyokaApiKey
		opt.credentials = map[string]string{
			"apiKey": key,
		}
	}
}

func WithRetryWaitTime(retryWaitTime time.Duration) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.retryWaitTime = retryWaitTime
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
			option:    nil,
			logger:    logger,
			requestCh: make(chan *feedRequest, 100),
			done:      make(chan struct{}),
			mu:        sync.RWMutex{},
			requestMu: sync.RWMutex{},
		}, nil
	}

	// オプションの適用
	opt := &ClientOption{
		authType:            NoAuth,
		credentials:         make(map[string]string),
		httpTimeout:         defaultHttpTimeout,
		maxIdleConns:        defaultMaxIdleConns,
		maxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		idleConnTimeout:     defaultIdleConnTimeout,
		maxRetries:          defaultMaxRetries,
		retryWaitTime:       defaultRetryWaitTime,
	}

	//Set custom auth headers
	ch := make(map[string]string)
	for _, o := range opts {
		if o != nil {
			o(opt)
			switch opt.authType {
			case CloudflareAccess:
				ch["CF-Access-Client-Id"] = opt.credentials["clientId"]
				ch["CF-Access-Client-Secret"] = opt.credentials["clientSecret"]
			case GyokaApiKey:
				ch["X-API-Key"] = opt.credentials["apiKey"]
			}
		}
	}

	// editor.ClientOptionの作成
	baseTransport := &http.Transport{
		MaxIdleConns:        opt.maxIdleConns,
		MaxIdleConnsPerHost: opt.maxIdleConnsPerHost,
		IdleConnTimeout:     opt.idleConnTimeout,
		DisableCompression:  false,
		DisableKeepAlives:   false,
	}

	hc := &http.Client{
		Transport: &customHeaderTransport{
			customHeaders: ch,
			transport:     baseTransport,
		},
		Timeout: opt.httpTimeout,
	}

	c, err := client.NewClientWithResponses(url, client.WithHTTPClient(hc))
	if err != nil {
		return nil, fmt.Errorf("failed to create editor client: %w", err)
	}

	return &GyokaEditor{
		client:    c,
		option:    opt,
		logger:    logger,
		requestCh: make(chan *feedRequest, 100),
		done:      make(chan struct{}),
		mu:        sync.RWMutex{},
		requestMu: sync.RWMutex{},
	}, nil
}

func (e *GyokaEditor) Open(ctx context.Context) error {
	if e.client == nil {
		return fmt.Errorf("failed to open gyoka. client is nil")
	}

	var lastErr error
	for attempt := 0; attempt <= e.option.maxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoffDelay(attempt, e.option.retryWaitTime)
			e.logger.Info("retrying ping request", "attempt", attempt, "delay", delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := e.executePingRequest(ctx)
		if err == nil {
			go func() {
				if err := e.startWorker(); err != nil {
					e.logger.Error("worker error", "error", err)
				}
			}()
			return nil
		}

		lastErr = err
		if isNonRetryableError(err) {
			e.logger.Error("ping request failed with non-retryable error", "error", err)
			return err
		}

		if attempt < e.option.maxRetries {
			e.logger.Warn("ping request failed, will retry", "attempt", attempt, "error", err)
		}
	}

	e.logger.Error("ping request failed after all retries", "attempts", e.option.maxRetries+1, "error", lastErr)
	return lastErr
}

func (e *GyokaEditor) executePingRequest(ctx context.Context) error {
	resp, err := e.client.GetPing(ctx)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		if isRetryableError(resp.StatusCode) {
			return fmt.Errorf("retryable error: status=%d, body=%s", resp.StatusCode, string(bodyBytes))
		}
		return &NonRetryableError{fmt.Errorf("failed to open gyoka (non-retryable): status=%d, body=%s", resp.StatusCode, string(bodyBytes))}
	}

	var bodyData struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(bodyBytes, &bodyData); err != nil {
		return &NonRetryableError{fmt.Errorf("failed to parse response body as JSON: %v", err)}
	}
	expectedMessage := "Gyoka is available"
	if bodyData.Message != expectedMessage {
		return &NonRetryableError{fmt.Errorf("unexpected message: got %q, want %q", bodyData.Message, expectedMessage)}
	}

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
		for {
			select {
			case req, ok := <-e.requestCh:
				if !ok {
					break
				}
				err := e.processRequest(req)
				req.errCh <- err
			default:
				e.requestMu.Lock()
				pending := len(e.requestCh)
				e.requestMu.Unlock()

				if pending == 0 {
					e.logger.Info("requests draining completed.")
					e.closeOnce.Do(func() {
						close(e.done)
						close(e.requestCh)
					})
					e.logger.Info("worker shutdown completed")
					return
				}
			}
		}
	}()

	for {
		select {
		case <-e.done:
			return nil
		case req := <-e.requestCh:
			err := e.processRequest(req)
			req.errCh <- err
		}
	}
}

func (e *GyokaEditor) processRequest(req *feedRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= e.option.maxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoffDelay(attempt, e.option.retryWaitTime)
			e.logger.Info("retrying request", "operation", req.operation, "attempt", attempt, "delay", delay)
			time.Sleep(delay)
		}

		err := e.executeRequest(ctx, req)
		if err == nil {
			return nil
		}

		lastErr = err
		if isNonRetryableError(err) {
			e.logger.Error("request failed with non-retryable error", "operation", req.operation, "error", err)
			return err
		}

		if attempt < e.option.maxRetries {
			e.logger.Warn("request failed, will retry", "operation", req.operation, "attempt", attempt, "error", err)
		}
	}

	e.logger.Error("request failed after all retries", "operation", req.operation, "attempts", e.option.maxRetries+1, "error", lastErr)
	return lastErr
}

func (e *GyokaEditor) executeRequest(ctx context.Context, req *feedRequest) error {
	switch req.operation {
	case "add":
		params := req.AddParams
		uri := "at://" + params.Did + "/app.bsky.feed.post/" + params.Rkey
		var languages []string
		if len(params.Langs) == 0 {
			languages = nil
		} else {
			languages = params.Langs
		}
		// Fixing the missing type in composite literal error by specifying the type for Post
		body := client.PostAddPostJSONRequestBody{
			Feed: string(params.FeedUri),
			Post: client.AddPostPostParam{
				Cid:         params.Cid,
				FeedContext: nil, //not supported
				IndexedAt:   &params.IndexedAt,
				Languages:   &languages,
				Reason:      nil, //repost is not supported
				Uri:         uri,
			},
		}
		resp, err := e.client.PostAddPostWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return e.handleResponse(resp.StatusCode(), resp.Body)
	case "delete":
		params := req.DeleteParams
		uri := "at://" + params.Did + "/app.bsky.feed.post/" + params.Rkey
		body := client.PostRemovePostJSONRequestBody{
			Feed: string(params.FeedUri),
			Post: client.RemovePostPostParam{
				IndexedAt: nil, //delete all posts for URI
				Uri:       uri,
			},
		}
		resp, err := e.client.PostRemovePostWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return e.handleResponse(resp.StatusCode(), resp.Body)
	case "deleteByDid":
		params := req.DeleteByDidParams
		body := client.PostRemovePostByAuthorJSONRequestBody{
			Feed:   string(params.FeedUri),
			Author: params.Did,
		}
		resp, err := e.client.PostRemovePostByAuthorWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return e.handleResponse(resp.StatusCode(), resp.Body)
	case "trim":
		params := req.TrimParams
		body := client.PostTrimFeedJSONRequestBody{
			Feed:   string(params.FeedUri),
			Remain: params.Count,
		}
		resp, err := e.client.PostTrimFeedWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return e.handleResponse(resp.StatusCode(), resp.Body)
	default:
		return fmt.Errorf("unknown operation: %s", req.operation)
	}
}

func (e *GyokaEditor) handleResponse(statusCode int, body []byte) error {
	switch statusCode {
	case 200:
		return nil
	case 400, 401, 404:
		return &NonRetryableError{fmt.Errorf("request error (non-retryable): %s", string(body))}
	default:
		if isRetryableError(statusCode) {
			return fmt.Errorf("retryable error: status=%d, body=%s", statusCode, string(body))
		}
		return &NonRetryableError{fmt.Errorf("unexpected request error: status=%d, body=%s", statusCode, string(body))}
	}
}

type NonRetryableError struct {
	Err error
}

func (e *NonRetryableError) Error() string {
	return e.Err.Error()
}

func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

func isNonRetryableError(err error) bool {
	var nonRetryable *NonRetryableError
	return errors.As(err, &nonRetryable)
}

func (e *GyokaEditor) Load(ctx context.Context, params LoadParams) ([]types.Post, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		e.mu.RLock()
		defer e.mu.RUnlock()

		// getPosts from gyoka
		var lastErr error
		for attempt := 0; attempt <= e.option.maxRetries; attempt++ {
			if attempt > 0 {
				delay := calculateBackoffDelay(attempt, e.option.retryWaitTime)
				e.logger.Info("retrying load request", "attempt", attempt, "delay", delay)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
			}

			posts, err := e.executeLoadRequest(ctx, params)
			if err == nil {
				return posts, nil
			}

			lastErr = err
			if isNonRetryableError(err) {
				e.logger.Error("load request failed with non-retryable error", "error", err)
				return nil, err
			}

			if attempt < e.option.maxRetries {
				e.logger.Warn("load request failed, will retry", "attempt", attempt, "error", err)
			}
		}

		e.logger.Error("load request failed after all retries", "attempts", e.option.maxRetries+1, "error", lastErr)
		return nil, lastErr
	}
}

func (e *GyokaEditor) executeLoadRequest(ctx context.Context, params LoadParams) ([]types.Post, error) {
	p := &client.GetGetPostsParams{
		Feed:   string(params.FeedUri),
		Limit:  &params.Limit,
		Cursor: nil,
	}
	resp, err := e.client.GetGetPostsWithResponse(ctx, p)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode() {
	case 200:
		e.logger.Info("load posts from gyoka succeed", "feed", resp.JSON200.Feed, "cursor", resp.JSON200.Cursor)
		posts := make([]types.Post, len(resp.JSON200.Posts))
		for i, p := range resp.JSON200.Posts {
			posts[i] = types.Post{
				Uri:       types.PostUri(p.Uri),
				Cid:       p.Cid,
				IndexedAt: p.IndexedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
				//Langs is not supported in local cache
			}
		}
		return posts, nil
	case 400:
		e.logger.Error("failed to load posts.", "error", resp.JSON400.Error, "message", resp.JSON400.Message)
		return nil, &NonRetryableError{fmt.Errorf("bad request (non-retryable): %d", resp.StatusCode())}
	case 401:
		e.logger.Error("failed to load posts.", "error", resp.JSON401.Error, "message", resp.JSON401.Message)
		return nil, &NonRetryableError{fmt.Errorf("unauthorized (non-retryable): %d", resp.StatusCode())}
	case 404:
		e.logger.Error("failed to load posts. Feed may not be registered in gyoka", "error", resp.JSON404.Error, "message", resp.JSON404.Message)
		return nil, &NonRetryableError{fmt.Errorf("not found (non-retryable): %d", resp.StatusCode())}
	default:
		if isRetryableError(resp.StatusCode()) {
			if resp.StatusCode() == 500 {
				e.logger.Error("failed to load posts. Gyoka server has some problem", "error", resp.JSON500.Error, "message", resp.JSON500.Message)
			}
			return nil, fmt.Errorf("retryable error: status=%d", resp.StatusCode())
		}
		e.logger.Error("unexpected status code from GetGetPosts", "status", resp.StatusCode())
		return nil, &NonRetryableError{fmt.Errorf("unexpected status code (non-retryable): %d", resp.StatusCode())}
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

	errCh := make(chan error, 1)
	e.requestCh <- &feedRequest{
		operation: "add",
		AddParams: params,
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
	errCh := make(chan error, 1)
	e.requestCh <- &feedRequest{
		operation:    "delete",
		DeleteParams: params,
		errCh:        errCh,
	}
	return <-errCh
}

func (e *GyokaEditor) DeleteByDid(feedUri types.FeedUri, did string) error {
	if e.client == nil {
		e.logger.Info("No feed editor url is set. DeleteByDid request is skipped.")
		return nil
	}
	if err := feedUri.Validate(); err != nil {
		e.logger.Error("invalid feed uri", "error", err)
		return fmt.Errorf("invalid feed uri: %w", err)
	}

	errCh := make(chan error, 1)
	e.requestCh <- &feedRequest{
		operation:         "deleteByDid",
		DeleteByDidParams: DeleteByDidParams{FeedUri: feedUri, Did: did},
		errCh:             errCh,
	}

	return <-errCh
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
	e.requestCh <- &feedRequest{
		operation:  "trim",
		TrimParams: params,
		errCh:      errCh,
	}
	return <-errCh
}

func (e *GyokaEditor) Save(ctx context.Context, params SaveParams) error {
	// nothing to save
	return nil
}

func (e *GyokaEditor) Close(ctx context.Context) error {
	if e.client != nil {
		e.closeMu.Lock()
		if !e.closing {
			e.closing = true
			e.closeOnce.Do(func() {
				close(e.done)
			})
		}
		e.closeMu.Unlock()
	}
	return nil
}
