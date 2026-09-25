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
	"github.com/nus25/yuge/feed/logicblock"
	"github.com/nus25/yuge/feed/store"
	feedtypes "github.com/nus25/yuge/types"
)

type memoryLogicBlockStateRepository struct {
	states map[string][]byte
}

func (r *memoryLogicBlockStateRepository) LoadLogicBlockState(_ context.Context, feedID string, blockKey string) ([]byte, bool, error) {
	state, found := r.states[feedID+":"+blockKey]
	return state, found, nil
}

func (r *memoryLogicBlockStateRepository) SaveLogicBlockState(_ context.Context, feedID string, blockKey string, _ int, state []byte) error {
	r.states[feedID+":"+blockKey] = append([]byte(nil), state...)
	return nil
}

func (r *memoryLogicBlockStateRepository) DeleteLogicBlockState(_ context.Context, feedID string, blockKey string) error {
	delete(r.states, feedID+":"+blockKey)
	return nil
}

type plannedDeleteLogicBlock struct {
	mutations []logicblock.PostMutation
	observed  []feedtypes.Post
}

func (b *plannedDeleteLogicBlock) BlockType() string { return "test" }
func (b *plannedDeleteLogicBlock) BlockName() string { return "test" }
func (b *plannedDeleteLogicBlock) Config() types.LogicBlockConfig {
	return nil
}
func (b *plannedDeleteLogicBlock) Logger() *slog.Logger { return slog.Default() }
func (b *plannedDeleteLogicBlock) Test(string, string, *apibsky.FeedPost) bool {
	return true
}
func (b *plannedDeleteLogicBlock) Reset() error                   { return nil }
func (b *plannedDeleteLogicBlock) Shutdown(context.Context) error { return nil }
func (b *plannedDeleteLogicBlock) HandlePreDelete(posts logicblock.PostStore, did string, rkey string) ([]logicblock.PostMutation, error) {
	b.observed = posts.List("")
	return b.mutations, nil
}

