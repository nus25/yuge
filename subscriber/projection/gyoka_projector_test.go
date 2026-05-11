package projection

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nus25/yuge/feed/store/editor"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	"github.com/nus25/yuge/types"
)

type spyGyokaMutator struct {
	addErr      error
	deleteErr   error
	addCalls    int
	deleteCalls int
	lastAdd     editor.PostParams
	lastDelete  editor.DeleteParams
}

func (m *spyGyokaMutator) Add(params editor.PostParams) error {
	m.addCalls++
	m.lastAdd = params
	return m.addErr
}

func (m *spyGyokaMutator) Delete(params editor.DeleteParams) error {
	m.deleteCalls++
	m.lastDelete = params
	return m.deleteErr
}

func TestGyokaProjector_Project_AddEntry(t *testing.T) {
	t.Parallel()

	mutator := &spyGyokaMutator{}
	projector := NewGyokaProjector(mutator)
	entry := projectionrepo.Entry{
		ID:          1,
		Target:      "gyoka",
		Operation:   "add",
		PayloadJSON: `{"feedUri":"at://did:plc:test/app.bsky.feed.generator/sample","post":{"uri":"at://did:plc:user1/app.bsky.feed.post/post1","cid":"cid-1","indexedAt":"2026-05-11T08:00:00Z","langs":["ja","en"]}}`,
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
		Operation:   "trim",
		PayloadJSON: `{}`,
	})
	if err == nil {
		t.Fatal("Project() error = nil, want error")
	}
	if !errors.Is(err, ErrUnsupportedProjectionOperation) {
		t.Fatalf("Project() error = %v, want ErrUnsupportedProjectionOperation", err)
	}
}
