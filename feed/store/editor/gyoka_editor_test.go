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
				t.Errorf("failed to add post: %v", err)
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
