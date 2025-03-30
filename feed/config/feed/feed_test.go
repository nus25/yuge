package feed

import (
	"encoding/json"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/types"
)

func createMockConfigJSON(jsonstr string) (types.FeedConfig, error) {
	cfg := &FeedConfigImpl{}
	err := json.Unmarshal([]byte(jsonstr), &cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
func TestFeedConfig_ValidateYAML(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "正常系: 全てのフィールドが有効",
			config: `
logic:
  blocks:
    - type: regex
      options:
        value: test
        invert: false
        caseSensitive: false
store:
  trimAt: 120
  trimRemain: 100`,
			wantErr: false,
		},
		{
			name: "異常系: TrimAtがTrimRemainより小さい",
			config: `
logic:
  blocks:
    - type: regex
      options:
        value: test
        invert: false
        caseSensitive: false
store:
  trimAt: 50
  trimRemain: 100`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultFeedConfig()
			err := yaml.Unmarshal([]byte(tt.config), cfg)
			if err != nil {
				t.Fatalf("Failed to unmarshal config: %v", err)
			}

			err = cfg.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFeedConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name: "正常系: 全てのフィールドが有効",
			config: `{
				"logic": {
					"blocks": [
					{
						"type": "regex",
						"options": {
							"value": "test",
							"invert": false,
							"caseSensitive": false
						}
					}
				    ]},
				"store": {
					"trimAt": 120,
					"trimRemain": 100
				}
			}`,
			wantErr: false,
		},
		{
			//warnのみでエラーは出ない
			name: "異常系: TrimAtがTrimRemainより小さい",
			config: `{
				"logic": {
					"blocks": [
						{
							"type": "regex",
							"options": {
								"value": "test",
							    "invert": false,
							    "caseSensitive": false
						    }
						}
				    ]
				},
				"store": {
					"trimAt": 50,
					"trimRemain": 100
				}
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := createMockConfigJSON(tt.config)
			if err != nil {
				t.Fatalf("Failed to create mock config: %v", err)
			}
			err = cfg.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewFeedConfigFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantErr bool
	}{
		{
			name:    "正常系: 最小限の設定",
			jsonStr: `{}`,
			wantErr: false,
		},
		{
			name: "異常系: 不正なJSON形式",
			jsonStr: `{
				"logic": {
					"blocks": [
				}`,
			wantErr: true,
		},
		{
			name: "正常系: 一部の設定値",
			jsonStr: `{
				"store": {
					"trimAt": 100,
					"trimRemain": 50
				}
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewFeedConfigFromJSON(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFeedConfigFromJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && cfg == nil {
				t.Errorf("Expected non-nil config but got nil")
			}
			if cfg != nil {
				err = cfg.ValidateAll()
				if err != nil {
					t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

// TestFeedConfigDeepCopy テスト用の関数
func TestFeedConfigDeepCopy(t *testing.T) {
	// オリジナルの設定を作成
	original, err := NewFeedConfigFromJSON(`{
		"logic": {
			"blocks": [
				{
					"type": "regex",
					"options": {
						"value": "test",
						"invert": false,
						"caseSensitive": false
					}
				}
			]
		},
		"store": {
			"trimAt": 100,
			"trimRemain": 50
		},
		"detailedLog": true
	}`)
	if err != nil {
		t.Fatalf("Failed to create original config: %v", err)
	}

	// ディープコピーを実行
	copy := original.DeepCopy()

	// コピー後の値が同じかつことを確認
	// 値が同じことを確認
	if copy.DetailedLog() != original.DetailedLog() {
		t.Errorf("DetailedLog values don't match: original=%v, copy=%v", original.DetailedLog(), copy.DetailedLog())
	}

	if copy.Store().GetTrimAt() != original.Store().GetTrimAt() {
		t.Errorf("TrimAt values don't match: original=%v, copy=%v", original.Store().GetTrimAt(), copy.Store().GetTrimAt())
	}

	if copy.Store().GetTrimRemain() != original.Store().GetTrimRemain() {
		t.Errorf("TrimRemain values don't match: original=%v, copy=%v", original.Store().GetTrimRemain(), copy.Store().GetTrimRemain())
	}

	if len(copy.FeedLogic().GetLogicBlockConfigs()) != len(original.FeedLogic().GetLogicBlockConfigs()) {
		t.Errorf("Block count doesn't match: original=%v, copy=%v", len(original.FeedLogic().GetLogicBlockConfigs()), len(copy.FeedLogic().GetLogicBlockConfigs()))
	}

	// ポインタが異なることを確認
	if copy == original {
		t.Errorf("Copy and original have the same pointer: %p", original)
	}

	if copy.Store() == original.Store() {
		t.Errorf("Store pointers are the same: %p", original.Store())
	}

	if copy.FeedLogic() == original.FeedLogic() {
		t.Errorf("FeedLogic pointers are the same: %p", original.FeedLogic())
	}
}
