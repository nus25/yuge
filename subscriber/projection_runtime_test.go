package subscriber

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
)

func TestStartGyokaProjectionRuntime_CompletesPendingEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	persistence, err := openSQLiteRuntimePersistence(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	t.Cleanup(func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	if err := persistence.mutationCoordinator.AddPost(ctx, AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    "at://did:plc:test/app.bsky.feed.generator/sample",
		Did:        "did:plc:user1",
		Rkey:       "post1",
		Cid:        "cid-1",
		IndexedAt:  time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
		Langs:      []string{"ja"},
		MutationID: "mutation-1",
	}); err != nil {
		t.Fatalf("AddPost() error = %v", err)
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

	runtime, err := startGyokaProjectionRuntime(ctx, slog.New(slog.NewTextHandler(testWriter{t}, nil)), persistence.mutationDB, server.URL, gyokaProjectionRuntimeOptions{
		pollInterval: 10 * time.Millisecond,
	})
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
		t.Fatal("timed out waiting for Gyoka add request")
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

func TestFeedService_ReloadFeed_DoesNotRestartGyokaProjectionRuntime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	persistence, err := openSQLiteRuntimePersistence(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	t.Cleanup(func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	var pingCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/gyoka/ping":
			atomic.AddInt32(&pingCount, 1)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"message": "Gyoka is available"})
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"message": "success"})
		}
	}))
	defer server.Close()

	runtime, err := startGyokaProjectionRuntime(ctx, slog.New(slog.NewTextHandler(testWriter{t}, nil)), persistence.mutationDB, server.URL, gyokaProjectionRuntimeOptions{
		pollInterval: 10 * time.Millisecond,
	})
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

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "sample.yaml"), []byte(testConfig), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
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
	service, err := NewFeedService(configDir, dataDir, provider, nil, logger)
	if err != nil {
		t.Fatalf("NewFeedService() error = %v", err)
	}
	service.SetStoreLoader(persistence.postLoader)
	if err := service.CreateFeed(ctx, definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}
	if err := service.ReloadFeed(ctx, definition.ID); err != nil {
		t.Fatalf("ReloadFeed() first error = %v", err)
	}
	if err := service.ReloadFeed(ctx, definition.ID); err != nil {
		t.Fatalf("ReloadFeed() second error = %v", err)
	}

	if got := atomic.LoadInt32(&pingCount); got != 1 {
		t.Fatalf("Gyoka ping count = %d, want 1", got)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}
