package subscriber

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileFeedDefinitionProvider_GetFeedDefinitionList_IgnoresVersionFiles(t *testing.T) {
	configDir := t.TempDir()
	versionDir := filepath.Join(configDir, "version")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, FILE_NAME), []byte("feeds:\n  - id: configured-feed\n    uri: at://did:plc:configured/app.bsky.feed.generator/feed\n"), 0644); err != nil {
		t.Fatalf("WriteFile() feedlist error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "feedlist_v99_20260905_080000.yaml"), []byte("feeds:\n  - id: versioned-feed\n    uri: at://did:plc:versioned/app.bsky.feed.generator/feed\n"), 0644); err != nil {
		t.Fatalf("WriteFile() version error = %v", err)
	}

	provider, err := NewFileFeedDefinitionProvider(configDir)
	if err != nil {
		t.Fatalf("NewFileFeedDefinitionProvider() error = %v", err)
	}
	definitions, err := provider.GetFeedDefinitionList()
	if err != nil {
		t.Fatalf("GetFeedDefinitionList() error = %v", err)
	}
	if len(definitions.Feeds) != 1 || definitions.Feeds[0].ID != "configured-feed" {
		t.Fatalf("GetFeedDefinitionList() feeds = %#v, want configured feed only", definitions.Feeds)
	}
}
