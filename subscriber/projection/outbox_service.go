package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
)

const defaultRetryDelay = time.Second

type EntryProjector interface {
	Project(ctx context.Context, entry projectionrepo.Entry) error
}

type ProcessOutcome string

const (
	ProcessOutcomeNone      ProcessOutcome = "none"
	ProcessOutcomeCompleted ProcessOutcome = "completed"
	ProcessOutcomeRetried   ProcessOutcome = "retried"
	ProcessOutcomeDead      ProcessOutcome = "dead"
)

type ProcessResult struct {
	Processed bool
	Outcome   ProcessOutcome
}

type OutboxService struct {
	target    string
	repo      projectionrepo.OutboxRepository
	projector EntryProjector
}

func NewOutboxService(target string, repo projectionrepo.OutboxRepository, projector EntryProjector) *OutboxService {
	return &OutboxService{
		target:    target,
		repo:      repo,
		projector: projector,
	}
}

func (s *OutboxService) ProcessNextPending(ctx context.Context) (bool, error) {
	result, err := s.ProcessNextPendingStep(ctx)
	return result.Processed, err
}

func (s *OutboxService) ProcessNextPendingStep(ctx context.Context) (ProcessResult, error) {
	if s == nil || s.repo == nil {
		return ProcessResult{}, fmt.Errorf("outbox repository is required")
	}
	if s.projector == nil {
		return ProcessResult{}, fmt.Errorf("entry projector is required")
	}
	entry, ok, err := s.repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: s.target})
	if err != nil {
		return ProcessResult{}, fmt.Errorf("claim next pending outbox entry: %w", err)
	}
	if !ok {
		return ProcessResult{Processed: false, Outcome: ProcessOutcomeNone}, nil
	}
	if err := s.projector.Project(ctx, entry); err != nil {
		if errors.Is(err, ErrNonRetryableProjection) {
			if markErr := s.repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: entry.ID, LastError: err.Error()}); markErr != nil {
				return ProcessResult{Processed: true, Outcome: ProcessOutcomeNone}, fmt.Errorf("mark outbox entry %d dead: %w", entry.ID, markErr)
			}
			return ProcessResult{Processed: true, Outcome: ProcessOutcomeDead}, fmt.Errorf("project outbox entry %d: %w", entry.ID, err)
		}
		if markErr := s.repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
			ID:          entry.ID,
			LastError:   err.Error(),
			NextRetryAt: time.Now().UTC().Add(defaultRetryDelay),
		}); markErr != nil {
			return ProcessResult{Processed: true, Outcome: ProcessOutcomeNone}, fmt.Errorf("mark outbox entry %d retryable failure: %w", entry.ID, markErr)
		}
		return ProcessResult{Processed: true, Outcome: ProcessOutcomeRetried}, fmt.Errorf("project outbox entry %d: %w", entry.ID, err)
	}
	if err := s.repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: entry.ID}); err != nil {
		return ProcessResult{Processed: true, Outcome: ProcessOutcomeNone}, fmt.Errorf("mark outbox entry %d completed: %w", entry.ID, err)
	}
	return ProcessResult{Processed: true, Outcome: ProcessOutcomeCompleted}, nil
}