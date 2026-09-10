package subscriber

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	storepkg "github.com/nus25/yuge/feed/store"
	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	"github.com/nus25/yuge/types"
)

const defaultSQLiteFilename = "yuge.db"

type sqliteRuntimePersistence struct {
	mutationDB          *sql.DB
	loaderDB            *sql.DB
	mutationCoordinator *FeedMutationCoordinator
	postLoader          storepkg.PostLoader
}

func (p *sqliteRuntimePersistence) Close() error {
	if p == nil {
		return nil
	}
	var closeErr error
	if p.loaderDB != nil {
		if err := p.loaderDB.Close(); err != nil {
			closeErr = fmt.Errorf("close sqlite loader database: %w", err)
		}
	}
	if p.mutationDB != nil {
		if err := p.mutationDB.Close(); err != nil {
			if closeErr != nil {
				closeErr = fmt.Errorf("%v; close sqlite mutation database: %w", closeErr, err)
			} else {
				closeErr = fmt.Errorf("close sqlite mutation database: %w", err)
			}
		}
	}
	return closeErr
}

type sqlitePostLoader struct {
	repo storerepo.FeedRepository
}

func newSQLitePostLoader(db *sql.DB) storepkg.PostLoader {
	return &sqlitePostLoader{repo: storesqlite.NewFeedRepository(db)}
}

func (l *sqlitePostLoader) LoadPosts(ctx context.Context, params storepkg.LoadPostsParams) ([]types.Post, error) {
	if l == nil || l.repo == nil {
		return nil, nil
	}
	posts, err := l.repo.ListPosts(ctx, storerepo.ListPostsParams{
		FeedID: params.FeedID,
		Limit:  params.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("load sqlite posts: %w", err)
	}
	return posts, nil
}

func openSQLiteRuntimePersistence(ctx context.Context, dataDir string) (*sqliteRuntimePersistence, error) {
	mutationDB, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(dataDir, defaultSQLiteFilename),
		SyncMode:     "NORMAL",
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite mutation database: %w", err)
	}
	if err := storesqlite.Migrate(ctx, mutationDB); err != nil {
		_ = mutationDB.Close()
		return nil, fmt.Errorf("migrate sqlite mutation database: %w", err)
	}

	loaderDB, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(dataDir, defaultSQLiteFilename),
		SyncMode:     "NORMAL",
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		_ = mutationDB.Close()
		return nil, fmt.Errorf("open sqlite loader database: %w", err)
	}

	return &sqliteRuntimePersistence{
		mutationDB:          mutationDB,
		loaderDB:            loaderDB,
		mutationCoordinator: NewFeedMutationCoordinator(NewSQLiteFeedMutationTransactor(mutationDB, SQLiteFeedMutationTransactorOptions{})),
		postLoader:          newSQLitePostLoader(loaderDB),
	}, nil
}

func openSQLiteMutationCoordinator(ctx context.Context, dataDir string) (*sql.DB, *FeedMutationCoordinator, error) {
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(dataDir, defaultSQLiteFilename),
		SyncMode:     "NORMAL",
		BusyTimeout:  5 * time.Second,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite mutation database: %w", err)
	}
	if err := storesqlite.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate sqlite mutation database: %w", err)
	}
	return db, NewFeedMutationCoordinator(NewSQLiteFeedMutationTransactor(db, SQLiteFeedMutationTransactorOptions{})), nil
}
