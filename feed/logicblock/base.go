package logicblock

import (
	"context"
	"log/slog"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/types"
)

var _ LogicBlock = (*BaseLogicblock)(nil) //type check

type BaseLogicblock struct {
	blockType string
	config    types.LogicBlockConfig
	logger    *slog.Logger
}

func (l *BaseLogicblock) BlockType() string {
	return l.blockType
}

func (l *BaseLogicblock) BlockName() string {
	return l.config.GetBlockName()
}

func (l *BaseLogicblock) Config() types.LogicBlockConfig {
	return l.config
}

func (l *BaseLogicblock) Logger() *slog.Logger {
	return l.logger
}

func (l *BaseLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) (result bool) {
	return false
}

func (l *BaseLogicblock) Reset() error {
	return nil
}

func (l *BaseLogicblock) Shutdown(ctx context.Context) error {
	return nil
}

func NewBaseLogicblock(blockType string, cfg types.LogicBlockConfig, logger *slog.Logger) *BaseLogicblock {
	return &BaseLogicblock{
		blockType: blockType,
		config:    cfg,
		logger:    logger,
	}
}
