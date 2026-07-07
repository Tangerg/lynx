package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

type codebaseIndex struct {
	codebaseindex.Index
	available bool
	reindexed chan codebaseReindexCall
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

func TestRuntimeStartCodebaseReindexRequiresAvailableIndex(t *testing.T) {
	rt := &Runtime{}
	if err := rt.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex without index err = %v, want ErrNoEmbeddingModel", err)
	}

	rt.codebaseIndex = &codebaseIndex{}
	if err := rt.StartCodebaseReindex(context.Background(), "/repo"); !errors.Is(err, codebaseindex.ErrNoEmbeddingModel) {
		t.Fatalf("start reindex unavailable err = %v, want ErrNoEmbeddingModel", err)
	}
}

func TestRuntimeStartCodebaseReindexDetachesFromRequestCancel(t *testing.T) {
	idx := &codebaseIndex{available: true, reindexed: make(chan codebaseReindexCall, 1)}
	rt := &Runtime{codebaseIndex: idx}
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
