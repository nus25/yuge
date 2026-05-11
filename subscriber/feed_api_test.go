package subscriber

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	storerepo "github.com/nus25/yuge/feed/store/repository"
	storesqlite "github.com/nus25/yuge/feed/store/sqlite"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	projectionsqlite "github.com/nus25/yuge/subscriber/projection/sqlite"
	"github.com/nus25/yuge/types"
)

func createFeedService(t *testing.T) (*FeedService, string, error) {
	t.Helper()
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "feed-service-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	// deferを削除し、tempDirを返すように変更

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	dp, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create feed definition provider: %v", err)
	}
	fs, err := NewFeedService(configDir, dataDir, dp, logger)

	return fs, tempDir, err
}

func createJSONBody(t *testing.T, data map[string]any) io.Reader {
	t.Helper()
	jsonData, _ := json.Marshal(data)
	return bytes.NewBuffer(jsonData)
}

func TestAPIHandler_feedOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fs, tempDir, err := createFeedService(t)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}
	api := NewFeedApiHandler(fs)

	//create config file
	configFile := filepath.Join(tempDir, "config", "test-config.yaml")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte("{\"detailedLog\": true}"), 0644)

	router := gin.Default()
	// feed operations
	router.POST("/api/feed/:feedid", api.RegisterFeed)

	router.GET("/api/feed", api.ListFeed)
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		GET("", api.GetFeedInfo).
		GET("/status", api.GetFeedStatus).
		PUT("/status", api.UpdateFeedStatus).
		DELETE("", api.UnregisterFeed)

	//register feed
	req, _ := http.NewRequest("POST", "/api/feed/test-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	// Create request body with feed data
	req.Body = io.NopCloser(createJSONBody(t, map[string]any{
		"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		"configFile":    "test-config.yaml",
		"inactiveStart": false,
	}))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	body := recorder.Body.String()
	expectedBody := `{"feedId":"test-feed","message":"Feed created successfully","status":"active"}`
	if body != expectedBody {
		t.Errorf("Expected body %s, but got %s", expectedBody, body)
	}

	//// test list feed
	req, _ = http.NewRequest("GET", "/api/feed", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	body = recorder.Body.String()

	// JSONをパースして比較（lastUpdatedフィールドを除く）
	var actualData []map[string]any
	if err := json.Unmarshal([]byte(body), &actualData); err != nil {
		t.Errorf("JSONのパースに失敗: %v", err)
	}

	// lastUpdatedフィールドを無視するために削除
	if len(actualData) > 0 && actualData[0]["status"] != nil {
		if status, ok := actualData[0]["status"].(map[string]any); ok {
			delete(status, "lastUpdated")
		}
	}

	// 期待値
	expectedData := []map[string]any{
		{
			"id": "test-feed",
			"definition": map[string]any{
				"id":            "test-feed",
				"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
				"configFile":    "test-config.yaml",
				"inactiveStart": "false",
			},
			"status": map[string]any{
				"feedId":     "test-feed",
				"lastStatus": "active",
			},
		},
	}
	// Compare
	expectedJSON, _ := json.Marshal(expectedData)
	actualJSON, _ := json.Marshal(actualData)
	//remove lastUpdated field
	if len(actualData) > 0 && actualData[0]["status"] != nil {
		if status, ok := actualData[0]["status"].(map[string]any); ok {
			delete(status, "lastUpdated")
		}
	}
	if string(actualJSON) != string(expectedJSON) {
		t.Errorf("Expected value does not match actual value.\nExpected: %s\nActual: %s", string(expectedJSON), string(actualJSON))
	}

	//// test get feed info
	req, _ = http.NewRequest("GET", "/api/feed/test-feed", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	body = recorder.Body.String()

	// JSONをパースして比較（lastUpdatedフィールドを除く）
	var getFeedInfoActualData map[string]any
	if err := json.Unmarshal([]byte(body), &getFeedInfoActualData); err != nil {
		t.Errorf("JSONのパースに失敗: %v", err)
	}

	// lastpdatedフィールドを無視するために削除
	if status, ok := getFeedInfoActualData["status"].(map[string]any); ok {
		delete(status, "lastUpdated")
	}

	// 期待値
	// Check ID
	expectedID := "test-feed"
	actualID, ok := getFeedInfoActualData["id"].(string)
	if !ok || actualID != expectedID {
		t.Errorf("ID mismatch - Expected: %s, Got: %s", expectedID, actualID)
	}

	// Check URI
	expectedURI := "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed"
	actualURI, ok := getFeedInfoActualData["uri"].(string)
	if !ok || actualURI != expectedURI {
		t.Errorf("URI mismatch - Expected: %s, Got: %s", expectedURI, actualURI)
	}

	// Check Status
	expectedStatus := map[string]any{
		"feedId":     "test-feed",
		"lastStatus": "active",
	}
	actualStatus, ok := getFeedInfoActualData["status"].(map[string]any)
	if !ok {
		t.Error("Status field not found or invalid type")
	} else {
		delete(actualStatus, "lastUpdated")
		expectedStatusJSON, _ := json.Marshal(expectedStatus)
		actualStatusJSON, _ := json.Marshal(actualStatus)
		if string(actualStatusJSON) != string(expectedStatusJSON) {
			t.Errorf("Status mismatch - Expected: %s, Got: %s", string(expectedStatusJSON), string(actualStatusJSON))
		}
	}

	// Check Config
	expectedConfig := map[string]any{
		"detailedLog": true,
	}
	actualConfig, ok := getFeedInfoActualData["config"].(map[string]any)
	if !ok {
		t.Error("Config field not found or invalid type")
	} else {
		expectedConfigJSON, _ := json.Marshal(expectedConfig)
		actualConfigJSON, _ := json.Marshal(actualConfig)
		if string(actualConfigJSON) != string(expectedConfigJSON) {
			t.Errorf("Config mismatch - Expected: %s, Got: %s", string(expectedConfigJSON), string(actualConfigJSON))
		}
	}

	// Check Metrics
	actualMetrics, ok := getFeedInfoActualData["metrics"].(map[string]any)
	if !ok {
		t.Error("Metrics field not found or invalid type")
	} else {
		expectedMetrics := []map[string]any{
			{
				"description": "post count of the feed",
				"metricName":  "feed_post_count",
				"metricType":  "int",
			},
		}
		expectedMetricsJSON, _ := json.Marshal(expectedMetrics)
		actualMetricsJSON, _ := json.Marshal(actualMetrics["metrics"])
		if string(actualMetricsJSON) != string(expectedMetricsJSON) {
			t.Errorf("Metrics mismatch - Expected: %s, Got: %s", string(expectedMetricsJSON), string(actualMetricsJSON))
		}
	}

	//// test get feed status
	req, _ = http.NewRequest("GET", "/api/feed/test-feed/status", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	// レスポンスボディを取得して検証
	body = recorder.Body.String()

	var getFeedStatusActualData map[string]any
	if err := json.Unmarshal([]byte(body), &getFeedStatusActualData); err != nil {
		t.Errorf("JSONのパースに失敗: %v", err)
	}
	// delete lastUpdated field
	delete(getFeedStatusActualData, "lastUpdated")

	getFeedStatusExpectedData := map[string]any{
		"status": map[string]any{
			"feedId":     "test-feed",
			"lastStatus": "active",
		},
	}
	if statusMap, ok := getFeedStatusActualData["status"].(map[string]any); ok {
		delete(statusMap, "lastUpdated")
	}

	getFeedStatusExpectedJSON, _ := json.Marshal(getFeedStatusExpectedData)
	getFeedStatusActualJSON, _ := json.Marshal(getFeedStatusActualData)

	if string(getFeedStatusActualJSON) != string(getFeedStatusExpectedJSON) {
		t.Errorf("Expected value does not match actual value.\nExpected: %s\nActual: %s", string(getFeedStatusExpectedJSON), string(getFeedStatusActualJSON))
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	body = recorder.Body.String()

	//// test update feed status
	updateStatusBody := map[string]any{
		"status": "inactive",
	}
	updateStatusJSON, _ := json.Marshal(updateStatusBody)
	req, _ = http.NewRequest("PUT", "/api/feed/test-feed/status", bytes.NewBuffer(updateStatusJSON))
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// check status is updated
	req, _ = http.NewRequest("GET", "/api/feed/test-feed/status", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	var updatedStatusData map[string]any
	json.Unmarshal(recorder.Body.Bytes(), &updatedStatusData)

	statusMap, _ := updatedStatusData["status"].(map[string]any)
	if statusMap["lastStatus"] != "inactive" {
		t.Errorf("Expected status to be 'inactive', but got '%v'", statusMap["lastStatus"])
	}

	//// test unregister feed
	req, _ = http.NewRequest("DELETE", "/api/feed/test-feed", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// check feed is deleted
	_, exists := fs.GetFeedInfo("test-feed")
	if exists {
		t.Errorf("Expected feed to be deleted, but it exists")
	}
}

func TestFeedAPI_ListProjectionOps_ReturnsFailedEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-api.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-failed",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/failed",
		Target:      "gyoka",
		Operation:   "add",
		MutationID:  "m-failed",
		SubjectKey:  "feed-failed:post-1",
		OpKey:       "m-failed:add:post-1",
		PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() failed entry error = %v", err)
	}
	claimedFailed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() failed entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() failed entry ok = false, want true")
	}
	if err := repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
		ID:          claimedFailed.ID,
		LastError:   "temporary failure",
		NextRetryAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkRetryableFailure() error = %v", err)
	}

	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-dead",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
		Target:      "gyoka",
		Operation:   "delete",
		MutationID:  "m-dead",
		SubjectKey:  "feed-dead:post-2",
		OpKey:       "m-dead:delete:post-2",
		PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() dead entry error = %v", err)
	}
	claimedDead, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() dead entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() dead entry ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: claimedDead.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-completed",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
		Target:      "gyoka",
		Operation:   "add",
		MutationID:  "m-completed",
		SubjectKey:  "feed-completed:post-3",
		OpKey:       "m-completed:add:post-3",
		PayloadJSON: `{"uri":"at://did:plc:user3/app.bsky.feed.post/post3"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() completed entry error = %v", err)
	}
	claimedCompleted, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() completed entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() completed entry ok = false, want true")
	}
	if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: claimedCompleted.ID}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = repo
	router := gin.Default()
	router.GET("/api/admin/projection/ops", api.ListProjectionOps)

	req, _ := http.NewRequest("GET", "/api/admin/projection/ops?status=failed&target=gyoka&limit=10", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(response.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(response.Entries))
	}
	if got := response.Entries[0]["status"]; got != "failed" {
		t.Fatalf("entries[0].status = %v, want failed", got)
	}
	if got := response.Entries[0]["feedId"]; got != "feed-failed" {
		t.Fatalf("entries[0].feedId = %v, want feed-failed", got)
	}
	if got := response.Entries[0]["lastError"]; got != "temporary failure" {
		t.Fatalf("entries[0].lastError = %v, want temporary failure", got)
	}
}

func TestFeedAPI_RetryProjectionOp_RequeuesDeadEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-api-retry.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-dead",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
		Target:      "gyoka",
		Operation:   "delete",
		MutationID:  "m-dead",
		SubjectKey:  "feed-dead:post-2",
		OpKey:       "m-dead:delete:post-2",
		PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: claimed.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = repo
	router := gin.Default()
	router.POST("/api/admin/projection/ops/:id/retry", api.RetryProjectionOp)

	req, _ := http.NewRequest("POST", "/api/admin/projection/ops/"+strconv.FormatInt(claimed.ID, 10)+"/retry", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	entries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "pending", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("pending entries len = %d, want 1", len(entries))
	}
	if entries[0].LastError != "" {
		t.Fatalf("pending LastError = %q, want empty", entries[0].LastError)
	}
	if entries[0].NextRetryAt != "" {
		t.Fatalf("pending NextRetryAt = %q, want empty", entries[0].NextRetryAt)
	}
}

func TestFeedAPI_DeleteProjectionOp_RemovesDeadEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-api-delete.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
		FeedID:      "feed-dead",
		FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
		Target:      "gyoka",
		Operation:   "delete",
		MutationID:  "m-dead",
		SubjectKey:  "feed-dead:post-2",
		OpKey:       "m-dead:delete:post-2",
		PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
		Status:      "pending",
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: claimed.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = repo
	router := gin.Default()
	router.DELETE("/api/admin/projection/ops/:id", api.DeleteProjectionOp)

	req, _ := http.NewRequest("DELETE", "/api/admin/projection/ops/"+strconv.FormatInt(claimed.ID, 10), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	entries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "dead", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("dead entries len = %d, want 0", len(entries))
	}
}

func TestFeedAPI_PurgeCompletedProjectionOps_RemovesLimitedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-api-purge.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	for index := 0; index < 3; index++ {
		if err := repo.Enqueue(ctx, projectionrepo.EnqueueParams{
			FeedID:      "feed-completed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-completed-" + strconv.Itoa(index),
			SubjectKey:  "feed-completed:post-" + strconv.Itoa(index),
			OpKey:       "m-completed:add:post-" + strconv.Itoa(index),
			PayloadJSON: `{"uri":"at://did:plc:user3/app.bsky.feed.post/post3"}`,
			Status:      "pending",
		}); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		claimed, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
		if err != nil {
			t.Fatalf("ClaimNextPending() error = %v", err)
		}
		if !ok {
			t.Fatal("ClaimNextPending() ok = false, want true")
		}
		if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: claimed.ID}); err != nil {
			t.Fatalf("MarkCompleted() error = %v", err)
		}
	}

	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = repo
	router := gin.Default()
	router.POST("/api/admin/projection/ops/purge-completed", api.PurgeCompletedProjectionOps)

	reqBody := createJSONBody(t, map[string]any{"target": "gyoka", "limit": 2})
	req, _ := http.NewRequest("POST", "/api/admin/projection/ops/purge-completed", reqBody)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	entries, err := repo.ListByStatus(ctx, projectionrepo.ListByStatusParams{Target: "gyoka", Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("completed entries len = %d, want 1", len(entries))
	}
}

func TestFeedAPI_GetProjectionOpSummary_ReturnsCountsIncludingFailed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	db, err := storesqlite.Open(ctx, storesqlite.Options{
		Path:         filepath.Join(t.TempDir(), "projection-api-summary.db"),
		SyncMode:     "NORMAL",
		BusyTimeout:  100 * time.Millisecond,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storesqlite.Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := projectionsqlite.NewOutboxRepository(db)
	entries := []projectionrepo.EnqueueParams{
		{
			FeedID:      "feed-failed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/failed",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-failed",
			SubjectKey:  "feed-failed:post-1",
			OpKey:       "m-failed:add:post-1",
			PayloadJSON: `{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-ready",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/ready",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-ready",
			SubjectKey:  "feed-ready:post-2",
			OpKey:       "m-ready:add:post-2",
			PayloadJSON: `{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-completed",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/completed",
			Target:      "gyoka",
			Operation:   "add",
			MutationID:  "m-completed",
			SubjectKey:  "feed-completed:post-3",
			OpKey:       "m-completed:add:post-3",
			PayloadJSON: `{"uri":"at://did:plc:user3/app.bsky.feed.post/post3"}`,
			Status:      "pending",
		},
		{
			FeedID:      "feed-dead",
			FeedURI:     "at://did:plc:test/app.bsky.feed.generator/dead",
			Target:      "gyoka",
			Operation:   "delete",
			MutationID:  "m-dead",
			SubjectKey:  "feed-dead:post-4",
			OpKey:       "m-dead:delete:post-4",
			PayloadJSON: `{"uri":"at://did:plc:user4/app.bsky.feed.post/post4"}`,
			Status:      "pending",
		},
	}
	for _, entry := range entries {
		if err := repo.Enqueue(ctx, entry); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", entry.OpKey, err)
		}
	}

	failedEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() failed entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() failed entry ok = false, want true")
	}
	if err := repo.MarkRetryableFailure(ctx, projectionrepo.MarkRetryableFailureParams{
		ID:          failedEntry.ID,
		LastError:   "temporary failure",
		NextRetryAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkRetryableFailure() error = %v", err)
	}

	completedEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() completed entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() completed entry ok = false, want true")
	}
	if err := repo.MarkCompleted(ctx, projectionrepo.MarkCompletedParams{ID: completedEntry.ID}); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}

	deadEntry, ok, err := repo.ClaimNextPending(ctx, projectionrepo.ClaimNextPendingParams{Target: "gyoka"})
	if err != nil {
		t.Fatalf("ClaimNextPending() dead entry error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextPending() dead entry ok = false, want true")
	}
	if err := repo.MarkDead(ctx, projectionrepo.MarkDeadParams{ID: deadEntry.ID, LastError: "unsupported operation"}); err != nil {
		t.Fatalf("MarkDead() error = %v", err)
	}

	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = repo
	router := gin.Default()
	router.GET("/api/admin/projection/ops/summary", api.GetProjectionOpSummary)

	req, _ := http.NewRequest("GET", "/api/admin/projection/ops/summary?target=gyoka", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Target string         `json:"target"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Target != "gyoka" {
		t.Fatalf("target = %s, want gyoka", response.Target)
	}
	if got := response.Counts["pending"]; got != 2 {
		t.Fatalf("pending count = %d, want 2", got)
	}
	if got := response.Counts["failed"]; got != 1 {
		t.Fatalf("failed count = %d, want 1", got)
	}
	if got := response.Counts["completed"]; got != 1 {
		t.Fatalf("completed count = %d, want 1", got)
	}
	if got := response.Counts["dead"]; got != 1 {
		t.Fatalf("dead count = %d, want 1", got)
	}
	if got := response.Counts["processing"]; got != 0 {
		t.Fatalf("processing count = %d, want 0", got)
	}
}

func TestFeedAPI_ListProjectionOps_RejectsInvalidStatusQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = noopOutboxRepository{}
	router := gin.Default()
	router.GET("/api/admin/projection/ops", api.ListProjectionOps)

	req, _ := http.NewRequest("GET", "/api/admin/projection/ops?status=invalid", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestFeedAPI_RetryProjectionOp_RejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = noopOutboxRepository{}
	router := gin.Default()
	router.POST("/api/admin/projection/ops/:id/retry", api.RetryProjectionOp)

	req, _ := http.NewRequest("POST", "/api/admin/projection/ops/not-a-number/retry", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestFeedAPI_PurgeCompletedProjectionOps_RejectsMalformedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewFeedApiHandler(nil)
	api.ProjectionOutbox = noopOutboxRepository{}
	router := gin.Default()
	router.POST("/api/admin/projection/ops/purge-completed", api.PurgeCompletedProjectionOps)

	req, _ := http.NewRequest("POST", "/api/admin/projection/ops/purge-completed", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

type noopOutboxRepository struct{}

func (noopOutboxRepository) Enqueue(ctx context.Context, params projectionrepo.EnqueueParams) error {
	return errors.New("unexpected Enqueue call")
}

func (noopOutboxRepository) ListByStatus(ctx context.Context, params projectionrepo.ListByStatusParams) ([]projectionrepo.Entry, error) {
	return nil, errors.New("unexpected ListByStatus call")
}

func (noopOutboxRepository) CountByStatus(ctx context.Context, params projectionrepo.CountByStatusParams) ([]projectionrepo.StatusCount, error) {
	return nil, errors.New("unexpected CountByStatus call")
}

func (noopOutboxRepository) ClaimNextPending(ctx context.Context, params projectionrepo.ClaimNextPendingParams) (projectionrepo.Entry, bool, error) {
	return projectionrepo.Entry{}, false, errors.New("unexpected ClaimNextPending call")
}

func (noopOutboxRepository) MarkCompleted(ctx context.Context, params projectionrepo.MarkCompletedParams) error {
	return errors.New("unexpected MarkCompleted call")
}

func (noopOutboxRepository) MarkRetryableFailure(ctx context.Context, params projectionrepo.MarkRetryableFailureParams) error {
	return errors.New("unexpected MarkRetryableFailure call")
}

func (noopOutboxRepository) MarkDead(ctx context.Context, params projectionrepo.MarkDeadParams) error {
	return errors.New("unexpected MarkDead call")
}

func (noopOutboxRepository) Requeue(ctx context.Context, params projectionrepo.RequeueParams) error {
	return errors.New("unexpected Requeue call")
}

func (noopOutboxRepository) Delete(ctx context.Context, params projectionrepo.DeleteParams) error {
	return errors.New("unexpected Delete call")
}

func (noopOutboxRepository) PurgeCompleted(ctx context.Context, params projectionrepo.PurgeCompletedParams) (int64, error) {
	return 0, errors.New("unexpected PurgeCompleted call")
}

var testConfig = `logic:
    blocks:
      #日本語設定のないものは除外
      - type: remove
        options:
          subject: language
          language: ja
          operator: '!='
store:
  trimAt: 24
  trimRemain: 20
detailedLog: false`

func TestAPIHandler_GetConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fs, tempDir, err := createFeedService(t)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}
	api := NewFeedApiHandler(fs)

	// create config file
	configFile := filepath.Join(tempDir, "config", "test-config.yaml")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte(testConfig), 0644)

	router := gin.Default()
	router.POST("/api/feed/:feedid", api.RegisterFeed)
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		GET("/config", api.GetConfig)

	// register feed
	req, _ := http.NewRequest("POST", "/api/feed/test-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, map[string]any{
		"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		"configFile":    "test-config.yaml",
		"inactiveStart": false,
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	// get config
	req, _ = http.NewRequest("GET", "/api/feed/test-feed/config", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	var configData map[string]any
	json.Unmarshal(recorder.Body.Bytes(), &configData)

	// check config data
	logic, ok := configData["logic"].(map[string]any)
	if !ok {
		t.Errorf("Expected logic configuration, but it was missing or invalid")
	}

	blocks, ok := logic["blocks"].([]any)
	if !ok || len(blocks) == 0 {
		t.Errorf("Expected blocks in logic configuration, but they were missing or invalid")
	}

	block := blocks[0].(map[string]any)
	if block["type"] != "remove" {
		t.Errorf("Expected block type 'remove', but got '%v'", block["type"])
	}

	options := block["options"].(map[string]any)
	if options["subject"] != "language" || options["language"] != "ja" || options["operator"] != "!=" {
		t.Errorf("Block options do not match expected values: %v", options)
	}

	store, ok := configData["store"].(map[string]any)
	if !ok {
		t.Errorf("Expected store configuration, but it was missing or invalid")
	}

	if store["trimAt"] != float64(24) || store["trimRemain"] != float64(20) {
		t.Errorf("Store configuration does not match expected values: %v", store)
	}

	detailedLog, ok := configData["detailedLog"].(bool)
	if !ok || detailedLog != false {
		t.Errorf("Expected detailedLog to be false, but got %v", detailedLog)
	}
}

func TestAPIHandler_PostOperations(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fs, tempDir, err := createFeedService(t)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}
	api := NewFeedApiHandler(fs)

	// create config file
	configFile := filepath.Join(tempDir, "config", "test-config.yaml")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte(testConfig), 0644)

	router := gin.Default()
	router.POST("/api2/feed/:feedid", api.RegisterFeed)
	router.Group("/api2/feed/:feedid").Use(api.ValidateFeedId()).
		POST("/post/:did/:rkey", api.AddPost).
		GET("/post", api.GetAllPosts).
		GET("/post/:did", api.GetPostsByDid).
		GET("/post/:did/:rkey", api.GetPostByRkey).
		DELETE("/post/:did/:rkey", api.DeletePost)

	// register feed
	req, _ := http.NewRequest("POST", "/api2/feed/test-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, map[string]any{
		"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		"configFile":    "test-config.yaml",
		"inactiveStart": false,
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Errorf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
		t.Errorf("body: %s", recorder.Body.String())
		return
	}

	// add post
	testDid := "did:plc:test123"
	testRkey := "testrkey456"
	testUri := "at://" + testDid + "/app.bsky.feed.post/" + testRkey
	postData := struct {
		CID       string   `json:"cid"`
		IndexedAt string   `json:"indexedAt"`
		Langs     []string `json:"langs,omitempty"`
	}{
		CID:       "bafyreia1",
		IndexedAt: "2024-01-01T00:00:00Z",
		Langs:     []string{"en", "jp"},
	}

	req, _ = http.NewRequest("POST", "/api2/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
	req.Header.Set("Content-Type", "application/json")
	jsonData, _ := json.Marshal(postData)
	body := bytes.NewBuffer(jsonData)
	req.Body = io.NopCloser(body)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// get all posts
	req, _ = http.NewRequest("GET", "/api2/feed/test-feed/post", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	var getAllPostsResp GetAllPostsResponse
	err = json.Unmarshal(recorder.Body.Bytes(), &getAllPostsResp)
	if err != nil {
		t.Fatalf("JSONのアンマーシャルに失敗しました: %v", err)
	}
	if len(getAllPostsResp.Posts) != 1 {
		t.Errorf("Expected 1 post, but got %d", len(getAllPostsResp.Posts))
	}

	// get posts by DID
	req, _ = http.NewRequest("GET", "/api2/feed/test-feed/post/"+testDid, nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	var didPosts GetPostsByDidResponse
	err = json.Unmarshal(recorder.Body.Bytes(), &didPosts)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(didPosts.Posts) != 1 {
		t.Errorf("Expected 1 post for DID, but got %d", len(didPosts.Posts))
	}

	// get post by RKey
	req, _ = http.NewRequest("GET", "/api2/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	var post GetPostByRkeyResponse
	err = json.Unmarshal(recorder.Body.Bytes(), &post)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if string(post.Post.Uri) != testUri {
		t.Errorf("Expected to get a post by rkey, but got %s", string(post.Post.Uri))
	}

	// delete post
	req, _ = http.NewRequest("DELETE", "/api2/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	var deletePostResponse DeletePostByRkeyResponse
	err = json.Unmarshal(recorder.Body.Bytes(), &deletePostResponse)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if deletePostResponse.Message != "post deleted successfully" {
		t.Errorf("Expected message to be 'post deleted successfully', but got %s", deletePostResponse.Message)
	}
	if string(deletePostResponse.Deleted.Uri) != testUri {
		t.Errorf("Expected to delete a post, but got %s", deletePostResponse.Deleted.Uri)
	}
	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// check post is deleted
	req, _ = http.NewRequest("GET", "/api2/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, but got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestAPIHandler_PostOperationsUseCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fs, tempDir, err := createFeedService(t)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}
	configFile := filepath.Join(tempDir, "config", "test-config.yaml")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte(testConfig), 0644)

	registerAPI := NewFeedApiHandler(fs)
	spy := &spyPostMutationCoordinator{}
	api := NewFeedApiHandler(fs)
	api.MutationCoordinator = spy

	router := gin.Default()
	router.POST("/api/feed/:feedid", registerAPI.RegisterFeed)
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		POST("/post/:did/:rkey", api.AddPost).
		DELETE("/post/:did/:rkey", api.DeletePost).
		GET("/post/:did/:rkey", api.GetPostByRkey)

	req, _ := http.NewRequest("POST", "/api/feed/test-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, map[string]any{
		"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		"configFile":    "test-config.yaml",
		"inactiveStart": false,
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
	}

	t.Run("add uses coordinator and updates cache on success", func(t *testing.T) {
		spy.addPostErr = nil
		spy.addPostCalls = 0
		spy.lastAddParams = AddPostParams{}

		testDid := "did:plc:test123"
		testRkey := "testrkey456"
		postData := map[string]any{
			"cid":       "bafyreia1",
			"indexedAt": "2024-01-01T00:00:00Z",
			"langs":     []string{"en", "ja"},
		}

		req, _ := http.NewRequest("POST", "/api/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = io.NopCloser(createJSONBody(t, postData))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}
		if spy.addPostCalls != 1 {
			t.Fatalf("coordinator AddPost() calls = %d, want 1", spy.addPostCalls)
		}
		if spy.lastAddParams.FeedID != "test-feed" {
			t.Fatalf("coordinator FeedID = %q, want test-feed", spy.lastAddParams.FeedID)
		}
		if spy.lastAddParams.TrimAt != 24 || spy.lastAddParams.TrimRemain != 20 {
			t.Fatalf("coordinator trim params = (%d, %d), want (24, 20)", spy.lastAddParams.TrimAt, spy.lastAddParams.TrimRemain)
		}

		req, _ = http.NewRequest("GET", "/api/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}
	})

	t.Run("delete coordinator failure leaves cache untouched", func(t *testing.T) {
		fi, _ := fs.GetFeedInfo("test-feed")
		testDid := "did:plc:testdelete"
		testRkey := "rkey1"
		now := time.Now().UTC()
		if err := fi.Feed.AddPost(testDid, testRkey, "cid-delete", now, []string{"ja"}); err != nil {
			t.Fatalf("Feed.AddPost() error = %v", err)
		}

		spy.deletePostErr = errors.New("boom")
		spy.deletePostCalls = 0
		spy.lastDeleteParams = DeletePostParams{}

		req, _ := http.NewRequest("DELETE", "/api/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)

		if response.Code != http.StatusInternalServerError {
			t.Fatalf("Expected status code %d, but got %d", http.StatusInternalServerError, response.Code)
		}
		if spy.deletePostCalls != 1 {
			t.Fatalf("coordinator DeletePost() calls = %d, want 1", spy.deletePostCalls)
		}
		if spy.lastDeleteParams.Post.Uri != types.PostUri("at://"+testDid+"/app.bsky.feed.post/"+testRkey) {
			t.Fatalf("coordinator Post URI = %q", spy.lastDeleteParams.Post.Uri)
		}

		post, exists := fi.Feed.GetPost(testDid, testRkey)
		if !exists {
			t.Fatal("expected post to remain in cache after coordinator failure")
		}
		if post.Uri != types.PostUri("at://"+testDid+"/app.bsky.feed.post/"+testRkey) {
			t.Fatalf("remaining post URI = %q", post.Uri)
		}
	})
}

func TestAPIHandler_DeletePostByDidUsesCoordinator(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fs, tempDir, err := createFeedService(t)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}
	configFile := filepath.Join(tempDir, "config", "test-config.yaml")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte(testConfig), 0644)

	registerAPI := NewFeedApiHandler(fs)
	spy := &spyPostMutationCoordinator{}
	api := NewFeedApiHandler(fs)
	api.MutationCoordinator = spy

	router := gin.Default()
	router.POST("/api/feed/:feedid", registerAPI.RegisterFeed)
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		DELETE("/post/:did", api.DeletePostByDid).
		GET("/post/:did", api.GetPostsByDid)

	req, _ := http.NewRequest("POST", "/api/feed/test-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, map[string]any{
		"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		"configFile":    "test-config.yaml",
		"inactiveStart": false,
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
	}

	seedPosts := func(t *testing.T, did string, rkeys ...string) {
		t.Helper()
		fi, _ := fs.GetFeedInfo("test-feed")
		for index, rkey := range rkeys {
			if err := fi.Feed.AddPost(did, rkey, "cid-"+rkey, time.Date(2026, 5, 11, 8, 0, index, 0, time.UTC), []string{"ja"}); err != nil {
				t.Fatalf("Feed.AddPost(%s) error = %v", rkey, err)
			}
		}
	}

	t.Run("success deletes all matching posts through coordinator", func(t *testing.T) {
		const did = "did:plc:deleteall"
		seedPosts(t, did, "post1", "post2")
		spy.deletePostErr = nil
		spy.deletePostCalls = 0

		req, _ := http.NewRequest("DELETE", "/api/feed/test-feed/post/"+did, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}
		if spy.deletePostCalls != 2 {
			t.Fatalf("coordinator DeletePost() calls = %d, want 2", spy.deletePostCalls)
		}

		req, _ = http.NewRequest("GET", "/api/feed/test-feed/post/"+did, nil)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		var posts GetPostsByDidResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &posts); err != nil {
			t.Fatalf("unmarshal posts error = %v", err)
		}
		if len(posts.Posts) != 0 {
			t.Fatalf("remaining posts = %d, want 0", len(posts.Posts))
		}
	})

	t.Run("coordinator failure leaves matching posts in cache", func(t *testing.T) {
		const did = "did:plc:deletefail"
		seedPosts(t, did, "post1", "post2")
		spy.deletePostErr = errors.New("boom")
		spy.deletePostCalls = 0

		req, _ := http.NewRequest("DELETE", "/api/feed/test-feed/post/"+did, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("Expected status code %d, but got %d", http.StatusInternalServerError, recorder.Code)
		}
		if spy.deletePostCalls != 1 {
			t.Fatalf("coordinator DeletePost() calls = %d, want 1", spy.deletePostCalls)
		}

		req, _ = http.NewRequest("GET", "/api/feed/test-feed/post/"+did, nil)
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, req)

		var posts GetPostsByDidResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &posts); err != nil {
			t.Fatalf("unmarshal posts error = %v", err)
		}
		if len(posts.Posts) != 2 {
			t.Fatalf("remaining posts = %d, want 2", len(posts.Posts))
		}
	})
}

func TestAPIHandler_ReloadAndClearFeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	configFile := filepath.Join(configDir, "test-config.yaml")
	if err := os.WriteFile(configFile, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create feed definition provider: %v", err)
	}
	fs, err := NewFeedService(configDir, dataDir, provider, logger)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}

	ctx := context.Background()
	db, coordinator, err := openSQLiteMutationCoordinator(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteMutationCoordinator() error = %v", err)
	}
	defer db.Close()
	fs.SetStoreLoader(newSQLitePostLoader(db))
	fs.SetMutationCoordinator(coordinator)

	definition := FeedDefinition{
		ID:         "test-feed",
		URI:        "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		ConfigFile: "test-config.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}

	repo := storesqlite.NewFeedRepository(db)
	seedPost := types.Post{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:test123/app.bsky.feed.post/testrkey456"),
		Cid:       "reloadfeed",
		IndexedAt: "2024-01-01T00:00:00Z",
	}
	if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: definition.ID, Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}
	if err := fs.CreateFeed(ctx, definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}

	api := NewFeedApiHandler(fs)

	router := gin.Default()
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		POST("/reload", api.ReloadFeed).
		POST("/clear", api.ClearFeed).
		GET("/post", api.GetAllPosts)
	recorder := httptest.NewRecorder()

	// フィードをリロード
	req, _ := http.NewRequest("POST", "/api/feed/test-feed/reload", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// フィードをクリア
	req, _ = http.NewRequest("POST", "/api/feed/test-feed/clear", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}

	// 投稿が削除されたことを確認
	req, _ = http.NewRequest("GET", "/api/feed/test-feed/post", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	var posts GetAllPostsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &posts); err != nil {
		t.Fatalf("Failed to unmarshal posts response: %v", err)
	}
	if len(posts.Posts) != 0 {
		t.Errorf("Expected 0 posts after clear and reload, but got %d", len(posts.Posts))
	}

	persistedPosts, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: definition.ID})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(persistedPosts) != 0 {
		t.Errorf("Expected 0 persisted posts after clear and reload, but got %d", len(persistedPosts))
	}
}

func TestAPIHandler_ClearFeed_PreservesStateOnCoordinatorFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	configFile := filepath.Join(configDir, "test-config.yaml")
	if err := os.WriteFile(configFile, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create feed definition provider: %v", err)
	}
	fs, err := NewFeedService(configDir, dataDir, provider, logger)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}

	ctx := context.Background()
	db, _, err := openSQLiteMutationCoordinator(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteMutationCoordinator() error = %v", err)
	}
	defer db.Close()
	fs.SetStoreLoader(newSQLitePostLoader(db))
	fs.SetMutationCoordinator(&spyPostMutationCoordinator{deletePostErr: errors.New("boom")})

	definition := FeedDefinition{
		ID:         "test-feed",
		URI:        "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		ConfigFile: "test-config.yaml",
	}
	if err := provider.AddFeedDefinition(definition); err != nil {
		t.Fatalf("AddFeedDefinition() error = %v", err)
	}

	repo := storesqlite.NewFeedRepository(db)
	seedPost := types.Post{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:test123/app.bsky.feed.post/testrkey456"),
		Cid:       "reloadfeed",
		IndexedAt: "2024-01-01T00:00:00Z",
	}
	if err := repo.PutPost(ctx, storerepo.PutPostParams{FeedID: definition.ID, Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}
	if err := fs.CreateFeed(ctx, definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}

	api := NewFeedApiHandler(fs)
	router := gin.Default()
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		POST("/clear", api.ClearFeed).
		GET("/post", api.GetAllPosts)

	req, _ := http.NewRequest("POST", "/api/feed/test-feed/clear", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("Expected status code %d, but got %d", http.StatusInternalServerError, recorder.Code)
	}

	req, _ = http.NewRequest("GET", "/api/feed/test-feed/post", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
	}
	var posts GetAllPostsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &posts); err != nil {
		t.Fatalf("Failed to unmarshal posts response: %v", err)
	}
	if len(posts.Posts) != 1 {
		t.Fatalf("Expected 1 post to remain after failed clear, but got %d", len(posts.Posts))
	}

	persistedPosts, err := repo.ListPosts(ctx, storerepo.ListPostsParams{FeedID: definition.ID})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(persistedPosts) != 1 {
		t.Fatalf("Expected 1 persisted post to remain after failed clear, but got %d", len(persistedPosts))
	}
}

