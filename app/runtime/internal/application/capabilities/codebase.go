package capabilities

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// HasCodebaseIndex reports whether this runtime has an index store wired.
func (c *Coordinator) HasCodebaseIndex() bool {
	return c.codebase != nil
}

// SearchCodebase returns semantic search hits for root, building the index when
// needed.
func (c *Coordinator) SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	if c.codebase == nil {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	return c.codebase.Search(ctx, root, query, limit)
}

// CodebaseIndexStatus returns root's current semantic-index state.
func (c *Coordinator) CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error) {
	if c.codebase == nil {
		return codebaseindex.Status{State: codebaseindex.StateNone}, nil
	}
	return c.codebase.Status(ctx, root)
}

// StartCodebaseReindex starts a full rebuild that outlives the request context,
// owned by this component's task group (canceled + joined by Close).
func (c *Coordinator) StartCodebaseReindex(ctx context.Context, root string) error {
	if c.codebase == nil || !c.codebase.Available(ctx) {
		return codebaseindex.ErrNoEmbeddingModel
	}
	if !c.tasks.Start(ctx, func(ctx context.Context) {
		_ = c.codebase.Reindex(ctx, root)
	}) {
		return errClosed
	}
	return nil
}
