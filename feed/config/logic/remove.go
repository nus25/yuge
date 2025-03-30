package logic

import (
	"slices"
	"strings"

	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

func init() {
	RegisterFactory(RemoveBlockType, &RemoveLogicBlockFactory{})
}

// RemoveLogicBlockConfig defines a logic block for removing by specific elements
// The following values are available for subject:
// - "item": post type (reply, repost)
// - "language": post language with operator (== or !=)
// For validation, see Validate() method
type RemoveLogicBlockConfig struct {
	BaseLogicBlockConfig
}

const (
	RemoveBlockType       = "remove"
	RemoveOptionSubject   = "subject"
	RemoveOptionValue     = "value"
	RemoveOptionLanguage  = "language"
	RemoveOptionOperator  = "operator"
	RemoveSubjectItem     = "item"
	RemoveSubjectLanguage = "language"
	RemoveValueReply      = "reply"
	RemoveValueRepost     = "repost"
	RemoveOperatorEq      = "=="
	RemoveOperatorNe      = "!="
)

// RemoveLogicBlockFactory is a factory for creating RemoveLogicBlockConfig
type RemoveLogicBlockFactory struct{}

func (f *RemoveLogicBlockFactory) Create(base BaseLogicBlockConfig) (types.LogicBlockConfig, error) {
	config := &RemoveLogicBlockConfig{BaseLogicBlockConfig: base}
	//determine the config elements based on the subject
	subject, exists := config.GetStringOption(RemoveOptionSubject)
	if !exists {
		return nil, errors.NewValidationError(RemoveOptionSubject, subject, "subject cannot be empty")
	}
	switch subject {
	case RemoveSubjectItem:
		config.definitions = RemoveItemConfigElements
	case RemoveSubjectLanguage:
		config.definitions = RemoveSubjectConfigElements
	}
	return config, nil
}

var elementDefinitionSubject = types.ConfigElementDefinition{
	Type:         types.ElementTypeString,
	Key:          RemoveOptionSubject,
	DefaultValue: "",
	Required:     true,
	Validator: func(value interface{}) error {
		arr := []string{RemoveSubjectItem, RemoveSubjectLanguage}
		if !slices.Contains(arr, value.(string)) {
			return errors.NewValidationError(RemoveOptionSubject, value, "subject must be one of the following: "+strings.Join(arr, ", "))
		}
		return nil
	},
}

var RemoveItemConfigElements = map[string]types.ConfigElementDefinition{
	RemoveOptionSubject: elementDefinitionSubject,
	RemoveOptionValue: {
		Type:         types.ElementTypeString,
		Key:          RemoveOptionValue,
		DefaultValue: "",
		Required:     true,
		Validator: func(value interface{}) error {
			arr := []string{RemoveValueReply, RemoveValueRepost}
			if !slices.Contains(arr, value.(string)) {
				return errors.NewValidationError(RemoveOptionValue, value, "value must be one of the following: "+strings.Join(arr, ", "))
			}
			return nil
		},
	},
}

var RemoveSubjectConfigElements = map[string]types.ConfigElementDefinition{
	RemoveOptionSubject: elementDefinitionSubject,
	RemoveOptionLanguage: {
		Type:         types.ElementTypeString,
		Key:          RemoveOptionLanguage,
		DefaultValue: "",
		Required:     true,
		Validator: func(value interface{}) error {
			if value == "" {
				return errors.NewValidationError(RemoveOptionLanguage, value, "language cannot be empty")
			}
			return nil
		},
	},
	RemoveOptionOperator: {
		Type:         types.ElementTypeString,
		Key:          RemoveOptionOperator,
		DefaultValue: "",
		Required:     true,
		Validator: func(value interface{}) error {
			if value == "" {
				return errors.NewValidationError(RemoveOptionOperator, value, "operator cannot be empty")
			}
			arr := []string{RemoveOperatorEq, RemoveOperatorNe}
			if !slices.Contains(arr, value.(string)) {
				return errors.NewValidationError(RemoveOptionOperator, value, "operator must be one of the following: "+strings.Join(arr, ", "))
			}
			return nil
		},
	},
}

func (l *RemoveLogicBlockConfig) ValidateAll() error {
	// set definitions based on subject
	subject, exists := l.GetStringOption(RemoveOptionSubject)
	if !exists {
		return errors.NewValidationError(RemoveOptionSubject, subject, "subject cannot be empty")
	}
	switch subject {
	case RemoveSubjectItem:
		l.definitions = RemoveItemConfigElements
	case RemoveSubjectLanguage:
		l.definitions = RemoveSubjectConfigElements
	}
	return l.BaseLogicBlockConfig.ValidateAll()
}
