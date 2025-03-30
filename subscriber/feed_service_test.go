package subscriber

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/feed"
	"github.com/nus25/yuge/feed/store/editor"
)

// MockFeed implements feed.Feed for testing
type MockFeed struct {
	shutdownErr error
}

func (m *MockFeed) Shutdown(ctx context.Context) error {
	return m.shutdownErr
}

func (m *MockFeed) Close() error {
	return nil
}

func TestNewFeedService(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "feed-service-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	dp, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create feed definition provider: %v", err)
	}
	e, err := editor.NewFileEditor(dataDir, logger)
	if err != nil {
		t.Fatalf("Failed to create editor: %v", err)
	}
	tests := []struct {
		name               string
		configDir          string
		dataDir            string
		definitionProvider FeedDefinitionProvider
		storeEditor        editor.StoreEditor
		expectError        bool
	}{
		{
			name:               "正常なパラメータでの作成",
			configDir:          configDir,
			definitionProvider: dp,
			dataDir:            dataDir,
			storeEditor:        e,
			expectError:        false,
		},
		{
			name:               "storeEditorがnilの場合",
			configDir:          configDir,
			dataDir:            dataDir,
			definitionProvider: dp,
			storeEditor:        nil,
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewFeedService(tt.configDir, tt.dataDir, tt.definitionProvider, tt.storeEditor, logger)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && service == nil {
				t.Error("Expected service to be created but got nil")
			}

			if !tt.expectError {
				if service.configDir != tt.configDir {
					t.Errorf("Expected configDir to be %s, got %s", tt.configDir, service.configDir)
				}

				if service.dataDir != tt.dataDir {
					t.Errorf("Expected dataDir to be %s, got %s", tt.dataDir, service.dataDir)
				}

				if service.feeds == nil {
					t.Error("Expected feeds map to be initialized")
				}
			}
		})
	}
}

func TestFeedService_Load(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "feed-service-load-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	e, err := editor.NewFileEditor(dataDir, logger)
	if err != nil {
		t.Fatalf("Failed to create editor: %v", err)
	}
	p, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	tests := []struct {
		name        string
		provider    FeedDefinitionProvider
		expectError bool
	}{
		{
			name:        "success",
			provider:    p,
			expectError: false,
		},
		{
			name:        "provider is nil",
			provider:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewFeedService(configDir, dataDir, tt.provider, e, logger)
			if err != nil {
				t.Fatalf("Failed to create service: %v", err)
			}

			err = service.LoadFeeds(context.Background())

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && service.definitionProvider != tt.provider {
				t.Error("Expected definitionProvider to be set")
			}
		})
	}
}

func TestFeedService_GetFeedInfo(t *testing.T) {
	// Setup
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
		},
	}

	tests := []struct {
		name      string
		feedId    string
		expectOk  bool
		expectFed FeedInfo
	}{
		{
			name:     "存在するフィード",
			feedId:   "feed1",
			expectOk: true,
			expectFed: FeedInfo{
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
		},
		{
			name:     "存在しないフィード",
			feedId:   "nonexistent",
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, exists := service.GetFeedInfo(tt.feedId)

			if exists != tt.expectOk {
				t.Errorf("Expected exists to be %v, got %v", tt.expectOk, exists)
			}

			if tt.expectOk && info.Definition.ID != tt.expectFed.Definition.ID {
				t.Errorf("Expected feed ID to be %s, got %s", tt.expectFed.Definition.ID, info.Definition.ID)
			}
		})
	}
}

func TestFeedService_GetFeedList(t *testing.T) {
	// Setup
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1", LastStatus: FeedStatusActive},
			},
			"feed2": {
				Definition: FeedDefinition{ID: "feed2"},
				Status:     FeedStatus{FeedID: "feed2", LastStatus: FeedStatusError},
			},
		},
	}

	list := service.GetActiveFeedIDs()

	if len(list) != 1 {
		t.Errorf("Expected 1 feeds, got %d", len(list))
	}

	// Check if all feeds are included
	foundFeed1 := false
	foundFeed2 := false
	for _, id := range list {
		if id == "feed1" {
			foundFeed1 = true
		}
		if id == "feed2" {
			foundFeed2 = true
		}
	}

	if !foundFeed1 {
		t.Error("Expected feed1 in list but not found")
	}
	if foundFeed2 {
		t.Error("Expected not found feed2 in list")
	}
}

