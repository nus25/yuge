package subscriber

import (
	"errors"
	"fmt"
	"net/http"
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
	ErrGyokaConfigNotFound              = errors.New("gyoka configuration file not found")
	ErrGyokaConfigInvalid               = errors.New("gyoka configuration is invalid")
	ErrGyokaAppPasswordNotSet           = errors.New("GYOKA_APP_PASSWORD is not set")
	ErrGyokaAuthenticationNotSet        = errors.New("GYOKA_APP_PASSWORD or GYOKA_PRIVATE_KEY is not set")
	ErrGyokaPrivateKeyInvalid           = errors.New("GYOKA_PRIVATE_KEY is invalid")
	ErrGyokaAudienceNotSet              = errors.New("gyoka audience is not set")
	ErrGyokaDirectHostNotSet            = errors.New("gyoka direct host is not set")
	ErrGyokaDirectHostInvalid           = errors.New("gyoka direct host must be an HTTP(S) URL")
	ErrGyokaHostNotSet                  = errors.New("gyoka host is not set")
	ErrGyokaUserIdentityNotSet          = errors.New("gyoka user identity is not set")
	ErrGyokaUserIdentityInvalid         = errors.New("gyoka user identity must be a DID for inter-service authentication")
	ErrGyokaHeaderEnvNotSet             = errors.New("gyoka header environment variable is not set")
	ErrGyokaHeadersRequirePrivateKey    = errors.New("gyoka custom headers require private key authentication")
	ErrGyokaAuthorizationHeaderReserved = errors.New("Authorization header is reserved for inter-service authentication")
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
	headers              http.Header
	authMode             gyokaAuthMode
	minRequestIntervalMS int
}

type gyokaConfigFile struct {
	Host                 string            `yaml:"host"`
	DirectHost           string            `yaml:"directHost"`
	AllowInsecureHTTP    bool              `yaml:"allowInsecureHttp"`
	Audience             string            `yaml:"audience"`
	UserIdentity         string            `yaml:"userIdentity"`
	MinRequestIntervalMS int               `yaml:"minRequestIntervalMs"`
	Headers              map[string]string `yaml:"headers"`
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
		if len(fileConfig.Headers) > 0 {
			return gyokaProjectionConfig{}, ErrGyokaHeadersRequirePrivateKey
		}
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
	directHost, err := parseGyokaDirectHost(fileConfig.DirectHost, fileConfig.AllowInsecureHTTP)
	if err != nil {
		return gyokaProjectionConfig{}, err
	}
	issuerDID, err := syntax.ParseDID(fileConfig.UserIdentity)
	if err != nil {
		return gyokaProjectionConfig{}, fmt.Errorf("%w: %v", ErrGyokaUserIdentityInvalid, err)
	}
	headers, err := resolveGyokaHeaders(fileConfig.Headers)
	if err != nil {
		return gyokaProjectionConfig{}, err
	}
	config.host = directHost
	config.audience = fileConfig.Audience
	config.issuerDID = issuerDID
	config.privateKey = privateKey
	config.headers = headers
	config.authMode = gyokaAuthInterService
	return config, nil
}

func resolveGyokaHeaders(configured map[string]string) (http.Header, error) {
	if len(configured) == 0 {
		return nil, nil
	}

	headers := make(http.Header, len(configured))
	for name, rawValue := range configured {
		if !isValidGyokaHeaderName(name) {
			return nil, fmt.Errorf("%w: invalid gyoka header name %q", ErrGyokaConfigInvalid, name)
		}
		canonicalName := http.CanonicalHeaderKey(name)
		if strings.EqualFold(name, "Authorization") {
			return nil, ErrGyokaAuthorizationHeaderReserved
		}

		var missingEnv string
		value := os.Expand(rawValue, func(envName string) string {
			envValue, ok := os.LookupEnv(envName)
			if !ok || strings.TrimSpace(envValue) == "" {
				missingEnv = envName
				return ""
			}
			return envValue
		})
		if missingEnv != "" {
			return nil, fmt.Errorf("%w: %s", ErrGyokaHeaderEnvNotSet, missingEnv)
		}
		headers.Set(canonicalName, value)
	}
	return headers, nil
}

func isValidGyokaHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for position := 0; position < len(name); position++ {
		character := name[position]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func parseGyokaDirectHost(rawURL string, allowInsecureHTTP bool) (string, error) {
	directHost := strings.TrimSpace(rawURL)
	if directHost == "" {
		return "", ErrGyokaDirectHostNotSet
	}
	parsed, err := url.Parse(directHost)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrGyokaDirectHostInvalid
	}
	if parsed.Scheme == "http" && !allowInsecureHTTP {
		return "", fmt.Errorf("%w: HTTPS is required unless allowInsecureHttp is true", ErrGyokaDirectHostInvalid)
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
