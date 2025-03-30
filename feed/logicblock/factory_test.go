package logicblock

import (
	"log/slog"
	"testing"

	"github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
)

func TestLogicblockFactory(t *testing.T) {
	logger := slog.Default()
	factory := FactoryInstance()

	tests := []struct {
		name        string
		config      types.LogicBlockConfig
		expectError bool
	}{
		{
			name: "正規表現ブロックの作成",
			config: &logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": true,
						"invert":        false,
					},
				},
			},
			expectError: false,
		},
		{
			name: "除外ブロックの作成",
			config: &logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject": "item",
						"value":   "reply",
					},
				},
			},
			expectError: false,
		},
		{
			name: "不正なブロックタイプ",
			config: &logic.BaseLogicBlockConfig{
				BlockType: "invalid",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, err := factory.Create(tt.config, logger)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
				if block != nil {
					t.Error("expected nil block but got non-nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if block == nil {
					t.Error("expected non-nil block but got nil")
				}
			}
		})
	}
}