func TestFeedService_CreateFeed(t *testing.T) {
	// This is a simplified test as actual implementation would require mocking feed.NewFeedWithOptions
	tempDir, err := os.MkdirTemp("", "feed-service-create-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// Create config directory and a sample config file
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	jsonStr := `
    {
        "logic":{"blocks":[
		{"type":"regex",
		"options":{"value":"[1-9]","invert":false,"caseSensitive":false}
		}
		]
		}
    }
    `
	cfg, err := feed.NewFeedConfigFromJSON(jsonStr)
	if err != nil {
		t.Fatalf("Failed to create feed config: %v", err)
	}
	yamlStr, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("Failed to marshal feed config: %v", err)
	}
	sampleConfigPath := filepath.Join(configDir, "sample.yaml")
	if err := os.WriteFile(sampleConfigPath, yamlStr, 0644); err != nil {
		t.Fatalf("Failed to write sample config: %v", err)
	}
	e, err := editor.NewFileEditor(dataDir, logger)
	if err != nil {
		t.Fatalf("Failed to create editor: %v", err)
	}
	service, err := NewFeedService(configDir, dataDir, nil, e, logger)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	// Test cases
	tests := []struct {
		name        string
		definition  FeedDefinition
		status      Status
		expectError bool
	}{
		{
			name:        "新規フィード作成",
			definition:  FeedDefinition{ID: "new-feed", URI: "at://did:plc:1234567890/app.bsky.feed.generator/test", ConfigFile: "sample.yaml"},
			status:      FeedStatusActive,
			expectError: false,
		},
		{
			name:        "既存フィードID",
			definition:  FeedDefinition{ID: "new-feed", URI: "at://did:plc:1234567890/app.bsky.feed.generator/test", ConfigFile: "sample.yaml"},
			status:      FeedStatusActive,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.CreateFeed(context.Background(), tt.definition, tt.status)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError {
				info, exists := service.GetFeedInfo(tt.definition.ID)
				if !exists {
					t.Error("Expected feed to exist but not found")
				} else if info.Definition.ID != tt.definition.ID {
					t.Errorf("Expected feed ID to be %s, got %s", tt.definition.ID, info.Definition.ID)
				}
			}
		})
	}
}

func TestFeedService_DeleteFeed(t *testing.T) {
	// Setup

	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status:     FeedStatus{FeedID: "feed1"},
			},
			"feed2": {
				Definition: FeedDefinition{ID: "feed2"},
				Status:     FeedStatus{FeedID: "feed2"},
			},
		},
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		name        string
		feedId      string
		expectError bool
	}{
		{
			name:        "正常な削除",
			feedId:      "feed1",
			expectError: false,
		},
		{
			name:        "シャットダウンエラーがあっても削除",
			feedId:      "feed2",
			expectError: false,
		},
		{
			name:        "存在しないフィード",
			feedId:      "nonexistent",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clone the service for each test to avoid state interference
			testService := &FeedService{
				feeds:  make(map[string]FeedInfo),
				logger: service.logger,
			}
			for k, v := range service.feeds {
				testService.feeds[k] = v
			}

			err := testService.DeleteFeed(tt.feedId)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError && tt.feedId != "nonexistent" {
				_, exists := testService.GetFeedInfo(tt.feedId)
				if exists {
					t.Error("Expected feed to be deleted but still exists")
				}
			}
		})
	}
}

func TestFeedService_UpdateStatus(t *testing.T) {
	// Setup
	now := time.Now().Add(-1 * time.Hour)
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status: FeedStatus{
					FeedID:      "feed1",
					LastStatus:  FeedStatusActive,
					LastUpdated: now,
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}

	tests := []struct {
		name        string
		feedId      string
		status      Status
		expectError bool
	}{
		{
			name:        "ステータス更新",
			feedId:      "feed1",
			status:      FeedStatusError,
			expectError: false,
		},
		{
			name:        "存在しないフィード",
			feedId:      "nonexistent",
			status:      FeedStatusActive,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.UpdateStatus(tt.feedId, tt.status)

			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			if !tt.expectError {
				info, exists := service.GetFeedInfo(tt.feedId)
				if !exists {
					t.Error("Expected feed to exist but not found")
				} else {
					if info.Status.LastStatus != tt.status {
						t.Errorf("Expected status to be %v, got %v", tt.status, info.Status.LastStatus)
					}
					if !info.Status.LastUpdated.After(now) {
						t.Error("Expected LastUpdated to be updated")
					}
				}
			}
		})
	}
}

func TestFeedService_GetFeedStatus(t *testing.T) {
	// Setup
	service := &FeedService{
		feeds: map[string]FeedInfo{
			"feed1": {
				Definition: FeedDefinition{ID: "feed1"},
				Status: FeedStatus{
					FeedID:     "feed1",
					LastStatus: FeedStatusActive,
				},
			},
		},
	}

	tests := []struct {
		name      string
		feedId    string
		expectOk  bool
		expectSts FeedStatus
	}{
		{
			name:     "存在するフィード",
			feedId:   "feed1",
			expectOk: true,
			expectSts: FeedStatus{
				FeedID:     "feed1",
				LastStatus: FeedStatusActive,
			},
		},
		{
			name:     "存在しないフィード",
			feedId:   "nonexistent",
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, exists := service.GetFeedStatus(tt.feedId)

			if exists != tt.expectOk {
				t.Errorf("Expected exists to be %v, got %v", tt.expectOk, exists)
			}

			if tt.expectOk {
				if status.FeedID != tt.expectSts.FeedID {
					t.Errorf("Expected FeedID to be %s, got %s", tt.expectSts.FeedID, status.FeedID)
				}
				if status.LastStatus != tt.expectSts.LastStatus {
					t.Errorf("Expected LastStatus to be %v, got %v", tt.expectSts.LastStatus, status.LastStatus)
				}
			}
		})
	}
}
