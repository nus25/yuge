package cli

import (
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/nus25/yuge/feed/config/types"
)

func TestPublishFeed_WithMockServer(t *testing.T) {
	// Create temporary avatar files
	tmpPngAvatar := createTestAvatarPngFile(t)
	defer os.Remove(tmpPngAvatar)

	tmpJpegAvatar := createTestAvatarJpegFile(t)
	defer os.Remove(tmpJpegAvatar)

	// Define test cases
	testCases := []struct {
		name                string
		avatarPath          string
		contentMode         string
		expectedContentMode string
		acceptsInteractions bool
		expectError         bool
	}{
		{
			name:                "PNG avatar with unspecified content mode",
			avatarPath:          tmpPngAvatar,
			contentMode:         "unspecified",
			expectedContentMode: CONTENT_MODE_UNSPECIFIED,
			acceptsInteractions: true,
			expectError:         false,
		},
		{
			name:                "JPEG avatar with video content mode",
			avatarPath:          tmpJpegAvatar,
			contentMode:         "video",
			expectedContentMode: CONTENT_MODE_VIDEO,
			acceptsInteractions: false,
			expectError:         false,
		},
		{
			name:                "PNG avatar with none content mode",
			avatarPath:          tmpPngAvatar,
			contentMode:         "",
			expectedContentMode: CONTENT_MODE_UNSPECIFIED,
			acceptsInteractions: true,
			expectError:         false,
		},
		{
			name:                "JPEG avatar without interactions",
			avatarPath:          tmpJpegAvatar,
			contentMode:         "unspecified",
			expectedContentMode: CONTENT_MODE_UNSPECIFIED,
			acceptsInteractions: false,
			expectError:         false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test data
			testIdentifier := "test.bsky.social"
			testPassword := "test-password"
			testServiceDID := "did:web:test.example.com"
			testDisplayName := "Test Feed"
			testDescription := "Test feed description"
			testRecordKey := "testfeed"

			// Track verification results
			var (
				sessionVerified   bool
				getRecordVerified bool
				uploadVerified    bool
				putRecordVerified bool
			)

			// Create mock ATProto server
			mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/xrpc/com.atproto.server.createSession":
					if handleCreateSessionWithVerification(t, w, r, testIdentifier, testPassword) {
						sessionVerified = true
					}
				case "/xrpc/com.atproto.server.deleteSession":
					handleDeleteSession(t, w, r)
				case "/xrpc/com.atproto.repo.getRecord":
					if handleGetRecordWithVerification(t, w, r, testRecordKey) {
						getRecordVerified = true
					}
				case "/xrpc/com.atproto.repo.uploadBlob":
					if handleUploadBlobWithVerification(t, w, r) {
						uploadVerified = true
					}
				case "/xrpc/com.atproto.repo.putRecord":
					if handlePutRecordWithVerification(t, w, r, testRecordKey, testServiceDID, testDisplayName, testDescription, tc.expectedContentMode, tc.acceptsInteractions) {
						putRecordVerified = true
					}
				default:
					t.Logf("Unhandled request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer mockServer.Close()

			// Create test parameters
			parsedIdentifier, _ := syntax.ParseAtIdentifier(testIdentifier)
			params := &FeedParams{
				RecordKey:           testRecordKey,
				Identifier:          *parsedIdentifier,
				Password:            testPassword,
				ServiceDID:          testServiceDID,
				DisplayName:         testDisplayName,
				Description:         testDescription,
				AvatarPath:          tc.avatarPath,
				ContentMode:         tc.contentMode,
				AcceptsInteractions: tc.acceptsInteractions,
				Host:                mockServer.URL,
				DryRun:              false,
			}

			// Load avatar
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
			avatarData, err := loadAvatar(context.Background(), params.AvatarPath, logger)
			if err != nil {
				t.Fatalf("Failed to load avatar: %v", err)
			}

			// Create session
			ctx := context.Background()
			client, err := createSessionWithClient(ctx, params.Host, params.Identifier.String(), params.Password, logger)
			if err != nil {
				t.Fatalf("Failed to create session: %v", err)
			}
			defer cleanupSessionWithClient(ctx, client, logger)

			// Publish feed
			var yugeConfig types.FeedConfig
			err = publishFeedWithClient(ctx, client, params, avatarData, yugeConfig, logger, true)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Fatalf("publishFeedWithClient failed: %v", err)
			}

			// Verify all endpoints were called correctly
			if !sessionVerified {
				t.Error("createSession was not verified correctly")
			}
			if !getRecordVerified {
				t.Error("getRecord was not verified correctly")
			}
			if !uploadVerified {
				t.Error("uploadBlob was not verified correctly")
			}
			if !putRecordVerified {
				t.Error("putRecord was not verified correctly")
			}
		})
	}
}

