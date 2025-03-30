package logicblock

import (
	"log/slog"
	"testing"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/logic"
)

func TestRemoveLogicblock(t *testing.T) {
	tests := []struct {
		name     string
		config   logic.RemoveLogicBlockConfig
		post     *apibsky.FeedPost
		expected bool
	}{
		{
			name: "リプライの除外",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject": "item",
						"value":   "reply",
					},
				},
			},
			post: &apibsky.FeedPost{
				Reply: &apibsky.FeedPost_ReplyRef{},
			},
			expected: false,
		},
		{
			name: "==の場合、言語が一つでも一致した場合はfalse",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject":  "language",
						"language": "ja",
						"operator": "==",
					},
				},
			},
			post: &apibsky.FeedPost{
				Langs: []string{"ja", "en"},
			},
			expected: false,
		},
		{
			name: "!=の場合、言語が一つでも一致しなければfalse",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject":  "language",
						"language": "fr",
						"operator": "!=",
					},
				},
			},
			post: &apibsky.FeedPost{
				Langs: []string{"ja", "en"},
			},
			expected: false,
		},
		{
			name: "==の場合、言語が一致しない場合はtrue",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject":  "language",
						"language": "fr",
						"operator": "==",
					},
				},
			},
			post: &apibsky.FeedPost{
				Langs: []string{"ja", "en"},
			},
			expected: true,
		},
		{
			name: "!=の場合、言語が全て一致する場合はtrue",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject":  "language",
						"language": "fr",
						"operator": "!=",
					},
				},
			},
			post: &apibsky.FeedPost{
				Langs: []string{"fr"},
			},
			expected: true,
		},
		{
			name: "==の場合、言語が未指定の場合はtrue",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject":  "language",
						"language": "fr",
						"operator": "==",
					},
				},
			},
			post: &apibsky.FeedPost{
				Langs: nil,
			},
			expected: true,
		},
		{
			name: "!=の場合、言語が未指定の場合はfalse",
			config: logic.RemoveLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "remove",
					Options: map[string]interface{}{
						"subject":  "language",
						"language": "fr",
						"operator": "!=",
					},
				},
			},
			post: &apibsky.FeedPost{
				Langs: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.Default()
			block, err := NewRemoveLogicBlock(&tt.config, logger)
			if err != nil {
				t.Fatalf("failed to create remove logicblock: %v", err)
			}
			result := block.Test("testdid", "constantRkey", tt.post)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
