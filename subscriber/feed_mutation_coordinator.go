package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	storerepo "github.com/nus25/yuge/feed/store/repository"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	"github.com/nus25/yuge/types"
)

type FeedMutationTransactor interface {
	WithinTx(ctx context.Context, fn func(context.Context, storerepo.FeedRepository, projectionrepo.OutboxRepository) error) error
}

type FeedMutationCoordinator struct {
	transactor FeedMutationTransactor
}

type AddPostParams struct {
	FeedID     string
	FeedURI    types.FeedUri
	Did        string
	Rkey       string
	Cid        string
	IndexedAt  time.Time
	Langs      []string
	TrimAt     int
	TrimRemain int
	MutationID string
}

type DeletePostParams struct {
	FeedID     string
	FeedURI    types.FeedUri
	Post       types.Post
	MutationID string
}

func NewFeedMutationCoordinator(transactor FeedMutationTransactor) *FeedMutationCoordinator {
	return &FeedMutationCoordinator{transactor: transactor}
}

func (c *FeedMutationCoordinator) AddPost(ctx context.Context, params AddPostParams) error {
	if c == nil || c.transactor == nil {
		return fmt.Errorf("feed mutation transactor is required")
	}
	mutationID := params.MutationID
	if mutationID == "" {
		mutationID = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}

	postURI := types.PostUri(fmt.Sprintf("at://%s/app.bsky.feed.post/%s", params.Did, params.Rkey))
	post := types.Post{
		Feed:      params.FeedURI,
		Uri:       postURI,
		Cid:       params.Cid,
		IndexedAt: params.IndexedAt.UTC().Format(time.RFC3339Nano),
		Langs:     params.Langs,
	}
	subjectKey := fmt.Sprintf("%s:%s", params.FeedID, postURI)
	opKey := fmt.Sprintf("%s:add:%s", mutationID, subjectKey)
	payloadJSON, err := json.Marshal(struct {
		FeedURI types.FeedUri `json:"feedUri"`
		Post    types.Post    `json:"post"`
	}{
		FeedURI: params.FeedURI,
		Post:    post,
	})
	if err != nil {
		return fmt.Errorf("marshal add post payload: %w", err)
	}

	return c.transactor.WithinTx(ctx, func(ctx context.Context, feedRepo storerepo.FeedRepository, outboxRepo projectionrepo.OutboxRepository) error {
		if err := feedRepo.PutPost(ctx, storerepo.PutPostParams{
			FeedID: params.FeedID,
			Post:   post,
		}); err != nil {
			return err
		}
		if err := outboxRepo.Enqueue(ctx, projectionrepo.EnqueueParams{
			FeedID:      params.FeedID,
			FeedURI:     string(params.FeedURI),
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  mutationID,
			SubjectKey:  subjectKey,
			OpKey:       opKey,
			PayloadJSON: string(payloadJSON),
			Status:      "pending",
		}); err != nil {
			return err
		}
		if params.TrimAt > 0 {
			trimmedPosts, err := feedRepo.TrimOverflow(ctx, storerepo.TrimOverflowParams{
				FeedID: params.FeedID,
				TrimAt: params.TrimAt,
				Remain: params.TrimRemain,
			})
			if err != nil {
				return err
			}
			for _, trimmedPost := range trimmedPosts {
				trimmedSubjectKey := fmt.Sprintf("%s:%s", params.FeedID, trimmedPost.Uri)
				trimmedOpKey := fmt.Sprintf("%s:delete:%s", mutationID, trimmedSubjectKey)
				trimmedPayloadJSON, err := json.Marshal(struct {
					FeedURI types.FeedUri `json:"feedUri"`
					Post    types.Post    `json:"post"`
				}{
					FeedURI: params.FeedURI,
					Post:    trimmedPost,
				})
				if err != nil {
					return fmt.Errorf("marshal trimmed delete payload: %w", err)
				}
				if err := outboxRepo.Enqueue(ctx, projectionrepo.EnqueueParams{
					FeedID:      params.FeedID,
					FeedURI:     string(params.FeedURI),
					Target:      "gyoka",
					Operation:   "delete",
					MutationID:  mutationID,
					SubjectKey:  trimmedSubjectKey,
					OpKey:       trimmedOpKey,
					PayloadJSON: string(trimmedPayloadJSON),
					Status:      "pending",
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (c *FeedMutationCoordinator) DeletePost(ctx context.Context, params DeletePostParams) error {
	if c == nil || c.transactor == nil {
		return fmt.Errorf("feed mutation transactor is required")
	}
	mutationID := params.MutationID
	if mutationID == "" {
		mutationID = fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}

	subjectKey := fmt.Sprintf("%s:%s", params.FeedID, params.Post.Uri)
	opKey := fmt.Sprintf("%s:delete:%s", mutationID, subjectKey)
	payloadJSON, err := json.Marshal(struct {
		FeedURI types.FeedUri `json:"feedUri"`
		Post    types.Post    `json:"post"`
	}{
		FeedURI: params.FeedURI,
		Post:    params.Post,
	})
	if err != nil {
		return fmt.Errorf("marshal delete post payload: %w", err)
	}

	return c.transactor.WithinTx(ctx, func(ctx context.Context, feedRepo storerepo.FeedRepository, outboxRepo projectionrepo.OutboxRepository) error {
		if err := feedRepo.DeletePost(ctx, storerepo.DeletePostParams{
			FeedID:  params.FeedID,
			PostURI: params.Post.Uri,
		}); err != nil {
			return err
		}
		if err := outboxRepo.Enqueue(ctx, projectionrepo.EnqueueParams{
			FeedID:      params.FeedID,
			FeedURI:     string(params.FeedURI),
			Target:      "gyoka",
			Operation:   "delete",
			MutationID:  mutationID,
			SubjectKey:  subjectKey,
			OpKey:       opKey,
			PayloadJSON: string(payloadJSON),
			Status:      "pending",
		}); err != nil {
			return err
		}
		return nil
	})
}
