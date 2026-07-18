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

func (i *codebaseIndex) Available(ctx context.Context) (bool, error) {
	i.availableCtxErr = ctx.Err()
	if i.availability != nil {
		return i.availability(ctx)
	}
	return i.available, i.availabilityErr
}

func (i *codebaseIndex) Reindex(ctx context.Context, root string) error {
	i.reindexed <- codebaseReindexCall{root: root, err: ctx.Err()}
	return nil
}

func (i *codebaseIndex) Search(_ context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	i.searchRoot = root
	i.searchQuery = query
	i.searchLimit = limit
	return i.hits, nil
}

func (*codebaseIndex) EnsureIndexed(context.Context, string) error { return nil }

func (i *codebaseIndex) Status(_ context.Context, root string) (codebaseindex.Status, error) {
	i.statusRoot = root
	return i.status, nil
}

func TestStatusReturnsNoneWhenUnconfigured(t *testing.T) {
	c := New(nil)

	got, err := c.Status(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if got.State != codebaseindex.StateNone {
		t.Fatalf("state = %q, want none", got.State)
	}
}

func TestSearchUsesSearchPort(t *testing.T) {
	idx := &codebaseIndex{hits: []codebaseindex.Hit{{
		Path:  "runtime/codebase.go",
		Score: 0.95,
	}}}
	c := New(idx)

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
	c := New(idx)

	got, err := c.Status(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("Status err = %v", err)
	}
	if got.State != codebaseindex.StateReady || idx.statusRoot != "/repo" {
		t.Fatalf("status = %+v, root = %q", got, idx.statusRoot)
	}
}

func TestStartReindexRequiresAvailableIndex(t *testing.T) {
	c := New(nil)
	if err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex without index err = %v, want ErrNoEmbeddingModel", err)
	}

	c = New(&codebaseIndex{})
	if err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex unavailable err = %v, want ErrNoEmbeddingModel", err)
	}
}

func TestStartReindexPreservesAvailabilityFailure(t *testing.T) {
	wantErr := errors.New("provider store unavailable")
	c := New(&codebaseIndex{availabilityErr: wantErr})

	err := c.StartReindex(context.Background(), "/repo")
	if !errors.Is(err, wantErr) {
		t.Fatalf("StartReindex error = %v, want %v", err, wantErr)
	}
}

func TestStartReindexDetachesFromRequestCancel(t *testing.T) {
	idx := &codebaseIndex{available: true, reindexed: make(chan codebaseReindexCall, 1)}
	c := New(idx)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := c.StartReindex(ctx, "/repo"); err != nil {
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

func TestStartReindexRejectsClosedComponent(t *testing.T) {
	c := New(&codebaseIndex{available: true})
	c.Close()
	if err := c.StartReindex(context.Background(), "/repo"); !errors.Is(err, errClosed) {
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
	c := New(idx)
	result := make(chan error, 1)
	go func() {
		result <- c.StartReindex(context.Background(), "/repo")
	}()
	<-started

	c.Close()
	if err := <-result; !errors.Is(err, errClosed) {
		t.Fatalf("StartReindex error = %v, want %v", err, errClosed)
	}
}
