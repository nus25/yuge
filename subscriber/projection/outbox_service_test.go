package projection

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
)

type spyEntryProjector struct {
	err          error
	projected    []projectionrepo.Entry
	projectCalls int
}

func (p *spyEntryProjector) Project(ctx context.Context, entry projectionrepo.Entry) error {
	p.projectCalls++
	p.projected = append(p.projected, entry)
	return p.err
}

func TestOutboxService_ProcessNextPending_CompletesClaimedEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "outbox-service.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-1",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
		Target:      "gyoka",
		Operation:   "add",
		MutationID:  "m-1",
		SubjectKey:  "feed-1:at://did:plc:user1/app.bsky.feed.post/post1",
		OpKey:       "m-1:add:feed-1:at://did:plc:user1/app.bsky.feed.post/post1",
		PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	projector := &spyEntryProjector{}
	service := NewOutboxService("gyoka", repo, projector)

	processed, err := service.ProcessNextPending(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPending() error = %v", err)
	}
	if !processed {
		t.Fatal("ProcessNextPending() processed = false, want true")
	}
	if projector.projectCalls != 1 {
		t.Fatalf("projector calls = %d, want 1", projector.projectCalls)
	}

	completedEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() completed error = %v", err)
	}
	if len(completedEntries) != 1 {
		t.Fatalf("completed entries len = %d, want 1", len(completedEntries))
	}
}

func TestOutboxService_ProcessNextPending_RetryableErrorRequeuesEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "outbox-service-error.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-1",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
		Target:      "gyoka",
		Operation:   "add",
		MutationID:  "m-1",
		SubjectKey:  "feed-1:at://did:plc:user1/app.bsky.feed.post/post1",
		OpKey:       "m-1:add:feed-1:at://did:plc:user1/app.bsky.feed.post/post1",
		PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	service := NewOutboxService("gyoka", repo, &spyEntryProjector{err: errors.New("boom")})

	processed, err := service.ProcessNextPending(ctx)
	if err == nil {
		t.Fatal("ProcessNextPending() error = nil, want error")
	}
	if !processed {
		t.Fatal("ProcessNextPending() processed = false, want true")
	}

	processingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "processing", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() processing error = %v", err)
	}
	if len(processingEntries) != 0 {
		t.Fatalf("processing entries len = %d, want 0", len(processingEntries))
	}

	pendingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() pending error = %v", err)
	}
	if len(pendingEntries) != 1 {
		t.Fatalf("pending entries len = %d, want 1", len(pendingEntries))
	}
	if pendingEntries[0].RetryCount != 1 {
		t.Fatalf("pending RetryCount = %d, want 1", pendingEntries[0].RetryCount)
	}
	if pendingEntries[0].NextRetryAt == "" {
		t.Fatal("pending NextRetryAt is empty")
	}
	if pendingEntries[0].LastError == "" {
		t.Fatal("pending LastError is empty")
	}
}

func TestOutboxService_ProcessNextPending_NonRetryableErrorMovesEntryToDead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "outbox-service-dead.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-1",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
		Target:      "gyoka",
		Operation:   "trim",
		MutationID:  "m-1",
		SubjectKey:  "feed-1:trim",
		OpKey:       "m-1:trim:feed-1",
		PayloadJSON: `{}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	service := NewOutboxService("gyoka", repo, NewGyokaProjector(&spyGyokaMutator{}))

	processed, err := service.ProcessNextPending(ctx)
	if err == nil {
		t.Fatal("ProcessNextPending() error = nil, want error")
	}
	if !processed {
		t.Fatal("ProcessNextPending() processed = false, want true")
	}

	processingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "processing", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() processing error = %v", err)
	}
	if len(processingEntries) != 0 {
		t.Fatalf("processing entries len = %d, want 0", len(processingEntries))
	}

	deadEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "dead", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() dead error = %v", err)
	}
	if len(deadEntries) != 1 {
		t.Fatalf("dead entries len = %d, want 1", len(deadEntries))
	}
	if deadEntries[0].LastError == "" {
		t.Fatal("dead LastError is empty")
	}
}

func TestOutboxService_ProcessNextPending_ReturnsFalseWhenQueueEmpty(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "outbox-service-empty.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	service := NewOutboxService("gyoka", projectionsqlite.NewOutboxRepository(db), &spyEntryProjector{})

	processed, err := service.ProcessNextPending(ctx)
	if err != nil {
		t.Fatalf("ProcessNextPending() error = %v", err)
	}
	if processed {
		t.Fatal("ProcessNextPending() processed = true, want false")
	}
}
