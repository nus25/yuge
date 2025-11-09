package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"log/slog"

	"github.com/nus25/yuge/types"
)

func TestGyokaEditor(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name     string
		did      string
		rkey     string
		cid      string
		feed     string
		endpoint string
		langs    []string
	}{
		{
			name:     "正常系",
			did:      "did:plc:test",
			rkey:     "test",
			cid:      "test-cid",
			feed:     "at://did:plc:test/app.bsky.feed.generator/test-feed",
			endpoint: "http://test.example",
			langs:    []string{"jp", "en"},
		},
		{
			name:     "エンドポイントなし",
			did:      "did:plc:test",
			rkey:     "test",
			cid:      "test-cid",
			feed:     "at://did:plc:test/app.bsky.feed.generator/test-feed",
			endpoint: "",
			langs:    []string{"jp", "en"},
		},
	}

	for _, tt := range tests {
		t.Run("Add_"+tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.endpoint != "" {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/gyoka/ping" {
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(map[string]any{
							"message": "Gyoka is available",
						})
						return
					}

					if r.URL.Path != "/api/feed/addPost" {
						t.Errorf("expected path /feed/add, got %s", r.URL.Path)
					}
					if r.Method != "POST" {
						t.Errorf("expected method POST, got %s", r.Method)
					}

					// todo fix request check and respose
					type CreatePostRequest struct {
						Feed types.FeedUri `json:"feed"`
						Post *types.Post   `json:"post"`
					}
					var req CreatePostRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					if err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if req.Feed != types.FeedUri(tt.feed) {
						t.Errorf("expected feed %s, got %s", tt.feed, req.Feed)
					}
					if req.Post.Uri != types.PostUri("at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey) {
						t.Errorf("expected URI %s, got %s", "at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey, req.Post.Uri)
					}
					resp := struct {
						Message string `json:"message"`
					}{
						Message: "success",
					}
					json.NewEncoder(w).Encode(resp)
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				tt.endpoint = server.URL
			}

			client, err := NewGyokaEditor(tt.endpoint, logger)
			if err != nil {
				t.Fatalf("failed to create editor: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go client.Open(ctx)
			time.Sleep(100 * time.Millisecond) // workerの起動を待つ

			err = client.Add(PostParams{
				FeedUri:   types.FeedUri(tt.feed),
				Did:       tt.did,
				Rkey:      tt.rkey,
				Cid:       tt.cid,
				IndexedAt: time.Now(),
				Langs:     tt.langs,
			})
			if err != nil {
				if tt.endpoint == "" && !strings.Contains(err.Error(), "no feed editor url is set.") {
					t.Errorf("unexpected error: %v", err)
				}
			}

			time.Sleep(100 * time.Millisecond) // リクエストの処理を待つ
		})

		t.Run("Delete_"+tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.endpoint != "" {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/gyoka/ping" {
						w.WriteHeader(http.StatusOK)
						json.NewEncoder(w).Encode(map[string]any{
							"message": "Gyoka is available",
						})
						return
					}
					if r.URL.Path != "/api/feed/removePost" {
						t.Errorf("expected path /feed/delete, got %s", r.URL.Path)
					}
					if r.Method != "POST" {
						t.Errorf("expected method POST, got %s", r.Method)
					}

					type DeletePostRequest struct {
						Feed types.FeedUri `json:"feed"`
						Post *types.Post   `json:"post"`
					}
					var req DeletePostRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					if err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if req.Feed != types.FeedUri(tt.feed) {
						t.Errorf("expected feed %s, got %s", tt.feed, req.Feed)
					}
					if req.Post.Uri != types.PostUri("at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey) {
						t.Errorf("expected URI %s, got %s", "at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey, req.Post.Uri)
					}
					type DeletePostResponse struct {
						Message string `json:"message"`
					}
					resp := DeletePostResponse{
						Message: "success",
					}
					json.NewEncoder(w).Encode(resp)

					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				tt.endpoint = server.URL
			}

			client, err := NewGyokaEditor(tt.endpoint, logger)
			if err != nil {
				t.Fatalf("failed to create editor: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go client.Open(ctx)
			time.Sleep(100 * time.Millisecond) // workerの起動を待つ

			err = client.Delete(DeleteParams{
				FeedUri: types.FeedUri(tt.feed),
				Did:     tt.did,
				Rkey:    tt.rkey,
			})
			if err != nil {
				t.Errorf("failed to delete post: %v", err)
			}

			time.Sleep(100 * time.Millisecond) // リクエストの処理を待つ
		})
	}
}

func TestRetryFunctionality(t *testing.T) {
	logger := slog.Default()

	t.Run("AddPost_RetryOnServerError", func(t *testing.T) {
		var attemptCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			atomic.AddInt32(&attemptCount, 1)
			if atomic.LoadInt32(&attemptCount) < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{
					"error":   "internal_error",
					"message": "server error",
				})
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "success",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		err = client.Add(PostParams{
			FeedUri:   types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test"),
			Did:       "did:plc:test",
			Rkey:      "test",
			Cid:       "test-cid",
			IndexedAt: time.Now(),
			Langs:     []string{"en"},
		})

		if err != nil {
			t.Errorf("expected success after retries, got error: %v", err)
		}

		finalAttempts := atomic.LoadInt32(&attemptCount)
		if finalAttempts != 3 {
			t.Errorf("expected 3 attempts, got %d", finalAttempts)
		}
	})

	t.Run("AddPost_NoRetryOnClientError", func(t *testing.T) {
		var attemptCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			atomic.AddInt32(&attemptCount, 1)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error":   "bad_request",
				"message": "invalid input",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		err = client.Add(PostParams{
			FeedUri:   types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test"),
			Did:       "did:plc:test",
			Rkey:      "test",
			Cid:       "test-cid",
			IndexedAt: time.Now(),
			Langs:     []string{"en"},
		})

		if err == nil {
			t.Error("expected error for bad request, got nil")
		}

		finalAttempts := atomic.LoadInt32(&attemptCount)
		if finalAttempts != 1 {
			t.Errorf("expected 1 attempt (no retry for 400), got %d", finalAttempts)
		}
	})

	t.Run("Open_RetryOnServerError", func(t *testing.T) {
		var attemptCount int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attemptCount, 1)
			if atomic.LoadInt32(&attemptCount) < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Gyoka is available",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err = client.Open(ctx)
		if err != nil {
			t.Errorf("expected success after retries, got error: %v", err)
		}

		finalAttempts := atomic.LoadInt32(&attemptCount)
		if finalAttempts != 3 {
			t.Errorf("expected 3 attempts, got %d", finalAttempts)
		}
	})
}

