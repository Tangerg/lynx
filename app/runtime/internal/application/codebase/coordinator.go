// Package codebase owns the @codebase semantic-index use cases — search, status,
// and background reindex. A nil index
// means @codebase is disabled (no embedding store wired); the methods degrade to
// "unavailable" rather than erroring construction. The component task group owns
// the request-detached reindex, canceled + joined by [Coordinator.Close].
package codebase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/component/taskgroup"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
)

// errClosed reports that a background reindex could not be scheduled because the
// component is shutting down.
var errClosed = errors.New("codebase: closed")

// ErrRootResolverUnavailable reports a malformed composition root. Normal
// deployments always provide the workspace resolver; keeping the failure
// explicit prevents semantic-index calls from accepting an unscoped path.
var ErrRootResolverUnavailable = errors.New("codebase: workspace root resolver unavailable")

// RootResolver is the narrow workspace context dependency required by codebase
// use cases. The codebase component owns when its operations need a canonical
// project root; resolving paths remains the workspace component's concern.
type RootResolver interface {
	ResolveRoot(cwd string) (string, error)
}

// Index is the semantic-index capability these use cases consume.
type Index interface {
	Search(ctx context.Context, cwd, query string, topK int) ([]codebaseindex.Hit, error)
	Reindex(ctx context.Context, cwd string) error
	Status(ctx context.Context, cwd string) (codebaseindex.Status, error)
	Available(ctx context.Context) (bool, error)
}

// Status combines the index's durable status with the transient operation id
// that this coordinator owns. Delivery projects this neutral value onto wire.
type Status struct {
	Index       codebaseindex.Status
	OperationID string
}

// Coordinator drives the @codebase semantic index.
type Coordinator struct {
	index Index
	roots RootResolver
	tasks taskgroup.Group

	activeMu sync.Mutex
	active   map[string]string // canonical root -> operation ID
}

// New returns a Coordinator over index (nil to disable @codebase) scoped by
// roots. Root resolution belongs here so no Delivery handler must orchestrate
// workspace context before invoking a codebase use case.
func New(index Index, roots RootResolver) *Coordinator {
	return &Coordinator{index: index, roots: roots, active: make(map[string]string)}
}

// Close cancels + joins the background reindex tasks (§10.3).
func (c *Coordinator) Close() { c.tasks.Close() }

// Available reports whether semantic-index use cases are wired in this runtime.
func (c *Coordinator) Available() bool { return c != nil && c.index != nil }

// Search returns semantic search hits for cwd, building the index when needed.
func (c *Coordinator) Search(ctx context.Context, cwd, query string, limit int) ([]codebaseindex.Hit, error) {
	if c.index == nil {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	root, err := c.root(cwd)
	if err != nil {
		return nil, err
	}
	return c.index.Search(ctx, root, query, limit)
}

// Status returns cwd's current semantic-index state and any in-flight rebuild.
func (c *Coordinator) Status(ctx context.Context, cwd string) (Status, error) {
	root, err := c.root(cwd)
	if err != nil {
		return Status{}, err
	}
	if c.index == nil {
		return Status{Index: codebaseindex.Status{State: codebaseindex.StateNone}}, nil
	}
	status, err := c.index.Status(ctx, root)
	if err != nil {
		return Status{}, err
	}
	return Status{Index: status, OperationID: c.activeOperation(root)}, nil
}

// StartReindex starts a full rebuild for cwd that outlives the request context, owned by
// this component's task group (canceled + joined by Close).
func (c *Coordinator) StartReindex(ctx context.Context, cwd string) (string, error) {
	if c.index == nil {
		return "", codebaseindex.ErrNoEmbeddingModel
	}
	root, err := c.root(cwd)
	if err != nil {
		return "", err
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
		if err := c.index.Reindex(taskCtx, root); err != nil {
			// The asynchronous result channel is the status state; preserve the
			// detailed operational cause only on the request trace.
			trace.SpanFromContext(taskCtx).RecordError(err)
		}
	}()
	return operationID, nil
}

func (c *Coordinator) activeOperation(root string) string {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	return c.active[root]
}

func (c *Coordinator) root(cwd string) (string, error) {
	if c.roots == nil {
		return "", ErrRootResolverUnavailable
	}
	return c.roots.ResolveRoot(cwd)
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
