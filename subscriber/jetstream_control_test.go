package subscriber

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRuntimeJetstreamController_ConnectWarnsOnInvalidCursor(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ctrl := NewRuntimeJetstreamController(logger, nil, "ws://localhost:6008/subscribe", 123)
	invalidCursor := int64(-1)

	_, _ = ctrl.Connect(JetstreamConnectRequest{Cursor: &invalidCursor})

	logOutput := buf.String()
	if !strings.Contains(logOutput, "invalid cursor") {
		t.Fatalf("expected warn log containing 'invalid cursor', got: %s", logOutput)
	}
}

func TestRuntimeJetstreamController_ConnectKeepsRequestedCursorOnReconnect(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctrl := NewRuntimeJetstreamController(logger, nil, "ws://localhost:6008/subscribe", 111)

	done := make(chan struct{})
	ctrl.done = done
	ctrl.cancel = func() {
		ctrl.cursor = 999 // simulate old loop writing back previous cursor on shutdown
		close(done)
	}

	requested := int64(123456)
	_, _ = ctrl.Connect(JetstreamConnectRequest{Cursor: &requested})

	ctrl.mu.Lock()
	actual := ctrl.cursor
	ctrl.mu.Unlock()

	if actual != requested {
		t.Fatalf("expected cursor %d after reconnect request, got %d", requested, actual)
	}
}
