package core

import (
	"context"
	"time"
)

// ProcessSnapshot is the portable form of a running [Process] — what
// goes into a [ProcessStore] and comes back out on restore. The
// snapshot captures everything the runtime needs to either (a) audit
// a completed process or (b) resume a suspended one against the same
// agent definition.
//
// The framework re-binds runtime references (the executable
// [*Agent], its planner, blackboard implementation) by name at
// restore time: a snapshot is portable across redeploys as long as
// the same agent + goal names exist on the target platform.
//
// Two intentional limitations apply to the serialized state:
//
//   - Function-valued blackboard entries do not round-trip. Snapshot
//     code must marshal blackboard values with [encoding/json] (or a
//     user-supplied marshaler in custom [ProcessStore]
//     implementations); values that don't survive that pass are
//     dropped silently. Actions that need closures should keep them
//     on the agent definition, not the blackboard.
//   - The failure chain collapses to a string. The runtime preserves
//     [error.Error()] but loses [errors.Is]/[errors.As] across
//     persistence; treat the field as a human-readable diagnostic.
type ProcessSnapshot struct {
	// ID is the original process id. A restored process keeps the
	// same id so external systems referencing it stay valid.
	ID string `json:"id"`

	// ParentID is empty for top-level processes; populated for
	// child processes created via Platform.CreateChildProcess.
	ParentID string `json:"parent_id,omitempty"`

	// AgentName identifies the agent definition the runtime
	// re-binds the snapshot to. Restore fails when no agent with
	// this name is deployed.
	AgentName string `json:"agent_name"`

	// AgentVersion is the agent definition's version at capture
	// time. Snapshot consumers can compare against the current
	// deployed agent's version to detect schema drift.
	AgentVersion string `json:"agent_version,omitempty"`

	// StartedAt mirrors [Process.StartedAt]; preserved across
	// restore.
	StartedAt time.Time `json:"started_at"`

	// CapturedAt is the snapshot wall-clock time. Useful for
	// debugging "when did this state come from" without inspecting
	// store metadata.
	CapturedAt time.Time `json:"captured_at"`

	// Status is the most recent [AgentProcessStatus]. Resumable
	// statuses (StatusWaiting / StatusPaused) re-enter the tick
	// loop; terminal statuses are loaded read-only.
	Status AgentProcessStatus `json:"status"`

	// GoalName identifies the goal currently being pursued. Empty
	// when the process hasn't yet selected one. Re-bound at
	// restore from the agent's goal set.
	GoalName string `json:"goal_name,omitempty"`

	// LastWorld is the snapshot of [WorldState] the planner most
	// recently observed. The planner re-runs after restore so
	// this is informational; restoring without it still works.
	LastWorld WorldState `json:"last_world,omitempty"`

	// History is the full action-invocation history captured at
	// snapshot time. Restore replays it for diagnostics; the
	// runtime does NOT re-execute completed actions.
	History []SnapshotActionInvocation `json:"history,omitempty"`

	// Failure is the most recent error message. Empty when the
	// process has not failed.
	Failure string `json:"failure,omitempty"`

	// Cost / Tokens are the rolling budget totals. Restore
	// re-installs them on the process budget.
	Cost   float64 `json:"cost"`
	Tokens int     `json:"tokens"`

	// LLMInvocations / EmbeddingInvocations preserve the full
	// per-call history. Useful for cost audit even after the
	// process has terminated.
	LLMInvocations       []LLMInvocation       `json:"llm_invocations,omitempty"`
	EmbeddingInvocations []EmbeddingInvocation `json:"embedding_invocations,omitempty"`

	// Blackboard captures the named bindings + conditions of the
	// process's blackboard. Function values are dropped (see
	// package-level note). Custom blackboard implementations may
	// extend this map with provider-specific keys.
	Blackboard map[string]any `json:"blackboard,omitempty"`

	// Conditions captures explicit boolean state set via
	// [BlackboardWriter.SetCondition]. Kept separate from
	// Blackboard because conditions have their own truth-value
	// semantics.
	Conditions map[string]bool `json:"conditions,omitempty"`

	// Objects captures the ordered "anonymous artifacts" list
	// produced by [BlackboardWriter.AddObject]. Same JSON-only
	// constraint as Blackboard.
	Objects []any `json:"objects,omitempty"`
}

// SnapshotActionInvocation is the portable form of one action
// history row. Mirrors [ActionInvocation] but with a string Status
// field instead of the enum so JSON consumers don't need to know
// the integer encoding.
type SnapshotActionInvocation struct {
	ActionName string        `json:"action_name"`
	Timestamp  time.Time     `json:"timestamp"`
	Duration   time.Duration `json:"duration_ns"`
	Status     string        `json:"status"`
	Attempts   int           `json:"attempts"`
}

// ProcessStore is the persistence SPI for agent processes. Backends
// implement this to hold snapshots — typical use cases:
//
//   - Long-running agents that must survive process restart
//   - Cross-node resumption (handoff a paused process to a different
//     worker)
//   - Audit / replay (snapshot every tick, inspect later)
//
// Lynx ships [NewInMemoryProcessStore] as a reference implementation
// suitable for tests + development. Production backends (postgres,
// redis, mongodb, ...) live in the top-level `agentstore/` module
// once that lands.
//
// All methods are expected to be safe for concurrent use.
type ProcessStore interface {
	// Save persists a snapshot under its [ProcessSnapshot.ID].
	// Existing snapshots with the same id are overwritten.
	Save(ctx context.Context, snapshot ProcessSnapshot) error

	// Load returns the most recent snapshot for id. The returned
	// error wraps a sentinel ([ErrSnapshotNotFound]) when the id
	// is unknown.
	Load(ctx context.Context, id string) (ProcessSnapshot, error)

	// Delete removes the snapshot for id. Returns nil when the id
	// is unknown — deletes are idempotent.
	Delete(ctx context.Context, id string) error

	// List returns every known process id, in any order. Backends
	// that paginate naturally may return a stable subset and let
	// callers iterate via repeated calls — the interface does not
	// dictate pagination semantics.
	List(ctx context.Context) ([]string, error)
}

// ErrSnapshotNotFound is the sentinel [ProcessStore.Load] wraps when
// asked for an unknown id. Callers special-case via errors.Is.
var ErrSnapshotNotFound = errSnapshotNotFound{}

type errSnapshotNotFound struct{}

func (errSnapshotNotFound) Error() string { return "process store: snapshot not found" }
