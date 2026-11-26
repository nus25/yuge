package subscriber

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestLoadGyokaProjectionConfigPrivateKeyFile(t *testing.T) {
	const validKey = "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D"
	const invalidKey = "invalid-secret-key-value"
	tests := []struct {
		name       string
		keyContent string
		absolute   bool
		missing    bool
		directory  bool
		wantErr    bool
	}{
		{name: "relative path and newline", keyContent: validKey + "\n"},
		{name: "absolute path", keyContent: validKey, absolute: true},
		{name: "missing file", missing: true, wantErr: true},
		{name: "unreadable file", directory: true, wantErr: true},
		{name: "empty file", wantErr: true},
		{name: "invalid key", keyContent: invalidKey, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			keyPath := filepath.Join(configDir, "private.key")
			if tt.directory {
				if err := os.Mkdir(keyPath, 0700); err != nil {
					t.Fatal(err)
				}
			} else if !tt.missing {
				if err := os.WriteFile(keyPath, []byte(tt.keyContent), 0600); err != nil {
					t.Fatal(err)
				}
			}
			configuredPath := "private.key"
			if tt.absolute {
				configuredPath = keyPath
			}
			configBody := "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"privateKeyFile: " + strconv.Quote(configuredPath) + "\n"
			if err := os.WriteFile(filepath.Join(configDir, "gyoka.yaml"), []byte(configBody), 0600); err != nil {
				t.Fatal(err)
			}
			config, enabled, err := loadGyokaProjectionConfigForStartup(configDir, "app-password")
			if tt.wantErr {
				if err == nil || enabled {
					t.Fatalf("expected disabled projection and error, got enabled=%v error=%v", enabled, err)
				}
				if strings.Contains(err.Error(), invalidKey) {
					t.Fatal("error exposes private key")
				}
				return
			}
			if err != nil || !enabled || config.authMode != gyokaAuthInterService || config.privateKey == nil {
				t.Fatalf("expected private key authentication, got enabled=%v mode=%v error=%v", enabled, config.authMode, err)
			}
		})
	}
}

func TestGyokaAuthModeString(t *testing.T) {
	tests := []struct {
		mode           gyokaAuthMode
		want           string
		wantCredential string
	}{
		{mode: gyokaAuthDisabled, want: "disabled", wantCredential: "disabled"},
		{mode: gyokaAuthPassword, want: "pds_proxy_password", wantCredential: "app_password"},
		{mode: gyokaAuthInterService, want: "direct_inter_service", wantCredential: "private_key"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if got := tt.mode.credentialType(); got != tt.wantCredential {
				t.Errorf("credentialType() = %q, want %q", got, tt.wantCredential)
			}
		})
	}
}

