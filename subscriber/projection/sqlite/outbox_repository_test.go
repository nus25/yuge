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

func TestOutboxRepositoryClaimNextPending_ReclaimsOnlyStaleProcessingRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-reclaim.db"),
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
	fixtures := []projectionrepo.EnqueueParams{
		{
			FeedID:      "feed-stale",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/stale",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-stale",
			SubjectKey:  "feed-stale:post-1",
			OpKey:       "m-stale:add:post-1",
			PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-fresh",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/fresh",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-fresh",
			SubjectKey:  "feed-fresh:post-2",
			OpKey:       "m-fresh:add:post-2",
			PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
			Status:      "pending",
		},
	}
	for _, fixture := range fixtures {
		if err := repo.Enqueue(ctx, fixture); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	staleClaimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() stale error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() stale ok = false, want true")
	}

	freshClaimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() fresh error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() fresh ok = false, want true")
	}

	staleUpdatedAt := time.Now().UTC().Add(-defaultProcessingReclaimTimeout - time.Minute).Format(time.RFC3339Nano)
	freshUpdatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `UPDATE projection_outbox SET updated_at = ? WHERE id = ?;`, staleUpdatedAt, staleClaimed.ID); err != nil {
		t.Fatalf("ExecContext() stale updated_at error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE projection_outbox SET updated_at = ? WHERE id = ?;`, freshUpdatedAt, freshClaimed.ID); err != nil {
		t.Fatalf("ExecContext() fresh updated_at error = %v", err)
	}

	reclaimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() reclaim error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() reclaim ok = false, want true")
	}
	if reclaimed.ID != staleClaimed.ID {
		t.Fatalf("reclaimed id = %d, want %d", reclaimed.ID, staleClaimed.ID)
	}

	processingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "processing", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() processing error = %v", err)
	}
	if len(processingEntries) != 2 {
		t.Fatalf("processing entries len = %d, want 2", len(processingEntries))
	}
	for _, entry := range processingEntries {
		if entry.ID == freshClaimed.ID && entry.UpdatedAt != freshUpdatedAt {
			t.Fatalf("fresh processing row was unexpectedly reclaimed: updated_at = %s, want %s", entry.UpdatedAt, freshUpdatedAt)
		}
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

func TestOutboxRepositoryClearFeed_ReplacesPendingEntriesWithSingleTrim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-clear.db"),
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
	fixtures := []projectionrepo.EnqueueParams{
		{
			FeedID:      "feed-1",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-add",
			SubjectKey:  "feed-1:subject-add",
			OpKey:       "m-add:add:feed-1",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-1",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
			Target:      "gyoka",
			Operation:   "delete",
			MutationID:  "m-delete",
			SubjectKey:  "feed-1:subject-delete",
			OpKey:       "m-delete:delete:feed-1",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-2",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/other",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-other",
			SubjectKey:  "feed-2:subject-add",
			OpKey:       "m-other:add:feed-2",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/other"}`,
			Status:      "pending",
		},
	}
	for _, fixture := range fixtures {
		if err := repo.Enqueue(ctx, fixture); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	if _, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"}); err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	} else if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}

	if err := repo.ClearFeed(ctx, projectionrepo.ClearFeedParams{
		FeedID:     "feed-1",
		FeedURI:    "at://did:plc:test/app.bsky.feed.generator/sample",
		Target:     "gyoka",
		MutationID: "mutation-clear-1",
		Count:      0,
	}); err != nil {
		t.Fatalf("ClearFeed() error = %v", err)
	}

	pendingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(pendingEntries) != 2 {
		t.Fatalf("pending entries len = %d, want 2", len(pendingEntries))
	}
	if pendingEntries[0].FeedID != "feed-2" || pendingEntries[0].Operation != "add" {
		t.Fatalf("pendingEntries[0] = %+v, want other feed add", pendingEntries[0])
	}
	if pendingEntries[1].FeedID != "feed-1" || pendingEntries[1].Operation != "trim" {
		t.Fatalf("pendingEntries[1] = %+v, want feed-1 trim", pendingEntries[1])
	}
	if pendingEntries[1].SubjectKey != "feed-1:trim" {
		t.Fatalf("trim SubjectKey = %s, want feed-1:trim", pendingEntries[1].SubjectKey)
	}

	processingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "processing", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() processing error = %v", err)
	}
	if len(processingEntries) != 1 {
		t.Fatalf("processing entries len = %d, want 1", len(processingEntries))
	}
	if processingEntries[0].FeedID != "feed-1" {
		t.Fatalf("processing FeedID = %s, want feed-1", processingEntries[0].FeedID)
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

func TestOutboxRepositoryClaimNextPendingBatchClaimsContiguousAddsOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-claim-batch.db"),
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
	fixtures := []projectionrepo.EnqueueParams{
		{
			FeedID:      "feed-1",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-1",
			SubjectKey:  "feed-1:post-1",
			OpKey:       "m-1:add:post-1",
			PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-1",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-2",
			SubjectKey:  "feed-1:post-2",
			OpKey:       "m-2:add:post-2",
			PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post2"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-1",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
			Target:      "gyoka",
			Operation:   "delete",
			MutationID:  "m-3",
			SubjectKey:  "feed-1:post-3",
			OpKey:       "m-3:delete:post-3",
			PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post3"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-1",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/sample",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-4",
			SubjectKey:  "feed-1:post-4",
			OpKey:       "m-4:add:post-4",
			PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post4"}`,
			Status:      "pending",
		},
	}
	for _, fixture := range fixtures {
		if err := repo.Enqueue(ctx, fixture); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	entries, ok, err := repo.ClaimNextPendingBatch(ctx, projectionrepo.ClaimNextPendingBatchParams{Target: "gyoka", Limit: 10})
	if err != nil {
		t.Fatalf("ClaimNextPendingBatch() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPendingBatch() ok = false, want true")
	}
	if len(entries) != 2 {
		t.Fatalf("ClaimNextPendingBatch() len = %d, want 2", len(entries))
	}
	for _, entry := range entries {
		if entry.Operation != "add" {
			t.Fatalf("claimed operation = %s, want add", entry.Operation)
		}
		if entry.Status != "processing" {
			t.Fatalf("claimed status = %s, want processing", entry.Status)
		}
	}

	pendingEntries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() pending error = %v", err)
	}
	if len(pendingEntries) != 2 {
		t.Fatalf("pending entries len = %d, want 2", len(pendingEntries))
	}
	if pendingEntries[0].Operation != "delete" {
		t.Fatalf("first pending operation = %s, want delete", pendingEntries[0].Operation)
	}
}

func TestOutboxRepositoryClaimNextPendingSkipsManualFailedRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-skip-failed.db"),
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
	if err := repo.MarkFailed(ctx, projectionrepo.MarkFailedParams{ID: claimedFailed.ID, LastError: "batch add failed"}); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}

	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-ready",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/ready",
		Target:      "gyoka",
		Operation:   "delete",
		MutationID:  "m-ready",
		SubjectKey:  "feed-ready:post-2",
		OpKey:       "m-ready:delete:post-2",
		PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() ready entry error = %v", err)
	}

	claimedReady, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() ready entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ready entry ok = false, want true")
	}
	if claimedReady.ID == claimedFailed.ID {
		t.Fatal("ClaimNextPending() claimed manual failed row, want ready row")
	}
	if claimedReady.Operation != "delete" {
		t.Fatalf("claimed ready operation = %s, want delete", claimedReady.Operation)
	}

	if err := repo.Requeue(ctx, projectionrepo.RequeueParams{ID: claimedFailed.ID}); err != nil {
		t.Fatalf("Requeue() error = %v", err)
	}

	if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: claimedReady.ID}); err != nil {
		t.Fatalf("MarkCompleted() ready entry error = %v", err)
	}

	requeued, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() requeued error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() requeued ok = false, want true")
	}
	if requeued.ID != claimedFailed.ID {
		t.Fatalf("requeued id = %d, want %d", requeued.ID, claimedFailed.ID)
	}
}

func TestOutboxRepositoryCountByStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-counts.db"),
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
	target := "gyoka-counts"
	entries := []projectionrepo.EnqueueParams{
		{
			FeedID:      "feed-pending",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/pending",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-pending",
			SubjectKey:  "subject-pending",
			OpKey:       "op-pending",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-ready",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/ready",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-ready",
			SubjectKey:  "subject-ready",
			OpKey:       "op-ready",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-completed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-completed",
			SubjectKey:  "subject-completed",
			OpKey:       "op-completed",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-dead",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
			Target:      target,
			Operation:   "delete",
			MutationID:  "m-dead",
			SubjectKey:  "subject-dead",
			OpKey:       "op-dead",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
	}
	for _, entry := range entries {
		if err := repo.Enqueue(ctx, entry); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", entry.OpKey, err)
		}
	}

	failedEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
	if err != nil {
		t.Fatalf("ClaimNextPending() first error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() first ok = false, want true")
	}
	if err := repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
		ID:          failedEntry.ID,
		LastError:   "temporary failure",
		NextRetryAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkRetryableFailure() error = %v", err)
	}

	completedEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
	if err != nil {
		t.Fatalf("ClaimNextPending() second error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() second ok = false, want true")
	}
	if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: completedEntry.ID}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	deadEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
	if err != nil {
		t.Fatalf("ClaimNextPending() third error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() third ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: deadEntry.ID, LastError: "boom"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	counts, err := repo.CountByStatus(ctx, projectionrepo.CountByStatusParams{Target: target})
	if err != nil {
		t.Fatalf("CountByStatus() error = %v", err)
	}

	countsByStatus := make(map[string]int64, len(counts))
	for _, count := range counts {
		countsByStatus[count.Status] = count.Count
	}

	if got := countsByStatus["pending"]; got != 2 {
		t.Fatalf("pending count = %d, want 2", got)
	}
	if got := countsByStatus["completed"]; got != 1 {
		t.Fatalf("completed count = %d, want 1", got)
	}
	if got := countsByStatus["dead"]; got != 1 {
		t.Fatalf("dead count = %d, want 1", got)
	}
	if got := countsByStatus["failed"]; got != 1 {
		t.Fatalf("failed count = %d, want 1", got)
	}
	if got := countsByStatus["processing"]; got != 0 {
		t.Fatalf("processing count = %d, want 0", got)
	}
}
