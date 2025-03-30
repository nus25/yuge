package logicblock

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
)

func TestUserListLogicblock(t *testing.T) {
	// テストサーバーを設定
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/app.bsky.graph.getList" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		response := map[string]interface{}{
			"list": map[string]interface{}{
				"uri":           "at://did:plc:testdid/app.bsky.graph.list/testrkey",
				"cid":           "abcyreigf2mooobnoer6abfvlliatgwh7l2l3puamtpe4zxys7pofgmk73e",
				"name":          "My mute list",
				"purpose":       "app.bsky.graph.defs#modlist",
				"listItemCount": 2,
				"indexedAt":     "2023-10-04T15:14:09.654Z",
				"labels":        []interface{}{},
			},
			"items": []map[string]interface{}{
				{
					"uri": "at://did:plc:test1/app.bsky.graph.listitem/test1",
					"subject": map[string]interface{}{
						"did":         "did:plc:jlwuz7v5q3amfktznwjtest1",
						"handle":      "test1.bsky.social",
						"displayName": "Test User 1",
						"avatar":      "https://cdn.example.com/img/avatar/test1.jpg",
						"labels":      []interface{}{},
						"createdAt":   "2023-04-23T02:14:11.398Z",
						"description": "Test user 1 description",
						"indexedAt":   "2024-01-14T14:59:27.533Z",
					},
				},
				{
					"uri": "at://did:plc:test2/app.bsky.graph.listitem/test2",
					"subject": map[string]interface{}{
						"did":         "did:plc:3vdq5uaaqtygwaghfxbtest2",
						"handle":      "test2.bsky.social",
						"displayName": "Test User 2",
						"avatar":      "https://cdn.example.com/img/avatar/test2.jpg",
						"labels":      []interface{}{},
						"createdAt":   "2023-04-23T02:14:11.398Z",
						"description": "Test user 2 description",
						"indexedAt":   "2024-01-14T14:59:27.533Z",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()
	testHost := ts.URL

	tests := []struct {
		name     string
		config   types.LogicBlockConfig
		did      string
		post     *apibsky.FeedPost
		wantErr  bool
		wantPass bool
	}{
		{
			name: "invalid block type",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "invalid",
					Options: map[string]interface{}{
						"subject":    "item",
						"value":      "reply",
						"apiBaseURL": testHost,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "missing listUri",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "userlist",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid listUri type",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "userlist",
					Options: map[string]interface{}{
						"listUri":    123,
						"apiBaseURL": testHost,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty listUri",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "userlist",
					Options: map[string]interface{}{
						"listUri":    "",
						"apiBaseURL": testHost,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid allow type",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "userlist",
					Options: map[string]interface{}{
						"listUri":    "at://did:plc:xxx/app.bsky.graph.list/xxx",
						"allow":      "true",
						"apiBaseURL": testHost,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid config with allow=true",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "userlist",
					Options: map[string]interface{}{
						"listUri":    "at://did:plc:xxx/app.bsky.graph.list/xxx",
						"allow":      true,
						"apiBaseURL": testHost,
					},
				},
			},
			did:      "did:plc:jlwuz7v5q3amfktznwjtest1",
			wantPass: true,
		},
		{
			name: "valid config with allow=false",
			config: &logic.UserListLogicBlockConfig{
				BaseLogicBlockConfig: logic.BaseLogicBlockConfig{
					BlockType: "userlist",
					Options: map[string]interface{}{
						"listUri":    "at://did:plc:xxx/app.bsky.graph.list/xxx",
						"allow":      false,
						"apiBaseURL": testHost,
					},
				},
			},
			did:      "did:plc:3vdq5uaaqtygwaghfxbtest2",
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.Default()
			lb, err := NewUserListLogicBlock(tt.config, logger)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewUserListLogicblock() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				return
			}

			got := lb.Test(tt.did, "constantRkey", nil)
			if got != tt.wantPass {
				t.Errorf("Test() = %v, want %v", got, tt.wantPass)
			}
		})
	}
}