// handleCreateSessionWithVerification validates request parameters
func handleCreateSessionWithVerification(t *testing.T, w http.ResponseWriter, r *http.Request, expectedIdentifier, expectedPassword string) bool {
	if r.Method != http.MethodPost {
		t.Errorf("createSession: expected POST, got %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	// Parse and verify request body
	var req struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("createSession: failed to decode request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return false
	}

	// Verify identifier
	if req.Identifier != expectedIdentifier {
		t.Errorf("createSession: identifier mismatch - expected %s, got %s", expectedIdentifier, req.Identifier)
		return false
	}

	// Verify password
	if req.Password != expectedPassword {
		t.Errorf("createSession: password mismatch - expected %s, got %s", expectedPassword, req.Password)
		return false
	}

	// Return mock session response
	resp := map[string]interface{}{
		"did":            "did:plc:test123456789",
		"didDoc":         map[string]interface{}{},
		"handle":         "test.bsky.social",
		"email":          "test@example.com",
		"emailConfirmed": true,
		"accessJwt":      "mock-access-jwt-token",
		"refreshJwt":     "mock-refresh-jwt-token",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	t.Log("createSession: verification passed")
	return true
}

// handleGetRecordWithVerification validates request parameters
func handleGetRecordWithVerification(t *testing.T, w http.ResponseWriter, r *http.Request, expectedRkey string) bool {
	if r.Method != http.MethodGet {
		t.Errorf("getRecord: expected GET, got %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	// Verify query parameters
	query := r.URL.Query()
	repo := query.Get("repo")
	collection := query.Get("collection")
	rkey := query.Get("rkey")

	if repo == "" {
		t.Error("getRecord: repo parameter is missing")
		return false
	}

	if collection != COLLECTION_TYPE_FEED_GENERATOR {
		t.Errorf("getRecord: collection mismatch - expected %s, got %s", COLLECTION_TYPE_FEED_GENERATOR, collection)
		return false
	}

	if rkey != expectedRkey {
		t.Errorf("getRecord: rkey mismatch - expected %s, got %s", expectedRkey, rkey)
		return false
	}

	// Return 400 RecordNotFound to simulate no existing record
	resp := map[string]interface{}{
		"error":   "RecordNotFound",
		"message": "Record not found",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(resp)

	t.Log("getRecord: verification passed")
	return true
}

// handleUploadBlobWithVerification validates blob upload
func handleUploadBlobWithVerification(t *testing.T, w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		t.Errorf("uploadBlob: expected POST, got %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	// Verify Content-Type header
	contentType := r.Header.Get("Content-Type")
	if contentType != "*/*" {
		t.Errorf("uploadBlob: unexpected content type - got %s", contentType)
		return false
	}

	// Read and verify uploaded data
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("uploadBlob: failed to read body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return false
	}

	if len(data) == 0 {
		t.Error("uploadBlob: empty data received")
		return false
	}

	if len(data) > MAX_AVATAR_SIZE_BYTES {
		t.Errorf("uploadBlob: data exceeds size limit - got %d bytes", len(data))
		return false
	}

	// Return mock blob response
	resp := map[string]interface{}{
		"blob": map[string]interface{}{
			"$type":    "blob",
			"ref":      map[string]interface{}{"$link": "bafkreifuzpp32ljpc5uta4jh43svbolzaknmqlmqox55yngk7lp3tvvib5"},
			"mimeType": contentType,
			"size":     len(data),
		},
	}

	w.Header().Set("Content-Type", "*/*")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	t.Logf("uploadBlob: verification passed - %d bytes of %s", len(data), contentType)
	return true
}

// handlePutRecordWithVerification validates the feed record body
func handlePutRecordWithVerification(t *testing.T, w http.ResponseWriter, r *http.Request, expectedRkey, expectedServiceDID, expectedDisplayName, expectedDescription, expectedContentMode string, expectedAcceptsInteractions bool) bool {
	if r.Method != http.MethodPost {
		t.Errorf("putRecord: expected POST, got %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}

	// Parse request body
	var req atproto.RepoPutRecord_Input
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("putRecord: failed to read body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return false
	}

	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		t.Errorf("putRecord: failed to decode request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return false
	}

	// Verify rkey
	if req.Rkey != expectedRkey {
		t.Errorf("putRecord: rkey mismatch - expected %s, got %s", expectedRkey, req.Rkey)
		return false
	}

	// Verify collection
	if req.Collection != COLLECTION_TYPE_FEED_GENERATOR {
		t.Errorf("putRecord: collection mismatch - expected %s, got %s", COLLECTION_TYPE_FEED_GENERATOR, req.Collection)
		return false
	}

	// Verify repo (DID)
	if req.Repo == "" {
		t.Error("putRecord: repo is empty")
		return false
	}

	// Parse the record field to verify its contents
	var recordData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &recordData); err != nil {
		t.Errorf("putRecord: failed to parse full body: %v", err)
		return false
	}

	record, ok := recordData["record"].(map[string]interface{})
	if !ok {
		t.Error("putRecord: record field is missing or invalid")
		return false
	}

	// Verify displayName
	displayName, ok := record["displayName"].(string)
	if !ok || displayName != expectedDisplayName {
		t.Errorf("putRecord: displayName mismatch - expected %s, got %v", expectedDisplayName, displayName)
		return false
	}

	// Verify description
	description, ok := record["description"].(string)
	if !ok || description != expectedDescription {
		t.Errorf("putRecord: description mismatch - expected %s, got %v", expectedDescription, description)
		return false
	}

	// Verify did (serviceDID)
	did, ok := record["did"].(string)
	if !ok || did != expectedServiceDID {
		t.Errorf("putRecord: did mismatch - expected %s, got %v", expectedServiceDID, did)
		return false
	}

	// Verify createdAt exists
	if _, ok := record["createdAt"].(string); !ok {
		t.Error("putRecord: createdAt field is missing or invalid")
		return false
	}

	// Verify avatar blob exists
	if avatar, ok := record["avatar"].(map[string]interface{}); ok {
		if _, hasRef := avatar["ref"]; !hasRef {
			t.Error("putRecord: avatar blob ref is missing")
			return false
		}
		if _, hasMime := avatar["mimeType"]; !hasMime {
			t.Error("putRecord: avatar blob mimeType is missing")
			return false
		}
	} else {
		t.Error("putRecord: avatar field is missing or invalid")
		return false
	}

	// Verify contentMode
	contentMode, ok := record["contentMode"].(string)
	if !ok || contentMode != expectedContentMode {
		t.Errorf("putRecord: contentMode mismatch - expected %s, got %v", expectedContentMode, contentMode)
		return false
	}

	// Verify acceptsInteractions
	acceptsInteractions, ok := record["acceptsInteractions"].(bool)
	if !ok || acceptsInteractions != expectedAcceptsInteractions {
		t.Errorf("putRecord: acceptsInteractions mismatch - expected %v, got %v", expectedAcceptsInteractions, acceptsInteractions)
		return false
	}

	// Return mock put record response
	resp := map[string]interface{}{
		"uri": "at://did:plc:test123456789/app.bsky.feed.generator/" + expectedRkey,
		"cid": "bafyreimockcid123",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)

	t.Log("putRecord: verification passed")
	return true
}

// handleDeleteSession mocks com.atproto.server.deleteSession
func handleDeleteSession(t *testing.T, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// createTestAvatarPngFile creates a temporary PNG file for testing
func createTestAvatarPngFile(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "test-avatar-*.png")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	// Create a simple 10x10 PNG image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if err := png.Encode(tmpFile, img); err != nil {
		t.Fatalf("Failed to encode PNG: %v", err)
	}

	return tmpFile.Name()
}

func createTestAvatarJpegFile(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "test-avatar-*.jpeg")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	// Create a simple 10x10 JPEG image
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if err := jpeg.Encode(tmpFile, img, nil); err != nil {
		t.Fatalf("Failed to encode JPEG: %v", err)
	}

	return tmpFile.Name()
}
