package gyoka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	client "github.com/nus25/gyoka-client/go"
	"github.com/nus25/yuge/types"
)

const (
	defaultHttpTimeout         = 30 * time.Second
	defaultMaxIdleConns        = 10
	defaultMaxIdleConnsPerHost = 10
	defaultIdleConnTimeout     = 90 * time.Second
	defaultMaxRetries          = 3
	defaultRetryWaitTime       = 2 * time.Second
	maxBatchSize               = 25
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
	BatchAddParams    BatchPostParams
	DeleteParams      DeleteParams
	DeleteByDidParams DeleteByDidParams
	TrimParams        TrimParams
}

type GyokaEditor struct {
	client *client.ClientWithResponses
	option *ClientOption
	logger *slog.Logger
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
	headers             map[string]string
	httpTimeout         time.Duration
	maxIdleConns        int
	maxIdleConnsPerHost int
	idleConnTimeout     time.Duration
	maxRetries          int
	retryWaitTime       time.Duration
}

// WithHeaders adds arbitrary HTTP headers sent with every request to gyoka.
// Calling it multiple times merges the given headers with previously added ones.
func WithHeaders(headers map[string]string) ClientOptionFunc {
	return func(opt *ClientOption) {
		for k, v := range headers {
			opt.headers[k] = v
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
			client: nil,
			option: nil,
			logger: logger,
		}, nil
	}

	opt := &ClientOption{
		headers:             make(map[string]string),
		httpTimeout:         defaultHttpTimeout,
		maxIdleConns:        defaultMaxIdleConns,
		maxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		idleConnTimeout:     defaultIdleConnTimeout,
		maxRetries:          defaultMaxRetries,
		retryWaitTime:       defaultRetryWaitTime,
	}

	for _, o := range opts {
		if o != nil {
			o(opt)
		}
	}
	ch := opt.headers

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
		client: c,
		option: opt,
		logger: logger,
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
			if err := waitForRetry(ctx, delay); err != nil {
				return err
			}
		}

		err := e.executePingRequest(ctx)
		if err == nil {
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

func waitForRetry(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
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
func (e *GyokaEditor) processRequest(req *feedRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt <= e.option.maxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoffDelay(attempt, e.option.retryWaitTime)
			e.logger.Info("retrying request", "operation", req.operation, "attempt", attempt, "delay", delay)
			if err := waitForRetry(ctx, delay); err != nil {
				return err
			}
		}

		err := e.executeRequest(ctx, req)
		if err == nil {
			return nil
		}

		lastErr = err
		if isNonRetryableError(err) {
			e.logger.Error("request failed with non-retryable error", "operation", req.operation, "error", err, "params", req)
			return err
		}

		if attempt < e.option.maxRetries {
			e.logger.Warn("request failed, will retry", "operation", req.operation, "attempt", attempt, "error", err, "params", req)
		}
	}

	e.logger.Error("request failed after all retries", "operation", req.operation, "attempts", e.option.maxRetries+1, "error", lastErr, "params", req)
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
		body := client.PostAddPostJSONRequestBody{
			Feed: string(params.FeedUri),
			Post: client.AddPostPostParam{
				Cid:         params.Cid,
				FeedContext: nil,
				IndexedAt:   &params.IndexedAt,
				Languages:   &languages,
				Reason:      nil,
				Uri:         uri,
			},
		}
		resp, err := e.client.PostAddPostWithResponse(ctx, body)
		if err != nil {
			return err
		}
		return e.handleResponse(resp.StatusCode(), resp.Body)
	case "batchAdd":
		params := req.BatchAddParams

		feedMap := make(map[string][]client.BatchAddPostPostParam)
		for _, entry := range params.Entries {
			feedUri := string(entry.FeedUri)
			uri := "at://" + entry.Did + "/app.bsky.feed.post/" + entry.Rkey
			var languages []string
			if len(entry.Langs) == 0 {
				languages = nil
			} else {
				languages = entry.Langs
			}

			post := client.BatchAddPostPostParam{
				Cid:         entry.Cid,
				FeedContext: nil,
				IndexedAt:   &entry.IndexedAt,
				Languages:   &languages,
				Reason:      nil,
				Uri:         uri,
			}
			feedMap[feedUri] = append(feedMap[feedUri], post)
		}

		entries := make([]struct {
			Feed  string                         `json:"feed"`
			Posts []client.BatchAddPostPostParam `json:"posts"`
		}, 0, len(feedMap))

		for feedUri, posts := range feedMap {
			entries = append(entries, struct {
				Feed  string                         `json:"feed"`
				Posts []client.BatchAddPostPostParam `json:"posts"`
			}{
				Feed:  feedUri,
				Posts: posts,
			})
		}

		body := client.PostBatchAddPostsJSONRequestBody{
			Entries: entries,
		}

		resp, err := e.client.PostBatchAddPostsWithResponse(ctx, body)
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
				IndexedAt: nil,
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
	}

	var lastErr error
	for attempt := 0; attempt <= e.option.maxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoffDelay(attempt, e.option.retryWaitTime)
			e.logger.Info("retrying load request", "attempt", attempt, "delay", delay)
			if err := waitForRetry(ctx, delay); err != nil {
				return nil, err
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
		e.logger.Info("no feed editor url is set. add request is skipped.")
		return fmt.Errorf("no feed editor url is set.add request is skipped")
	}
	if err := params.FeedUri.Validate(); err != nil {
		e.logger.Error("invalid feed uri", "error", err)
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	return e.processRequest(&feedRequest{
		operation: "add",
		AddParams: params,
	})
}

func (e *GyokaEditor) BatchAdd(params BatchPostParams) error {
	if e.client == nil {
		e.logger.Info("No feed editor url is set. BatchAdd request is skipped.")
		return nil
	}

	for _, entry := range params.Entries {
		if err := entry.FeedUri.Validate(); err != nil {
			e.logger.Error("invalid feed uri", "error", err)
			return fmt.Errorf("invalid feed uri: %w", err)
		}
	}

	totalCount := len(params.Entries)
	if totalCount == 0 {
		return nil
	}
	if totalCount > maxBatchSize {
		return fmt.Errorf("batch size exceeds limit: %d > %d", totalCount, maxBatchSize)
	}

	e.logger.Info("processing batch add request", "total_entries", totalCount)
	err := e.processRequest(&feedRequest{
		operation:      "batchAdd",
		BatchAddParams: params,
	})
	if err != nil {
		e.logger.Error("batch request failed", "total_entries", totalCount, "error", err)
		return err
	}

	e.logger.Info("batch add completed successfully", "total_entries", totalCount)
	return nil
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
	return e.processRequest(&feedRequest{
		operation:    "delete",
		DeleteParams: params,
	})
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
	return e.processRequest(&feedRequest{
		operation:         "deleteByDid",
		DeleteByDidParams: DeleteByDidParams{FeedUri: feedUri, Did: did},
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
	return e.processRequest(&feedRequest{
		operation:  "trim",
		TrimParams: params,
	})
}

func (e *GyokaEditor) Save(ctx context.Context, params SaveParams) error {
	return nil
}

func (e *GyokaEditor) Close(ctx context.Context) error {
	_ = ctx
	return nil
}
