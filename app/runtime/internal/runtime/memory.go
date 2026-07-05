package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// HasMemory reports whether this runtime is backed by a long-term knowledge
// store.
func (r *Runtime) HasMemory() bool {
	return r.knowledge != nil
}

// ListMemoryEntries enumerates LYRA.md entries across scopes.
func (r *Runtime) ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error) {
	return r.knowledge.List(ctx, cwd)
}

// GetMemory returns the LYRA.md content for one scope.
func (r *Runtime) GetMemory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error) {
	return r.knowledge.Get(ctx, scope, cwd)
}

// UpdateMemory overwrites the LYRA.md content for one scope.
func (r *Runtime) UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error {
	return r.knowledge.Update(ctx, scope, cwd, content)
}
