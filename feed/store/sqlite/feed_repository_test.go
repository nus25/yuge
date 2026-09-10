package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	storerepo "github.com/nus25/yuge/feed/store/repository"
	"github.com/nus25/yuge/types"
)

func TestFeedRepositoryCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, Options{
		Path:         filepath.Join(t.TempDir(), "feed.db"),
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
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewFeedRepository(db)

	newer := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post2"),
		Cid:       "cid-2",
		IndexedAt: "2026-05-11T03:00:00Z",
		Langs:     []string{"ja"},
	}
	older := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: "2026-05-11T02:00:00Z",
		Langs:     []string{"en"},
	}

	for _, post := range []types.Post{older, newer} {
		if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: post}); err != nil {
			t.Fatalf("PutPost(%s) error = %v", post.Uri, err)
		}
	}

	posts, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1", Limit: 1})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("ListPosts() len = %d, want 1", len(posts))
	}
	if posts[0].Uri != newer.Uri {
		t.Fatalf("ListPosts()[0].Uri = %s, want %s", posts[0].Uri, newer.Uri)
	}

	state := storerepo.FeedState{
		FeedID:         "feed-1",
		FeedURI:        newer.Feed,
		Status:         "active",
		ConfigRevision: "rev-1",
		UpdatedAt:      "2026-05-11T03:05:00Z",
	}
	if err := repo.PutFeedState(ctx, storerepo.PutFeedStateParams{State: state}); err != nil {
		t.Fatalf("PutFeedState() error = %v", err)
	}

	gotState, ok, err := repo.GetFeedState(ctx, "feed-1")
	if err != nil {
		t.Fatalf("GetFeedState() error = %v", err)
	}
	if !ok {
		t.Fatal("GetFeedState() ok = false, want true")
	}
	if gotState.ConfigRevision != state.ConfigRevision {
		t.Fatalf("GetFeedState().ConfigRevision = %s, want %s", gotState.ConfigRevision, state.ConfigRevision)
	}

	if err := repo.DeletePost(ctx, storerepo.DeletePostParams{FeedID: "feed-1", PostURI: newer.Uri}); err != nil {
		t.Fatalf("DeletePost() error = %v", err)
	}

	posts, err = repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() after delete error = %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("ListPosts() after delete len = %d, want 1", len(posts))
	}
	if posts[0].Uri != older.Uri {
		t.Fatalf("remaining post Uri = %s, want %s", posts[0].Uri, older.Uri)
	}
}

func TestFeedRepositoryTrimOverflowKeepsNewestPosts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, Options{
		Path:         filepath.Join(t.TempDir(), "feed-trim.db"),
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
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewFeedRepository(db)
	posts := []types.Post{
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
			Cid:       "cid-1",
			IndexedAt: "2026-05-11T01:00:00Z",
			Langs:     []string{"ja"},
		},
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user2/app.bsky.feed.post/post2"),
			Cid:       "cid-2",
			IndexedAt: "2026-05-11T02:00:00Z",
			Langs:     []string{"ja"},
		},
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user3/app.bsky.feed.post/post3"),
			Cid:       "cid-3",
			IndexedAt: "2026-05-11T03:00:00Z",
			Langs:     []string{"ja"},
		},
	}
	for _, post := range posts {
		if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: post}); err != nil {
			t.Fatalf("PutPost(%s) error = %v", post.Uri, err)
		}
	}

	trimmedPosts, err := repo.TrimOverflow(ctx, storerepo.TrimOverflowParams{FeedID: "feed-1", TrimAt: 2, Remain: 2})
	if err != nil {
		t.Fatalf("TrimOverflow() error = %v", err)
	}
	if len(trimmedPosts) != 1 {
		t.Fatalf("TrimOverflow() len = %d, want 1", len(trimmedPosts))
	}
	if trimmedPosts[0].Uri != posts[0].Uri {
		t.Fatalf("TrimOverflow()[0].Uri = %s, want %s", trimmedPosts[0].Uri, posts[0].Uri)
	}

	remainingPosts, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(remainingPosts) != 2 {
		t.Fatalf("ListPosts() len = %d, want 2", len(remainingPosts))
	}
	if remainingPosts[0].Uri != posts[2].Uri {
		t.Fatalf("remainingPosts[0].Uri = %s, want %s", remainingPosts[0].Uri, posts[2].Uri)
	}
	if remainingPosts[1].Uri != posts[1].Uri {
		t.Fatalf("remainingPosts[1].Uri = %s, want %s", remainingPosts[1].Uri, posts[1].Uri)
	}
}

func TestFeedRepositoryDeleteAllPosts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := Open(ctx, Options{
		Path:         filepath.Join(t.TempDir(), "feed-delete-all.db"),
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
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewFeedRepository(db)
	posts := []types.Post{
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
			Cid:       "cid-1",
			IndexedAt: "2026-05-11T01:00:00Z",
			Langs:     []string{"ja"},
		},
		{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user2/app.bsky.feed.post/post2"),
			Cid:       "cid-2",
			IndexedAt: "2026-05-11T02:00:00Z",
			Langs:     []string{"en"},
		},
	}
	for _, post := range posts {
		if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-1", Post: post}); err != nil {
			t.Fatalf("PutPost(%s) error = %v", post.Uri, err)
		}
	}
	if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: "feed-2", Post: posts[0]}); err != nil {
		t.Fatalf("PutPost(feed-2) error = %v", err)
	}

	if err := repo.DeleteAllPosts(ctx, "feed-1"); err != nil {
		t.Fatalf("DeleteAllPosts() error = %v", err)
	}

	remainingFeedOne, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-1"})
	if err != nil {
		t.Fatalf("ListPosts(feed-1) error = %v", err)
	}
	if len(remainingFeedOne) != 0 {
		t.Fatalf("ListPosts(feed-1) len = %d, want 0", len(remainingFeedOne))
	}

	remainingFeedTwo, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: "feed-2"})
	if err != nil {
		t.Fatalf("ListPosts(feed-2) error = %v", err)
	}
	if len(remainingFeedTwo) != 1 {
		t.Fatalf("ListPosts(feed-2) len = %d, want 1", len(remainingFeedTwo))
	}
}
