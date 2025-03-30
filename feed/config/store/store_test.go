package store

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/goccy/go-yaml"
	yugeErrors "github.com/nus25/yuge/feed/errors"
)

func TestStoreConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name           string
		config         *StoreConfigImpl
		wantErr        bool
		wantErrType    error
		wantComponent  string
		wantKey        string
		wantErrMessage string
	}{
		{
			name: "正常系: 有効な設定",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: 50,
			},
			wantErr: false,
		},
		{
			name: "異常系: TrimAtが0",
			config: &StoreConfigImpl{
				TrimAt:     0,
				TrimRemain: 50,
			},
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "StoreConfig",
			wantKey:        "trimAt",
			wantErrMessage: "trimAt must be greater than 0",
		},
		{
			name: "異常系: TrimRemainが負数",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: -1,
			},
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "StoreConfig",
			wantKey:        "trimRemain",
			wantErrMessage: "trimRemain must be greater than or equal to 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				var configErr *yugeErrors.ConfigError
				if !errors.As(err, &configErr) {
					t.Errorf("expected ConfigError, got %T", err)
					return
				}
				if configErr.Component != tt.wantComponent {
					t.Errorf("expected Component %q, got %q", tt.wantComponent, configErr.Component)
				}
				if configErr.Key != tt.wantKey {
					t.Errorf("expected Key %q, got %q", tt.wantKey, configErr.Key)
				}
				if configErr.Message != tt.wantErrMessage {
					t.Errorf("expected Message %q, got %q", tt.wantErrMessage, configErr.Message)
				}
			}
		})
	}
}

func TestStoreConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		config         *StoreConfigImpl
		key            string
		value          interface{}
		wantErr        bool
		wantErrType    error
		wantComponent  string
		wantKey        string
		wantErrMessage string
	}{
		{
			name: "正常系: 有効なtrimAt",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: 50,
			},
			key:     "trimAt",
			value:   200,
			wantErr: false,
		},
		{
			name: "異常系: 無効なtrimAt",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: 50,
			},
			key:            "trimAt",
			value:          0,
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "StoreConfig",
			wantKey:        "trimAt",
			wantErrMessage: "trimAt must be greater than 0",
		},
		{
			name: "異常系: 無効なtrimRemain",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: 50,
			},
			key:            "trimRemain",
			value:          -1,
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "StoreConfig",
			wantKey:        "trimRemain",
			wantErrMessage: "trimRemain must be greater than or equal to 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				var configErr *yugeErrors.ConfigError
				if !errors.As(err, &configErr) {
					t.Errorf("expected ConfigError, got %T", err)
					return
				}
				if configErr.Component != tt.wantComponent {
					t.Errorf("expected Component %q, got %q", tt.wantComponent, configErr.Component)
				}
				if configErr.Key != tt.wantKey {
					t.Errorf("expected Key %q, got %q", tt.wantKey, configErr.Key)
				}
				if configErr.Message != tt.wantErrMessage {
					t.Errorf("expected Message %q, got %q", tt.wantErrMessage, configErr.Message)
				}
			}
		})
	}
}

func TestStoreConfig_MarshalUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		config  *StoreConfigImpl
		wantErr bool
	}{
		{
			name: "正常系: 基本的な設定",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: 50,
			},
			wantErr: false,
		},
		{
			name: "正常系: デフォルト値",
			config: &StoreConfigImpl{
				TrimAt:     2000,
				TrimRemain: 1500,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON Marshal
			jsonData, err := json.Marshal(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Test JSON Unmarshal
			var unmarshaled StoreConfigImpl
			err = json.Unmarshal(jsonData, &unmarshaled)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Compare original and unmarshaled
			if tt.config.TrimAt != unmarshaled.TrimAt {
				t.Errorf("TrimAt mismatch: got %v, want %v", unmarshaled.TrimAt, tt.config.TrimAt)
			}
			if tt.config.TrimRemain != unmarshaled.TrimRemain {
				t.Errorf("TrimRemain mismatch: got %v, want %v", unmarshaled.TrimRemain, tt.config.TrimRemain)
			}
		})
	}
}

func TestStoreConfig_MarshalUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		config  *StoreConfigImpl
		wantErr bool
	}{
		{
			name: "正常系: 基本的な設定",
			config: &StoreConfigImpl{
				TrimAt:     100,
				TrimRemain: 50,
			},
			wantErr: false,
		},
		{
			name: "正常系: デフォルト値",
			config: &StoreConfigImpl{
				TrimAt:     2000,
				TrimRemain: 1500,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test YAML Marshal
			yamlData, err := yaml.Marshal(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Test YAML Unmarshal
			var unmarshaled StoreConfigImpl
			err = yaml.Unmarshal(yamlData, &unmarshaled)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Compare original and unmarshaled
			if tt.config.TrimAt != unmarshaled.TrimAt {
				t.Errorf("TrimAt mismatch: got %v, want %v", unmarshaled.TrimAt, tt.config.TrimAt)
			}
			if tt.config.TrimRemain != unmarshaled.TrimRemain {
				t.Errorf("TrimRemain mismatch: got %v, want %v", unmarshaled.TrimRemain, tt.config.TrimRemain)
			}
		})
	}
}
