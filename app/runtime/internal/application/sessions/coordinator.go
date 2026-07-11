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

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// SessionStore is the lifecycle coordinator's consumer view of session
// persistence. It intentionally excludes unrelated read/update operations from
// domain/session.Store: lifecycle write-sets only need branch, restore, delete,
// child traversal, and post-fork rename.
type SessionStore interface {
	Fork(ctx context.Context, parentID, atMessageID string) (session.Session, error)
	Rename(ctx context.Context, id, title string) error
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
	// ForgetSession releases the turn dispatcher's process-local state for a
	// session that is being removed (the SessionStart gate) — see turn.Dispatcher.
	ForgetSession(sessionID string)
	// RunInTx runs fn inside one storage transaction; the store calls the
	// closure makes join it through the context.
	RunInTx(ctx context.Context, fn func(context.Context) error) error
}

// Turns is the turn-dispatcher slice the lifecycle coordinator drives to
// abandon (Cancel) or continue (Resume / Rehydrate) the process backing a run.
// The kernel turn dispatcher satisfies it. Held as a fixed collaborator, not a
// per-call parameter: it is the same dispatcher for every lifecycle write-set.
type Turns interface {
	Cancel(context.Context, turn.TurnHandle) error
	Resume(context.Context, turn.TurnHandle, interrupts.Resolution, []string) error
	Rehydrate(context.Context, turn.RehydrateRequest) (turn.TurnHandle, error)
}

// Coordinator executes session/run lifecycle write-sets across the domain
// stores, coordinates single-writer run admission (the per-session and
// per-working-tree slots), and tears down the turn process behind an abandoned
// run. Stateless beyond its collaborators and the in-process admission gates;
// safe to share.
type Coordinator struct {
	s     Stores
	turns Turns
	// trees serializes short run admissions against destructive working-tree
	// mutations (file rollback) for every transport using this coordinator.
	trees WorkingTreeGate
}

// Dependencies is the collaborator set [New] wires into a Coordinator: the
// consumer-defined store surface and the turn dispatcher.
type Dependencies struct {
	Stores Stores
	Turns  Turns
}

// ErrRunNotFound reports that a lifecycle operation targeted no live or parked run.
var ErrRunNotFound = errors.New("sessions: run not found")

// ErrInterruptNotOpen reports that an interrupt resume/cancel target is no
// longer open.
var ErrInterruptNotOpen = errors.New("sessions: interrupt not open")

// ErrSessionBusy reports that a session already has an active or parked run.
var ErrSessionBusy = errors.New("sessions: session busy")

// New returns a Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{s: deps.Stores, turns: deps.Turns}
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
