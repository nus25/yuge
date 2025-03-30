package logic

import (
	"time"

	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

type BaseLogicBlockConfig struct {
	definitions map[string]types.ConfigElementDefinition
	BlockName   string                 `yaml:"name,omitempty" json:"name,omitempty"`
	BlockType   string                 `yaml:"type" json:"type"`
	Options     map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

func (c *BaseLogicBlockConfig) Create(base BaseLogicBlockConfig) types.LogicBlockConfig {
	return &BaseLogicBlockConfig{
		BlockName: base.BlockName,
		BlockType: base.BlockType,
		Options:   base.Options,
	}
}

func (c *BaseLogicBlockConfig) GetBlockType() string {
	return c.BlockType
}

func (c *BaseLogicBlockConfig) GetBlockName() string {
	return c.BlockName
}

func (c *BaseLogicBlockConfig) GetOptions() map[string]interface{} {
	return c.Options
}

// Helper methods for type-safe value retrieval
func (c *BaseLogicBlockConfig) GetOption(key string) interface{} {
	if c.Options == nil {
		return nil
	}
	return c.Options[key]
}

func (c *BaseLogicBlockConfig) GetStringOption(key string) (val string, exists bool) {
	if v, ok := c.GetOption(key).(string); ok {
		return v, true
	}
	return "", false
}

func (c *BaseLogicBlockConfig) GetBoolOption(key string) (val bool, exists bool) {
	if v, ok := c.GetOption(key).(bool); ok {
		return v, true
	}
	return false, false
}

func (c *BaseLogicBlockConfig) GetIntOption(key string) (val int, exists bool) {
	if v, ok := c.GetOption(key).(int); ok {
		return v, true
	}
	if v, ok := c.GetOption(key).(uint64); ok {
		return int(v), true
	}
	if v, ok := c.GetOption(key).(float64); ok {
		return int(v), true
	}
	return 0, false
}

func (c *BaseLogicBlockConfig) GetDurationOption(key string) (val time.Duration, exists bool) {
	if v, ok := c.GetOption(key).(string); ok {
		if duration, err := time.ParseDuration(v); err == nil {
			return duration, true
		}
	}
	if v, ok := c.GetOption(key).(time.Duration); ok {
		return v, true
	}
	if v, ok := c.GetOption(key).(int64); ok {
		return time.Duration(v), true
	}
	if v, ok := c.GetOption(key).(float64); ok {
		return time.Duration(int64(v)), true
	}
	return 0, false
}

func (c *BaseLogicBlockConfig) GetStringArrayOption(key string) (val []string, exists bool) {
	words, err := types.ConvertStringArray(c.GetOption(key))
	if err != nil {
		return nil, false
	}
	return words, true
}

func (l *BaseLogicBlockConfig) ValidateAll() error {
	//required check
	for key, element := range l.definitions {
		if element.Required {
			if _, exists := l.Options[key]; !exists {
				return errors.NewValidationError(key, nil, "required option is missing")
			}
		}
	}

	// validate
	for key, value := range l.Options {
		if err := l.Validate(key, value); err != nil {
			return err
		}
	}

	return nil
}

func (l *BaseLogicBlockConfig) Validate(key string, value interface{}) error {
	if element, exists := l.definitions[key]; exists {
		if err := element.ValidateType(key, value); err != nil {
			return err
		}
		if element.Validator != nil {
			// Get converted value
			convertedValue, err := element.ConvertValue(value)
			if err != nil {
				return errors.NewValidationError(key, value, "value conversion failed")
			}
			// Execute validation with converted value
			return element.Validator(convertedValue)
		}
		return nil
	}
	return errors.NewValidationError(key, value, "invalid key")
}

func (l *BaseLogicBlockConfig) Update(key string, value interface{}) error {
	if err := l.Validate(key, value); err != nil {
		return err
	}
	definition, ok := l.definitions[key]
	if !ok {
		return errors.NewValidationError(key, value, "invalid key")
	}
	convertedValue, err := definition.ConvertValue(value)
	if err != nil {
		return errors.NewValidationError(key, value, "value conversion failed")
	}
	l.Options[key] = convertedValue

	return nil
}

func (l *BaseLogicBlockConfig) DeepCopy() types.LogicBlockConfig {
	copy := &BaseLogicBlockConfig{
		BlockName: l.BlockName,
		BlockType: l.BlockType,
		Options:   make(map[string]interface{}),
	}
	for k, v := range l.Options {
		copy.Options[k] = v
	}
	return copy
}