func TestBackoffCalculation(t *testing.T) {
	baseDelay := 100 * time.Millisecond

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 0},
		{1, baseDelay},
		{2, 2 * baseDelay},
		{3, 4 * baseDelay},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := calculateBackoffDelay(tt.attempt, baseDelay)
			if tt.attempt == 0 {
				if delay != 0 {
					t.Errorf("expected 0 delay for attempt 0, got %v", delay)
				}
				return
			}

			expectedBase := float64(tt.expected)
			actualFloat := float64(delay)
			jitterRange := expectedBase * 0.1

			if actualFloat < expectedBase-jitterRange || actualFloat > expectedBase+jitterRange {
				t.Errorf("delay %v not within expected range %v ± %v", delay, tt.expected, jitterRange)
			}
		})
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{408, true},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.statusCode), func(t *testing.T) {
			result := isRetryableError(tt.statusCode)
			if result != tt.retryable {
				t.Errorf("expected %v for status %d, got %v", tt.retryable, tt.statusCode, result)
			}
		})
	}
}

func TestAuthHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("CloudflareAccess", func(t *testing.T) {
		testId := "test-id"
		testSecret := "test-secret"
		// test server
		mux := http.NewServeMux()
		mux.HandleFunc("/api/gyoka/ping", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("CF-Access-Client-Id") != testId {
				t.Errorf("CF-Access-Client-Id in header mismatching %s", r.Header.Get("CF-Access-Client-Id"))
			}
			if r.Header.Get("CF-Access-Client-Secret") != testSecret {
				t.Errorf("CF-Access-Client-Secret in header mismatching %s", r.Header.Get("CF-Access-Client-Secret"))
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Gyoka is available",
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		// test client
		client, err := NewGyokaEditor(server.URL, logger, WithCfToken(testId, testSecret))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = client.Open(ctx)
		if err != nil {
			t.Error("error in request")
		}
	})
	t.Run("Apikey", func(t *testing.T) {
		testKey := "test-key"
		// test server
		mux := http.NewServeMux()
		mux.HandleFunc("/api/gyoka/ping", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Api-Key") != testKey {
				t.Errorf("X-Api-Key in header mismatching %s", r.Header.Get("X-Api-Key"))
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Gyoka is available",
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()

		// test client
		client, err := NewGyokaEditor(server.URL, logger, WithApiKey(testKey))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = client.Open(ctx)
		if err != nil {
			t.Error("error in request")
		}
	})
	t.Run("Both", func(t *testing.T) {
		testId := "test-id"
		testSecret := "test-secret"
		testKey := "test-key"
		// test server
		mux := http.NewServeMux()
		mux.HandleFunc("/api/gyoka/ping", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("CF-Access-Client-Id") != testId {
				t.Errorf("CF-Access-Client-Id in header mismatching %s", r.Header.Get("CF-Access-Client-Id"))
			}
			if r.Header.Get("CF-Access-Client-Secret") != testSecret {
				t.Errorf("CF-Access-Client-Secret in header mismatching %s", r.Header.Get("CF-Access-Client-Secret"))
			}
			if r.Header.Get("X-Api-Key") != testKey {
				t.Errorf("X-Api-Key in header mismatching %s", r.Header.Get("X-Api-Key"))
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Gyoka is available",
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()
		var opts []ClientOptionFunc
		opts = append(opts, WithCfToken(testId, testSecret), WithApiKey(testKey))
		// test client
		client, err := NewGyokaEditor(server.URL, logger, opts...)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = client.Open(ctx)
		if err != nil {
			t.Error("error in request")
		}
	})
	t.Run("NoAuth", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/gyoka/ping", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("CF-Access-Client-Id") != "" {
				t.Error("CF-Access-Client-Id is in header")
			}
			if r.Header.Get("CF-Access-Client-Secret") != "" {
				t.Error("CF-Access-Client-Secret is in header")
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Gyoka is available",
			})
		})
		server := httptest.NewServer(mux)
		defer server.Close()
		client, err := NewGyokaEditor(server.URL, logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		err = client.Open(ctx)
		if err != nil {
			t.Error("error in request")
		}
	})
}

