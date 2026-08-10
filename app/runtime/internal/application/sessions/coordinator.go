// Package sessions owns the cross-domain atomic write-sets behind a few
// session/run lifecycle use-cases — rollback truncation, the session-delete
// cascade, the import/restore sequence, and the child-Run subtree purge. Each
// spans several domain stores (the session row, the transcript, the chat history
// log, open interrupts) and several commit as ONE transaction via RunInTx, so a
// mid-sequence failure leaves no half-mutated session.
//
// The Coordinator reads canonical transcript values, decides each mutation, and
// executes it atomically. These rules are use-case orchestration and stay
// independent of request decoding, presentation, and stream management.
package sessions

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Store is the coordinator's consumer view of session persistence: the
// session-aggregate reads/create and the atomic multi-field Patch. Patch is
// single-domain (all on the session row), so it stays an
// aggregate store method; the multi-store write-sets (fork, rollback, restore,
// delete) go through [WriteSets].
type Store interface {
	List(ctx context.Context) ([]session.Session, error)
	ListPage(ctx context.Context, afterFavorite bool, afterUpdatedAt int64, afterID string, limit int) ([]session.Session, error)
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	Ensure(ctx context.Context, sess session.Session) (session.Session, error)
	Patch(ctx context.Context, id string, patch session.Patch) (session.Session, error)
}

// InterruptStore is the lifecycle coordinator's read view of open HITL
// interrupt. Consuming an interrupt is part of the run coordinator's atomic
// segment-opening commit; deleting one is part of an atomic write-set
// ([WriteSets.ApplyRollback] / ApplyDelete / ApplyTerminal), not a lone call.
type InterruptStore interface {
	List(ctx context.Context, sessionID string) ([]runs.Pending, error)
	Get(ctx context.Context, runID string) (runs.Pending, bool, error)
}

// TranscriptStore is the lifecycle coordinator's read view of a session's item
// history. The Runs those items belong to come from [RunStore] — one record, one
// owner — and the coordinator reads both inside one storage transaction when it
// needs them to agree.
type TranscriptStore interface {
	List(ctx context.Context, sessionID string) ([]transcript.Item, error)
}

// PlanBoundaries is the lifecycle coordinator's read view of the Plan a Run
// boundary recorded. Rollback and fork both need "the value as of that run", which
// the live projection cannot answer: it keeps the latest value and no history. A
// false ok is "never recorded", not "empty" — see [PlanBoundary]. nil disables it
// (no Plan state to move with a boundary).
type PlanBoundaries interface {
	Boundary(ctx context.Context, runID string) ([]plan.Step, bool, error)
}

// RunStore is the lifecycle coordinator's read view of a session's Runs. Every
// boundary it computes — a rollback target, a fork point, an abandoned park — is
// derived from the Run timeline, so it reads the Runs themselves rather than
// inferring them from the items they produced.
type RunStore interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
}

// WriteSets are the atomic durable write-sets the coordinator commits through the
// persistence boundary (§8.1): each applies its whole multi-store mutation in ONE
// transaction, so the coordinator never stitches a transaction across table-CRUD
// calls with the boundary hidden in the context (§8.4). The application decides
// the plan; the implementation executes it atomically, enriching nothing.
type WriteSets interface {
	// ApplyFork branches a child session off the plan's parent, seeds its chat log
	// with the resolved history prefix and its Plan with the boundary value,
	// and titles it — atomically — returning the created child.
	ApplyFork(ctx context.Context, plan ForkPlan) (session.Session, error)
	// ApplyRollback truncates the chat log to the boundary, drops each
	// past-boundary run, republishes the boundary's plan projection, and
	// terminalizes an abandoned parked run — atomically. Delegated work is
	// already represented by child Runs in the same session.
	ApplyRollback(ctx context.Context, plan RollbackPlan) error
	// ApplyRestore recreates a session under its original id and replaces its
	// whole history (clear old session-owned projections + seed decoded
	// messages/runs/items) — atomically.
	ApplyRestore(ctx context.Context, plan RestorePlan) error
	// ApplyDelete removes all durable state for the addressed session —
	// transcript, chat log, plan, session approval rules, interrupts, admission
	// rows, and the session row — atomically.
	ApplyDelete(ctx context.Context, plan DeletePlan) error
	// ApplyTerminal ends a parked run: it persists the terminal transcript
	// projection, drops the open interrupt, and closes admission — atomically.
	ApplyTerminal(ctx context.Context, plan TerminalPlan) error
}

