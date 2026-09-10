package subscriber

import (
	"context"
	"database/sql"
	"fmt"

	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
)

type SQLiteFeedMutationTransactorOptions struct {
	FeedRepositoryFactory   func(*sql.Tx) storerepo.FeedRepository
	OutboxRepositoryFactory func(*sql.Tx) projectionrepo.OutboxRepository
}

type SQLiteFeedMutationTransactor struct {
	db                      *sql.DB
	feedRepositoryFactory   func(*sql.Tx) storerepo.FeedRepository
	outboxRepositoryFactory func(*sql.Tx) projectionrepo.OutboxRepository
}

func NewSQLiteFeedMutationTransactor(db *sql.DB, opts SQLiteFeedMutationTransactorOptions) *SQLiteFeedMutationTransactor {
	transactor := &SQLiteFeedMutationTransactor{
		db:                      db,
		feedRepositoryFactory:   opts.FeedRepositoryFactory,
		outboxRepositoryFactory: opts.OutboxRepositoryFactory,
	}
	if transactor.feedRepositoryFactory == nil {
		transactor.feedRepositoryFactory = func(tx *sql.Tx) storerepo.FeedRepository {
			return storesqlite.NewFeedRepository(tx)
		}
	}
	if transactor.outboxRepositoryFactory == nil {
		transactor.outboxRepositoryFactory = func(tx *sql.Tx) projectionrepo.OutboxRepository {
			return projectionsqlite.NewOutboxRepository(tx)
		}
	}
	return transactor
}

func (t *SQLiteFeedMutationTransactor) WithinTx(ctx context.Context, fn func(context.Context, storerepo.FeedRepository, projectionrepo.OutboxRepository) error) error {
	if t == nil || t.db == nil {
		return fmt.Errorf("sqlite database is required")
	}
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin feed mutation transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(ctx, t.feedRepositoryFactory(tx), t.outboxRepositoryFactory(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit feed mutation transaction: %w", err)
	}
	committed = true
	return nil
}
