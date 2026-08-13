package codebase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
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
	searchErr       error
	status          codebaseindex.Status
	searchIndexing  bool

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
	return New(index, staticRootResolver{}, nil)
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

func (i *codebaseIndex) Search(_ context.Context, root, query string, limit int, indexing func()) ([]codebaseindex.Hit, error) {
	i.searchRoot = root
	i.searchQuery = query
	i.searchLimit = limit
	if i.searchIndexing {
		i.status.State = codebaseindex.StateIndexing
		indexing()
		if i.searchErr == nil {
			i.status.State = codebaseindex.StateReady
		} else {
			i.status.State = codebaseindex.StateError
		}
	}
	return i.hits, i.searchErr
}

func (i *codebaseIndex) Status(_ context.Context, root string) (codebaseindex.Status, error) {
	i.statusRoot = root
	return i.status, nil
}

// TestUnassembledIndexIsUnavailableNotEmpty pins the distinction API.md §7.10
// draws: a runtime that never assembled a semantic index says so
// (capability_not_negotiated), rather than reporting an empty index — which would
// invite the client to offer a "build it" button for a capability that is absent.
func TestUnassembledIndexIsUnavailableNotEmpty(t *testing.T) {
	c := newCoordinator(nil)

	if _, err := c.Status(context.Background(), "/repo"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("status err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Search(context.Background(), "/repo", "q", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("search err = %v, want ErrUnavailable", err)
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

func TestSearchPublishesReconciliationStateTransitions(t *testing.T) {
	idx := &codebaseIndex{searchIndexing: true}
	var c *Coordinator
	var states []codebaseindex.State
	c = New(idx, staticRootResolver{}, func(notice invalidation.Notice) {
		if notice.Resource != invalidation.Codebase {
			t.Fatalf("notice = %+v, want Codebase", notice)
		}
		status, err := c.Status(t.Context(), "/repo")
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, status.Index.State)
	})

	if _, err := c.Search(t.Context(), "/repo", "runtime facade", 4); err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0] != codebaseindex.StateIndexing || states[1] != codebaseindex.StateReady {
		t.Fatalf("published states = %v, want [indexing ready]", states)
	}

	idx.searchIndexing = false
	if _, err := c.Search(t.Context(), "/repo", "cached", 4); err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("cached search published %d states, want 2", len(states))
	}
}

func TestSearchPublishesFailedReconciliation(t *testing.T) {
	wantErr := errors.New("embedding failed")
	idx := &codebaseIndex{searchIndexing: true, searchErr: wantErr}
	var states []codebaseindex.State
	var c *Coordinator
	c = New(idx, staticRootResolver{}, func(invalidation.Notice) {
		status, err := c.Status(t.Context(), "/repo")
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, status.Index.State)
	})

	if _, err := c.Search(t.Context(), "/repo", "runtime facade", 4); !errors.Is(err, wantErr) {
		t.Fatalf("Search error = %v, want %v", err, wantErr)
	}
	if len(states) != 2 || states[0] != codebaseindex.StateIndexing || states[1] != codebaseindex.StateError {
		t.Fatalf("published states = %v, want [indexing error]", states)
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
	if _, err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("start reindex without index err = %v, want ErrUnavailable", err)
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
	requireCoordinatorShutdown(t, c)
	status, statusErr = c.Status(context.Background(), "/repo")
	if statusErr != nil || status.OperationID != "" {
		t.Fatalf("status after completion = %+v, err=%v, want no active operation", status, statusErr)
	}
}

func TestStartReindexPublishesStartBeforeFinish(t *testing.T) {
	release := make(chan struct{})
	idx := &codebaseIndex{
		available: true,
		reindex: func(_ context.Context, _ string) error {
			<-release
			return nil
		},
	}
	notices := make(chan invalidation.Notice, 2)
	c := New(idx, staticRootResolver{}, func(notice invalidation.Notice) { notices <- notice })

	operationID, err := c.StartReindex(t.Context(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case notice := <-notices:
		if notice.Resource != invalidation.Codebase {
			t.Fatalf("start notice = %+v", notice)
		}
		status, statusErr := c.Status(t.Context(), "/repo")
		if statusErr != nil || status.OperationID != operationID {
			t.Fatalf("status at start = %+v, %v; want operation %q", status, statusErr, operationID)
		}
	default:
		t.Fatal("start notice was not published synchronously")
	}

	close(release)
	requireCoordinatorShutdown(t, c)
	select {
	case notice := <-notices:
		if notice.Resource != invalidation.Codebase {
			t.Fatalf("finish notice = %+v", notice)
		}
	case <-time.After(time.Second):
		t.Fatal("finish notice was not published")
	}
}

func TestStartReindexRejectsClosedComponent(t *testing.T) {
	c := newCoordinator(&codebaseIndex{available: true})
	requireCoordinatorShutdown(t, c)
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

	requireCoordinatorShutdown(t, c)
	if err := <-result; !errors.Is(err, errClosed) {
		t.Fatalf("StartReindex error = %v, want %v", err, errClosed)
	}
}
