package gyoka

import (
	"time"

	"github.com/nus25/yuge/types"
)

var ErrInvalidPostReason = types.ErrInvalidPostReason

type LoadParams struct {
	FeedId  string
	FeedUri types.FeedUri
	Limit   int
}

type SaveParams struct {
	Posts   []types.Post
	FeedId  string
	FeedUri types.FeedUri
}

type PostParams struct {
	FeedUri     types.FeedUri
	Did         string
	Rkey        string
	Cid         string
	IndexedAt   time.Time
	Langs       []string
	FeedContext *string
	Reason      *PostReason
}

type PostReason = types.PostReason

type BatchPostParams struct {
	Entries []PostParams
}

type DeleteParams struct {
	FeedUri types.FeedUri
	Did     string
	Rkey    string
}

type BatchDeleteParams struct {
	Entries []DeleteParams
}

type DeleteByDidParams struct {
	FeedUri types.FeedUri
	Did     string
}

type TrimParams struct {
	FeedUri types.FeedUri
	Count   int
}
