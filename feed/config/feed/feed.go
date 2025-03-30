package feed

import (
	"encoding/json"

	"github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/store"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

var _ types.FeedConfig = (*FeedConfigImpl)(nil)

const DefaultDetailedLog bool = false

type feedConfigInternal struct {
	FeedLogic   *types.FeedLogicConfig `yaml:"logic,omitempty" json:"logic,omitempty"`
	Store       *types.StoreConfig     `yaml:"store,omitempty" json:"store,omitempty"`
	DetailedLog *bool                  `yaml:"detailedLog,omitempty" json:"detailedLog,omitempty"`
}

// FeedConfigImpl is readonly config values
type FeedConfigImpl struct {
	internal feedConfigInternal
}

func DefaultFeedConfig() *FeedConfigImpl {
	detailedLog := DefaultDetailedLog
	cfg := NewFeedConfig(logic.DefaultFeedLogicConfig(), store.DefaultStoreConfig(), &detailedLog)
	return cfg
}

// NewFeedConfig creates a new FeedConfig with the given parameters
func NewFeedConfig(feedLogic types.FeedLogicConfig, store types.StoreConfig, detailedLog *bool) *FeedConfigImpl {
	return &FeedConfigImpl{
		internal: feedConfigInternal{
			FeedLogic:   &feedLogic,
			Store:       &store,
			DetailedLog: detailedLog,
		},
	}
}

func NewFeedConfigFromJSON(jsonStr string) (*FeedConfigImpl, error) {
	var config FeedConfigImpl
	if err := json.Unmarshal([]byte(jsonStr), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// DeepCopy creates a deep copy of the FeedConfig
func (f *FeedConfigImpl) DeepCopy() types.FeedConfig {
	copy := FeedConfigImpl{
		internal: feedConfigInternal{},
	}

	if f.internal.FeedLogic != nil {
		flCopy := (*f.internal.FeedLogic).DeepCopy()
		copy.internal.FeedLogic = &flCopy
	}

	if f.internal.Store != nil {
		storeCopy := (*f.internal.Store).DeepCopy()
		copy.internal.Store = &storeCopy
	}

	if f.internal.DetailedLog != nil {
		copy.internal.DetailedLog = f.internal.DetailedLog
	}

	return &copy
}

func (f *FeedConfigImpl) MarshalJSON() ([]byte, error) {
	return json.Marshal(feedConfigInternal{
		FeedLogic:   f.internal.FeedLogic,
		Store:       f.internal.Store,
		DetailedLog: f.internal.DetailedLog,
	})
}

func (f *FeedConfigImpl) UnmarshalJSON(data []byte) error {
	aux := struct {
		FeedLogic   *logic.FeedLogicConfigimpl `json:"logic"`
		Store       *store.StoreConfigImpl     `json:"store,omitempty"`
		DetailedLog *bool                      `json:"detailedLog,omitempty"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.FeedLogic != nil {
		flImpl := types.FeedLogicConfig(aux.FeedLogic)
		f.internal.FeedLogic = &flImpl
	} else {
		f.internal.FeedLogic = nil
	}
	if aux.Store != nil {
		storeImpl := types.StoreConfig(aux.Store)
		f.internal.Store = &storeImpl
	} else {
		f.internal.Store = nil
	}
	f.internal.DetailedLog = aux.DetailedLog
	return nil
}

func (f *FeedConfigImpl) MarshalYAML() (interface{}, error) {
	return feedConfigInternal{
		FeedLogic:   f.internal.FeedLogic,
		Store:       f.internal.Store,
		DetailedLog: f.internal.DetailedLog,
	}, nil
}

func (f *FeedConfigImpl) UnmarshalYAML(unmarshal func(interface{}) error) error {
	aux := &struct {
		FeedLogic   *logic.FeedLogicConfigimpl `yaml:"logic"`
		Store       *store.StoreConfigImpl     `yaml:"store,omitempty"`
		DetailedLog *bool                      `yaml:"detailedLog,omitempty"`
	}{}
	if err := unmarshal(aux); err != nil {
		return err
	}
	if aux.FeedLogic != nil {
		flImpl := types.FeedLogicConfig(aux.FeedLogic)
		f.internal.FeedLogic = &flImpl
	} else {
		f.internal.FeedLogic = nil
	}
	if aux.Store != nil {
		storeImpl := types.StoreConfig(aux.Store)
		f.internal.Store = &storeImpl
	} else {
		f.internal.Store = nil
	}
	f.internal.DetailedLog = aux.DetailedLog
	return nil
}

func (f *FeedConfigImpl) FeedLogic() types.FeedLogicConfig {
	if f.internal.FeedLogic == nil {
		return logic.DefaultFeedLogicConfig()
	}
	return *f.internal.FeedLogic
}

func (f *FeedConfigImpl) Store() types.StoreConfig {
	if f.internal.Store == nil {
		return store.DefaultStoreConfig()
	}
	return *f.internal.Store
}

func (f *FeedConfigImpl) DetailedLog() bool {
	if f.internal.DetailedLog == nil {
		return DefaultDetailedLog
	}
	return *f.internal.DetailedLog
}

func (f *FeedConfigImpl) ValidateAll() error {
	// FeedLogic
	if f.FeedLogic() != nil {
		if err := f.FeedLogic().ValidateAll(); err != nil {
			return errors.NewConfigError("FeedConfig", "logic", err.Error())
		}
	}

	// Store
	if f.Store() != nil {
		if err := f.Store().ValidateAll(); err != nil {
			return errors.NewConfigError("FeedConfig", "store", err.Error())
		}
	}

	return nil
}

func (f *FeedConfigImpl) Validate(key string, value interface{}) error {
	switch key {
	case "logic":
		feedLogic := f.FeedLogic()
		if feedLogic == nil {
			return errors.NewConfigError("FeedConfig", key, "feed logic is nil")
		}
		if err := feedLogic.Validate(key, value); err != nil {
			return errors.NewConfigError("FeedConfig", key, err.Error())
		}
	case "store.trimAt", "store.trimRemain":
		store := f.Store()
		if store == nil {
			return errors.NewConfigError("FeedConfig", key, "store is nil")
		}

		storeKey := key
		if key == "store.trimAt" {
			storeKey = "trimAt"
		} else if key == "store.trimRemain" {
			storeKey = "trimRemain"
		}

		if err := store.Validate(storeKey, value); err != nil {
			return errors.NewConfigError("FeedConfig", key, err.Error())
		}
	}
	return nil
}
