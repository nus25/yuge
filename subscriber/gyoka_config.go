package subscriber

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/goccy/go-yaml"
)

const defaultGyokaMinRequestIntervalMS = 1000

var (
	ErrGyokaConfigNotFound       = errors.New("gyoka configuration file not found")
	ErrGyokaConfigInvalid        = errors.New("gyoka configuration is invalid")
	ErrGyokaAppPasswordNotSet    = errors.New("GYOKA_APP_PASSWORD is not set")
	ErrGyokaAuthenticationNotSet = errors.New("GYOKA_APP_PASSWORD or GYOKA_PRIVATE_KEY is not set")
	ErrGyokaPrivateKeyInvalid    = errors.New("GYOKA_PRIVATE_KEY is invalid")
	ErrGyokaAudienceNotSet       = errors.New("gyoka audience is not set")
	ErrGyokaDirectHostNotSet     = errors.New("gyoka direct host is not set")
	ErrGyokaDirectHostInvalid    = errors.New("gyoka direct host must be an HTTP(S) URL")
	ErrGyokaHostNotSet           = errors.New("gyoka host is not set")
	ErrGyokaUserIdentityNotSet   = errors.New("gyoka user identity is not set")
	ErrGyokaUserIdentityInvalid  = errors.New("gyoka user identity must be a DID for inter-service authentication")
)

type gyokaAuthMode uint8

const (
	gyokaAuthDisabled gyokaAuthMode = iota
	gyokaAuthPassword
	gyokaAuthInterService
)

func (m gyokaAuthMode) String() string {
	switch m {
	case gyokaAuthPassword:
		return "pds_proxy_password"
	case gyokaAuthInterService:
		return "direct_inter_service"
	default:
		return "disabled"
	}
}

type gyokaProjectionConfig struct {
	host                 string
	audience             string
	userIdentity         string
	appPassword          string
	issuerDID            syntax.DID
	privateKey           atcrypto.PrivateKey
	authMode             gyokaAuthMode
	minRequestIntervalMS int
}

type gyokaConfigFile struct {
	Host                 string `yaml:"host"`
	DirectHost           string `yaml:"directHost"`
	Audience             string `yaml:"audience"`
	UserIdentity         string `yaml:"userIdentity"`
	MinRequestIntervalMS int    `yaml:"minRequestIntervalMs"`
}

func loadGyokaProjectionConfig(configDirectory, appPassword, privateKeyEncoded string) (gyokaProjectionConfig, error) {
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
	if strings.TrimSpace(fileConfig.UserIdentity) == "" {
		return gyokaProjectionConfig{}, ErrGyokaUserIdentityNotSet
	}
	if fileConfig.MinRequestIntervalMS <= 0 {
		fileConfig.MinRequestIntervalMS = defaultGyokaMinRequestIntervalMS
	}

	config := gyokaProjectionConfig{
		host:                 fileConfig.Host,
		userIdentity:         fileConfig.UserIdentity,
		minRequestIntervalMS: fileConfig.MinRequestIntervalMS,
	}
	if strings.TrimSpace(privateKeyEncoded) == "" {
		if strings.TrimSpace(appPassword) == "" {
			return gyokaProjectionConfig{}, ErrGyokaAuthenticationNotSet
		}
		if strings.TrimSpace(fileConfig.Host) == "" {
			return gyokaProjectionConfig{}, ErrGyokaHostNotSet
		}
		config.appPassword = appPassword
		config.authMode = gyokaAuthPassword
		return config, nil
	}

	privateKey, err := atcrypto.ParsePrivateMultibase(privateKeyEncoded)
	if err != nil {
		return gyokaProjectionConfig{}, fmt.Errorf("%w: %v", ErrGyokaPrivateKeyInvalid, err)
	}
	if strings.TrimSpace(fileConfig.Audience) == "" {
		return gyokaProjectionConfig{}, ErrGyokaAudienceNotSet
	}
	directHost, err := parseGyokaDirectHost(fileConfig.DirectHost)
	if err != nil {
		return gyokaProjectionConfig{}, err
	}
	issuerDID, err := syntax.ParseDID(fileConfig.UserIdentity)
	if err != nil {
		return gyokaProjectionConfig{}, fmt.Errorf("%w: %v", ErrGyokaUserIdentityInvalid, err)
	}
	config.host = directHost
	config.audience = fileConfig.Audience
	config.issuerDID = issuerDID
	config.privateKey = privateKey
	config.authMode = gyokaAuthInterService
	return config, nil
}

func parseGyokaDirectHost(rawURL string) (string, error) {
	directHost := strings.TrimSpace(rawURL)
	if directHost == "" {
		return "", ErrGyokaDirectHostNotSet
	}
	parsed, err := url.Parse(directHost)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrGyokaDirectHostInvalid
	}
	return directHost, nil
}

func loadGyokaProjectionConfigForStartup(configDirectory, appPassword, privateKey string) (gyokaProjectionConfig, bool, error) {
	config, err := loadGyokaProjectionConfig(configDirectory, appPassword, privateKey)
	if err != nil {
		return gyokaProjectionConfig{}, false, err
	}
	return config, true, nil
}
