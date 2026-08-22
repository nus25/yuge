package subscriber

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nus25/yuge/subscriber/projection"
	"github.com/nus25/yuge/subscriber/projection/gyoka"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
)

const defaultProjectionPollInterval = 250 * time.Millisecond
const defaultProjectionMetricsInterval = 30 * time.Second

type gyokaProjectionRuntimeOptions struct {
	pollInterval    time.Duration
	metricsInterval time.Duration
	clientOptions   []gyoka.ClientOptionFunc
}

type gyokaProjectionRuntime struct {
	cancel context.CancelFunc
	done   chan struct{}
	editor *gyoka.GyokaEditor
}

func startGyokaProjectionRuntime(parentCtx context.Context, logger *slog.Logger, db *sql.DB, config gyoka.ClientConfig, opts gyokaProjectionRuntimeOptions) (*gyokaProjectionRuntime, error) {
	if config.Host == "" {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	pollInterval := opts.pollInterval
	if pollInterval <= 0 {
		pollInterval = defaultProjectionPollInterval
	}
	metricsInterval := opts.metricsInterval
	if metricsInterval <= 0 {
		metricsInterval = defaultProjectionMetricsInterval
	}
	gyokaEditor, err := gyoka.NewGyokaEditor(parentCtx, config, logger, opts.clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("create gyoka editor: %w", err)
	}
	openCtx, cancelOpen := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancelOpen()
	if err := gyokaEditor.Open(openCtx); err != nil {
		return nil, fmt.Errorf("open gyoka editor: %w", err)
	}

	runCtx, cancel := context.WithCancel(parentCtx)
	done := make(chan struct{})
	repo := projectionsqlite.NewOutboxRepository(db)
	service := projection.NewOutboxService("gyoka", repo, projection.NewGyokaProjector(gyokaEditor))
	go func() {
		defer close(done)
		nextMetricsCollection := time.Time{}
		for {
			select {
			case <-runCtx.Done():
				return
			default:
			}

			now := time.Now()
			if nextMetricsCollection.IsZero() || !now.Before(nextMetricsCollection) {
				if err := collectProjectionOutboxMetrics(runCtx, repo, "gyoka"); err != nil && !errors.Is(err, context.Canceled) {
					logger.Warn("failed to collect projection outbox metrics", "target", "gyoka", "error", err)
				}
				nextMetricsCollection = now.Add(metricsInterval)
			}

			result, err := service.ProcessNextPendingStep(runCtx)
			if err != nil {
				switch result.Outcome {
				case projection.ProcessOutcomeDead:
					projectionOutboxErrors.WithLabelValues("gyoka", "non_retryable").Inc()
					projectionOutboxDead.WithLabelValues("gyoka").Inc()
				case projection.ProcessOutcomeFailed:
					projectionOutboxErrors.WithLabelValues("gyoka", "manual").Inc()
				case projection.ProcessOutcomeRetried:
					projectionOutboxErrors.WithLabelValues("gyoka", "retryable").Inc()
					projectionOutboxRetried.WithLabelValues("gyoka").Inc()
				default:
					projectionOutboxErrors.WithLabelValues("gyoka", "internal").Inc()
				}
				logger.Warn("gyoka projection step failed", "error", err)
			} else if result.Outcome == projection.ProcessOutcomeCompleted {
				projectionOutboxCompleted.WithLabelValues("gyoka").Inc()
			}
			if result.Processed {
				continue
			}

			timer := time.NewTimer(pollInterval)
			select {
			case <-runCtx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
			}
		}
	}()

	return &gyokaProjectionRuntime{cancel: cancel, done: done, editor: gyokaEditor}, nil
}

func collectProjectionOutboxMetrics(ctx context.Context, repo projectionrepo.OutboxRepository, target string) error {
	if repo == nil {
		return fmt.Errorf("outbox repository is required")
	}
	counts, err := repo.CountByStatus(ctx, projectionrepo.CountByStatusParams{Target: target})
	if err != nil {
		return fmt.Errorf("count outbox entries by status: %w", err)
	}
	for _, status := range projectionOutboxStatuses {
		projectionOutboxEntries.WithLabelValues(target, status).Set(0)
	}
	for _, count := range counts {
		projectionOutboxEntries.WithLabelValues(target, count.Status).Set(float64(count.Count))
	}
	return nil
}

func (r *gyokaProjectionRuntime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	if r.done != nil {
		select {
		case <-r.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if r.editor != nil {
		if err := r.editor.Close(ctx); err != nil {
			return fmt.Errorf("close gyoka editor: %w", err)
		}
	}
	return nil
}
