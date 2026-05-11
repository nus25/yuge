package projection

import (
	"context"
	"errors"
	"fmt"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
)

const defaultRetryDelay = time.Second
const defaultBatchClaimLimit = 25

type EntryProjector interface {
	Project(ctx context.Context, entry projectionrepo.Entry) error
}

type BatchEntryProjector interface {
	ProjectBatch(ctx context.Context, entries []projectionrepo.Entry) error
}

type ProcessOutcome string

const (
	ProcessOutcomeNone      ProcessOutcome = "none"
	ProcessOutcomeCompleted ProcessOutcome = "completed"
	ProcessOutcomeRetried   ProcessOutcome = "retried"
	ProcessOutcomeDead      ProcessOutcome = "dead"
	ProcessOutcomeFailed    ProcessOutcome = "failed"
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
	if batchProjector, ok := s.projector.(BatchEntryProjector); ok {
		entries, claimed, err := s.repo.ClaimNextPendingBatch(ctx, projectionrepo.ClaimNextPendingBatchParams{Target: s.target, Limit: defaultBatchClaimLimit})
		if err != nil {
			return ProcessResult{}, fmt.Errorf("claim next pending outbox batch: %w", err)
		}
		if !claimed {
			return ProcessResult{Processed: false, Outcome: ProcessOutcomeNone}, nil
		}
		if len(entries) == 1 {
			return s.processSingleEntry(ctx, entries[0])
		}
		if err := batchProjector.ProjectBatch(ctx, entries); err != nil {
			if markErr := s.markFailedEntries(ctx, entries, err.Error()); markErr != nil {
				return ProcessResult{Processed: true, Outcome: ProcessOutcomeNone}, fmt.Errorf("mark outbox batch failed: %w", markErr)
			}
			return ProcessResult{Processed: true, Outcome: ProcessOutcomeFailed}, fmt.Errorf("project outbox batch starting at %d: %w", entries[0].ID, err)
		}
		if err := s.markCompletedEntries(ctx, entries); err != nil {
			return ProcessResult{Processed: true, Outcome: ProcessOutcomeNone}, fmt.Errorf("mark outbox batch completed: %w", err)
		}
		return ProcessResult{Processed: true, Outcome: ProcessOutcomeCompleted}, nil
	}
	return s.processNextSingleEntry(ctx)
}

func (s *OutboxService) processNextSingleEntry(ctx context.Context) (ProcessResult, error) {
	entry, ok, err := s.repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: s.target})
	if err != nil {
		return ProcessResult{}, fmt.Errorf("claim next pending outbox entry: %w", err)
	}
	if !ok {
		return ProcessResult{Processed: false, Outcome: ProcessOutcomeNone}, nil
	}
	return s.processSingleEntry(ctx, entry)
}

func (s *OutboxService) processSingleEntry(ctx context.Context, entry projectionrepo.Entry) (ProcessResult, error) {
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

func (s *OutboxService) markCompletedEntries(ctx context.Context, entries []projectionrepo.Entry) error {
	for _, entry := range entries {
		if err := s.repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: entry.ID}); err != nil {
			return fmt.Errorf("mark outbox entry %d completed: %w", entry.ID, err)
		}
	}
	return nil
}

func (s *OutboxService) markFailedEntries(ctx context.Context, entries []projectionrepo.Entry, lastError string) error {
	for _, entry := range entries {
		if err := s.repo.MarkFailed(ctx, projectionrepo.MarkFailedParams{ID: entry.ID, LastError: lastError}); err != nil {
			return fmt.Errorf("mark outbox entry %d failed: %w", entry.ID, err)
		}
	}
	return nil
}
