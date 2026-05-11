package subscriber

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/feed"
	storepkg "github.com/nus25/yuge/feed/store"
	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	"github.com/nus25/yuge/types"
)

type blockingPostLoader struct {
	targetCalls int
	ready       chan struct{}
	release     chan struct{}

	mu    sync.Mutex
	calls int
	once  sync.Once
}

func newBlockingPostLoader(targetCalls int) *blockingPostLoader {
	return &blockingPostLoader{
		targetCalls: targetCalls,
		ready:       make(chan struct{}),
		release:     make(chan struct{}),
	}
}

type staticPostLoader struct {
	posts []types.Post
}

func (l *staticPostLoader) LoadPosts(ctx context.Context, params storepkg.LoadPostsParams) ([]types.Post, error) {
	posts := make([]types.Post, len(l.posts))
	copy(posts, l.posts)
	return posts, nil
}

func (l *blockingPostLoader) LoadPosts(ctx context.Context, params storepkg.LoadPostsParams) ([]types.Post, error) {
	l.mu.Lock()
	l.calls++
	if l.calls >= l.targetCalls {
		l.once.Do(func() { close(l.ready) })
	}
	l.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.release:
		return nil, nil
	}
}

func listPendingOutboxOperations(ctx context.Context, db *sql.DB, feedID string) ([]projectionrepo.Entry, error) {
	entries, err := projectionsqlite.NewOutboxRepository(db).ListByStatus(ctx, projectionrepo.ListByStatusParams{
		Target: "gyoka",
		Status: "pending",
		Limit:  128,
	})
	if err != nil {
		return nil, err
	}
	filtered := make([]projectionrepo.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.FeedID == feedID {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

// MockFeed implements feed.Feed for testing
type MockFeed struct {
	shutdownErr error
}

func (m *MockFeed) Shutdown(ctx context.Context) error {
	return m.shutdownErr
}

func (m *MockFeed) Close() error {
	return nil
}

func TestNewFeedService(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "feed-service-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	dp, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create feed definition provider: %v", err)
	}
	tests := []struct {
		name               string
		configDir          string
		dataDir            string
		definitionProvider FeedDefinitionProvider
		expectError        bool
	}{
		{
			name:               "正常なパラメータでの作成",
			configDir:          configDir,
			definitionProvider: dp,
			dataDir:            dataDir,
			expectError:        false,
		},
		{
			name:               "definitionProviderがnilでも生成できる",
			configDir:          configDir,
			definitionProvider: nil,
			dataDir:            dataDir,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewFeedService(tt.configDir, tt.dataDir, tt.definitionProvider, logger)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && service == nil {
				t.Error("Expected service to be created but got nil")
			}

			if !tt.expectError {
				if service.configDir != tt.configDir {
					t.Errorf("Expected configDir to be %s, got %s", tt.configDir, service.configDir)
				}
				if service.dataDir != tt.dataDir {
					t.Errorf("Expected dataDir to be %s, got %s", tt.dataDir, service.dataDir)
				}
				if service.feeds == nil {
					t.Error("Expected feeds map to be initialized")
				}
			}
		})
	}
}

func TestFeedService_Load(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "feed-service-load-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	p, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	tests := []struct {
		name        string
		provider    FeedDefinitionProvider
		expectError bool
	}{
		{
			name:        "success",
			provider:    p,
			expectError: false,
		},
		{
			name:        "provider is nil",
			provider:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewFeedService(configDir, dataDir, tt.provider, logger)
			if err != nil {
				t.Fatalf("Failed to create service: %v", err)
			}

			err = service.LoadFeeds(context.Background())

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && service.definitionProvider != tt.provider {
				t.Error("Expected definitionProvider to be set")
			}
		})
	}
}

