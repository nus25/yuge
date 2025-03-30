package logic

import (
	"time"

	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

func init() {
	RegisterFactory(DropInBlockType, &DropInLogicBlockFactory{})
}

type DropInLogicBlockConfig struct {
	BaseLogicBlockConfig
	ExpireDuration time.Duration
	TargetWord     []string
	CancelWord     []string
	IgnoreWord     []string
}

const (
	DropInBlockType            = "dropin"
	DropInOptionTargetWord     = "targetWord"     // required
	DropInOptionCancelWord     = "cancelWord"     // optional
	DropInOptionIgnoreWord     = "ignoreWord"     // optional
	DropInOptionExpireDuration = "expireDuration" // optional
)

// DropInLogicBlockFactory is a factory for creating DropInLogicBlockConfig
type DropInLogicBlockFactory struct{}

func (f *DropInLogicBlockFactory) Create(base BaseLogicBlockConfig) (types.LogicBlockConfig, error) {
	cfg := DropInLogicBlockConfig{BaseLogicBlockConfig: base}
	cfg.definitions = DropInConfigElements
	cfg.ExpireDuration, _ = cfg.GetDurationOption(DropInOptionExpireDuration)
	cfg.TargetWord, _ = cfg.GetStringArrayOption(DropInOptionTargetWord)
	cfg.CancelWord, _ = cfg.GetStringArrayOption(DropInOptionCancelWord)
	cfg.IgnoreWord, _ = cfg.GetStringArrayOption(DropInOptionIgnoreWord)

	return &cfg, nil
}

var DropInConfigElements = map[string]types.ConfigElementDefinition{
	DropInOptionTargetWord: {
		Type:         types.ElementTypeStringArray,
		Key:          DropInOptionTargetWord,
		DefaultValue: nil,
		Required:     true,
		Validator: func(value interface{}) error {
			words, err := types.ConvertStringArray(value)
			if err != nil {
				return errors.NewValidationError(DropInOptionTargetWord, value, "must be a string array")
			}
			if len(words) == 0 {
				return errors.NewValidationError(DropInOptionTargetWord, value, "must not be empty")
			}
			return nil
		},
	},
	DropInOptionCancelWord: {
		Type:         types.ElementTypeStringArray,
		Key:          DropInOptionCancelWord,
		DefaultValue: []string{},
		Required:     false,
		Validator: func(value interface{}) error {
			words, err := types.ConvertStringArray(value)
			if err != nil {
				return errors.NewValidationError(DropInOptionCancelWord, value, "must be a string array")
			}
			if len(words) == 0 {
				return errors.NewValidationError(DropInOptionCancelWord, value, "must not be empty")
			}
			return nil
		},
	},
	DropInOptionIgnoreWord: {
		Type:         types.ElementTypeStringArray,
		Key:          DropInOptionIgnoreWord,
		DefaultValue: []string{},
		Required:     false,
		Validator: func(value interface{}) error {
			words, err := types.ConvertStringArray(value)
			if err != nil {
				return errors.NewValidationError(DropInOptionIgnoreWord, value, "must be a string array")
			}
			if len(words) == 0 {
				return errors.NewValidationError(DropInOptionIgnoreWord, value, "must not be empty")
			}
			return nil
		},
	},
	DropInOptionExpireDuration: {
		Type:         types.ElementTypeDuration,
		Key:          DropInOptionExpireDuration,
		DefaultValue: time.Duration(0),
		Required:     false,
		Validator: func(value interface{}) error {
			_, ok := value.(time.Duration)
			if !ok {
				return errors.NewValidationError(DropInOptionExpireDuration, value, "must be a duration")
			}
			return nil
		},
	},
}
