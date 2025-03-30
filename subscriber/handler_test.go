package subscriber

import (
	"context"
	"log/slog"
	"testing"

	"github.com/bluesky-social/jetstream/pkg/models"
)

func TestHandlePostEvent(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.Default()
	fs, err := NewFeedService("", tmpDir, nil, nil, logger)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	h := &Handler{
		logger:      logger,
		FeedService: fs,
	}

	tests := []struct {
		name       string
		event      *models.Event
		wantErr    bool
		shouldAdd  bool
		operation  string
		collection string
		postText   string
		isReply    bool
	}{
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
		{
			name: "nil commit",
			event: &models.Event{
				Commit: nil,
			},
			wantErr: false,
		},
		{
			name: "non-post collection",
			event: &models.Event{
				Commit: &models.Commit{
					Collection: "app.bsky.feed.like",
				},
			},
			wantErr: false,
		},
		{
			name: "post collection",
			event: &models.Event{
				Commit: &models.Commit{
					Collection: "app.bsky.feed.post",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.HandlePostEvent(context.Background(), tt.event)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
