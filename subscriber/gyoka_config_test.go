package subscriber

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGyokaAuthModeString(t *testing.T) {
	tests := []struct {
		mode gyokaAuthMode
		want string
	}{
		{mode: gyokaAuthDisabled, want: "disabled"},
		{mode: gyokaAuthPassword, want: "pds_proxy_password"},
		{mode: gyokaAuthInterService, want: "direct_inter_service"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadGyokaProjectionConfig(t *testing.T) {
	tests := []struct {
		name           string
		configBody     string
		appPassword    string
		privateKey     string
		wantErr        error
		wantHost       string
		wantAudience   string
		wantUserID     string
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
			configBody: "directHost: http://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			privateKey:     "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantHost:       "http://localhost:3000",
			wantAudience:   "did:web:gyoka.example.com#gyoka_editor",
			wantUserID:     "did:plc:ewvi7nxzyoun6zhxrhs64oiz",
			wantAuthMode:   gyokaAuthInterService,
			wantIntervalMS: defaultGyokaMinRequestIntervalMS,
		},
		{
			name: "private key takes precedence over app password",
			configBody: "directHost: http://localhost:3000\n" +
				"audience: did:web:gyoka.example.com#gyoka_editor\n" +
				"userIdentity: did:plc:ewvi7nxzyoun6zhxrhs64oiz\n",
			appPassword:    "app-password",
			privateKey:     "z42tvqQS5sVhaV1jLZ5P6ZKEPEbSpYavNVmT88YDYV3MEZ8D",
			wantHost:       "http://localhost:3000",
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
			configDir := t.TempDir()
			if tt.configBody != "" {
				if err := os.WriteFile(filepath.Join(configDir, "gyoka.yaml"), []byte(tt.configBody), 0600); err != nil {
					t.Fatalf("write gyoka config: %v", err)
				}
			}

			config, err := loadGyokaProjectionConfig(configDir, tt.appPassword, tt.privateKey)
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

	config, enabled, err := loadGyokaProjectionConfigForStartup(configDir, "", "")
	if !errors.Is(err, ErrGyokaConfigNotFound) {
		t.Fatalf("loadGyokaProjectionConfigForStartup() error = %v, want %v", err, ErrGyokaConfigNotFound)
	}
	if enabled {
		t.Fatal("loadGyokaProjectionConfigForStartup() enabled = true, want false")
	}
	if config != (gyokaProjectionConfig{}) {
		t.Fatalf("loadGyokaProjectionConfigForStartup() config = %+v, want empty config", config)
	}

	if err := os.WriteFile(filepath.Join(configDir, "gyoka.yaml"), []byte("host: gyoka.example.com\nuserIdentity: yuge.bsky.social\n"), 0600); err != nil {
		t.Fatalf("write gyoka config: %v", err)
	}
	config, enabled, err = loadGyokaProjectionConfigForStartup(configDir, "app-password", "")
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
