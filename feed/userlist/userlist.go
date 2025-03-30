package userlist

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/nus25/yuge/feed/errors"
)

type UserList struct {
	logger     *slog.Logger
	Uri        string
	dids       map[string]struct{}
	apiBaseURL string
}

// NewUserList creates a new UserList
func NewUserList(uri string, l *slog.Logger) (*UserList, error) {
	return NewUserListWithHost(uri, "https://public.api.bsky.app", l)
}

// NewUserListWithHost creates a new UserList with a custom apiBaseURL
func NewUserListWithHost(uri string, apiBaseURL string, l *slog.Logger) (*UserList, error) {
	logger := l.With("component", "userlist")
	if uri == "" {
		return nil, errors.NewConfigError("userlist", "uri", "uri is required")
	}
	if apiBaseURL == "" {
		return nil, errors.NewConfigError("userlist", "apiBaseURL", "apiBaseURL is required")
	}

	b := UserList{
		logger:     logger,
		Uri:        uri,
		dids:       make(map[string]struct{}),
		apiBaseURL: apiBaseURL,
	}
	if err := b.Load(); err != nil {
		b.logger.Error("failed to load list", "error", err)
		return nil, err
	}

	return &b, nil
}

// Load loads the user list from the apiBaseURL
func (l *UserList) Load() error {

	l.logger.Info("loading user list", "uri", l.Uri)
	listUrl := l.apiBaseURL + "/xrpc/app.bsky.graph.getList?list=" + l.Uri

	// Create HTTP client
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	defer client.CloseIdleConnections()

	req, err := http.NewRequest("GET", listUrl, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get list: %d, %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var result struct {
		Items []struct {
			Subject struct {
				Did string `json:"did"`
			} `json:"subject"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Add DIDs to the map
	l.dids = make(map[string]struct{}, len(result.Items))
	for _, item := range result.Items {
		l.dids[item.Subject.Did] = struct{}{}
	}

	l.logger.Info("list loaded", "count", len(l.dids))
	l.logger.Debug("list dids", "dids", l.dids)
	return nil
}

// List returns the list of DIDs
func (l *UserList) List() []string {
	dids := make([]string, 0, len(l.dids))
	for did := range l.dids {
		dids = append(dids, did)
	}
	return dids
}

// Contain checks if a DID is in the list
func (l *UserList) Contain(did string) bool {
	_, exists := l.dids[did]
	return exists
}
