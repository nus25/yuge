package subscriber

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/jetstream/pkg/models"
	"github.com/gorilla/websocket"
	jetstreamClient "github.com/nus25/yuge/subscriber/pkg/client"
)

func TestJetstreamCursorStore(t *testing.T) {
	store := newJetstreamCursorStore(t.TempDir())
	cursor, err := store.Load()
	if err != nil || cursor != -1 {
		t.Fatalf("missing cursor: got %d, %v", cursor, err)
	}
	if err := store.Save(123456); err != nil {
		t.Fatalf("save cursor: %v", err)
	}
	cursor, err = store.Load()
	if err != nil || cursor != 123456 {
		t.Fatalf("saved cursor: got %d, %v", cursor, err)
	}
	if err := store.Save(-1); err != nil {
		t.Fatalf("reset cursor: %v", err)
	}
	cursor, err = store.Load()
	if err != nil || cursor != -1 {
		t.Fatalf("reset cursor: got %d, %v", cursor, err)
	}
}

func TestJetstreamCursorStoreRejectsInvalidFiles(t *testing.T) {
	for _, content := range []string{"not json", `{}`, `{"cursor":0}`, `{"cursor":-1}`, `{"cursor":123} trailing`} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "jetstream-cursor.json"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := newJetstreamCursorStore(dir).Load(); err == nil {
				t.Fatalf("expected invalid cursor error for %q", content)
			}
		})
	}
}

func TestJetstreamCursorStoreErrors(t *testing.T) {
	store := newJetstreamCursorStore(filepath.Join(t.TempDir(), "missing"))
	if err := store.Save(123); err == nil {
		t.Fatal("expected save to fail when directory is missing")
	}
	if _, err := initialJetstreamCursor(store, cursorPointer(123)); err == nil {
		t.Fatal("expected override save failure")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "jetstream-cursor.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := newJetstreamCursorStore(dir).Load(); err == nil {
		t.Fatal("expected read error")
	}
}

func TestInitialJetstreamCursor(t *testing.T) {
	store := newJetstreamCursorStore(t.TempDir())
	if err := store.Save(123); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		override *int64
		want     int64
	}{
		{name: "saved", want: 123},
		{name: "override", override: cursorPointer(456), want: 456},
		{name: "live", override: cursorPointer(-1), want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			cursor, err := initialJetstreamCursor(store, test.override)
			if err != nil || cursor != test.want {
				t.Fatalf("got %d, %v; want %d", cursor, err, test.want)
			}
		})
	}
}

func cursorPointer(cursor int64) *int64 { return &cursor }

type cursorTestScheduler struct{}

func (cursorTestScheduler) AddWork(context.Context, string, *models.Event) error { return nil }
func (cursorTestScheduler) Shutdown()                                            {}

type cursorCloseTestScheduler struct {
	shutdown bool
}

func (scheduler *cursorCloseTestScheduler) AddWork(context.Context, string, *models.Event) error {
	return nil
}

func (scheduler *cursorCloseTestScheduler) Shutdown() { scheduler.shutdown = true }

func TestJetstreamServerCloseKeepsSchedulerAvailable(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	}))
	defer server.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := jetstreamClient.DefaultClientConfig()
	config.Compress = false
	config.WebsocketURL = "ws" + strings.TrimPrefix(server.URL, "http")
	scheduler := &cursorCloseTestScheduler{}
	client, err := jetstreamClient.NewClient(config, logger, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	_ = client.ConnectAndRead(context.Background(), -1)
	if scheduler.shutdown {
		t.Fatal("server close stopped the scheduler needed for reconnect")
	}
}

func TestJetstreamCursorPersistenceAndReconnect(t *testing.T) {
	requests := make(chan string, 4)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		requests <- request.URL.Query().Get("cursor")
		if err := connection.WriteJSON(map[string]interface{}{"time_us": 456}); err != nil {
			return
		}
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := jetstreamClient.DefaultClientConfig()
	config.Compress = false
	config.WebsocketURL = "ws" + strings.TrimPrefix(server.URL, "http")
	client, err := jetstreamClient.NewClient(config, logger, cursorTestScheduler{})
	if err != nil {
		t.Fatal(err)
	}
	store := newJetstreamCursorStore(t.TempDir())
	controller := NewRuntimeJetstreamController(logger, &Handler{logger: logger, Jsc: client}, config.WebsocketURL, 123)
	controller.cursorStore = store
	controller.cursorSaveInterval = 10 * time.Millisecond
	defer func() { _, _ = controller.Disconnect() }()
	if _, err := controller.Connect(JetstreamConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := <-requests; got != "123" {
		t.Fatalf("initial cursor = %q", got)
	}
	deadline := time.After(2 * time.Second)
	for {
		cursor, err := store.Load()
		if err == nil && cursor == 456 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("cursor not saved: %d, %v", cursor, err)
		case <-time.After(5 * time.Millisecond):
		}
	}
	if _, err := controller.Disconnect(); err != nil {
		t.Fatal(err)
	}
	cursor, err := initialJetstreamCursor(store, nil)
	if err != nil || cursor != 456 {
		t.Fatalf("restart cursor = %d, %v", cursor, err)
	}
	client, err = jetstreamClient.NewClient(config, logger, cursorTestScheduler{})
	if err != nil {
		t.Fatal(err)
	}
	controller = NewRuntimeJetstreamController(logger, &Handler{logger: logger, Jsc: client}, config.WebsocketURL, cursor)
	controller.cursorStore = store
	controller.lastSavedCursor = cursor
	controller.cursorSaveInterval = time.Hour
	if _, err := controller.Connect(JetstreamConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := <-requests; got != "456" {
		t.Fatalf("resumed cursor = %q", got)
	}
	reset := int64(-1)
	if _, err := controller.Connect(JetstreamConnectRequest{Cursor: &reset}); err != nil {
		t.Fatal(err)
	}
	if cursor, err := store.Load(); err != nil || cursor != -1 {
		t.Fatalf("reset cursor = %d, %v", cursor, err)
	}
	if got := <-requests; got != "" {
		t.Fatalf("live cursor = %q", got)
	}
}

func TestJetstreamCursorSavedOnDisconnect(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		defer connection.Close()
		if err := connection.WriteJSON(map[string]interface{}{"time_us": 789}); err != nil {
			return
		}
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	config := jetstreamClient.DefaultClientConfig()
	config.Compress = false
	config.WebsocketURL = "ws" + strings.TrimPrefix(server.URL, "http")
	client, err := jetstreamClient.NewClient(config, logger, cursorTestScheduler{})
	if err != nil {
		t.Fatal(err)
	}
	store := newJetstreamCursorStore(t.TempDir())
	controller := NewRuntimeJetstreamController(logger, &Handler{logger: logger, Jsc: client}, config.WebsocketURL, -1)
	controller.cursorStore = store
	controller.cursorSaveInterval = time.Hour
	defer controller.Disconnect()
	if _, err := controller.Connect(JetstreamConnectRequest{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for client.CurrentCursor() != 789 {
		select {
		case <-deadline:
			t.Fatal("event was not received")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if _, err := controller.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if cursor, err := store.Load(); err != nil || cursor != 789 {
		t.Fatalf("final cursor = %d, %v", cursor, err)
	}
}
