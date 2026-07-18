// Package sessions owns the cross-domain atomic write-sets behind a few
// session/run lifecycle use-cases — rollback truncation, the session-delete
// cascade, the import/restore sequence, and the subagent subtree purge. Each
// spans several domain stores (the session row, the transcript, the chat history
// log, open interrupts) and several commit as ONE transaction via RunInTx, so a
// mid-sequence failure leaves no half-mutated session.
//
// These are use-case orchestration, not protocol adaptation: keeping them here
// (driven by the protocol adapter, which still owns wire decode and streaming
// registry concerns) holds the "thin delivery" line and lets the write-sets be
// tested without standing up the wire. The Coordinator reads canonical
// transcript values, decides the mutation, and executes it atomically.
package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionStore is the coordinator's consumer view of session persistence: the
// session-aggregate reads/create, the atomic multi-field Patch, and child
// traversal. Patch is single-domain (all on the session row), so it stays an
// aggregate store method; the multi-store write-sets (fork, rollback, restore,
// delete) go through [WriteSets].
type SessionStore interface {
	List(ctx context.Context) ([]session.Session, error)
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	Patch(ctx context.Context, id string, patch session.Patch) (session.Session, error)
	Children(ctx context.Context, parentID string) ([]session.Session, error)
}

// InterruptStore is the lifecycle coordinator's read view of open HITL
// interrupts. Consuming an interrupt is part of the run coordinator's atomic
// segment-opening commit; deleting one is part of an atomic write-set
// ([WriteSets.ApplyRollback] / ApplyDelete / ApplyTerminal), not a lone call.
type InterruptStore interface {
	List(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	Get(ctx context.Context, runID string) (interrupts.Pending, bool, error)
}

type TranscriptStore interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error)
}

// WriteSets are the atomic durable write-sets the coordinator commits through the
// persistence adapter (§8.1): each applies its whole multi-store mutation in ONE
// transaction, so the coordinator never stitches a transaction across table-CRUD
// calls with the boundary hidden in the context (§8.4). The application decides
// the plan; the adapter executes it atomically, enriching nothing.
type WriteSets interface {
	// ApplyFork branches a child session off the plan's parent, seeds its chat log
	// with the resolved history prefix, and titles it — atomically — returning the
	// created child.
	ApplyFork(ctx context.Context, plan ForkPlan) (session.Session, error)
	// ApplyRollback truncates the chat log to the boundary, drops each
	// past-boundary run, clears the now-invalid todo projection, terminalizes an
	// abandoned parked run, and removes attributed internal subtask subtrees — atomically.
	ApplyRollback(ctx context.Context, plan RollbackPlan) error
	// ApplyRestore recreates a session under its original id and replaces its
	// whole history (clear old session-owned projections + seed decoded
	// messages/runs/items) — atomically.
	ApplyRestore(ctx context.Context, plan RestorePlan) error
	// ApplyDelete removes all durable state for the plan's post-order session
	// cascade — transcript, chat log, todos, session approval rules, interrupts,
	// admission rows, and session rows — atomically.
	ApplyDelete(ctx context.Context, plan DeletePlan) error
	// ApplyTerminal ends a parked run: it persists the terminal transcript
	// projection, drops the open interrupt, and closes admission — atomically.
	ApplyTerminal(ctx context.Context, plan TerminalPlan) error
}

// Stores is the consumer-defined surface the Coordinator drives — the atomic
// write-sets, the session-scoped read/create/patch store, open-interrupt reads,
// coherent aggregate snapshots, and the process-local resume gate. Every
// durable mutation goes through an atomic write-set port
// ([WriteSets] or [SessionStore.Patch]); the coordinator no longer stitches a
// transaction across table-CRUD calls (§8.4). The composition root supplies an
// adapter so the Coordinator depends only on what it calls, not the whole runtime.
type Stores interface {
	WriteSets
	Session() SessionStore
	Interrupts() InterruptStore
	Transcript() TranscriptStore
	// ReadSnapshot returns the session aggregate, conversation history, and
	// transcript from one storage transaction.
	ReadSnapshot(ctx context.Context, sessionID string) (Snapshot, error)
	// ForgetSession releases the executor's process-local state for a session
	// that is being removed (the SessionStart gate).
	ForgetSession(sessionID string)
}

// Snapshot is one coherent, canonical session read used to produce portable
// exports. The application owns this shape; delivery only projects it onto the
// selected wire format.
type Snapshot struct {
	Session     session.Session
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []transcript.Run
	ToolResults []offload.ToolResultBlob
}

// RunRef identifies the durable turn a lifecycle write-set acts on, without
// naming the executor's handle representation — the engine-neutral coordinates
// the [Turns] adapter rebuilds a concrete handle from.
type RunRef struct {
	SessionID string
	TurnID    string
}

