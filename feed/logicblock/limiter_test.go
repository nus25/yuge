package logicblock

import (
	"log/slog"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/errors"
)

// テストヘルパー関数
func createLimiterLogicBlock(t *testing.T, count int, window, cleanup time.Duration) (*LimiterLogicblock, error) {
	t.Helper()
	cfg := &logic.LimiterLogicBlockConfig{
		BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
			BlockType: "limiter",
			Options: map[string]interface{}{
				"count":       count,
				"timeWindow":  window,
				"cleanupFreq": cleanup,
			},
		},
	}
	logger := slog.Default()
	block, err := NewLimiterLogicBlock(cfg, logger)
	if err != nil {
		return nil, err
	}
	return block.(*LimiterLogicblock), nil
}

func TestLimiterLogicblock_Test(t *testing.T) {
	tests := []struct {
		name     string
		did      string
		count    int
		window   time.Duration
		cleanup  time.Duration
		wantPass bool
		setup    func(*LimiterLogicblock, string) // セットアップ関数
	}{
		{
			name:     "正常系: 制限内の投稿",
			did:      "did:example:user1",
			count:    5,
			window:   1 * time.Hour,
			cleanup:  10 * time.Minute,
			wantPass: true,
		},
		{
			name:     "異常系: 制限を超えた投稿",
			did:      "did:example:user2",
			count:    2,
			window:   1 * time.Hour,
			cleanup:  10 * time.Minute,
			wantPass: false,
			setup: func(lb *LimiterLogicblock, did string) {
				post := &bsky.FeedPost{}
				for i := 0; i < 2; i++ {
					lb.Test(did, "constantRkey", post)
				}
			},
		},
		{
			name:     "正常系: 時間経過後のリセット",
			did:      "did:example:user4",
			count:    1,
			window:   1 * time.Second,
			cleanup:  2 * time.Second,
			wantPass: true,
			setup: func(lb *LimiterLogicblock, did string) {
				post := &bsky.FeedPost{}
				lb.Test(did, "constantRkey", post)
				time.Sleep(2 * time.Second) // 時間枠を超えて待機
			},
		},
		{
			name:     "正常系: 異なるユーザーは独立してカウント",
			did:      "did:example:user5",
			count:    1,
			window:   1 * time.Hour,
			cleanup:  10 * time.Minute,
			wantPass: true,
			setup: func(lb *LimiterLogicblock, did string) {
				post := &bsky.FeedPost{}
				// 別のユーザーで制限まで投稿
				otherDid := "did:example:other"
				lb.Test(otherDid, "constantRkey", post)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb, err := createLimiterLogicBlock(t, tt.count, tt.window, tt.cleanup)
			if err != nil {
				if _, ok := err.(*errors.ConfigError); !ok {
					t.Logf("expected error: %v", err)
					return
				} else if tt.wantPass {
					t.Fatalf("failed to create limiter logicblock: %v", err)
				}
			}

			if tt.setup != nil {
				tt.setup(lb, tt.did)
			}

			post := &bsky.FeedPost{}
			result := lb.Test(tt.did, "constantRkey", post)
			if result != tt.wantPass {
				t.Errorf("Test() = %v, want %v", result, tt.wantPass)
			}
		})
	}
}

func TestLimiterLogicblockInvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		count       int
		window      time.Duration
		cleanup     time.Duration
		expectedErr bool
	}{
		{
			name:        "負の制限回数",
			count:       -1,
			window:      1 * time.Hour,
			cleanup:     10 * time.Minute,
			expectedErr: true,
		},
		{
			name:        "ゼロの時間枠",
			count:       5,
			window:      0,
			cleanup:     10 * time.Minute,
			expectedErr: true,
		},
		{
			name:        "0.9秒の時間枠",
			count:       5,
			window:      900 * time.Millisecond,
			cleanup:     10 * time.Minute,
			expectedErr: true,
		},
		{
			name:        "負の時間枠",
			count:       5,
			window:      -1 * time.Hour,
			cleanup:     10 * time.Minute,
			expectedErr: true,
		},
		{
			name:        "負のクリーンアップ間隔",
			count:       5,
			window:      1 * time.Hour,
			cleanup:     -10 * time.Minute,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &logic.LimiterLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "limiter",
					Options: map[string]interface{}{
						"count":       tt.count,
						"timeWindow":  tt.window,
						"cleanupFreq": tt.cleanup,
					},
				},
			}
			logger := slog.Default()
			_, err := NewLimiterLogicBlock(cfg, logger)
			if (err != nil) != tt.expectedErr {
				t.Errorf("NewLimiterLogicblock() error = %v, wantErr %v", err, tt.expectedErr)
			}
		})
	}
}
