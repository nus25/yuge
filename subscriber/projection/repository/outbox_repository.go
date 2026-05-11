package repository

import (
	"context"
	"time"
)

type Entry struct {
	ID          int64
	FeedID      string
	FeedURI     string
	Target      string
	Operation   string
	MutationID  string
	SubjectKey  string
	OpKey       string
	PayloadJSON string
	Status      string
	RetryCount  int
	NextRetryAt string
	LastError   string
	CreatedAt   string
	UpdatedAt   string
	CompletedAt string
}

type EnqueueParams struct {
	FeedID      string
	FeedURI     string
	Target      string
	Operation   string
	MutationID  string
	SubjectKey  string
	OpKey       string
	PayloadJSON string
	Status      string
}

type ListByStatusParams struct {
	Target string
	Status string
	Limit  int
}

type ClaimNextPendingParams struct {
	Target string
}

type MarkCompletedParams struct {
	ID int64
}

type MarkRetryableFailureParams struct {
	ID          int64
	LastError   string
	NextRetryAt time.Time
}

type MarkDeadParams struct {
	ID        int64
	LastError string
}

type RequeueParams struct {
	ID int64
}

type DeleteParams struct {
	ID int64
}

type PurgeCompletedParams struct {
	Target string
	Limit  int
}

type CountByStatusParams struct {
	Target string
}

type StatusCount struct {
	Status string
	Count  int64
}

type OutboxRepository interface {
	Enqueue(ctx context.Context, params EnqueueParams) error
	ListByStatus(ctx context.Context, params ListByStatusParams) ([]Entry, error)
	CountByStatus(ctx context.Context, params CountByStatusParams) ([]StatusCount, error)
	ClaimNextPending(ctx context.Context, params ClaimNextPendingParams) (Entry, bool, error)
	MarkCompleted(ctx context.Context, params MarkCompletedParams) error
	MarkRetryableFailure(ctx context.Context, params MarkRetryableFailureParams) error
	MarkDead(ctx context.Context, params MarkDeadParams) error
	Requeue(ctx context.Context, params RequeueParams) error
	Delete(ctx context.Context, params DeleteParams) error
	PurgeCompleted(ctx context.Context, params PurgeCompletedParams) (int64, error)
}
