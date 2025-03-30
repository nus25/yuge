package provider

import "github.com/nus25/yuge/feed/config/types"

type FeedConfigProvider interface {
	Load() (types.FeedConfig, error)
	Save() error
	FeedConfig() types.FeedConfig
	Update(cfg types.FeedConfig) error
}
