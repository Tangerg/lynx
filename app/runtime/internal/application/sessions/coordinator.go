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
// tested without standing up the wire. The adapter lifts wire blobs into domain
// values; the Coordinator decides and executes the multi-domain mutation
// atomically.
package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// SessionStore is the coordinator's consumer view of session persistence: the
// session-aggregate CRUD (list / get / create / patch) plus the lifecycle
// write-set operations (branch, restore, delete, child traversal). It excludes
// only the turn-scoped touchpoints the dispatcher owns.
type SessionStore interface {
	List(ctx context.Context) ([]session.Session, error)
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	Rename(ctx context.Context, id, title string) error
	SetModel(ctx context.Context, id, model string) error
	SetCwd(ctx context.Context, id, cwd string) error
	SetMetadata(ctx context.Context, id string, meta map[string]any) error
	SetFavorite(ctx context.Context, id string, favorite bool) error
	Fork(ctx context.Context, parentID, atMessageID string) (session.Session, error)
	Restore(ctx context.Context, sess session.Session) error
	Children(ctx context.Context, parentID string) ([]session.Session, error)
	Delete(ctx context.Context, id string) error
}

// TranscriptStore is the lifecycle coordinator's write-set view of the durable
// transcript. Lifecycle operations import, rollback, and purge transcript
// records; they do not read transcript projections.
type TranscriptStore interface {
	AppendItem(ctx context.Context, it transcript.Item) error
	PutRun(ctx context.Context, r transcript.Run) error
	DeleteRun(ctx context.Context, sessionID, runID string) error
	DeleteSession(ctx context.Context, sessionID string) error
}

// InterruptStore is the lifecycle coordinator's view of open HITL interrupts.
// Put is used only to compensate a consumed cross-restart claim when rehydrate
// fails before any continuation can run.
type InterruptStore interface {
	Put(ctx context.Context, pending interrupts.Pending) error
	List(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	Get(ctx context.Context, parentRunID string) (interrupts.Pending, bool, error)
	Consume(ctx context.Context, parentRunID string) (interrupts.Pending, bool, error)
	Delete(ctx context.Context, parentRunID string) error
}

// Stores is the consumer-defined surface the Coordinator drives — the runtime's
// session-scoped stores plus the chat history log, the process-local resume
// gate (ForgetSession), and the transactional seam (RunInTx). The composition
// root supplies an adapter so the Coordinator depends only on what it calls,
// not the whole runtime.
type Stores interface {
	Session() SessionStore
	Transcript() TranscriptStore
	Interrupts() InterruptStore
	// ReadHistory returns the chat history log for a session.
	ReadHistory(ctx context.Context, sessionID string) ([]chat.Message, error)
	// TruncateMessages clamps a session's chat history log to keepN messages
	// (keepN=0 clears it).
	TruncateMessages(ctx context.Context, sessionID string, keepN int) error
	// SeedHistory replaces a session's chat history log with msgs.
	SeedHistory(ctx context.Context, sessionID string, msgs []chat.Message) error
	// ForgetSession releases the executor's process-local state for a session
	// that is being removed (the SessionStart gate).
	ForgetSession(sessionID string)
	// RunInTx runs fn inside one storage transaction; the store calls the
	// closure makes join it through the context.
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}

// RunRef identifies the durable turn a lifecycle write-set acts on, without
// naming the executor's handle representation — the engine-neutral coordinates
// the [Turns] adapter rebuilds a concrete handle from.
type RunRef struct {
	SessionID string
	TurnID    string
}

// RehydrateSpec describes rebuilding a parked turn's process from its durable
// interrupt snapshot after process-local state was lost.
type RehydrateSpec struct {
	SessionID      string
	ProcessID      string
	Approved       bool
	Provider       string
	Model          string
	InterruptKinds []string
}

// Handle is the opaque per-turn execution handle a resumed / rehydrated turn
// runs under. It is opaque to the coordinator: the delivery layer forwards it to
// the run coordinator (as its own opaque handle) without inspecting it.
type Handle = any

// Turns is the engine-neutral turn-control slice the lifecycle coordinator
// drives to abandon (Cancel) or continue (Resume / Rehydrate) the process
// backing a run. The composition root injects an adapter over the agent turn
// dispatcher that rebuilds a concrete handle from a [RunRef] and maps the
// dispatcher's resume outcomes onto [ErrParkClaimed] / [ErrTurnNotLive] /
// [ErrRehydrateCommitted]. Held as a fixed collaborator, not a per-call
// parameter: it is the same executor for every lifecycle write-set.
type Turns interface {
	Cancel(ctx context.Context, ref RunRef) error
	Resume(ctx context.Context, ref RunRef, resolution interrupts.Resolution, interruptKinds []string) (Handle, error)
	Rehydrate(ctx context.Context, req RehydrateSpec) (Handle, error)
}

// DurableRuns is the coordinator's write view of the durable Run admission table
// (§8.2): the abandonment write-sets free or drop a session's admission slot so a
// run this coordinator cancels, rolls back, deletes, or restores never leaves the
// session durably busy. It is the durable counterpart to the in-process
// [RunAdmission] slot. The sqlite RunStateStore satisfies it structurally; a nil
// collaborator disables it (the in-process claim still guards within one process).
type DurableRuns interface {
	// Terminalize marks the session's non-terminal Run terminal, freeing the slot
	// when the session survives the abandonment (parked-cancel, rollback).
	Terminalize(ctx context.Context, sessionID, outcome string) error
	// DeleteForSession drops every Run row of a session whose durable state is
	// removed or replaced wholesale (delete-cascade, restore, subtree purge).
	DeleteForSession(ctx context.Context, sessionID string) error
}

// Coordinator executes session/run lifecycle write-sets across the domain
// stores, coordinates single-writer run admission (the per-session and
// per-working-tree slots), and tears down the turn process behind an abandoned
// run. Stateless beyond its collaborators and the in-process admission gates;
// safe to share.
type Coordinator struct {
	s     Stores
	turns Turns
	// runs is the durable admission table; the abandonment write-sets free/drop
	// its slot. nil disables the durable backstop (in-process claim only).
	runs DurableRuns
	// trees serializes short run admissions against destructive working-tree
	// mutations (file rollback) for every transport using this coordinator.
	trees WorkingTreeGate
}

// Dependencies is the collaborator set [New] wires into a Coordinator: the
// consumer-defined store surface, the turn dispatcher, and the durable
// run-admission table (nil to run in-process-only).
type Dependencies struct {
	Stores Stores
	Turns  Turns
	Runs   DurableRuns
}

// ErrRunNotFound reports that a lifecycle operation targeted no live or parked run.
var ErrRunNotFound = errors.New("sessions: run not found")

// ErrInterruptNotOpen reports that an interrupt resume/cancel target is no
// longer open.
var ErrInterruptNotOpen = errors.New("sessions: interrupt not open")

// ErrSessionBusy reports that a session already has an active or parked run.
var ErrSessionBusy = errors.New("sessions: session busy")

// ErrParkClaimed, ErrTurnNotLive, and ErrRehydrateCommitted are the
// engine-neutral resume outcomes the [Turns] adapter maps the executor's errors
// onto, so the coordinator branches on resume semantics without importing the
// agent turn package: ErrParkClaimed = another resume already claimed the parked
// turn; ErrTurnNotLive = the process is gone (fall back to rehydrate);
// ErrRehydrateCommitted = rehydrate already terminalized the process, so the
// consumed interrupt must NOT be restored (restoring would create a ghost).
var (
	ErrParkClaimed        = errors.New("sessions: parked turn already claimed")
	ErrTurnNotLive        = errors.New("sessions: turn not live")
	ErrRehydrateCommitted = errors.New("sessions: rehydrate already committed")
)

// New returns a Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{s: deps.Stores, turns: deps.Turns, runs: deps.Runs}
}

