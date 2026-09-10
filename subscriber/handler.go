package subscriber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/nus25/yuge/feed"
	jetstreamClient "github.com/nus25/yuge/subscriber/pkg/client"
	"github.com/nus25/yuge/types"
	"github.com/prometheus/client_golang/prometheus"
)

type PostMutationCoordinator interface {
	AddPost(ctx context.Context, params AddPostParams) error
	DeletePost(ctx context.Context, params DeletePostParams) error
	ClearFeed(ctx context.Context, params ClearFeedParams) error
	TrimFeed(ctx context.Context, params TrimFeedParams) error
}

type Handler struct {
	logger              *slog.Logger
	FeedService         *FeedService
	MutationCoordinator PostMutationCoordinator
	Jsc                 *jetstreamClient.Client
	nextMet             int64
}

func NewHandler(l *slog.Logger, fl *FeedService) *Handler {
	l = l.With("component", "Handler")
	return &Handler{
		logger:      l,
		FeedService: fl,
		nextMet:     -1,
	}
}

// jetstreamに接続してイベントを読む
// 接続が閉じられた場合はlast cursorを返す
func (h *Handler) HandleJetstream(ctx context.Context, log *slog.Logger, cursor int64) (int64, error) {
	h.logger.Info("starting jetstream handler", "cursor", cursor)
	// 30秒おきにpingで回線チェック
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second * 30)
		defer t.Stop()

		for {
			select {
			case <-t.C:
				log.Debug("send ping to jetstream.")
				if h.Jsc != nil {
					if err := h.Jsc.SendPing(); err != nil {
						log.Warn("failed to ping", "error", err)
					}
				}
			case <-done:
				log.Info("jetstream ping loop finished.")
				return
			}
		}
	}()
	defer close(done)

	//接続開始
	if err := h.Jsc.ConnectAndRead(ctx, cursor); err != nil {
		h.logger.Error("jetstream connection failed",
			"error", err,
			"cursor", cursor,
			"context", ctx.Err())
		if errors.Is(err, context.Canceled) {
			// 正常なキャンセルの場合
			return h.Jsc.Cursor, context.Canceled
		}
		if strings.Contains(err.Error(), "use of closed network connection") {
			// 接続が閉じられた場合
			return h.Jsc.Cursor, err
		}
		// その他の予期せぬエラーの場合
		return h.Jsc.Cursor, fmt.Errorf("failed to connect and read: %w", err)
	}

	return h.Jsc.Cursor, nil
}

func (h *Handler) HandlePostEvent(ctx context.Context, evt *models.Event) error {
	if evt == nil {
		return errors.New("received nil event")
	}
	if evt.Commit == nil {
		return nil
	}
	// ポストのイベントだけ処理する
	if evt.Commit.Collection != "app.bsky.feed.post" {
		return nil
	}

	postsProcessed.Inc()
	switch evt.Commit.Operation {
	case models.CommitOperationCreate:
		for id, fi := range h.FeedService.GetAllFeeds() {
			if fi.Status.LastStatus != FeedStatusActive || fi.Feed == nil {
				continue
			}
			sd, post, err := func() (bool, *apibsky.FeedPost, error) {
				// if panic occured set error status to the feed
				defer func() {
					if r := recover(); r != nil {
						h.logger.Error("panic occurred", "feed", id, "panic", r)
						fi.Status.SetError(fmt.Errorf("panic occurred in feed %s: %v", id, r))
						return
					}
				}()
				var post apibsky.FeedPost
				if err := json.Unmarshal(evt.Commit.Record, &post); err != nil {
					return false, nil, fmt.Errorf("failed to unmarshal post: %w", err)
				}
				ok, err := h.shouldAdd(fi.Feed, evt.Did, evt.Commit.RKey, &post)
				return ok, &post, err
			}()
			if err != nil {
				h.logger.Error("failed to check if post should be added", "error", err, "feed", id, "did", evt.Did, "rkey", evt.Commit.RKey)
				continue
			}
			if sd {
				go func(feedID string, feed feed.Feed, evt *models.Event, post *apibsky.FeedPost) {
					postsAdded.WithLabelValues(feedID).Inc()
					h.logger.Info("adding post", "feed", feedID, "did", evt.Did, "rkey", evt.Commit.RKey, "Langs", post.Langs)
					if err := h.addAcceptedPost(ctx, feedID, feed, evt, post); err != nil {
						h.logger.Error("failed to add post", "error", err, "feed", feedID, "did", evt.Did, "rkey", evt.Commit.RKey, "Langs", post.Langs)
						return
					}
				}(id, fi.Feed, evt, post)
			}
		}
	case models.CommitOperationDelete:
		for id, fi := range h.FeedService.GetAllFeeds() {
			if fi.Status.LastStatus == FeedStatusError || fi.Feed == nil {
				continue
			}
			if post, exists := fi.Feed.GetPost(evt.Did, evt.Commit.RKey); exists {
				go func(feedID string, feed feed.Feed, evt *models.Event, post types.Post) {
					postsDeleted.WithLabelValues(feedID).Inc()
					h.logger.Info("deleting post", "feed", feedID, "did", evt.Did, "rkey", evt.Commit.RKey)
					if err := h.deleteAcceptedPost(ctx, feedID, feed, evt, post); err != nil {
						h.logger.Error("failed to delete post", "error", err, "feed", feedID, "did", evt.Did, "rkey", evt.Commit.RKey)
						return
					}
				}(id, fi.Feed, evt, post)
			}
		}
	}
	return nil
}

