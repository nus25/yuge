package subscriber

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/nus25/yuge/feed/store/editor"
	"github.com/nus25/yuge/subscriber/projection"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
)

const defaultProjectionPollInterval = 250 * time.Millisecond

type gyokaProjectionRuntimeOptions struct {
	pollInterval time.Duration
	clientOptions []editor.ClientOptionFunc
}

type gyokaProjectionRuntime struct {
	cancel context.CancelFunc
	done   chan struct{}
	editor *editor.GyokaEditor
}

func startGyokaProjectionRuntime(parentCtx context.Context, logger *slog.Logger, db *sql.DB, endpoint string, opts gyokaProjectionRuntimeOptions) (*gyokaProjectionRuntime, error) {
	if endpoint == "" {
		return nil, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	pollInterval := opts.pollInterval
	if pollInterval <= 0 {
		pollInterval = defaultProjectionPollInterval
	}
	gyokaEditor, err := editor.NewGyokaEditor(endpoint, logger, opts.clientOptions...)
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
		for {
			select {
			case <-runCtx.Done():
				return
			default:
			}

			result, err := service.ProcessNextPendingStep(runCtx)
			if err != nil {
				switch result.Outcome {
				case projection.ProcessOutcomeDead:
					projectionOutboxErrors.WithLabelValues("gyoka", "non_retryable").Inc()
					projectionOutboxDead.WithLabelValues("gyoka").Inc()
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