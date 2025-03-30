package logicblock

import (
	"context"
	"fmt"
	"log/slog"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/dlclark/regexp2"
	config "github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

var _ LogicBlock = (*RegexLogicblock)(nil) //type check

func init() {
	FactoryInstance().RegisterCreator(BlockTypeRegex, NewRegexLogicBlock)
}

const BlockTypeRegex = config.RegexBlockType

type RegexLogicblock struct {
	*BaseLogicblock
	pattern       string
	caseSensitive bool
	invert        bool
	regexp        *regexp2.Regexp
}

func NewRegexLogicBlock(cfg types.LogicBlockConfig, logger *slog.Logger) (LogicBlock, error) {
	var re *regexp2.Regexp
	var err error
	if cfg.GetBlockType() != config.RegexBlockType {
		logger.Error("invalid block type", "type", cfg.GetBlockType())
		return nil, errors.NewConfigError("block type", cfg.GetBlockType(), "invalid block type")
	}
	rcfg, ok := cfg.(*config.RegexLogicBlockConfig)
	if !ok {
		logger.Error("invalid config type", "type", fmt.Sprintf("%T", cfg))
		return nil, errors.NewConfigError("config type", fmt.Sprintf("%T", cfg), "invalid config type")
	}
	//regex pattern
	pattern, ok := rcfg.GetStringOption(config.RegexOptionValue)
	if !ok {
		logger.Error("value option not found")
		return nil, errors.NewConfigError(config.RegexOptionValue, "", "value option not found")
	}
	if pattern == "" {
		logger.Error("empty regex pattern")
		return nil, errors.NewConfigError(config.RegexOptionValue, pattern, "empty regex pattern")
	}
	//caseSensitive
	caseSensitive, ok := rcfg.GetBoolOption(config.RegexOptionCaseSensitive)
	if !ok {
		logger.Error("caseSensitive option not found")
		return nil, errors.NewConfigError(config.RegexOptionCaseSensitive, "", "caseSensitive option not found")
	}
	//invert
	invert, ok := rcfg.GetBoolOption(config.RegexOptionInvert)
	if !ok {
		logger.Error("invert option not found")
		return nil, errors.NewConfigError(config.RegexOptionInvert, "", "invert option not found")
	}

	logger.Info("compiling regex pattern", "pattern", pattern, "caseSensitive", caseSensitive)
	if caseSensitive {
		re, err = regexp2.Compile(pattern, 0)
	} else {
		re, err = regexp2.Compile(pattern, regexp2.IgnoreCase)
	}
	if err != nil {
		logger.Error("failed to compile regex pattern", "error", err)
		return nil, errors.NewConfigError(config.RegexOptionValue, pattern, fmt.Sprintf("invalid regex pattern: %v", err))
	}
	return &RegexLogicblock{
		BaseLogicblock: &BaseLogicblock{
			blockType: BlockTypeRegex,
			config:    cfg,
			logger:    logger,
		},
		pattern:       pattern,
		caseSensitive: caseSensitive,
		invert:        invert,
		regexp:        re,
	}, nil
}

func (l *RegexLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) (result bool) {
	if post.Text == "" {
		return false
	}

	text := post.Text
	matched, err := l.regexp.MatchString(text)
	if err != nil {
		return false
	}
	if l.invert {
		return !matched
	}
	return matched
}

func (l *RegexLogicblock) Reset() error {
	return nil
}

func (l *RegexLogicblock) Shutdown(ctx context.Context) error {
	return nil
}
