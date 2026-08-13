// Package agentmemory owns review, curation, and search use cases for
// agent-maintained memory.
package agentmemory

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// ErrUnavailable reports that a requested agent-memory capability is not wired.
var ErrUnavailable = errors.New("agentmemory: unavailable")

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
	Review(ctx context.Context, id string, decision domain.ReviewDecision, now time.Time) error
	Update(ctx context.Context, id string, content *string, pinned *bool, now time.Time) (domain.Item, error)
	Delete(ctx context.Context, id string) error
	Add(ctx context.Context, scope domain.Scope, project, content string, now time.Time) (item domain.Item, created bool, err error)
}

// Config bundles the review use case's driven ports. Store may be nil to
// disable review. Roots is required only for project-scoped requests; a
// missing resolver reports an explicit unavailable error.
type Config struct {
	Store         Store
	Roots         RootResolver
	Now           func() time.Time
	Invalidations invalidation.Publish
}

// Coordinator implements agent-memory review commands and queries.
type Coordinator struct {
	store         Store
	roots         RootResolver
	now           func() time.Time
	invalidations invalidation.Publish
}

// New builds the coordinator. Nil stores are valid disabled states so
// capability negotiation and optional maintenance remain truthful.
func New(cfg Config) *Coordinator {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{store: cfg.Store, roots: cfg.Roots, now: now, invalidations: cfg.Invalidations}
}

// Available reports whether agent-memory review operations are wired.
func (c *Coordinator) Available() bool { return c != nil && c.store != nil }

// List returns active and pending memory items for scope/cwd.
func (c *Coordinator) List(ctx context.Context, scope domain.Scope, cwd string) ([]domain.Item, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	project, err := c.project(scope, cwd)
	if err != nil {
		return nil, err
	}
	return c.store.List(ctx, scope, project)
}

// Review accepts or rejects an extracted proposal.
func (c *Coordinator) Review(ctx context.Context, id string, decision domain.ReviewDecision) error {
	if !c.Available() {
		return ErrUnavailable
	}
	if _, err := decision.Result(); err != nil {
		return err
	}
	if err := c.store.Review(ctx, id, decision, c.now()); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return nil
}

// Update applies the content/pin patch as one use case and returns the saved
// item. The persistence port commits both requested fields atomically.
func (c *Coordinator) Update(ctx context.Context, id string, content *string, pinned *bool) (domain.Item, error) {
	if !c.Available() {
		return domain.Item{}, ErrUnavailable
	}
	item, err := c.store.Update(ctx, id, content, pinned, c.now())
	if err != nil {
		return domain.Item{}, err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return item, nil
}

// Delete removes one memory item.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	if !c.Available() {
		return ErrUnavailable
	}
	if err := c.store.Delete(ctx, id); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return nil
}

// Add creates an immediately-active user-authored memory item.
func (c *Coordinator) Add(ctx context.Context, scope domain.Scope, cwd, content string) (domain.Item, error) {
	if !c.Available() {
		return domain.Item{}, ErrUnavailable
	}
	project, err := c.project(scope, cwd)
	if err != nil {
		return domain.Item{}, err
	}
	item, created, err := c.store.Add(ctx, scope, project, content, c.now())
	if err != nil {
		return domain.Item{}, err
	}
	if created {
		c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return item, nil
}

func (c *Coordinator) project(scope domain.Scope, cwd string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if scope == domain.ScopeUser {
		return "", nil
	}
	if c.roots == nil {
		return "", ErrUnavailable
	}
	return c.roots.ResolveRoot(cwd)
}
