package logic

import (
	"time"

	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

func init() {
	RegisterFactory(LimiterBlockType, &LimiterLogicBlockFactory{})
}

// listUri: stringユーザーリストのURI
// 例: at://did:plc:xxx/app.bsky.graph.list/xxx
// allow: bool trueの場合リスト内のDidのみを通過する。falseの場合リスト内のDidを遮断する
type LimiterLogicBlockConfig struct {
	BaseLogicBlockConfig
}

const (
	LimiterBlockType         = "limiter"
	LimiterOptionCount       = "count"       //required
	LimiterOptionTimeWindow  = "timeWindow"  //required
	LimiterOptionCleanupFreq = "cleanupFreq" //optional
)

// LimiterLogicBlockFactory is a factory for creating LimiterLogicBlockConfig
type LimiterLogicBlockFactory struct{}

func (f *LimiterLogicBlockFactory) Create(base BaseLogicBlockConfig) (types.LogicBlockConfig, error) {
	cfg := LimiterLogicBlockConfig{BaseLogicBlockConfig: base}
	cfg.definitions = LimiterConfigElements
	return &cfg, nil
}

var LimiterConfigElements = map[string]types.ConfigElementDefinition{
	LimiterOptionCount: {
		Type:         types.ElementTypeInt,
		Key:          LimiterOptionCount,
		DefaultValue: nil,
		Required:     true,
		Validator: func(value interface{}) error {
			var count int
			var ok bool
			if count, ok = value.(int); !ok {
				if v, ok := value.(uint64); ok {
					count = int(v)
				} else if v, ok := value.(float64); ok {
					count = int(v)
				} else {
					return errors.NewValidationError(LimiterOptionCount, value, "must be an integer")
				}
			}
			if count <= 0 {
				return errors.NewValidationError(LimiterOptionCount, value, "must be positive")
			}
			return nil
		},
	},
	LimiterOptionTimeWindow: {
		Type:         types.ElementTypeDuration,
		Key:          LimiterOptionTimeWindow,
		DefaultValue: nil,
		Required:     true,
		Validator: func(value interface{}) error {
			duration, ok := value.(time.Duration)
			if !ok {
				return errors.NewValidationError(LimiterOptionTimeWindow, value, "must be a duration")
			}
			if duration <= 0 {
				return errors.NewValidationError(LimiterOptionTimeWindow, value, "must be positive")
			}
			return nil
		},
	},
	LimiterOptionCleanupFreq: {
		Type:         types.ElementTypeDuration,
		Key:          LimiterOptionCleanupFreq,
		DefaultValue: 10 * time.Minute,
		Required:     false,
		Validator: func(value interface{}) error {
			duration, ok := value.(time.Duration)
			if !ok {
				return errors.NewValidationError(LimiterOptionCleanupFreq, value, "must be a duration")
			}
			if duration <= 0 {
				return errors.NewValidationError(LimiterOptionCleanupFreq, value, "must be positive")
			}
			return nil
		},
	},
}
