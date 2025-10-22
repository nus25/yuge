package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nus25/yuge/types"
)

var _ StoreEditor = (*FileEditor)(nil) //type check

const (
	StoreFileName = "store.json"
)

type FileEditor struct {
	logger *slog.Logger
	mu     sync.RWMutex
	dir    string
}

func NewFileEditor(dir string, logger *slog.Logger) (*FileEditor, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &FileEditor{
		dir:    dir,
		logger: logger,
		mu:     sync.RWMutex{},
	}, nil
}

func (e *FileEditor) Open(initCtx context.Context) error {
	if err := e.initialize(initCtx); err != nil {
		return fmt.Errorf("failed to initialize file editor: %w", err)
	}

	return nil
}

func (e *FileEditor) createFeedDir(feedId string) (feedDir string, err error) {
	feedDir = filepath.Join(e.dir, feedId)
	if _, err := os.Stat(feedDir); os.IsNotExist(err) {
		if err := os.MkdirAll(feedDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create feed directory: %w", err)
		}
	}
	return feedDir, nil
}

func (e *FileEditor) initialize(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		dir := e.dir
		// create directory if not exists
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}
		return nil
	}
}

func (e *FileEditor) Load(ctx context.Context, params LoadParams) ([]types.Post, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		e.mu.RLock()
		defer e.mu.RUnlock()
		if params.FeedId == "" {
			return nil, fmt.Errorf("feed id is required")
		}
		feedDir, err := e.createFeedDir(params.FeedId)
		if err != nil {
			return nil, fmt.Errorf("failed to create feed directory: %w", err)
		}
		filePath := filepath.Join(feedDir, StoreFileName)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// Create feed directory if not exists
			e.logger.Info("file editor: creating empty file", "path", filePath) // create empty file
			if err := os.WriteFile(filePath, []byte("[]"), 0644); err != nil {
				return nil, fmt.Errorf("failed to create empty file: %w", err)
			}
		}

		e.logger.Info("loading feed file", "path", filePath)

		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		var posts []types.Post
		if err := json.Unmarshal(data, &posts); err != nil {
			return nil, fmt.Errorf("failed to unmarshal posts: %w", err)
		}

		// Sort posts by IndexedAt in descending order (newest first)
		sort.Slice(posts, func(i, j int) bool {
			return posts[i].IndexedAt > posts[j].IndexedAt
		})

		// Apply limit if specified
		if params.Limit > 0 && len(posts) > params.Limit {
			posts = posts[:params.Limit]
		}

		return posts, nil
	}
}

func (e *FileEditor) Add(params PostParams) error {
	return nil
}

func (e *FileEditor) Delete(params DeleteParams) error {
	return nil
}

func (e *FileEditor) DeleteByDid(feedUri types.FeedUri, did string) error {
	return nil
}

func (e *FileEditor) Trim(params TrimParams) error {
	return nil
}

func (e *FileEditor) StartWorker(ctx context.Context) error {
	return nil
}

func (e *FileEditor) Save(ctx context.Context, params SaveParams) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		e.mu.Lock()
		defer e.mu.Unlock()
		feedDir, err := e.createFeedDir(params.FeedId)
		if err != nil {
			return fmt.Errorf("failed to create feed directory: %w", err)
		}
		filePath := filepath.Join(feedDir, StoreFileName)
		e.logger.Info("saving feed file", "path", filePath)
		data, err := json.MarshalIndent(params.Posts, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal posts: %w", err)
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		return nil
	}
}

func (e *FileEditor) Close(ctx context.Context) error {
	return nil
}
