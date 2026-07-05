package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// HasCodebaseIndex reports whether this runtime has an index store wired.
func (r *Runtime) HasCodebaseIndex() bool {
	return r.codebaseIndex != nil
}

// SearchCodebase returns semantic search hits for root, building the index when
// needed.
func (r *Runtime) SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	return r.codebaseIndex.Search(ctx, root, query, limit)
}

// CodebaseIndexStatus returns root's current semantic-index state.
func (r *Runtime) CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error) {
	if r.codebaseIndex == nil {
		return codebaseindex.Status{State: codebaseindex.StateNone}, nil
	}
	return r.codebaseIndex.Status(ctx, root)
}

// StartCodebaseReindex starts a full rebuild that outlives the request context.
func (r *Runtime) StartCodebaseReindex(ctx context.Context, root string) error {
	if r.codebaseIndex == nil || !r.codebaseIndex.Available(ctx) {
		return codebaseindex.ErrNoEmbeddingModel
	}
	go func() { _ = r.codebaseIndex.Reindex(context.WithoutCancel(ctx), root) }()
	return nil
}
