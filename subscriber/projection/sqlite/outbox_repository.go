package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
)

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type OutboxRepository struct {
	db dbtx
}

const defaultProcessingReclaimTimeout = 2 * time.Minute

func NewOutboxRepository(db dbtx) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func listReadyPendingEntries(ctx context.Context, querier dbtx, target string, now string, staleBefore string, limit int) ([]projectionrepo.Entry, error) {
	if limit <= 0 {
		limit = 1
	}
	rows, err := querier.QueryContext(ctx, `
		SELECT id, feed_id, feed_uri, target, operation, mutation_id, subject_key, op_key,
			payload_json, status, retry_count, COALESCE(next_retry_at, ''), COALESCE(last_error, ''),
			created_at, updated_at, COALESCE(completed_at, '')
		FROM projection_outbox
		WHERE target = ?
			AND (
				(
					status = 'pending'
					AND (
						COALESCE(last_error, '') = ''
						OR (next_retry_at IS NOT NULL AND next_retry_at <> '' AND next_retry_at <= ?)
					)
				)
				OR (status = 'processing' AND updated_at <= ?)
			)
		ORDER BY id ASC
		LIMIT ?;
	`, target, now, staleBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("list ready pending outbox entries: %w", err)
	}
	defer rows.Close()

	entries := make([]projectionrepo.Entry, 0, limit)
	for rows.Next() {
		entry, err := scanOutboxEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan ready pending outbox entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ready pending outbox entries: %w", err)
	}
	return entries, nil
}

