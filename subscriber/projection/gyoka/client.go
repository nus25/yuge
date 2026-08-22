package gyoka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	client "github.com/nus25/gyoka-client/go-atproto"
	gyokaschema "github.com/nus25/gyoka-client/go-atproto/schema/gyoka"
	"github.com/nus25/yuge/types"
)

const (
	defaultMaxRetries         = 3
	defaultRetryWaitTime      = 2 * time.Second
	defaultMinRequestInterval = time.Second
	maxBatchSize              = 25
)

func isRetryableError(statusCode int) bool {
	return statusCode >= http.StatusInternalServerError || statusCode == http.StatusTooManyRequests || statusCode == http.StatusRequestTimeout
}

func calculateBackoffDelay(attempt int, baseDelay time.Duration) time.Duration {
	if attempt == 0 {
		return 0
	}
	delay := float64(baseDelay) * math.Pow(2, float64(attempt-1))
	jitter := delay * 0.1 * (2.0*float64(time.Now().UnixNano()%1000)/1000.0 - 1.0)
	return time.Duration(delay + jitter)
}

type ClientConfig struct {
	Host         string
	UserIdentity string
	AppPassword  string
}

type gyokaAPI interface {
	Ping(context.Context) error
	AddPost(context.Context, *gyokaschema.FeedAddPost_Input) error
	BatchAddPosts(context.Context, *gyokaschema.FeedBatchAddPosts_Input) error
	BatchRemovePosts(context.Context, *gyokaschema.FeedBatchRemovePosts_Input) error
	RemovePost(context.Context, *gyokaschema.FeedRemovePost_Input) error
	RemovePostByAuthor(context.Context, *gyokaschema.FeedRemovePostByAuthor_Input) error
	TrimFeed(context.Context, *gyokaschema.FeedTrimFeed_Input) error
	GetPosts(context.Context, string, string, int64) (*gyokaschema.FeedGetPosts_Output, error)
}

type atprotoGyokaAPI struct {
	client *client.Client
}

func (a *atprotoGyokaAPI) Ping(ctx context.Context) error {
	_, err := a.client.Ping(ctx)
	return err
}

