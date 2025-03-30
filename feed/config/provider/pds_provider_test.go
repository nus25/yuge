package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/feed"
)

const (
	validResponse = `{
		"uri": "at://did:plc:testuser/app.bsky.feed.generator/yugetest",
		"cid": "bafyreifeycgxhtughsp6rzkh7wd6aqufrzovmtclbfxaadmubug72gwr7q",
		"value": {
		  "$type": "app.bsky.feed.generator",
		  "avatar": {
			"$type": "blob",
			"ref": {
			  "$link": "bafkreibbjewfbppt35vs4ebxi6bafkreibbjewfbppt35vs4ebxi6sskvqy"
			},
			"mimeType": "image/png",
			"size": 15195
		  },
		  "createdAt": "2025-02-18T09:53:24.000Z",
		  "description": "test feed for yuge",
		  "did": "did:web:feed-generator.example.com",
		  "displayName": "yugetest",
		  "yugeFeed": {
			"detailedLog": true,
			"logic": {
			  "blocks": [
				{
				  "options": {
					"subject": "item",
					"value": "reply"
				  },
				  "type": "remove"
				},
				{
				  "options": {
					"language": "ja",
					"operator": "!=",
					"subject": "language"
				  },
				  "type": "remove"
				},
				{
				  "options": {
					"caseSensitive": false,
					"invert": false,
					"value": "[\\\\p{Hiragana}]"
				  },
				  "type": "regex"
				},
				{
				  "options": {
					"caseSensitive": false,
					"invert": false,
					"value": "^(?!.*http)[^@](?!.*^\\\").{100,}(?!\\\".*)$"
				  },
				  "type": "regex"
				}
			  ]
			},
			"store": {
			  "trimAt": 1200,
			  "trimRemain": 1000
			}
		  }
		}
	  }`
)

// TestNewPDSFeedConfigProvider tests the creation of PDSFeedConfigProvider
func TestNewPDSFeedConfigProvider(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(validResponse))
	}))
	defer server.Close()

	uri := "at://repo/app.bsky.feed.generator/rkey"
	provider, err := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if provider == nil {
		t.Error("provider is nil")
	}
}

// TestPDSProviderLoad
func TestPDSProviderLoad(t *testing.T) {
	t.Run("Valid configuration", func(t *testing.T) {
		// create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// check request URL is correct
			expectedPath := "/xrpc/com.atproto.repo.getRecord"
			if r.URL.Path != expectedPath {
				t.Errorf("invalid path: %s, expected: %s", r.URL.Path, expectedPath)
			}

			// check request parameters are correct
			if r.URL.Query().Get("repo") != "did:plc:testuser" {
				t.Errorf("invalid repo: %s, expected: %s", r.URL.Query().Get("repo"), "did:plc:testuser")
			}

			if r.URL.Query().Get("rkey") != "yugetest" {
				t.Errorf("invalid rkey: %s, expected: %s", r.URL.Query().Get("rkey"), "yugetest")
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(validResponse))
		}))
		defer server.Close()

		uri := "at://did:plc:testuser/app.bsky.feed.generator/yugetest"

		provider, _ := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)
		config, err := provider.Load()

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if config == nil {
			t.Error("loaded config is nil")
		}

		if !config.DetailedLog() {
			t.Error("DetailedLog is not correct")
		}
	})

	t.Run("PDS client error", func(t *testing.T) {
		// create test server that returns error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		uri := "at://repo/app.bsky.feed.generator/rkey"

		provider, err := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)

		if provider != nil || err == nil {
			t.Error("expected error, but nil was returned")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// create test server that returns invalid JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"value": map[string]interface{}{
					"yugeFeed": `{"invalid json`,
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		uri := "at://repo/app.bsky.feed.generator/rkey"

		provider, err := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)

		if provider != nil || err == nil {
			t.Error("expected error, but nil was returned")
		}
	})
}

// TestPDSProviderSave
func TestPDSProviderSave(t *testing.T) {
	t.Run("save is not supported", func(t *testing.T) {
		// create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(validResponse))
		}))
		defer server.Close()

		uri := "at://repo/app.bsky.feed.generator/rkey"
		provider, _ := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)

		err := provider.Save()

		if err == nil {
			t.Error("Save operation is not supported, so an error was expected")
		}
	})
}

// TestPDSProviderFeedConfig tests the FeedConfig function
func TestPDSProviderFeedConfig(t *testing.T) {
	// Create test server
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(validResponse))
	}))

	// テスト終了時に2つのリクエストが送信されたことを確認
	t.Cleanup(func() {
		if requestCount != 2 {
			t.Errorf("Expected 2 requests, but got %d", requestCount)
		}
	})
	defer server.Close()

	uri := "at://repo/app.bsky.feed.generator/rkey"
	provider, _ := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)

	config, err := provider.Load()

	if err != nil {
		t.Errorf("Unexpected error during Load: %v", err)
	}

	if config == nil {
		t.Error("FeedConfig returned nil")
	}

	if !config.DetailedLog() {
		t.Error("DetailedLog value is not correct")
	}
}

// TestPDSProviderUpdate tests the Update function
func TestPDSProviderUpdate(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(validResponse))
	}))
	defer server.Close()

	uri := "at://repo/app.bsky.feed.generator/rkey"
	provider, err := NewPDSFeedConfigProviderWithBaseURL(uri, server.URL)
	if err != nil {
		t.Errorf("Unexpected error during NewPDSFeedConfigProvider: %v", err)
	}

	// Update configuration
	newConfigData := `
logic:
  blocks:
    - type: regex
      options:
        value: test
        invert: false
        caseSensitive: false
store:
  trimAt: 120
  trimRemain: 100`

	newCfg := feed.DefaultFeedConfig()
	err = yaml.Unmarshal([]byte(newConfigData), newCfg)
	err = provider.Update(newCfg)

	if err != nil {
		t.Errorf("Unexpected error during Update: %v", err)
	}

	// Get updated configuration
	config := provider.FeedConfig()

	if config == nil {
		t.Error("Configuration after update is nil")
	}

	if config.DetailedLog() { // should be false
		t.Error("DetailedLog value was not updated correctly")
	}
}
