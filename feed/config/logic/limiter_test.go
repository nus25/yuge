package logic

import (
	"testing"
	"time"
)

func TestLimiterLogicBlockConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name    string
		config  *BaseLogicBlockConfig
		wantErr bool
	}{
		{
			name: "正常系: 全ての必須フィールドが設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"count":       5,
					"timeWindow":  "1h",
					"cleanupFreq": "10m",
				},
			},
			wantErr: false,
		},
		{
			name: "異常系: countが設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"timeWindow":  "1h",
					"cleanupFreq": "10m",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: countが0以下",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"count":       0,
					"timeWindow":  "1h",
					"cleanupFreq": "10m",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系： countが文字列",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"count":       "invalid",
					"timeWindow":  "1h",
					"cleanupFreq": "10m",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: timeWindowが設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"count":       5,
					"cleanupFreq": "10m",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: timeWindowが文字列",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"count":       5,
					"timeWindow":  "invalid",
					"cleanupFreq": "10m",
				},
			},
			wantErr: true,
		},
		{
			name: "正常系: cleanupFreqが設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"count":      5,
					"timeWindow": "1h",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := (&LimiterLogicBlockFactory{}).Create(*tt.config)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			err = cfg.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLimiterLogicBlockConfig_Validate(t *testing.T) {
	config, err := (&LimiterLogicBlockFactory{}).Create(BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"count":       5,
			"timeWindow":  "1h",
			"cleanupFreq": "10m",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name    string
		key     string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "正常系: 有効なcount",
			key:     LimiterOptionCount,
			value:   5,
			wantErr: false,
		},
		{
			name:    "異常系: 無効なcount",
			key:     LimiterOptionCount,
			value:   0,
			wantErr: true,
		},
		{
			name:    "正常系: 有効なtimeWindow",
			key:     LimiterOptionTimeWindow,
			value:   1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "異常系: 無効なtimeWindow",
			key:     LimiterOptionTimeWindow,
			value:   0 * time.Second,
			wantErr: true,
		},
		{
			name:    "正常系: 有効なcleanupFreq",
			key:     LimiterOptionCleanupFreq,
			value:   10 * time.Minute,
			wantErr: false,
		},
		{
			name:    "異常系: 無効なcleanupFreq",
			key:     LimiterOptionCleanupFreq,
			value:   -1 * time.Second,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
