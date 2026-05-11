package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bluesky-social/indigo/util"
	storerepo "github.com/nus25/yuge/feed/store/repository"
	"github.com/nus25/yuge/types"
)

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type FeedRepository struct {
	db dbtx
}

func NewFeedRepository(db dbtx) *FeedRepository {
	return &FeedRepository{db: db}
}

func (r *FeedRepository) PutPost(ctx context.Context, params storerepo.PutPostParams) error {
	post := params.Post
	p, err := util.ParseAtUri(string(post.Uri))
	if err != nil {
		return fmt.Errorf("parse post uri: %w", err)
	}
	langsJSON, err := json.Marshal(post.Langs)
	if err != nil {
		return fmt.Errorf("marshal langs: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO feed_posts (
			feed_id, feed_uri, post_uri, did, rkey, cid, indexed_at, langs_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_id, post_uri) DO UPDATE SET
			feed_uri = excluded.feed_uri,
			did = excluded.did,
			rkey = excluded.rkey,
			cid = excluded.cid,
			indexed_at = excluded.indexed_at,
			langs_json = excluded.langs_json,
			updated_at = excluded.updated_at;
	`, params.FeedID, post.Feed, post.Uri, p.Did, p.Rkey, post.Cid, post.IndexedAt, string(langsJSON), now, now)
	if err != nil {
		return fmt.Errorf("put post: %w", err)
	}
	return nil
}

func (r *FeedRepository) ListPosts(ctx context.Context, params storerepo.ListPostsParams) ([]types.Post, error) {
	query := `
		SELECT feed_uri, post_uri, cid, indexed_at, langs_json
		FROM feed_posts
		WHERE feed_id = ?
		ORDER BY indexed_at DESC`
	args := []any{params.FeedID}
	if params.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, params.Limit)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	posts := make([]types.Post, 0)
	for rows.Next() {
		var (
			feedURI   string
			postURI   string
			cid       string
			indexedAt string
			langsJSON string
		)
		if err := rows.Scan(&feedURI, &postURI, &cid, &indexedAt, &langsJSON); err != nil {
			return nil, fmt.Errorf("scan post row: %w", err)
		}
		var langs []string
		if langsJSON != "" {
			if err := json.Unmarshal([]byte(langsJSON), &langs); err != nil {
				return nil, fmt.Errorf("unmarshal langs: %w", err)
			}
		}
		posts = append(posts, types.Post{
			Feed:      types.FeedUri(feedURI),
			Uri:       types.PostUri(postURI),
			Cid:       cid,
			IndexedAt: indexedAt,
			Langs:     langs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate post rows: %w", err)
	}
	return posts, nil
}

func (r *FeedRepository) DeletePost(ctx context.Context, params storerepo.DeletePostParams) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM feed_posts WHERE feed_id = ? AND post_uri = ?;`, params.FeedID, params.PostURI)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	return nil
}

func (r *FeedRepository) TrimOverflow(ctx context.Context, params storerepo.TrimOverflowParams) ([]types.Post, error) {
	if params.TrimAt <= 0 {
		return nil, nil
	}

	var postCount int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feed_posts WHERE feed_id = ?;`, params.FeedID).Scan(&postCount); err != nil {
		return nil, fmt.Errorf("count feed posts: %w", err)
	}
	if postCount <= params.TrimAt {
		return nil, nil
	}

	remain := params.Remain
	if remain < 0 {
		remain = 0
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT feed_uri, post_uri, cid, indexed_at, langs_json
		FROM feed_posts
		WHERE feed_id = ?
		ORDER BY indexed_at DESC
		LIMIT -1 OFFSET ?;
	`, params.FeedID, remain)
	if err != nil {
		return nil, fmt.Errorf("list overflow posts: %w", err)
	}
	defer rows.Close()

	trimmedPosts := make([]types.Post, 0)
	for rows.Next() {
		var (
			feedURI   string
			postURI   string
			cid       string
			indexedAt string
			langsJSON string
		)
		if err := rows.Scan(&feedURI, &postURI, &cid, &indexedAt, &langsJSON); err != nil {
			return nil, fmt.Errorf("scan overflow row: %w", err)
		}
		var langs []string
		if langsJSON != "" {
			if err := json.Unmarshal([]byte(langsJSON), &langs); err != nil {
				return nil, fmt.Errorf("unmarshal overflow langs: %w", err)
			}
		}
		trimmedPosts = append(trimmedPosts, types.Post{
			Feed:      types.FeedUri(feedURI),
			Uri:       types.PostUri(postURI),
			Cid:       cid,
			IndexedAt: indexedAt,
			Langs:     langs,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate overflow rows: %w", err)
	}

	for _, post := range trimmedPosts {
		if err := r.DeletePost(ctx, storerepo.DeletePostParams{FeedID: params.FeedID, PostURI: post.Uri}); err != nil {
			return nil, fmt.Errorf("delete overflow post %s: %w", post.Uri, err)
		}
	}
	return trimmedPosts, nil
}

func (r *FeedRepository) PutFeedState(ctx context.Context, params storerepo.PutFeedStateParams) error {
	state := params.State
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO feed_state (feed_id, feed_uri, status, config_revision, last_loaded_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_id) DO UPDATE SET
			feed_uri = excluded.feed_uri,
			status = excluded.status,
			config_revision = excluded.config_revision,
			last_loaded_at = excluded.last_loaded_at,
			updated_at = excluded.updated_at;
	`, state.FeedID, state.FeedURI, state.Status, state.ConfigRevision, state.LastLoadedAt, state.UpdatedAt)
	if err != nil {
		return fmt.Errorf("put feed state: %w", err)
	}
	return nil
}

func (r *FeedRepository) GetFeedState(ctx context.Context, feedID string) (storerepo.FeedState, bool, error) {
	var state storerepo.FeedState
	err := r.db.QueryRowContext(ctx, `
		SELECT feed_id, feed_uri, status, config_revision, last_loaded_at, updated_at
		FROM feed_state WHERE feed_id = ?;
	`, feedID).Scan(&state.FeedID, &state.FeedURI, &state.Status, &state.ConfigRevision, &state.LastLoadedAt, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return storerepo.FeedState{}, false, nil
	}
	if err != nil {
		return storerepo.FeedState{}, false, fmt.Errorf("get feed state: %w", err)
	}
	return state, true, nil
}
