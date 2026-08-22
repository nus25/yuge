package gyoka

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	client "github.com/nus25/gyoka-client/go-atproto"
	gyokaschema "github.com/nus25/gyoka-client/go-atproto/schema/gyoka"
)

type fakeGyokaAPI struct {
	pingErr          error
	addInput         *gyokaschema.FeedAddPost_Input
	batchAddInput    *gyokaschema.FeedBatchAddPosts_Input
	batchRemoveInput *gyokaschema.FeedBatchRemovePosts_Input
}

func (a *fakeGyokaAPI) Ping(context.Context) error { return a.pingErr }

func (a *fakeGyokaAPI) AddPost(_ context.Context, input *gyokaschema.FeedAddPost_Input) error {
	a.addInput = input
	return nil
}

func (a *fakeGyokaAPI) BatchAddPosts(_ context.Context, input *gyokaschema.FeedBatchAddPosts_Input) error {
	a.batchAddInput = input
	return nil
}

func (a *fakeGyokaAPI) BatchRemovePosts(_ context.Context, input *gyokaschema.FeedBatchRemovePosts_Input) error {
	a.batchRemoveInput = input
	return nil
}

func (a *fakeGyokaAPI) RemovePost(context.Context, *gyokaschema.FeedRemovePost_Input) error {
	return nil
}

func (a *fakeGyokaAPI) RemovePostByAuthor(context.Context, *gyokaschema.FeedRemovePostByAuthor_Input) error {
	return nil
}

func (a *fakeGyokaAPI) TrimFeed(context.Context, *gyokaschema.FeedTrimFeed_Input) error {
	return nil
}

func (a *fakeGyokaAPI) GetPosts(context.Context, string, string, int64) (*gyokaschema.FeedGetPosts_Output, error) {
	return nil, nil
}

func TestGyokaEditor_AddMapsATProtoLexiconInput(t *testing.T) {
	api := &fakeGyokaAPI{}
	editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))
	indexedAt := time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC)

	if err := editor.Add(PostParams{
		FeedUri:   "at://did:plc:feed/app.bsky.feed.generator/sample",
		Did:       "did:plc:author",
		Rkey:      "post-1",
		Cid:       "bafy-test",
		IndexedAt: indexedAt,
		Langs:     []string{"ja"},
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if api.addInput == nil {
		t.Fatal("AddPost() input is nil")
	}
	if api.addInput.Feed != "at://did:plc:feed/app.bsky.feed.generator/sample" {
		t.Errorf("feed = %q", api.addInput.Feed)
	}
	if api.addInput.Post.Uri != "at://did:plc:author/app.bsky.feed.post/post-1" {
		t.Errorf("post URI = %q", api.addInput.Post.Uri)
	}
	if api.addInput.Post.IndexedAt == nil || *api.addInput.Post.IndexedAt != indexedAt.Format(time.RFC3339Nano) {
		t.Errorf("indexedAt = %v, want %s", api.addInput.Post.IndexedAt, indexedAt.Format(time.RFC3339Nano))
	}
}

func TestClassifyGyokaError(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		wantRetryable bool
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, wantRetryable: false},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantRetryable: false},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantRetryable: true},
		{name: "service unavailable", statusCode: http.StatusServiceUnavailable, wantRetryable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyGyokaError(&client.APIError{StatusCode: tt.statusCode})
			var nonRetryable *NonRetryableError
			if gotRetryable := !errors.As(err, &nonRetryable); gotRetryable != tt.wantRetryable {
				t.Fatalf("retryable = %t, want %t", gotRetryable, tt.wantRetryable)
			}
		})
	}
}

func TestGyokaEditor_BatchAddRejectsOversizedBatch(t *testing.T) {
	entries := make([]PostParams, maxBatchSize+1)
	if err := newGyokaEditor(&fakeGyokaAPI{}, nil).BatchAdd(BatchPostParams{Entries: entries}); err == nil {
		t.Fatal("BatchAdd() error = nil, want batch size error")
	}
}

func TestGyokaEditor_BatchRemoveMapsATProtoLexiconInput(t *testing.T) {
	api := &fakeGyokaAPI{}
	editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))

	if err := editor.BatchRemove(BatchDeleteParams{Entries: []DeleteParams{
		{
			FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
			Did:     "did:plc:author1",
			Rkey:    "post-1",
		},
		{
			FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
			Did:     "did:plc:author2",
			Rkey:    "post-2",
		},
	}}); err != nil {
		t.Fatalf("BatchRemove() error = %v", err)
	}

	if api.batchRemoveInput == nil {
		t.Fatal("BatchRemovePosts() input is nil")
	}
	if len(api.batchRemoveInput.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(api.batchRemoveInput.Entries))
	}
	entry := api.batchRemoveInput.Entries[0]
	if entry.Feed != "at://did:plc:feed/app.bsky.feed.generator/sample" {
		t.Errorf("feed = %q", entry.Feed)
	}
	if len(entry.Posts) != 2 {
		t.Fatalf("posts len = %d, want 2", len(entry.Posts))
	}
	if entry.Posts[0].Uri != "at://did:plc:author1/app.bsky.feed.post/post-1" {
		t.Errorf("first post URI = %q", entry.Posts[0].Uri)
	}
	if entry.Posts[1].Uri != "at://did:plc:author2/app.bsky.feed.post/post-2" {
		t.Errorf("second post URI = %q", entry.Posts[1].Uri)
	}
}
