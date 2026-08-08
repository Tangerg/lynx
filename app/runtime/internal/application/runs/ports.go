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
// consumer side and satisfied structurally by composition. No interface mirrors
// an executor implementation: each one names the single application use case
// that consumes it.

// RootExecutionStarter validates execution-specific input, stages a new root,
// then begins it after the opening write-set is durable. ValidateRootStart runs
// before Session resolution and may inspect only input and frozen policy fields.
// StageRoot receives the resolved Session/workspace fields and must not cross
// the model/tool side-effect boundary. BeginRoot runs only after admission commits.
type RootExecutionStarter interface {
	ValidateRootStart(start RootExecutionStart) error
	StageRoot(ctx context.Context, start RootExecutionStart) (ExecutorRef, error)
	BeginRoot(ctx context.Context, ref ExecutorRef) error
}

// ExecutionObserver attaches the application Run pump to one ordered stream of
// application-owned executor facts. Observation does not advance executor state.
type ExecutionObserver interface {
	Observe(ctx context.Context, ref ExecutorRef) (iter.Seq[ExecutorEvent], error)
}

// ExecutionReleaser tears down an unadmitted, terminal, lost, or otherwise
// invalid executor tree. It is deliberately not called Cancel: the application
// Cancel use case decides and durably records the product outcome; this port only
// releases executor-owned resources after that decision or after failed staging.
type ExecutionReleaser interface {
	Release(ctx context.Context, ref ExecutorRef) error
}

// ConversationReader supplies the validated Host conversation used only to
// seed a fresh executor working context. Once execution starts, restoration is
// self-contained in the executor checkpoint and never re-reads Conversation.
type ConversationReader interface {
	Read(ctx context.Context, sessionID string) ([]corechat.Message, error)
}

// ContinuationExecutor is the temporary old-executor boundary consumed by the
// waiting use case. P6 replaces its exact shape from the real Agent2 consumer;
// keeping it separate prevents those provisional operations from polluting the
// P3 root contract.
type ContinuationExecutor interface {
	ClaimWaiting(ctx context.Context, ref ExecutorRef) (ExecutorRef, error)
	ResumeWaiting(ctx context.Context, ref ExecutorRef, answers []SuspensionAnswer, interruptKinds []interrupt.Kind) error
	RestoreWaiting(ctx context.Context, req RehydrateExecution) (ExecutorRef, error)
}

// ExecutionSteerer is the temporary semantic steer boundary. P6 will derive its
// final shape and safe-boundary behavior from the Agent2 consumer.
type ExecutionSteerer interface {
	Steer(ctx context.Context, ref ExecutorRef, input []transcript.ContentBlock) error
}

// RunningSubtreeCanceler is consumed only by child Run cancellation. P7 owns its
// Agent2 replacement.
type RunningSubtreeCanceler interface {
	CancelRunningSubtree(ctx context.Context, ref ExecutorRef, memberID string) error
}

// WaitingSubtreeCancellationPreparer is the provisional old-executor capability
// consumed by waiting child cancellation. Its concrete contract is replaced in
// P7 after the required Agent2 prepared-change capability exists.
type WaitingSubtreeCancellationPreparer interface {
	PrepareWaitingSubtreeCancellation(
		ctx context.Context,
		ref ExecutorRef,
		memberID string,
	) (PreparedWaitingSubtreeCancellation, error)
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
	CanceledMemberIDs    []string
	PendingInterruptions []MemberInterruption
	Checkpoint           ExecutorCheckpoint
	Mutation             WaitingSubtreeMutation
}

// SessionReader resolves the product Session a Run belongs to.
type SessionReader interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

// SessionCreator owns the two Session creation paths consumed by Run start.
// PrepareScheduled returns the caller-identified Session value whose durable
// creation belongs to the opening write-set.
type SessionCreator interface {
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	PrepareScheduled(ctx context.Context, id, title, cwd string) (session.Session, error)
}

// ActiveRunReader reports the Session's current non-terminal Run for admission.
type ActiveRunReader interface {
	ActiveRun(ctx context.Context, sessionID string) (transcript.Run, bool, error)
}

// PendingInterruptReader projects root-owned waiting boundaries by Session or
// addressed Run without exposing checkpoint storage.
type PendingInterruptReader interface {
	ListOpenInterrupts(ctx context.Context, sessionID string) ([]Pending, error)
	LookupOpenInterrupt(ctx context.Context, runID string) (Pending, bool, error)
}

// RunTerminationCommitter owns the atomic application write-sets for canceling
// a parked Run or declaring its executor state unrecoverable.
type RunTerminationCommitter interface {
	ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) (transcript.Run, error)
	ApplyRunLost(ctx context.Context, sessionID, runID string, finishedAt time.Time) error
}

// SessionPorts groups independently consumed Session-side capabilities for
// composition. Coordinator stores each capability separately.
type SessionPorts struct {
	Reader       SessionReader
	Creator      SessionCreator
	ActiveRuns   ActiveRunReader
	Interrupts   PendingInterruptReader
	Terminations RunTerminationCommitter
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

// RootExecutionStart carries a fresh root's input, frozen policy, and resolved
// execution scope. During pre-admission validation the scope fields are empty;
// the Application fills them before passing the value to StageRoot.
type RootExecutionStart struct {
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
	// WorkingContext is the complete provider-neutral context for a fresh
	// execution, including the current user message as its final entry. It is a
	// seed, not a reference back to mutable Conversation state.
	WorkingContext []corechat.Message
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
	MemberID   string
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