func TestLoadGyokaProjectionConfig(t *testing.T) {
	tests := []struct {
		name           string
		configBody     string
		env            map[string]string
		appPassword    string
		privateKey     string
		wantErr        error
		wantHost       string
		wantAudience   string
		wantUserID     string
		wantHeaders    map[string]string
		wantAuthMode   gyokaAuthMode
		wantIntervalMS int
	}{
		{
			name:        "missing gyoka yaml",
			appPassword: "app-password",
			wantErr:     ErrGyokaConfigNotFound,
		},
		{
			name: "missing app password",
			configBody: "host: gyoka.example.com\n" +
				"userIdentity: yuge.bsky.social\n",
			wantErr: ErrGyokaAuthenticationNotSet,
		},
		{
			name:        "malformed yaml",
			configBody:  "host: [\n",
			appPassword: "app-password",
			wantErr:     ErrGyokaConfigInvalid,
		},
		{
			name:        "missing host",
			configBody:  "userIdentity: yuge.bsky.social\n",
			appPassword: "app-password",
			wantErr:     ErrGyokaHostNotSet,
		},
		{
			name:        "missing user identity",
			configBody:  "host: gyoka.example.com\n",
			appPassword: "app-password",
			wantErr:     ErrGyokaUserIdentityNotSet,
		},
		{
			name: "valid configuration",
			configBody: "host: gyoka.example.com\n" +
				"userIdentity: yuge.bsky.social\n" +
				"minRequestIntervalMs: 2500\n",
			appPassword:    "app-password",
			wantHost:       "gyoka.example.com",
			wantUserID:     "yuge.bsky.social",
			wantAuthMode:   gyokaAuthPassword,
			wantIntervalMS: 2500,
		},
		{
			name: "private key authentication",
			configBody: "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey:     "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantHost:       "https://localhost:3000",
			wantAudience:   "did:web:gyoka.example.com#gyoka_editor",
			wantUserID:     "did:plc:ewvi7nxzyoun6zhxrhs64oiz",
			wantAuthMode:   gyokaAuthInterService,
			wantIntervalMS: defaultGyokaMinRequestIntervalMS,
		},
		{
			name: "private key authentication rejects HTTP by default",
			configBody: "directHost: http://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"headers:\n  X-Test-Secret: test-secret\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaDirectHostInvalid,
		},
		{
			name: "private key authentication rejects HTTP when explicitly disabled",
			configBody: "directHost: http://localhost:3000\n" +
				"allowInsecureHttp: false\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaDirectHostInvalid,
		},
		{
			name: "private key authentication allows HTTP when explicitly enabled",
			configBody: "directHost: http://localhost:3000\n" +
				"allowInsecureHttp: true\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"headers:\n  X-Test-Secret: test-secret\n",
			privateKey:     "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantHost:       "http://localhost:3000",
			wantAudience:   "did:web:gyoka.example.com#gyoka_editor",
			wantUserID:     "did:plc:ewvi7nxzyoun6zhxrhs64oiz",
			wantHeaders:    map[string]string{"X-Test-Secret": "test-secret"},
			wantAuthMode:   gyokaAuthInterService,
			wantIntervalMS: defaultGyokaMinRequestIntervalMS,
		},
		{
			name: "private key authentication resolves custom headers from environment",
			configBody: "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"headers:\n" +
				"  CF-Access-Client-Id: ${YUGE_TEST_GYOKA_ACCESS_ID}\n" +
				"  CF-Access-Client-Secret: ${YUGE_TEST_GYOKA_ACCESS_SECRET}\n",
			env: map[string]string{
				"YUGE_TEST_GYOKA_ACCESS_ID":     "gyoka-test-access-id-8041",
				"YUGE_TEST_GYOKA_ACCESS_SECRET": "gyoka-test-access-secret-2c57",
			},
			privateKey:   "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantHost:     "https://localhost:3000",
			wantAudience: "did:web:gyoka.example.com#gyoka_editor",
			wantUserID:   "did:plc:ewvi7nxzyoun6zhxrhs64oiz",
			wantHeaders: map[string]string{
				"CF-Access-Client-Id":     "gyoka-test-access-id-8041",
				"CF-Access-Client-Secret": "gyoka-test-access-secret-2c57",
			},
			wantAuthMode:   gyokaAuthInterService,
			wantIntervalMS: defaultGyokaMinRequestIntervalMS,
		},
		{
			name: "custom header requires environment variable",
			configBody: "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"headers:\n" +
				"  CF-Access-Client-Id: ${YUGE_TEST_GYOKA_ACCESS_ID_MISSING}\n",
			env:        map[string]string{"YUGE_TEST_GYOKA_ACCESS_ID_MISSING": ""},
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaHeaderEnvNotSet,
		},
		{
			name: "custom headers require private key authentication",
			configBody: "host: gyoka.example.com\n" +
				"userIdentity: yuge.bsky.social\n" +
				"headers:\n" +
				"  X-Gyoka-Test: test-value\n",
			appPassword: "app-password",
			wantErr:     ErrGyokaHeadersRequirePrivateKey,
		},
		{
			name: "authorization header is reserved",
			configBody: "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"headers:\n" +
				"  Authorization: Bearer custom-token\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaAuthorizationHeaderReserved,
		},
		{
			name: "invalid custom header name",
			configBody: "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n" +
				"headers:\n" +
				"  \"Bad Header\": value\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaConfigInvalid,
		},
		{
			name: "private key takes precedence over app password",
			configBody: "directHost: https://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			appPassword:    "app-password",
			privateKey:     "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantHost:       "https://localhost:3000",
			wantAudience:   "did:web:gyoka.example.com#gyoka_editor",
			wantUserID:     "did:plc:ewvi7nxzyoun6zhxrhs64oiz",
			wantAuthMode:   gyokaAuthInterService,
			wantIntervalMS: defaultGyokaMinRequestIntervalMS,
		},
		{
			name: "private key authentication requires audience",
			configBody: "directHost: http://localhost:3000\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaAudienceNotSet,
		},
		{
			name: "invalid private key",
			configBody: "directHost: http://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey: "invalid",
			wantErr:    ErrGyokaPrivateKeyInvalid,
		},
		{
			name: "private key authentication requires direct host",
			configBody: "audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaDirectHostNotSet,
		},
		{
			name: "private key authentication requires HTTP direct host",
			configBody: "directHost: gyoka.example.com\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey: "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantErr:    ErrGyokaDirectHostInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			configDir := t.TempDir()
			configBody := tt.configBody
			if tt.privateKey != "" {
				if err := os.WriteFile(filepath.Join(configDir, "private.key"), []byte(tt.privateKey), 0600); err != nil {
					t.Fatalf("write private key: %v", err)
				}
				configBody += "privateKeyFile: private.key\n"
			}
			if tt.configBody != "" {
				if err := os.WriteFile(filepath.Join(configDir, "gyoka.yaml"), []byte(configBody), 0600); err != nil {
					t.Fatalf("write gyoka config: %v", err)
				}
			}

			config, err := loadGyokaProjectionConfig(configDir, tt.appPassword)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("loadGyokaProjectionConfig() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadGyokaProjectionConfig() error = %v", err)
			}
			if config.host != tt.wantHost {
				t.Errorf("host = %q, want %q", config.host, tt.wantHost)
			}
			if config.userIdentity != tt.wantUserID {
				t.Errorf("userIdentity = %q, want %q", config.userIdentity, tt.wantUserID)
			}
			if config.audience != tt.wantAudience {
				t.Errorf("audience = %q, want %q", config.audience, tt.wantAudience)
			}
			for name, want := range tt.wantHeaders {
				if got := config.headers.Get(name); got != want {
					t.Errorf("headers[%q] = %q, want %q", name, got, want)
				}
			}
			if config.authMode != tt.wantAuthMode {
				t.Errorf("authMode = %v, want %v", config.authMode, tt.wantAuthMode)
			}
			if tt.wantAuthMode == gyokaAuthInterService && config.privateKey == nil {
				t.Error("privateKey = nil, want parsed private key")
			}
			if config.minRequestIntervalMS != tt.wantIntervalMS {
				t.Errorf("minRequestIntervalMS = %d, want %d", config.minRequestIntervalMS, tt.wantIntervalMS)
			}
		})
	}
}

