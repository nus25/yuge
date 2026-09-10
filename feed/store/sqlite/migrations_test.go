package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAndMigrateCreatesInitialSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "yuge.db")

	db, err := Open(ctx, Options{
		Path:         dbPath,
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
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() second call error = %v", err)
	}

	assertPragmaString(t, db, "journal_mode", "wal")
	assertPragmaInt(t, db, "foreign_keys", 1)
	assertPragmaInt(t, db, "busy_timeout", 100)

	for _, tableName := range []string{"feed_posts", "feed_state", "projection_outbox"} {
		if !tableExists(t, db, tableName) {
			t.Fatalf("expected table %q to exist", tableName)
		}
	}
	for _, indexName := range []string{"idx_feed_posts_feed_time", "idx_feed_posts_feed_did", "idx_projection_outbox_scan", "idx_projection_outbox_completed"} {
		if !indexExists(t, db, indexName) {
			t.Fatalf("expected index %q to exist", indexName)
		}
	}
}

func assertPragmaString(t *testing.T, db *sql.DB, pragma string, want string) {
	t.Helper()

	var got string
	query := "PRAGMA " + pragma + ";"
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("QueryRow(%q) error = %v", query, err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %q, want %q", pragma, got, want)
	}
}

func assertPragmaInt(t *testing.T, db *sql.DB, pragma string, want int) {
	t.Helper()

	var got int
	query := "PRAGMA " + pragma + ";"
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("QueryRow(%q) error = %v", query, err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %d, want %d", pragma, got, want)
	}
}

func tableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()

	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?;",
		tableName,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("tableExists(%q) query error = %v", tableName, err)
	}
	return name == tableName
}

func indexExists(t *testing.T, db *sql.DB, indexName string) bool {
	t.Helper()

	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?;",
		indexName,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("indexExists(%q) query error = %v", indexName, err)
	}
	return name == indexName
}
