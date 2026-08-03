// Package plan defines the session-scoped execution plan maintained by the
// root Agent. A Plan is one ordered list of Steps; it is neither a second task
// system nor Plan-mode state. Model presentation and persistence live in
// adapters.
package plan

import (
	"errors"
	"fmt"
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

// ErrInvalid marks a Plan that violates a domain invariant.
var ErrInvalid = errors.New("plan: invalid plan")

// Validate verifies a complete Plan replacement. An empty Plan is valid and
// means clear the current Plan. At most one Step may be in progress so the
// execution focus remains unambiguous.
func Validate(steps []Step) error {
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

// State is the latest complete Plan projection for one session. Revision is
// monotonic because a Plan is replaced wholesale; it lets clients reject an
// older replacement even when the new Plan is shorter.
type State struct {
	Steps    []Step
	Revision uint64
	// UpdatedAt is zero exactly while Revision is zero: no Plan has been set.
	UpdatedAt time.Time
}
