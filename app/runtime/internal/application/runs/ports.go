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
// after Session selection resolution but before any execution staging, and may
// inspect only immutable input and frozen policy fields. StageRoot receives the resolved Session/workspace fields and must not cross
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

// RunningRootCancellationRequester submits an accepted product cancellation
// intent to the live executor before the Application stops observation. A nil
// result only confirms command admission; it neither commits the product outcome
// nor releases resources, and in-flight effects may settle before the next safe
// boundary.
type RunningRootCancellationRequester interface {
	RequestRootCancellation(ctx context.Context, ref ExecutorRef, reason string) error
}

// ConversationReader supplies the validated Host conversation used only to
// seed a fresh executor working context. Once execution starts, restoration is
// self-contained in the executor checkpoint and never re-reads Conversation.
type ConversationReader interface {
	Read(ctx context.Context, sessionID string) ([]corechat.Message, error)
}

// WorkingContextComposer turns one Host conversation seed into the complete,
// provider-neutral context for a fresh root execution. It owns prompt-layer
// reads and lifecycle-hook preparation but neither reads Conversation nor
// starts an executor. Implementations must return a self-contained snapshot;
// the executor never calls back into this port while running or restoring.
type WorkingContextComposer interface {
	ComposeWorkingContext(
		ctx context.Context,
		input WorkingContextInput,
	) ([]corechat.Message, error)
}

// WorkingContextInput is the exact fresh-root context composition input. Seed
// contains the canonical Host conversation followed by CurrentUserMessage.
// PromptText is the user-authored text supplied to prompt hooks and relevance
// recall; it is kept distinct from media and from the historical seed.
type WorkingContextInput struct {
	SessionID  string
	CWD        string
	PromptText string
	Seed       []corechat.Message
}

// WaitingExecutionContinuer stages an exact live or restored waiting tree
// without advancing it, then submits its already-validated semantic answers
// only after the continuation Segment is durable. The opaque checkpoint is
// supplied by the Application; implementations must never reread Conversation
// or privately query application persistence.
type WaitingExecutionContinuer interface {
	StageContinuation(ctx context.Context, continuation WaitingContinuation) (ExecutorRef, error)
	BeginContinuation(
		ctx context.Context,
		ref ExecutorRef,
		answers []InterruptAnswer,
		allowedInterrupts []interrupt.Kind,
	) error
}

// WaitingExecutionRestorer reconstructs one exact committed waiting tree
// without claiming or advancing it. It is used after the previous live owner is
// known obsolete; the supplied Application value and opaque checkpoint are the
// only recovery source.
type WaitingExecutionRestorer interface {
	RestoreWaitingExecution(
		ctx context.Context,
		continuation WaitingContinuation,
	) (ExecutorRef, error)
}

// RunningExecutionSteerer submits user content to the addressed live execution.
// The implementation must consume it only at its documented safe boundary and
// reject an execution that is already waiting or terminal.
type RunningExecutionSteerer interface {
	SubmitSteer(ctx context.Context, ref ExecutorRef, input []transcript.ContentBlock) error
}

// RunningSubtreeCanceler submits the Application's already-validated product
// cancellation intent to one live child executor subtree. It does not decide or
// persist the Run outcome; the ordered executor facts remain authoritative.
type RunningSubtreeCanceler interface {
	CancelRunningSubtree(
		ctx context.Context,
		ref ExecutorRef,
		memberID string,
		reason string,
	) error
}

// WaitingSubtreeCancellationPreparer freezes one exact waiting executor tree
// and returns its prospective product projection plus a one-shot change. The
// implementation may use a matching live tree or restore only the supplied
// opaque checkpoint; it never reads Application persistence.
type WaitingSubtreeCancellationPreparer interface {
	PrepareWaitingSubtreeCancellation(
		ctx context.Context,
		request WaitingSubtreeCancellationRequest,
	) (PreparedWaitingSubtreeCancellation, error)
}

// WaitingSubtreeDisposition is the application decision applied after a
// prepared waiting-tree transaction commits. A surviving external boundary
// keeps the executor parked; removing the final boundary immediately opens the
// already-committed continuation Segment.
type WaitingSubtreeDisposition uint8

