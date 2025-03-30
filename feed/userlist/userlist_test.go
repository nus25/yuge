package userlist

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"log/slog"
)

type testItems struct {
	Uri     string           `json:"uri"`
	Subject testItemsSubject `json:"subject"`
}

type testItemsSubject struct {
	Did         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
}

type testListResponse struct {
	List struct {
		Uri           string        `json:"uri"`
		Cid           string        `json:"cid"`
		Name          string        `json:"name"`
		Purpose       string        `json:"purpose"`
		ListItemCount int           `json:"listItemCount"`
		IndexedAt     string        `json:"indexedAt"`
		Labels        []interface{} `json:"labels"`
		Creator       struct {
			Did         string `json:"did"`
			Handle      string `json:"handle"`
			DisplayName string `json:"displayName"`
			Avatar      string `json:"avatar"`
		} `json:"creator"`
		Description string `json:"description"`
	} `json:"list"`
	Items []testItems `json:"items"`
}

func TestUserList(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *httptest.Server
		did      string
		wantErr  bool
		wantPass bool
	}{
		{
			name: "valid list contains did",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := testListResponse{}
					resp.List.Uri = "test-uri"
					resp.List.Name = "Test List Name"
					resp.Items = []testItems{
						{
							Uri: "test-item-uri-1",
							Subject: testItemsSubject{
								Did:         "test-subject-did-1",
								Handle:      "test-subject-handle-1",
								DisplayName: "Test Subject Display Name 1",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
				}))
			},
			did:      "test-subject-did-1",
			wantPass: true,
		},
		{
			name: "valid list does not contain did",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := testListResponse{}
					resp.List.Uri = "test-uri"
					resp.List.Name = "Test List Name"
					resp.Items = []testItems{
						{
							Uri: "test-item-uri-1",
							Subject: testItemsSubject{
								Did:         "test-subject-did-1",
								Handle:      "test-subject-handle-1",
								DisplayName: "Test Subject Display Name 1",
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(resp)
				}))
			},
			did:      "non-existent-did",
			wantPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := tt.setup()
			defer ts.Close()

			logger := slog.Default()
			ul, err := NewUserListWithHost(ts.URL, ts.URL, logger)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expected error = %v, got %v", tt.wantErr, err)
			}
			if err != nil {
				return
			}

			got := ul.Contain(tt.did)
			if got != tt.wantPass {
				t.Errorf("Contain() = %v, want %v", got, tt.wantPass)
			}
		})
	}
}
