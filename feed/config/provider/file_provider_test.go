package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/feed"
)

// TestFileFeedConfigProvider_Load tests the Load method of FileFeedConfigProvider
func TestFileFeedConfigProvider_Load(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "feed-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config file
	configPath := filepath.Join(tempDir, "feed-config.yaml")
	configData := []byte(`
logic:
  blocks:
    - type: remove
      options:
        subject: item
        value: reply
store:
  trimAt: 24
  trimRemain: 20
detailedLog: false
`)
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Create version directory and versioned config
	versionDir := filepath.Join(tempDir, "version")
	if err := os.Mkdir(versionDir, 0755); err != nil {
		t.Fatalf("Failed to create version directory: %v", err)
	}

	versionedConfigPath := filepath.Join(versionDir, "feed-config.yaml.1")
	if err := os.WriteFile(versionedConfigPath, configData, 0644); err != nil {
		t.Fatalf("Failed to write versioned config file: %v", err)
	}

	// Test loading from original file
	provider, err := NewFileFeedConfigProvider(configPath)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	config, err := provider.Load()
	if err != nil {
		t.Errorf("Failed to load config: %v", err)
	}
	if config == nil {
		t.Error("Loaded config is nil")
	}

	// Test Update and FeedConfig methods
	newConfigData := []byte(`logic:
  blocks:
    - type: regex
      options:
        value: [1-9]
        invert: true
        caseSensitive: true
store:
  trimAt: 240
  trimRemain: 200
detailedLog: false
`)
	newConfig := feed.FeedConfigImpl{}
	if err := yaml.Unmarshal(newConfigData, &newConfig); err != nil {
		t.Errorf("Failed to unmarshal new config: %v", err)
	}
	if err := provider.Update(&newConfig); err != nil {
		t.Errorf("Failed to update config: %v", err)
	}
	// 構造体の各フィールドを個別に比較
	providerConfig := provider.FeedConfig()
	//compare FeedLogic
	originalBlocks := providerConfig.FeedLogic().GetLogicBlockConfigs()
	newBlocks := newConfig.FeedLogic().GetLogicBlockConfigs()
	if len(originalBlocks) != len(newBlocks) {
		t.Errorf("Blocks length do not match: expected %v, got %v", len(originalBlocks), len(newBlocks))
	}
	for i, block := range originalBlocks {
		if block.GetBlockType() != newBlocks[i].GetBlockType() {
			t.Errorf("Blocks[%d] type do not match: expected %v, got %v", i, block.GetBlockType(), newBlocks[i].GetBlockType())
		}
	}
	//compare FeedStore
	if providerConfig.Store().GetTrimAt() != newConfig.Store().GetTrimAt() {
		t.Errorf("TrimAt fields do not match: expected %v, got %v", newConfig.Store().GetTrimAt(), providerConfig.Store().GetTrimAt())
	}
	if providerConfig.Store().GetTrimRemain() != newConfig.Store().GetTrimRemain() {
		t.Errorf("TrimRemain fields do not match: expected %v, got %v", newConfig.Store().GetTrimRemain(), providerConfig.Store().GetTrimRemain())
	}
	if providerConfig.DetailedLog() != newConfig.DetailedLog() {
		t.Errorf("DetailedLog fields do not match: expected %v, got %v", newConfig.DetailedLog(), providerConfig.DetailedLog())
	}
	// Test Save method
	if err := provider.Save(); err != nil {
		t.Errorf("Failed to save config: %v", err)
	}

	// Verify file was saved
	_, err = os.Stat(configPath)
	if err != nil {
		t.Errorf("Config file not found after save: %v", err)
	}
}

