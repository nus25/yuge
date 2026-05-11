package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bluesky-social/indigo/util"
	"github.com/nus25/yuge/subscriber/projection/gyoka"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	"github.com/nus25/yuge/types"
)

var ErrUnsupportedProjectionOperation = errors.New("unsupported projection operation")
var ErrNonRetryableProjection = errors.New("non-retryable projection error")

type GyokaMutator interface {
	Add(params gyoka.PostParams) error
	BatchAdd(params gyoka.BatchPostParams) error
	Delete(params gyoka.DeleteParams) error
	Trim(params gyoka.TrimParams) error
}

type GyokaProjector struct {
	mutator GyokaMutator
}

func NewGyokaProjector(mutator GyokaMutator) *GyokaProjector {
	return &GyokaProjector{mutator: mutator}
}

type projectionPayload struct {
	FeedURI types.FeedUri `json:"feedUri"`
	Post    types.Post    `json:"post"`
	Count   int           `json:"count,omitempty"`
}

func markNonRetryableProjection(err error) error {
	return fmt.Errorf("%w: %w", ErrNonRetryableProjection, err)
}

func (p *GyokaProjector) Project(ctx context.Context, entry projectionrepo.Entry) error {
	_ = ctx
	if p == nil || p.mutator == nil {
		return markNonRetryableProjection(fmt.Errorf("gyoka mutator is required"))
	}
	var payload projectionPayload
	if err := json.Unmarshal([]byte(entry.PayloadJSON), &payload); err != nil {
		return markNonRetryableProjection(fmt.Errorf("decode projection payload: %w", err))
	}

	switch entry.Operation {
	case "add":
		parsedURI, err := util.ParseAtUri(string(payload.Post.Uri))
		if err != nil {
			return markNonRetryableProjection(fmt.Errorf("parse projected post uri: %w", err))
		}
		indexedAt, err := time.Parse(time.RFC3339Nano, payload.Post.IndexedAt)
		if err != nil {
			return markNonRetryableProjection(fmt.Errorf("parse projected indexed_at: %w", err))
		}
		if err := p.mutator.Add(gyoka.PostParams{
			FeedUri:   payload.FeedURI,
			Did:       parsedURI.Did,
			Rkey:      parsedURI.Rkey,
			Cid:       payload.Post.Cid,
			IndexedAt: indexedAt,
			Langs:     payload.Post.Langs,
		}); err != nil {
			var nonRetryableErr *gyoka.NonRetryableError
			if errors.As(err, &nonRetryableErr) {
				return markNonRetryableProjection(fmt.Errorf("project add entry: %w", err))
			}
			return fmt.Errorf("project add entry: %w", err)
		}
		return nil
	case "delete":
		parsedURI, err := util.ParseAtUri(string(payload.Post.Uri))
		if err != nil {
			return markNonRetryableProjection(fmt.Errorf("parse projected post uri: %w", err))
		}
		if err := p.mutator.Delete(gyoka.DeleteParams{
			FeedUri: payload.FeedURI,
			Did:     parsedURI.Did,
			Rkey:    parsedURI.Rkey,
		}); err != nil {
			var nonRetryableErr *gyoka.NonRetryableError
			if errors.As(err, &nonRetryableErr) {
				return markNonRetryableProjection(fmt.Errorf("project delete entry: %w", err))
			}
			return fmt.Errorf("project delete entry: %w", err)
		}
		return nil
	case "trim":
		if err := p.mutator.Trim(gyoka.TrimParams{
			FeedUri: payload.FeedURI,
			Count:   payload.Count,
		}); err != nil {
			var nonRetryableErr *gyoka.NonRetryableError
			if errors.As(err, &nonRetryableErr) {
				return markNonRetryableProjection(fmt.Errorf("project trim entry: %w", err))
			}
			return fmt.Errorf("project trim entry: %w", err)
		}
		return nil
	default:
		return markNonRetryableProjection(fmt.Errorf("%w: %s", ErrUnsupportedProjectionOperation, entry.Operation))
	}
}

func (p *GyokaProjector) ProjectBatch(ctx context.Context, entries []projectionrepo.Entry) error {
	_ = ctx
	if p == nil || p.mutator == nil {
		return markNonRetryableProjection(fmt.Errorf("gyoka mutator is required"))
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) == 1 {
		return p.Project(ctx, entries[0])
	}

	batchEntries := make([]gyoka.PostParams, 0, len(entries))
	for _, entry := range entries {
		if entry.Operation != "add" {
			return markNonRetryableProjection(fmt.Errorf("%w: %s", ErrUnsupportedProjectionOperation, entry.Operation))
		}
		var payload projectionPayload
		if err := json.Unmarshal([]byte(entry.PayloadJSON), &payload); err != nil {
			return markNonRetryableProjection(fmt.Errorf("decode projection payload: %w", err))
		}
		parsedURI, err := util.ParseAtUri(string(payload.Post.Uri))
		if err != nil {
			return markNonRetryableProjection(fmt.Errorf("parse projected post uri: %w", err))
		}
		indexedAt, err := time.Parse(time.RFC3339Nano, payload.Post.IndexedAt)
		if err != nil {
			return markNonRetryableProjection(fmt.Errorf("parse projected indexed_at: %w", err))
		}
		batchEntries = append(batchEntries, gyoka.PostParams{
			FeedUri:   payload.FeedURI,
			Did:       parsedURI.Did,
			Rkey:      parsedURI.Rkey,
			Cid:       payload.Post.Cid,
			IndexedAt: indexedAt,
			Langs:     payload.Post.Langs,
		})
	}
	if err := p.mutator.BatchAdd(gyoka.BatchPostParams{Entries: batchEntries}); err != nil {
		return fmt.Errorf("project add batch: %w", err)
	}
	return nil
}
