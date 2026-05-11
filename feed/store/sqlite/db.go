package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const defaultBusyTimeout = 5 * time.Second

type Options struct {
	Path         string
	SyncMode     string
	BusyTimeout  time.Duration
	MaxOpenConns int
	MaxIdleConns int
}

func Open(ctx context.Context, opts Options) (*sql.DB, error) {
	if opts.Path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(opts.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite3", opts.Path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if opts.MaxOpenConns <= 0 {
		opts.MaxOpenConns = 1
	}
	if opts.MaxIdleConns < 0 {
		opts.MaxIdleConns = 0
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = 1
	}
	if opts.BusyTimeout <= 0 {
		opts.BusyTimeout = defaultBusyTimeout
	}
	if opts.SyncMode == "" {
		opts.SyncMode = "NORMAL"
	}

	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxIdleConns)

	if err := applyPragmas(ctx, db, opts); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return db, nil
}

func applyPragmas(ctx context.Context, db *sql.DB, opts Options) error {
	pragmaStatements := []string{
		"PRAGMA journal_mode = WAL;",
		fmt.Sprintf("PRAGMA synchronous = %s;", strings.ToUpper(opts.SyncMode)),
		fmt.Sprintf("PRAGMA busy_timeout = %d;", opts.BusyTimeout.Milliseconds()),
		"PRAGMA foreign_keys = ON;",
		"PRAGMA temp_store = MEMORY;",
		"PRAGMA wal_autocheckpoint = 1000;",
	}

	for _, stmt := range pragmaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply pragma %q: %w", stmt, err)
		}
	}
	return nil
}
