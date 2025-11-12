package cli

import (
	"context"
	"encoding/json"
	"io"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/xrpc"
)

var _ ATProtoClient = (*XRPCClientWrapper)(nil) //type check

type repoListRecordsRespWithRawRecord struct {
	Cursor  *string                     `json:"cursor,omitempty"`
	Records []*repoRecordWithRawMessage `json:"records"`
}

type repoRecordWithRawMessage struct {
	Uri   string          `json:"uri"`
	Cid   string          `json:"cid"`
	Value json.RawMessage `json:"value"`
}

type RepoPutRecordWithRawRecord_Input struct {
	atproto.RepoPutRecord_Input
	Record json.RawMessage `json:"record"` // LexiconTypeDecoder型のフィールドの代わりにRawMessageを使う
}

// ATProtoClient defines the interface for AT Protocol operations
type ATProtoClient interface {
	GetRecord(ctx context.Context, collection, did, rkey string) (*atproto.RepoGetRecord_Output, error)
	ListRecords(ctx context.Context, repo, collection string, limit int64, cursor *string, reverse bool) (*repoListRecordsRespWithRawRecord, error)
	UploadBlob(ctx context.Context, data io.Reader) (*atproto.RepoUploadBlob_Output, error)
	PutRecord(ctx context.Context, input *RepoPutRecordWithRawRecord_Input) (*atproto.RepoPutRecord_Output, error)
	DeleteRecord(ctx context.Context, input *atproto.RepoDeleteRecord_Input) (*atproto.RepoDeleteRecord_Output, error)
	CreateSession(ctx context.Context, input *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error)
	DeleteSession(ctx context.Context) error
	GetDID() string
}

// XRPCClientWrapper wraps xrpc.Client to implement ATProtoClient interface
type XRPCClientWrapper struct {
	client *xrpc.Client
}

// NewXRPCClientWrapper creates a new wrapper for xrpc.Client
func NewXRPCClientWrapper(host string) *XRPCClientWrapper {
	return &XRPCClientWrapper{
		client: &xrpc.Client{Host: host},
	}
}

// GetRecord retrieves a record from the repository
func (w *XRPCClientWrapper) GetRecord(ctx context.Context, collection, did, rkey string) (*atproto.RepoGetRecord_Output, error) {
	return atproto.RepoGetRecord(ctx, w.client, "", collection, did, rkey)
}

// UploadBlob uploads a blob to the repository
func (w *XRPCClientWrapper) UploadBlob(ctx context.Context, data io.Reader) (*atproto.RepoUploadBlob_Output, error) {
	return atproto.RepoUploadBlob(ctx, w.client, data)
}

// PutRecord puts a record into the repository
func (w *XRPCClientWrapper) PutRecord(ctx context.Context, input *RepoPutRecordWithRawRecord_Input) (*atproto.RepoPutRecord_Output, error) {
	body := map[string]any{
		"repo":       input.Repo,
		"collection": input.Collection,
		"rkey":       input.Rkey,
		"validate":   *input.Validate,
		"record":     input.Record,
	}
	if input.SwapCommit != nil {
		body["swapCommit"] = *input.SwapCommit
	}
	if input.SwapRecord != nil {
		body["swapRecord"] = *input.SwapRecord
	}

	var resp = atproto.RepoPutRecord_Output{}
	if err := w.client.Do(ctx, xrpc.Procedure, "application/json", "com.atproto.repo.putRecord", nil, body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListRecords lists records from the repository
func (w *XRPCClientWrapper) ListRecords(ctx context.Context, repo, collection string, limit int64, cursor *string, reverse bool) (*repoListRecordsRespWithRawRecord, error) {
	params := map[string]any{
		"repo":       repo,
		"collection": collection,
		"limit":      limit,
	}
	if cursor != nil {
		params["cursor"] = *cursor
	}

	var resp repoListRecordsRespWithRawRecord
	if err := w.client.Do(ctx, xrpc.Query, "", "com.atproto.repo.listRecords", params, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteRecord deletes a record from the repository
func (w *XRPCClientWrapper) DeleteRecord(ctx context.Context, input *atproto.RepoDeleteRecord_Input) (*atproto.RepoDeleteRecord_Output, error) {
	return atproto.RepoDeleteRecord(ctx, w.client, input)
}

// CreateSession creates a new session
func (w *XRPCClientWrapper) CreateSession(ctx context.Context, input *atproto.ServerCreateSession_Input) (*atproto.ServerCreateSession_Output, error) {
	output, err := atproto.ServerCreateSession(ctx, w.client, input)
	if err != nil {
		return nil, err
	}

	// Set auth info on the client
	w.client.Auth = &xrpc.AuthInfo{
		AccessJwt:  output.AccessJwt,
		RefreshJwt: output.RefreshJwt,
		Handle:     output.Handle,
		Did:        output.Did,
	}

	return output, nil
}

// DeleteSession deletes the current session
func (w *XRPCClientWrapper) DeleteSession(ctx context.Context) error {
	if w.client.Auth != nil {
		// Use refresh token to delete session
		w.client.Auth.AccessJwt = w.client.Auth.RefreshJwt
		return atproto.ServerDeleteSession(ctx, w.client)
	}
	return nil
}

// GetDID returns the DID of the authenticated user
func (w *XRPCClientWrapper) GetDID() string {
	if w.client.Auth != nil {
		return w.client.Auth.Did
	}
	return ""
}