func TestTrim(t *testing.T) {
	t.Run("trim request", func(t *testing.T) {
		var reqcount int
		var receivedFeed string
		var receivedCount int

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}
			if got := strings.TrimSuffix(r.URL.Path, "/"); got != "/api/feed/trimPosts" {
				t.Errorf("path = %s, want /api/feed/trimPosts", got)
			}
			if r.Method != "POST" {
				t.Errorf("expected method POST, got %s", r.Method)
			}
			reqcount++
			var req struct {
				Feed   string `json:"feed"`
				Remain int    `json:"remain"`
			}
			err := json.NewDecoder(r.Body).Decode(&req)
			if err != nil {
				t.Errorf("unwanted body %+v", r.Body)
			}
			receivedFeed = req.Feed
			receivedCount = req.Remain
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message":      "success",
				"deletedCount": 10,
			})
		}))
		defer ts.Close()

		client, err := NewGyokaEditor(ts.URL, nil, nil)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		feed := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		params := TrimParams{
			FeedUri: feed,
			Count:   100,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// クライアントを開始
		if err = client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}

		// フィードをトリム
		if err = client.Trim(params); err != nil {
			t.Errorf("failed to trim feed: %v", err)
		}

		if reqcount != 1 {
			t.Errorf("request count = %d, want 1", reqcount)
		}
		if got := receivedFeed; got != string(feed) {
			t.Errorf("received feed = %s, want %s", got, string(feed))
		}
		if got := receivedCount; got != params.Count {
			t.Errorf("received count = %d, want %d", got, params.Count)
		}
	})
}

