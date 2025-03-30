package subscriber

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nus25/yuge/feed/store/editor"
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
	e, err := editor.NewFileEditor(dataDir, logger)
	if err != nil {
		t.Fatalf("Failed to create editor: %v", err)
	}
	dp, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create feed definition provider: %v", err)
	}
	fs, err := NewFeedService(configDir, dataDir, dp, e, logger)

	return fs, tempDir, err
}

func createJSONBody(t *testing.T, data map[string]interface{}) io.Reader {
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
	req.Body = io.NopCloser(createJSONBody(t, map[string]interface{}{
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
	var actualData []map[string]interface{}
	if err := json.Unmarshal([]byte(body), &actualData); err != nil {
		t.Errorf("JSONのパースに失敗: %v", err)
	}

	// lastUpdatedフィールドを無視するために削除
	if len(actualData) > 0 && actualData[0]["status"] != nil {
		if status, ok := actualData[0]["status"].(map[string]interface{}); ok {
			delete(status, "lastUpdated")
		}
	}

	// 期待値
	expectedData := []map[string]interface{}{
		{
			"id": "test-feed",
			"definition": map[string]interface{}{
				"id":            "test-feed",
				"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
				"configFile":    "test-config.yaml",
				"inactiveStart": "false",
			},
			"status": map[string]interface{}{
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
		if status, ok := actualData[0]["status"].(map[string]interface{}); ok {
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
	var getFeedInfoActualData map[string]interface{}
	if err := json.Unmarshal([]byte(body), &getFeedInfoActualData); err != nil {
		t.Errorf("JSONのパースに失敗: %v", err)
	}

	// lastpdatedフィールドを無視するために削除
	if status, ok := getFeedInfoActualData["status"].(map[string]interface{}); ok {
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
	expectedStatus := map[string]interface{}{
		"feedId":     "test-feed",
		"lastStatus": "active",
	}
	actualStatus, ok := getFeedInfoActualData["status"].(map[string]interface{})
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
	expectedConfig := map[string]interface{}{
		"detailedLog": true,
	}
	actualConfig, ok := getFeedInfoActualData["config"].(map[string]interface{})
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
	actualMetrics, ok := getFeedInfoActualData["metrics"].(map[string]interface{})
	if !ok {
		t.Error("Metrics field not found or invalid type")
	} else {
		expectedMetrics := []map[string]interface{}{
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

	var getFeedStatusActualData map[string]interface{}
	if err := json.Unmarshal([]byte(body), &getFeedStatusActualData); err != nil {
		t.Errorf("JSONのパースに失敗: %v", err)
	}
	// delete lastUpdated field
	if _, ok := getFeedStatusActualData["lastUpdated"]; ok {
		delete(getFeedStatusActualData, "lastUpdated")
	}

	getFeedStatusExpectedData := map[string]interface{}{
		"status": map[string]interface{}{
			"feedId":     "test-feed",
			"lastStatus": "active",
		},
	}
	if statusMap, ok := getFeedStatusActualData["status"].(map[string]interface{}); ok {
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
	updateStatusBody := map[string]interface{}{
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

	var updatedStatusData map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &updatedStatusData)

	statusMap, _ := updatedStatusData["status"].(map[string]interface{})
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
	req.Body = io.NopCloser(createJSONBody(t, map[string]interface{}{
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

	var configData map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &configData)

	// check config data
	logic, ok := configData["logic"].(map[string]interface{})
	if !ok {
		t.Errorf("Expected logic configuration, but it was missing or invalid")
	}

	blocks, ok := logic["blocks"].([]interface{})
	if !ok || len(blocks) == 0 {
		t.Errorf("Expected blocks in logic configuration, but they were missing or invalid")
	}

	block := blocks[0].(map[string]interface{})
	if block["type"] != "remove" {
		t.Errorf("Expected block type 'remove', but got '%v'", block["type"])
	}

	options := block["options"].(map[string]interface{})
	if options["subject"] != "language" || options["language"] != "ja" || options["operator"] != "!=" {
		t.Errorf("Block options do not match expected values: %v", options)
	}

	store, ok := configData["store"].(map[string]interface{})
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
	req.Body = io.NopCloser(createJSONBody(t, map[string]interface{}{
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
	postData := map[string]interface{}{
		"cid":       "bafyreia1",
		"indexedAt": "2024-01-01T00:00:00Z",
	}

	req, _ = http.NewRequest("POST", "/api2/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, postData))
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

func TestAPIHandler_ReloadAndClearFeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fs, tempDir, err := createFeedService(t)
	defer os.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to create feed service: %v", err)
	}
	api := NewFeedApiHandler(fs)

	// 設定ファイルを作成
	configFile := filepath.Join(tempDir, "config", "test-config.yaml")
	os.MkdirAll(filepath.Dir(configFile), 0755)
	os.WriteFile(configFile, []byte(testConfig), 0644)

	router := gin.Default()
	router.POST("/api/feed/:feedid", api.RegisterFeed)
	router.Group("/api/feed/:feedid").Use(api.ValidateFeedId()).
		POST("/reload", api.ReloadFeed).
		POST("/clear", api.ClearFeed).
		POST("/post/:did/:rkey", api.AddPost).
		GET("/post", api.GetAllPosts)

	// フィードを登録
	req, _ := http.NewRequest("POST", "/api/feed/test-feed", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, map[string]interface{}{
		"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
		"configFile":    "test-config.yaml",
		"inactiveStart": false,
	}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Errorf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
		return
	}

	// 投稿を追加
	testDid := "did:plc:test123"
	testRkey := "testrkey456"
	postData := map[string]interface{}{
		"cid":       "reloadfeed",
		"indexedAt": "2024-01-01T00:00:00Z",
	}

	req, _ = http.NewRequest("POST", "/api/feed/test-feed/post/"+testDid+"/"+testRkey, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(createJSONBody(t, postData))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		return
	}

	// フィードをリロード
	req, _ = http.NewRequest("POST", "/api/feed/test-feed/reload", nil)
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

	var posts []interface{}
	json.Unmarshal(recorder.Body.Bytes(), &posts)
	if len(posts) != 0 {
		t.Errorf("Expected 0 posts after clear, but got %d", len(posts))
	}
}
