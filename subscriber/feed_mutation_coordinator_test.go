package subscriber

import (
	"context"
	"errors"
	"testing"
	"time"

	storerepo "github.com/nus25/yuge/feed/store/repository"
	projectionrepo "github.com/nus25/yuge/subscriber/projection/repository"
	"github.com/nus25/yuge/types"
)

type fakeFeedMutationTransactor struct {
	feedRepo   *fakeFeedRepository
	outboxRepo *fakeOutboxRepository
	committed  bool
	rolledBack bool
}

func (t *fakeFeedMutationTransactor) WithinTx(ctx context.Context, fn func(context.Context, storerepo.FeedRepository, projectionrepo.OutboxRepository) error) error {
	err := fn(ctx, t.feedRepo, t.outboxRepo)
	if err != nil {
		t.rolledBack = true
		return err
	}
	t.committed = true
	return nil
}

type fakeFeedRepository struct {
	putPostErr    error
	putCalls      int
	lastPut       storerepo.PutPostParams
	deletePostErr error
	deleteCalls   int
	lastDelete    storerepo.DeletePostParams
	trimmedPosts  []types.Post
	trimErr       error
	trimCalls     int
	lastTrim      storerepo.TrimOverflowParams
}

func (r *fakeFeedRepository) PutPost(ctx context.Context, params storerepo.PutPostParams) error {
	r.putCalls++
	r.lastPut = params
	return r.putPostErr
}

func (r *fakeFeedRepository) ListPosts(ctx context.Context, params storerepo.ListPostsParams) ([]types.Post, error) {
	return nil, nil
}

func (r *fakeFeedRepository) DeletePost(ctx context.Context, params storerepo.DeletePostParams) error {
	r.deleteCalls++
	r.lastDelete = params
	return r.deletePostErr
}

func (r *fakeFeedRepository) TrimOverflow(ctx context.Context, params storerepo.TrimOverflowParams) ([]types.Post, error) {
	r.trimCalls++
	r.lastTrim = params
	trimmed := make([]types.Post, len(r.trimmedPosts))
	copy(trimmed, r.trimmedPosts)
	return trimmed, r.trimErr
}

func (r *fakeFeedRepository) PutFeedState(ctx context.Context, params storerepo.PutFeedStateParams) error {
	return nil
}

func (r *fakeFeedRepository) GetFeedState(ctx context.Context, feedID string) (storerepo.FeedState, bool, error) {
	return storerepo.FeedState{}, false, nil
}

type fakeOutboxRepository struct {
	enqueueErr   error
	lastEnqueue  projectionrepo.EnqueueParams
	enqueueCalls int
}

func (r *fakeOutboxRepository) Enqueue(ctx context.Context, params projectionrepo.EnqueueParams) error {
	r.enqueueCalls++
	r.lastEnqueue = params
	return r.enqueueErr
}

func (r *fakeOutboxRepository) ListByStatus(ctx context.Context, params projectionrepo.ListByStatusParams) ([]projectionrepo.Entry, error) {
	return nil, nil
}

func (r *fakeOutboxRepository) CountByStatus(ctx context.Context, params projectionrepo.CountByStatusParams) ([]projectionrepo.StatusCount, error) {
	return nil, nil
}

func (r *fakeOutboxRepository) ClaimNextPending(ctx context.Context, params projectionrepo.ClaimNextPendingParams) (projectionrepo.Entry, bool, error) {
	return projectionrepo.Entry{}, false, nil
}

func (r *fakeOutboxRepository) ClaimNextPendingBatch(ctx context.Context, params projectionrepo.ClaimNextPendingBatchParams) ([]projectionrepo.Entry, bool, error) {
	return nil, false, nil
}

func (r *fakeOutboxRepository) MarkCompleted(ctx context.Context, params projectionrepo.MarkCompletedParams) error {
	return nil
}

func (r *fakeOutboxRepository) MarkRetryableFailure(ctx context.Context, params projectionrepo.MarkRetryableFailureParams) error {
	return nil
}

func (r *fakeOutboxRepository) MarkFailed(ctx context.Context, params projectionrepo.MarkFailedParams) error {
	return nil
}

func (r *fakeOutboxRepository) MarkDead(ctx context.Context, params projectionrepo.MarkDeadParams) error {
	return nil
}

func (r *fakeOutboxRepository) Requeue(ctx context.Context, params projectionrepo.RequeueParams) error {
	return nil
}

func (r *fakeOutboxRepository) Delete(ctx context.Context, params projectionrepo.DeleteParams) error {
	return nil
}

func (r *fakeOutboxRepository) PurgeCompleted(ctx context.Context, params projectionrepo.PurgeCompletedParams) (int64, error) {
	return 0, nil
}

func TestFeedMutationCoordinator_AddPost_RollsBackWhenOutboxEnqueueFails(t *testing.T) {
	t.Parallel()

	transactor := &fakeFeedMutationTransactor{
		feedRepo:   &fakeFeedRepository{},
		outboxRepo: &fakeOutboxRepository{enqueueErr: errors.New("enqueue failed")},
	}
	coordinator := NewFeedMutationCoordinator(transactor)

	err := coordinator.AddPost(context.Background(), AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Did:        "did:plc:user1",
		Rkey:       "post1",
		Cid:        "cid-1",
		IndexedAt:  time.Date(2026, 5, 11, 4, 0, 0, 0, time.UTC),
		Langs:      []string{"ja"},
		MutationID: "mutation-1",
	})
	if err == nil {
		t.Fatal("expected AddPost to fail when outbox enqueue fails")
	}
	if transactor.committed {
		t.Fatal("transaction committed on enqueue failure")
	}
	if !transactor.rolledBack {
		t.Fatal("transaction did not roll back on enqueue failure")
	}
}

