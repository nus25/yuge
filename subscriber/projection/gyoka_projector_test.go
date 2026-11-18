package projection

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nus25/yuge/subscriber/projection/gyoka"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	"github.com/nus25/yuge/types"
)

type spyGyokaMutator struct {
	addErr            error
	batchAddErr       error
	batchRemoveErr    error
	deleteErr         error
	trimErr           error
	addCalls          int
	batchCalls        int
	batchRemoveCalls  int
	deleteCalls       int
	trimCalls         int
	lastAdd           gyoka.PostParams
	lastBatch         gyoka.BatchPostParams
	lastBatchRemove   gyoka.BatchDeleteParams
	lastDelete        gyoka.DeleteParams
	lastTrim          gyoka.TrimParams
	batchAddParams    []gyoka.BatchPostParams
	batchRemoveParams []gyoka.BatchDeleteParams
}

func (m *spyGyokaMutator) Add(params gyoka.PostParams) error {
	m.addCalls++
	m.lastAdd = params
	return m.addErr
}

func (m *spyGyokaMutator) BatchAdd(params gyoka.BatchPostParams) error {
	m.batchCalls++
	m.lastBatch = params
	m.batchAddParams = append(m.batchAddParams, params)
	return m.batchAddErr
}

func (m *spyGyokaMutator) BatchRemove(params gyoka.BatchDeleteParams) error {
	m.batchRemoveCalls++
	m.lastBatchRemove = params
	m.batchRemoveParams = append(m.batchRemoveParams, params)
	return m.batchRemoveErr
}

func (m *spyGyokaMutator) Delete(params gyoka.DeleteParams) error {
	m.deleteCalls++
	m.lastDelete = params
	return m.deleteErr
}

func (m *spyGyokaMutator) Trim(params gyoka.TrimParams) error {
	m.trimCalls++
	m.lastTrim = params
	return m.trimErr
}

func TestGyokaProjector_Project_AddEntry(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	repostURI := "at://did:plc:reposter/app.bsky.feed.repost/repost-1"
	entry := projectionrepo.Entry{
		ID:          1,
		Target:      "gyoka",
		Operation:   "add",
		PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user1/app.bsky.feed.post/post1","cid":"cid-1","indexedAt":"2026-05-11T08:00:00Z","langs":["ja","en"],"feedContext":"matches the requested topic","reason":{"repost":"at://did:plc:reposter/app.bsky.feed.repost/repost-1"}}}`,
	}

	if err := projector.Project(context.Background(), entry); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if mutator.addCalls != 1 {
		t.Fatalf("Add() calls = %d, want 1", mutator.addCalls)
	}
	if mutator.deleteCalls != 0 {
		t.Fatalf("Delete() calls = %d, want 0", mutator.deleteCalls)
	}
	if mutator.lastAdd.FeedUri != types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample") {
		t.Fatalf("Add() FeedUri = %s", mutator.lastAdd.FeedUri)
	}
	if mutator.lastAdd.Did != "did:plc:user1" || mutator.lastAdd.Rkey != "post1" || mutator.lastAdd.Cid != "cid-1" {
		t.Fatalf("Add() params = %+v", mutator.lastAdd)
	}
	if !reflect.DeepEqual(mutator.lastAdd.Langs, []string{"ja", "en"}) {
		t.Fatalf("Add() langs = %v, want [ja en]", mutator.lastAdd.Langs)
	}
	if !mutator.lastAdd.IndexedAt.Equal(time.Date(2026, 5, 11, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("Add() IndexedAt = %v", mutator.lastAdd.IndexedAt)
	}
	if mutator.lastAdd.Reason == nil || mutator.lastAdd.Reason.Repost == nil || *mutator.lastAdd.Reason.Repost != repostURI {
		t.Fatalf("Add() Reason = %+v, want repost %q", mutator.lastAdd.Reason, repostURI)
	}
}

