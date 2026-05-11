package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
)

func TestOutboxRepositoryEnqueueAndListByStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewOutboxRepository(db)
	params := projectionrepo.EnqueueParams{
		FeedID:      "feed-1",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
		Target:      "gyoka",
		Operation:   "add",
		MutationID:  "m-1",
		SubjectKey:  "feed-1:at://did:plc:user1/app.bsky.feed.post/post1",
		OpKey:       "m-1:add:feed-1:at://did:plc:user1/app.bsky.feed.post/post1",
		PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
		Status:      "pending",
	}

	if err := repo.Enqueue(ctx, params); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := repo.Enqueue(ctx, params); err != nil {
		t.Fatalf("Enqueue() duplicate error = %v", err)
	}

	entries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{
		Target: "gyoka",
		Status: "pending",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListByStatus() len = %d, want 1", len(entries))
	}
	if entries[0].OpKey != params.OpKey {
		t.Fatalf("ListByStatus()[0].OpKey = %s, want %s", entries[0].OpKey, params.OpKey)
	}
}
