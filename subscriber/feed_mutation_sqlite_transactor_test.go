package subscriber

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	"github.com/nus25/yuge/types"
)

type failingOutboxRepository struct {
	enqueueErr error
}

func (r *failingOutboxRepository) Enqueue(ctx context.Context, params projectionrepo.EnqueueParams) error {
	return r.enqueueErr
}

func (r *failingOutboxRepository) ListByStatus(ctx context.Context, params projectionrepo.ListByStatusParams) ([]projectionrepo.Entry, error) {
	return nil, nil
}

func (r *failingOutboxRepository) CountByStatus(ctx context.Context, params projectionrepo.CountByStatusParams) ([]projectionrepo.StatusCount, error) {
	return nil, nil
}

func (r *failingOutboxRepository) ClaimNextPending(ctx context.Context, params projectionrepo.ClaimNextPendingParams) (projectionrepo.Entry, bool, error) {
	return projectionrepo.Entry{}, false, nil
}

func (r *failingOutboxRepository) ClaimNextPendingBatch(ctx context.Context, params projectionrepo.ClaimNextPendingBatchParams) ([]projectionrepo.Entry, bool, error) {
	return nil, false, nil
}

func (r *failingOutboxRepository) MarkCompleted(ctx context.Context, params projectionrepo.MarkCompletedParams) error {
	return nil
}

func (r *failingOutboxRepository) MarkRetryableFailure(ctx context.Context, params projectionrepo.MarkRetryableFailureParams) error {
	return nil
}

func (r *failingOutboxRepository) MarkFailed(ctx context.Context, params projectionrepo.MarkFailedParams) error {
	return nil
}

func (r *failingOutboxRepository) MarkDead(ctx context.Context, params projectionrepo.MarkDeadParams) error {
	return nil
}

func (r *failingOutboxRepository) Requeue(ctx context.Context, params projectionrepo.RequeueParams) error {
	return nil
}

func (r *failingOutboxRepository) Delete(ctx context.Context, params projectionrepo.DeleteParams) error {
	return nil
}

func (r *failingOutboxRepository) PurgeCompleted(ctx context.Context, params projectionrepo.PurgeCompletedParams) (int64, error) {
	return 0, nil
}

func openMutationTestDB(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}

func TestSQLiteFeedMutationTransactor_RollsBackWhenOutboxEnqueueFails(t *testing.T) {
	t.Parallel()

	ctx := openMutationTestDB(t)
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "mutation.db"),
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

	transactor := NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{
		OutboxRepositoryFactory: func(tx *sql.Tx) projectionrepo.OutboxRepository {
			return &failingOutboxRepository{enqueueErr: errors.New("boom")}
		},
	})
	coordinator := NewFeedMutationCoordinator(transactor)

	err = coordinator.AddPost(ctx, AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Did:        "did:plc:user1",
		Rkey:       "post1",
		Cid:        "cid-1",
		IndexedAt:  time.Date(2026, 5, 11, 5, 0, 0, 0, time.UTC),
		Langs:      []string{"ja"},
		MutationID: "mutation-1",
	})
	if err == nil {
		t.Fatal("expected AddPost to fail")
	}

	feedRepo := storesqlite.NewFeedRepository(db)
	posts, err := feedRepo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("ListPosts() len = %d, want 0 after rollback", len(posts))
	}
}

func TestSQLiteFeedMutationTransactor_CommitsPostAndOutbox(t *testing.T) {
	t.Parallel()

	ctx := openMutationTestDB(t)
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "mutation-success.db"),
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

	transactor := NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{})
	coordinator := NewFeedMutationCoordinator(transactor)

	err = coordinator.AddPost(ctx, AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Did:        "did:plc:user1",
		Rkey:       "post1",
		Cid:        "cid-1",
		IndexedAt:  time.Date(2026, 5, 11, 5, 0, 0, 0, time.UTC),
		Langs:      []string{"ja"},
		MutationID: "mutation-1",
	})
	if err != nil {
		t.Fatalf("AddPost() error = %v", err)
	}

	feedRepo := storesqlite.NewFeedRepository(db)
	posts, err := feedRepo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("ListPosts() len = %d, want 1", len(posts))
	}

	outboxRepo := projectionsqlite.NewOutboxRepository(db)
	entries, err := outboxRepo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListByStatus() len = %d, want 1", len(entries))
	}
}