const (
	// WaitingSubtreeStaysWaiting keeps the surviving external boundaries parked.
	WaitingSubtreeStaysWaiting WaitingSubtreeDisposition = iota + 1
	// WaitingSubtreeResumesRunning resumes the paused surviving executor members.
	WaitingSubtreeResumesRunning
)

// WaitingSubtreeChange is the one-shot executor capability attached to a
// prepared cancellation. Apply installs the committed tree disposition after
// the application transaction succeeds. Continue advances a disposition that
// removed the final external boundary; it is a distinct activation phase so an
// execution failure cannot be mistaken for a failed state installation.
// Discard idempotently releases a change that was not applied.
type WaitingSubtreeChange interface {
	Apply(disposition WaitingSubtreeDisposition) error
	Continue(ctx context.Context) error
	Discard() error
}

// PreparedWaitingSubtreeCancellation separates immutable application data from
// the one-shot executor capability that can apply it. No persistence behavior
// crosses the application boundary.
type PreparedWaitingSubtreeCancellation struct {
	// CanceledMemberIDs names the exact product members projected as canceled.
	CanceledMemberIDs []string
	// PausedMemberIDs names surviving members held before child-outcome consumption.
	PausedMemberIDs []string
	// PendingInterruptions contains the surviving external waiting boundaries.
	PendingInterruptions []MemberInterruption
	// Checkpoint is the opaque complete-tree state that Change.Apply installs.
	Checkpoint ExecutorCheckpoint
	// Change owns the frozen executor source until Apply or Discard resolves it.
	Change WaitingSubtreeChange
}

// SessionReader resolves the product Session a Run belongs to.
type SessionReader interface {
	Get(ctx context.Context, id string) (session.Session, error)
}

// SessionCreator owns the two Session creation paths consumed by Run start.
// PrepareScheduled returns the caller-identified Session and, only when it does
// not already exist, the exact initial aggregate owned by the opening write-set.
type SessionCreator interface {
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	PrepareScheduled(
		ctx context.Context,
		id, title, cwd string,
		selection modelref.Selection,
	) (session.Session, *session.Session, error)
}

// ActiveRunReader reports the Session's current non-terminal Run for admission.
type ActiveRunReader interface {
	ActiveRun(ctx context.Context, sessionID string) (run.Run, bool, error)
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
	ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) (run.Run, error)
	ApplyRunLost(ctx context.Context, sessionID, runID string, finishedAt time.Time) error
	ApplyClaimedRunLost(ctx context.Context, pending Pending, finishedAt time.Time) error
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
// identity; Tree resolves any root or child Run to its complete
// root/descendant aggregate in one read, so a tree-scoped command does not first
// race a target lookup against a second tree lookup. The projection returns
// facts, not cancellation policy: application/domain code owns topology
// validation and subtree meaning.
type RunProjection interface {
	Run(ctx context.Context, runID string) (run.Run, bool, error)
	Tree(ctx context.Context, runID string) ([]run.Run, error)
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
	Limits         run.Limits
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
	// GoalIncarnationID stamps a Goal-mode autonomous run with its goal incarnation
	// so a terminal outcome report only signals that Goal; empty for ordinary Runs.
	GoalIncarnationID string
}

// WaitingMember is the minimum durable Application state an executor needs to
// rebind one surviving member after restoring an opaque tree. Product lineage
// remains independent of executor topology and is cross-checked at the executor
// boundary.
type WaitingMember struct {
	RunID           string
	MemberID        string
	ParentRunID     string
	SpawnedByItemID string
	ModelSelection  modelref.Selection
	Metrics         run.Metrics
}

// WaitingContinuation is the complete Application-owned input for staging one
// parked tree. Checkpoint payload interpretation remains executor-private;
// Members carries only surviving product identities and cumulative accounting.
type WaitingContinuation struct {
	SessionID                string
	ExecutorID               string
	RootRunID                string
	Members                  []WaitingMember
	Checkpoint               ExecutorCheckpoint
	Capabilities             run.Capabilities
	ChildRunAdmissionEnabled bool
}

// IsolationProvider resolves the sandbox working-copy directory an isolated
// session's Run executes in, creating it from the project directory on first
// use. nil means isolation is unavailable and an isolated start is refused.
type IsolationProvider interface {
	Workspace(ctx context.Context, sessionID, projectRoot string) (string, error)
}
