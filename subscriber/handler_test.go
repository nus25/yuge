package subscriber

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/nus25/yuge/feed/config/provider"
	cfgTypes "github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/metrics"
	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	"github.com/nus25/yuge/types"
)

type blockingDeleteCoordinator struct {
	inner         PostMutationCoordinator
	deleteStarted chan struct{}
	deleteRelease chan struct{}
	startedOnce   bool
}

func (c *blockingDeleteCoordinator) AddPost(ctx context.Context, params AddPostParams) error {
	return c.inner.AddPost(ctx, params)
}

func (c *blockingDeleteCoordinator) DeletePost(ctx context.Context, params DeletePostParams) error {
	if !c.startedOnce {
		c.startedOnce = true
		close(c.deleteStarted)
		<-c.deleteRelease
	}
	return c.inner.DeletePost(ctx, params)
}

type spyPostMutationCoordinator struct {
	addPostErr       error
	addPostCalls     int
	lastAddParams    AddPostParams
	deletePostErr    error
	deletePostCalls  int
	lastDeleteParams DeletePostParams
	sequence         *[]string
}

func (s *spyPostMutationCoordinator) AddPost(ctx context.Context, params AddPostParams) error {
	s.addPostCalls++
	s.lastAddParams = params
	if s.sequence != nil {
		*s.sequence = append(*s.sequence, "coordinator")
	}
	return s.addPostErr
}

func (s *spyPostMutationCoordinator) DeletePost(ctx context.Context, params DeletePostParams) error {
	s.deletePostCalls++
	s.lastDeleteParams = params
	if s.sequence != nil {
		*s.sequence = append(*s.sequence, "coordinator")
	}
	return s.deletePostErr
}

type fakeHandlerFeed struct {
	feedID          string
	feedURI         string
	config          cfgTypes.FeedConfig
	addPostErr      error
	addPostCalls    int
	deletePostErr   error
	deletePostCalls int
	lastDid         string
	lastRkey        string
	lastCID         string
	lastTime        time.Time
	lastLangs       []string
	lastDeletedDid  string
	lastDeletedRkey string
	sequence        *[]string
}

func (f *fakeHandlerFeed) FeedId() string  { return f.feedID }
func (f *fakeHandlerFeed) FeedUri() string { return f.feedURI }
func (f *fakeHandlerFeed) AddPost(did string, rkey string, cid string, t time.Time, langs []string) error {
	f.addPostCalls++
	f.lastDid = did
	f.lastRkey = rkey
	f.lastCID = cid
	f.lastTime = t
	f.lastLangs = append([]string(nil), langs...)
	if f.sequence != nil {
		*f.sequence = append(*f.sequence, "feed")
	}
	return f.addPostErr
}
func (f *fakeHandlerFeed) DeletePost(did string, rkey string) error {
	f.deletePostCalls++
	f.lastDeletedDid = did
	f.lastDeletedRkey = rkey
	if f.sequence != nil {
		*f.sequence = append(*f.sequence, "feed")
	}
	return f.deletePostErr
}
func (f *fakeHandlerFeed) DeletePostByDid(did string) ([]types.Post, error) { return nil, nil }
func (f *fakeHandlerFeed) GetPost(did string, rkey string) (types.Post, bool) {
	return types.Post{}, false
}
func (f *fakeHandlerFeed) ListPost(did string) []types.Post                          { return nil }
func (f *fakeHandlerFeed) Test(did string, rkey string, post *apibsky.FeedPost) bool { return true }
func (f *fakeHandlerFeed) PostCount() int                                            { return 0 }
func (f *fakeHandlerFeed) Shutdown(ctx context.Context) error                        { return nil }
func (f *fakeHandlerFeed) Clear() error                                              { return nil }
func (f *fakeHandlerFeed) Config() cfgTypes.FeedConfig                               { return f.config }
func (f *fakeHandlerFeed) Metrics() *metrics.Metrics                                 { return nil }
func (f *fakeHandlerFeed) ProcessCommand(logicBlockName string, command string, args map[string]string) (string, error) {
	return "", nil
}

