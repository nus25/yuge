package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nus25/yuge/feed/config/feed"
	"github.com/nus25/yuge/feed/config/types"
)

var _ FeedConfigProvider = (*PDSFeedConfigProvider)(nil) //type check

const (
	BlueskyAPIBaseURL = "https://public.api.bsky.app"
)

// PDSFeedConfigProvider provides feed configuration from PDS.
type PDSFeedConfigProvider struct {
	apiBaseURL string
	uri        string
	config     types.FeedConfig
}

// NewPDSFeedConfigProvider creates a new PDSProvider instance.
// uri is the URI of the PDS "app.bsky.feed.generator" record to load the feed configuration from.
func NewPDSFeedConfigProvider(uri string) (FeedConfigProvider, error) {
	return NewPDSFeedConfigProviderWithBaseURL(uri, "")
}

// apiBaseURL is the base URL of the XRPC API.if nil, BlueskyAPIBaseURL will be used.
func NewPDSFeedConfigProviderWithBaseURL(uri string, apiBaseURL string) (FeedConfigProvider, error) {
	provider := &PDSFeedConfigProvider{
		uri: uri,
	}

	if apiBaseURL == "" {
		apiBaseURL = BlueskyAPIBaseURL
	}
	provider.apiBaseURL = apiBaseURL

	// Initial load
	cfg, err := provider.Load()
	if err != nil {
		return nil, err
	}
	provider.config = cfg

	return provider, nil
}

// Load loads configuration from PDS.
func (p *PDSFeedConfigProvider) Load() (types.FeedConfig, error) {
	slog.Info("loading feed config from PDS", "uri", p.uri)

	// Parse URI
	parts := strings.Split(p.uri, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid feed URI format: %s", p.uri)
	}

	repo := parts[2]
	rkey := parts[4]

	url := fmt.Sprintf("%s/xrpc/com.atproto.repo.getRecord?repo=%s&collection=app.bsky.feed.generator&rkey=%s", p.apiBaseURL, repo, rkey)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get record: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse JSON
	var record struct {
		Value struct {
			YugeFeed json.RawMessage `json:"yugeFeed"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	// Parse JSON from string
	var yugeFeedData json.RawMessage
	if err := json.Unmarshal(record.Value.YugeFeed, &yugeFeedData); err != nil {
		return nil, fmt.Errorf("failed to parse yugeFeed JSON string: %w", err)
	}

	// Create config object
	var cfg feed.FeedConfigImpl

	// Unmarshal JSON to feed config
	if err := cfg.UnmarshalJSON(yugeFeedData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal feed config: %w", err)
	}

	// Validate config
	if err := cfg.ValidateAll(); err != nil {
		return nil, fmt.Errorf("invalid feed config: %w", err)
	}

	slog.Info("feed config loaded from PDS",
		"feedLogic", cfg.FeedLogic(),
		"store", func() string {
			if cfg.Store() == nil {
				return "nil"
			}
			return fmt.Sprintf("trimAt=%d,trimRemain=%d", cfg.Store().GetTrimAt(), cfg.Store().GetTrimRemain())
		}(),
		"detailedLog", cfg.DetailedLog())

	p.config = &cfg
	return &cfg, nil
}

// Save saves current configuration to PDS.
// Note: Writing to PDS is not supported in the current version.
func (p *PDSFeedConfigProvider) Save() error {
	slog.Warn("Save operation is not supported in PDSProvider")
	return fmt.Errorf("save operation is not supported in PDSProvider")
}

// FeedConfig returns the current configuration.
func (p *PDSFeedConfigProvider) FeedConfig() types.FeedConfig {
	return p.config
}

// Update updates the configuration.
func (p *PDSFeedConfigProvider) Update(cfg types.FeedConfig) error {
	newCfg := cfg.DeepCopy()
	p.config = newCfg
	slog.Info("configuration updated in PDSProvider (note: not saved to PDS)")
	return nil
}
