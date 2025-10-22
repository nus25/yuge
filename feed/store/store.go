package store

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nus25/yuge/feed/config/store"
	cfgTypes "github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/store/editor"
	"github.com/nus25/yuge/types"
)

var _ Store = (*StoreImpl)(nil) // Type check

const fitstCapacity = 1500

// Store is an interface for managing feed posts
type Store interface {
	SetConfig(cfg cfgTypes.StoreConfig)

	Load(ctx context.Context) error
	// Set feed URI
	SetFeedUri(uri types.FeedUri)

	// Add a new post
	Add(did string, rkey string, cid string, t time.Time, langs []string) error

	// Delete specified post
	Delete(did string, rkey string) error

	// Delete posts by DID
	DeleteByDid(did string) (deleted []types.Post, err error)

	// List stored posts
	// If DID is specified, returns only posts for that DID
	List(did string) []types.Post

	// Get specified post
	// Returns nil if not found
	GetPost(did string, rkey string) (post *types.Post, exists bool)

	// Returns post count
	PostCount() int

	// Trim posts to specified count
	Trim(remain int) error

	// Safely shutdown store
	Shutdown(ctx context.Context) error
}

// StoreImpl is basic implementation for managing feed posts
type StoreImpl struct {
	feedId    string
	feedUri   types.FeedUri
	posts     []types.Post
	postIndex map[types.PostUri]struct{} // Index for faster searching
	editor    editor.StoreEditor
	mu        sync.RWMutex
	config    cfgTypes.StoreConfig
	logger    *slog.Logger
}

type StoreOptions struct {
	FeedId  string
	FeedUri types.FeedUri
	Config  cfgTypes.StoreConfig
	Editor  editor.StoreEditor
	Logger  *slog.Logger
}

func NewStore(ctx context.Context, options StoreOptions) (Store, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	l := options.Logger
	if l == nil {
		l = slog.Default().With("component", "Store")
	} else {
		l = l.With("component", "Store")
	}
	e := options.Editor
	if e == nil {
		l.Info("feed editor is not set. store will skip syncing")
	} else {
		if err := e.Open(ctx); err != nil {
			return nil, fmt.Errorf("failed to open editor: %w", err)
		}
	}
	cfg := options.Config
	if cfg == nil {
		cfg = store.DefaultStoreConfig()
	}

	store := &StoreImpl{
		feedId:    options.FeedId,
		feedUri:   options.FeedUri,
		editor:    e,
		posts:     make([]types.Post, 0, fitstCapacity),
		postIndex: make(map[types.PostUri]struct{}),
		config:    cfg,
		logger:    l,
	}
	return store, nil
}

func (s *StoreImpl) SetConfig(cfg cfgTypes.StoreConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Info("updating store config", "config", cfg)
	s.config = cfg
}

func (s *StoreImpl) SetFeedId(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Info("updating feed id", "id", id)
	s.feedId = id
}

func (s *StoreImpl) SetFeedUri(uri types.FeedUri) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Info("updating feed uri", "uri", uri)
	s.feedUri = uri
}

func (s *StoreImpl) Load(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.feedUri == "" {
		return fmt.Errorf("feed uri is not set")
	}
	if err := s.feedUri.Validate(); err != nil {
		return fmt.Errorf("invalid feed uri: %w", err)
	}
	s.posts = make([]types.Post, 0, fitstCapacity)
	s.postIndex = make(map[types.PostUri]struct{})

	posts, err := s.editor.Load(ctx, editor.LoadParams{
		FeedId:  s.feedId,
		FeedUri: s.feedUri,
		Limit:   s.config.GetTrimAt(),
	})
	if err != nil {
		return fmt.Errorf("failed to load posts: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		s.posts = posts
		for _, post := range posts {
			s.postIndex[post.Uri] = struct{}{}
		}
		s.logger.Info("loaded posts", "count", len(posts))
		return nil
	}
}

func (s *StoreImpl) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.editor.Save(ctx, editor.SaveParams{
		Posts:   s.posts,
		FeedUri: s.feedUri,
		FeedId:  s.feedId,
	}); err != nil {
		return fmt.Errorf("failed to save posts: %w", err)
	}
	return nil
}

func (s *StoreImpl) List(did string) []types.Post {
	s.mu.RLock()
	defer s.mu.RUnlock()
	filteredPosts := s.listPost(did)
	return filteredPosts
}

