package logic

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/nus25/yuge/feed/config/types"
	yugeErrors "github.com/nus25/yuge/feed/errors"
)

func TestFeedLogicConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		wantErr        bool
		wantErrType    error
		wantComponent  string
		wantKey        string
		wantErrMessage string
	}{
		{
			name: "正常系: 複数の有効なロジックブロック",
			config: `blocks:
- type: regex
  options:
    value: test
    invert: false
    caseSensitive: true
- type: remove
  options:
    subject: language
    language: en
    operator: ==
`,
			wantErr: false,
		},
		{
			name:           "正常系: ロジックブロックが空",
			config:         `blocks:`,
			wantErr:        false,
			wantErrType:    nil,
			wantComponent:  "FeedLogic",
			wantKey:        "blocks",
			wantErrMessage: "",
		},
		{
			name: "異常系: 無効なブロックタイプ",
			config: `blocks:
- type: regex
  options:
    value: test
    invert: invalid
    caseSensitive: true
`,
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "FeedLogic",
			wantKey:        "logicBlocks[0]",
			wantErrMessage: "invalid logic block: validation failed for invert: must be a boolean or a string that can be converted to a boolean (got: invalid)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			cfg := FeedLogicConfigimpl{}
			err = yaml.Unmarshal([]byte(tt.config), &cfg)
			if err != nil {
				t.Fatalf("Failed to unmarshal config: %v", err)
			}
			err = cfg.ValidateAll()
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

func TestFeedLogicConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		config         *FeedLogicConfigimpl
		key            string
		value          interface{}
		wantErr        bool
		wantErrType    error
		wantComponent  string
		wantKey        string
		wantErrMessage string
	}{
		{
			name:   "正常系: 有効なロジックブロック",
			config: &FeedLogicConfigimpl{},
			key:    "logicBlocks",
			value: []types.LogicBlockConfig{
				&RegexLogicBlockConfig{
					BaseLogicBlockConfig: BaseLogicBlockConfig{
						BlockType: "regex",
						Options: map[string]interface{}{
							"value":         "test",
							"invert":        false,
							"caseSensitive": true,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name:           "異常系: 空のロジックブロック",
			config:         &FeedLogicConfigimpl{},
			key:            "blocks",
			value:          []types.LogicBlockConfig{},
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "FeedLogic",
			wantKey:        "blocks",
			wantErrMessage: "at least one logic block is required",
		},
		{
			name:           "異常系: 無効な型",
			config:         &FeedLogicConfigimpl{},
			key:            "blocks",
			value:          "invalid",
			wantErr:        true,
			wantErrType:    &yugeErrors.ConfigError{},
			wantComponent:  "FeedLogic",
			wantKey:        "blocks",
			wantErrMessage: "invalid type for logicBlocks: expected []LogicBlockConfig",
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

func TestFeedLogicConfigimpl_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		config  *FeedLogicConfigimpl
		want    string
		wantErr bool
	}{
		{
			name: "正常系: 空のブロック",
			config: &FeedLogicConfigimpl{
				LogicBlocks: []types.LogicBlockConfig{},
			},
			want:    `{"blocks":[]}`,
			wantErr: false,
		},
		{
			name: "正常系: 1つのブロック",
			config: &FeedLogicConfigimpl{
				LogicBlocks: []types.LogicBlockConfig{
					&CustomLogicBlockConfig{
						BaseLogicBlockConfig: BaseLogicBlockConfig{
							BlockType: "custom",
							Options: map[string]interface{}{
								"key": "value",
							},
						},
					},
				},
			},
			want:    `{"blocks":[{"type":"custom","options":{"key":"value"}}]}`,
			wantErr: false,
		},
		{
			name: "正常系: 複数のブロック",
			config: &FeedLogicConfigimpl{
				LogicBlocks: []types.LogicBlockConfig{
					&CustomLogicBlockConfig{
						BaseLogicBlockConfig: BaseLogicBlockConfig{
							BlockType: "custom1",
							Options: map[string]interface{}{
								"key1": "value1",
							},
						},
					},
					&CustomLogicBlockConfig{
						BaseLogicBlockConfig: BaseLogicBlockConfig{
							BlockType: "custom2",
							Options: map[string]interface{}{
								"key2": "value2",
							},
						},
					},
				},
			},
			want:    `{"blocks":[{"type":"custom1","options":{"key1":"value1"}},{"type":"custom2","options":{"key2":"value2"}}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("MarshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestFeedLogicConfigimpl_DeepCopy(t *testing.T) {
	// Create a test config with some logic blocks
	original := &FeedLogicConfigimpl{
		LogicBlocks: []types.LogicBlockConfig{
			&BaseLogicBlockConfig{
				BlockType: "test1",
				Options: map[string]interface{}{
					"key1": "value1",
				},
			},
			&BaseLogicBlockConfig{
				BlockType: "test2",
				Options: map[string]interface{}{
					"key2": "value2",
				},
			},
		},
	}

	// Create a deep copy
	copiedInterface := original.DeepCopy()
	copied, ok := copiedInterface.(*FeedLogicConfigimpl)
	if !ok {
		t.Fatalf("DeepCopy() returned wrong type: %T", copiedInterface)
	}

	// Check that the copy has the same number of blocks
	if len(copied.LogicBlocks) != len(original.LogicBlocks) {
		t.Errorf("DeepCopy() LogicBlocks length = %v, want %v", len(copied.LogicBlocks), len(original.LogicBlocks))
	}

	// Check that each block has the correct type and options
	for i, block := range original.LogicBlocks {
		if copied.LogicBlocks[i].GetBlockType() != block.GetBlockType() {
			t.Errorf("DeepCopy() LogicBlocks[%d].BlockType = %v, want %v",
				i, copied.LogicBlocks[i].GetBlockType(), block.GetBlockType())
		}

		// Check options
		originalBlock, _ := block.(*BaseLogicBlockConfig)
		copiedBlock, _ := copied.LogicBlocks[i].(*BaseLogicBlockConfig)

		for key, val := range originalBlock.Options {
			if copiedBlock.Options[key] != val {
				t.Errorf("DeepCopy() LogicBlocks[%d].Options[%s] = %v, want %v",
					i, key, copiedBlock.Options[key], val)
			}
		}
	}

	// Verify it's a deep copy by modifying the original
	originalBlock, _ := original.LogicBlocks[0].(*BaseLogicBlockConfig)
	originalBlock.Options["key1"] = "modified"

	copiedBlock, _ := copied.LogicBlocks[0].(*BaseLogicBlockConfig)
	if copiedBlock.Options["key1"] == "modified" {
		t.Errorf("DeepCopy() didn't create a deep copy, changes to original affected the copy")
	}
}

func TestFeedLogicConfigimpl_GetLogicBlockConfigs(t *testing.T) {
	// Create a test config with some logic blocks
	config := &FeedLogicConfigimpl{
		LogicBlocks: []types.LogicBlockConfig{
			&BaseLogicBlockConfig{BlockType: "test1"},
			&BaseLogicBlockConfig{BlockType: "test2"},
		},
	}

	// Get the logic blocks
	blocks := config.GetLogicBlockConfigs()

	// Check that the returned blocks are the same as the original
	if len(blocks) != len(config.LogicBlocks) {
		t.Errorf("GetLogicBlockConfigs() returned %v blocks, want %v", len(blocks), len(config.LogicBlocks))
	}

	for i, block := range blocks {
		if block.GetBlockType() != config.LogicBlocks[i].GetBlockType() {
			t.Errorf("GetLogicBlockConfigs()[%d].BlockType = %v, want %v",
				i, block.GetBlockType(), config.LogicBlocks[i].GetBlockType())
		}
	}
}

func TestFeedLogicConfigimpl_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *FeedLogicConfigimpl)
	}{
		{
			name: "正常系: 有効なJSON",
			json: `{
				"blocks": [
					{
						"type": "regex",
						"options": {
							"value": "test",
							"invert": false
						}
					},
					{
						"type": "remove",
						"options": {
							"subject": "language",
							"language": "en"
						}
					}
				]
			}`,
			wantErr: false,
			check: func(t *testing.T, config *FeedLogicConfigimpl) {
				if len(config.LogicBlocks) != 2 {
					t.Errorf("UnmarshalJSON() LogicBlocks length = %v, want 2", len(config.LogicBlocks))
				}
				if config.LogicBlocks[0].GetBlockType() != "regex" {
					t.Errorf("UnmarshalJSON() LogicBlocks[0].BlockType = %v, want regex",
						config.LogicBlocks[0].GetBlockType())
				}
				if config.LogicBlocks[1].GetBlockType() != "remove" {
					t.Errorf("UnmarshalJSON() LogicBlocks[1].BlockType = %v, want remove",
						config.LogicBlocks[1].GetBlockType())
				}
			},
		},
		{
			name:    "異常系: 無効なJSON",
			json:    `{invalid json`,
			wantErr: true,
		},
		{
			name: "異常系: 無効なブロックタイプ",
			json: `{
				"blocks": [
					{
						"type": "invalid_type",
						"options": {}
					}
				]
			}`,
			wantErr: false, // カスタムブロックとして処理されるため
			check: func(t *testing.T, config *FeedLogicConfigimpl) {
				if len(config.LogicBlocks) != 1 {
					t.Errorf("UnmarshalJSON() LogicBlocks length = %v, want 1", len(config.LogicBlocks))
				}
				if _, ok := config.LogicBlocks[0].(*CustomLogicBlockConfig); !ok {
					t.Errorf("UnmarshalJSON() LogicBlocks[0] type = %T, want *CustomLogicBlockConfig",
						config.LogicBlocks[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &FeedLogicConfigimpl{}
			err := config.UnmarshalJSON([]byte(tt.json))

			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tt.check != nil {
				tt.check(t, config)
			}
		})
	}
}