// WorkspaceCheckpoints is the coordinator's view of a session's working-tree
// checkpoint store (shadow git): Restore resets the tree to a run-boundary
// snapshot — the filesystem half of a file rollback (§8.5) — and DropSession
// discards a deleted session's snapshots as the last step of the delete cascade.
// Restore is reentrant (a git reset to an already-restored tree is a no-op), so
// the recoverable operation can re-drive it at boot. A disabled store or missing
// snapshot surfaces as [ErrCheckpointUnavailable]; the composition root maps the
// checkpoint adapter's own sentinel onto it so the coordinator stays free of the
// adapter package.
type WorkspaceCheckpoints interface {
	Restore(ctx context.Context, sessionID, cwd, runID string) error
	// DropSession removes a session's checkpoint history. Best-effort cleanup
	// after the durable delete — a failed drop leaks a shadow repo but corrupts no
	// session state.
	DropSession(sessionID string) error
}

// WorkspaceMutations is the recoverable operation log for file rollbacks (§8.5):
// the working tree and the durable history can't commit in one ACID
// transaction, so Record logs the intent before either is touched, Complete
// clears it once both commit, and ListPending returns the operations a crash
// left unfinished for boot recovery to re-drive. The composition root injects a
// store whose writes commit independently (not joined to any rollback
// transaction) — the log is precisely the marker that the two resources change
// out of transaction. nil disables the log (rollback runs without a recovery
// record, degrading to best-effort).
type WorkspaceMutations interface {
	Record(ctx context.Context, m execution.WorkspaceMutation) error
	Complete(ctx context.Context, sessionID string) error
	ListPending(ctx context.Context) ([]execution.WorkspaceMutation, error)
}

// Turns is the engine-neutral process cleanup slice the session lifecycle
// coordinator uses when delete/rollback abandons parked turns. User-visible
// resume/cancel/steer orchestration belongs to application/runs.
type Turns interface {
	Cancel(ctx context.Context, ref RunRef) error
}

// Coordinator executes session/run lifecycle write-sets across the domain
// stores, coordinates single-writer run admission (the per-session and
// per-working-tree slots), and tears down the turn process behind an abandoned
// run. Stateless beyond its collaborators and the in-process admission gates;
// safe to share.
type Coordinator struct {
	s     Stores
	turns Turns
	paths CwdResolver
	// checkpoints resets the working tree to a run-boundary checkpoint for a file
	// rollback and drops a deleted session's snapshots; nil disables both (file
	// restore is rejected as [ErrCheckpointUnavailable], drop no-ops).
	checkpoints WorkspaceCheckpoints
	// mutations is the §8.5 recoverable operation log guarding a file+history
	// rollback across the working tree and the durable history; nil disables it.
	mutations WorkspaceMutations
	// trees serializes short run admissions against destructive working-tree
	// mutations (file rollback) for every transport using this coordinator.
	trees WorkingTreeGate
}

// Dependencies is the collaborator set [New] wires into a Coordinator: the
// consumer-defined store surface (including the atomic durable write-sets), the
// turn dispatcher, the working-tree checkpoint store, and the recoverable
// rollback operation log.
type Dependencies struct {
	Stores      Stores
	Turns       Turns
	Paths       CwdResolver
	Checkpoints WorkspaceCheckpoints
	Mutations   WorkspaceMutations
}

// ErrSessionBusy reports that a session already has an active or parked run.
var ErrSessionBusy = errors.New("sessions: session busy")

// New returns a Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{
		s:           deps.Stores,
		turns:       deps.Turns,
		paths:       deps.Paths,
		checkpoints: deps.Checkpoints,
		mutations:   deps.Mutations,
	}
}

// ClaimWorkingTreeRun reserves cwd's working tree for a run segment admission,
// serializing it against any in-flight destructive mutation of the same tree.
func (c *Coordinator) ClaimWorkingTreeRun(cwd string) (WorkingTreeAdmission, bool) {
	return c.trees.ClaimRun(cwd)
}

// AcquireWorkingTreeRun is the closure-based consumer seam used by
// application/runs. Returning only a release function keeps the concrete
// admission token inside this package while preserving idempotent release.
func (c *Coordinator) AcquireWorkingTreeRun(cwd string) (func(), bool) {
	admission, ok := c.ClaimWorkingTreeRun(cwd)
	if !ok {
		return nil, false
	}
	return admission.Release, true
}

// ClaimWorkingTreeMutation reserves exclusive access to cwd's working tree for a
// destructive mutation such as file rollback.
func (c *Coordinator) ClaimWorkingTreeMutation(cwd string) (WorkingTreeAdmission, bool) {
	return c.trees.ClaimMutation(cwd)
}