func (s *StoreImpl) listPost(did string) []types.Post {
	// Return all posts if DID is nil
	if did == "" {
		posts := make([]types.Post, len(s.posts))
		copy(posts, s.posts)
		return posts
	}

	// Extract only posts matching DID if specified
	filteredPosts := make([]types.Post, 0)
	prefix := "at://" + did + "/"
	for _, post := range s.posts {
		if strings.HasPrefix(string(post.Uri), prefix) {
			filteredPosts = append(filteredPosts, post)
		}
	}
	return filteredPosts
}

func (s *StoreImpl) Add(did string, rkey string, cid string, t time.Time, langs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	uri := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, rkey)
	if _, exists := s.postIndex[types.PostUri(uri)]; exists {
		return nil
	}

	post := types.Post{
		Uri:       types.PostUri(uri),
		Cid:       cid,
		IndexedAt: t.UTC().Format(time.RFC3339Nano),
		//Language is not supported in cache
	}

	s.posts = append(s.posts, post)
	s.postIndex[post.Uri] = struct{}{}

	if s.editor != nil {
		if err := s.editor.Add(editor.PostParams{
			FeedUri:   s.feedUri,
			Did:       did,
			Rkey:      rkey,
			Cid:       cid,
			IndexedAt: t,
			Langs:     langs,
		}); err != nil {
			return err
		}
	}

	// Check if trim needed
	if s.config != nil && s.config.GetTrimAt() > 0 && len(s.posts) > s.config.GetTrimAt() {
		if err := s.trim(s.config.GetTrimRemain()); err != nil {
			return err
		}
	}

	return nil
}

func (s *StoreImpl) Delete(did string, rkey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deletePost(did, rkey)
}

func (s *StoreImpl) DeleteByDid(did string) (deleted []types.Post, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	uriPrefix := fmt.Sprintf("at://%s/app.bsky.feed.post/", did)
	var remainingPosts []types.Post
	for _, post := range s.posts {
		if strings.HasPrefix(string(post.Uri), uriPrefix) {
			deleted = append(deleted, post)
			delete(s.postIndex, post.Uri)
		} else {
			remainingPosts = append(remainingPosts, post)
		}
	}
	s.posts = remainingPosts

	if s.editor != nil {
		err := s.editor.DeleteByDid(s.feedUri, did)
		if err != nil {
			return nil, err
		}
	}

	return deleted, nil
}

func (s *StoreImpl) deletePost(did string, rkey string) error {
	uri := fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, rkey)
	if _, exists := s.postIndex[types.PostUri(uri)]; !exists {
		return nil
	}

	for i, post := range s.posts {
		if post.Uri == types.PostUri(uri) {
			s.posts = append(s.posts[:i], s.posts[i+1:]...)
			delete(s.postIndex, post.Uri)
			break
		}
	}
	if s.editor != nil {
		return s.editor.Delete(editor.DeleteParams{
			FeedUri: s.feedUri,
			Did:     did,
			Rkey:    rkey,
		})
	}
	return nil
}

func (s *StoreImpl) GetPost(did string, rkey string) (post *types.Post, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uri := types.PostUri(fmt.Sprintf("at://%s/app.bsky.feed.post/%s", did, rkey))
	if _, exists = s.postIndex[uri]; exists {
		for _, post := range s.posts {
			if post.Uri == uri {
				return &post, true
			}
		}
	}
	return nil, false
}

func (s *StoreImpl) Trim(remain int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.trim(remain)
}

func (s *StoreImpl) trim(remain int) error {
	s.logger.Info("trimming posts", "remain", remain, "current", len(s.posts))

	if len(s.posts) <= remain {
		return nil
	}
	sort.Slice(s.posts, func(i, j int) bool {
		return s.posts[i].IndexedAt > s.posts[j].IndexedAt
	})

	// Create new slice to hold up to trim count
	newPosts := make([]types.Post, remain, len(s.posts)+1)
	copy(newPosts, s.posts[:remain])

	// Recreate index with minimum required size
	newIndex := make(map[types.PostUri]struct{}, remain)
	for _, post := range newPosts {
		newIndex[post.Uri] = struct{}{}
	}

	s.posts = newPosts
	s.postIndex = newIndex

	if s.editor != nil {
		return s.editor.Trim(editor.TrimParams{
			FeedUri: s.feedUri,
			Count:   remain,
		})
	}
	return nil
}

func (s *StoreImpl) PostCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.posts)
}