func TestLoadGyokaProjectionConfigForStartup(t *testing.T) {
	configDir := t.TempDir()

	config, enabled, err := loadGyokaProjectionConfigForStartup(configDir, "")
	if !errors.Is(err, ErrGyokaConfigNotFound) {
		t.Fatalf("loadGyokaProjectionConfigForStartup() error = %v, want %v", err, ErrGyokaConfigNotFound)
	}
	if enabled {
		t.Fatal("loadGyokaProjectionConfigForStartup() enabled = true, want false")
	}
	if !reflect.DeepEqual(config, gyokaProjectionConfig{}) {
		t.Fatalf("loadGyokaProjectionConfigForStartup() config = %+v, want empty config", config)
	}

	if err := os.WriteFile(filepath.Join(configDir, "gyoka.yaml"), []byte("host: gyoka.example.com\nuserIdentity: yuge.bsky.social\n"), 0600); err != nil {
		t.Fatalf("write gyoka config: %v", err)
	}
	t.Setenv("GYOKA_PRIVATE_KEY", "invalid")
	config, enabled, err = loadGyokaProjectionConfigForStartup(configDir, "app-password")
	if err != nil {
		t.Fatalf("loadGyokaProjectionConfigForStartup() error = %v", err)
	}
	if !enabled {
		t.Fatal("loadGyokaProjectionConfigForStartup() enabled = false, want true")
	}
	if config.host != "gyoka.example.com" {
		t.Errorf("host = %q, want %q", config.host, "gyoka.example.com")
	}
}