// Integration test for Feed
func TestFeedIntegration(t *testing.T) {
	// Create test configuration
	config := createTestConfig(t)

	// Create Feed
	ctx := context.Background()
	feed, err := NewFeedWithOptions(ctx, "test-feed", "at://did:plc:test/app.bsky.feed.generator/test", FeedOptions{
		Config: config,
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
	err = feed.AddPost("did:plc:user1", "post1", "cid1", time.Now(), []string{"en", "fr"})
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
	err = feed.AddPost("did:plc:user1", "post1", "cid1", time.Now(), []string{"en", "fr"})
	if err != nil {
		t.Errorf("Failed to delete post: %v", err)
	}
	err = feed.AddPost("did:plc:user2", "post2", "cid2", time.Now(), []string{"jp"})
	if err != nil {
		t.Errorf("Failed to delete post: %v", err)
	}
	err = feed.AddPost("did:plc:user2", "post3", "cid3", time.Now(), nil)
	if err != nil {
		t.Errorf("Failed to delete post: %v", err)
	}
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

	// Trim feed
	for i := 0; i < 3; i++ {
		if err := feed.AddPost("did:plc:user1", fmt.Sprintf("trimpost%d", i), fmt.Sprintf("cid%d", i), time.Now(), nil); err != nil {
			t.Errorf("Failed to add post: %v", err)
		}
	}
	if err := feed.Trim(1); err != nil {
		t.Errorf("Failed to trim feed: %v", err)
	}
	if count := feed.PostCount(); count != 1 {
		t.Errorf("Expected post count to be 1 after trim, got %d", count)
	}
	if err := feed.Trim(-1); err == nil {
		t.Error("Expected error when trimming with negative remain")
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

func TestFeedRestoresDropInState(t *testing.T) {
	ctx := context.Background()
	const (
		feedID  = "stateful-feed"
		feedURI = "at://did:plc:test/app.bsky.feed.generator/stateful"
	)
	stateRepository := &memoryLogicBlockStateRepository{states: make(map[string][]byte)}
	configJSON := `{
		"logic": {
			"blocks": [{
				"name": "active-authors",
				"type": "dropin",
				"options": {
					"targetWord": ["hello"],
					"expireDuration": "1h"
				}
			}]
		}
	}`
	feedConfig, err := feed.NewFeedConfigFromJSON(configJSON)
	if err != nil {
		t.Fatalf("NewFeedConfigFromJSON() error = %v", err)
	}

	first, err := NewFeedWithOptions(ctx, feedID, feedURI, FeedOptions{
		Config:               feedConfig,
		LogicBlockStateStore: stateRepository,
	})
	if err != nil {
		t.Fatalf("first NewFeedWithOptions() error = %v", err)
	}
	if !first.Test("did:plc:author", "trigger", &apibsky.FeedPost{Text: "hello"}) {
		t.Fatal("trigger post did not pass drop-in block")
	}
	if err := first.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown() error = %v", err)
	}

	restarted, err := NewFeedWithOptions(ctx, feedID, feedURI, FeedOptions{
		Config:               feedConfig,
		LogicBlockStateStore: stateRepository,
	})
	if err != nil {
		t.Fatalf("restarted NewFeedWithOptions() error = %v", err)
	}
	defer restarted.Shutdown(ctx)
	if !restarted.Test("did:plc:author", "ordinary", &apibsky.FeedPost{Text: "ordinary post"}) {
		t.Fatal("watchlist state was not restored after feed recreation")
	}
}

func TestFeedPlanDelete_AppliesRelatedPostMutations(t *testing.T) {
	ctx := context.Background()
	feedURI := "at://did:plc:test/app.bsky.feed.generator/test"
	postStore, err := store.NewStore(ctx, store.StoreOptions{FeedId: "test-feed", FeedUri: feedtypes.FeedUri(feedURI)})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	block := &plannedDeleteLogicBlock{mutations: []logicblock.PostMutation{
		{
			Operation: logicblock.PostMutationAdd,
			Post: feedtypes.Post{
				Uri:       "at://did:plc:user1/app.bsky.feed.post/replacement",
				Cid:       "replacement-cid",
				IndexedAt: "2026-05-11T04:00:00Z",
			},
		},
		{
			Operation: logicblock.PostMutationDelete,
			Post:      feedtypes.Post{Uri: "at://did:plc:user1/app.bsky.feed.post/related"},
		},
	}}
	targetFeed := &feedImpl{
		id:          "test-feed",
		uri:         feedtypes.FeedUri(feedURI),
		store:       postStore,
		logicblocks: []logicblock.LogicBlock{block},
		logger:      slog.Default(),
	}
	for _, rkey := range []string{"target", "related"} {
		if err := targetFeed.AddPost("did:plc:user1", rkey, rkey+"-cid", time.Now(), nil); err != nil {
			t.Fatalf("AddPost(%s) error = %v", rkey, err)
		}
	}

	mutations, err := targetFeed.PlanDelete("did:plc:user1", "target")
	if err != nil {
		t.Fatalf("PlanDelete() error = %v", err)
	}
	if len(block.observed) != 2 {
		t.Fatalf("PostStore.List() count = %d, want 2", len(block.observed))
	}
	if len(mutations) != 3 || mutations[2].Operation != logicblock.PostMutationDelete {
		t.Fatalf("planned mutations = %+v, want related mutations followed by target delete", mutations)
	}
	if err := targetFeed.ApplyPostMutations(mutations); err != nil {
		t.Fatalf("ApplyPostMutations() error = %v", err)
	}
	if _, exists := targetFeed.GetPost("did:plc:user1", "target"); exists {
		t.Fatal("target post remains after applying delete plan")
	}
	if _, exists := targetFeed.GetPost("did:plc:user1", "related"); exists {
		t.Fatal("related post remains after applying delete plan")
	}
	replacement, exists := targetFeed.GetPost("did:plc:user1", "replacement")
	if !exists {
		t.Fatal("replacement post was not added")
	}
	if replacement.Feed != feedtypes.FeedUri(feedURI) {
		t.Fatalf("replacement feed = %s, want %s", replacement.Feed, feedURI)
	}
}

// Test for feed filtering
func TestFeedFiltering(t *testing.T) {
	// Create test configuration
	config := createTestConfig(t)

	// Create Feed
	ctx := context.Background()
	feed, err := NewFeedWithOptions(ctx, "test-filter", "at://did:plc:test/app.bsky.feed.generator/filter", FeedOptions{
		Config: config,
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