func TestFeedMutationCoordinator_AddPost_CommitsAndBuildsContracts(t *testing.T) {
	t.Parallel()

	transactor := &fakeFeedMutationTransactor{
		feedRepo:   &fakeFeedRepository{},
		outboxRepo: &fakeOutboxRepository{},
	}
	coordinator := NewFeedMutationCoordinator(transactor)

	err := coordinator.AddPost(context.Background(), AddPostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Did:        "did:plc:user1",
		Rkey:       "post1",
		Cid:        "cid-1",
		IndexedAt:  time.Date(2026, 5, 11, 4, 0, 0, 0, time.UTC),
		Langs:      []string{"ja"},
		MutationID: "mutation-1",
	})
	if err != nil {
		t.Fatalf("AddPost() error = %v", err)
	}
	if !transactor.committed {
		t.Fatal("transaction was not committed on success")
	}
	if transactor.rolledBack {
		t.Fatal("transaction rolled back on success")
	}
	if transactor.feedRepo.putCalls != 1 {
		t.Fatalf("PutPost calls = %d, want 1", transactor.feedRepo.putCalls)
	}
	if transactor.outboxRepo.enqueueCalls != 1 {
		t.Fatalf("Enqueue calls = %d, want 1", transactor.outboxRepo.enqueueCalls)
	}
	if transactor.feedRepo.lastPut.Post.Feed != types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample") {
		t.Fatalf("PutPost feed = %s", transactor.feedRepo.lastPut.Post.Feed)
	}
	if transactor.outboxRepo.lastEnqueue.OpKey != "mutation-1:add:feed-1:at://did:plc:user1/app.bsky.feed.post/post1" {
		t.Fatalf("OpKey = %s", transactor.outboxRepo.lastEnqueue.OpKey)
	}
	if transactor.outboxRepo.lastEnqueue.Target != "gyoka" {
		t.Fatalf("Target = %s, want gyoka", transactor.outboxRepo.lastEnqueue.Target)
	}
}

func TestFeedMutationCoordinator_DeletePost_RollsBackWhenOutboxEnqueueFails(t *testing.T) {
	t.Parallel()

	transactor := &fakeFeedMutationTransactor{
		feedRepo:   &fakeFeedRepository{},
		outboxRepo: &fakeOutboxRepository{enqueueErr: errors.New("enqueue failed")},
	}
	coordinator := NewFeedMutationCoordinator(transactor)

	err := coordinator.DeletePost(context.Background(), DeletePostParams{
		FeedID:  "feed-1",
		FeedURI: types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Post: types.Post{
			Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
			Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
			Cid:       "cid-1",
			IndexedAt: time.Date(2026, 5, 11, 4, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Langs:     []string{"ja"},
		},
		MutationID: "mutation-2",
	})
	if err == nil {
		t.Fatal("expected DeletePost to fail when outbox enqueue fails")
	}
	if transactor.committed {
		t.Fatal("transaction committed on enqueue failure")
	}
	if !transactor.rolledBack {
		t.Fatal("transaction did not roll back on enqueue failure")
	}
}

func TestFeedMutationCoordinator_DeletePost_CommitsAndBuildsContracts(t *testing.T) {
	t.Parallel()

	transactor := &fakeFeedMutationTransactor{
		feedRepo:   &fakeFeedRepository{},
		outboxRepo: &fakeOutboxRepository{},
	}
	coordinator := NewFeedMutationCoordinator(transactor)
	post := types.Post{
		Feed:      types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Uri:       types.PostUri("at://did:plc:user1/app.bsky.feed.post/post1"),
		Cid:       "cid-1",
		IndexedAt: time.Date(2026, 5, 11, 4, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Langs:     []string{"ja"},
	}

	err := coordinator.DeletePost(context.Background(), DeletePostParams{
		FeedID:     "feed-1",
		FeedURI:    types.FeedUri("at://did:plc:test/app.bsky.feed.generator/sample"),
		Post:       post,
		MutationID: "mutation-2",
	})
	if err != nil {
		t.Fatalf("DeletePost() error = %v", err)
	}
	if !transactor.committed {
		t.Fatal("transaction was not committed on success")
	}
	if transactor.rolledBack {
		t.Fatal("transaction rolled back on success")
	}
	if transactor.feedRepo.deleteCalls != 1 {
		t.Fatalf("DeletePost calls = %d, want 1", transactor.feedRepo.deleteCalls)
	}
	if transactor.outboxRepo.enqueueCalls != 1 {
		t.Fatalf("Enqueue calls = %d, want 1", transactor.outboxRepo.enqueueCalls)
	}
	if transactor.feedRepo.lastDelete.PostURI != post.Uri {
		t.Fatalf("DeletePost PostURI = %s, want %s", transactor.feedRepo.lastDelete.PostURI, post.Uri)
	}
	if transactor.outboxRepo.lastEnqueue.Operation != "delete" {
		t.Fatalf("Operation = %s, want delete", transactor.outboxRepo.lastEnqueue.Operation)
	}
	if transactor.outboxRepo.lastEnqueue.OpKey != "mutation-2:delete:feed-1:at://did:plc:user1/app.bsky.feed.post/post1" {
		t.Fatalf("OpKey = %s", transactor.outboxRepo.lastEnqueue.OpKey)
	}
	if transactor.outboxRepo.lastEnqueue.Target != "gyoka" {
		t.Fatalf("Target = %s, want gyoka", transactor.outboxRepo.lastEnqueue.Target)
	}
}