func TestAPIHandler_AddAcceptedPost_WaitsForClearFeedOnSameFeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	configFile := filepath.Join(configDir, "test-config.yaml")
	if err := os.WriteFile(configFile, []byte(testConfig), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	ctx := context.Background()
	persistence, err := openSQLiteRuntimePersistence(ctx, dataDir)
	if err != nil {
		t.Fatalf("openSQLiteRuntimePersistence() error = %v", err)
	}
	defer persistence.Close()

	fs, err := NewFeedService(configDir, dataDir, nil, logger)
	if err != nil {
		t.Fatalf("NewFeedService() error = %v", err)
	}
	fs.SetStoreLoader(persistence.postLoader)
	blockingCoordinator := &blockingDeleteCoordinator{
		inner:         persistence.mutationCoordinator,
		deleteStarted: make(chan struct{}),
		deleteRelease: make(chan struct{}),
	}
	fs.SetMutationCoordinator(blockingCoordinator)

	definition := FeedDefinition{
		ID:         "test-feed",
		URI:        "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		ConfigFile: "test-config.yaml",
	}
	seedPost := types.Post{
		Feed:      types.FeedUri(definition.URI),
		Uri:       types.PostUri("at://did:plc:seed/app.bsky.feed.post/post1"),
		Cid:       "cid-seed",
		IndexedAt: "2026-05-11T10:00:00Z",
	}
	if err := storesqlite.NewFeedRepository(persistence.mutationDB).PutPost(ctx, storerepo.PutPostParams{FeedID: definition.ID, Post: seedPost}); err != nil {
		t.Fatalf("PutPost() error = %v", err)
	}
	if err := fs.CreateFeed(ctx, definition, FeedStatusActive); err != nil {
		t.Fatalf("CreateFeed() error = %v", err)
	}
	info, exists := fs.GetFeedInfo(definition.ID)
	if !exists {
		t.Fatal("expected feed info to exist")
	}

	api := NewFeedApiHandler(fs)
	api.MutationCoordinator = blockingCoordinator

	clearErrCh := make(chan error, 1)
	go func() {
		clearErrCh <- fs.ClearFeed(ctx, definition.ID)
	}()

	select {
	case <-blockingCoordinator.deleteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ClearFeed to start deleting persisted posts")
	}

	addErrCh := make(chan error, 1)
	go func() {
		addErrCh <- api.addAcceptedPost(ctx, definition.ID, info.Feed, "did:plc:new", "post2", "cid-new", time.Date(2026, 5, 11, 10, 1, 0, 0, time.UTC), []string{"en"})
	}()

	select {
	case err := <-addErrCh:
		t.Fatalf("addAcceptedPost() completed before ClearFeed released its gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(blockingCoordinator.deleteRelease)
	if err := <-clearErrCh; err != nil {
		t.Fatalf("ClearFeed() error = %v", err)
	}
	if err := <-addErrCh; err != nil {
		t.Fatalf("addAcceptedPost() error = %v", err)
	}

	persistedPosts, err := storesqlite.NewFeedRepository(persistence.loaderDB).ListPosts(ctx, storerepo.ListPostsParams{FeedID: definition.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListPosts() error = %v", err)
	}
	if len(persistedPosts) != 1 {
		t.Fatalf("persisted posts len = %d, want 1", len(persistedPosts))
	}
	if persistedPosts[0].Uri != types.PostUri("at://did:plc:new/app.bsky.feed.post/post2") {
		t.Fatalf("persisted post uri = %s, want new post", persistedPosts[0].Uri)
	}
	cachePosts := info.Feed.ListPost("")
	if len(cachePosts) != 1 {
		t.Fatalf("cache posts len = %d, want 1", len(cachePosts))
	}
	if cachePosts[0].Uri != persistedPosts[0].Uri {
		t.Fatalf("cache post uri = %s, want %s", cachePosts[0].Uri, persistedPosts[0].Uri)
	}
}
