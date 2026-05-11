package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
)

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type OutboxRepository struct {
	db dbtx
}

func NewOutboxRepository(db dbtx) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) Enqueue(ctx context.Context, params projectionrepo.EnqueueParams) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO projection_outbox (
			feed_id, feed_uri, target, operation, mutation_id, subject_key, op_key, payload_json,
			status, retry_count, next_retry_at, last_error, created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, '', ?, ?, NULL)
		ON CONFLICT(target, op_key) DO NOTHING;
	`, params.FeedID, params.FeedURI, params.Target, params.Operation, params.MutationID, params.SubjectKey, params.OpKey, params.PayloadJSON, params.Status, now, now)
	if err != nil {
		return fmt.Errorf("enqueue outbox entry: %w", err)
	}
	return nil
}

func (r *OutboxRepository) ListByStatus(ctx context.Context, params projectionrepo.ListByStatusParams) ([]projectionrepo.Entry, error) {
	query := `
		SELECT id, feed_id, feed_uri, target, operation, mutation_id, subject_key, op_key,
			payload_json, status, retry_count, COALESCE(next_retry_at, ''), COALESCE(last_error, ''),
			created_at, updated_at, COALESCE(completed_at, '')
		FROM projection_outbox
		WHERE target = ? AND status = ?
		ORDER BY id ASC`
	args := []any{params.Target, params.Status}
	if params.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, params.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list outbox entries: %w", err)
	}
	defer rows.Close()

	entries := make([]projectionrepo.Entry, 0)
	for rows.Next() {
		var entry projectionrepo.Entry
		if err := rows.Scan(
			&entry.ID,
			&entry.FeedID,
			&entry.FeedURI,
			&entry.Target,
			&entry.Operation,
			&entry.MutationID,
			&entry.SubjectKey,
			&entry.OpKey,
			&entry.PayloadJSON,
			&entry.Status,
			&entry.RetryCount,
			&entry.NextRetryAt,
			&entry.LastError,
			&entry.CreatedAt,
			&entry.UpdatedAt,
			&entry.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox rows: %w", err)
	}
	return entries, nil
}
