// Package plan owns the session-scoped execution Plan maintained by the root
// Agent. A Plan is one ordered list of Steps; it is neither a second task
// system nor Plan-mode state. The aggregate owns replacement, validation, and
// monotonic revision semantics. Persistence and presentation remain outside.
package plan

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Status is one Step's execution state.
type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Valid reports whether s is a recognized Step status.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusInProgress, StatusCompleted:
		return true
	default:
		return false
	}
}

// Step is one ordered unit of work in a Plan.
type Step struct {
	Description string
	Status      Status
}

var (
	// ErrInvalid marks Plan state that violates a domain invariant.
	ErrInvalid = errors.New("plan: invalid plan")
	// ErrRevisionConflict marks a replacement based on a stale Plan revision.
	ErrRevisionConflict = errors.New("plan: revision conflict")
)

// ValidateSteps verifies an ordered Plan value. An empty value is valid and
// means clear the current Plan. At most one Step may be in progress so the
// execution focus remains unambiguous.
func ValidateSteps(steps []Step) error {
	inProgress := 0
	for index, step := range steps {
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("%w: step %d description is required", ErrInvalid, index)
		}
		if !step.Status.Valid() {
			return fmt.Errorf("%w: step %d has unknown status %q", ErrInvalid, index, step.Status)
		}
		if step.Status == StatusInProgress {
			inProgress++
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("%w: at most one step may be in_progress", ErrInvalid)
	}
	return nil
}

// Snapshot is the persistence representation of a Plan aggregate. It is a
// technical reconstruction boundary, not a mutation API.
type Snapshot struct {
	Steps     []Step
	Revision  uint64
	UpdatedAt time.Time
}

// State is the latest complete Plan for one session. Its zero value means no
// Plan replacement has been committed. All changes go through Replace so one
// replacement receives exactly one revision and update time.
type State struct {
	steps     []Step
	revision  uint64
	updatedAt time.Time
}

// Restore reconstructs a Plan aggregate from a trusted persistence boundary
// while rechecking every invariant.
func Restore(snapshot Snapshot) (State, error) {
	state := State{
		steps:     cloneSteps(snapshot.Steps),
		revision:  snapshot.Revision,
		updatedAt: canonicalTime(snapshot.UpdatedAt),
	}
	if err := state.Validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

// Replace returns the next complete Plan state. The caller supplies the clock
// value; the aggregate owns revision advancement and rejects time travel or
// revision overflow.
func (s State) Replace(steps []Step, updatedAt time.Time) (State, error) {
	if err := s.Validate(); err != nil {
		return State{}, fmt.Errorf("%w: current state: %v", ErrInvalid, err)
	}
	if err := ValidateSteps(steps); err != nil {
		return State{}, err
	}
	updatedAt = canonicalTime(updatedAt)
	if updatedAt.IsZero() {
		return State{}, fmt.Errorf("%w: replacement time is required", ErrInvalid)
	}
	if !s.updatedAt.IsZero() && updatedAt.Before(s.updatedAt) {
		return State{}, fmt.Errorf("%w: replacement time precedes current state", ErrInvalid)
	}
	if s.revision == math.MaxUint64 {
		return State{}, fmt.Errorf("%w: revision overflow", ErrInvalid)
	}
	return State{
		steps:     cloneSteps(steps),
		revision:  s.revision + 1,
		updatedAt: updatedAt,
	}, nil
}

// Validate verifies the aggregate's reconstruction and lifecycle invariants.
func (s State) Validate() error {
	if err := ValidateSteps(s.steps); err != nil {
		return err
	}
	if s.revision == 0 {
		if len(s.steps) != 0 || !s.updatedAt.IsZero() {
			return fmt.Errorf("%w: zero revision must be an unwritten Plan", ErrInvalid)
		}
		return nil
	}
	if s.updatedAt.IsZero() {
		return fmt.Errorf("%w: committed Plan has no update time", ErrInvalid)
	}
	return nil
}

// Steps returns a defensive copy of the ordered Plan value.
func (s State) Steps() []Step { return cloneSteps(s.steps) }

// Revision returns the monotonic replacement revision.
func (s State) Revision() uint64 { return s.revision }

// UpdatedAt returns when this replacement was committed.
func (s State) UpdatedAt() time.Time { return s.updatedAt }

// Snapshot returns a defensive persistence representation.
func (s State) Snapshot() Snapshot {
	return Snapshot{Steps: cloneSteps(s.steps), Revision: s.revision, UpdatedAt: s.updatedAt}
}

func cloneSteps(steps []Step) []Step {
	if len(steps) == 0 {
		return nil
	}
	return append([]Step(nil), steps...)
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.UTC()
}
