package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

var errRuntimeClosed = errors.New("runtime: closed")

// HasCodebaseIndex reports whether this runtime has an index store wired.
func (r *Runtime) HasCodebaseIndex() bool {
	return r.codebase != nil
}

// SearchCodebase returns semantic search hits for root, building the index when
// needed.
func (r *Runtime) SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	if r.codebase == nil {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	return r.codebase.Search(ctx, root, query, limit)
}

// CodebaseIndexStatus returns root's current semantic-index state.
func (r *Runtime) CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error) {
	if r.codebase == nil {
		return codebaseindex.Status{State: codebaseindex.StateNone}, nil
	}
	return r.codebase.Status(ctx, root)
}

// StartCodebaseReindex starts a full rebuild that outlives the request context.
func (r *Runtime) StartCodebaseReindex(ctx context.Context, root string) error {
	if r.codebase == nil || !r.codebase.Available(ctx) {
		return codebaseindex.ErrNoEmbeddingModel
	}
	if !r.tasks.Start(ctx, func(ctx context.Context) {
		_ = r.codebase.Reindex(ctx, root)
	}) {
		return errRuntimeClosed
	}
	return nil
}
