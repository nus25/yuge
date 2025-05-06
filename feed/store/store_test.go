package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/nus25/yuge/feed/store/editor"
	"github.com/nus25/yuge/types"
)

// Mocks
type MockEditor struct {
	posts []types.Post
}

func (m *MockEditor) Open(ctx context.Context) error {
	return nil
}

func (m *MockEditor) Load(ctx context.Context, params editor.LoadParams) ([]types.Post, error) {
	return m.posts, nil
}

func (m *MockEditor) Save(ctx context.Context, params editor.SaveParams) error {
	m.posts = params.Posts
	return nil
}

func (m *MockEditor) Add(params editor.PostParams) error {
	m.posts = append(m.posts, types.Post{
		Feed:      params.FeedUri,
		Uri:       types.PostUri("at://" + params.Did + "/app.bsky.feed.post/" + params.Rkey),
		Cid:       params.Cid,
		IndexedAt: params.IndexedAt.Format(time.RFC3339),
	})
	return nil
}

func (m *MockEditor) Delete(params editor.DeleteParams) error {
	for i, p := range m.posts {
		if string(p.Uri) == "at://"+params.Did+"/app.bsky.feed.post/"+params.Rkey {
			m.posts = append(m.posts[:i], m.posts[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MockEditor) Trim(params editor.TrimParams) error {
	count := params.Count
	if len(m.posts) > count {
		m.posts = m.posts[:count]
	}
	return nil
}

func (m *MockEditor) Close(ctx context.Context) error {
	return nil
}

// tests
func TestStore(t *testing.T) {
	logger := slog.Default()
	ctx := context.Background()
	t.Run("basic operations", func(t *testing.T) {
		storeOpts := StoreOptions{
			Logger:  logger,
			FeedId:  "test",
			FeedUri: types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test"),
			Editor:  &MockEditor{},
		}
		s, err := NewStore(ctx, storeOpts)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		// Test SetFeedUri
		feedUri := types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test")
		s.SetFeedUri(feedUri)

		// Test Add
		did := "did:plc:1234"
		rkey := "test1"
		cid := "bafyreia"
		now := time.Now()
		langs := []string{"jp", "en"}

		err = s.Add(did, rkey, cid, now, langs)
		if err != nil {
			t.Fatalf("failed to add post: %v", err)
		}

		// Test Delete
		err = s.Delete(did, rkey)
		if err != nil {
			t.Fatalf("failed to delete post: %v", err)
		}

		_, exists := s.GetPost(did, rkey)
		if exists {
			t.Error("post should not exist after deletion")
		}
	})

	t.Run("load with no feed uri", func(t *testing.T) {
		storeOpts := StoreOptions{
			Logger: logger,
			FeedId: "test",
			Editor: &MockEditor{},
		}
		s, err := NewStore(ctx, storeOpts)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		err = s.Load(ctx)
		if err == nil {
			t.Error("expected error when loading with no feed uri")
		}
	})

	t.Run("concurrent operations", func(t *testing.T) {
		storeOpts := StoreOptions{
			Logger:  logger,
			FeedId:  "test",
			FeedUri: types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test"),
		}
		s, err := NewStore(ctx, storeOpts)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		feedUri := types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test")
		s.SetFeedUri(feedUri)

		// Add multiple posts concurrently
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(i int) {
				did := "did:plc:1234"
				rkey := fmt.Sprintf("test%d", i)
				cid := fmt.Sprintf("bafyreia%d", i)
				err := s.Add(did, rkey, cid, time.Now(), []string{"jp", "us"})
				if err != nil {
					t.Errorf("failed to add post: %v", err)
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Run("with editor", func(t *testing.T) {
		mockEditor := &MockEditor{}
		storeOpts := StoreOptions{
			Logger:  logger,
			FeedId:  "test",
			FeedUri: types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test"),
			Editor:  mockEditor,
		}
		s, err := NewStore(ctx, storeOpts)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		feedUri := types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test")
		s.SetFeedUri(feedUri)

		did := "did:plc:1234"
		rkey := "test1"
		cid := "bafyreia"
		now := time.Now()
		langs := []string{"jp", "en"}

		err = s.Add(did, rkey, cid, now, langs)
		if err != nil {
			t.Fatalf("failed to add post: %v", err)
		}

		err = s.Shutdown(ctx)
		if err != nil {
			t.Fatalf("failed to shutdown store: %v", err)
		}
	})
}

func TestList(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()

	t.Run("list all posts", func(t *testing.T) {
		storeOpts := StoreOptions{
			Logger:  logger,
			FeedId:  "test",
			FeedUri: types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test"),
		}
		s, err := NewStore(ctx, storeOpts)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		feedUri := types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test")
		s.SetFeedUri(feedUri)

		// Add test posts
		did1 := "did:plc:1234"
		did2 := "did:plc:5678"
		posts := []struct {
			did   string
			rkey  string
			cid   string
			langs []string
		}{
			{did1, "test1", "bafyreia1", []string{"jp"}},
			{did1, "test2", "bafyreia2", nil},
			{did2, "test3", "bafyreia3", []string{}},
		}

		for _, p := range posts {
			err := s.Add(p.did, p.rkey, p.cid, time.Now(), p.langs)
			if err != nil {
				t.Fatalf("failed to add post: %v", err)
			}
		}

		// List all posts
		allPosts := s.List("")
		if len(allPosts) != 3 {
			t.Errorf("expected 3 posts, got %d", len(allPosts))
		}

		// List posts for specific DID
		did1Posts := s.List(did1)
		if len(did1Posts) != 2 {
			t.Errorf("expected 2 posts for did1, got %d", len(did1Posts))
		}
		for _, post := range did1Posts {
			if !strings.HasPrefix(string(post.Uri), "at://"+did1+"/") {
				t.Errorf("post URI %s does not have expected prefix at://%s/", post.Uri, did1)
			}
		}

		did2Posts := s.List(did2)
		if len(did2Posts) != 1 {
			t.Errorf("expected 1 post for did2, got %d", len(did2Posts))
		}
		if !strings.HasPrefix(string(did2Posts[0].Uri), "at://"+did2+"/") {
			t.Errorf("post URI %s does not have expected prefix at://%s/", did2Posts[0].Uri, did2)
		}

		// List posts for non-existent DID
		nonExistDid := "did:plc:9999"
		nonExistPosts := s.List(nonExistDid)
		if len(nonExistPosts) != 0 {
			t.Errorf("expected 0 posts for non-existent DID, got %d", len(nonExistPosts))
		}
	})
}

func TestDeleteByDid(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	mockEditor := MockEditor{}
	t.Run("delete posts by did", func(t *testing.T) {
		storeOpts := StoreOptions{
			Logger:  logger,
			FeedId:  "test",
			FeedUri: types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test"),
			Editor:  &mockEditor,
		}
		s, err := NewStore(ctx, storeOpts)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		feedUri := types.FeedUri("at://did:plc:1234/app.bsky.feed.generator/test")
		s.SetFeedUri(feedUri)

		// Add test posts
		did1 := "did:plc:1234"
		did2 := "did:plc:5678"
		posts := []struct {
			did   string
			rkey  string
			cid   string
			langs []string
		}{
			{did1, "test1", "bafyreia1", []string{"jp"}},
			{did1, "test2", "bafyreia2", []string{"jp"}},
			{did2, "test3", "bafyreia3", []string{"jp"}},
		}

		for _, p := range posts {
			err := s.Add(p.did, p.rkey, p.cid, time.Now(), p.langs)
			if err != nil {
				t.Fatalf("failed to add post: %v", err)
			}
		}

		// Delete posts for did1
		deleted, err := s.DeleteByDid(did1)
		if err != nil {
			t.Fatalf("failed to delete posts by DID: %v", err)
		}
		if len(deleted) != 2 {
			t.Errorf("expected 2 deleted posts, got %d", len(deleted))
		}

		// Verify deleted posts
		for _, d := range deleted {
			uri := string(d.Uri)
			parts := strings.Split(uri, "/")
			if parts[2] != did1 {
				t.Errorf("expected DID %s, got %s", did1, parts[2])
			}
			if parts[4] != "test1" && parts[4] != "test2" {
				t.Errorf("unexpected rkey: %s", parts[4])
			}
		}

		// Verify remaining posts
		remainingPosts := s.List("")
		if len(remainingPosts) != 1 {
			t.Errorf("expected 1 remaining post, got %d", len(remainingPosts))
		}
		if !strings.HasPrefix(string(remainingPosts[0].Uri), "at://"+did2+"/") {
			t.Errorf("remaining post URI %s does not have expected prefix at://%s/", remainingPosts[0].Uri, did2)
		}
	})
}
