package execution

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
)

// ErrProcessStateNotFound reports that no durable process tree exists for
// the requested root identity.
var ErrProcessStateNotFound = errors.New("process state not found")

// ErrInvalidProcessTreeState reports a malformed application-owned envelope
// around an executor's opaque process state.
var ErrInvalidProcessTreeState = errors.New("invalid process tree state")

// ProcessState is one executor process projected into App-owned persistence
// vocabulary. Payload is opaque to every layer except the executor adapter;
// App retains only the identity, topology, and start time it needs for durable
// ownership and product lineage.
type ProcessState struct {
	ID        string
	ParentID  string
	StartedAt time.Time
	Payload   []byte
}

// ProcessTreeState is the complete App-owned persistence envelope for one
// executor process tree. It deliberately does not expose the executor SDK's
// snapshot types.
type ProcessTreeState struct {
	RootID    string
	Processes []ProcessState
}

// Validate verifies the application envelope without interpreting Payload.
func (tree ProcessTreeState) Validate() error {
	if strings.TrimSpace(tree.RootID) == "" || strings.TrimSpace(tree.RootID) != tree.RootID {
		return fmt.Errorf("%w: root ID must be non-empty without surrounding whitespace", ErrInvalidProcessTreeState)
	}
	if len(tree.Processes) == 0 {
		return fmt.Errorf("%w: process tree is empty", ErrInvalidProcessTreeState)
	}

	byID := make(map[string]ProcessState, len(tree.Processes))
	children := make(map[string][]string, len(tree.Processes))
	for index, process := range tree.Processes {
		if strings.TrimSpace(process.ID) == "" || strings.TrimSpace(process.ID) != process.ID {
			return fmt.Errorf("%w: processes[%d] has an invalid ID", ErrInvalidProcessTreeState, index)
		}
		if process.ParentID != strings.TrimSpace(process.ParentID) || process.ParentID == process.ID {
			return fmt.Errorf("%w: process %q has an invalid parent ID", ErrInvalidProcessTreeState, process.ID)
		}
		if process.StartedAt.IsZero() {
			return fmt.Errorf("%w: process %q has no start time", ErrInvalidProcessTreeState, process.ID)
		}
		if len(process.Payload) == 0 {
			return fmt.Errorf("%w: process %q has an empty payload", ErrInvalidProcessTreeState, process.ID)
		}
		if _, duplicate := byID[process.ID]; duplicate {
			return fmt.Errorf("%w: duplicate process ID %q", ErrInvalidProcessTreeState, process.ID)
		}
		byID[process.ID] = process
		children[process.ParentID] = append(children[process.ParentID], process.ID)
	}

	root, ok := byID[tree.RootID]
	if !ok {
		return fmt.Errorf("%w: root process %q is missing", ErrInvalidProcessTreeState, tree.RootID)
	}
	if root.ParentID != "" {
		return fmt.Errorf("%w: root process %q has parent %q", ErrInvalidProcessTreeState, root.ID, root.ParentID)
	}
	for _, process := range tree.Processes {
		if process.ID == tree.RootID {
			continue
		}
		if _, found := byID[process.ParentID]; !found {
			return fmt.Errorf(
				"%w: process %q has parent %q outside tree %q",
				ErrInvalidProcessTreeState,
				process.ID,
				process.ParentID,
				tree.RootID,
			)
		}
	}

	reached := 0
	pending := []string{tree.RootID}
	seen := make(map[string]struct{}, len(tree.Processes))
	for len(pending) > 0 {
		processID := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if _, duplicate := seen[processID]; duplicate {
			continue
		}
		seen[processID] = struct{}{}
		reached++
		pending = append(pending, children[processID]...)
	}
	if reached != len(tree.Processes) {
		return fmt.Errorf(
			"%w: %d of %d processes are unreachable from root %q",
			ErrInvalidProcessTreeState,
			len(tree.Processes)-reached,
			len(tree.Processes),
			tree.RootID,
		)
	}
	return nil
}

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