func TestFeedService_LoadFeeds_HydratesPostsFromStoreLoader(t *testing.T) {
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

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	definition := FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(dataDir, "yuge.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	seedPost := types.Post{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: "2026-05-11T10:00:00Z",
		Langs:     []string{"ja"},
	}
	if err := storesqlite.NewFeedRepository(db).PutPost(ctx, storerepo.PutPostParams{FeedID: definition.ID, Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}

	service, err := NewFeedService(configDir, dataDir, provider, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	service.SetStoreLoader(newSQLitePostLoader(db))

	if err := service.LoadFeeds(ctx); err != nil {
		t.Fatalf("LoadFeeds() error = %v", err)
	}

	info, exists := service.GetFeedInfo(definition.ID)
	if !exists {
		t.Fatal("expected feed info to exist after LoadFeeds")
	}
	posts := info.Feed.ListPost("")
	if len(posts) != 1 {
		t.Fatalf("ListPost() len = %d, want 1", len(posts))
	}
	if posts[0].Uri != seedPost.Uri {
		t.Fatalf("ListPost()[0].Uri = %s, want %s", posts[0].Uri, seedPost.Uri)
	}
}

func TestFeedService_LoadFeeds_WithoutLoader_IgnoresLegacySnapshot(t *testing.T) {
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

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	definition := FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}

	ctx := context.Background()
	legacyFeedDir := filepath.Join(dataDir, definition.ID)
	if err := os.MkdirAll(legacyFeedDir, 0755); err != nil {
		t.Fatalf("MkdirAll() legacy feed dir error = %v", err)
	}
	legacyPosts := []types.Post{{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}}
	legacyPayload, err := json.Marshal(legacyPosts)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyFeedDir, "store.json"), legacyPayload, 0644); err != nil {
		t.Fatalf("WriteFile() legacy snapshot error = %v", err)
	}
	service, err := NewFeedService(configDir, dataDir, provider, logger)
	if err != nil {
		t.Fatalf("NewFeedService() error = %v", err)
	}
	if err := service.LoadFeeds(ctx); err != nil {
		t.Fatalf("LoadFeeds() error = %v", err)
	}
	reloadedInfo, exists := service.GetFeedInfo(definition.ID)
	if !exists {
		t.Fatal("expected feed info to exist after LoadFeeds")
	}
	posts := reloadedInfo.Feed.ListPost("")
	if len(posts) != 0 {
		t.Fatalf("ListPost() len = %d, want 0 without loader-backed import", len(posts))
	}
}

func TestFeedService_GetFeedInfo(t *testing.T) {
	// Setup
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
		},
	}

	tests := []struct {
		name      string
		feedId    string
		expectOk  bool
		expectFed FeedInfo
	}{
		{
			name:     "存在するフィード",
			feedId:   "feed1",
			expectOk: true,
			expectFed: FeedInfo{
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
		},
		{
			name:     "存在しないフィード",
			feedId:   "nonexistent",
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, exists := service.GetFeedInfo(tt.feedId)

			if exists != tt.expectOk {
				t.Errorf("Expected exists to be %v, got %v", tt.expectOk, exists)
			}

			if tt.expectOk && info.Definition.ID != tt.expectFed.Definition.ID {
				t.Errorf("Expected feed ID to be %s, got %s", tt.expectFed.Definition.ID, info.Definition.ID)
			}
		})
	}
}

func TestFeedService_GetFeedList(t *testing.T) {
	// Setup
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
			"feed2": {
				Definition: FeedDefinition{ID: "feed2"},
				Status:     FeedStatus{FeedID: "feed2", LastStatus: FeedStatusError},
			},
		},
	}

	list := service.GetActiveFeedIDs()

	if len(list) != 1 {
		t.Errorf("Expected 1 feeds, got %d", len(list))
	}

	// Check if all feeds are included
	foundFeed1 := false
	foundFeed2 := false
	for _, id := range list {
		if id == "feed1" {
			foundFeed1 = true
		}
		if id == "feed2" {
			foundFeed2 = true
		}
	}

	if !foundFeed1 {
		t.Error("Expected feed1 in list but not found")
	}
	if foundFeed2 {
		t.Error("Expected not found feed2 in list")
	}
}

func TestFeedService_GetAllFeeds_ReturnsSnapshot(t *testing.T) {
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
		},
	}

	snapshot := service.GetAllFeeds()
	delete(snapshot, "feed1")
	snapshot["feed2"] = FeedInfo{
		Definition: FeedDefinition{ID: "feed2"},
		Status:     FeedStatus{FeedID: "feed2", LastStatus: FeedStatusActive},
	}

	if _, exists := service.GetFeedInfo("feed1"); !exists {
		t.Fatal("expected feed1 to remain in service after mutating snapshot")
	}
	if _, exists := service.GetFeedInfo("feed2"); exists {
		t.Fatal("expected feed2 mutation on snapshot to not affect service state")
	}
}

