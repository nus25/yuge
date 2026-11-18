package gyoka

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	client "github.com/nus25/gyoka-client/go-atproto"
	gyokaschema "github.com/nus25/gyoka-client/go-atproto/schema/gyoka"
)

type fakeGyokaAPI struct {
	pingErr     error
	putInput    *gyokaschema.FeedPutPosts_Input
	removeInput *gyokaschema.FeedRemovePosts_Input
}

// check type
var _ GyokaAPI = (*fakeGyokaAPI)(nil)

func (a *fakeGyokaAPI) Ping(context.Context) error { return a.pingErr }

func (a *fakeGyokaAPI) PutPosts(_ context.Context, input *gyokaschema.FeedPutPosts_Input) error {
	a.putInput = input
	return nil
}

func (a *fakeGyokaAPI) RemovePosts(_ context.Context, input *gyokaschema.FeedRemovePosts_Input) error {
	a.removeInput = input
	return nil
}

func (a *fakeGyokaAPI) RemovePostsByAuthor(context.Context, *gyokaschema.FeedRemovePostsByAuthor_Input) error {
	return nil
}

func (a *fakeGyokaAPI) TrimPosts(context.Context, *gyokaschema.FeedTrimPosts_Input) error {
	return nil
}

func (a *fakeGyokaAPI) ListPosts(ctx context.Context, feed string, postUri string, author string, cursor string, limit int64) (*gyokaschema.FeedListPosts_Output, error) {
	return nil, nil
}

func TestGyokaEditor_AddMapsATProtoLexiconInput(t *testing.T) {
	api := &fakeGyokaAPI{}
	editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))
	indexedAt := time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC)

	if err := editor.Add(PostParams{
		FeedUri:   "at://did:plc:feed/app.bsky.feed.generator/sample",
		Did:       "did:plc:author",
		Rkey:      "post-1",
		Cid:       "bafy-test",
		IndexedAt: indexedAt,
		Langs:     []string{"ja"},
	}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if api.putInput == nil {
		t.Fatal("PutPosts() input is nil")
	}
	if api.putInput.Feed != "at://did:plc:feed/app.bsky.feed.generator/sample" {
		t.Errorf("feed = %q", api.putInput.Feed)
	}
	if api.putInput.Posts[0].Uri != "at://did:plc:author/app.bsky.feed.post/post-1" {
		t.Errorf("post URI = %q", api.putInput.Posts[0].Uri)
	}
	if api.putInput.Posts[0].IndexedAt == nil || *api.putInput.Posts[0].IndexedAt != indexedAt.Format(time.RFC3339Nano) {
		t.Errorf("indexedAt = %v, want %s", api.putInput.Posts[0].IndexedAt, indexedAt.Format(time.RFC3339Nano))
	}
	if api.putInput.Posts[0].Reason != nil {
		t.Errorf("reason = %v, want nil", api.putInput.Posts[0].Reason)
	}
}

func TestGyokaEditor_AddMapsOptionalPostMetadata(t *testing.T) {
	repostURI := "at://did:plc:reposter/app.bsky.feed.repost/repost-1"
	tests := []struct {
		name       string
		reason     *PostReason
		wantRepost string
		wantPin    bool
	}{
		{
			name:       "repost reason",
			reason:     &PostReason{Repost: &repostURI},
			wantRepost: repostURI,
		},
		{
			name:    "pin reason",
			reason:  &PostReason{Pin: true},
			wantPin: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &fakeGyokaAPI{}
			editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))

			if err := editor.Add(PostParams{
				FeedUri:   "at://did:plc:feed/app.bsky.feed.generator/sample",
				Did:       "did:plc:author",
				Rkey:      "post-1",
				Cid:       "bafy-test",
				IndexedAt: time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC),
				Reason:    test.reason,
			}); err != nil {
				t.Fatalf("Add() error = %v", err)
			}

			post := api.putInput.Posts[0]
			if test.wantRepost != "" {
				if post.Reason == nil || post.Reason.FeedDefs_SkeletonReasonRepost == nil || post.Reason.FeedDefs_SkeletonReasonRepost.Repost != test.wantRepost {
					t.Fatalf("repost reason = %+v, want %q", post.Reason, test.wantRepost)
				}
			}
			if test.wantPin {
				if post.Reason == nil || post.Reason.FeedDefs_SkeletonReasonPin == nil {
					t.Fatalf("pin reason = %+v, want pin", post.Reason)
				}
			}
		})
	}
}

