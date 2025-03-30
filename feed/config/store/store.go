package store

import (
	"fmt"
	"log/slog"

	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

var _ types.StoreConfig = (*StoreConfigImpl)(nil) //type check

type StoreConfigImpl struct {
	TrimAt     int `yaml:"trimAt" json:"trimAt"`
	TrimRemain int `yaml:"trimRemain" json:"trimRemain"`
}

func DefaultStoreConfig() types.StoreConfig {
	return &StoreConfigImpl{
		TrimAt:     0,
		TrimRemain: 0,
	}
}

type storeConfigAlias StoreConfigImpl

// if trimAt and trimRemain are both 0, it means that the store is disabled
func (s *StoreConfigImpl) ValidateAll() error {
	if s.TrimAt == 0 && s.TrimRemain == 0 {
		return nil
	}
	if s.TrimAt <= 0 {
		return errors.NewConfigError("StoreConfig", "trimAt", "trimAt must be greater than 0")
	}
	if s.TrimRemain < 0 {
		return errors.NewConfigError("StoreConfig", "trimRemain", "trimRemain must be greater than or equal to 0")
	}
	if s.TrimAt < s.TrimRemain {
		slog.Warn("trimAt should be greater than trimRemain", "trimAt", s.TrimAt, "trimRemain", s.TrimRemain)
	}
	return nil
}

func (s *StoreConfigImpl) Validate(key string, value interface{}) error {
	switch key {
	case "trimAt":
		if v, ok := value.(int); ok {
			if v <= 0 {
				return errors.NewConfigError("StoreConfig", key, "trimAt must be greater than 0")
			}
		} else {
			return errors.NewConfigError("StoreConfig", key, fmt.Sprintf("invalid type for trimAt: %T", value))
		}
	case "trimRemain":
		if v, ok := value.(int); ok {
			if v < 0 {
				return errors.NewConfigError("StoreConfig", key, "trimRemain must be greater than or equal to 0")
			}
		} else {
			return errors.NewConfigError("StoreConfig", key, fmt.Sprintf("invalid type for trimRemain: %T", value))
		}
	}
	return nil
}

func (s *StoreConfigImpl) Update(key string, value interface{}) error {
	if err := s.Validate(key, value); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	switch key {
	case "trimAt":
		if v, ok := value.(float64); ok {
			s.TrimAt = int(v)
		} else if v, ok := value.(int); ok {
			s.TrimAt = v
		}
	case "trimRemain":
		if v, ok := value.(float64); ok {
			s.TrimRemain = int(v)
		} else if v, ok := value.(int); ok {
			s.TrimRemain = v
		}
	}
	return nil
}

func (s *StoreConfigImpl) GetTrimAt() int {
	return s.TrimAt
}

func (s *StoreConfigImpl) GetTrimRemain() int {
	return s.TrimRemain
}

func (s *StoreConfigImpl) DeepCopy() types.StoreConfig {
	return &StoreConfigImpl{
		TrimAt:     s.TrimAt,
		TrimRemain: s.TrimRemain,
	}
}
