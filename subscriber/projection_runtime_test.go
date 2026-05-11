package subscriber

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}
