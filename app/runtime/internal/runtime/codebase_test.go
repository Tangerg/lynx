package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

type codebaseIndex struct {
	available bool
	reindexed chan codebaseReindexCall
	hits      []codebaseindex.Hit
	status    codebaseindex.Status

	searchRoot  string
	searchQuery string
	searchLimit int
	statusRoot  string
}

type codebaseReindexCall struct {
	root string
	err  error
}

func (i *codebaseIndex) Available(context.Context) bool {
	return i.available
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

func (i *codebaseIndex) Status(_ context.Context, root string) (codebaseindex.Status, error) {
	i.statusRoot = root
	return i.status, nil
}

func TestRuntimeCodebaseStatusReturnsNoneWhenUnconfigured(t *testing.T) {
	rt := &Runtime{}

	got, err := rt.CodebaseIndexStatus(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("codebase status: %v", err)
	}
	if got.State != codebaseindex.StateNone {
		t.Fatalf("state = %q, want none", got.State)
	}
}

func TestRuntimeCodebaseSearchUsesSearchPort(t *testing.T) {
	idx := &codebaseIndex{hits: []codebaseindex.Hit{{
		Path:  "runtime/codebase.go",
		Score: 0.95,
	}}}
	rt := &Runtime{codebaseSearch: idx}

	got, err := rt.SearchCodebase(context.Background(), "/repo", "runtime facade", 4)
	if err != nil {
		t.Fatalf("SearchCodebase err = %v", err)
	}
	if len(got) != 1 || got[0].Path != "runtime/codebase.go" {
		t.Fatalf("SearchCodebase = %+v", got)
	}
	if idx.searchRoot != "/repo" || idx.searchQuery != "runtime facade" || idx.searchLimit != 4 {
		t.Fatalf("search root=%q query=%q limit=%d", idx.searchRoot, idx.searchQuery, idx.searchLimit)
	}
}

func TestRuntimeCodebaseStatusUsesStatusPort(t *testing.T) {
	idx := &codebaseIndex{status: codebaseindex.Status{
		State: codebaseindex.StateReady,
	}}
	rt := &Runtime{codebaseStatus: idx}

	got, err := rt.CodebaseIndexStatus(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("CodebaseIndexStatus err = %v", err)
	}
	if got.State != codebaseindex.StateReady || idx.statusRoot != "/repo" {
		t.Fatalf("status = %+v, root = %q", got, idx.statusRoot)
	}
}

func TestRuntimeStartCodebaseReindexRequiresAvailableIndex(t *testing.T) {
	rt := &Runtime{}
	if err := rt.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex without index err = %v, want ErrNoEmbeddingModel", err)
	}

	idx := &codebaseIndex{}
	rt.codebaseAvailability = idx
	rt.codebaseReindex = idx
	if err := rt.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex unavailable err = %v, want ErrNoEmbeddingModel", err)
	}
}

func TestRuntimeStartCodebaseReindexDetachesFromRequestCancel(t *testing.T) {
	idx := &codebaseIndex{available: true, reindexed: make(chan codebaseReindexCall, 1)}
	rt := &Runtime{
		codebaseAvailability: idx,
		codebaseReindex:      idx,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := rt.StartCodebaseReindex(ctx, "/repo"); err != nil {
		t.Fatalf("start reindex: %v", err)
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
