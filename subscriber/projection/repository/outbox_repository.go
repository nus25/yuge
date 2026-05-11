package repository

import "context"

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

type OutboxRepository interface {
	Enqueue(ctx context.Context, params EnqueueParams) error
	ListByStatus(ctx context.Context, params ListByStatusParams) ([]Entry, error)
}