func TestGyokaEditor_BatchAddMapsOptionalPostMetadata(t *testing.T) {
	repostURI := "at://did:plc:reposter/app.bsky.feed.repost/repost-1"
	api := &fakeGyokaAPI{}
	editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))

	if err := editor.BatchAdd(BatchPostParams{
		FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
		Posts: []PostParams{
			{
				FeedUri:   "at://did:plc:feed/app.bsky.feed.generator/sample",
				Did:       "did:plc:author1",
				Rkey:      "post-1",
				Cid:       "bafy-test-1",
				IndexedAt: time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC),
				Reason:    &PostReason{Repost: &repostURI},
			},
			{
				FeedUri:   "at://did:plc:feed/app.bsky.feed.generator/sample",
				Did:       "did:plc:author2",
				Rkey:      "post-2",
				Cid:       "bafy-test-2",
				IndexedAt: time.Date(2026, 8, 22, 12, 31, 0, 0, time.UTC),
				Reason:    &PostReason{Pin: true},
			},
		}}); err != nil {
		t.Fatalf("BatchAdd() error = %v", err)
	}

	if api.putInput == nil || len(api.putInput.Posts) != 2 {
		t.Fatalf("BatchAddPosts() input = %+v", api.putInput)
	}
	posts := api.putInput.Posts

	if posts[0].Reason == nil || posts[0].Reason.FeedDefs_SkeletonReasonRepost == nil || posts[0].Reason.FeedDefs_SkeletonReasonRepost.Repost != repostURI {
		t.Errorf("repost reason = %+v, want %q", posts[0].Reason, repostURI)
	}
	if posts[1].Reason == nil || posts[1].Reason.FeedDefs_SkeletonReasonPin == nil {
		t.Errorf("pin reason = %+v, want pin", posts[1].Reason)
	}
}

func TestGyokaEditor_AddRejectsInvalidPostReason(t *testing.T) {
	repostURI := "at://did:plc:reposter/app.bsky.feed.repost/repost-1"
	for _, test := range []struct {
		name   string
		reason *PostReason
	}{
		{name: "empty", reason: &PostReason{}},
		{name: "repost and pin", reason: &PostReason{Repost: &repostURI, Pin: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &fakeGyokaAPI{}
			editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))
			err := editor.Add(PostParams{
				FeedUri:   "at://did:plc:feed/app.bsky.feed.generator/sample",
				Did:       "did:plc:author",
				Rkey:      "post-1",
				Cid:       "bafy-test",
				IndexedAt: time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC),
				Reason:    test.reason,
			})
			if !errors.Is(err, ErrInvalidPostReason) {
				t.Fatalf("Add() error = %v, want ErrInvalidPostReason", err)
			}
			if api.putInput != nil {
				t.Fatalf("AddPost() input = %+v, want nil", api.putInput)
			}
		})
	}
}

