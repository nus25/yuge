package subscriber

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type mockJetstreamController struct {
	connectStatus    JetstreamStatusResponse
	disconnectStatus JetstreamStatusResponse
	status           JetstreamStatusResponse
	connectErr       error
	disconnectErr    error
	connectReq       JetstreamConnectRequest
	connectCalled    bool
	disconnectCalled bool
}

func (m *mockJetstreamController) Connect(req JetstreamConnectRequest) (JetstreamStatusResponse, error) {
	m.connectCalled = true
	m.connectReq = req
	if m.connectErr != nil {
		return JetstreamStatusResponse{}, m.connectErr
	}
	if m.connectStatus.WebsocketURL == "" {
		return m.status, nil
	}
	return m.connectStatus, nil
}

func (m *mockJetstreamController) Disconnect() (JetstreamStatusResponse, error) {
	m.disconnectCalled = true
	if m.disconnectErr != nil {
		return JetstreamStatusResponse{}, m.disconnectErr
	}
	if m.disconnectStatus.WebsocketURL == "" {
		return m.status, nil
	}
	return m.disconnectStatus, nil
}

func (m *mockJetstreamController) Status() JetstreamStatusResponse {
	return m.status
}

func TestAPIHandler_JetstreamEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockCtrl := &mockJetstreamController{
		status: JetstreamStatusResponse{
			Connected:    false,
			WebsocketURL: "ws://localhost:6008/subscribe",
			Cursor:       0,
		},
		connectStatus: JetstreamStatusResponse{
			Connected:    true,
			WebsocketURL: "wss://jet.example/subscribe",
			Cursor:       12345,
		},
		disconnectStatus: JetstreamStatusResponse{
			Connected:    false,
			WebsocketURL: "wss://jet.example/subscribe",
			Cursor:       12345,
		},
	}

	api := NewJetstreamApiHandler(mockCtrl)

	r := gin.Default()
	r.POST("/api/jetstream/connect", api.Connect)
	r.POST("/api/jetstream/disconnect", api.Disconnect)
	r.GET("/api/jetstream/status", api.Status)

	t.Run("connect success with optional params", func(t *testing.T) {
		body := map[string]any{
			"url":    "wss://jet.example/subscribe",
			"cursor": int64(12345),
		}
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, "/api/jetstream/connect", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !mockCtrl.connectCalled {
			t.Fatal("expected connect to be called")
		}
		if mockCtrl.connectReq.URL == nil || *mockCtrl.connectReq.URL != "wss://jet.example/subscribe" {
			t.Fatalf("unexpected url: %+v", mockCtrl.connectReq.URL)
		}
		if mockCtrl.connectReq.Cursor == nil || *mockCtrl.connectReq.Cursor != 12345 {
			t.Fatalf("unexpected cursor: %+v", mockCtrl.connectReq.Cursor)
		}
	})

	t.Run("connect invalid json", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/jetstream/connect", bytes.NewBufferString("{"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("connect internal error", func(t *testing.T) {
		mockCtrl.connectErr = errors.New("boom")
		defer func() { mockCtrl.connectErr = nil }()

		req, _ := http.NewRequest(http.MethodPost, "/api/jetstream/connect", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected status 500, got %d", rec.Code)
		}
	})

	t.Run("disconnect success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/api/jetstream/disconnect", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if !mockCtrl.disconnectCalled {
			t.Fatal("expected disconnect to be called")
		}
	})

	t.Run("status success", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/jetstream/status", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		var got JetstreamStatusResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.WebsocketURL == "" {
			t.Fatal("expected websocketURL in status response")
		}
	})
}

func TestAPIHandler_JetstreamEndpoints_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewJetstreamApiHandler(NewUnavailableJetstreamController())
	r := gin.Default()
	r.POST("/api/jetstream/connect", api.Connect)
	r.POST("/api/jetstream/disconnect", api.Disconnect)
	r.GET("/api/jetstream/status", api.Status)

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "connect", method: http.MethodPost, path: "/api/jetstream/connect"},
		{name: "disconnect", method: http.MethodPost, path: "/api/jetstream/disconnect"},
		{name: "status", method: http.MethodGet, path: "/api/jetstream/status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, tc.path, bytes.NewBufferString("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected status 503, got %d", rec.Code)
			}
		})
	}
}