func TestGyokaProjector_Project_DeleteEntry(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entry := projectionrepo.Entry{
		ID:          2,
		Target:      "gyoka",
		Operation:   "delete",
		PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user2/app.bsky.feed.post/post2","cid":"cid-2","indexedAt":"2026-05-11T08:01:00Z","langs":["ja"]}}`,
	}

	if err := projector.Project(context.Background(), entry); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if mutator.deleteCalls != 1 {
		t.Fatalf("Delete() calls = %d, want 1", mutator.deleteCalls)
	}
	if mutator.addCalls != 0 {
		t.Fatalf("Add() calls = %d, want 0", mutator.addCalls)
	}
	if mutator.lastDelete.FeedUri != types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample") {
		t.Fatalf("Delete() FeedUri = %s", mutator.lastDelete.FeedUri)
	}
	if mutator.lastDelete.Did != "did:plc:user2" || mutator.lastDelete.Rkey != "post2" {
		t.Fatalf("Delete() params = %+v", mutator.lastDelete)
	}
}

func TestGyokaProjector_Project_RejectsUnsupportedOperation(t *testing.T) {
	t.Parallel()

	projector := NewGyokaProjector(&spyGyokaMutator{})
	err := projector.Project(context.Background(), projectionrepo.Entry{
		ID:          3,
		Target:      "gyoka",
		Operation:   "noop",
		PayloadJSON: `{}`,
	})
	if err == nil {
		t.Fatal("Project() error = nil, want error")
	}
	if !errors.Is(err, ErrUnsupportedProjectionOperation) {
		t.Fatalf("Project() error = %v, want ErrUnsupportedProjectionOperation", err)
	}
}

func TestGyokaProjector_Project_TrimEntry(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entry := projectionrepo.Entry{
		ID:          3,
		Target:      "gyoka",
		Operation:   "trim",
		PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","count":0}`,
	}

	if err := projector.Project(context.Background(), entry); err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if mutator.trimCalls != 1 {
		t.Fatalf("Trim() calls = %d, want 1", mutator.trimCalls)
	}
	if mutator.lastTrim.FeedUri != types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample") {
		t.Fatalf("Trim() FeedUri = %s", mutator.lastTrim.FeedUri)
	}
	if mutator.lastTrim.Count != 0 {
		t.Fatalf("Trim() Count = %d, want 0", mutator.lastTrim.Count)
	}
}

func TestGyokaProjector_ProjectBatch_AddEntries(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entries := []projectionrepo.Entry{
		{
			ID:          1,
			Target:      "gyoka",
			Operation:   "add",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user1/app.bsky.feed.post/post1","cid":"cid-1","indexedAt":"2026-05-11T08:00:00Z","langs":["ja"],"feedContext":"matches the requested topic"}}`,
		},
		{
			ID:          2,
			Target:      "gyoka",
			Operation:   "add",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user2/app.bsky.feed.post/post2","cid":"cid-2","indexedAt":"2026-05-11T08:01:00Z","langs":["en"]}}`,
		},
	}

	if err := projector.ProjectBatch(context.Background(), entries); err != nil {
		t.Fatalf("ProjectBatch() error = %v", err)
	}
	if mutator.batchCalls != 1 {
		t.Fatalf("BatchAdd() calls = %d, want 1", mutator.batchCalls)
	}
	if mutator.addCalls != 0 {
		t.Fatalf("Add() calls = %d, want 0", mutator.addCalls)
	}
	if len(mutator.lastBatch.Posts) != 2 {
		t.Fatalf("BatchAdd() posts len = %d, want 2", len(mutator.lastBatch.Posts))
	}
	if mutator.lastBatch.Posts[0].Did != "did:plc:user1" || mutator.lastBatch.Posts[1].Did != "did:plc:user2" {
		t.Fatalf("BatchAdd() posts = %+v", mutator.lastBatch.Posts)
	}
}