// TestLoadFeedConfigFromFile tests the LoadFeedConfigFromFile function
func TestLoadFeedConfigFromFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "feed-config-load-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config file
	configPath := filepath.Join(tempDir, "feed-config.yaml")
	configData := []byte(`
logic:
  blocks:
    - type: remove
      options:
        subject: item
        value: reply
store:
  trimAt: 36
  trimRemain: 32
detailedLog: false
`)

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}
	provider, err := NewFileFeedConfigProvider(configPath)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Test loading config
	config, err := provider.Load()
	if err != nil {
		t.Errorf("Failed to load config from file: %v", err)
	}
	if config == nil {
		t.Error("Loaded config is nil")
	}
	if config.Store().GetTrimAt() != 36 {
		t.Errorf("TrimAt fields do not match: expected %v, got %v", 36, config.Store().GetTrimAt())
	}
	if config.Store().GetTrimRemain() != 32 {
		t.Errorf("TrimRemain fields do not match: expected %v, got %v", 32, config.Store().GetTrimRemain())
	}

	// Test loading non-existent file
	provider, err = NewFileFeedConfigProvider("^/invalid-path")
	if err == nil {
		t.Error("Expected error when loading non-existent file, got nil")
	}
}

// TestLoadInvalidFeedConfigFromFile tests loading an invalid YAML file
func TestLoadInvalidFeedConfigFromFile(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "feed-config-invalid-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create invalid YAML config file
	configPath := filepath.Join(tempDir, "invalid-feed-config.yaml")
	invalidConfigData := []byte(`
logic:
  blocks:
    - type: remove
      options:
        subject: item
        value: reply
store:
  trimAt: "not-a-number"  # Invalid value type
  trimRemain: 32
detailedLog: false
`)

	if err := os.WriteFile(configPath, invalidConfigData, 0644); err != nil {
		t.Fatalf("Failed to write invalid test config file: %v", err)
	}
	_, err = NewFileFeedConfigProvider(configPath)
	if err == nil {
		t.Error("Expected error when loading invalid YAML file, got nil")
	}

	// Create malformed YAML config file
	malformedPath := filepath.Join(tempDir, "malformed-feed-config.yaml")
	malformedConfigData := []byte(`
logic:
  blocks:
    - type: remove
      options:
        subject: item
    value: reply  # Malformed YAML indentation
store:
  trimAt: 36
  trimRemain: 32
detailedLog: false
`)

	if err := os.WriteFile(malformedPath, malformedConfigData, 0644); err != nil {
		t.Fatalf("Failed to write malformed test config file: %v", err)
	}
	_, err = NewFileFeedConfigProvider(malformedPath)
	if err == nil {
		t.Fatalf("Expected error when loading malformed YAML file, got nil")
	}
}

func TestLoadFeedConfigFromInvalidPath(t *testing.T) {
	// Test loading from non-existent file path
	nonExistentPath := "/path/does/not/exist/config.yaml"
	provider, err := NewFileFeedConfigProvider(nonExistentPath)
	if provider != nil {
		t.Error("Expected nil provider when loading from non-existent file path, got nil")
	}
	if err == nil {
		t.Error("Expected error when loading from non-existent file path, got nil")
	}

	// Test loading from empty file path
	provider, err = NewFileFeedConfigProvider("")
	if provider != nil {
		t.Error("Expected nil provider when loading from empty file path, got nil")
	}
	if err == nil {
		t.Error("Expected error when loading from empty file path, got nil")
	}

	// Test loading from directory instead of file
	tempDir, err := os.MkdirTemp("", "feed-config-test")
	if provider != nil {
		t.Error("Expected nil provider when loading from directory instead of file, got nil")
	}
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	provider, err = NewFileFeedConfigProvider(tempDir)
	if provider != nil {
		t.Error("Expected nil provider when loading from directory instead of file, got nil")
	}
	if err == nil {
		t.Error("Expected error when loading from directory instead of file, got nil")
	}
}
func TestNewFileFeedConfigProviderInvalidPath(t *testing.T) {
	// Test with non-existent file path
	nonExistentPath := "/path/does/not/exist/config.yaml"
	_, err := NewFileFeedConfigProvider(nonExistentPath)
	if err == nil {
		t.Error("Expected error when using non-existent file path, but got nil")
	}

	// Test with empty file path
	_, err = NewFileFeedConfigProvider("")
	if err == nil {
		t.Error("Expected error when using empty file path, but got nil")
	}

	// Test with directory instead of file
	tempDir, err := os.MkdirTemp("", "feed-config-test")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	_, err = NewFileFeedConfigProvider(tempDir)
	if err == nil {
		t.Error("Expected error when using directory as file path, but got nil")
	}
}
