package logic

import (
	"fmt"

	"github.com/dlclark/regexp2"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

func init() {
	RegisterFactory(RegexBlockType, &RegexLogicBlockFactory{})
}

// RegexLogicBlockConfig defines a filtering logic block based on regular expressions.
// It allows filtering posts based on regex pattern matching against post content.
// The matching can be configured in several ways:
// - value: The regex pattern to match against
// - invert: If true, inverts the match result (keeps non-matching posts)
// - caseSensitive: If true, performs case-sensitive regex matching
type RegexLogicBlockConfig struct {
	BaseLogicBlockConfig
}

const (
	RegexBlockType           = "regex"
	RegexOptionValue         = "value"         // required
	RegexOptionInvert        = "invert"        // required
	RegexOptionCaseSensitive = "caseSensitive" // required
)

// RegexLogicBlockFactory is a factory for creating RegexLogicBlockConfig
type RegexLogicBlockFactory struct{}

func (f *RegexLogicBlockFactory) Create(base BaseLogicBlockConfig) (types.LogicBlockConfig, error) {
	cfg := RegexLogicBlockConfig{BaseLogicBlockConfig: base}
	cfg.definitions = RegexConfigElements
	return &cfg, nil
}

var RegexConfigElements = map[string]types.ConfigElementDefinition{
	RegexOptionValue: {
		Type:         types.ElementTypeString,
		Key:          RegexOptionValue,
		DefaultValue: "",
		Required:     true,
		Validator: func(value interface{}) error {
			if _, ok := value.(string); !ok {
				return errors.NewValidationError(RegexOptionValue, value, "must be a string")
			}
			if _, err := regexp2.Compile(value.(string), 0); err != nil {
				return errors.NewValidationError(RegexOptionValue, value, fmt.Sprintf("invalid regex pattern: %v", err))
			}
			if value == "" {
				return errors.NewValidationError(RegexOptionValue, value, "must not be empty")
			}
			return nil
		},
	},
	RegexOptionInvert: {
		Type:         types.ElementTypeBool,
		Key:          RegexOptionInvert,
		DefaultValue: false,
		Required:     true,
		Validator: func(value interface{}) error {
			if _, ok := value.(bool); !ok {
				return errors.NewValidationError(RegexOptionInvert, value, "must be a boolean")
			}
			return nil
		},
	},
	RegexOptionCaseSensitive: {
		Type:         types.ElementTypeBool,
		Key:          RegexOptionCaseSensitive,
		DefaultValue: true,
		Required:     true,
		Validator: func(value interface{}) error {
			if _, ok := value.(bool); !ok {
				return errors.NewValidationError(RegexOptionCaseSensitive, value, "must be a boolean")
			}
			return nil
		},
	},
}