func (a *atprotoGyokaAPI) AddPost(ctx context.Context, input *gyokaschema.FeedAddPost_Input) error {
	_, err := a.client.AddPost(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) BatchAddPosts(ctx context.Context, input *gyokaschema.FeedBatchAddPosts_Input) error {
	_, err := a.client.BatchAddPosts(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) BatchRemovePosts(ctx context.Context, input *gyokaschema.FeedBatchRemovePosts_Input) error {
	_, err := a.client.BatchRemovePosts(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) RemovePost(ctx context.Context, input *gyokaschema.FeedRemovePost_Input) error {
	_, err := a.client.RemovePost(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) RemovePostByAuthor(ctx context.Context, input *gyokaschema.FeedRemovePostByAuthor_Input) error {
	_, err := a.client.RemovePostByAuthor(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) TrimFeed(ctx context.Context, input *gyokaschema.FeedTrimFeed_Input) error {
	_, err := a.client.TrimFeed(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) GetPosts(ctx context.Context, cursor, feed string, limit int64) (*gyokaschema.FeedGetPosts_Output, error) {
	return a.client.GetPosts(ctx, cursor, feed, limit)
}

type feedRequest struct {
	operation         string
	addParams         PostParams
	batchAddParams    BatchPostParams
	batchDeleteParams BatchDeleteParams
	deleteParams      DeleteParams
	deleteByDidParams DeleteByDidParams
	trimParams        TrimParams
}

type GyokaEditor struct {
	client            gyokaAPI
	option            *ClientOption
	logger            *slog.Logger
	requestScheduleMu sync.Mutex
	nextRequestAt     time.Time
}

type ClientOptionFunc func(*ClientOption)

type ClientOption struct {
	maxRetries         int
	retryWaitTime      time.Duration
	minRequestInterval time.Duration
}

func WithRetryWaitTime(retryWaitTime time.Duration) ClientOptionFunc {
	return func(opt *ClientOption) { opt.retryWaitTime = retryWaitTime }
}

func WithMinRequestInterval(minRequestInterval time.Duration) ClientOptionFunc {
	return func(opt *ClientOption) { opt.minRequestInterval = minRequestInterval }
}

func NewGyokaEditor(ctx context.Context, config ClientConfig, logger *slog.Logger, opts ...ClientOptionFunc) (*GyokaEditor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	atprotoClient, err := client.New(ctx, config.Host, config.UserIdentity, config.AppPassword)
	if err != nil {
		return nil, fmt.Errorf("create AT Protocol Gyoka client: %w", err)
	}
	logger.Info("authenticated AT Protocol Gyoka client", "host", config.Host, "userIdentity", config.UserIdentity)
	return newGyokaEditor(&atprotoGyokaAPI{client: atprotoClient}, logger, opts...), nil
}

func newGyokaEditor(api gyokaAPI, logger *slog.Logger, opts ...ClientOptionFunc) *GyokaEditor {
	if logger == nil {
		logger = slog.Default()
	}
	option := &ClientOption{
		maxRetries:         defaultMaxRetries,
		retryWaitTime:      defaultRetryWaitTime,
		minRequestInterval: defaultMinRequestInterval,
	}
	for _, optionFunc := range opts {
		if optionFunc != nil {
			optionFunc(option)
		}
	}
	return &GyokaEditor{client: api, option: option, logger: logger.With("component", "gyoka editor")}
}

func (e *GyokaEditor) Open(ctx context.Context) error {
	return e.retry(ctx, "ping", func(ctx context.Context) error {
		return e.client.Ping(ctx)
	})
}

func (e *GyokaEditor) retry(ctx context.Context, operation string, request func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt <= e.option.maxRetries; attempt++ {
		if attempt > 0 {
			if err := waitForRetry(ctx, calculateBackoffDelay(attempt, e.option.retryWaitTime)); err != nil {
				return err
			}
		}
		err := classifyGyokaError(request(ctx))
		if err == nil {
			return nil
		}
		lastErr = err
		if isNonRetryableError(err) {
			return err
		}
		if attempt < e.option.maxRetries {
			e.logger.Warn("gyoka request failed, will retry", "operation", operation, "attempt", attempt, "error", err)
		}
	}
	return lastErr
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (e *GyokaEditor) processRequest(req *feedRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return e.retry(ctx, req.operation, func(ctx context.Context) error {
		if err := e.waitForRequestSlot(ctx); err != nil {
			return err
		}
		return e.executeRequest(ctx, req)
	})
}

func (e *GyokaEditor) waitForRequestSlot(ctx context.Context) error {
	if e.option.minRequestInterval <= 0 {
		return nil
	}
	e.requestScheduleMu.Lock()
	defer e.requestScheduleMu.Unlock()
	if delay := time.Until(e.nextRequestAt); delay > 0 {
		if err := waitForRetry(ctx, delay); err != nil {
			return err
		}
	}
	e.nextRequestAt = time.Now().Add(e.option.minRequestInterval)
	return nil
}

func (e *GyokaEditor) executeRequest(ctx context.Context, req *feedRequest) error {
	switch req.operation {
	case "add":
		return e.client.AddPost(ctx, addPostInput(req.addParams))
	case "batchAdd":
		return e.client.BatchAddPosts(ctx, batchAddPostsInput(req.batchAddParams))
	case "batchRemove":
		return e.client.BatchRemovePosts(ctx, batchRemovePostsInput(req.batchDeleteParams))
	case "delete":
		params := req.deleteParams
		return e.client.RemovePost(ctx, &gyokaschema.FeedRemovePost_Input{
			Feed: string(params.FeedUri),
			Post: &gyokaschema.FeedRemovePost_PostRef{Uri: postURI(params.Did, params.Rkey)},
		})
	case "deleteByDid":
		params := req.deleteByDidParams
		return e.client.RemovePostByAuthor(ctx, &gyokaschema.FeedRemovePostByAuthor_Input{Feed: string(params.FeedUri), Author: params.Did})
	case "trim":
		params := req.trimParams
		return e.client.TrimFeed(ctx, &gyokaschema.FeedTrimFeed_Input{Feed: string(params.FeedUri), Remain: int64(params.Count)})
	default:
		return fmt.Errorf("unknown operation: %s", req.operation)
	}
}

func addPostInput(params PostParams) *gyokaschema.FeedAddPost_Input {
	indexedAt := params.IndexedAt.UTC().Format(time.RFC3339Nano)
	return &gyokaschema.FeedAddPost_Input{
		Feed: string(params.FeedUri),
		Post: &gyokaschema.FeedAddPost_PostInput{
			Cid:       params.Cid,
			IndexedAt: &indexedAt,
			Languages: params.Langs,
			Uri:       postURI(params.Did, params.Rkey),
		},
	}
}

func batchAddPostsInput(params BatchPostParams) *gyokaschema.FeedBatchAddPosts_Input {
	postsByFeed := make(map[string][]*gyokaschema.FeedBatchAddPosts_PostInput)
	for _, entry := range params.Entries {
		indexedAt := entry.IndexedAt.UTC().Format(time.RFC3339Nano)
		postsByFeed[string(entry.FeedUri)] = append(postsByFeed[string(entry.FeedUri)], &gyokaschema.FeedBatchAddPosts_PostInput{
			Cid:       entry.Cid,
			IndexedAt: &indexedAt,
			Languages: entry.Langs,
			Uri:       postURI(entry.Did, entry.Rkey),
		})
	}
	entries := make([]*gyokaschema.FeedBatchAddPosts_EntryInput, 0, len(postsByFeed))
	for feed, posts := range postsByFeed {
		entries = append(entries, &gyokaschema.FeedBatchAddPosts_EntryInput{Feed: feed, Posts: posts})
	}
	return &gyokaschema.FeedBatchAddPosts_Input{Entries: entries}
}

func batchRemovePostsInput(params BatchDeleteParams) *gyokaschema.FeedBatchRemovePosts_Input {
	postsByFeed := make(map[string][]*gyokaschema.FeedBatchRemovePosts_PostInput)
	for _, entry := range params.Entries {
		postsByFeed[string(entry.FeedUri)] = append(postsByFeed[string(entry.FeedUri)], &gyokaschema.FeedBatchRemovePosts_PostInput{
			Uri: postURI(entry.Did, entry.Rkey),
		})
	}
	entries := make([]*gyokaschema.FeedBatchRemovePosts_EntryInput, 0, len(postsByFeed))
	for feed, posts := range postsByFeed {
		entries = append(entries, &gyokaschema.FeedBatchRemovePosts_EntryInput{Feed: feed, Posts: posts})
	}
	return &gyokaschema.FeedBatchRemovePosts_Input{Entries: entries}
}

func postURI(did, rkey string) string {
	return "at://" + did + "/app.bsky.feed.post/" + rkey
}

type NonRetryableError struct{ Err error }

func (e *NonRetryableError) Error() string { return e.Err.Error() }
func (e *NonRetryableError) Unwrap() error { return e.Err }

func classifyGyokaError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && !isRetryableError(apiErr.StatusCode) {
		return &NonRetryableError{Err: err}
	}
	return err
}

func isNonRetryableError(err error) bool {
	var nonRetryable *NonRetryableError
	return errors.As(err, &nonRetryable)
}

func (e *GyokaEditor) Load(ctx context.Context, params LoadParams) ([]types.Post, error) {
	var output *gyokaschema.FeedGetPosts_Output
	err := e.retry(ctx, "get posts", func(ctx context.Context) error {
		var err error
		output, err = e.client.GetPosts(ctx, "", string(params.FeedUri), int64(params.Limit))
		return err
	})
	if err != nil {
		return nil, err
	}
	posts := make([]types.Post, 0, len(output.Posts))
	for _, post := range output.Posts {
		posts = append(posts, types.Post{Uri: types.PostUri(post.Uri), Cid: post.Cid, IndexedAt: post.IndexedAt, Langs: post.Languages})
	}
	return posts, nil
}

func (e *GyokaEditor) Add(params PostParams) error {
	if err := params.FeedUri.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	return e.processRequest(&feedRequest{operation: "add", addParams: params})
}

func (e *GyokaEditor) BatchAdd(params BatchPostParams) error {
	if len(params.Entries) == 0 {
		return nil
	}
	if len(params.Entries) > maxBatchSize {
		return fmt.Errorf("batch size exceeds limit: %d > %d", len(params.Entries), maxBatchSize)
	}
	for _, entry := range params.Entries {
		if err := entry.FeedUri.Validate(); err != nil {
			return fmt.Errorf("invalid feed uri: %w", err)
		}
	}
	return e.processRequest(&feedRequest{operation: "batchAdd", batchAddParams: params})
}

func (e *GyokaEditor) BatchRemove(params BatchDeleteParams) error {
	if len(params.Entries) == 0 {
		return nil
	}
	if len(params.Entries) > maxBatchSize {
		return fmt.Errorf("batch size exceeds limit: %d > %d", len(params.Entries), maxBatchSize)
	}
	for _, entry := range params.Entries {
		if err := entry.FeedUri.Validate(); err != nil {
			return fmt.Errorf("invalid feed uri: %w", err)
		}
	}
	return e.processRequest(&feedRequest{operation: "batchRemove", batchDeleteParams: params})
}

func (e *GyokaEditor) Delete(params DeleteParams) error {
	if err := params.FeedUri.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	return e.processRequest(&feedRequest{operation: "delete", deleteParams: params})
}

func (e *GyokaEditor) DeleteByDid(feedURI types.FeedUri, did string) error {
	if err := feedURI.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	return e.processRequest(&feedRequest{operation: "deleteByDid", deleteByDidParams: DeleteByDidParams{FeedUri: feedURI, Did: did}})
}

func (e *GyokaEditor) Trim(params TrimParams) error {
	if params.Count < 0 {
		return fmt.Errorf("invalid count: %d", params.Count)
	}
	if err := params.FeedUri.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	return e.processRequest(&feedRequest{operation: "trim", trimParams: params})
}

func (e *GyokaEditor) Save(context.Context, SaveParams) error { return nil }
func (e *GyokaEditor) Close(context.Context) error            { return nil }
