package logic

import "github.com/nus25/yuge/feed/config/types"

type LogicBlockFactory interface {
	Create(base BaseLogicBlockConfig) (types.LogicBlockConfig, error)
}

// Factory registration map
var logicBlockFactories = map[string]LogicBlockFactory{}

// RegisterFactory registers a factory for a specific block type.
func RegisterFactory(blockType string, factory LogicBlockFactory) {
	logicBlockFactories[blockType] = factory
}
