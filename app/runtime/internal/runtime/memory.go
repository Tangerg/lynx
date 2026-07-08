package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/knowledge"
)

type memoryList interface {
	List(ctx context.Context, cwd string) ([]knowledge.Entry, error)
}

type memoryRead interface {
	Get(ctx context.Context, scope knowledge.Scope, cwd string) (string, error)
}

type memoryWrite interface {
	Update(ctx context.Context, scope knowledge.Scope, cwd string, content string) error
}

// ErrMemoryUnavailable reports that this runtime was built without a knowledge store.
var ErrMemoryUnavailable = errors.New("runtime: memory unavailable")

// HasMemory reports whether this runtime is backed by a long-term knowledge
// store.
func (r *Runtime) HasMemory() bool {
	return r.memoryList != nil && r.memoryRead != nil && r.memoryWrite != nil
}

// ListMemoryEntries enumerates LYRA.md entries across scopes.
func (r *Runtime) ListMemoryEntries(ctx context.Context, cwd string) ([]knowledge.Entry, error) {
	if r.memoryList == nil {
		return nil, ErrMemoryUnavailable
	}
	return r.memoryList.List(ctx, cwd)
}

// Memory returns the LYRA.md content for one scope.
func (r *Runtime) Memory(ctx context.Context, scope knowledge.Scope, cwd string) (string, error) {
	if r.memoryRead == nil {
		return "", ErrMemoryUnavailable
	}
	return r.memoryRead.Get(ctx, scope, cwd)
}

// UpdateMemory overwrites the LYRA.md content for one scope.
func (r *Runtime) UpdateMemory(ctx context.Context, scope knowledge.Scope, cwd string, content string) error {
	if r.memoryWrite == nil {
		return ErrMemoryUnavailable
	}
	return r.memoryWrite.Update(ctx, scope, cwd, content)
}
