package codebase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

type codebaseIndex struct {
	available       bool
	availabilityErr error
	availableCtxErr error
	availability    func(context.Context) (bool, error)
	reindex         func(context.Context, string) error
	reindexed       chan codebaseReindexCall
	hits            []codebaseindex.Hit
	status          codebaseindex.Status

	searchRoot  string
	searchQuery string
	searchLimit int
	statusRoot  string
}

type codebaseReindexCall struct {
	root string
	err  error
}

type staticRootResolver struct{}

func (staticRootResolver) ResolveRoot(cwd string) (string, error) { return cwd, nil }

func newCoordinator(index Index) *Coordinator {
	return New(index, staticRootResolver{})
}

func (i *codebaseIndex) Available(ctx context.Context) (bool, error) {
	i.availableCtxErr = ctx.Err()
	if i.availability != nil {
		return i.availability(ctx)
	}
	return i.available, i.availabilityErr
}

func (i *codebaseIndex) Reindex(ctx context.Context, root string) error {
	if i.reindex != nil {
		return i.reindex(ctx, root)
	}
	i.reindexed <- codebaseReindexCall{root: root, err: ctx.Err()}
	return nil
}

func (i *codebaseIndex) Search(_ context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	i.searchRoot = root
	i.searchQuery = query
	i.searchLimit = limit
	return i.hits, nil
}

func (i *codebaseIndex) Status(_ context.Context, root string) (codebaseindex.Status, error) {
	i.statusRoot = root
	return i.status, nil
}

func TestStatusReturnsNoneWhenUnconfigured(t *testing.T) {
	c := newCoordinator(nil)

	got, err := c.Status(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if got.Index.State != codebaseindex.StateNone {
		t.Fatalf("state = %q, want none", got.Index.State)
	}
}

func TestSearchUsesSearchPort(t *testing.T) {
	idx := &codebaseIndex{hits: []codebaseindex.Hit{{
		Path:  "runtime/codebase.go",
		Score: 0.95,
	}}}
	c := newCoordinator(idx)

	got, err := c.Search(context.Background(), "/repo", "runtime facade", 4)
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}
	if len(got) != 1 || got[0].Path != "runtime/codebase.go" {
		t.Fatalf("Search = %+v", got)
	}
	if idx.searchRoot != "/repo" || idx.searchQuery != "runtime facade" || idx.searchLimit != 4 {
		t.Fatalf("search root=%q query=%q limit=%d", idx.searchRoot, idx.searchQuery, idx.searchLimit)
	}
}

func TestStatusUsesStatusPort(t *testing.T) {
	idx := &codebaseIndex{status: codebaseindex.Status{State: codebaseindex.StateReady}}
	c := newCoordinator(idx)

	got, err := c.Status(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("Status err = %v", err)
	}
	if got.Index.State != codebaseindex.StateReady || idx.statusRoot != "/repo" {
		t.Fatalf("status = %+v, root = %q", got, idx.statusRoot)
	}
}

func TestStartReindexRequiresAvailableIndex(t *testing.T) {
	c := newCoordinator(nil)
	if _, err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex without index err = %v, want ErrNoEmbeddingModel", err)
	}

	c = newCoordinator(&codebaseIndex{})
	if _, err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex unavailable err = %v, want ErrNoEmbeddingModel", err)
	}
}

func TestStartReindexPreservesAvailabilityFailure(t *testing.T) {
	wantErr := errors.New("provider store unavailable")
	c := newCoordinator(&codebaseIndex{availabilityErr: wantErr})

	_, err := c.StartReindex(context.Background(), "/repo")
	if !errors.Is(err, wantErr) {
		t.Fatalf("StartReindex error = %v, want %v", err, wantErr)
	}
}

func TestStartReindexDetachesFromRequestCancel(t *testing.T) {
	idx := &codebaseIndex{available: true, reindexed: make(chan codebaseReindexCall, 1)}
	c := newCoordinator(idx)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.StartReindex(ctx, "/repo"); err != nil {
		t.Fatalf("start reindex: %v", err)
	}
	if idx.availableCtxErr != nil {
		t.Fatalf("availability ctx err = %v, want nil", idx.availableCtxErr)
	}

	select {
	case got := <-idx.reindexed:
		if got.root != "/repo" {
			t.Fatalf("reindex root = %q, want /repo", got.root)
		}
		if got.err != nil {
			t.Fatalf("reindex ctx err = %v, want nil", got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("reindex did not start")
	}
}

func TestStartReindexCoalescesOperationsByRoot(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	idx := &codebaseIndex{
		available: true,
		reindex: func(_ context.Context, _ string) error {
			close(started)
			<-finish
			return nil
		},
	}
	c := newCoordinator(idx)

	first, err := c.StartReindex(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("start first reindex: %v", err)
	}
	<-started
	second, err := c.StartReindex(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("start coalesced reindex: %v", err)
	}
	status, statusErr := c.Status(context.Background(), "/repo")
	if statusErr != nil || second != first || status.OperationID != first {
		t.Fatalf("coalesced operation = %q, status = %+v, err=%v, want %q", second, status, statusErr, first)
	}

	close(finish)
	c.Close()
	status, statusErr = c.Status(context.Background(), "/repo")
	if statusErr != nil || status.OperationID != "" {
		t.Fatalf("status after completion = %+v, err=%v, want no active operation", status, statusErr)
	}
}

func TestStartReindexRejectsClosedComponent(t *testing.T) {
	c := newCoordinator(&codebaseIndex{available: true})
	c.Close()
	if _, err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, errClosed) {
		t.Fatalf("StartReindex error = %v, want %v", err, errClosed)
	}
}

func TestCloseCancelsAndJoinsReindexAvailabilityCheck(t *testing.T) {
	started := make(chan struct{})
	idx := &codebaseIndex{availability: func(ctx context.Context) (bool, error) {
		close(started)
		<-ctx.Done()
		return false, ctx.Err()
	}}
	c := newCoordinator(idx)
	result := make(chan error, 1)
	go func() {
		_, err := c.StartReindex(context.Background(), "/repo")
		result <- err
	}()
	<-started

	c.Close()
	if err := <-result; !errors.Is(err, errClosed) {
		t.Fatalf("StartReindex error = %v, want %v", err, errClosed)
	}
}