func TestNewGyokaEditorUsesInterServiceAuthenticationWhenPrivateKeyIsConfigured(t *testing.T) {
	const (
		issuerDID = "did:plc:ewvi7nxzyoun6zhxrhsample"
		audience  = "did:web:gyoka.example.com#gyoka_editor"
	)

	privateKey, err := atcrypto.GeneratePrivateKeyP256()
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	publicKey, err := privateKey.PublicKey()
	if err != nil {
		t.Fatalf("get public key: %v", err)
	}
	directory := identity.NewMockDirectory()
	directory.Insert(identity.Identity{
		DID: syntax.DID(issuerDID),
		Keys: map[string]identity.VerificationMethod{
			"atproto": {Type: "Multikey", PublicKeyMultibase: publicKey.Multibase()},
		},
	})
	validator := auth.ServiceAuthValidator{Audience: audience, Dir: directory}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/xrpc/net.nusno.gyoka.ping" {
			t.Errorf("request path = %q, want ping endpoint", request.URL.Path)
		}
		if proxy := request.Header.Get("Atproto-Proxy"); proxy != "" {
			t.Errorf("Atproto-Proxy = %q, want empty", proxy)
		}
		endpoint := syntax.NSID("net.nusno.gyoka.ping")
		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if _, err := validator.Validate(request.Context(), token, &endpoint); err != nil {
			t.Errorf("validate service auth JWT: %v", err)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"message":"ok"}`))
	}))
	defer server.Close()

	editor, err := NewGyokaEditor(context.Background(), ClientConfig{
		Host:         server.URL,
		Audience:     audience,
		UserIdentity: issuerDID,
		AppPassword:  "app-password-that-must-not-be-used",
		PrivateKey:   privateKey,
	}, nil, WithMinRequestInterval(0))
	if err != nil {
		t.Fatalf("NewGyokaEditor() error = %v", err)
	}
	if err := editor.Open(context.Background()); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
}

func TestClassifyGyokaError(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		wantRetryable bool
	}{
		{name: "bad request", statusCode: http.StatusBadRequest, wantRetryable: false},
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantRetryable: false},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, wantRetryable: true},
		{name: "service unavailable", statusCode: http.StatusServiceUnavailable, wantRetryable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyGyokaError(&client.APIError{StatusCode: tt.statusCode})
			var nonRetryable *NonRetryableError
			if gotRetryable := !errors.As(err, &nonRetryable); gotRetryable != tt.wantRetryable {
				t.Fatalf("retryable = %t, want %t", gotRetryable, tt.wantRetryable)
			}
		})
	}
}

func TestGyokaEditor_BatchAddRejectsOversizedBatch(t *testing.T) {
	entries := make([]PostParams, maxBatchSize+1)
	if err := newGyokaEditor(&fakeGyokaAPI{}, nil).BatchAdd(BatchPostParams{
		FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
		Posts:   entries}); err == nil {
		t.Fatal("BatchAdd() error = nil, want batch size error")
	}
}

func TestGyokaEditor_BatchRemoveMapsATProtoLexiconInput(t *testing.T) {
	api := &fakeGyokaAPI{}
	editor := newGyokaEditor(api, nil, WithMinRequestInterval(0))

	if err := editor.BatchRemove(BatchDeleteParams{
		FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
		Uris: []DeleteParams{
			{
				FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
				Did:     "did:plc:author1",
				Rkey:    "post-1",
			},
			{
				FeedUri: "at://did:plc:feed/app.bsky.feed.generator/sample",
				Did:     "did:plc:author2",
				Rkey:    "post-2",
			},
		}}); err != nil {
		t.Fatalf("BatchRemove() error = %v", err)
	}

	if api.removeInput == nil {
		t.Fatal("BatchRemovePosts() input is nil")
	}
	if api.removeInput.Feed != "at://did:plc:feed/app.bsky.feed.generator/sample" {
		t.Errorf("feed = %q", api.removeInput.Feed)
	}
	if len(api.removeInput.Uris) != 2 {
		t.Fatalf("posts len = %d, want 2", len(api.removeInput.Uris))
	}
	if api.removeInput.Uris[0] != "at://did:plc:author1/app.bsky.feed.post/post-1" {
		t.Errorf("first post URI = %q", api.removeInput.Uris[0])
	}
	if api.removeInput.Uris[1] != "at://did:plc:author2/app.bsky.feed.post/post-2" {
		t.Errorf("second post URI = %q", api.removeInput.Uris[1])
	}
}
