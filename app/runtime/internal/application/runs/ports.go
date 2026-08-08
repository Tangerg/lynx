package runs

import (
	"context"
	"iter"
	"time"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// The ports this package consumes to run a Segment. They are defined here on the
// consumer side and satisfied structurally by composition.
//
// The application drives execution through implementation-neutral
// [SegmentExecutor] and [ExecutionControl] ports. It observes the
// application-owned [ExecutionFact] family and addresses work through durable
// [ExecutorRef] values.

// ExecutionCanceler tears down a live or parked execution by durable identity. It is a
// shared capability both the pump ([SegmentExecutor]) and the control surface
// ([ExecutionControl]) need; naming it once keeps an implementation from exposing the
// same teardown under two method names.
type ExecutionCanceler interface {
	CancelExecution(ctx context.Context, ref ExecutorRef) error
}

// WaitingSubtreeDisposition is the application decision applied after a
// prepared waiting-tree transaction commits. A surviving external boundary
// keeps the executor parked; removing the final boundary immediately opens the
// already-committed continuation Segment.
type WaitingSubtreeDisposition uint8

const (
	waitingSubtreeDispositionInvalid WaitingSubtreeDisposition = iota
	WaitingSubtreeRemainsInterrupted
	WaitingSubtreeContinues
)

// WaitingSubtreeMutation is the live executor lease attached to a data-only
// prepared cancellation. Commit crosses the executor boundary only after the
// application transaction succeeds; Abort releases its lifecycle claim.
type WaitingSubtreeMutation interface {
	Commit(ctx context.Context, disposition WaitingSubtreeDisposition) error
	Abort()
}

// PreparedWaitingSubtreeCancellation separates immutable application data from
// the live executor lease that can apply it. No persistence behavior crosses the
// application boundary.
type PreparedWaitingSubtreeCancellation struct {
	CanceledProcessIDs []string
	PendingSuspensions []ProcessSuspension
	Checkpoint         ExecutorCheckpoint
	Mutation           WaitingSubtreeMutation
}

// SegmentExecutor is what the run pump needs to observe and cancel the
// execution backing a Run Segment.
type SegmentExecutor interface {
	Events(ctx context.Context, ref ExecutorRef) (iter.Seq[ExecutorEvent], error)
	ExecutionCanceler
}

// SessionLifecycle is the run use cases' narrow view of session persistence,
// open interrupts, the atomic parked-run abandon write-set, and the in-process
// working-tree admission gate. It is implemented by application/sessions; runs
// owns the ordering in which these capabilities are used.
type SessionLifecycle interface {
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	PrepareScheduled(ctx context.Context, id, title, cwd string) (session.Session, error)
	ActiveRun(ctx context.Context, sessionID string) (transcript.Run, bool, error)
	ListOpenInterrupts(ctx context.Context, sessionID string) ([]Pending, error)
	LookupOpenInterrupt(ctx context.Context, runID string) (Pending, bool, error)
	ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) (transcript.Run, error)
	ApplyRunLost(ctx context.Context, sessionID, runID string, finishedAt time.Time) error
}

// RunProjection is the run use cases' durable Run read. Run answers point
// identity; RunTree resolves any root or child Run to its complete
// root/descendant aggregate in one read, so a tree-scoped command does not first
// race a target lookup against a second tree lookup. The projection returns
// facts, not cancellation policy: application/domain code owns topology
// validation and subtree meaning.
type RunProjection interface {
	Run(ctx context.Context, runID string) (transcript.Run, bool, error)
	RunTree(ctx context.Context, runID string) ([]transcript.Run, error)
}

// ItemProjection resolves the exact transcript Item a command plans to replace.
// Waiting child cancellation uses it to freeze the parent spawning Item before
// either executor or persistence side effects begin.
type ItemProjection interface {
	Item(ctx context.Context, itemID string) (transcript.Item, bool, error)
}

// StartExecution is the semantic command the run use case sends after resolving
// the Session and its working directory.
type StartExecution struct {
	SessionID string
	Message   string
	Media     []*media.Media
	// CWD is the execution directory — the sandbox copy for an isolated
	// run, else the session's project directory. The durable run record keeps the
	// project directory; only the executor sees the copy.
	CWD string
	// WorkspaceCWD is the persistent Session workspace. It differs from CWD only
	// for an isolated Run and is used by product capabilities that must outlive
	// the scratch copy.
	WorkspaceCWD   string
	Isolated       bool
	ModelSelection modelref.Selection
	Limits         run.RunLimits
	Options        *corechat.Options
	InterruptKinds []interrupt.Kind
	// ChildRunAdmissionEnabled installs the executor-to-application admission
	// handshake for AgentTool children. It is deliberately explicit and defaults
	// off; the Run's frozen application policy is its sole production source.
	ChildRunAdmissionEnabled bool
	// GoalLeaseID stamps a Goal-mode autonomous run with its goal incarnation
	// so a terminal outcome report only signals that Goal; empty for ordinary Runs.
	GoalLeaseID string
}

// RehydrateExecution describes rebuilding a parked execution from its durable
// checkpoint after executor-local state was lost.
type RehydrateExecution struct {
	SessionID  string
	ExecutorID string
	ProcessID  string
	RootRunID  string
	// ChildRuns restores the application identities of already-admitted child
	// executor members so lifecycle hooks never need executor topology.
	ChildRuns                []ChildRunBinding
	ModelSelection           modelref.Selection
	CWD                      string
	WorkspaceCWD             string
	Isolated                 bool
	GoalLeaseID              string
	Limits                   run.RunLimits
	ChildRunAdmissionEnabled bool
}

// IsolationProvider resolves the sandbox working-copy directory an isolated
// session's Run executes in, creating it from the project directory on first
// use. nil means isolation is unavailable and an isolated start is refused.
type IsolationProvider interface {
	Workspace(ctx context.Context, sessionID, projectRoot string) (string, error)
}

// ExecutionControl is the run use cases' implementation-neutral control
// surface. Validation happens before Session creation.
type ExecutionControl interface {
	ValidateStart(start StartExecution) error
	PrepareStart(ctx context.Context, req StartExecution) (ExecutorRef, error)
	Activate(ctx context.Context, ref ExecutorRef) error
	Prepare(ctx context.Context, ref ExecutorRef) (ExecutorRef, error)
	Resume(ctx context.Context, ref ExecutorRef, answers []SuspensionAnswer, interruptKinds []interrupt.Kind) error
	Rehydrate(ctx context.Context, req RehydrateExecution) (ExecutorRef, error)
	ExecutionCanceler
	// CancelSubtree terminates exactly the addressed executor process and its
	// descendants while the owning execution continues. processID is an opaque
	// identity previously observed through ExecutorSource; the implementation must
	// prove that it belongs to ref before crossing the executor side effect.
	CancelSubtree(ctx context.Context, ref ExecutorRef, processID string) error
	// PrepareWaitingSubtreeCancellation claims a parked execution and computes an
	// executor transition plan without changing live execution or retaining an
	// executor lock. The returned capability owns the application claim until Commit or
	// Abort.
	PrepareWaitingSubtreeCancellation(
		ctx context.Context,
		ref ExecutorRef,
		processID string,
	) (PreparedWaitingSubtreeCancellation, error)
	Steer(ctx context.Context, ref ExecutorRef, input []transcript.ContentBlock) error
}
