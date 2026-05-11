package subscriber

import (
	"context"
	"testing"
	"time"

	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	dto "github.com/prometheus/client_model/go"
)

func TestCollectProjectionOutboxMetrics_UpdatesGaugeByStatus(t *testing.T) {
	ctx := context.Background()
	persistence, err := openSQLiteRuntimePersistence(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	t.Cleanup(func() {
		if err := persistence.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	repo := projectionsqlite.NewOutboxRepository(persistence.mutationDB)
	target := "metrics-test"
	entries := []projectionrepo.EnqueueParams{
		{
			FeedID:      "feed-pending",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/pending",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-pending",
			SubjectKey:  "subject-pending",
			OpKey:       "op-pending",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-ready",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/ready",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-ready",
			SubjectKey:  "subject-ready",
			OpKey:       "op-ready",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-completed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-completed",
			SubjectKey:  "subject-completed",
			OpKey:       "op-completed",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-dead",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
			Target:      target,
			Operation:   "delete",
			MutationID:  "m-dead",
			SubjectKey:  "subject-dead",
			OpKey:       "op-dead",
			PayloadJSON: `{}`,
			Status:      "pending",
		},
	}
	for _, entry := range entries {
		if err := repo.Enqueue(ctx, entry); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", entry.OpKey, err)
		}
	}

	failedEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
	if err != nil {
		t.Fatalf("ClaimNextPending() first error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() first ok = false, want true")
	}
	if err := repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
		ID:          failedEntry.ID,
		LastError:   "boom",
		NextRetryAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkRetryableFailure() error = %v", err)
	}

	completedEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
	if err != nil {
		t.Fatalf("ClaimNextPending() second error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() second ok = false, want true")
	}
	if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: completedEntry.ID}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	deadEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
	if err != nil {
		t.Fatalf("ClaimNextPending() third error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() third ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: deadEntry.ID, LastError: "boom"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	projectionOutboxEntries.WithLabelValues(target, "pending").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "processing").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "completed").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "dead").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "failed").Set(99)

	if err := collectProjectionOutboxMetrics(ctx, repo, target); err != nil {
		t.Fatalf("collectProjectionOutboxMetrics() error = %v", err)
	}

	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "pending")); got != 2 {
		t.Fatalf("pending gauge = %v, want 2", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "processing")); got != 0 {
		t.Fatalf("processing gauge = %v, want 0", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "completed")); got != 1 {
		t.Fatalf("completed gauge = %v, want 1", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "dead")); got != 1 {
		t.Fatalf("dead gauge = %v, want 1", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "failed")); got != 1 {
		t.Fatalf("failed gauge = %v, want 1", got)
	}
}

func gaugeValue(t *testing.T, gauge interface{ Write(*dto.Metric) error }) float64 {
	t.Helper()
	metric := &dto.Metric{}
	if err := gauge.Write(metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if metric.Gauge == nil || metric.Gauge.Value == nil {
		t.Fatal("metric gauge value is nil")
	}
	return metric.GetGauge().GetValue()
}
