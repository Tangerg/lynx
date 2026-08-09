package runs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// ErrExecutorCheckpointNotFound reports that no durable executor checkpoint
// exists for the requested root member identity.
var ErrExecutorCheckpointNotFound = errors.New("executor checkpoint not found")

// ErrInvalidExecutorCheckpoint reports malformed host-owned metadata around
// an executor's opaque continuation state.
var ErrInvalidExecutorCheckpoint = errors.New("invalid executor checkpoint")

// ExecutionScope is the immutable host context shared by a root execution and
// every delegated child. Sessions, workspace isolation, and autonomous-goal
// leases are host facts, not planner state.
type ExecutionScope struct {
	SessionID    string
	CWD          string
	WorkspaceCWD string
	Isolated     bool
	GoalLeaseID  string
}

// Validate rejects ambiguous host identities before they cross a durable
// continuation boundary.
func (s ExecutionScope) Validate() error {
	if strings.TrimSpace(s.SessionID) == "" {
		return errors.New("execution: scope session ID is required")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "session ID", value: s.SessionID},
		{name: "working dir", value: s.CWD},
		{name: "workspace dir", value: s.WorkspaceCWD},
		{name: "goal lease ID", value: s.GoalLeaseID},
	} {
		if field.value != strings.TrimSpace(field.value) {
			return fmt.Errorf("execution: scope %s has surrounding whitespace", field.name)
		}
	}
	return nil
}

// ExecutorCheckpoint is one root-owned durable continuation aggregate. Payload
// contains the complete executor tree and is opaque outside its executor
// implementation; the host owns only the aggregate identity and metadata needed
// to decide whether and how the continuation may be restored.
type ExecutorCheckpoint struct {
	RootMemberID   string
	Payload        []byte
	BuildID        string
	Scope          ExecutionScope
	ModelSelection modelref.Selection
	Limits         run.RunLimits
	Usage          accounting.Snapshot
}

// ExecutorCheckpointExpectation is the durable identity and host context a
// durable continuation must still belong to before it may be retained or
// restored. It contains no executor topology: every field is independently
// known by the owning Run and Session.
type ExecutorCheckpointExpectation struct {
	RootMemberID   string
	SessionID      string
	CWD            string
	WorkspaceCWD   string
	Isolated       bool
	GoalLeaseID    string
	ModelSelection modelref.Selection
	Limits         run.RunLimits
}

// Clone returns an ownership-independent checkpoint value.
func (c ExecutorCheckpoint) Clone() ExecutorCheckpoint {
	c.Payload = append([]byte(nil), c.Payload...)
	c.Usage.Models = append([]accounting.ModelUsage(nil), c.Usage.Models...)
	return c
}

// Validate verifies the host-owned metadata without interpreting the
// executor payload.
func (c ExecutorCheckpoint) Validate() error {
	if strings.TrimSpace(c.RootMemberID) == "" || c.RootMemberID != strings.TrimSpace(c.RootMemberID) {
		return fmt.Errorf("%w: root member ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if len(c.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", ErrInvalidExecutorCheckpoint)
	}
	if strings.TrimSpace(c.BuildID) == "" || c.BuildID != strings.TrimSpace(c.BuildID) {
		return fmt.Errorf("%w: build ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if err := c.Scope.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := c.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("%w: model selection: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := c.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: limits: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := c.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: usage: %w", ErrInvalidExecutorCheckpoint, err)
	}
	return nil
}

// ValidateOwnership proves that the checkpoint and its owning Run aggregate
// name the same root member and Session. Callers use this at every atomic
// Pending/checkpoint write boundary so two separately valid values cannot be
// committed as one mismatched continuation.
func (c ExecutorCheckpoint) ValidateOwnership(rootMemberID, sessionID string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(rootMemberID) == "" || rootMemberID != strings.TrimSpace(rootMemberID) {
		return fmt.Errorf("%w: expected root member ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if strings.TrimSpace(sessionID) == "" || sessionID != strings.TrimSpace(sessionID) {
		return fmt.Errorf("%w: expected session ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if c.RootMemberID != rootMemberID {
		return fmt.Errorf(
			"%w: root member ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			c.RootMemberID,
			rootMemberID,
		)
	}
	if c.Scope.SessionID != sessionID {
		return fmt.Errorf(
			"%w: session ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			c.Scope.SessionID,
			sessionID,
		)
	}
	return nil
}

// ValidateFor proves both ownership and every host fact independently known at
// restore time. This prevents one logical execution from running tools in
// the checkpoint workspace while hooks or delegated work use the Session's
// current workspace.
func (c ExecutorCheckpoint) ValidateFor(expected ExecutorCheckpointExpectation) error {
	if err := c.ValidateOwnership(expected.RootMemberID, expected.SessionID); err != nil {
		return err
	}
	if expected.CWD != strings.TrimSpace(expected.CWD) {
		return fmt.Errorf("%w: expected working dir has surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if expected.WorkspaceCWD != strings.TrimSpace(expected.WorkspaceCWD) {
		return fmt.Errorf("%w: expected workspace dir has surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if err := expected.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("%w: expected model selection: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := expected.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: expected limits: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if expected.GoalLeaseID != strings.TrimSpace(expected.GoalLeaseID) {
		return fmt.Errorf("%w: expected goal lease ID has surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if c.Scope.CWD != expected.CWD {
		return fmt.Errorf(
			"%w: working dir %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			c.Scope.CWD,
			expected.CWD,
		)
	}
	if c.Scope.WorkspaceCWD != expected.WorkspaceCWD {
		return fmt.Errorf(
			"%w: workspace dir %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			c.Scope.WorkspaceCWD,
			expected.WorkspaceCWD,
		)
	}
	if c.Scope.Isolated != expected.Isolated {
		return fmt.Errorf(
			"%w: isolation %t does not match owner %t",
			ErrInvalidExecutorCheckpoint,
			c.Scope.Isolated,
			expected.Isolated,
		)
	}
	if c.Scope.GoalLeaseID != expected.GoalLeaseID {
		return fmt.Errorf(
			"%w: goal lease ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			c.Scope.GoalLeaseID,
			expected.GoalLeaseID,
		)
	}
	if c.ModelSelection != expected.ModelSelection {
		return fmt.Errorf(
			"%w: model selection %q/%q does not match owner %q/%q",
			ErrInvalidExecutorCheckpoint,
			c.ModelSelection.Provider(),
			c.ModelSelection.Model(),
			expected.ModelSelection.Provider(),
			expected.ModelSelection.Model(),
		)
	}
	if c.Limits != expected.Limits {
		return fmt.Errorf(
			"%w: limits %+v do not match owner %+v",
			ErrInvalidExecutorCheckpoint,
			c.Limits,
			expected.Limits,
		)
	}
	return nil
}

func validateCheckpointSessionScope(
	checkpoint ExecutorCheckpoint,
	sess session.Session,
) error {
	if checkpoint.Scope.WorkspaceCWD != sess.CWD || checkpoint.Scope.Isolated != sess.Isolated {
		return fmt.Errorf("%w: checkpoint workspace scope differs from Session", ErrExecutorStateLost)
	}
	if sess.Isolated {
		if strings.TrimSpace(checkpoint.Scope.CWD) == "" {
			return fmt.Errorf("%w: isolated checkpoint working directory is empty", ErrExecutorStateLost)
		}
		return nil
	}
	if checkpoint.Scope.CWD != sess.CWD {
		return fmt.Errorf("%w: checkpoint working directory differs from Session", ErrExecutorStateLost)
	}
	return nil
}
