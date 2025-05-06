package editor

import (
	"context"
	"time"

	"github.com/nus25/yuge/types"
)

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
	FeedUri   types.FeedUri
	Did       string
	Rkey      string
	Cid       string
	IndexedAt time.Time
	Langs     []string
}

type DeleteParams struct {
	FeedUri types.FeedUri
	Did     string
	Rkey    string
}

type TrimParams struct {
	FeedUri types.FeedUri
	Count   int
}

// StoreEditor はフィードの編集操作を定義する
type StoreEditor interface {
	Open(ctx context.Context) error

	Load(ctx context.Context, params LoadParams) ([]types.Post, error)
	Save(ctx context.Context, params SaveParams) error
	// Add はフィードに投稿を追加します
	Add(params PostParams) error

	// Delete はフィードから投稿を削除します
	Delete(params DeleteParams) error

	// Trim はフィードの投稿数を指定された数に制限します
	Trim(params TrimParams) error

	// Close はフィードエディタの接続を終了します
	Close(ctx context.Context) error
}
