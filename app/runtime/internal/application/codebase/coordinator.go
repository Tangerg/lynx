// Package codebase owns the @codebase semantic-index use cases — search, status,
// and background reindex — over the domain [codebaseindex.Index]. A nil index
// means @codebase is disabled (no embedding store wired); the methods degrade to
// "unavailable" rather than erroring construction. The component task group owns
// the request-detached reindex, canceled + joined by [Coordinator.Close].
package codebase

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// errClosed reports that a background reindex could not be scheduled because the
// component is shutting down.
var errClosed = errors.New("codebase: closed")

// Coordinator drives the @codebase semantic index.
type Coordinator struct {
	index codebaseindex.Index
	tasks taskgroup.Group
}

// New returns a Coordinator over index (nil to disable @codebase).
func New(index codebaseindex.Index) *Coordinator {
	return &Coordinator{index: index}
}

// Close cancels + joins the background reindex tasks (§10.3).
func (c *Coordinator) Close() { c.tasks.Close() }

// HasIndex reports whether this runtime has an index store wired.
func (c *Coordinator) HasIndex() bool { return c.index != nil }

// Search returns semantic search hits for root, building the index when needed.
func (c *Coordinator) Search(ctx context.Context, root, query string, limit int) ([]codebaseindex.Hit, error) {
	if c.index == nil {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	return c.index.Search(ctx, root, query, limit)
}

// Status returns root's current semantic-index state.
func (c *Coordinator) Status(ctx context.Context, root string) (codebaseindex.Status, error) {
	if c.index == nil {
		return codebaseindex.Status{State: codebaseindex.StateNone}, nil
	}
	return c.index.Status(ctx, root)
}

// StartReindex starts a full rebuild that outlives the request context, owned by
// this component's task group (canceled + joined by Close).
func (c *Coordinator) StartReindex(ctx context.Context, root string) error {
	if c.index == nil || !c.index.Available(ctx) {
		return codebaseindex.ErrNoEmbeddingModel
	}
	if !c.tasks.Start(ctx, func(ctx context.Context) {
		_ = c.index.Reindex(ctx, root)
	}) {
		return errClosed
	}
	return nil
}
