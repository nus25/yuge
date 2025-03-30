package types

type Validatable interface {
	ValidateAll() error
	Validate(key string, value interface{}) error
}

type ConfigProvider interface {
	Load() (FeedConfig, error)
	Save() error
	Update(cfg FeedConfig) error
}

// FeedConfig は設定全体を表す構造体のインターフェース
type FeedConfig interface {
	ValidateAll() error
	Validate(key string, value interface{}) error
	FeedLogic() FeedLogicConfig
	Store() StoreConfig
	DetailedLog() bool
	DeepCopy() FeedConfig
}

type FeedLogicConfig interface {
	Validatable
	GetLogicBlockConfigs() []LogicBlockConfig
	DeepCopy() FeedLogicConfig
}

type LogicBlockConfig interface {
	Validatable
	GetBlockType() string
	GetBlockName() string
	GetOptions() map[string]interface{}
	GetOption(key string) interface{}
	DeepCopy() LogicBlockConfig
}

type StoreConfig interface {
	Validatable
	DeepCopy() StoreConfig
	GetTrimAt() int
	GetTrimRemain() int
}