// terminalizeRun frees the session's durable admission slot after this
// coordinator abandons its non-terminal run (parked-cancel, rollback). The
// session survives, so the row is terminalized (canceled), not deleted.
// Best-effort: a missed write leaves a non-terminal row with no open interrupt,
// which the next boot's ReconcileOrphans sweeps.
func (c *Coordinator) terminalizeRun(ctx context.Context, sessionID string) {
	if c.runs != nil {
		_ = c.runs.Terminalize(ctx, sessionID, execution.OutcomeCanceled.String())
	}
}

// deleteRunRows drops a session's durable admission rows when its whole state is
// removed or replaced (delete-cascade, restore, purge). Joins the caller's
// transaction via ctx when invoked inside RunInTx.
func (c *Coordinator) deleteRunRows(ctx context.Context, sessionID string) error {
	if c.runs == nil {
		return nil
	}
	return c.runs.DeleteForSession(ctx, sessionID)
}

// ClaimWorkingTreeRun reserves cwd's working tree for a run segment admission,
// serializing it against any in-flight destructive mutation of the same tree.
func (c *Coordinator) ClaimWorkingTreeRun(cwd string) (WorkingTreeAdmission, bool) {
	return c.trees.ClaimRun(worktree.CanonicalCwd(cwd))
}

// ClaimWorkingTreeMutation reserves exclusive access to cwd's working tree for a
// destructive mutation such as file rollback.
func (c *Coordinator) ClaimWorkingTreeMutation(cwd string) (WorkingTreeAdmission, bool) {
	return c.trees.ClaimMutation(worktree.CanonicalCwd(cwd))
}
