package subscriber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

const defaultGyokaMinRequestIntervalMS = 1000

var (
	ErrGyokaConfigNotFound     = errors.New("gyoka configuration file not found")
	ErrGyokaConfigInvalid      = errors.New("gyoka configuration is invalid")
	ErrGyokaAppPasswordNotSet  = errors.New("GYOKA_APP_PASSWORD is not set")
	ErrGyokaHostNotSet         = errors.New("gyoka host is not set")
	ErrGyokaUserIdentityNotSet = errors.New("gyoka user identity is not set")
)

type gyokaProjectionConfig struct {
	host                 string
	userIdentity         string
	appPassword          string
	minRequestIntervalMS int
}

type gyokaConfigFile struct {
	Host                 string `yaml:"host"`
	UserIdentity         string `yaml:"userIdentity"`
	MinRequestIntervalMS int    `yaml:"minRequestIntervalMs"`
}

func loadGyokaProjectionConfig(configDirectory, appPassword string) (gyokaProjectionConfig, error) {
	configPath := filepath.Join(configDirectory, "gyoka.yaml")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return gyokaProjectionConfig{}, fmt.Errorf("%w: %s", ErrGyokaConfigNotFound, configPath)
		}
		return gyokaProjectionConfig{}, fmt.Errorf("read gyoka configuration: %w", err)
	}

	var fileConfig gyokaConfigFile
	if err := yaml.Unmarshal(contents, &fileConfig); err != nil {
		return gyokaProjectionConfig{}, fmt.Errorf("%w: %v", ErrGyokaConfigInvalid, err)
	}
	if strings.TrimSpace(fileConfig.Host) == "" {
		return gyokaProjectionConfig{}, ErrGyokaHostNotSet
	}
	if strings.TrimSpace(fileConfig.UserIdentity) == "" {
		return gyokaProjectionConfig{}, ErrGyokaUserIdentityNotSet
	}
	if strings.TrimSpace(appPassword) == "" {
		return gyokaProjectionConfig{}, ErrGyokaAppPasswordNotSet
	}
	if fileConfig.MinRequestIntervalMS <= 0 {
		fileConfig.MinRequestIntervalMS = defaultGyokaMinRequestIntervalMS
	}

	return gyokaProjectionConfig{
		host:                 fileConfig.Host,
		userIdentity:         fileConfig.UserIdentity,
		appPassword:          appPassword,
		minRequestIntervalMS: fileConfig.MinRequestIntervalMS,
	}, nil
}

func loadGyokaProjectionConfigForStartup(configDirectory, appPassword string) (gyokaProjectionConfig, bool, error) {
	config, err := loadGyokaProjectionConfig(configDirectory, appPassword)
	if err != nil {
		return gyokaProjectionConfig{}, false, err
	}
	return config, true, nil
}
