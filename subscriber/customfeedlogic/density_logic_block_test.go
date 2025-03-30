package customfeedlogic

import (
	"context"
	"log/slog"
	"testing"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/logic"
)

func TestDensityLogicBlock(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		threshold int
		want      bool
	}{
		{
			name:      "empty text",
			text:      "",
			threshold: 10,
			want:      false,
		},
		{
			name:      "text with less unique chars than threshold",
			text:      "aaaaaaaa", // 1 unique char
			threshold: 10,
			want:      false,
		},
		{
			name:      "text with equal unique chars to threshold",
			text:      "abcdefghij", // 10 unique chars
			threshold: 10,
			want:      true,
		},
		{
			name:      "text with more unique chars than threshold",
			text:      "abcdefghijklmnop", // 16 unique chars
			threshold: 10,
			want:      true,
		},
		{
			name:      "text with emojis",
			text:      "abc😀def😎ghi", // 9 unique chars after emoji removal
			threshold: 10,
			want:      false,
		},
		{
			name:      "japanese text",
			text:      "こんにちは世界世界世界世界", // 7 unique chars
			threshold: 8,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test config
			cfg := &logic.BaseLogicBlockConfig{
				BlockType: BlockTypeDensity,
				Options: map[string]interface{}{
					"threshold": tt.threshold,
				},
			}

			// Create logic block
			lb, err := NewDensityLogicBlock(cfg, slog.Default())
			if err != nil {
				t.Fatalf("failed to create density logic block: %v", err)
			}

			// Create test post
			post := &apibsky.FeedPost{
				Text: tt.text,
			}

			// Test
			got := lb.Test("test-did", "constantRkey", post)
			if got != tt.want {
				t.Errorf("DensityLogicBlock.Test() = %v, want %v", got, tt.want)
			}

			if err := lb.Shutdown(context.Background()); err != nil {
				t.Errorf("failed to shutdown: %v", err)
			}
			if err := lb.Reset(); err != nil {
				t.Errorf("failed to reset: %v", err)
			}
		})
	}
}

func TestDensityLogicBlock_DefaultThreshold(t *testing.T) {
	cfg := &logic.BaseLogicBlockConfig{
		BlockType: BlockTypeDensity,
	}

	lb, err := NewDensityLogicBlock(cfg, slog.Default())
	if err != nil {
		t.Fatalf("failed to create density logic block: %v", err)
	}
	post := &apibsky.FeedPost{
		Text: "こんにちは世界1234", // 11 unique chars
	}

	got := lb.Test("test-did", "constantRkey", post)
	if got != true {
		t.Errorf("DensityLogicBlock.Test() = %v, want %v", got, true)
	}
}

func TestDensityLogicBlock_InvalidThreshold(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "10 unique chars should pass with invalid threshold",
			text: "こんにちは世界123", // 10 unique chars
			want: true,
		},
		{
			name: "9 unique chars should not pass with invalid threshold",
			text: "こんにちは世界12", // 9 unique chars
			want: false,
		},
	}

	cfg := &logic.BaseLogicBlockConfig{
		BlockType: BlockTypeDensity,
		Options: map[string]interface{}{
			"threshold": "invalid",
		},
	}

	lb, err := NewDensityLogicBlock(cfg, slog.Default())
	if err != nil {
		t.Fatalf("failed to create density logic block: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			post := &apibsky.FeedPost{
				Text: tt.text,
			}
			got := lb.Test("test-did", "constantRkey", post)
			if got != tt.want {
				t.Errorf("DensityLogicBlock.Test() = %v, want %v", got, tt.want)
			}
		})
	}
}
