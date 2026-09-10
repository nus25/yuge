package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

func Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS feed_posts (
			feed_id TEXT NOT NULL,
			feed_uri TEXT NOT NULL,
			post_uri TEXT NOT NULL,
			did TEXT NOT NULL,
			rkey TEXT NOT NULL,
			cid TEXT NOT NULL,
			indexed_at TEXT NOT NULL,
			langs_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (feed_id, post_uri)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_feed_posts_feed_time
			ON feed_posts(feed_id, indexed_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_feed_posts_feed_did
			ON feed_posts(feed_id, did);`,
		`CREATE TABLE IF NOT EXISTS feed_state (
			feed_id TEXT PRIMARY KEY,
			feed_uri TEXT NOT NULL,
			status TEXT NOT NULL,
			config_revision TEXT,
			last_loaded_at TEXT,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS projection_outbox (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			feed_id TEXT NOT NULL,
			feed_uri TEXT NOT NULL,
			target TEXT NOT NULL,
			operation TEXT NOT NULL,
			mutation_id TEXT NOT NULL,
			subject_key TEXT NOT NULL,
			op_key TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			status TEXT NOT NULL,
			retry_count INTEGER NOT NULL DEFAULT 0,
			next_retry_at TEXT,
			last_error TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT,
			UNIQUE(target, op_key)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_projection_outbox_scan
			ON projection_outbox(target, status, next_retry_at, id);`,
		`CREATE INDEX IF NOT EXISTS idx_projection_outbox_completed
			ON projection_outbox(target, status, completed_at, id);`,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("execute migration statement: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}
	return nil
}
