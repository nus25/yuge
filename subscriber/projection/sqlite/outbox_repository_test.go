package sqlite

import (
	"context"
	"path/filepath"
	"strconv"
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

func TestOutboxRepositoryClaimAndCompleteSinglePendingRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-claim.db"),
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

	claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}
	if claimed.Status != "processing" {
		t.Fatalf("claimed.Status = %s, want processing", claimed.Status)
	}

	pendingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() pending error = %v", err)
	}
	if len(pendingEntries) != 0 {
		t.Fatalf("pending entries len = %d, want 0", len(pendingEntries))
	}

	if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: claimed.ID}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	completedEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() completed error = %v", err)
	}
	if len(completedEntries) != 1 {
		t.Fatalf("completed entries len = %d, want 1", len(completedEntries))
	}
	if completedEntries[0].CompletedAt == "" {
		t.Fatal("completed entry CompletedAt is empty")
	}

	_, ok, err = repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() second error = %v", err)
	}
	if ok {
		t.Fatal("ClaimNextPending() second ok = true, want false")
	}
}

func TestOutboxRepositoryMarkRetryableFailureRequeuesClaimedRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-retryable.db"),
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

	claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}

	nextRetryAt := time.Now().UTC().Add(3 * time.Second)
	if err := repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
		ID:          claimed.ID,
		LastError:   "temporary failure",
		NextRetryAt: nextRetryAt,
	}); err != nil {
		t.Fatalf("MarkRetryableFailure() error = %v", err)
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
	if pendingEntries[0].LastError != "temporary failure" {
		t.Fatalf("pending LastError = %q, want temporary failure", pendingEntries[0].LastError)
	}
	if pendingEntries[0].NextRetryAt == "" {
		t.Fatal("pending NextRetryAt is empty")
	}
}

func TestOutboxRepositoryMarkDeadMovesClaimedRowToDead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-dead.db"),
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

	claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}

	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: claimed.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	deadEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "dead", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() dead error = %v", err)
	}
	if len(deadEntries) != 1 {
		t.Fatalf("dead entries len = %d, want 1", len(deadEntries))
	}
	if deadEntries[0].RetryCount != 1 {
		t.Fatalf("dead RetryCount = %d, want 1", deadEntries[0].RetryCount)
	}
	if deadEntries[0].LastError != "unsupported operation" {
		t.Fatalf("dead LastError = %q, want unsupported operation", deadEntries[0].LastError)
	}
	if deadEntries[0].CompletedAt != "" {
		t.Fatalf("dead CompletedAt = %q, want empty", deadEntries[0].CompletedAt)
	}
}

func TestOutboxRepositoryRequeueReadiesFailedAndDeadRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-requeue.db"),
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
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-failed",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/failed",
		Target:      "gyoka",
		Operation:   "add",
		MutationID:  "m-failed",
		SubjectKey:  "feed-failed:post-1",
		OpKey:       "m-failed:add:post-1",
		PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() failed entry error = %v", err)
	}
	claimedFailed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() failed entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() failed entry ok = false, want true")
	}
	if err := repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
		ID:          claimedFailed.ID,
		LastError:   "temporary failure",
		NextRetryAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkRetryableFailure() error = %v", err)
	}

	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-dead",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
		Target:      "gyoka",
		Operation:   "delete",
		MutationID:  "m-dead",
		SubjectKey:  "feed-dead:post-2",
		OpKey:       "m-dead:delete:post-2",
		PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() dead entry error = %v", err)
	}
	claimedDead, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() dead entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() dead entry ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: claimedDead.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	if err := repo.Requeue(ctx, projectionrepo.RequeueParams{ID: claimedFailed.ID}); err != nil {
		t.Fatalf("Requeue() failed entry error = %v", err)
	}
	if err := repo.Requeue(ctx, projectionrepo.RequeueParams{ID: claimedDead.ID}); err != nil {
		t.Fatalf("Requeue() dead entry error = %v", err)
	}

	pendingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() pending error = %v", err)
	}
	if len(pendingEntries) != 2 {
		t.Fatalf("pending entries len = %d, want 2", len(pendingEntries))
	}
	for _, entry := range pendingEntries {
		if entry.LastError != "" {
			t.Fatalf("pending LastError = %q, want empty", entry.LastError)
		}
		if entry.NextRetryAt != "" {
			t.Fatalf("pending NextRetryAt = %q, want empty", entry.NextRetryAt)
		}
	}
}

func TestOutboxRepositoryDeleteRemovesNonProcessingRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-delete.db"),
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
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-dead",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
		Target:      "gyoka",
		Operation:   "delete",
		MutationID:  "m-dead",
		SubjectKey:  "feed-dead:post-2",
		OpKey:       "m-dead:delete:post-2",
		PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: claimed.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	if err := repo.Delete(ctx, projectionrepo.DeleteParams{ID: claimed.ID}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	deadEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "dead", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(deadEntries) != 0 {
		t.Fatalf("dead entries len = %d, want 0", len(deadEntries))
	}
}

func TestOutboxRepositoryPurgeCompletedDeletesLimitedRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-purge.db"),
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
	for index := 0; index < 3; index++ {
		if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
			FeedID:      "feed-completed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-completed-" + strconv.Itoa(index),
			SubjectKey:  "feed-completed:post-" + strconv.Itoa(index),
			OpKey:       "m-completed:add:post-" + strconv.Itoa(index),
			PayloadJSON: `{"uri":"at://did:plc:user3/app.bsky.feed.post/post3"}`,
			Status:      "pending",
		}); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
		if err != nil {
			t.Fatalf("ClaimNextPending() error = %v", err)
		}
		if !ok {
			t.Fatal("ClaimNextPending() ok = false, want true")
		}
		if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: claimed.ID}); err != nil {
			t.Fatalf("MarkCompleted() error = %v", err)
		}
	}

	deletedCount, err := repo.PurgeCompleted(ctx, projectionrepo.PurgeCompletedParams{Target: "gyoka", Limit: 2})
	if err != nil {
		t.Fatalf("PurgeCompleted() error = %v", err)
	}
	if deletedCount != 2 {
		t.Fatalf("deletedCount = %d, want 2", deletedCount)
	}

	completedEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(completedEntries) != 1 {
		t.Fatalf("completed entries len = %d, want 1", len(completedEntries))
	}
}
