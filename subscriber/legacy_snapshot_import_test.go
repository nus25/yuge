package subscriber

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	"github.com/nus25/yuge/types"
)

func TestImportLegacyFileSnapshots_ImportsPostsAndHydratesFeed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll() config error = %v", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("MkdirAll() data error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sample.yaml"), []byte(testConfig), 0644); err != nil {
		t.Fatalf("WriteFile() config error = %v", err)
	}

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	definition := FeedDefinition{
		ID:         "legacy-feed",
		URI:        "at://did:plc:legacy/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}

	legacyPosts := []types.Post{{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 14, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}}
	legacyFeedDir := filepath.Join(dataDir, definition.ID)
	if err := os.MkdirAll(legacyFeedDir, 0755); err != nil {
		t.Fatalf("MkdirAll() legacy feed error = %v", err)
	}
	legacyPayload, err := json.Marshal(legacyPosts)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyFeedDir, legacyStoreSnapshotFileName), legacyPayload, 0644); err != nil {
		t.Fatalf("WriteFile() legacy snapshot error = %v", err)
	}

	persistence, err := openSQLiteRuntimePersistence(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	t.Cleanup(func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	result, err := importLegacyFileSnapshots(ctx, logger, provider, dataDir, persistence.mutationDB, importLegacyFileSnapshotsOptions{})
	if err != nil {
		t.Fatalf("importLegacyFileSnapshots() error = %v", err)
	}
	if result.ImportedPosts != 1 {
		t.Fatalf("ImportedPosts = %d, want 1", result.ImportedPosts)
	}
	if result.ImportedFeeds != 1 {
		t.Fatalf("ImportedFeeds = %d, want 1", result.ImportedFeeds)
	}

	service, err := NewFeedService(configDir, dataDir, provider, logger)
	if err != nil {
		t.Fatalf("NewFeedService() error = %v", err)
	}
	service.SetStoreLoader(persistence.postLoader)
	if err := service.LoadFeeds(ctx); err != nil {
		t.Fatalf("LoadFeeds() error = %v", err)
	}

	info, exists := service.GetFeedInfo(definition.ID)
	if !exists {
		t.Fatal("expected imported feed to exist after LoadFeeds")
	}
	posts := info.Feed.ListPost("")
	if len(posts) != 1 {
		t.Fatalf("ListPost() len = %d, want 1", len(posts))
	}
	if posts[0].Uri != legacyPosts[0].Uri {
		t.Fatalf("ListPost()[0].Uri = %s, want %s", posts[0].Uri, legacyPosts[0].Uri)
	}

	entries, err := projectionsqlite.NewOutboxRepository(persistence.loaderDB).ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("pending outbox entries len = %d, want 0 for hydrate-only import", len(entries))
	}
}

func TestImportLegacyFileSnapshots_EnqueueProjection_ReplaysImportedPosts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll() config error = %v", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("MkdirAll() data error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sample.yaml"), []byte(testConfig), 0644); err != nil {
		t.Fatalf("WriteFile() config error = %v", err)
	}

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	definition := FeedDefinition{
		ID:         "legacy-feed",
		URI:        "at://did:plc:legacy/app.bsky.feed.generator/test",
		ConfigFile: "sample.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}

	legacyPosts := []types.Post{{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 14, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}}
	legacyFeedDir := filepath.Join(dataDir, definition.ID)
	if err := os.MkdirAll(legacyFeedDir, 0755); err != nil {
		t.Fatalf("MkdirAll() legacy feed error = %v", err)
	}
	legacyPayload, err := json.Marshal(legacyPosts)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyFeedDir, legacyStoreSnapshotFileName), legacyPayload, 0644); err != nil {
		t.Fatalf("WriteFile() legacy snapshot error = %v", err)
	}

	persistence, err := openSQLiteRuntimePersistence(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	t.Cleanup(func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	result, err := importLegacyFileSnapshots(ctx, logger, provider, dataDir, persistence.mutationDB, importLegacyFileSnapshotsOptions{EnqueueProjection: true})
	if err != nil {
		t.Fatalf("importLegacyFileSnapshots() error = %v", err)
	}
	if result.EnqueuedProjectionOps != 1 {
		t.Fatalf("EnqueuedProjectionOps = %d, want 1", result.EnqueuedProjectionOps)
	}

	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/gyoka/ping":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"message": "Gyoka is available"})
		case "/api/feed/addPost":
			requests <- r.URL.Path
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"message": "success"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	runtime, err := startGyokaProjectionRuntime(ctx, logger, persistence.mutationDB, server.URL, gyokaProjectionRuntimeOptions{pollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("startGyokaProjectionRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := runtime.Close(shutdownCtx); err != nil {
			t.Fatalf("runtime.Close() error = %v", err)
		}
	})

	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Gyoka replay add request")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		completedEntries, err := projectionsqlite.NewOutboxRepository(persistence.loaderDB).ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "completed", Limit: 10})
		if err != nil {
			t.Fatalf("ListByStatus() error = %v", err)
		}
		if len(completedEntries) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("completed entries len = %d, want 1", len(completedEntries))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
