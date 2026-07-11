package capabilities

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

func (i *codebaseIndex) Available(context.Context) bool { return i.available }

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

func TestCodebaseStatusReturnsNoneWhenUnconfigured(t *testing.T) {
	c := New(Config{})

	got, err := c.CodebaseIndexStatus(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("codebase status: %v", err)
	}
	if got.State != codebaseindex.StateNone {
		t.Fatalf("state = %q, want none", got.State)
	}
}

func TestCodebaseSearchUsesSearchPort(t *testing.T) {
	idx := &codebaseIndex{hits: []codebaseindex.Hit{{
		Path:  "runtime/codebase.go",
		Score: 0.95,
	}}}
	c := New(Config{Codebase: idx})

	got, err := c.SearchCodebase(context.Background(), "/repo", "runtime facade", 4)
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

func TestCodebaseStatusUsesStatusPort(t *testing.T) {
	idx := &codebaseIndex{status: codebaseindex.Status{State: codebaseindex.StateReady}}
	c := New(Config{Codebase: idx})

	got, err := c.CodebaseIndexStatus(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("CodebaseIndexStatus err = %v", err)
	}
	if got.State != codebaseindex.StateReady || idx.statusRoot != "/repo" {
		t.Fatalf("status = %+v, root = %q", got, idx.statusRoot)
	}
}

func TestStartCodebaseReindexRequiresAvailableIndex(t *testing.T) {
	c := New(Config{})
	if err := c.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex without index err = %v, want ErrNoEmbeddingModel", err)
	}

	c = New(Config{Codebase: &codebaseIndex{}})
	if err := c.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex unavailable err = %v, want ErrNoEmbeddingModel", err)
	}
}

func TestStartCodebaseReindexDetachesFromRequestCancel(t *testing.T) {
	idx := &codebaseIndex{available: true, reindexed: make(chan codebaseReindexCall, 1)}
	c := New(Config{Codebase: idx})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := c.StartCodebaseReindex(ctx, "/repo"); err != nil {
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

func TestStartCodebaseReindexRejectsClosedComponent(t *testing.T) {
	c := New(Config{Codebase: &codebaseIndex{available: true}})
	c.Close()
	if err := c.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, errClosed) {
		t.Fatalf("StartCodebaseReindex error = %v, want %v", err, errClosed)
	}
}
