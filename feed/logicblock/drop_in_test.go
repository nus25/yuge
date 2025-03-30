package logicblock

import (
	"context"
	"log/slog"
	"testing"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	config "github.com/nus25/yuge/feed/config/logic"
)

func TestNewDropInLogicBlock(t *testing.T) {
	logger := slog.Default()

	t.Run("正常系", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					config.DropInOptionTargetWord:     []string{"Test"},
					config.DropInOptionCancelWord:     []string{"CancEl"},
					config.DropInOptionIgnoreWord:     []string{"ignore"},
					config.DropInOptionExpireDuration: time.Hour,
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		dropIn, ok := block.(*DropInLogicblock)
		if !ok {
			t.Error("expected DropInLogicblock type")
		}

		if dropIn.blockType != BlockTypeDropIn {
			t.Errorf("expected name %s, got %s", BlockTypeDropIn, dropIn.blockType)
		}

		if len(dropIn.targetWord) != 1 || dropIn.targetWord[0] != "test" {
			t.Errorf("expected targetWord [test], got %v", dropIn.targetWord)
		}

		if len(dropIn.cancelWord) != 1 || dropIn.cancelWord[0] != "cancel" {
			t.Errorf("expected cancelWord [cancel], got %v", dropIn.cancelWord)
		}

		if len(dropIn.ignoreWord) != 1 || dropIn.ignoreWord[0] != "ignore" {
			t.Errorf("expected ignoreWord [ignore], got %v", dropIn.ignoreWord)
		}

		if dropIn.expireDuration != time.Hour {
			t.Errorf("expected expireDuration 1h, got %v", dropIn.expireDuration)
		}
	})

	t.Run("異常系_不正なBlockType", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: "invalid",
			},
		}

		_, err := NewDropInLogicBlock(cfg, logger)
		if err == nil {
			t.Error("expected error but got nil")
		}
	})

	t.Run("異常系_不正な設定タイプ", func(t *testing.T) {
		cfg := &config.BaseLogicBlockConfig{
			BlockType: BlockTypeDropIn,
		}

		_, err := NewDropInLogicBlock(cfg, logger)
		if err == nil {
			t.Error("expected error but got nil")
		}
	})

	t.Run("異常系_targetWord未設定", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options:   map[string]interface{}{},
			},
		}

		_, err := NewDropInLogicBlock(cfg, logger)
		if err == nil {
			t.Error("expected error but got nil")
		}
	})

	t.Run("異常系_targetWord空配列", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options:   map[string]interface{}{},
			},
		}

		_, err := NewDropInLogicBlock(cfg, logger)
		if err == nil {
			t.Error("expected error but got nil")
		}
	})
}

func TestDropInLogicblock_Test(t *testing.T) {
	logger := slog.Default()

	t.Run("正常系_targetWordのみ設定", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// Test with matching post
		post := &apibsky.FeedPost{
			Text: "Hello world",
		}
		if !block.Test("did1", "rkey1", post) {
			t.Error("expected true but got false")
		}

		// Test with non-matching post
		post = &apibsky.FeedPost{
			Text: "world",
		}
		if block.Test("did1", "rkey1", post) {
			t.Error("expected false but got true")
		}
	})

	t.Run("正常系_全オプション設定", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord:     []string{"hello"},
					config.DropInOptionCancelWord:     []string{"bye"},
					config.DropInOptionIgnoreWord:     []string{"ignore"},
					config.DropInOptionExpireDuration: time.Hour,
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// Test with matching post
		post := &apibsky.FeedPost{
			Text: "hello world",
		}
		if !block.Test("did1", "rkey1", post) {
			t.Error("expected true but got false")
		}

		// Test with ignore word
		post = &apibsky.FeedPost{
			Text: "hello ignore",
		}
		if block.Test("did1", "rkey1", post) {
			t.Error("expected false but got true")
		}

		// Test with cancel word
		post = &apibsky.FeedPost{
			Text: "bye world",
		}
		if block.Test("did1", "rkey1", post) {
			t.Error("expected false but got true")
		}

		// Test with non-matching post
		post = &apibsky.FeedPost{
			Text: "world",
		}
		if block.Test("did1", "rkey1", post) {
			t.Error("expected false but got true")
		}
	})

	t.Run("正常系_watchlist期限切れ", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord:     []string{"hello"},
					config.DropInOptionExpireDuration: time.Millisecond,
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// Add to watchlist
		post := &apibsky.FeedPost{
			Text: "hello world",
		}
		if !block.Test("did1", "rkey1", post) {
			t.Error("expected true but got false")
		}

		// Wait for expiration
		time.Sleep(time.Millisecond * 2)

		// Test after expiration
		post = &apibsky.FeedPost{
			Text: "world",
		}
		if block.Test("did1", "rkey1", post) {
			t.Error("expected false but got true")
		}
	})
}