func TestFeedService_CreateFeed(t *testing.T) {
	// This is a simplified test as actual implementation would require mocking feed.NewFeedWithOptions
	tempDir, err := os.MkdirTemp("", "feed-service-create-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// Create config directory and a sample config file
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	jsonStr := `
    {
        "logic":{"blocks":[
		{"type":"regex",
		"options":{"value":"[1-9]","invert":false,"caseSensitive":false}
		}
		]
		}
    }
    `
	cfg, err := feed.NewFeedConfigFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("Failed to create feed config: %v", err)
	}
	yamlStr, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal feed config: %v", err)
	}
	sampleConfigPath := filepath.Join(configDir, "sample.yaml")
	if err := os.WriteFile(sampleConfigPath, yamlStr, 0644); err != nil {
		t.Fatalf("Failed to write sample config: %v", err)
	}
	service, err := NewFeedService(configDir, dataDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	// Test cases
	tests := []struct {
		name        string
		definition  FeedDefinition
		status      Status
		expectError bool
	}{
		{
			name:        "新規フィード作成",
			definition:  FeedDefinition{ID: "new-feed", URI: "at://did:plc:1234567890/app.bsky.feed.generator/test", ConfigFile: "sample.yaml"},
			status:      FeedStatusActive,
			expectError: false,
		},
		{
			name:        "既存フィードID",
			definition:  FeedDefinition{ID: "new-feed", URI: "at://did:plc:1234567890/app.bsky.feed.generator/test", ConfigFile: "sample.yaml"},
			status:      FeedStatusActive,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.CreateFeed(context.Background(), tt.definition, tt.status)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError {
				info, exists := service.GetFeedInfo(tt.definition.ID)
				if !exists {
					t.Error("Expected feed to exist but not found")
				} else if info.Definition.ID != tt.definition.ID {
					t.Errorf("Expected feed ID to be %s, got %s", tt.definition.ID, info.Definition.ID)
				}
			}
		})
	}
}

