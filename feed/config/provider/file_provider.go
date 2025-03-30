package provider

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/feed"
	"github.com/nus25/yuge/feed/config/types"
)

var _ FeedConfigProvider = (*FileFeedConfigProvider)(nil) //type check

// FileFeedConfigProvider provides feed configuration from a configuration file.
type FileFeedConfigProvider struct {
	configPath string
	config     types.FeedConfig
}

// NewFileFeedConfigProvider creates a new FileProvider instance.
func NewFileFeedConfigProvider(configPath string) (FeedConfigProvider, error) {
	// Convert to absolute path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", absPath)
	}

	provider := &FileFeedConfigProvider{
		configPath: absPath,
	}

	// Initial load
	cfg, err := provider.Load()
	if err != nil {
		return nil, err
	}
	provider.config = cfg

	return provider, nil
}

func (p *FileFeedConfigProvider) Load() (types.FeedConfig, error) {
	// Check version directory
	configDir := filepath.Dir(p.configPath)
	versionDir := filepath.Join(configDir, "version")
	baseFileName := filepath.Base(p.configPath)
	// Find the latest version file
	var latestFile string
	var latestTime time.Time
	if _, err := os.Stat(versionDir); !os.IsNotExist(err) {
		entries, err := os.ReadDir(versionDir)
		if err == nil && len(entries) > 0 {

			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), baseFileName) {
					info, err := entry.Info()
					if err == nil {
						if info.ModTime().After(latestTime) {
							latestTime = info.ModTime()
							latestFile = filepath.Join(versionDir, entry.Name())
						}
					}
				}
			}
		}
	}

	// Load from the latest version file if available
	var data []byte
	var err error
	if latestFile != "" && !latestTime.IsZero() {
		data, err = os.ReadFile(latestFile)
		slog.Info("load latest file", "path", latestFile)
		if err != nil {
			// If failed to read version file, load from original file
			data, err = os.ReadFile(p.configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read configuration file: %w", err)
			}
		}
	} else {
		// If no version file exists, load from original file
		data, err = os.ReadFile(p.configPath)
		slog.Info("load original file", "path", p.configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read configuration file: %w", err)
		}
	}

	var cfg feed.FeedConfigImpl
	// Decode to struct
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate configuration
	if err := cfg.ValidateAll(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	p.config = &cfg
	return &cfg, nil
}

// Save saves the current configuration to file.
func (p *FileFeedConfigProvider) Save() error {
	if p.configPath == "" {
		return fmt.Errorf("no configuration file path specified")
	}

	// Convert configuration to YAML
	data, err := yaml.Marshal(p.config)
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	slog.Info("Saving feed configuration", "path", p.configPath)
	return saveConfigFile(p.configPath, data)
}

// FeedConfig returns the current configuration.
func (p *FileFeedConfigProvider) FeedConfig() types.FeedConfig {
	return p.config
}

// Update updates the configuration.
func (p *FileFeedConfigProvider) Update(cfg types.FeedConfig) error {
	newCfg := cfg.DeepCopy()
	p.config = newCfg
	return nil
}

// saveConfigFile saves configuration data to a file and manages versioning
func saveConfigFile(configPath string, data []byte) error {
	// Create version management directory
	configDir := filepath.Dir(configPath)
	versionDir := filepath.Join(configDir, "version")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fmt.Errorf("failed to create version directory: %w", err)
	}

	// Backup current configuration to version directory
	timestamp := time.Now().Format("20060102_150405")
	versionPath := filepath.Join(versionDir, filepath.Base(configPath)+"."+timestamp)
	if currentData, err := os.ReadFile(configPath); err == nil {
		if err := os.WriteFile(versionPath, currentData, 0644); err != nil {
			return fmt.Errorf("failed to save version of config file: %w", err)
		}
	}

	// Write new configuration
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
