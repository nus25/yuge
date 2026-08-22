package subscriber

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGyokaProjectionConfig(t *testing.T) {
	tests := []struct {
		name           string
		configBody     string
		appPassword    string
		wantErr        error
		wantHost       string
		wantUserID     string
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
			wantErr: ErrGyokaAppPasswordNotSet,
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
			wantIntervalMS: 2500,
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
			if config.minRequestIntervalMS != tt.wantIntervalMS {
				t.Errorf("minRequestIntervalMS = %d, want %d", config.minRequestIntervalMS, tt.wantIntervalMS)
			}
		})
	}
}