func TestSQLiteFeedMutationTransactor_AddPost_TrimsOverflowAndEnqueuesDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "mutation-trim.db"),
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

	feedRepo := storesqlite.NewFeedRepository(db)
	seedPosts := []types.Post{
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
			Cid:       "cid-1",
			IndexedAt: time.Date(2026, 5, 11, 5, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Langs:     []string{"ja"},
		},
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user2/app.bsky.feed.post/post2"),
			Cid:       "cid-2",
			IndexedAt: time.Date(2026, 5, 11, 5, 1, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Langs:     []string{"ja"},
		},
	}
	for _, post := range seedPosts {
		if err := feedRepo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: post}); err != nil {
			t.Fatalf("PutPost(%s) error = %v", post.Uri, err)
		}
	}

	coordinator := NewFeedMutationCoordinator(NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{}))
	err = coordinator.AddPost(ctx, AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Did:        "did:plc:user3",
		Rkey:       "post3",
		Cid:        "cid-3",
		IndexedAt:  time.Date(2026, 5, 11, 5, 2, 0, 0, time.UTC),
		Langs:      []string{"ja"},
		MutationID: "mutation-3",
		TrimAt:     2,
		TrimRemain: 2,
	})
	if err != nil {
		t.Fatalf("AddPost() error = %v", err)
	}

	posts, err := feedRepo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("ListPosts() len = %d, want 2", len(posts))
	}
	if posts[0].Uri != types.PostUri("at://did:plc:user3/app.bsky.feed.post/post3") {
		t.Fatalf("ListPosts()[0].Uri = %s, want newest post", posts[0].Uri)
	}
	if posts[1].Uri != seedPosts[1].Uri {
		t.Fatalf("ListPosts()[1].Uri = %s, want %s", posts[1].Uri, seedPosts[1].Uri)
	}

	entries, err := projectionsqlite.NewOutboxRepository(db).ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ListByStatus() len = %d, want 2", len(entries))
	}
	if entries[0].Operation != "add" {
		t.Fatalf("entries[0].Operation = %s, want add", entries[0].Operation)
	}
	if entries[1].Operation != "delete" {
		t.Fatalf("entries[1].Operation = %s, want delete", entries[1].Operation)
	}
	if entries[1].SubjectKey != fmt.Sprintf("feed-1:%s", seedPosts[0].Uri) {
		t.Fatalf("entries[1].SubjectKey = %s, want %s", entries[1].SubjectKey, fmt.Sprintf("feed-1:%s", seedPosts[0].Uri))
	}
}

func TestSQLiteFeedMutationTransactor_ConcurrentAddPost(t *testing.T) {
	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "mutation-concurrent.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  250 * time.Millisecond,
		MaxOpenConns: 4,
		MaxIdleConns: 4,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	coordinator := NewFeedMutationCoordinator(NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{}))

	const writes = 12
	errCh := make(chan error, writes)
	var wg sync.WaitGroup
	for i := 0; i < writes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errCh <- coordinator.AddPost(ctx, AddPostParams{
				FeedID:     "feed-1",
				FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
				Did:        fmt.Sprintf("did:plc:user%d", i),
				Rkey:       fmt.Sprintf("post%d", i),
				Cid:        fmt.Sprintf("cid-%d", i),
				IndexedAt:  time.Date(2026, 5, 11, 6, 30, i, 0, time.UTC),
				Langs:      []string{"ja"},
				MutationID: fmt.Sprintf("mutation-%d", i),
			})
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("AddPost() error = %v", err)
		}
	}

	feedRepo := storesqlite.NewFeedRepository(db)
	posts, err := feedRepo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != writes {
		t.Fatalf("ListPosts() len = %d, want %d", len(posts), writes)
	}

	outboxRepo := projectionsqlite.NewOutboxRepository(db)
	entries, err := outboxRepo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: writes + 1})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != writes {
		t.Fatalf("ListByStatus() len = %d, want %d", len(entries), writes)
	}
}

func TestSQLiteFeedMutationTransactor_DeletePost_RollsBackWhenOutboxEnqueueFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "mutation-delete-rollback.db"),
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

	feedRepo := storesqlite.NewFeedRepository(db)
	post := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 7, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}
	if err := feedRepo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: post}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}

	transactor := NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{
		OutboxRepositoryFactory: func(tx *sql.Tx) projectionrepo.OutboxRepository {
			return &failingOutboxRepository{enqueueErr: errors.New("boom")}
		},
	})
	coordinator := NewFeedMutationCoordinator(transactor)

	err = coordinator.DeletePost(ctx, DeletePostParams{
		FeedID:     "feed-1",
		FeedURI:    post.Feed,
		Post:       post,
		MutationID: "mutation-delete-1",
	})
	if err == nil {
		t.Fatal("expected DeletePost to fail")
	}

	posts, err := feedRepo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("ListPosts() len = %d, want 1 after rollback", len(posts))
	}
}

func TestSQLiteFeedMutationTransactor_DeletePost_CommitsDeleteAndOutbox(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "mutation-delete-success.db"),
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

	feedRepo := storesqlite.NewFeedRepository(db)
	post := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 7, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}
	if err := feedRepo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: post}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}

	coordinator := NewFeedMutationCoordinator(NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{}))
	err = coordinator.DeletePost(ctx, DeletePostParams{
		FeedID:     "feed-1",
		FeedURI:    post.Feed,
		Post:       post,
		MutationID: "mutation-delete-1",
	})
	if err != nil {
		t.Fatalf("DeletePost() error = %v", err)
	}

	posts, err := feedRepo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("ListPosts() len = %d, want 0", len(posts))
	}

	outboxRepo := projectionsqlite.NewOutboxRepository(db)
	entries, err := outboxRepo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListByStatus() len = %d, want 1", len(entries))
	}
	if entries[0].Operation != "delete" {
		t.Fatalf("Operation = %s, want delete", entries[0].Operation)
	}
}
