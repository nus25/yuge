package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestLogicBlockStateRepositoryPersistsStateAcrossDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "yuge.db")

	db, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	repository := NewLogicBlockStateRepository(db)
	if err := repository.SaveLogicBlockState(ctx, "feed-1", "active-authors", 1, []byte(`{"did:plc:author":{"rkey":"trigger"}}`)); err != nil {
		t.Fatalf("SaveLogicBlockState() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, Options{Path: path})
	if err != nil {
		t.Fatalf("reopen database error = %v", err)
	}
	defer reopened.Close()
	state, found, err := NewLogicBlockStateRepository(reopened).LoadLogicBlockState(ctx, "feed-1", "active-authors")
	if err != nil {
		t.Fatalf("LoadLogicBlockState() error = %v", err)
	}
	if !found {
		t.Fatal("LoadLogicBlockState() did not find saved state")
	}
	if got, want := string(state), `{"did:plc:author":{"rkey":"trigger"}}`; got != want {
		t.Fatalf("state = %s, want %s", got, want)
	}
}
