// Package plans owns the application use cases for reading and replacing a
// session's execution Plan. Domain state decides each replacement; persistence
// only compares the expected revision and saves that decided state.
package plans

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
)

// Store is the use case's consumer-owned persistence port.
type Store interface {
	State(ctx context.Context, sessionID string) (plan.State, error)
	Save(ctx context.Context, sessionID string, expectedRevision uint64, replacement plan.State) error
}

// Clock supplies the commit time for a Plan replacement.
type Clock func() time.Time

// Coordinator executes Plan use cases over one canonical store.
type Coordinator struct {
	store         Store
	now           Clock
	invalidations invalidation.Publish
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Store         Store
	Now           Clock
	Invalidations invalidation.Publish
}

// New returns a Plan Coordinator. A nil Store means the optional capability is
// unavailable; callers should omit its tools and application wiring.
func New(deps Dependencies) *Coordinator {
	if deps.Store == nil {
		return nil
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Coordinator{store: deps.Store, now: deps.Now, invalidations: deps.Invalidations}
}

// State returns the canonical Plan aggregate for one session.
func (c *Coordinator) State(ctx context.Context, sessionID string) (plan.State, error) {
	if c == nil || c.store == nil {
		return plan.State{}, errors.New("plans: store is unavailable")
	}
	if strings.TrimSpace(sessionID) == "" || sessionID != strings.TrimSpace(sessionID) {
		return plan.State{}, errors.New("plans: session ID is required and must not contain surrounding whitespace")
	}
	state, err := c.store.State(ctx, sessionID)
	if err != nil {
		return plan.State{}, err
	}
	if err := state.Validate(); err != nil {
		return plan.State{}, fmt.Errorf("plans: read invalid state: %w", err)
	}
	return state, nil
}

// Replace computes and commits one complete replacement using optimistic
// concurrency. An empty steps slice clears the Plan under a new revision.
func (c *Coordinator) Replace(ctx context.Context, sessionID string, steps []plan.Step) (plan.State, error) {
	replacement, err := c.PrepareReplacement(ctx, sessionID, steps)
	if err != nil {
		return plan.State{}, err
	}
	if err := c.store.Save(ctx, sessionID, replacement.ExpectedRevision(), replacement.State()); err != nil {
		return plan.State{}, err
	}
	c.invalidations.Notify(invalidation.InSession(invalidation.PlanState, sessionID))
	return replacement.State(), nil
}

// PrepareReplacement decides a replacement without committing it. Cross-
// aggregate use cases use this to include the exact Plan transition in their
// own atomic write set.
func (c *Coordinator) PrepareReplacement(ctx context.Context, sessionID string, steps []plan.Step) (Replacement, error) {
	current, err := c.State(ctx, sessionID)
	if err != nil {
		return Replacement{}, err
	}
	return c.replace(current, steps)
}

// PrepareInitial decides the first Plan state for a not-yet-created session.
// It is used when a cross-aggregate write set assigns the session identity.
func (c *Coordinator) PrepareInitial(steps []plan.Step) (Replacement, error) {
	if c == nil {
		return Replacement{}, errors.New("plans: coordinator is unavailable")
	}
	return c.replace(plan.State{}, steps)
}

func (c *Coordinator) replace(current plan.State, steps []plan.Step) (Replacement, error) {
	next, err := current.Replace(steps, c.now())
	if err != nil {
		return Replacement{}, err
	}
	return newReplacement(current.Revision(), next)
}

// Replacement is an immutable, application-decided Plan state transition.
// Persistence implementations may execute it but may not enrich or reinterpret it.
type Replacement struct {
	expectedRevision uint64
	state            plan.State
}

func newReplacement(expectedRevision uint64, state plan.State) (Replacement, error) {
	replacement := Replacement{expectedRevision: expectedRevision, state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedRevision returns the state revision this replacement was based on.
func (r Replacement) ExpectedRevision() uint64 { return r.expectedRevision }

// State returns the already-decided replacement state.
func (r Replacement) State() plan.State {
	return r.state
}

// Validate verifies that the replacement advances its expected revision once.
func (r Replacement) Validate() error {
	if err := r.state.Validate(); err != nil {
		return fmt.Errorf("plans: invalid replacement state: %w", err)
	}
	if r.expectedRevision == ^uint64(0) || r.state.Revision() != r.expectedRevision+1 {
		return fmt.Errorf("plans: replacement revision %d does not follow expected revision %d", r.state.Revision(), r.expectedRevision)
	}
	return nil
}