// SnapshotReader returns the canonical session aggregate from one storage
// transaction.
type SnapshotReader interface {
	ReadSnapshot(ctx context.Context, sessionID string) (Snapshot, error)
}

// Forgetter releases process-local executor state after a session is
// removed from durable storage.
type Forgetter interface {
	ForgetSession(sessionID string)
}

// Snapshot is one coherent, canonical Session aggregate read used by use cases
// that must reason across multiple persisted projections.
type Snapshot struct {
	Session     session.Session
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []run.Run
	ToolResults []toolresult.Blob
	// Plan is the session-scoped Plan as a semantic VALUE — items only, no
	// revision and no update time. Those are this runtime's ordering tokens: a
	// snapshot that carried them could hand a restored value a position in the
	// revision space that the restoring runtime never issued, and a client would
	// then ignore the newer value as stale.
	Plan []plan.Step
}

// WorkspaceCheckpoints is the coordinator's view of a session's working-tree
// checkpoint store (shadow git): Restore resets the tree to a run-boundary
// snapshot — the filesystem half of a file rollback (§8.5) — and DropSession
// discards a deleted session's snapshots as the last step of the delete cascade.
// Restore is reentrant (a git reset to an already-restored tree is a no-op), so
// the recoverable operation can re-drive it at boot. A disabled store or missing
// snapshot surfaces as [ErrCheckpointUnavailable]; a reset that may have changed
// only part of the tree surfaces as [ErrCheckpointRestoreIncomplete]. The
// Implementations translate storage failures into these use-case errors.
type WorkspaceCheckpoints interface {
	Restore(ctx context.Context, sessionID, cwd, runID string) error
	// DropSession removes a session's checkpoint history after the durable
	// aggregate deletion. A failure cannot roll the transaction back, but it is
	// returned to the caller and never disguised as successful cleanup.
	DropSession(sessionID string) error
}

// SandboxDiscarder destroys a session's isolated sandbox working copy — the
// scratch-tree half of the delete/rollback/restore cascades, run POST-COMMIT
// (a filesystem RemoveAll, never inside the durable transaction) alongside the
// checkpoint drop. nil disables it (no-op — the session was not isolated or
// isolation is off). A failure cannot roll the durable delete back, so it is
// returned as cleanup, never disguised as success.
type SandboxDiscarder interface {
	Discard(sessionID string) error
}

// GoalMutationGuard serializes a session write-set with Goal lifecycle
// commands. It owns the complete commit boundary: afterCommit runs exactly
// once after commit succeeds, even when quiescing an affected Goal fails.
// nil disables it (Goal mode off).
type GoalMutationGuard interface {
	WithSessionMutation(
		ctx context.Context,
		sessionIDs []string,
		commit func(context.Context) error,
		afterCommit func(context.Context) error,
	) error
}

// WorkspaceMutations is the recoverable operation log for file rollbacks (§8.5):
// a Git reset is not atomic across paths, and the optional durable-history cut
// cannot share its transaction. Record logs the intent before the tree is
// touched, Complete clears it once all requested effects commit, and ListPending
// returns interrupted operations for boot recovery. Its store commits writes
// independently (not joined to any rollback
// transaction) — the log is precisely the marker that the two resources change
// out of transaction. nil disables the log (rollback runs without a recovery
// record, degrading to best-effort).
type WorkspaceMutations interface {
	Record(ctx context.Context, m WorkspaceMutation) error
	Complete(ctx context.Context, sessionID string) error
	ListPending(ctx context.Context) ([]WorkspaceMutation, error)
}

// ExecutionReleaser is the engine-neutral resource lifecycle slice the Session
// coordinator uses when delete or rollback abandons parked executions. Product
// resume, cancel, and steer orchestration belongs to application/runs.
type ExecutionReleaser interface {
	Release(ctx context.Context, ref runs.ExecutorRef) error
}

