package logicblock

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	config "github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
	"github.com/nus25/yuge/feed/limiter"
)

var _ LogicBlock = (*LimiterLogicblock)(nil) //type check
var _ CommandProcessor = (*LimiterLogicblock)(nil)

const (
	BlockTypeLimiter    = config.LimiterBlockType
	LimiterCommandList  = "list"
	LimiterCommandClear = "clear"
)

func init() {
	FactoryInstance().RegisterCreator(BlockTypeLimiter, NewLimiterLogicBlock)
}

type LimiterLogicblock struct {
	*BaseLogicblock
	limitCount  int
	limitWindow time.Duration
	cleanupFreq time.Duration
	limiter     *limiter.PostLimiter
}

func NewLimiterLogicBlock(cfg types.LogicBlockConfig, logger *slog.Logger) (LogicBlock, error) {
	if cfg.GetBlockType() != BlockTypeLimiter {
		logger.Error("invalid block type", "type", cfg.GetBlockType())
		return nil, errors.NewConfigError("block type", cfg.GetBlockType(), "invalid block type")
	}
	lcfg, ok := cfg.(*config.LimiterLogicBlockConfig)
	if !ok {
		logger.Error("invalid config type", "type", fmt.Sprintf("%T", cfg))
		return nil, errors.NewConfigError("config type", fmt.Sprintf("%T", cfg), "invalid config type")
	}
	//count
	c, ok := lcfg.GetIntOption(config.LimiterOptionCount)
	if !ok {
		logger.Error("limitCount option not found")
		return nil, errors.NewConfigError(config.LimiterOptionCount, "", "limitCount option not found")
	}
	if c <= 0 {
		logger.Error("limitCount must be greater than 0", "limitCount", c)
		return nil, errors.NewConfigError(config.LimiterOptionCount, fmt.Sprintf("%d", c), "limitCount must be greater than 0")
	}
	//timeWindow
	w, ok := lcfg.GetDurationOption(config.LimiterOptionTimeWindow)
	if !ok {
		logger.Error("timeWindow option not found")
		return nil, errors.NewConfigError(config.LimiterOptionTimeWindow, "", "timeWindow option not found")
	}
	if w < time.Second {
		logger.Error("timeWindow must be greater than 1 second", "timeWindow", w)
		return nil, errors.NewConfigError(config.LimiterOptionTimeWindow, w.String(), "timeWindow must be greater than 1 second")
	}
	//cleanupFreq
	f, ok := lcfg.GetDurationOption(config.LimiterOptionCleanupFreq)
	if !ok {
		logger.Error("cleanupFreq option not found")
		return nil, errors.NewConfigError(config.LimiterOptionCleanupFreq, "", "cleanupFreq option not found")
	}
	if f <= time.Second {
		logger.Error("cleanupFreq must be greater than 1 second", "cleanupFreq", f)
		return nil, errors.NewConfigError(config.LimiterOptionCleanupFreq, f.String(), "cleanupFreq must be greater than 1 second")
	}

	l, _ := limiter.NewPostLimiter(c, w, f)

	return &LimiterLogicblock{
		BaseLogicblock: &BaseLogicblock{
			blockType: BlockTypeLimiter,
			config:    cfg,
			logger:    logger,
		},
		limitCount:  c,
		limitWindow: w,
		cleanupFreq: f,
		limiter:     l,
	}, nil
}

func (l *LimiterLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) bool {
	if l.limiter != nil {
		if isAllowed, _ := l.limiter.RecordPost(did); !isAllowed {
			l.logger.Warn("too many posts from user", "did", did)
			return false
		}
	}
	return true
}

func (l *LimiterLogicblock) Reset() error {
	l.logger.Info("resetting limiter")
	l.limiter.Clear()
	return nil
}

func (l *LimiterLogicblock) Shutdown(ctx context.Context) error {
	return nil
}

func (l *LimiterLogicblock) ProcessCommand(command string, args map[string]string) (message string, err error) {
	switch strings.ToLower(command) {
	case LimiterCommandList:
		return fmt.Sprintf("%v", l.limiter.GetRecords()), nil
	case LimiterCommandClear:
		l.limiter.Clear()
		return "cleared", nil
	default:
		return "", fmt.Errorf("invalid command: %s", command)
	}
}
