package agentmemory

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// CurationStore is the persistence port for automatic memory maintenance. The
// append-only ledger and embeddings are internal implementation state; only a
// successful Reconcile changes the public agent-memory generation.
type CurationStore interface {
	AppendLedger(ctx context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error)
	PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]domain.LedgerFact, error)
	State(ctx context.Context, project string) (domain.State, error)
	Reconcile(ctx context.Context, project string, expectedWatermark, through int64, contents []string, now time.Time) (bool, error)
	Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	UnembeddedItems(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	SetEmbeddings(ctx context.Context, vectors map[string][]float32) error
}

// CurationConfig bundles the automatic-maintenance ports.
type CurationConfig struct {
	Store         CurationStore
	Invalidations invalidation.Publish
}

// Curation owns the append-ledger and generation-publication use cases. It is
// deliberately separate from Coordinator so review consumers cannot invoke
// background maintenance operations.
type Curation struct {
	store         CurationStore
	invalidations invalidation.Publish
}

// NewCuration builds the automatic memory-maintenance use case.
func NewCuration(cfg CurationConfig) *Curation {
	return &Curation{store: cfg.Store, invalidations: cfg.Invalidations}
}

// Available reports whether automatic memory maintenance is wired.
func (c *Curation) Available() bool { return c != nil && c.store != nil }

// AppendLedger records newly extracted facts. The ledger is not a public read
// model, so appending it does not invalidate agentMemory.list.
func (c *Curation) AppendLedger(ctx context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	return c.store.AppendLedger(ctx, batch)
}

// PendingLedger returns facts not yet incorporated into the curated generation.
func (c *Curation) PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]domain.LedgerFact, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	return c.store.PendingLedger(ctx, project, watermark, limit)
}

// State returns the current curation watermark.
func (c *Curation) State(ctx context.Context, project string) (domain.State, error) {
	if !c.Available() {
		return domain.State{}, ErrUnavailable
	}
	return c.store.State(ctx, project)
}

// PublishGeneration publishes one compare-and-swap-protected curated
// generation and invalidates the public projection only for the winning fold.
func (c *Curation) PublishGeneration(ctx context.Context, project string, expectedWatermark, through int64, contents []string, now time.Time) (bool, error) {
	if !c.Available() {
		return false, ErrUnavailable
	}
	published, err := c.store.Reconcile(ctx, project, expectedWatermark, through, contents, now)
	if err != nil {
		return false, err
	}
	if published {
		c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return published, nil
}

// Items returns the stored generation used as input to the next fold.
func (c *Curation) Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	return c.store.Items(ctx, scope, project)
}

// UnembeddedItems returns curated items whose search vectors need backfilling.
func (c *Curation) UnembeddedItems(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	return c.store.UnembeddedItems(ctx, scope, project)
}

// SetEmbeddings updates search-only vector state and therefore does not
// invalidate the human-readable agent-memory projection.
func (c *Curation) SetEmbeddings(ctx context.Context, vectors map[string][]float32) error {
	if !c.Available() {
		return ErrUnavailable
	}
	return c.store.SetEmbeddings(ctx, vectors)
}
