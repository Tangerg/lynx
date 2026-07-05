package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

// ErrMemoryUnavailable reports that this runtime was built without a knowledge store.
var ErrMemoryUnavailable = errors.New("runtime: memory unavailable")

// HasMemory reports whether this runtime is backed by a long-term knowledge
// store.
func (r *Runtime) HasMemory() bool {
	return r.knowledge != nil
}

// ListMemoryEntries enumerates LYRA.md entries across scopes.
func (r *Runtime) ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error) {
	if r.knowledge == nil {
		return nil, ErrMemoryUnavailable
	}
	return r.knowledge.List(ctx, cwd)
}

// GetMemory returns the LYRA.md content for one scope.
func (r *Runtime) GetMemory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error) {
	if r.knowledge == nil {
		return "", ErrMemoryUnavailable
	}
	return r.knowledge.Get(ctx, scope, cwd)
}

// UpdateMemory overwrites the LYRA.md content for one scope.
func (r *Runtime) UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error {
	if r.knowledge == nil {
		return ErrMemoryUnavailable
	}
	return r.knowledge.Update(ctx, scope, cwd, content)
}