// Coordinator executes session/run lifecycle write-sets across the domain
// stores, coordinates single-writer run admission (the per-session and
// per-working-tree slots), and tears down the executor behind an abandoned Run.
// Stateless beyond its collaborators and the in-process admission gates;
// safe to share.
type Coordinator struct {
	sessions          Store
	interrupts        InterruptStore
	transcript        TranscriptStore
	runs              RunStore
	boundaries        PlanBoundaries
	snapshots         SnapshotReader
	writes            WriteSets
	forgetter         Forgetter
	executionReleaser ExecutionReleaser
	paths             CWDResolver
	defaultModel      string
	// checkpoints resets the working tree to a run-boundary checkpoint for a file
	// rollback and drops a deleted session's snapshots; nil disables both (file
	// restore is rejected as [ErrCheckpointUnavailable], drop no-ops).
	checkpoints WorkspaceCheckpoints
	// sandbox destroys a deleted/rolled-back/restored session's isolated working
	// copy, post-commit alongside the checkpoint drop; nil disables it.
	sandbox SandboxDiscarder
	// goals serializes a session write-set with Goal lifecycle commands and
	// quiesces its loop after a successful commit; nil disables Goal mode.
	goals GoalMutationGuard
	// mutations is the §8.5 recoverable operation log guarding a file+history
	// rollback across the working tree and the durable history; nil disables it.
	mutations WorkspaceMutations
	// admissions is shared with Runs and owns the process-local session and
	// working-tree facts.
	admissions Admissions
	// changed tells clients a committed session mutation moved something they hold.
	// Every notice is published from a post-commit boundary, never from the commit
	// itself: a signal for a transaction that then rolled back would send every
	// listener to re-read state that never changed. nil publishes nothing.
	changed change.Publish
}

// Dependencies is the collaborator set [New] wires into a Coordinator. Durable
// mutations cross one cohesive WriteSets transaction; reads and process-local
// cleanup are independent ports rather than accessor methods on a store bag.
type Dependencies struct {
	Sessions          Store
	Interrupts        InterruptStore
	Transcript        TranscriptStore
	Runs              RunStore
	Boundaries        PlanBoundaries
	Snapshots         SnapshotReader
	Writes            WriteSets
	Forgetter         Forgetter
	ExecutionReleaser ExecutionReleaser
	Paths             CWDResolver
	DefaultModel      string
	Checkpoints       WorkspaceCheckpoints
	Sandbox           SandboxDiscarder
	Goals             GoalMutationGuard
	Mutations         WorkspaceMutations
	Admissions        Admissions
	// Changed publishes post-commit invalidations for the session projections a
	// mutation moved. nil disables them (no runtime change stream wired).
	Changed change.Publish
}

// ErrSessionBusy reports that a session already has an active or parked run.
var ErrSessionBusy = errors.New("sessions: session busy")

// New returns a Coordinator over deps.
func New(deps Dependencies) *Coordinator {
	return &Coordinator{
		sessions:          deps.Sessions,
		interrupts:        deps.Interrupts,
		transcript:        deps.Transcript,
		runs:              deps.Runs,
		boundaries:        deps.Boundaries,
		snapshots:         deps.Snapshots,
		writes:            deps.Writes,
		forgetter:         deps.Forgetter,
		executionReleaser: deps.ExecutionReleaser,
		paths:             deps.Paths,
		defaultModel:      deps.DefaultModel,
		checkpoints:       deps.Checkpoints,
		sandbox:           deps.Sandbox,
		goals:             deps.Goals,
		mutations:         deps.Mutations,
		admissions:        deps.Admissions,
		changed:           deps.Changed,
	}
}

// ClaimWorkingTreeMutation reserves exclusive access to cwd's working tree for a
// destructive mutation such as file rollback.
func (c *Coordinator) ClaimWorkingTreeMutation(cwd string) (WorkingTreeAdmission, bool) {
	if c.admissions == nil {
		return WorkingTreeAdmission{}, false
	}
	release, ok := c.admissions.AcquireWorkingTreeMutation(cwd)
	if !ok {
		return WorkingTreeAdmission{}, false
	}
	return heldWorkingTreeAdmission(release), true
}
