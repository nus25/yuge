package subscriber

import (
	"encoding/json"
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
	m := map[string]interface{}{
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