func TestDeleteByDid(t *testing.T) {
	t.Run("deleteByDid request", func(t *testing.T) {
		var reqcount int
		var receivedFeed string
		var receivedAuthor string

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}
			if got := strings.TrimSuffix(r.URL.Path, "/"); got != "/api/feed/removePostByAuthor" {
				t.Errorf("path = %s, want /api/feed/removePostByAuthor", got)
			}
			if r.Method != "POST" {
				t.Errorf("expected method POST, got %s", r.Method)
			}
			reqcount++
			var req struct {
				Feed   string `json:"feed"`
				Author string `json:"author"`
			}
			err := json.NewDecoder(r.Body).Decode(&req)
			if err != nil {
				t.Errorf("failed to decode body: %v", err)
			}
			receivedFeed = req.Feed
			receivedAuthor = req.Author
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message":      "success",
				"deletedCount": 5,
			})
		}))
		defer ts.Close()

		client, err := NewGyokaEditor(ts.URL, nil, nil)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		feed := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		did := "did:plc:testauthor"

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err = client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}

		if err = client.DeleteByDid(feed, did); err != nil {
			t.Errorf("failed to delete by did: %v", err)
		}

		if reqcount != 1 {
			t.Errorf("request count = %d, want 1", reqcount)
		}
		if got := receivedFeed; got != string(feed) {
			t.Errorf("received feed = %s, want %s", got, string(feed))
		}
		if got := receivedAuthor; got != did {
			t.Errorf("received author = %s, want %s", got, did)
		}
	})
}

func TestGyokaEditorErrorMessages(t *testing.T) {
	logger := slog.Default()

	t.Run("Add_InvalidFeedUri", func(t *testing.T) {
		client, err := NewGyokaEditor("example.com", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		err = client.Add(PostParams{
			FeedUri:   types.FeedUri("invalid-uri"),
			Did:       "did:plc:test",
			Rkey:      "test",
			Cid:       "test-cid",
			IndexedAt: time.Now(),
			Langs:     []string{"en"},
		})

		if err == nil {
			t.Error("expected error for invalid feed uri, got nil")
		}
		if !strings.Contains(err.Error(), "invalid feed uri") {
			t.Errorf("expected error message to contain 'invalid feed uri', got: %v", err)
		}
	})

	t.Run("Add_ServerError_ErrorMessage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"error":   "database_error",
				"message": "failed to connect to database",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		err = client.Add(PostParams{
			FeedUri:   types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test"),
			Did:       "did:plc:test",
			Rkey:      "test",
			Cid:       "test-cid",
			IndexedAt: time.Now(),
			Langs:     []string{"en"},
		})

		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "retryable error") {
			t.Errorf("expected error message to contain 'retryable error', got: %v", err)
		}
	})

	t.Run("Add_BadRequest_ErrorMessage", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error":   "validation_error",
				"message": "invalid post format",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		err = client.Add(PostParams{
			FeedUri:   types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test"),
			Did:       "did:plc:test",
			Rkey:      "test",
			Cid:       "test-cid",
			IndexedAt: time.Now(),
			Langs:     []string{"en"},
		})

		if err == nil {
			t.Error("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "non-retryable") {
			t.Errorf("expected error message to contain 'non-retryable', got: %v", err)
		}
	})

	t.Run("Delete_InvalidFeedUri", func(t *testing.T) {
		client, err := NewGyokaEditor("example.com", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		err = client.Delete(DeleteParams{
			FeedUri: types.FeedUri("invalid-uri"),
			Did:     "did:plc:test",
			Rkey:    "test",
		})

		if err == nil {
			t.Error("expected error for invalid feed uri, got nil")
		}
		if !strings.Contains(err.Error(), "invalid feed uri") {
			t.Errorf("expected error message to contain 'invalid feed uri', got: %v", err)
		}
	})

	t.Run("Trim_NegativeCount", func(t *testing.T) {
		client, err := NewGyokaEditor("example.com", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		err = client.Trim(TrimParams{
			FeedUri: types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test"),
			Count:   -1,
		})

		if err == nil {
			t.Error("expected error for negative count, got nil")
		}
		if !strings.Contains(err.Error(), "invalid count") {
			t.Errorf("expected error message to contain 'invalid count', got: %v", err)
		}
	})

	t.Run("Open_InvalidResponse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Invalid message",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = client.Open(ctx)
		if err == nil {
			t.Error("expected error for invalid response message, got nil")
		}
		if !strings.Contains(err.Error(), "unexpected message") {
			t.Errorf("expected error message to contain 'unexpected message', got: %v", err)
		}
	})

	t.Run("Open_MalformedJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("not json"))
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger, WithRetryWaitTime(100*time.Microsecond))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = client.Open(ctx)
		if err == nil {
			t.Error("expected error for malformed JSON, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse response body as JSON") {
			t.Errorf("expected error message to contain 'failed to parse response body as JSON', got: %v", err)
		}
	})

	t.Run("DeleteByDid_InvalidFeedUri", func(t *testing.T) {
		client, err := NewGyokaEditor("example.com", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		err = client.DeleteByDid(types.FeedUri("invalid-uri"), "did:plc:test")

		if err == nil {
			t.Error("expected error for invalid feed uri, got nil")
		}
		if !strings.Contains(err.Error(), "invalid feed uri") {
			t.Errorf("expected error message to contain 'invalid feed uri', got: %v", err)
		}
	})
}

