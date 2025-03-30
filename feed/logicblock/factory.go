package logicblock

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/nus25/yuge/feed/config/types"
)

type LogicBlockFactory struct {
	Creators map[string]LogicBlockCreator
}

var instance *LogicBlockFactory
var once sync.Once

func FactoryInstance() *LogicBlockFactory {
	once.Do(func() {
		instance = &LogicBlockFactory{
			Creators: make(map[string]LogicBlockCreator),
		}
	})
	return instance
}

func (f *LogicBlockFactory) RegisterCreator(name string, creator func(types.LogicBlockConfig, *slog.Logger) (LogicBlock, error)) {
	f.Creators[name] = creator
}

func (f *LogicBlockFactory) Create(cfg types.LogicBlockConfig, logger *slog.Logger) (LogicBlock, error) {
	creator, ok := f.Creators[cfg.GetBlockType()]
	if !ok {
		logger.Error("unknown logic block type", "type", cfg.GetBlockType())
		return nil, fmt.Errorf("unknown logic block type: %s", cfg.GetBlockType())
	}
	return creator(cfg, logger)
}
