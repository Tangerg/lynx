// Package codebase owns the @codebase semantic-index use cases — search, status,
// and background reindex — over the domain [codebaseindex.Index]. A nil index
// means @codebase is disabled (no embedding store wired); the methods degrade to
// "unavailable" rather than erroring construction. The component task group owns
// the request-detached reindex, canceled + joined by [Coordinator.Close].
package codebase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/google/uuid"
)

// errClosed reports that a background reindex could not be scheduled because the
// component is shutting down.
var errClosed = errors.New("codebase: closed")

// Coordinator drives the @codebase semantic index.
type Coordinator struct {
	index codebaseindex.Index
	tasks taskgroup.Group

	activeMu sync.Mutex
	active   map[string]string // canonical root -> operation ID
}

// New returns a Coordinator over index (nil to disable @codebase).
func New(index codebaseindex.Index) *Coordinator {
	return &Coordinator{index: index, active: make(map[string]string)}
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
func (c *Coordinator) StartReindex(ctx context.Context, root string) (string, error) {
	if c.index == nil {
		return "", codebaseindex.ErrNoEmbeddingModel
	}
	taskCtx, release, ok := c.tasks.Attach(ctx)
	if !ok {
		return "", errClosed
	}
	available, err := c.index.Available(taskCtx)
	if err != nil {
		closed := taskCtx.Err() != nil
		release()
		if closed {
			return "", errClosed
		}
		return "", fmt.Errorf("codebase: check embedding availability: %w", err)
	}
	if !available {
		release()
		return "", codebaseindex.ErrNoEmbeddingModel
	}
	operationID, started := c.beginOperation(root)
	if !started {
		release()
		return operationID, nil
	}
	go func() {
		defer release()
		defer c.endOperation(root, operationID)
		// Reindex records every accepted task's terminal failure in Status; the
		// management surface is the asynchronous operation's result channel.
		_ = c.index.Reindex(taskCtx, root)
	}()
	return operationID, nil
}

// ActiveOperation returns the in-flight reindex operation for root, or an
// empty string when that root is idle.
func (c *Coordinator) ActiveOperation(root string) string {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	return c.active[root]
}

func (c *Coordinator) beginOperation(root string) (string, bool) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	if operationID := c.active[root]; operationID != "" {
		return operationID, false
	}
	operationID := "op_" + uuid.NewString()
	c.active[root] = operationID
	return operationID, true
}

func (c *Coordinator) endOperation(root, operationID string) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	if c.active[root] == operationID {
		delete(c.active, root)
	}
}