// フィードで定義された判定ロジックでevtをフィルタする
func (h *Handler) shouldAdd(feed feed.Feed, did string, rkey string, post *apibsky.FeedPost) (shuldAdd bool, err error) {
	defer func() {
		if shuldAdd {
			h.logger.Debug("post found", "feed", feed.FeedId(), "text", post.Text)
		}
	}()
	// 判定ロジック
	if post.Text != "" {
		timer := prometheus.NewTimer(feedLogicLatency.WithLabelValues(feed.FeedId()))
		defer timer.ObserveDuration()
		return feed.Test(did, rkey, post), nil
	}

	return false, nil
}

func (h *Handler) addAcceptedPost(ctx context.Context, feedID string, targetFeed feed.Feed, evt *models.Event, post *apibsky.FeedPost) error {
	run := func() error {
		indexedAt := time.Now().UTC()
		trimAt := 0
		trimRemain := 0
		if cfg := targetFeed.Config(); cfg != nil && cfg.Store() != nil {
			trimAt = cfg.Store().GetTrimAt()
			trimRemain = cfg.Store().GetTrimRemain()
		}
		if h.MutationCoordinator != nil {
			if err := h.MutationCoordinator.AddPost(ctx, AddPostParams{
				FeedID:     feedID,
				FeedURI:    types.FeedUri(targetFeed.FeedUri()),
				Did:        evt.Did,
				Rkey:       evt.Commit.RKey,
				Cid:        evt.Commit.CID,
				IndexedAt:  indexedAt,
				Langs:      post.Langs,
				TrimAt:     trimAt,
				TrimRemain: trimRemain,
			}); err != nil {
				return fmt.Errorf("persist accepted post: %w", err)
			}
		}
		if err := targetFeed.AddPost(evt.Did, evt.Commit.RKey, evt.Commit.CID, indexedAt, post.Langs); err != nil {
			return fmt.Errorf("update feed cache: %w", err)
		}
		return nil
	}
	if h.FeedService != nil {
		return h.FeedService.withFeedOperationLock(feedID, run)
	}
	return run()
}

func (h *Handler) deleteAcceptedPost(ctx context.Context, feedID string, targetFeed feed.Feed, evt *models.Event, post types.Post) error {
	run := func() error {
		if h.MutationCoordinator != nil {
			if err := h.MutationCoordinator.DeletePost(ctx, DeletePostParams{
				FeedID:  feedID,
				FeedURI: types.FeedUri(targetFeed.FeedUri()),
				Post:    post,
			}); err != nil {
				return fmt.Errorf("persist accepted delete: %w", err)
			}
		}
		if err := targetFeed.DeletePost(evt.Did, evt.Commit.RKey); err != nil {
			return fmt.Errorf("update feed cache after delete: %w", err)
		}
		return nil
	}
	if h.FeedService != nil {
		return h.FeedService.withFeedOperationLock(feedID, run)
	}
	return run()
}
