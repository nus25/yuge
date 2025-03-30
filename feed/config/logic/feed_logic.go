package logic

import (
	"encoding/json"
	"fmt"

	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

var _ types.FeedLogicConfig = (*FeedLogicConfigimpl)(nil) //type check

type FeedLogicConfigimpl struct {
	LogicBlocks []types.LogicBlockConfig `yaml:"blocks" json:"blocks"`
}

func DefaultFeedLogicConfig() *FeedLogicConfigimpl {
	return &FeedLogicConfigimpl{
		LogicBlocks: []types.LogicBlockConfig{},
	}
}

func (f *FeedLogicConfigimpl) DeepCopy() types.FeedLogicConfig {
	copy := FeedLogicConfigimpl{
		LogicBlocks: make([]types.LogicBlockConfig, len(f.LogicBlocks)),
	}
	for i, block := range f.LogicBlocks {
		copy.LogicBlocks[i] = block.DeepCopy()
	}
	return &copy
}

func (f *FeedLogicConfigimpl) GetLogicBlockConfigs() []types.LogicBlockConfig {
	return f.LogicBlocks
}

type tempLogicBlockConfig struct {
	Type    string                 `yaml:"type" json:"type"`
	Name    string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

func (f *FeedLogicConfigimpl) createLogicBlocks(blocks []tempLogicBlockConfig) ([]types.LogicBlockConfig, error) {
	logicBlocks := make([]types.LogicBlockConfig, len(blocks))
	for i, block := range blocks {
		var logicBlock types.LogicBlockConfig
		var err error
		base := BaseLogicBlockConfig{
			BlockType: block.Type,
			BlockName: block.Name,
			Options:   block.Options,
		}

		if factory, ok := logicBlockFactories[block.Type]; ok {
			logicBlock, err = factory.Create(base)
			if err != nil {
				return nil, err
			}
		} else {
			logicBlock = &CustomLogicBlockConfig{BaseLogicBlockConfig: base}
		}

		logicBlocks[i] = logicBlock
	}
	return logicBlocks, nil
}

func (f *FeedLogicConfigimpl) UnmarshalJSON(data []byte) error {
	var tempConfig struct {
		LogicBlocks []tempLogicBlockConfig `json:"blocks"`
	}

	if err := json.Unmarshal(data, &tempConfig); err != nil {
		return err
	}

	logicBlocks, err := f.createLogicBlocks(tempConfig.LogicBlocks)
	if err != nil {
		return err
	}
	f.LogicBlocks = logicBlocks
	return nil
}

func (f *FeedLogicConfigimpl) MarshalYAML() (interface{}, error) {
	blocks := make([]tempLogicBlockConfig, len(f.LogicBlocks))
	for i, block := range f.LogicBlocks {
		blocks[i] = tempLogicBlockConfig{
			Type:    block.GetBlockType(),
			Name:    block.GetBlockName(),
			Options: block.GetOptions(),
		}
	}

	return struct {
		LogicBlocks []tempLogicBlockConfig `yaml:"blocks"`
	}{
		LogicBlocks: blocks,
	}, nil
}

func (f *FeedLogicConfigimpl) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var tempConfig struct {
		LogicBlocks []tempLogicBlockConfig `yaml:"blocks"`
	}

	if err := unmarshal(&tempConfig); err != nil {
		return err
	}

	logicBlocks, err := f.createLogicBlocks(tempConfig.LogicBlocks)
	if err != nil {
		return err
	}
	f.LogicBlocks = logicBlocks
	return nil
}

func (f *FeedLogicConfigimpl) ValidateAll() error {
	for i, block := range f.LogicBlocks {
		if err := block.ValidateAll(); err != nil {
			return errors.NewConfigError(
				"FeedLogic",
				fmt.Sprintf("logicBlocks[%d]", i),
				fmt.Sprintf("invalid logic block: %v", err),
			)
		}
	}
	return nil
}

func (f *FeedLogicConfigimpl) Validate(key string, value interface{}) error {
	if key == "blocks" {
		if blocks, ok := value.([]types.LogicBlockConfig); ok {
			if len(blocks) == 0 {
				return errors.NewConfigError("FeedLogic", key, "at least one logic block is required")
			}
			for i, block := range blocks {
				if err := block.ValidateAll(); err != nil {
					return errors.NewConfigError(
						"FeedLogic",
						fmt.Sprintf("%s[%d]", key, i),
						fmt.Sprintf("invalid logic block: %v", err),
					)
				}
			}
		} else {
			return errors.NewConfigError("FeedLogic", key, "invalid type for logicBlocks: expected []LogicBlockConfig")
		}
	}
	return nil
}