func scanOutboxEntry(scanner interface{ Scan(dest ...any) error }) (projectionrepo.Entry, error) {
	var entry projectionrepo.Entry
	if err := scanner.Scan(
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
		return projectionrepo.Entry{}, err
	}
	return entry, nil
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

func (r *OutboxRepository) ClearFeed(ctx context.Context, params projectionrepo.ClearFeedParams) error {
	payloadJSON, err := json.Marshal(struct {
		FeedURI string `json:"feedUri"`
		Count   int    `json:"count"`
	}{
		FeedURI: params.FeedURI,
		Count:   params.Count,
	})
	if err != nil {
		return fmt.Errorf("marshal clear feed payload: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM projection_outbox
		WHERE feed_id = ? AND target = ? AND status = 'pending';
	`, params.FeedID, params.Target); err != nil {
		return fmt.Errorf("delete pending outbox entries for clear feed: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	subjectKey := fmt.Sprintf("%s:trim", params.FeedID)
	opKey := fmt.Sprintf("%s:trim:%s:%d", params.MutationID, params.FeedID, params.Count)
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO projection_outbox (
			feed_id, feed_uri, target, operation, mutation_id, subject_key, op_key, payload_json,
			status, retry_count, next_retry_at, last_error, created_at, updated_at, completed_at
		) VALUES (?, ?, ?, 'trim', ?, ?, ?, ?, 'pending', 0, NULL, '', ?, ?, NULL)
		ON CONFLICT(target, op_key) DO NOTHING;
	`, params.FeedID, params.FeedURI, params.Target, params.MutationID, subjectKey, opKey, string(payloadJSON), now, now)
	if err != nil {
		return fmt.Errorf("enqueue clear feed trim entry: %w", err)
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
		entry, err := scanOutboxEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan outbox row: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox rows: %w", err)
	}
	return entries, nil
}

func (r *OutboxRepository) CountByStatus(ctx context.Context, params projectionrepo.CountByStatusParams) ([]projectionrepo.StatusCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT status, count
		FROM (
			SELECT status, COUNT(*) AS count, 0 AS sort_order
			FROM projection_outbox
			WHERE target = ?
			GROUP BY status

			UNION ALL

			SELECT 'failed' AS status, COUNT(*) AS count, 1 AS sort_order
			FROM projection_outbox
			WHERE target = ? AND status = 'pending' AND COALESCE(last_error, '') <> ''
		)
		ORDER BY sort_order ASC, status ASC
	`, params.Target, params.Target)
	if err != nil {
		return nil, fmt.Errorf("count outbox entries by status: %w", err)
	}
	defer rows.Close()

	counts := make([]projectionrepo.StatusCount, 0)
	for rows.Next() {
		var count projectionrepo.StatusCount
		if err := rows.Scan(&count.Status, &count.Count); err != nil {
			return nil, fmt.Errorf("scan outbox status count: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate outbox status counts: %w", err)
	}
	return counts, nil
}

func (r *OutboxRepository) ClaimNextPending(ctx context.Context, params projectionrepo.ClaimNextPendingParams) (projectionrepo.Entry, bool, error) {
	entries, ok, err := r.ClaimNextPendingBatch(ctx, projectionrepo.ClaimNextPendingBatchParams{Target: params.Target, Limit: 1})
	if err != nil {
		return projectionrepo.Entry{}, false, err
	}
	if !ok {
		return projectionrepo.Entry{}, false, nil
	}
	return entries[0], true, nil
}

func (r *OutboxRepository) ClaimNextPendingBatch(ctx context.Context, params projectionrepo.ClaimNextPendingBatchParams) ([]projectionrepo.Entry, bool, error) {
	beginner, ok := r.db.(txBeginner)
	if !ok {
		return nil, false, fmt.Errorf("claim next pending requires transaction-capable database")
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin outbox claim transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	staleBefore := time.Now().UTC().Add(-defaultProcessingReclaimTimeout).Format(time.RFC3339Nano)
	entries, err := listReadyPendingEntries(ctx, tx, params.Target, now, staleBefore, params.Limit)
	if err != nil {
		return nil, false, fmt.Errorf("select pending outbox entries: %w", err)
	}
	if len(entries) == 0 {
		return nil, false, nil
	}

	claimedEntries := make([]projectionrepo.Entry, 0, len(entries))
	claimedEntries = append(claimedEntries, entries[0])
	if entries[0].Operation == "add" || entries[0].Operation == "delete" {
		for _, entry := range entries[1:] {
			if entry.Operation != entries[0].Operation {
				break
			}
			claimedEntries = append(claimedEntries, entry)
		}
	}

	for index, entry := range claimedEntries {
		result, err := tx.ExecContext(ctx, `
			UPDATE projection_outbox
			SET status = 'processing', updated_at = ?, completed_at = NULL
			WHERE id = ? AND (
				status = 'pending'
				OR (status = 'processing' AND updated_at <= ?)
			);
		`, now, entry.ID, staleBefore)
		if err != nil {
			return nil, false, fmt.Errorf("claim pending outbox entry %d: %w", entry.ID, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, false, fmt.Errorf("read claim rows affected for entry %d: %w", entry.ID, err)
		}
		if rowsAffected == 0 {
			return nil, false, nil
		}
		claimedEntries[index].Status = "processing"
		claimedEntries[index].UpdatedAt = now
		claimedEntries[index].CompletedAt = ""
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit outbox claim transaction: %w", err)
	}
	committed = true
	return claimedEntries, true, nil
}

func (r *OutboxRepository) MarkCompleted(ctx context.Context, params projectionrepo.MarkCompletedParams) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `
		UPDATE projection_outbox
		SET status = 'completed', updated_at = ?, completed_at = ?, last_error = ''
		WHERE id = ? AND status = 'processing';
	`, now, now, params.ID)
	if err != nil {
		return fmt.Errorf("mark outbox entry completed: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read completed rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("mark outbox entry completed: no processing row for id %d", params.ID)
	}
	return nil
}

func (r *OutboxRepository) MarkRetryableFailure(ctx context.Context, params projectionrepo.MarkRetryableFailureParams) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	nextRetryAt := params.NextRetryAt.UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `
		UPDATE projection_outbox
		SET status = 'pending', retry_count = retry_count + 1, next_retry_at = ?, last_error = ?, updated_at = ?, completed_at = NULL
		WHERE id = ? AND status = 'processing';
	`, nextRetryAt, params.LastError, now, params.ID)
	if err != nil {
		return fmt.Errorf("mark outbox entry retryable failure: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read retryable failure rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("mark outbox entry retryable failure: no processing row for id %d", params.ID)
	}
	return nil
}

func (r *OutboxRepository) MarkFailed(ctx context.Context, params projectionrepo.MarkFailedParams) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `
		UPDATE projection_outbox
		SET status = 'pending', retry_count = retry_count + 1, next_retry_at = NULL, last_error = ?, updated_at = ?, completed_at = NULL
		WHERE id = ? AND status = 'processing';
	`, params.LastError, now, params.ID)
	if err != nil {
		return fmt.Errorf("mark outbox entry failed: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read failed rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("mark outbox entry failed: no processing row for id %d", params.ID)
	}
	return nil
}

func (r *OutboxRepository) MarkDead(ctx context.Context, params projectionrepo.MarkDeadParams) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `
		UPDATE projection_outbox
		SET status = 'dead', retry_count = retry_count + 1, last_error = ?, updated_at = ?, completed_at = NULL
		WHERE id = ? AND status = 'processing';
	`, params.LastError, now, params.ID)
	if err != nil {
		return fmt.Errorf("mark outbox entry dead: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read dead rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("mark outbox entry dead: no processing row for id %d", params.ID)
	}
	return nil
}

func (r *OutboxRepository) Requeue(ctx context.Context, params projectionrepo.RequeueParams) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := r.db.ExecContext(ctx, `
		UPDATE projection_outbox
		SET status = 'pending', next_retry_at = NULL, last_error = '', updated_at = ?, completed_at = NULL
		WHERE id = ? AND (
			status = 'dead' OR (status = 'pending' AND COALESCE(last_error, '') <> '')
		);
	`, now, params.ID)
	if err != nil {
		return fmt.Errorf("requeue outbox entry: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read requeue rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("requeue outbox entry: no retryable row for id %d", params.ID)
	}
	return nil
}

func (r *OutboxRepository) Delete(ctx context.Context, params projectionrepo.DeleteParams) error {
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM projection_outbox
		WHERE id = ? AND status <> 'processing';
	`, params.ID)
	if err != nil {
		return fmt.Errorf("delete outbox entry: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read delete rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("delete outbox entry: no deletable row for id %d", params.ID)
	}
	return nil
}

func (r *OutboxRepository) PurgeCompleted(ctx context.Context, params projectionrepo.PurgeCompletedParams) (int64, error) {
	if params.Limit <= 0 {
		return 0, fmt.Errorf("purge completed outbox entries: limit must be positive")
	}
	result, err := r.db.ExecContext(ctx, `
		DELETE FROM projection_outbox
		WHERE id IN (
			SELECT id
			FROM projection_outbox
			WHERE target = ? AND status = 'completed'
			ORDER BY id ASC
			LIMIT ?
		);
	`, params.Target, params.Limit)
	if err != nil {
		return 0, fmt.Errorf("purge completed outbox entries: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read purge rows affected: %w", err)
	}
	return rowsAffected, nil
}