func TestDropInLogicblock_Shutdown(t *testing.T) {
	logger := slog.Default()

	t.Run("正常系_watchlist停止", func(t *testing.T) {

		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// Add to watchlist
		post := &apibsky.FeedPost{
			Text: "hello world",
		}
		if !block.Test("did1", "rkey1", post) {
			t.Error("expected true but got false")
		}

		// Shutdown
		if err := block.Shutdown(context.Background()); err != nil {
			t.Errorf("failed to shutdown: %v", err)
		}

		// Test after shutdown
		post = &apibsky.FeedPost{
			Text: "hello world",
		}
		if !block.Test("did1", "rkey1", post) {
			t.Error("expected true but got false")
		}
	})

	t.Run("正常系_watchlistクリア", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// Add to watchlist
		post := &apibsky.FeedPost{
			Text: "hello world",
		}
		if !block.Test("did1", "rkey1", post) {
			t.Error("expected true but got false")
		}

		// Clear
		if err := block.Reset(); err != nil {
			t.Errorf("failed to reset: %v", err)
		}

		// Test after clear
		post = &apibsky.FeedPost{
			Text: "world",
		}
		if block.Test("did1", "rkey1", post) {
			t.Error("expected false but got true")
		}
	})
}

func TestDropInLogicblock_ProcessCommand(t *testing.T) {
	logger := slog.Default()

	t.Run("正常系_add", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		msg, err := block.(CommandProcessor).ProcessCommand("add", map[string]string{"did": "did1", "rkey": "rkey1"})
		if err != nil {
			t.Errorf("failed to process command: %v", err)
		}
		if msg != "add success" {
			t.Errorf("expected add success, got %s", msg)
		}
	})

	t.Run("正常系_reset", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// First add
		_, err = block.(CommandProcessor).ProcessCommand("add", map[string]string{"did": "did1", "rkey": "rkey1"})
		if err != nil {
			t.Errorf("failed to process add command: %v", err)
		}

		// Then delete
		msg, err := block.(CommandProcessor).ProcessCommand("delete", map[string]string{"did": "did1"})
		if err != nil {
			t.Errorf("failed to process delete command: %v", err)
		}
		if msg != "delete success" {
			t.Errorf("expected delete success, got %s", msg)
		}
	})

	t.Run("正常系_reset", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		// Add some entries
		_, err = block.(CommandProcessor).ProcessCommand("add", map[string]string{"did": "did1", "rkey": "rkey1"})
		if err != nil {
			t.Errorf("failed to process add command: %v", err)
		}

		// delete
		_, err = block.(CommandProcessor).ProcessCommand("delete", map[string]string{"did": "did1"})
		if err != nil {
			t.Errorf("failed to process delete command: %v", err)
		}

		// Clear all
		msg, err := block.(CommandProcessor).ProcessCommand("reset", map[string]string{})
		if err != nil {
			t.Errorf("failed to process reset command: %v", err)
		}
		if msg != "reset success" {
			t.Errorf("expected reset success, got %s", msg)
		}
	})

	t.Run("異常系_invalid_command", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		_, err = block.(CommandProcessor).ProcessCommand("invalid", map[string]string{})
		if err == nil {
			t.Error("expected error but got nil")
		}
	})

	t.Run("異常系_missing_params", func(t *testing.T) {
		cfg := &config.DropInLogicBlockConfig{
			BaseLogicBlockConfig: config.BaseLogicBlockConfig{
				BlockType: BlockTypeDropIn,
				Options: map[string]interface{}{
					config.DropInOptionTargetWord: []string{"hello"},
				},
			},
		}

		block, err := NewDropInLogicBlock(cfg, logger)
		if err != nil {
			t.Fatalf("failed to create block: %v", err)
		}

		_, err = block.(CommandProcessor).ProcessCommand("add", map[string]string{})
		if err == nil {
			t.Error("expected error but got nil")
		}
		if err.Error() != "invalid command parameters: add did:  rkey: " {
			t.Errorf("expected error message, got %s", err.Error())
		}
	})
}
