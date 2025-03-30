package logicblock

import (
	"context"
	"fmt"
	"log/slog"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	config "github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

var _ LogicBlock = (*RemoveLogicblock)(nil) //type check

func init() {
	FactoryInstance().RegisterCreator(BlockTypeRemove, NewRemoveLogicBlock)
}

const BlockTypeRemove = config.RemoveBlockType

type RemoveLogicblock struct {
	*BaseLogicblock
	subject  string
	value    string
	language string
	operator string
}

func NewRemoveLogicBlock(cfg types.LogicBlockConfig, logger *slog.Logger) (LogicBlock, error) {
	if cfg.GetBlockType() != BlockTypeRemove {
		return nil, errors.NewConfigError("block type", cfg.GetBlockType(), "invalid block type")
	}
	rcfg, ok := cfg.(*config.RemoveLogicBlockConfig)
	if !ok {
		return nil, errors.NewConfigError("config type", fmt.Sprintf("%T", cfg), "invalid config type")
	}

	subject, ok := rcfg.GetStringOption(config.RemoveOptionSubject)
	if !ok {
		return nil, errors.NewConfigError("subject option not found", "", "")
	}

	var value, language, operator string

	switch subject {
	case config.RemoveSubjectItem:
		value, ok = rcfg.GetStringOption(config.RemoveOptionValue)
		if !ok {
			return nil, errors.NewConfigError("value option not found", "", "")
		}
	case config.RemoveSubjectLanguage:
		language, ok = rcfg.GetStringOption(config.RemoveOptionLanguage)
		if !ok {
			return nil, errors.NewConfigError("language option not found", "", "")
		}
		operator, ok = rcfg.GetStringOption(config.RemoveOptionOperator)
		if !ok {
			return nil, errors.NewConfigError("operator option not found", "", "")
		}
	default:
		return nil, errors.NewConfigError("invalid subject", subject, "invalid subject")
	}

	return &RemoveLogicblock{
		BaseLogicblock: &BaseLogicblock{
			blockType: BlockTypeRemove,
			config:    cfg,
			logger:    logger,
		},
		subject:  subject,
		value:    value,
		language: language,
		operator: operator,
	}, nil
}

// Returns true if the post does not match the removal condition
func (l *RemoveLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) (result bool) {
	switch l.subject {
	case config.RemoveSubjectItem:
		if l.value == config.RemoveValueReply && post.Reply != nil {
			return false
		}
	case config.RemoveSubjectLanguage:
		if post.Langs != nil {
			switch l.operator {
			case config.RemoveOperatorEq:
				for _, lang := range post.Langs {
					if l.language == lang {
						return false //一つでも一致すればfail
					}
				}
				return true //どれも該当しなければpass
			case config.RemoveOperatorNe:
				for _, lang := range post.Langs {
					if l.language != lang {
						return false //一つでも不一致ならばfail
					}
				}
				return true //どれも該当しなければpass
			}
		} else {
			return l.operator == config.RemoveOperatorEq //langsがnilの場合はoperatorがeqのみpass
		}
	}
	return true
}

func (l *RemoveLogicblock) Reset() error {
	return nil
}

func (l *RemoveLogicblock) Shutdown(ctx context.Context) error {
	return nil
}
