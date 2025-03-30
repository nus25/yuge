package feed

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/feed"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/store/editor"
)

// Integration test for Feed
func TestFeedIntegration(t *testing.T) {
	// Create test configuration
	config := createTestConfig(t)

	// Create in-memory store
	dir := t.TempDir()
	fileEditor, err := editor.NewFileEditor(dir, slog.Default())
	if err != nil {
		t.Fatalf("Failed to create file editor: %v", err)
	}

	// Create Feed
	ctx := context.Background()
	feed, err := NewFeedWithOptions(ctx, "test-feed", "at://did:plc:test/app.bsky.feed.generator/test", FeedOptions{
		Config:      config,
		StoreEditor: fileEditor,
	})

	if err != nil {
		t.Fatalf("Failed to create feed: %v", err)
	}

	// feed uri
	uri := feed.FeedUri()
	if uri != "at://did:plc:test/app.bsky.feed.generator/test" {
		t.Errorf("Expected feed uri to be at://did:plc:test/app.bsky.feed.generator/test, got %v", uri)
	}

	// Add post
	err = feed.AddPost("did:plc:user1", "post1", "cid1", time.Now())
	if err != nil {
		t.Errorf("Failed to add post: %v", err)
	}

	// Check if post was added
	post, exists := feed.GetPost("did:plc:user1", "post1")
	if !exists {
		t.Error("Post should exist but doesn't")
	}
	if post.Uri != "at://did:plc:user1/app.bsky.feed.post/post1" {
		t.Errorf("Post data mismatch, got %v", post)
	}

	// Check post count
	if count := feed.PostCount(); count != 1 {
		t.Errorf("Expected post count to be 1, got %d", count)
	}

	// list post
	posts := feed.ListPost("did:plc:user1")
	if len(posts) != 1 {
		t.Errorf("Expected post count to be 1, got %d", len(posts))
	}

	// Delete post
	err = feed.DeletePost("did:plc:user1", "post1")
	if err != nil {
		t.Errorf("Failed to delete post: %v", err)
	}

	// Verify post doesn't exist after deletion
	_, exists = feed.GetPost("did:plc:user1", "post1")
	if exists {
		t.Error("Post should not exist after deletion")
	}

	// delete post by did
	err = feed.AddPost("did:plc:user1", "post1", "cid1", time.Now())
	err = feed.AddPost("did:plc:user2", "post2", "cid2", time.Now())
	err = feed.AddPost("did:plc:user2", "post3", "cid3", time.Now())
	deleted, err := feed.DeletePostByDid("did:plc:user2")
	if err != nil {
		t.Errorf("Failed to delete post: %v", err)
	}
	if len(deleted) != 2 {
		t.Errorf("Expected 2 posts to be deleted, got %d", len(deleted))
	}

	// Clear feed
	err = feed.Clear()
	if err != nil {
		t.Errorf("Failed to clear feed: %v", err)
	}

	// config
	cfg := feed.Config()
	if cfg == nil {
		t.Error("Config should not be nil")
	}
	blocks := cfg.FeedLogic().GetLogicBlockConfigs()
	if len(blocks) != 2 {
		t.Errorf("Expected 2 blocks, got %d", len(blocks))
	}

	// Shutdown
	err = feed.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown feed: %v", err)
	}
}

// Test for feed filtering
func TestFeedFiltering(t *testing.T) {
	// Create test configuration
	config := createTestConfig(t)

	// Create in-memory store
	dir := t.TempDir()
	fileEditor, err := editor.NewFileEditor(dir, slog.Default())
	if err != nil {
		t.Fatalf("Failed to create file editor: %v", err)
	}

	// Create Feed
	ctx := context.Background()
	feed, err := NewFeedWithOptions(ctx, "test-filter", "at://did:plc:test/app.bsky.feed.generator/filter", FeedOptions{
		Config:      config,
		StoreEditor: fileEditor,
	})

	if err != nil {
		t.Fatalf("Failed to create feed: %v", err)
	}

	// Create test post
	testPost := &apibsky.FeedPost{
		Text: "これはテスト投稿です。日本語テキスト。",
	}

	// Set reply property
	testPost.Reply = &apibsky.FeedPost_ReplyRef{}

	// Reply should be filtered out
	if feed.Test("did:plc:user1", "constantRkey", testPost) {
		t.Error("Reply post should be filtered out")
	}

	// Non-reply post
	testPost.Reply = nil
	testPost.Text = "これはテスト投稿です。日本語テキスト。"
	testPost.Langs = []string{"ja"}

	// Japanese text should pass
	if !feed.Test("did:plc:user1", "constantRkey", testPost) {
		t.Error("Japanese text post should pass the filter")
	}

	// English only post should be filtered
	testPost.Text = "This is an English only post."
	testPost.Langs = []string{"en"}
	if feed.Test("did:plc:user1", "constantRkey", testPost) {
		t.Error("English only post should be filtered out")
	}

	// Shutdown
	err = feed.Shutdown(ctx)
	if err != nil {
		t.Errorf("Failed to shutdown feed: %v", err)
	}
}

// Function to create test configuration
func createTestConfig(t *testing.T) types.FeedConfig {
	t.Helper()
	// Create config from JSON string
	jsonStr := `{
		"logic": {
			"blocks": [{
				"type": "remove",
				"options": {
					"subject": "item",
					"value": "reply"
				}
			},{
				"type": "remove",
				"options": {
					"subject": "language",
					"language": "ja",
					"operator": "!="
				}
			}]
		},
		"detailedLog": true
	}`

	feedConfig, err := feed.NewFeedConfigFromJSON(jsonStr)
	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal config: %v", err))
	}

	return feedConfig
}
