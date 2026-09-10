package subscriber

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	storepkg "github.com/nus25/yuge/feed/store"
	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	"github.com/nus25/yuge/types"
)

func TestOpenSQLiteMutationCoordinator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, coordinator, err := openSQLiteMutationCoordinator(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("openSQLiteMutationCoordinator() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if coordinator == nil {
		t.Fatal("expected coordinator to be created")
	}

	err = coordinator.AddPost(ctx, AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Did:        "did:plc:user1",
		Rkey:       "post1",
		Cid:        "cid-1",
		IndexedAt:  time.Date(2026, 5, 11, 6, 0, 0, 0, time.UTC),
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

func TestOpenSQLiteRuntimePersistence_LoaderReadsWhileWriterTransactionOpen(t *testing.T) {
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
	if persistence.mutationCoordinator == nil {
		t.Fatal("expected mutation coordinator to be created")
	}
	if persistence.postLoader == nil {
		t.Fatal("expected post loader to be created")
	}

	committedPost := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/feed-1"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}
	feedRepo := storesqlite.NewFeedRepository(persistence.mutationDB)
	if err := feedRepo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: committedPost}); err != nil {
		t.Fatalf("PutPost() committed error = %v", err)
	}

	tx, err := persistence.mutationDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	pendingPost := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/feed-2"),
		Uri:       types.PostUri("at://did:plc:user2/app.bsky.feed.post/post2"),
		Cid:       "cid-2",
		IndexedAt: time.Date(2026, 5, 11, 8, 1, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"en"},
	}
	if err := storesqlite.NewFeedRepository(tx).PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-2", Post: pendingPost}); err != nil {
		t.Fatalf("PutPost() pending error = %v", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	posts, err := persistence.postLoader.LoadPosts(readCtx, storepkg.LoadPostsParams{
		FeedID:  "feed-1",
		FeedURI: committedPost.Feed,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("LoadPosts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("LoadPosts() len = %d, want 1", len(posts))
	}
	if posts[0].Uri != committedPost.Uri {
		t.Fatalf("LoadPosts()[0].Uri = %s, want %s", posts[0].Uri, committedPost.Uri)
	}
}

func TestOpenSQLiteRuntimePersistence_MutationCoordinatorWaitsForWriterLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	persistence, err := openSQLiteRuntimePersistence(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	t.Cleanup(func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	blockingDB, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(dataDir, defaultSQLiteFilename),
		SyncMode:     "NORMAL",
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() blocking db error = %v", err)
	}
	t.Cleanup(func() {
		if err := blockingDB.Close(); err != nil {
			t.Fatalf("blockingDB.Close() error = %v", err)
		}
	})

	blockingTx, err := blockingDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() blocking error = %v", err)
	}
	blockingReleased := false
	defer func() {
		if !blockingReleased {
			_ = blockingTx.Rollback()
		}
	}()

	blockingPost := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/blocker"),
		Uri:       types.PostUri("at://did:plc:blocker/app.bsky.feed.post/blocker"),
		Cid:       "cid-blocker",
		IndexedAt: time.Date(2026, 5, 11, 8, 2, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}
	if err := storesqlite.NewFeedRepository(blockingTx).PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-blocker", Post: blockingPost}); err != nil {
		t.Fatalf("PutPost() blocking error = %v", err)
	}

	addCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addErrCh := make(chan error, 1)
	go func() {
		addErrCh <- persistence.mutationCoordinator.AddPost(addCtx, AddPostParams{
			FeedID:     "feed-1",
			FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/feed-1"),
			Did:        "did:plc:user1",
			Rkey:       "post1",
			Cid:        "cid-1",
			IndexedAt:  time.Date(2026, 5, 11, 8, 3, 0, 0, time.UTC),
			Langs:      []string{"en"},
			MutationID: "mutation-1",
		})
	}()

	select {
	case err := <-addErrCh:
		t.Fatalf("AddPost() completed before writer lock released: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	postsBeforeRelease, err := storesqlite.NewFeedRepository(persistence.loaderDB).ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListPosts() before release error = %v", err)
	}
	if len(postsBeforeRelease) != 0 {
		t.Fatalf("ListPosts() before release len = %d, want 0", len(postsBeforeRelease))
	}

	if err := blockingTx.Rollback(); err != nil {
		t.Fatalf("Rollback() blocking tx error = %v", err)
	}
	blockingReleased = true

	if err := <-addErrCh; err != nil {
		t.Fatalf("AddPost() error = %v", err)
	}

	posts, err := storesqlite.NewFeedRepository(persistence.loaderDB).ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("ListPosts() len = %d, want 1", len(posts))
	}
	if posts[0].Uri != types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1") {
		t.Fatalf("ListPosts()[0].Uri = %s, want %s", posts[0].Uri, types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"))
	}

	entries, err := projectionsqlite.NewOutboxRepository(persistence.loaderDB).ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListByStatus() len = %d, want 1", len(entries))
	}
	if entries[0].FeedID != "feed-1" {
		t.Fatalf("ListByStatus()[0].FeedID = %s, want feed-1", entries[0].FeedID)
	}
}