func TestHandlePostEvent(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.Default()
	fs, err := NewFeedService("", tmpDir, nil, nil, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	h := &Handler{
		logger:      logger,
		FeedService: fs,
	}

	tests := []struct {
		name       string
		event      *models.Event
		wantErr    bool
		shouldAdd  bool
		operation  string
		collection string
		postText   string
		isReply    bool
	}{
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
		{
			name: "nil commit",
			event: &models.Event{
				Commit: nil,
			},
			wantErr: false,
		},
		{
			name: "non-post collection",
			event: &models.Event{
				Commit: &models.Commit{
					Collection: "app.bsky.feed.like",
				},
			},
			wantErr: false,
		},
		{
			name: "post collection",
			event: &models.Event{
				Commit: &models.Commit{
					Collection: "app.bsky.feed.post",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.HandlePostEvent(context.Background(), tt.event)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestHandlerAddAcceptedPost(t *testing.T) {
	logger := slog.Default()
	evt := &models.Event{
		Did: "did:plc:user1",
		Commit: &models.Commit{
			RKey: "post1",
			CID:  "cid-1",
		},
	}
	post := &apibsky.FeedPost{Langs: []string{"ja", "en"}}

	t.Run("coordinator persists before cache update", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "handler-test-config.yaml")
		if err := os.WriteFile(configPath, []byte(testConfig), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		cfgProvider, err := provider.NewFileFeedConfigProvider(configPath)
		if err != nil {
			t.Fatalf("NewFileFeedConfigProvider() error = %v", err)
		}
		sequence := make([]string, 0, 2)
		coordinator := &spyPostMutationCoordinator{sequence: &sequence}
		feed := &fakeHandlerFeed{
			feedID:   "feed-1",
			feedURI:  "at://did:plc:test/app.bsky.feed.generator/sample",
			config:   cfgProvider.FeedConfig(),
			sequence: &sequence,
		}
		h := &Handler{
			logger:              logger,
			MutationCoordinator: coordinator,
		}

		runErr := h.addAcceptedPost(context.Background(), feed.FeedId(), feed, evt, post)
		if runErr != nil {
			t.Fatalf("addAcceptedPost() error = %v", runErr)
		}
		if coordinator.addPostCalls != 1 {
			t.Fatalf("coordinator AddPost() calls = %d, want 1", coordinator.addPostCalls)
		}
		if feed.addPostCalls != 1 {
			t.Fatalf("feed AddPost() calls = %d, want 1", feed.addPostCalls)
		}
		if !reflect.DeepEqual(sequence, []string{"coordinator", "feed"}) {
			t.Fatalf("call sequence = %v, want [coordinator feed]", sequence)
		}
		if coordinator.lastAddParams.FeedID != feed.feedID {
			t.Fatalf("coordinator FeedID = %q, want %q", coordinator.lastAddParams.FeedID, feed.feedID)
		}
		if coordinator.lastAddParams.FeedURI != types.FeedUri(feed.feedURI) {
			t.Fatalf("coordinator FeedURI = %q, want %q", coordinator.lastAddParams.FeedURI, feed.feedURI)
		}
		if coordinator.lastAddParams.Did != evt.Did || coordinator.lastAddParams.Rkey != evt.Commit.RKey || coordinator.lastAddParams.Cid != evt.Commit.CID {
			t.Fatalf("coordinator params = %+v, want did/rkey/cid from event", coordinator.lastAddParams)
		}
		if coordinator.lastAddParams.TrimAt != 24 || coordinator.lastAddParams.TrimRemain != 20 {
			t.Fatalf("coordinator trim params = (%d, %d), want (24, 20)", coordinator.lastAddParams.TrimAt, coordinator.lastAddParams.TrimRemain)
		}
		if !reflect.DeepEqual(feed.lastLangs, post.Langs) {
			t.Fatalf("feed langs = %v, want %v", feed.lastLangs, post.Langs)
		}
		if feed.lastTime.IsZero() {
			t.Fatal("feed AddPost() time is zero")
		}
	})

	t.Run("coordinator failure skips cache update", func(t *testing.T) {
		coordinator := &spyPostMutationCoordinator{addPostErr: errors.New("boom")}
		feed := &fakeHandlerFeed{
			feedID:  "feed-1",
			feedURI: "at://did:plc:test/app.bsky.feed.generator/sample",
		}
		h := &Handler{
			logger:              logger,
			MutationCoordinator: coordinator,
		}

		err := h.addAcceptedPost(context.Background(), feed.FeedId(), feed, evt, post)
		if err == nil {
			t.Fatal("expected addAcceptedPost to fail")
		}
		if feed.addPostCalls != 0 {
			t.Fatalf("feed AddPost() calls = %d, want 0", feed.addPostCalls)
		}
	})
}

func TestHandlerAddAcceptedPost_WaitsForClearFeedOnSameFeed(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	sampleConfigPath := filepath.Join(configDir, "sample.yaml")
	if err := os.WriteFile(sampleConfigPath, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write sample config: %v", err)
	}

	ctx := context.Background()
	persistence, err := openSQLiteRuntimePersistence(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	defer func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()

	service, err := NewFeedService(configDir, dataDir, nil, nil, logger)
	if err != nil {
		t.Fatalf("NewFeedService() error = %v", err)
	}
	service.SetStoreLoader(persistence.postLoader)
	blockingCoordinator := &blockingDeleteCoordinator{
		inner:         persistence.mutationCoordinator,
		deleteStarted: make(chan struct{}),
		deleteRelease: make(chan struct{}),
	}
	service.SetMutationCoordinator(blockingCoordinator)

	definition := FeedDefinition{
		ID:         "feed-1",
		URI:        "at://did:plc:test/app.bsky.feed.generator/sample",
		ConfigFile: "sample.yaml",
	}
	seedPost := types.Post{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:seed/app.bsky.feed.post/post1"),
		Cid:       "cid-seed",
		IndexedAt: "2026-05-11T10:00:00Z",
		Langs:     []string{"ja"},
	}
	if err := storesqlite.NewFeedRepository(persistence.mutationDB).PutPost(ctx, storerepo.PutPostParams{FeedID: definition.ID, Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}
	if err := service.CreateFeed(ctx, definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}
	info, exists := service.GetFeedInfo(definition.ID)
	if !exists {
		t.Fatal("expected feed info to exist")
	}

	h := &Handler{
		logger:              logger,
		FeedService:         service,
		MutationCoordinator: blockingCoordinator,
	}
	clearErrCh := make(chan error, 1)
	go func() {
		clearErrCh <- service.ClearFeed(ctx, definition.ID)
	}()

	select {
	case <-blockingCoordinator.deleteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ClearFeed to start deleting persisted posts")
	}

	addErrCh := make(chan error, 1)
	go func() {
		addErrCh <- h.addAcceptedPost(ctx, definition.ID, info.Feed, &models.Event{
			Did: "did:plc:new",
			Commit: &models.Commit{
				RKey: "post2",
				CID:  "cid-new",
			},
		}, &apibsky.FeedPost{Langs: []string{"en"}})
	}()

	select {
	case err := <-addErrCh:
		t.Fatalf("addAcceptedPost() completed before ClearFeed released its gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(blockingCoordinator.deleteRelease)
	if err := <-clearErrCh; err != nil {
		t.Fatalf("ClearFeed() error = %v", err)
	}
	if err := <-addErrCh; err != nil {
		t.Fatalf("addAcceptedPost() error = %v", err)
	}

	posts, err := storesqlite.NewFeedRepository(persistence.loaderDB).ListPosts(ctx, storerepo.ListPostsParams{FeedID: definition.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("persisted posts len = %d, want 1", len(posts))
	}
	if posts[0].Uri != types.PostUri("at://did:plc:new/app.bsky.feed.post/post2") {
		t.Fatalf("persisted post uri = %s, want new post", posts[0].Uri)
	}
	cachePosts := info.Feed.ListPost("")
	if len(cachePosts) != 1 {
		t.Fatalf("cache posts len = %d, want 1", len(cachePosts))
	}
	if cachePosts[0].Uri != posts[0].Uri {
		t.Fatalf("cache post uri = %s, want %s", cachePosts[0].Uri, posts[0].Uri)
	}
}

func TestHandlerDeleteAcceptedPost(t *testing.T) {
	logger := slog.Default()
	evt := &models.Event{
		Did: "did:plc:user1",
		Commit: &models.Commit{
			RKey: "post1",
		},
	}
	post := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 5, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja", "en"},
	}

	t.Run("coordinator persists delete before cache update", func(t *testing.T) {
		sequence := make([]string, 0, 2)
		coordinator := &spyPostMutationCoordinator{sequence: &sequence}
		feed := &fakeHandlerFeed{
			feedID:   "feed-1",
			feedURI:  "at://did:plc:test/app.bsky.feed.generator/sample",
			sequence: &sequence,
		}
		h := &Handler{logger: logger, MutationCoordinator: coordinator}

		err := h.deleteAcceptedPost(context.Background(), feed.FeedId(), feed, evt, post)
		if err != nil {
			t.Fatalf("deleteAcceptedPost() error = %v", err)
		}
		if coordinator.deletePostCalls != 1 {
			t.Fatalf("coordinator DeletePost() calls = %d, want 1", coordinator.deletePostCalls)
		}
		if feed.deletePostCalls != 1 {
			t.Fatalf("feed DeletePost() calls = %d, want 1", feed.deletePostCalls)
		}
		if !reflect.DeepEqual(sequence, []string{"coordinator", "feed"}) {
			t.Fatalf("call sequence = %v, want [coordinator feed]", sequence)
		}
		if coordinator.lastDeleteParams.FeedID != feed.feedID {
			t.Fatalf("coordinator FeedID = %q, want %q", coordinator.lastDeleteParams.FeedID, feed.feedID)
		}
		if coordinator.lastDeleteParams.Post.Uri != post.Uri {
			t.Fatalf("coordinator Post URI = %q, want %q", coordinator.lastDeleteParams.Post.Uri, post.Uri)
		}
		if feed.lastDeletedDid != evt.Did || feed.lastDeletedRkey != evt.Commit.RKey {
			t.Fatalf("feed delete args = %q/%q, want %q/%q", feed.lastDeletedDid, feed.lastDeletedRkey, evt.Did, evt.Commit.RKey)
		}
	})

	t.Run("coordinator failure skips cache delete", func(t *testing.T) {
		coordinator := &spyPostMutationCoordinator{deletePostErr: errors.New("boom")}
		feed := &fakeHandlerFeed{feedID: "feed-1", feedURI: "at://did:plc:test/app.bsky.feed.generator/sample"}
		h := &Handler{logger: logger, MutationCoordinator: coordinator}

		err := h.deleteAcceptedPost(context.Background(), feed.FeedId(), feed, evt, post)
		if err == nil {
			t.Fatal("expected deleteAcceptedPost to fail")
		}
		if feed.deletePostCalls != 0 {
			t.Fatalf("feed DeletePost() calls = %d, want 0", feed.deletePostCalls)
		}
	})
}
