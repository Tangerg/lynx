package execution

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

// ErrProcessSnapshotNotFound reports that no durable process tree exists for
// the requested root identity.
var ErrProcessSnapshotNotFound = errors.New("process snapshot not found")

// TurnScope is the immutable application execution context shared by a root
// turn and every delegated child. It belongs to Runtime rather than Agent:
// sessions, workspace isolation, and autonomous-goal leases are host concepts,
// not planner state.
type TurnScope struct {
	SessionID   string
	Cwd         string
	Isolated    bool
	GoalLeaseID string
}

// Validate rejects ambiguous host identities before they cross a durable
// continuation boundary.
func (s TurnScope) Validate() error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "session ID", value: s.SessionID},
		{name: "working dir", value: s.Cwd},
		{name: "goal lease ID", value: s.GoalLeaseID},
	} {
		if field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("execution: turn scope %s has surrounding whitespace", field.name)
		}
	}
	return nil
}

// ProcessCheckpoint is the application-owned metadata committed atomically
// with an Agent process tree. Agent owns executable continuation state; Runtime
// owns the executable build identity, host scope, provider identity, execution
// budget, and accounting projection required to resume it correctly.
type ProcessCheckpoint struct {
	BuildID  string
	Scope    TurnScope
	Provider string
	Budget   accounting.Budget
	Usage    accounting.Snapshot
}

// Validate verifies every application-owned continuation invariant.
func (c ProcessCheckpoint) Validate() error {
	if strings.TrimSpace(c.BuildID) == "" || c.BuildID != strings.TrimSpace(c.BuildID) {
		return errors.New("execution: process checkpoint has invalid build ID")
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if c.Provider != strings.TrimSpace(c.Provider) {
		return errors.New("execution: process checkpoint provider has surrounding whitespace")
	}
	if err := c.Budget.Validate(); err != nil {
		return fmt.Errorf("execution: process checkpoint budget: %w", err)
	}
	if err := c.Usage.Validate(); err != nil {
		return fmt.Errorf("execution: process checkpoint usage: %w", err)
	}
	return nil
}
