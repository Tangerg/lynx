// Package plan owns the Session-scoped ordered execution Plan. It is not a
// generic state bag and it does not own the separate read-only Plan-mode policy.
package plan

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrInvalid         = errors.New("plan: invalid state")
	ErrNotFound        = errors.New("plan: session not found")
	ErrVersionConflict = errors.New("plan: version conflict")
)

type Status string

const (
	Pending    Status = "pending"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
)

func (status Status) Valid() bool {
	switch status {
	case Pending, InProgress, Completed:
		return true
	default:
		return false
	}
}

type Step struct {
	Description string
	Status      Status
}

type State struct {
	sessionID string
	steps     []Step
	revision  uint64
	updatedAt time.Time
}

// Boundary is the Plan value captured when a root Run terminalizes. Presence
// is meaningful: a recorded empty Boundary is different from an imported Run
// whose historical Plan is unknowable.
type Boundary struct{ steps []Step }

func NewBoundary(steps []Step) (Boundary, error) {
	normalized := cloneSteps(steps)
	for index := range normalized {
		normalized[index].Description = strings.TrimSpace(normalized[index].Description)
	}
	if err := validateSteps(normalized); err != nil {
		return Boundary{}, err
	}
	return Boundary{steps: normalized}, nil
}

func (value Boundary) Steps() []Step { return cloneSteps(value.steps) }

func New(sessionID string) (State, error) {
	value := State{sessionID: strings.TrimSpace(sessionID), steps: []Step{}}
	if err := value.Validate(); err != nil {
		return State{}, err
	}
	return value, nil
}

type Restore struct {
	SessionID string
	Steps     []Step
	Revision  uint64
	UpdatedAt time.Time
}

func Rehydrate(snapshot Restore) (State, error) {
	value := State{
		sessionID: strings.TrimSpace(snapshot.SessionID), steps: cloneSteps(snapshot.Steps),
		revision: snapshot.Revision, updatedAt: snapshot.UpdatedAt.UTC(),
	}
	if err := value.Validate(); err != nil {
		return State{}, err
	}
	return value, nil
}

func (value State) Replace(steps []Step, now time.Time) (State, error) {
	if value.revision == math.MaxUint64 {
		return State{}, fmt.Errorf("%w: revision overflow", ErrInvalid)
	}
	normalized := cloneSteps(steps)
	for index := range normalized {
		normalized[index].Description = strings.TrimSpace(normalized[index].Description)
	}
	next := State{sessionID: value.sessionID, steps: normalized, revision: value.revision + 1, updatedAt: now.UTC()}
	if !value.updatedAt.IsZero() && next.updatedAt.Before(value.updatedAt) {
		return State{}, fmt.Errorf("%w: replacement time moved backwards", ErrInvalid)
	}
	if err := next.Validate(); err != nil {
		return State{}, err
	}
	return next, nil
}

func (value State) Validate() error {
	if value.sessionID == "" {
		return fmt.Errorf("%w: session identity is required", ErrInvalid)
	}
	if value.revision == 0 {
		if len(value.steps) != 0 || !value.updatedAt.IsZero() {
			return fmt.Errorf("%w: unwritten Plan must be empty", ErrInvalid)
		}
		return nil
	}
	if value.updatedAt.IsZero() {
		return fmt.Errorf("%w: committed Plan needs an update time", ErrInvalid)
	}
	return validateSteps(value.steps)
}

func validateSteps(steps []Step) error {
	inProgress := 0
	for index, step := range steps {
		if strings.TrimSpace(step.Description) == "" || !step.Status.Valid() {
			return fmt.Errorf("%w: invalid step %d", ErrInvalid, index)
		}
		if step.Status == InProgress {
			inProgress++
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("%w: at most one step may be in_progress", ErrInvalid)
	}
	return nil
}

func (value State) SessionID() string    { return value.sessionID }
func (value State) Steps() []Step        { return cloneSteps(value.steps) }
func (value State) Revision() uint64     { return value.revision }
func (value State) UpdatedAt() time.Time { return value.updatedAt }

func cloneSteps(steps []Step) []Step {
	if len(steps) == 0 {
		return []Step{}
	}
	return append([]Step(nil), steps...)
}
