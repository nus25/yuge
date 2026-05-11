package subscriber

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	"github.com/nus25/yuge/types"
)

const legacyStoreSnapshotFileName = "store.json"

type importLegacyFileSnapshotsOptions struct {
	EnqueueProjection bool
	ProjectionTarget  string
}

type importLegacyFileSnapshotsResult struct {
	ImportedFeeds         int
	ImportedPosts         int
	EnqueuedProjectionOps int
}

func importLegacyFileSnapshots(ctx context.Context, logger *slog.Logger, definitionProvider FeedDefinitionProvider, dataDir string, db *sql.DB, opts importLegacyFileSnapshotsOptions) (importLegacyFileSnapshotsResult, error) {
	if definitionProvider == nil {
		return importLegacyFileSnapshotsResult{}, fmt.Errorf("feed definition provider is required")
	}
	if db == nil {
		return importLegacyFileSnapshotsResult{}, fmt.Errorf("sqlite database is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if opts.ProjectionTarget == "" {
		opts.ProjectionTarget = "gyoka"
	}

	definitions, err := definitionProvider.GetFeedDefinitionList()
	if err != nil {
		return importLegacyFileSnapshotsResult{}, fmt.Errorf("get feed definition list: %w", err)
	}

	feedRepo := storesqlite.NewFeedRepository(db)
	outboxRepo := projectionsqlite.NewOutboxRepository(db)
	result := importLegacyFileSnapshotsResult{}

	for _, definition := range definitions.Feeds {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		legacyPath := filepath.Join(dataDir, definition.ID, legacyStoreSnapshotFileName)
		posts, ok, err := loadLegacySnapshotPosts(legacyPath)
		if err != nil {
			return result, fmt.Errorf("load legacy snapshot for feed %s: %w", definition.ID, err)
		}
		if !ok || len(posts) == 0 {
			continue
		}

		result.ImportedFeeds++
		for _, post := range posts {
			post.Feed = types.FeedUri(definition.URI)
			if err := feedRepo.PutPost(ctx, storerepo.PutPostParams{
				FeedID: definition.ID,
				Post:   post,
			}); err != nil {
				return result, fmt.Errorf("put imported post for feed %s: %w", definition.ID, err)
			}
			result.ImportedPosts++

			if !opts.EnqueueProjection {
				continue
			}
			payloadJSON, err := json.Marshal(struct {
				FeedURI types.FeedUri `json:"feedUri"`
				Post    types.Post    `json:"post"`
			}{
				FeedURI: types.FeedUri(definition.URI),
				Post:    post,
			})
			if err != nil {
				return result, fmt.Errorf("marshal imported projection payload for feed %s: %w", definition.ID, err)
			}
			mutationID := fmt.Sprintf("legacy-import:%s", definition.ID)
			subjectKey := fmt.Sprintf("%s:%s", definition.ID, post.Uri)
			opKey := fmt.Sprintf("legacy-import:add:%s", subjectKey)
			if err := outboxRepo.Enqueue(ctx, projectionrepo.EnqueueParams{
				FeedID:      definition.ID,
				FeedURI:     definition.URI,
				Target:      opts.ProjectionTarget,
				Operation:   "add",
				MutationID:  mutationID,
				SubjectKey:  subjectKey,
				OpKey:       opKey,
				PayloadJSON: string(payloadJSON),
				Status:      "pending",
			}); err != nil {
				return result, fmt.Errorf("enqueue imported projection for feed %s: %w", definition.ID, err)
			}
			result.EnqueuedProjectionOps++
		}
		logger.Info("imported legacy snapshot", "feedId", definition.ID, "posts", len(posts), "enqueueProjection", opts.EnqueueProjection)
	}

	return result, nil
}

func loadLegacySnapshotPosts(path string) ([]types.Post, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read legacy snapshot: %w", err)
	}
	var posts []types.Post
	if err := json.Unmarshal(data, &posts); err != nil {
		return nil, false, fmt.Errorf("unmarshal legacy snapshot: %w", err)
	}
	return posts, true, nil
}
