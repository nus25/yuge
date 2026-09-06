package subscriber

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nus25/yuge/feed"
)

type FeedInfo struct {
	Definition FeedDefinition
	Feed       feed.Feed
	Status     FeedStatus
}

type FeedStatus struct {
	FeedID      string    `json:"feedId"`
	LastUpdated time.Time `json:"lastUpdated"`
	LastStatus  Status    `json:"lastStatus"`
	Error       string    `json:"error,omitempty"`
}

func (fs *FeedStatus) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"feedId":      fs.FeedID,
		"lastUpdated": fs.LastUpdated.UTC().Format(time.RFC3339),
		"lastStatus":  fs.LastStatus.String(),
	}
	if fs.Error != "" {
		m["error"] = fs.Error
	}
	return json.Marshal(m)
}

func (fs *FeedStatus) SetError(err error) {
	fs.LastStatus = FeedStatusError
	fs.LastUpdated = time.Now()
	if err != nil {
		fs.Error = err.Error()
	} else {
		fs.Error = ""
	}
}

type Status int

const (
	FeedStatusUnknown Status = iota
	FeedStatusActive
	FeedStatusInactive
	FeedStatusError
)

func (s Status) String() string {
	switch s {
	case FeedStatusActive:
		return "active"
	case FeedStatusInactive:
		return "inactive"
	case FeedStatusError:
		return "error"
	default:
		return "unknown"
	}
}

func (s *Status) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	switch value {
	case "active":
		*s = FeedStatusActive
	case "inactive":
		*s = FeedStatusInactive
	case "error":
		*s = FeedStatusError
	case "unknown":
		*s = FeedStatusUnknown
	default:
		return fmt.Errorf("invalid feed status: %s", value)
	}
	return nil
}
