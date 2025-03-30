package editor

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/nus25/yuge/types"
)

func TestFileEditor(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	l := slog.Default()
	t.Run("basic operations", func(t *testing.T) {
		editor, err := NewFileEditor(dataDir, l)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if err := editor.Open(ctx); err != nil {
			t.Fatalf("failed to open editor: %v", err)
		}
		defer editor.Close(ctx)

		feed := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		did := "did:plc:test"
		rkey := "test1"
		cid := "bafyreia"
		now := time.Now()

		// Test Load
		_, err = editor.Load(ctx, LoadParams{
			FeedId:  "test",
			FeedUri: feed,
			Limit:   10,
		})
		if err != nil {
			t.Fatalf("failed to load posts: %v", err)
		}

		// Test Add
		err = editor.Add(PostParams{
			FeedUri:   feed,
			Did:       did,
			Rkey:      rkey,
			Cid:       cid,
			IndexedAt: now,
		})
		if err != nil {
			t.Fatalf("failed to add post: %v", err)
		}

		// Test Delete
		err = editor.Delete(DeleteParams{
			FeedUri: feed,
			Did:     did,
			Rkey:    rkey,
		})
		if err != nil {
			t.Fatalf("failed to delete post: %v", err)
		}
	})

	t.Run("trim posts", func(t *testing.T) {
		editor, err := NewFileEditor(dataDir, l)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if err := editor.Open(ctx); err != nil {
			t.Fatalf("failed to open editor: %v", err)
		}
		defer editor.Close(ctx)

		feed := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")

		// Add multiple posts
		for i := 0; i < 5; i++ {
			err := editor.Add(PostParams{
				FeedUri:   feed,
				Did:       "did:plc:test",
				Rkey:      fmt.Sprintf("test%d", i),
				Cid:       fmt.Sprintf("bafyreia%d", i),
				IndexedAt: time.Now(),
			})
			if err != nil {
				t.Fatalf("failed to add post: %v", err)
			}
		}

		// Trim to 3 posts
		err = editor.Trim(TrimParams{
			FeedUri: feed,
			Count:   3,
		})
		if err != nil {
			t.Fatalf("failed to trim posts: %v", err)
		}
	})

	t.Run("file persistence", func(t *testing.T) {
		editor, err := NewFileEditor(dataDir, l)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if err := editor.Open(ctx); err != nil {
			t.Fatalf("failed to open editor: %v", err)
		}

		feed := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		_, err = editor.Load(ctx, LoadParams{
			FeedId:  "test",
			FeedUri: feed,
			Limit:   10,
		})
		if err != nil {
			t.Fatalf("failed to load posts: %v", err)
		}

		// Add a post and save
		testDid := "did:plc:test"
		testRkey := "test1"
		testCid := "bafyreia"
		testIndexedAt := time.Now()
		posts := []types.Post{
			{
				Feed:      feed,
				Uri:       types.PostUri(fmt.Sprintf("at://%s/app.bsky.feed.post/%s", testDid, testRkey)),
				Cid:       testCid,
				IndexedAt: testIndexedAt.Format(time.RFC3339),
			},
			{
				Feed:      feed,
				Uri:       types.PostUri(fmt.Sprintf("at://%s/app.bsky.feed.post/%s", testDid, "wanttrim")),
				Cid:       "wanttrim",
				IndexedAt: testIndexedAt.Add(-10 * time.Minute).Format(time.RFC3339),
			},
		}

		err = editor.Add(PostParams{
			FeedUri:   feed,
			Did:       testDid,
			Rkey:      testRkey,
			Cid:       testCid,
			IndexedAt: testIndexedAt,
		})
		if err != nil {
			t.Fatalf("failed to add post: %v", err)
		}

		err = editor.Save(ctx, SaveParams{
			Posts:   posts,
			FeedUri: feed,
			FeedId:  "test",
		})
		if err != nil {
			t.Fatalf("failed to save posts: %v", err)
		}

		editor.Close(ctx)

		// Create new editor instance and verify data persists
		editor2, err := NewFileEditor(dataDir, l)
		if err != nil {
			t.Fatalf("failed to create second editor: %v", err)
		}
		if err := editor2.Open(ctx); err != nil {
			t.Fatalf("failed to open second editor: %v", err)
		}
		defer editor2.Close(ctx)

		//no Limit
		posts, err = editor2.Load(ctx, LoadParams{
			FeedId:  "test",
			FeedUri: feed,
			Limit:   2,
		})
		if err != nil {
			t.Fatalf("failed to load posts: %v", err)
		}
		if len(posts) != 2 {
			t.Errorf("expected 2 posts, got %d", len(posts))
		}

		//limit 1 post
		posts, err = editor2.Load(ctx, LoadParams{
			FeedId:  "test",
			FeedUri: feed,
			Limit:   1,
		})
		if err != nil {
			t.Fatalf("failed to load posts: %v", err)
		}
		if len(posts) != 1 {
			t.Errorf("expected 1 post, got %d", len(posts))
		}
		// 最新の投稿が最初に来ることを確認
		if posts[0].Cid != testCid {
			t.Errorf("expected Cid %s, got %s", testCid, posts[0].Cid)
		}
		if posts[0].IndexedAt != testIndexedAt.Format(time.RFC3339) {
			t.Errorf("expected IndexedAt %s, got %s", testIndexedAt.Format(time.RFC3339), posts[0].IndexedAt)
		}
	})
}
