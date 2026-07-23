// Package agentmemory owns the human-in-the-loop use cases for agent-maintained
// memory. It translates workspace-scoped requests into durable memory actions;
// Delivery only validates/projections protocol values and never drives a
// domain persistence port directly.
package agentmemory

import (
	"context"
	"errors"
	"time"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// ErrUnavailable reports that no review store is wired for this runtime.
var ErrUnavailable = errors.New("agentmemory: review unavailable")

// RootResolver is the narrow workspace dependency this use case consumes.
// Its implementation belongs to the workspace application component; the
// agent-memory package does not learn filesystem or path-normalization details.
type RootResolver interface {
	ResolveRoot(cwd string) (string, error)
}

// Store is the review-oriented persistence port consumed by this coordinator.
// Extraction and search declare their own narrower consumer views.
type Store interface {
	List(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	SetStatus(ctx context.Context, id string, status domain.Status, now time.Time) error
	Update(ctx context.Context, id string, content *string, pinned *bool, now time.Time) (domain.Item, error)
	Delete(ctx context.Context, id string) error
	Add(ctx context.Context, scope domain.Scope, project, content string, now time.Time) (domain.Item, error)
}

// Config bundles the use case's driven ports. Store may be nil to represent a
// deliberately disabled capability. Roots is required only for project-scoped
// requests; a missing resolver reports an explicit unavailable error.
type Config struct {
	Store Store
	Roots RootResolver
	Now   func() time.Time
}

// Coordinator implements agent-memory review commands and queries.
type Coordinator struct {
	store Store
	roots RootResolver
	now   func() time.Time
}

// New builds the review coordinator. A nil store is a valid disabled runtime
// state so capability negotiation can remain truthful.
func New(cfg Config) *Coordinator {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{store: cfg.Store, roots: cfg.Roots, now: now}
}

// HasStore reports whether agent-memory review operations are available.
func (c *Coordinator) HasStore() bool { return c != nil && c.store != nil }

// List returns active and pending memory items for scope/cwd.
func (c *Coordinator) List(ctx context.Context, scope domain.Scope, cwd string) ([]domain.Item, error) {
	if !c.HasStore() {
		return nil, ErrUnavailable
	}
	project, err := c.project(scope, cwd)
	if err != nil {
		return nil, err
	}
	return c.store.List(ctx, scope, project)
}

// Review accepts or rejects an extracted proposal.
func (c *Coordinator) Review(ctx context.Context, id string, status domain.Status) error {
	if !c.HasStore() {
		return ErrUnavailable
	}
	return c.store.SetStatus(ctx, id, status, c.now())
}

// Update applies the content/pin patch as one use case and returns the saved
// item. The persistence port commits both requested fields atomically.
func (c *Coordinator) Update(ctx context.Context, id string, content *string, pinned *bool) (domain.Item, error) {
	if !c.HasStore() {
		return domain.Item{}, ErrUnavailable
	}
	return c.store.Update(ctx, id, content, pinned, c.now())
}

// Delete removes one memory item.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	if !c.HasStore() {
		return ErrUnavailable
	}
	return c.store.Delete(ctx, id)
}

// Add creates an immediately-active user-authored memory item.
func (c *Coordinator) Add(ctx context.Context, scope domain.Scope, cwd, content string) (domain.Item, error) {
	if !c.HasStore() {
		return domain.Item{}, ErrUnavailable
	}
	project, err := c.project(scope, cwd)
	if err != nil {
		return domain.Item{}, err
	}
	return c.store.Add(ctx, scope, project, content, c.now())
}

func (c *Coordinator) project(scope domain.Scope, cwd string) (string, error) {
	if scope == domain.ScopeUser {
		return "", nil
	}
	if c.roots == nil {
		return "", ErrUnavailable
	}
	return c.roots.ResolveRoot(cwd)
}