func TestBatchAdd(t *testing.T) {
	logger := slog.Default()

	t.Run("BatchAdd_MultipleAdds", func(t *testing.T) {
		var requestCount int32
		var lastBatchSize int

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			atomic.AddInt32(&requestCount, 1)

			if r.URL.Path == "/api/feed/addPost" {
				// Single add request (first one)
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "success",
				})
				return
			}

			if r.URL.Path == "/api/feed/batchAddPosts" {
				// Batch add request
				var req struct {
					Entries []struct {
						Feed  string `json:"feed"`
						Posts []struct {
							Uri string `json:"uri"`
							Cid string `json:"cid"`
						} `json:"posts"`
					} `json:"entries"`
				}
				err := json.NewDecoder(r.Body).Decode(&req)
				if err != nil {
					t.Errorf("failed to decode batch request body: %v", err)
					return
				}

				totalPosts := 0
				for _, entry := range req.Entries {
					totalPosts += len(entry.Posts)
				}
				lastBatchSize = totalPosts

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "batch success",
				})
				return
			}

			t.Errorf("unexpected path: %s", r.URL.Path)
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		// Add 3 posts in quick succession
		feedUri := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")

		for i := 0; i < 3; i++ {
			err = client.Add(PostParams{
				FeedUri:   feedUri,
				Did:       "did:plc:test",
				Rkey:      fmt.Sprintf("test%d", i),
				Cid:       fmt.Sprintf("test-cid-%d", i),
				IndexedAt: time.Now(),
				Langs:     []string{"en"},
			})
			if i == 0 && err != nil {
				t.Errorf("failed to add first post: %v", err)
			}
			// Subsequent adds return immediately (batched)
		}

		// Wait for batch to be processed
		time.Sleep(2 * time.Second)

		// Should have 2 requests: 1 individual add + 1 batch add
		finalRequestCount := atomic.LoadInt32(&requestCount)
		if finalRequestCount != 2 {
			t.Errorf("expected 2 requests (1 add + 1 batch), got %d", finalRequestCount)
		}

		// Batch should contain 2 posts (excluding the first one)
		if lastBatchSize != 2 {
			t.Errorf("expected batch size 2, got %d", lastBatchSize)
		}
	})

	t.Run("BatchAdd_ExplicitBatch", func(t *testing.T) {
		var requestCount int32
		var receivedEntries int

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			if r.URL.Path != "/api/feed/batchAddPosts" {
				t.Errorf("expected path /api/feed/batchAddPosts, got %s", r.URL.Path)
				return
			}

			atomic.AddInt32(&requestCount, 1)

			var req struct {
				Entries []struct {
					Feed  string `json:"feed"`
					Posts []struct {
						Uri       string    `json:"uri"`
						Cid       string    `json:"cid"`
						IndexedAt time.Time `json:"indexedAt"`
					} `json:"posts"`
				} `json:"entries"`
			}
			err := json.NewDecoder(r.Body).Decode(&req)
			if err != nil {
				t.Errorf("failed to decode request body: %v", err)
				return
			}

			receivedEntries = len(req.Entries)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "batch success",
			})
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		// Use explicit BatchAdd
		feedUri := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		entries := []PostParams{
			{
				FeedUri:   feedUri,
				Did:       "did:plc:test1",
				Rkey:      "test1",
				Cid:       "test-cid-1",
				IndexedAt: time.Now(),
				Langs:     []string{"en"},
			},
			{
				FeedUri:   feedUri,
				Did:       "did:plc:test2",
				Rkey:      "test2",
				Cid:       "test-cid-2",
				IndexedAt: time.Now(),
				Langs:     []string{"ja"},
			},
		}

		err = client.BatchAdd(BatchPostParams{Entries: entries})
		if err != nil {
			t.Errorf("failed to batch add posts: %v", err)
		}

		finalRequestCount := atomic.LoadInt32(&requestCount)
		if finalRequestCount != 1 {
			t.Errorf("expected 1 batch request, got %d", finalRequestCount)
		}

		if receivedEntries != 1 {
			t.Errorf("expected 1 feed entry, got %d", receivedEntries)
		}
	})

	t.Run("BatchAdd_InvalidFeedUri", func(t *testing.T) {
		client, err := NewGyokaEditor("example.com", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		entries := []PostParams{
			{
				FeedUri:   types.FeedUri("invalid-uri"),
				Did:       "did:plc:test",
				Rkey:      "test",
				Cid:       "test-cid",
				IndexedAt: time.Now(),
				Langs:     []string{"en"},
			},
		}

		err = client.BatchAdd(BatchPostParams{Entries: entries})
		if err == nil {
			t.Error("expected error for invalid feed uri, got nil")
		}
		if !strings.Contains(err.Error(), "invalid feed uri") {
			t.Errorf("expected error message to contain 'invalid feed uri', got: %v", err)
		}
	})

	t.Run("BatchAdd_NoClient", func(t *testing.T) {
		client, err := NewGyokaEditor("", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		entries := []PostParams{
			{
				FeedUri:   types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test"),
				Did:       "did:plc:test",
				Rkey:      "test",
				Cid:       "test-cid",
				IndexedAt: time.Now(),
				Langs:     []string{"en"},
			},
		}

		err = client.BatchAdd(BatchPostParams{Entries: entries})
		if err != nil {
			t.Errorf("expected no error when client is nil, got: %v", err)
		}
	})

	t.Run("BatchAdd_LargeBatch", func(t *testing.T) {
		var requestCount int32
		var totalProcessed int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/gyoka/ping" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "Gyoka is available",
				})
				return
			}

			if r.URL.Path == "/api/feed/batchAddPosts" {
				atomic.AddInt32(&requestCount, 1)

				var req struct {
					Entries []struct {
						Posts []interface{} `json:"posts"`
					} `json:"entries"`
				}
				json.NewDecoder(r.Body).Decode(&req)

				for _, entry := range req.Entries {
					atomic.AddInt32(&totalProcessed, int32(len(entry.Posts)))
				}

				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]any{
					"message": "batch success",
				})
				return
			}
		}))
		defer server.Close()

		client, err := NewGyokaEditor(server.URL, logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}
		time.Sleep(100 * time.Millisecond)

		// Create a batch larger than maxBatchSize (25)
		entries := make([]PostParams, 30)
		feedUri := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		for i := 0; i < 30; i++ {
			entries[i] = PostParams{
				FeedUri:   feedUri,
				Did:       "did:plc:test",
				Rkey:      fmt.Sprintf("test%d", i),
				Cid:       fmt.Sprintf("test-cid-%d", i),
				IndexedAt: time.Now(),
				Langs:     []string{"en"},
			}
		}

		err = client.BatchAdd(BatchPostParams{Entries: entries})
		if err != nil {
			t.Errorf("failed to batch add large set: %v", err)
		}

		// Should split into multiple batches (25 + 5)
		finalRequestCount := atomic.LoadInt32(&requestCount)
		if finalRequestCount != 2 {
			t.Errorf("expected 2 batch requests for 30 entries, got %d", finalRequestCount)
		}

		finalProcessed := atomic.LoadInt32(&totalProcessed)
		if finalProcessed != 30 {
			t.Errorf("expected 30 posts processed, got %d", finalProcessed)
		}
	})
}
