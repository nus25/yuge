package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type LogicBlockStateRepository struct {
	db dbtx
}

func NewLogicBlockStateRepository(db dbtx) *LogicBlockStateRepository {
	return &LogicBlockStateRepository{db: db}
}

func (r *LogicBlockStateRepository) LoadLogicBlockState(ctx context.Context, feedID string, blockKey string) ([]byte, bool, error) {
	var state string
	err := r.db.QueryRowContext(ctx, `
		SELECT state_json FROM logic_block_state WHERE feed_id = ? AND block_key = ?;
	`, feedID, blockKey).Scan(&state)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load logic block state: %w", err)
	}
	return []byte(state), true, nil
}

func (r *LogicBlockStateRepository) SaveLogicBlockState(ctx context.Context, feedID string, blockKey string, version int, state []byte) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO logic_block_state (feed_id, block_key, schema_version, state_json, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(feed_id, block_key) DO UPDATE SET
			schema_version = excluded.schema_version,
			state_json = excluded.state_json,
			updated_at = excluded.updated_at;
	`, feedID, blockKey, version, string(state), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save logic block state: %w", err)
	}
	return nil
}
