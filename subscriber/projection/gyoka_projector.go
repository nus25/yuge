package projection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bluesky-social/indigo/util"
	"github.com/nus25/yuge/feed/store/editor"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	"github.com/nus25/yuge/types"
)

var ErrUnsupportedProjectionOperation = errors.New("unsupported projection operation")
var ErrNonRetryableProjection = errors.New("non-retryable projection error")

type GyokaMutator interface {
	Add(params editor.PostParams) error
	Delete(params editor.DeleteParams) error
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
		if err := p.mutator.Add(editor.PostParams{
			FeedUri:   payload.FeedURI,
			Did:       parsedURI.Did,
			Rkey:      parsedURI.Rkey,
			Cid:       payload.Post.Cid,
			IndexedAt: indexedAt,
			Langs:     payload.Post.Langs,
		}); err != nil {
			var nonRetryableErr *editor.NonRetryableError
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
		if err := p.mutator.Delete(editor.DeleteParams{
			FeedUri: payload.FeedURI,
			Did:     parsedURI.Did,
			Rkey:    parsedURI.Rkey,
		}); err != nil {
			var nonRetryableErr *editor.NonRetryableError
			if errors.As(err, &nonRetryableErr) {
				return markNonRetryableProjection(fmt.Errorf("project delete entry: %w", err))
			}
			return fmt.Errorf("project delete entry: %w", err)
		}
		return nil
	default:
		return markNonRetryableProjection(fmt.Errorf("%w: %s", ErrUnsupportedProjectionOperation, entry.Operation))
	}
}