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

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
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
	Audience     string
	UserIdentity string
	AppPassword  string
	PrivateKey   atcrypto.PrivateKey
}

type GyokaAPI interface {
	Ping(context.Context) error
	PutPosts(context.Context, *gyokaschema.FeedPutPosts_Input) error
	RemovePosts(context.Context, *gyokaschema.FeedRemovePosts_Input) error
	RemovePostsByAuthor(context.Context, *gyokaschema.FeedRemovePostsByAuthor_Input) error
	TrimPosts(context.Context, *gyokaschema.FeedTrimPosts_Input) error
	ListPosts(ctx context.Context, feed string, postUri string, author string, cursor string, limit int64) (*gyokaschema.FeedListPosts_Output, error)
}

type atprotoGyokaAPI struct {
	client *client.Client
}

func (a *atprotoGyokaAPI) Ping(ctx context.Context) error {
	_, err := a.client.Ping(ctx)
	return err
}

func (a *atprotoGyokaAPI) PutPosts(ctx context.Context, input *gyokaschema.FeedPutPosts_Input) error {
	_, err := a.client.PutPosts(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) RemovePosts(ctx context.Context, input *gyokaschema.FeedRemovePosts_Input) error {
	_, err := a.client.RemovePosts(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) RemovePostsByAuthor(ctx context.Context, input *gyokaschema.FeedRemovePostsByAuthor_Input) error {
	_, err := a.client.RemovePostsByAuthor(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) TrimPosts(ctx context.Context, input *gyokaschema.FeedTrimPosts_Input) error {
	_, err := a.client.TrimPosts(ctx, input)
	return err
}

func (a *atprotoGyokaAPI) ListPosts(ctx context.Context, feed string, postUri string, author string, cursor string, limit int64) (*gyokaschema.FeedListPosts_Output, error) {
	return a.client.ListPosts(ctx, feed, postUri, author, cursor, limit)
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
	client            GyokaAPI
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
	atprotoClient, err := newATProtoClient(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create AT Protocol Gyoka client: %w", err)
	}
	logger.Info("authenticated AT Protocol Gyoka client", "host", config.Host, "userIdentity", config.UserIdentity)
	return newGyokaEditor(&atprotoGyokaAPI{client: atprotoClient}, logger, opts...), nil
}

func newATProtoClient(ctx context.Context, config ClientConfig) (*client.Client, error) {
	if config.PrivateKey == nil {
		return client.New(ctx, config.Host, config.UserIdentity, config.AppPassword)
	}
	issuerDID, err := syntax.ParseDID(config.UserIdentity)
	if err != nil {
		return nil, fmt.Errorf("parse inter-service issuer DID: %w", err)
	}
	return client.NewWithInterServiceAuth(client.InterServiceAuthConfig{
		Host:       config.Host,
		Audience:   config.Audience,
		Issuer:     issuerDID,
		PrivateKey: config.PrivateKey,
	})
}

func newGyokaEditor(api GyokaAPI, logger *slog.Logger, opts ...ClientOptionFunc) *GyokaEditor {
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
		return e.client.PutPosts(ctx, addPostInput(req.addParams))
	case "batchAdd":
		return e.client.PutPosts(ctx, batchAddPostsInput(req.batchAddParams))
	case "batchRemove":
		return e.client.RemovePosts(ctx, batchRemovePostsInput(req.batchDeleteParams))
	case "delete":
		params := req.deleteParams
		return e.client.RemovePosts(ctx, &gyokaschema.FeedRemovePosts_Input{
			Feed: string(params.FeedUri),
			Uris: []string{postURI(params.Did, params.Rkey)},
		})
	case "deleteByDid":
		params := req.deleteByDidParams
		return e.client.RemovePostsByAuthor(ctx, &gyokaschema.FeedRemovePostsByAuthor_Input{Feed: string(params.FeedUri), Author: params.Did})
	case "trim":
		params := req.trimParams
		return e.client.TrimPosts(ctx, &gyokaschema.FeedTrimPosts_Input{Feed: string(params.FeedUri), Remain: int64(params.Count)})
	default:
		return fmt.Errorf("unknown operation: %s", req.operation)
	}
}

func addPostInput(params PostParams) *gyokaschema.FeedPutPosts_Input {
	indexedAt := params.IndexedAt.UTC().Format(time.RFC3339Nano)
	return &gyokaschema.FeedPutPosts_Input{
		Feed: string(params.FeedUri),
		Posts: []*gyokaschema.FeedDefs_PostInput{
			{
				Cid:       params.Cid,
				IndexedAt: &indexedAt,
				Languages: params.Langs,
				Reason:    addPostReason(params.Reason),
				Uri:       postURI(params.Did, params.Rkey),
			},
		},
	}
}

func addPostReason(reason *PostReason) *gyokaschema.FeedDefs_PostInput_Reason {
	if reason == nil {
		return nil
	}
	if reason.Repost != nil {
		return &gyokaschema.FeedDefs_PostInput_Reason{
			FeedDefs_SkeletonReasonRepost: &gyokaschema.FeedDefs_SkeletonReasonRepost{Repost: *reason.Repost},
		}
	}
	return &gyokaschema.FeedDefs_PostInput_Reason{
		FeedDefs_SkeletonReasonPin: &gyokaschema.FeedDefs_SkeletonReasonPin{},
	}
}

func batchAddPostsInput(params BatchPostParams) *gyokaschema.FeedPutPosts_Input {
	if len(params.Posts) == 0 {
		return nil
	}
	posts := make([]*gyokaschema.FeedDefs_PostInput, 0, len(params.Posts))
	for _, entry := range params.Posts {
		indexedAt := entry.IndexedAt.UTC().Format(time.RFC3339Nano)
		posts = append(posts, &gyokaschema.FeedDefs_PostInput{
			Cid:       entry.Cid,
			IndexedAt: &indexedAt,
			Languages: entry.Langs,
			Reason:    addPostReason(entry.Reason),
			Uri:       postURI(entry.Did, entry.Rkey),
		})
	}
	return &gyokaschema.FeedPutPosts_Input{
		Feed:  string(params.FeedUri),
		Posts: posts,
	}
}

func batchRemovePostsInput(params BatchDeleteParams) *gyokaschema.FeedRemovePosts_Input {
	if len(params.Uris) == 0 {
		return nil
	}
	uris := make([]string, 0, len(params.Uris))
	for _, entry := range params.Uris {
		uris = append(uris, postURI(entry.Did, entry.Rkey))
	}
	return &gyokaschema.FeedRemovePosts_Input{
		Feed: string(params.FeedUri),
		Uris: uris,
	}
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
	var output *gyokaschema.FeedListPosts_Output
	err := e.retry(ctx, "get posts", func(ctx context.Context) error {
		var err error
		output, err = e.client.ListPosts(ctx, string(params.FeedUri), "", "", "", int64(params.Limit))
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
	if err := params.Reason.Validate(); err != nil {
		return fmt.Errorf("invalid post reason: %w", err)
	}
	return e.processRequest(&feedRequest{operation: "add", addParams: params})
}

func (e *GyokaEditor) BatchAdd(params BatchPostParams) error {
	if err := params.FeedUri.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	if len(params.Posts) == 0 {
		return nil
	}
	if len(params.Posts) > maxBatchSize {
		return fmt.Errorf("batch size exceeds limit: %d > %d", len(params.Posts), maxBatchSize)
	}
	feedURI := params.FeedUri
	for _, entry := range params.Posts {
		if entry.FeedUri != feedURI {
			return fmt.Errorf("all feed uris must be the same")
		}
	}
	for _, entry := range params.Posts {
		if err := entry.FeedUri.Validate(); err != nil {
			return fmt.Errorf("invalid feed uri: %w", err)
		}
		if err := entry.Reason.Validate(); err != nil {
			return fmt.Errorf("invalid post reason: %w", err)
		}
	}
	return e.processRequest(&feedRequest{operation: "batchAdd", batchAddParams: params})
}

func (e *GyokaEditor) BatchRemove(params BatchDeleteParams) error {
	if err := params.FeedUri.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	if len(params.Uris) == 0 {
		return nil
	}
	if len(params.Uris) > maxBatchSize {
		return fmt.Errorf("batch size exceeds limit: %d > %d", len(params.Uris), maxBatchSize)
	}
	feedURI := params.FeedUri
	for _, entry := range params.Uris {
		if entry.FeedUri != feedURI {
			return fmt.Errorf("all feed uris must be the same")
		}
	}
	for _, entry := range params.Uris {
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
