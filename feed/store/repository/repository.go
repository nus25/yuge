package repository

import (
	"context"

	"github.com/nus25/yuge/types"
)

type FeedState struct {
	FeedID         string
	FeedURI        types.FeedUri
	Status         string
	ConfigRevision string
	LastLoadedAt   string
	UpdatedAt      string
}

type PutPostParams struct {
	FeedID string
	Post   types.Post
}

type ListPostsParams struct {
	FeedID string
	Limit  int
}

type DeletePostParams struct {
	FeedID  string
	PostURI types.PostUri
}

type TrimOverflowParams struct {
	FeedID string
	TrimAt int
	Remain int
}

type PutFeedStateParams struct {
	State FeedState
}

type FeedRepository interface {
	PutPost(ctx context.Context, params PutPostParams) error
	ListPosts(ctx context.Context, params ListPostsParams) ([]types.Post, error)
	DeletePost(ctx context.Context, params DeletePostParams) error
	DeleteAllPosts(ctx context.Context, feedID string) error
	TrimOverflow(ctx context.Context, params TrimOverflowParams) ([]types.Post, error)
	PutFeedState(ctx context.Context, params PutFeedStateParams) error
	GetFeedState(ctx context.Context, feedID string) (FeedState, bool, error)
}
