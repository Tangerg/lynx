package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

type codebaseIndexAvailability interface {
	Available(ctx context.Context) bool
}

type codebaseIndexSearch interface {
	Search(ctx context.Context, cwd, query string, topK int) ([]codebaseindex.Hit, error)
}

type codebaseIndexStatus interface {
	Status(ctx context.Context, cwd string) (codebaseindex.Status, error)
}

type codebaseIndexReindex interface {
	Reindex(ctx context.Context, cwd string) error
}

// HasCodebaseIndex reports whether this runtime has an index store wired.
func (r *Runtime) HasCodebaseIndex() bool {
	return r.codebaseAvailability != nil && r.codebaseSearch != nil &&
		r.codebaseStatus != nil && r.codebaseReindex != nil
}

// SearchCodebase returns semantic search hits for root, building the index when
// needed.
func (r *Runtime) SearchCodebase(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	if r.codebaseSearch == nil {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	return r.codebaseSearch.Search(ctx, root, query, limit)
}

// CodebaseIndexStatus returns root's current semantic-index state.
func (r *Runtime) CodebaseIndexStatus(ctx context.Context, root string) (codebaseindex.Status, error) {
	if r.codebaseStatus == nil {
		return codebaseindex.Status{State: codebaseindex.StateNone}, nil
	}
	return r.codebaseStatus.Status(ctx, root)
}

// StartCodebaseReindex starts a full rebuild that outlives the request context.
func (r *Runtime) StartCodebaseReindex(ctx context.Context, root string) error {
	if r.codebaseAvailability == nil || r.codebaseReindex == nil || !r.codebaseAvailability.Available(ctx) {
		return codebaseindex.ErrNoEmbeddingModel
	}
	go func() { _ = r.codebaseReindex.Reindex(context.WithoutCancel(ctx), root) }()
	return nil
}
