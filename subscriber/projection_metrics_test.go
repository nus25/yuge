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

	projectionOutboxEntries.WithLabelValues(target, "pending", "01").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "processing", "02").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "completed", "03").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "dead", "04").Set(99)
	projectionOutboxEntries.WithLabelValues(target, "failed", "05").Set(99)

	if err := collectProjectionOutboxMetrics(ctx, repo, target); err != nil {
		t.Fatalf("collectProjectionOutboxMetrics() error = %v", err)
	}

	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "pending", "01")); got != 2 {
		t.Fatalf("pending gauge = %v, want 2", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "processing", "02")); got != 0 {
		t.Fatalf("processing gauge = %v, want 0", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "completed", "03")); got != 1 {
		t.Fatalf("completed gauge = %v, want 1", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "dead", "04")); got != 1 {
		t.Fatalf("dead gauge = %v, want 1", got)
	}
	if got := gaugeValue(t, projectionOutboxEntries.WithLabelValues(target, "failed", "05")); got != 1 {
		t.Fatalf("failed gauge = %v, want 1", got)
	}
}

func TestProjectionOutboxEntries_ExposesStatusOrderLabel(t *testing.T) {
	testCases := []struct {
		status string
		order  string
	}{
		{status: "pending", order: "01"},
		{status: "processing", order: "02"},
		{status: "completed", order: "03"},
		{status: "dead", order: "04"},
		{status: "failed", order: "05"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.status, func(t *testing.T) {
			metric := &dto.Metric{}
			if err := projectionOutboxEntries.WithLabelValues("status-order-test", testCase.status, testCase.order).Write(metric); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			for _, label := range metric.GetLabel() {
				if label.GetName() == "status_order" && label.GetValue() == testCase.order {
					return
				}
			}
			t.Errorf("status_order label = %q, want %q", metric.GetLabel(), testCase.order)
		})
	}
}

func TestPurgeExpiredProjectionOps_UsesDefaultSevenDayRetention(t *testing.T) {
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
	target := "purge-test"
	for index := 0; index < 2; index++ {
		if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
			FeedID:      "feed-completed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
			Target:      target,
			Operation:   "add",
			MutationID:  "m-purge-" + string(rune('a'+index)),
			SubjectKey:  "subject-purge-" + string(rune('a'+index)),
			OpKey:       "op-purge-" + string(rune('a'+index)),
			PayloadJSON: `{}`,
			Status:      "pending",
		}); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		entry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: target})
		if err != nil {
			t.Fatalf("ClaimNextPending() error = %v", err)
		}
		if !ok {
			t.Fatal("ClaimNextPending() ok = false, want true")
		}
		if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: entry.ID}); err != nil {
			t.Fatalf("MarkCompleted() error = %v", err)
		}
	}

	if _, err := persistence.mutationDB.ExecContext(ctx, `UPDATE projection_outbox SET completed_at = ? WHERE id = 1;`, time.Now().UTC().Add(-8*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("set completed_at: %v", err)
	}

	deletedCount, err := purgeExpiredProjectionOps(ctx, repo, target, time.Now().UTC(), projectionCompletedRetention(gyokaProjectionRuntimeOptions{}))
	if err != nil {
		t.Fatalf("purgeExpiredProjectionOps() error = %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("deletedCount = %d, want 1", deletedCount)
	}
	entries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: target, Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("completed entries len = %d, want 1", len(entries))
	}
}

func TestProjectionCompletedRetention(t *testing.T) {
	testCases := []struct {
		name string
		opts gyokaProjectionRuntimeOptions
		want time.Duration
	}{
		{
			name: "default",
			want: 7 * 24 * time.Hour,
		},
		{
			name: "configured",
			opts: gyokaProjectionRuntimeOptions{
				completedRetention: 48 * time.Hour,
			},
			want: 48 * time.Hour,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := projectionCompletedRetention(testCase.opts); got != testCase.want {
				t.Fatalf("projectionCompletedRetention() = %s, want %s", got, testCase.want)
			}
		})
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
