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
	SessionID         string
	CWD               string
	WorkspaceCWD      string
	Isolated          bool
	GoalIncarnationID string
}

// Validate rejects ambiguous host identities before they cross a durable
// continuation boundary.
func (e ExecutionScope) Validate() error {
	if strings.TrimSpace(e.SessionID) == "" {
		return errors.New("execution: scope session ID is required")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "session ID", value: e.SessionID},
		{name: "working dir", value: e.CWD},
		{name: "workspace dir", value: e.WorkspaceCWD},
		{name: "goal incarnation ID", value: e.GoalIncarnationID},
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
	Limits         run.Limits
	Capabilities   run.Capabilities
	Usage          accounting.Snapshot
}

// ExecutorCheckpointExpectation is the durable identity and host context a
// durable continuation must still belong to before it may be retained or
// restored. It contains no executor topology: every field is independently
// known by the owning Run and Session.
type ExecutorCheckpointExpectation struct {
	RootMemberID      string
	SessionID         string
	CWD               string
	WorkspaceCWD      string
	Isolated          bool
	GoalIncarnationID string
	ModelSelection    modelref.Selection
	Limits            run.Limits
	Capabilities      run.Capabilities
}

// Clone returns an ownership-independent checkpoint value.
func (e ExecutorCheckpoint) Clone() ExecutorCheckpoint {
	e.Payload = append([]byte(nil), e.Payload...)
	e.Capabilities = e.Capabilities.Clone()
	e.Usage.Models = append([]accounting.ModelUsage(nil), e.Usage.Models...)
	return e
}

// Validate verifies the host-owned metadata without interpreting the
// executor payload.
func (e ExecutorCheckpoint) Validate() error {
	if strings.TrimSpace(e.RootMemberID) == "" || e.RootMemberID != strings.TrimSpace(e.RootMemberID) {
		return fmt.Errorf("%w: root member ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if len(e.Payload) == 0 {
		return fmt.Errorf("%w: payload is empty", ErrInvalidExecutorCheckpoint)
	}
	if strings.TrimSpace(e.BuildID) == "" || e.BuildID != strings.TrimSpace(e.BuildID) {
		return fmt.Errorf("%w: build ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if err := e.Scope.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := e.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("%w: model selection: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := e.Limits.Validate(); err != nil {
		return fmt.Errorf("%w: limits: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := e.Capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: capabilities: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if err := e.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: usage: %w", ErrInvalidExecutorCheckpoint, err)
	}
	return nil
}

// ValidateOwnership proves that the checkpoint and its owning Run aggregate
// name the same root member and Session. Callers use this at every atomic
// Pending/checkpoint write boundary so two separately valid values cannot be
// committed as one mismatched continuation.
func (e ExecutorCheckpoint) ValidateOwnership(rootMemberID, sessionID string) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(rootMemberID) == "" || rootMemberID != strings.TrimSpace(rootMemberID) {
		return fmt.Errorf("%w: expected root member ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if strings.TrimSpace(sessionID) == "" || sessionID != strings.TrimSpace(sessionID) {
		return fmt.Errorf("%w: expected session ID must be non-empty without surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if e.RootMemberID != rootMemberID {
		return fmt.Errorf(
			"%w: root member ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.RootMemberID,
			rootMemberID,
		)
	}
	if e.Scope.SessionID != sessionID {
		return fmt.Errorf(
			"%w: session ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.Scope.SessionID,
			sessionID,
		)
	}
	return nil
}

// ValidateFor proves both ownership and every host fact independently known at
// restore time. This prevents one logical execution from running tools in
// the checkpoint workspace while hooks or delegated work use the Session's
// current workspace.
func (e ExecutorCheckpoint) ValidateFor(expected ExecutorCheckpointExpectation) error {
	if err := e.ValidateOwnership(expected.RootMemberID, expected.SessionID); err != nil {
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
	if err := expected.Capabilities.Validate(); err != nil {
		return fmt.Errorf("%w: expected capabilities: %w", ErrInvalidExecutorCheckpoint, err)
	}
	if expected.GoalIncarnationID != strings.TrimSpace(expected.GoalIncarnationID) {
		return fmt.Errorf("%w: expected goal incarnation ID has surrounding whitespace", ErrInvalidExecutorCheckpoint)
	}
	if e.Scope.CWD != expected.CWD {
		return fmt.Errorf(
			"%w: working dir %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.Scope.CWD,
			expected.CWD,
		)
	}
	if e.Scope.WorkspaceCWD != expected.WorkspaceCWD {
		return fmt.Errorf(
			"%w: workspace dir %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.Scope.WorkspaceCWD,
			expected.WorkspaceCWD,
		)
	}
	if e.Scope.Isolated != expected.Isolated {
		return fmt.Errorf(
			"%w: isolation %t does not match owner %t",
			ErrInvalidExecutorCheckpoint,
			e.Scope.Isolated,
			expected.Isolated,
		)
	}
	if e.Scope.GoalIncarnationID != expected.GoalIncarnationID {
		return fmt.Errorf(
			"%w: goal incarnation ID %q does not match owner %q",
			ErrInvalidExecutorCheckpoint,
			e.Scope.GoalIncarnationID,
			expected.GoalIncarnationID,
		)
	}
	if e.ModelSelection != expected.ModelSelection {
		return fmt.Errorf(
			"%w: model selection %q/%q does not match owner %q/%q",
			ErrInvalidExecutorCheckpoint,
			e.ModelSelection.Provider(),
			e.ModelSelection.Model(),
			expected.ModelSelection.Provider(),
			expected.ModelSelection.Model(),
		)
	}
	if e.Limits != expected.Limits {
		return fmt.Errorf(
			"%w: limits %+v do not match owner %+v",
			ErrInvalidExecutorCheckpoint,
			e.Limits,
			expected.Limits,
		)
	}
	if !e.Capabilities.Equal(expected.Capabilities) {
		return fmt.Errorf(
			"%w: capabilities %+v do not match owner %+v",
			ErrInvalidExecutorCheckpoint,
			e.Capabilities,
			expected.Capabilities,
		)
	}
	return nil
}

func validateCheckpointSessionScope(
	checkpoint ExecutorCheckpoint,
	sess session.Session,
) error {
	if checkpoint.Scope.WorkspaceCWD != sess.Workspace().Path() || checkpoint.Scope.Isolated != sess.Isolated() {
		return fmt.Errorf("%w: checkpoint workspace scope differs from Session", ErrExecutorStateLost)
	}
	if sess.Isolated() {
		if strings.TrimSpace(checkpoint.Scope.CWD) == "" {
			return fmt.Errorf("%w: isolated checkpoint working directory is empty", ErrExecutorStateLost)
		}
		return nil
	}
	if checkpoint.Scope.CWD != sess.Workspace().Path() {
		return fmt.Errorf("%w: checkpoint working directory differs from Session", ErrExecutorStateLost)
	}
	return nil
}