func TestFeedService_CreateFeed_WithLoader_SucceedsWithoutLegacyEditor(t *testing.T) {
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

	service, err := NewFeedService(configDir, dataDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	service.SetStoreLoader(&staticPostLoader{})
	if err := service.CreateFeed(context.Background(), FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestFeedService_CreateFeed_WithoutLoader_UsesEmptyInMemoryStore(t *testing.T) {
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

	service, err := NewFeedService(configDir, dataDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	if err := service.CreateFeed(context.Background(), FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}
	info, exists := service.GetFeedInfo("new-feed")
	if !exists {
		t.Fatal("expected feed info to exist after CreateFeed")
	}
	if got := len(info.Feed.ListPost("")); got != 0 {
		t.Fatalf("ListPost() len = %d, want 0 for in-memory only feed", got)
	}
}

func TestFeedService_CreateFeed_SerializesSameFeedID(t *testing.T) {
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

	service, err := NewFeedService(configDir, dataDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	loader := newBlockingPostLoader(1)
	service.SetStoreLoader(loader)

	definition := FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- service.CreateFeed(context.Background(), definition, FeedStatusActive)
	}()

	select {
	case <-loader.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first CreateFeed call to reach loader barrier")
	}
	go func() {
		errCh <- service.CreateFeed(context.Background(), definition, FeedStatusActive)
	}()
	close(loader.release)

	var errs []error
	for range 2 {
		errs = append(errs, <-errCh)
	}

	var successCount int
	var alreadyExistsCount int
	for _, err := range errs {
		if err == nil {
			successCount++
			continue
		}
		if strings.Contains(err.Error(), "already exists") {
			alreadyExistsCount++
			continue
		}
		t.Fatalf("unexpected CreateFeed() error = %v", err)
	}
	if successCount != 1 || alreadyExistsCount != 1 {
		t.Fatalf("CreateFeed() results = %v, want 1 success and 1 already exists error", errs)
	}
	if loader.calls != 1 {
		t.Fatalf("loader LoadPosts() calls = %d, want 1", loader.calls)
	}
	if _, exists := service.GetFeedInfo("new-feed"); !exists {
		t.Fatal("expected feed to exist after concurrent CreateFeed calls")
	}
}

func TestFeedService_CreateFeed_HydratesPostsFromStoreLoader(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "feed-service-create-hydrate-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

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
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(dataDir, "yuge.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repo := storesqlite.NewFeedRepository(db)
	seedPost := types.Post{
		Feed:      types.FeedUri("at://did:plc:1234567890/app.bsky.feed.generator/test"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: "2026-05-11T10:00:00Z",
		Langs:     []string{"ja"},
	}
	if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: "new-feed", Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}

	service, err := NewFeedService(configDir, dataDir, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	service.SetStoreLoader(newSQLitePostLoader(db))

	err = service.CreateFeed(context.Background(), FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}, FeedStatusActive)
	if err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}

	info, exists := service.GetFeedInfo("new-feed")
	if !exists {
		t.Fatal("expected feed info to exist")
	}
	posts := info.Feed.ListPost("")
	if len(posts) != 1 {
		t.Fatalf("ListPost() len = %d, want 1", len(posts))
	}
	if posts[0].Uri != seedPost.Uri {
		t.Fatalf("ListPost()[0].Uri = %s, want %s", posts[0].Uri, seedPost.Uri)
	}
}

func TestFeedService_ReloadFeed_SerializesSameFeedID(t *testing.T) {
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

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	service, err := NewFeedService(configDir, dataDir, provider, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}

	definition := FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}
	if err := service.CreateFeed(context.Background(), definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}

	loader := newBlockingPostLoader(1)
	service.SetStoreLoader(loader)

	errCh := make(chan error, 2)
	go func() {
		errCh <- service.ReloadFeed(context.Background(), "new-feed")
	}()

	select {
	case <-loader.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first ReloadFeed call to reach loader barrier")
	}
	go func() {
		errCh <- service.ReloadFeed(context.Background(), "new-feed")
	}()
	close(loader.release)

	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("ReloadFeed() error = %v", err)
		}
	}
	if loader.calls != 2 {
		t.Fatalf("loader LoadPosts() calls during reload = %d, want 2", loader.calls)
	}
	if _, exists := service.GetFeedInfo("new-feed"); !exists {
		t.Fatal("expected feed to exist after concurrent ReloadFeed calls")
	}
}

func TestFeedService_ClearFeed_RemovesPersistedPostsBeforeReload(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "feed-service-clear-reload-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

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
	db, coordinator, err := openSQLiteMutationCoordinator(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteMutationCoordinator() error = %v", err)
	}
	defer db.Close()

	repo := storesqlite.NewFeedRepository(db)
	seedPost := types.Post{
		Feed:      types.FeedUri("at://did:plc:1234567890/app.bsky.feed.generator/test"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: "2026-05-11T10:00:00Z",
		Langs:     []string{"ja"},
	}
	if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: "new-feed", Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	service, err := NewFeedService(configDir, dataDir, provider, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	service.SetStoreLoader(newSQLitePostLoader(db))
	service.SetMutationCoordinator(coordinator)

	definition := FeedDefinition{
		ID:         "new-feed",
		URI:        "at://did:plc:1234567890/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}
	if err := service.CreateFeed(ctx, definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}

	info, exists := service.GetFeedInfo("new-feed")
	if !exists {
		t.Fatal("expected feed info to exist")
	}
	if got := len(info.Feed.ListPost("")); got != 1 {
		t.Fatalf("initial ListPost() len = %d, want 1", got)
	}

	if err := service.ClearFeed(ctx, "new-feed"); err != nil {
		t.Fatalf("ClearFeed() error = %v", err)
	}
	if got := len(info.Feed.ListPost("")); got != 0 {
		t.Fatalf("ListPost() len after ClearFeed = %d, want 0", got)
	}

	persistedPosts, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "new-feed"})
	if err != nil {
		t.Fatalf("ListPosts() after ClearFeed error = %v", err)
	}
	if got := len(persistedPosts); got != 0 {
		t.Fatalf("persisted ListPosts() len after ClearFeed = %d, want 0", got)
	}

	if err := service.ReloadFeed(ctx, "new-feed"); err != nil {
		t.Fatalf("ReloadFeed() error = %v", err)
	}
	reloaded, exists := service.GetFeedInfo("new-feed")
	if !exists {
		t.Fatal("expected reloaded feed info to exist")
	}
	if got := len(reloaded.Feed.ListPost("")); got != 0 {
		t.Fatalf("ListPost() len after ReloadFeed = %d, want 0", got)
	}
	if status, err := listPendingOutboxOperations(ctx, db, "new-feed"); err != nil {
		t.Fatalf("listPendingOutboxOperations() error = %v", err)
	} else if got := len(status); got != 1 {
		t.Fatalf("pending outbox operation count = %d, want 1", got)
	}
}

func TestFeedService_DeleteFeed(t *testing.T) {
	// Setup

	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1"},
			},
			"feed2": {
				Definition: FeedDefinition{ID: "feed2"},
				Status:     FeedStatus{FeedID: "feed2"},
			},
		},
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		name        string
		feedId      string
		expectError bool
	}{
		{
			name:        "正常な削除",
			feedId:      "feed1",
			expectError: false,
		},
		{
			name:        "シャットダウンエラーがあっても削除",
			feedId:      "feed2",
			expectError: false,
		},
		{
			name:        "存在しないフィード",
			feedId:      "nonexistent",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clone the service for each test to avoid state interference
			testService := &FeedService{
				feeds:  make(map[string]FeedInfo),
				logger: service.logger,
			}
			maps.Copy(testService.feeds, service.feeds)

			err := testService.DeleteFeed(tt.feedId)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && tt.feedId != "nonexistent" {
				_, exists := testService.GetFeedInfo(tt.feedId)
				if exists {
					t.Error("Expected feed to be deleted but still exists")
				}
			}
		})
	}
}