func TestGyokaProjector_ProjectBatch_SplitsAddEntriesByFeed(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entries := []projectionrepo.Entry{
		{ID: 1, Target: "gyoka", Operation: "add", PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/feed-a","post":{"uri":"at://did:plc:user1/app.bsky.feed.post/post1","cid":"cid-1","indexedAt":"2026-05-11T08:00:00Z"}}`},
		{ID: 2, Target: "gyoka", Operation: "add", PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/feed-b","post":{"uri":"at://did:plc:user2/app.bsky.feed.post/post2","cid":"cid-2","indexedAt":"2026-05-11T08:01:00Z"}}`},
		{ID: 3, Target: "gyoka", Operation: "add", PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/feed-a","post":{"uri":"at://did:plc:user3/app.bsky.feed.post/post3","cid":"cid-3","indexedAt":"2026-05-11T08:02:00Z"}}`},
	}

	if err := projector.ProjectBatch(context.Background(), entries); err != nil {
		t.Fatalf("ProjectBatch() error = %v", err)
	}
	if mutator.batchCalls != 2 {
		t.Fatalf("BatchAdd() calls = %d, want 2", mutator.batchCalls)
	}
	if got := mutator.batchAddParams[0]; got.FeedUri != "at://did:plc:test/app.bsky.feed.generator/feed-a" || len(got.Posts) != 2 || got.Posts[0].Did != "did:plc:user1" || got.Posts[1].Did != "did:plc:user3" {
		t.Fatalf("first BatchAdd() params = %+v, want feed-a posts user1 and user3", got)
	}
	if got := mutator.batchAddParams[1]; got.FeedUri != "at://did:plc:test/app.bsky.feed.generator/feed-b" || len(got.Posts) != 1 || got.Posts[0].Did != "did:plc:user2" {
		t.Fatalf("second BatchAdd() params = %+v, want feed-b post user2", got)
	}
}

func TestGyokaProjector_ProjectBatch_DeleteEntries(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entries := []projectionrepo.Entry{
		{
			ID:          1,
			Target:      "gyoka",
			Operation:   "delete",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}}`,
		},
		{
			ID:          2,
			Target:      "gyoka",
			Operation:   "delete",
			PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}}`,
		},
	}

	if err := projector.ProjectBatch(context.Background(), entries); err != nil {
		t.Fatalf("ProjectBatch() error = %v", err)
	}
	if mutator.batchRemoveCalls != 1 {
		t.Fatalf("BatchRemove() calls = %d, want 1", mutator.batchRemoveCalls)
	}
	if mutator.deleteCalls != 0 {
		t.Fatalf("Delete() calls = %d, want 0", mutator.deleteCalls)
	}
	if len(mutator.lastBatchRemove.Uris) != 2 {
		t.Fatalf("BatchRemove() uris len = %d, want 2", len(mutator.lastBatchRemove.Uris))
	}
	if mutator.lastBatchRemove.Uris[0].Did != "did:plc:user1" || mutator.lastBatchRemove.Uris[1].Rkey != "post2" {
		t.Fatalf("BatchRemove() uris = %+v", mutator.lastBatchRemove.Uris)
	}
}

func TestGyokaProjector_ProjectBatch_SplitsDeleteEntriesByFeed(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entries := []projectionrepo.Entry{
		{ID: 1, Target: "gyoka", Operation: "delete", PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/feed-a","post":{"uri":"at://did:plc:user1/app.bsky.feed.post/post1"}}`},
		{ID: 2, Target: "gyoka", Operation: "delete", PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/feed-b","post":{"uri":"at://did:plc:user2/app.bsky.feed.post/post2"}}`},
		{ID: 3, Target: "gyoka", Operation: "delete", PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/feed-a","post":{"uri":"at://did:plc:user3/app.bsky.feed.post/post3"}}`},
	}

	if err := projector.ProjectBatch(context.Background(), entries); err != nil {
		t.Fatalf("ProjectBatch() error = %v", err)
	}
	if mutator.batchRemoveCalls != 2 {
		t.Fatalf("BatchRemove() calls = %d, want 2", mutator.batchRemoveCalls)
	}
	if got := mutator.batchRemoveParams[0]; got.FeedUri != "at://did:plc:test/app.bsky.feed.generator/feed-a" || len(got.Uris) != 2 || got.Uris[0].Did != "did:plc:user1" || got.Uris[1].Did != "did:plc:user3" {
		t.Fatalf("first BatchRemove() params = %+v, want feed-a posts user1 and user3", got)
	}
	if got := mutator.batchRemoveParams[1]; got.FeedUri != "at://did:plc:test/app.bsky.feed.generator/feed-b" || len(got.Uris) != 1 || got.Uris[0].Did != "did:plc:user2" {
		t.Fatalf("second BatchRemove() params = %+v, want feed-b post user2", got)
	}
}
