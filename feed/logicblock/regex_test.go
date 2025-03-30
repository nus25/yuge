package logicblock

import (
	"log/slog"
	"testing"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/logic"
)

func TestRegexLogicblock(t *testing.T) {
	tests := []struct {
		name     string
		config   logic.RegexLogicBlockConfig
		post     *apibsky.FeedPost
		expected bool
	}{
		{
			name: "Basic matching",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "This is a test message",
			},
			expected: true,
		},
		{
			name: "Case insensitive matching",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "This is a TEST message",
			},
			expected: true,
		},
		{
			name: "Case sensitive matching",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": true,
						"invert":        false,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "This is a TEST message",
			},
			expected: false,
		},
		{
			name: "Invert match result",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": false,
						"invert":        true,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "This is a test message",
			},
			expected: false,
		},
		{
			name: "Empty text",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "",
			},
			expected: false,
		},
		{
			name: "Pattern with special characters",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         `\d+`,
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "Contains 123 numbers",
			},
			expected: true,
		},
		{
			name: "Multi-line text",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			post: &apibsky.FeedPost{
				Text: "First line\ntest on second line\nthird line",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.Default()
			block, err := NewRegexLogicBlock(&tt.config, logger)
			if err != nil {
				t.Fatalf("failed to create regex logicblock: %v", err)
			}
			result := block.Test("testdid", "constantRkey", tt.post)
			if result != tt.expected {
				t.Errorf("%s: expected %v, got %v for text: %q", tt.name, tt.expected, result, tt.post.Text)
			}
		})
	}
}

func TestRegexLogicblockInvalidConfig(t *testing.T) {
	invalidConfigs := []struct {
		name        string
		config      logic.RegexLogicBlockConfig
		expectedErr bool
	}{
		{
			name: "Missing required option (value)",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Missing required option (caseSensitive)",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":  "test",
						"invert": false,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Missing required option (invert)",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "test",
						"caseSensitive": false,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Empty regex pattern",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "",
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Invalid regex pattern",
			config: logic.RegexLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "regex",
					Options: map[string]interface{}{
						"value":         "[",
						"caseSensitive": false,
						"invert":        false,
					},
				},
			},
			expectedErr: true,
		},
	}

	for _, tt := range invalidConfigs {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.Default()
			_, err := NewRegexLogicBlock(&tt.config, logger)
			if (err != nil) != tt.expectedErr {
				t.Errorf("expected error: %v, got: %v", tt.expectedErr, err)
			}
		})
	}
}