func TestFeedService_UpdateStatus(t *testing.T) {
	// Setup
	now := time.Now().Add(-1 * time.Hour)
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status: FeedStatus{
					FeedID:      "feed1",
					LastStatus:  FeedStatusActive,
					LastUpdated: now,
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		name        string
		feedId      string
		status      Status
		expectError bool
	}{
		{
			name:        "ステータス更新",
			feedId:      "feed1",
			status:      FeedStatusError,
			expectError: false,
		},
		{
			name:        "存在しないフィード",
			feedId:      "nonexistent",
			status:      FeedStatusActive,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.UpdateStatus(tt.feedId, tt.status)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError {
				info, exists := service.GetFeedInfo(tt.feedId)
				if !exists {
					t.Error("Expected feed to exist but not found")
				} else {
					if info.Status.LastStatus != tt.status {
						t.Errorf("Expected status to be %v, got %v", tt.status, info.Status.LastStatus)
					}
					if !info.Status.LastUpdated.After(now) {
						t.Error("Expected LastUpdated to be updated")
					}
				}
			}
		})
	}
}

func TestFeedService_GetFeedStatus(t *testing.T) {
	// Setup
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status: FeedStatus{
					FeedID:     "feed1",
					LastStatus: FeedStatusActive,
				},
			},
		},
	}

	tests := []struct {
		name      string
		feedId    string
		expectOk  bool
		expectSts FeedStatus
	}{
		{
			name:     "存在するフィード",
			feedId:   "feed1",
			expectOk: true,
			expectSts: FeedStatus{
				FeedID:     "feed1",
				LastStatus: FeedStatusActive,
			},
		},
		{
			name:     "存在しないフィード",
			feedId:   "nonexistent",
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, exists := service.GetFeedStatus(tt.feedId)

			if exists != tt.expectOk {
				t.Errorf("Expected exists to be %v, got %v", tt.expectOk, exists)
			}

			if tt.expectOk {
				if status.FeedID != tt.expectSts.FeedID {
					t.Errorf("Expected FeedID to be %s, got %s", tt.expectSts.FeedID, status.FeedID)
				}
				if status.LastStatus != tt.expectSts.LastStatus {
					t.Errorf("Expected LastStatus to be %v, got %v", tt.expectSts.LastStatus, status.LastStatus)
				}
			}
		})
	}
}
