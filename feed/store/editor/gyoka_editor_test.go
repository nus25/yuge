package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/bluesky-social/indigo/util"
	gt "github.com/nus25/gyoka-client/go/types"
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
	}{
		{
			name:     "正常系",
			did:      "did:plc:test",
			rkey:     "test",
			cid:      "test-cid",
			feed:     "at://did:plc:test/app.bsky.feed.generator/test-feed",
			endpoint: "http://test.example",
		},
		{
			name:     "エンドポイントなし",
			did:      "did:plc:test",
			rkey:     "test",
			cid:      "test-cid",
			feed:     "at://did:plc:test/app.bsky.feed.generator/test-feed",
			endpoint: "",
		},
	}

	for _, tt := range tests {
		t.Run("Add_"+tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.endpoint != "" {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/" {
						w.WriteHeader(http.StatusOK)
						return
					}

					if r.URL.Path != "/feed/add" {
						t.Errorf("expected path /feed/add, got %s", r.URL.Path)
					}
					if r.Method != "POST" {
						t.Errorf("expected method POST, got %s", r.Method)
					}

					type CreatePostRequest struct {
						Posts []types.Post `json:"posts"`
					}
					var req CreatePostRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					if err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if len(req.Posts) != 1 {
						t.Errorf("expected 1 post, got %d", len(req.Posts))
					}
					if req.Posts[0].Feed != types.FeedUri(tt.feed) {
						t.Errorf("expected feed %s, got %s", tt.feed, req.Posts[0].Feed)
					}
					if req.Posts[0].Uri != types.PostUri("at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey) {
						t.Errorf("expected URI %s, got %s", "at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey, req.Posts[0].Uri)
					}
					resp := struct {
						InsertedPosts []types.Post `json:"insertedPosts"`
						FailedPosts   []types.Post `json:"failedPosts"`
						Message       string       `json:"message"`
					}{
						InsertedPosts: req.Posts,
						FailedPosts:   []types.Post{},
						Message:       "success",
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
					if r.URL.Path == "/" {
						w.WriteHeader(http.StatusOK)
						return
					}
					if r.URL.Path != "/feed/delete" {
						t.Errorf("expected path /feed/delete, got %s", r.URL.Path)
					}
					if r.Method != "POST" {
						t.Errorf("expected method POST, got %s", r.Method)
					}

					type DeletePostRequest struct {
						Posts []types.Post `json:"posts"`
					}
					var req DeletePostRequest
					err := json.NewDecoder(r.Body).Decode(&req)
					if err != nil {
						t.Errorf("failed to decode request body: %v", err)
					}
					if len(req.Posts) != 1 {
						t.Errorf("expected 1 post, got %d", len(req.Posts))
					}
					if req.Posts[0].Feed != types.FeedUri(tt.feed) {
						t.Errorf("expected feed %s, got %s", tt.feed, req.Posts[0].Feed)
					}
					if req.Posts[0].Uri != types.PostUri("at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey) {
						t.Errorf("expected URI %s, got %s", "at://"+tt.did+"/app.bsky.feed.post/"+tt.rkey, req.Posts[0].Uri)
					}
					type DeletePostResponse struct {
						DeletedPosts []types.Post `json:"deletedPosts"`
						FailedPosts  []types.Post `json:"failedPosts"`
						Message      string       `json:"message"`
					}
					resp := DeletePostResponse{
						DeletedPosts: req.Posts,
						FailedPosts:  []types.Post{},
						Message:      "success",
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

		t.Run("Trim_"+tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.endpoint != "" {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/" {
						w.WriteHeader(http.StatusOK)
						return
					}
					if r.URL.Path != "/feed/trim" {
						t.Errorf("expected path /feed/trim, got %s", r.URL.Path)
					}
					if r.Method != "GET" {
						t.Errorf("expected method GET, got %s", r.Method)
					}

					feed := r.URL.Query().Get("feed")
					withinCount := r.URL.Query().Get("within-count")
					if feed != tt.feed {
						t.Errorf("expected feed %s, got %s", tt.feed, feed)
					}
					if withinCount != "100" {
						t.Errorf("expected within-count 100, got %s", withinCount)
					}
					type TrimResponse struct {
						DeletedCount int    `json:"deletedCount"`
						Message      string `json:"message"`
					}
					resp := TrimResponse{
						DeletedCount: 10,
						Message:      "success",
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

			err = client.Trim(TrimParams{
				FeedUri: types.FeedUri(tt.feed),
				Count:   100,
			})
			if err != nil {
				t.Errorf("failed to trim feed: %v", err)
			}

			time.Sleep(100 * time.Millisecond) // リクエストの処理を待つ
		})
	}
}

func TestAuthHeaders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("CloudflareAccess", func(t *testing.T) {
		client, err := NewGyokaEditor("http://test.example", logger, WithCfToken("test-id", "test-secret"))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		if int(client.client.GetAuthType()) != int(CloudflareAccess) {
			t.Errorf("expected auth type %d, got %d", int(CloudflareAccess), int(client.client.GetAuthType()))
		}
	})

	t.Run("BearerToken", func(t *testing.T) {
		client, err := NewGyokaEditor("http://test.example", logger, WithBearerToken("test-token"))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		if int(client.client.GetAuthType()) != int(BearerToken) {
			t.Errorf("expected auth type %d, got %d", int(BearerToken), int(client.client.GetAuthType()))
		}
	})

	t.Run("BasicAuth", func(t *testing.T) {
		client, err := NewGyokaEditor("http://test.example", logger, WithBasicAuth("test-user", "test-pass"))
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		if int(client.client.GetAuthType()) != int(BasicAuth) {
			t.Errorf("expected auth type %d, got %d", int(BasicAuth), int(client.client.GetAuthType()))
		}
	})

	t.Run("NoAuth", func(t *testing.T) {
		client, err := NewGyokaEditor("http://test.example", logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}
		if client.client == nil {
			t.Error("client is nil")
		}
		if int(client.client.GetAuthType()) != int(NoAuth) {
			t.Errorf("expected auth type %d, got %d", int(NoAuth), int(client.client.GetAuthType()))
		}
	})
}

func TestBufferingAdd(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("バッファリングとバッチ処理", func(t *testing.T) {
		// テストサーバーの起動
		mux := http.NewServeMux()
		var receivedPosts []gt.Post
		var reqcount = 0
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		mux.HandleFunc("/feed/add", func(w http.ResponseWriter, r *http.Request) {
			reqcount++
			var req struct {
				Posts []gt.Post `json:"posts"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			receivedPosts = append(receivedPosts, req.Posts...)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "success",
			})
		})

		server := httptest.NewServer(mux)
		defer server.Close()

		// クライアントの作成
		client, err := NewGyokaEditor(server.URL, logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		// テスト用のフィード
		feed := types.FeedUri("at://did:plc:userdid/app.bsky.feed.generator/samplefeed")

		// maxBatchSizeを超える数の投稿を追加
		posts := make([]PostParams, maxBatchSize+10)
		for i := range posts {
			posts[i] = PostParams{
				FeedUri:   feed,
				Did:       fmt.Sprintf("did:plc:testuser%d", i),
				Rkey:      fmt.Sprintf("rkey%d", i),
				Cid:       fmt.Sprintf("cid%d", i),
				IndexedAt: time.Now(),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// クライアントを開始
		if err = client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}

		// 投稿を追加
		for _, v := range posts {
			if err := client.Add(v); err != nil {
				t.Errorf("failed to add post: %v", err)
			}
		}

		// バッファの状態を確認(maxBatchSize分が送信済み)
		client.buffer.Lock()
		if got := len(client.buffer.addPosts); got != len(posts)-maxBatchSize {
			t.Errorf("buffer.addPosts length = %d, want %d", got, len(posts)-maxBatchSize)
		}
		client.buffer.Unlock()

		// 処理を要求
		client.processCh <- struct{}{}
		time.Sleep(200 * time.Millisecond) // 処理完了を待つ

		// バッファが空になったことを確認
		client.buffer.Lock()
		if got := len(client.buffer.addPosts); got != 0 {
			t.Errorf("buffer.addPosts length = %d, want 0", got)
		}
		client.buffer.Unlock()

		if reqcount != 2 {
			t.Errorf("request count = %d, want 2", reqcount)
		}

		// サーバーが受信した投稿を確認
		if got := len(receivedPosts); got != len(posts) {
			t.Errorf("received posts length = %d, want %d", got, len(posts))
		}
		for i, p := range receivedPosts {
			uri, _ := util.ParseAtUri(string(p.Uri))
			if got := uri.Did; got != fmt.Sprintf("did:plc:testuser%d", i) {
				t.Errorf("post[%d].Did = %s, want did:plc:testuser%d", i, got, i)
			}
			if got := uri.Rkey; got != fmt.Sprintf("rkey%d", i) {
				t.Errorf("post[%d].Rkey = %s, want rkey%d", i, got, i)
			}
			if got := p.Cid; got != fmt.Sprintf("cid%d", i) {
				t.Errorf("post[%d].Cid = %s, want cid%d", i, got, i)
			}
		}
	})
}

func TestBufferingDelete(t *testing.T) {
	t.Run("バッファリングされたdeleteリクエストが正しく処理される", func(t *testing.T) {
		var reqcount int
		var receivedPosts []gt.Post
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/feed/delete" {
				reqcount++
				var req struct {
					Posts []gt.Post `json:"posts"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Fatal(err)
				}
				receivedPosts = append(receivedPosts, req.Posts...)
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"message": "success",
				})
			} else if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer ts.Close()

		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		client, err := NewGyokaEditor(ts.URL, logger)
		if err != nil {
			t.Fatalf("failed to create editor: %v", err)
		}

		feed := types.FeedUri("at://did:plc:test/app.bsky.feed.generator/test")
		posts := make([]DeleteParams, maxBatchSize+10)
		for i := range posts {
			posts[i] = DeleteParams{
				FeedUri: feed,
				Did:     fmt.Sprintf("did:plc:testuser%d", i),
				Rkey:    fmt.Sprintf("rkey%d", i),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// クライアントを開始
		if err = client.Open(ctx); err != nil {
			t.Fatalf("failed to open client: %v", err)
		}

		// 投稿を削除
		for _, v := range posts {
			if err := client.Delete(v); err != nil {
				t.Errorf("failed to delete post: %v", err)
			}
		}

		// バッファの状態を確認(maxBatchSize分が送信済み)
		client.buffer.Lock()
		if got := len(client.buffer.deletePosts); got != len(posts)-maxBatchSize {
			t.Errorf("buffer.deletePosts length = %d, want %d", got, len(posts)-maxBatchSize)
		}
		client.buffer.Unlock()

		// 処理を要求
		client.processCh <- struct{}{}
		time.Sleep(200 * time.Millisecond) // 処理完了を待つ

		// バッファが空になったことを確認
		client.buffer.Lock()
		if got := len(client.buffer.deletePosts); got != 0 {
			t.Errorf("buffer.deletePosts length = %d, want 0", got)
		}
		client.buffer.Unlock()

		if reqcount != 2 {
			t.Errorf("request count = %d, want 2", reqcount)
		}

		// サーバーが受信した投稿を確認
		if got := len(receivedPosts); got != len(posts) {
			t.Errorf("received posts length = %d, want %d", got, len(posts))
		}
		for i, p := range receivedPosts {
			uri, _ := util.ParseAtUri(string(p.Uri))
			if got := uri.Did; got != fmt.Sprintf("did:plc:testuser%d", i) {
				t.Errorf("post[%d].Did = %s, want did:plc:testuser%d", i, got, i)
			}
			if got := uri.Rkey; got != fmt.Sprintf("rkey%d", i) {
				t.Errorf("post[%d].Rkey = %s, want rkey%d", i, got, i)
			}
		}
	})
}

func TestTrim(t *testing.T) {
	t.Run("trim request", func(t *testing.T) {
		var reqcount int
		var receivedFeed string
		var receivedCount int

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.WriteHeader(http.StatusOK)
				return
			}
			reqcount++
			if got := strings.TrimSuffix(r.URL.Path, "/"); got != "/feed/trim" {
				t.Errorf("path = %s, want /feed/trim", got)
			}
			if got := r.Method; got != http.MethodGet {
				t.Errorf("method = %s, want GET", got)
			}

			receivedFeed = r.URL.Query().Get("feed")
			count, _ := strconv.Atoi(r.URL.Query().Get("within-count"))
			receivedCount = count

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
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
