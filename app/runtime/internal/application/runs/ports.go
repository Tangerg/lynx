package runs

import (
	"context"
	"iter"
	"time"

	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app/runtime/internal/application/admission"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// The ports this package consumes to run a segment. They are defined here — on
// the consumer side — and satisfied structurally by the runtime / delivery /
// adapter implementations the composition root injects.
//
// The application drives execution through engine-neutral [SegmentExecutor]
// and [TurnControl] ports: it observes the application-owned [EngineEvent] sum
// type and addresses turns through durable [execution.TurnRef] values, so neither
// lifecycle nor Delivery depends on agent-SDK handle types.

// TurnCanceler tears down a live or parked turn by its durable identity. It is a
// shared capability both the pump ([SegmentExecutor]) and the control surface
// ([TurnControl]) need; naming it once keeps the adapter from implementing the
// same teardown under two method names.
type TurnCanceler interface {
	CancelTurn(ctx context.Context, ref execution.TurnRef) error
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
// application transaction succeeds; Abort releases the App lifecycle claim.
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
	Checkpoint         execution.ExecutorCheckpoint
	Mutation           WaitingSubtreeMutation
}

// SegmentExecutor is what the run pump needs to observe and cancel the agent
// turn backing a run segment. The concrete agent-execution adapter implements it.
type SegmentExecutor interface {
	TurnEvents(ctx context.Context, ref execution.TurnRef) (iter.Seq[ExecutorEvent], error)
	TurnCanceler
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
	ListOpenInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	GetOpenInterrupt(ctx context.Context, runID string) (interrupts.Pending, bool, error)
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

// StartTurn is the protocol-neutral command the run use case sends to the
// executor adapter after resolving the session and its working directory.
type StartTurn struct {
	SessionID string
	Message   string
	Media     []*media.Media
	// Cwd is the turn's EXECUTION directory — the sandbox copy for an isolated
	// run, else the session's project directory. The durable run record keeps the
	// project directory; only the executor sees the copy.
	Cwd            string
	Isolated       bool
	ModelSelection modelref.Selection
	Limits         execution.RunLimits
	Options        *corechat.Options
	InterruptKinds []execution.InterruptKind
	// ChildRunAdmissionEnabled installs the executor-to-application admission
	// handshake for AgentTool children. It is deliberately explicit and defaults
	// off; the Run's frozen application policy is its sole production source.
	ChildRunAdmissionEnabled bool
	// GoalLeaseID stamps a Goal-mode autonomous run with its goal incarnation
	// so report_goal_outcome only signals that Goal; empty for ordinary Runs.
	GoalLeaseID string
}

// RehydrateTurn describes rebuilding a parked executor turn from its durable
// checkpoint after executor-local state was lost.
type RehydrateTurn struct {
	SessionID string
	TurnID    string
	ProcessID string
	RootRunID string
	// ChildRuns restores the App identities of already-admitted child executor
	// members so product lifecycle hooks never need Framework topology.
	ChildRuns                []ChildRunBinding
	ModelSelection           modelref.Selection
	Cwd                      string
	Isolated                 bool
	GoalLeaseID              string
	Limits                   execution.RunLimits
	ChildRunAdmissionEnabled bool
}

// IsolationProvider resolves the sandbox working-copy directory an isolated
// session's run executes in, creating it from the project directory on first
// use. Implemented by the isolation adapter; nil when isolation is not
// configured (then an isolated session's start is refused).
type IsolationProvider interface {
	Workspace(ctx context.Context, sessionID, projectRoot string) (string, error)
}

// TurnControl is the run use cases' engine-neutral control surface. Validation
// happens before session creation; the adapter translates durable references
// into its concrete turn identity.
type TurnControl interface {
	ValidateStart(StartTurn) error
	PrepareStart(ctx context.Context, req StartTurn) (execution.TurnRef, error)
	Activate(ctx context.Context, ref execution.TurnRef) error
	Prepare(ctx context.Context, ref execution.TurnRef) (execution.TurnRef, error)
	Resume(ctx context.Context, ref execution.TurnRef, answers []interrupts.SuspensionAnswer, interruptKinds []execution.InterruptKind) error
	Rehydrate(ctx context.Context, req RehydrateTurn) (execution.TurnRef, error)
	TurnCanceler
	// CancelSubtree terminates exactly the addressed executor process and its
	// descendants while the owning turn continues. processID is an opaque
	// identity previously observed through ExecutorSource; the adapter must
	// prove that it belongs to ref before crossing the executor side effect.
	CancelSubtree(ctx context.Context, ref execution.TurnRef, processID string) error
	// PrepareWaitingSubtreeCancellation claims a parked App turn and computes an
	// executor transition plan without changing live execution or retaining an
	// executor lock. The returned capability owns the App claim until Commit or
	// Abort.
	PrepareWaitingSubtreeCancellation(
		ctx context.Context,
		ref execution.TurnRef,
		processID string,
	) (PreparedWaitingSubtreeCancellation, error)
	Steer(ctx context.Context, ref execution.TurnRef, input []transcript.ContentBlock) error
}

// Nudge is a non-durable live workspace change notification the pump forwards to
// subscribers after a file-mutating tool item — deliberately path-only, so the
// wire WorkspaceEvent shape stays in the delivery adapter.
type Nudge struct {
	Cwd   string
	Paths []string
}

// Effects is the durable side of a run segment. CommitEvent atomically persists
// one event's projections and its run-state transition (§8.3/§8.4) in a single
// transaction before publishing the corresponding event, so subscribers never
// observe state the durable stores cannot yet serve. Nudge is a non-durable live
// workspace notification. Finish synchronously establishes the checkpoint
// boundary while run admission is still held, then may generate the title off
// the live path. The adapter/runsegment.Effects satisfies it.
type Effects interface {
	// CommitOpening atomically persists every durable projection that leads a
	// segment. For a fresh Run it also admits the Run; for a continuation it
	// consumes the open interrupt and resumes the existing Run. Start does not
	// return until this succeeds.
	CommitOpening(ctx context.Context, opening OpeningCommit) error
	// CommitEvent applies commit's set parts (interrupt open, transcript item/run,
	// run-state transition) in one transaction. Every durable commit completes
	// before publication; any error aborts the segment.
	CommitEvent(ctx context.Context, commit EventCommit) error
	// CommitTreeBarrier atomically writes the captured executor checkpoint, the
	// one root-owned pending set, and every active Run suspension in the tree.
	CommitTreeBarrier(ctx context.Context, barrier TreeBarrierCommit) error
	// CommitWaitingSubtreeCancellation atomically persists one prepared process
	// checkpoint replacement and every application fact that makes the same
	// subtree cancellation durable.
	CommitWaitingSubtreeCancellation(
		ctx context.Context,
		commit WaitingSubtreeCancellationCommit,
	) (WaitingSubtreeCancellationResult, error)
	// Nudge publishes a non-durable live workspace change to subscribers.
	Nudge(cwd string, paths []string)
	// Finish establishes the terminal checkpoint before returning, then starts
	// non-boundary title maintenance off the live path. A parked run is resumable,
	// not a boundary, so Finish no-ops for it (fin.Parked). Accepted background
	// title failures remain observable on a terminal-maintenance span.
	Finish(ctx context.Context, fin Finish) error
}

type ItemReplacement struct {
	Expected    transcript.Item
	Replacement transcript.Item
}

// WaitingSubtreeCancellationCommit is the complete immutable write-set for one
// child cancellation performed while the Run tree is Interrupted.
type WaitingSubtreeCancellationCommit struct {
	RootRunID        string
	TargetRunID      string
	SessionID        string
	RootRun          transcript.Run
	ExpectedPending  interrupts.Pending
	RemainingPending *interrupts.Pending
	Checkpoint       execution.ExecutorCheckpoint
	TerminalRuns     []transcript.Run
	TerminalItems    []ItemReplacement
	ParentItem       ItemReplacement
	Resume           *execution.TreeResumeDraft
	OpeningEvents    []EventCommit
}

type WaitingSubtreeCancellationResult struct {
	TargetRun transcript.Run
	RootRun   transcript.Run
}

// OpeningCommit is the single atomic acceptance commit for a segment. Exactly
// one of Admit or Resume is set. Events contains the durable transcript
// canonical records produced by the reducer; applying admission and all opening
// facts in one transaction prevents a successful start response from naming a
// Run whose opening record does not exist.
type OpeningCommit struct {
	Admit            *execution.RunDraft
	Resume           *execution.TreeResumeDraft
	ScheduledSession *session.Session
	SessionModel     *SessionModelUpdate
	ScheduleFiring   string
	Events           []EventCommit
}

// SessionModelUpdate is the session fact established by accepting a fresh run
// with an explicit model selection. It is committed with that run's admission,
// so an opening rejection cannot leave the session advertising a model that
// never successfully started.
type SessionModelUpdate struct {
	SessionID string
	Model     string
}

// Finish describes the terminal run-boundary maintenance the Effects port runs
// after the live stream has closed. The pump builds it from run-boundary facts
// it already owns; a parked run is resumable, not a boundary.
type Finish struct {
	SessionID       string
	RunID           string
	Cwd             string
	Parked          bool
	OpeningUserText string
}

// segmentSpec is the already-prepared input to the package's segment
// supervisor. User-visible Start/Resume use cases build it; no outer layer may
// call the supervisor directly.
type segmentSpec struct {
	// RunID is the STABLE logical run id — minted once at the run's first segment
	// and carried unchanged through every resume, so admission / journal / durable
	// records key on the run, not the segment.
	RunID string
	// SegmentID identifies THIS streamed segment (a fresh one per runs.start /
	// runs.resume). The wire event envelope carries it so a client scopes its
	// stream-tree + reconnect-replay dedup to the segment.
	SegmentID string
	SessionID string
	Cwd       string
	// TurnID is the executor's durable turn identity recorded on the live run.
	TurnID           string
	ModelSelection   modelref.Selection
	GoalLeaseID      string
	ScheduledSession *session.Session
	SessionModel     *SessionModelUpdate
	ScheduleFiring   string
	CreatedAt        time.Time
	OpeningUserText  string
	Input            []transcript.ContentBlock
	// Limits is the allowance the Run is admitted under, frozen for the whole Run
	// — a continuation carries the first segment's caps rather than renegotiating.
	Limits execution.RunLimits
	// ProtocolProfile is the protocol contract negotiated for a FRESH Run. A
	// continuation leaves it zero and reads the Run's own from Continuation: the
	// profile is the Run's, and a resume request has no say in it.
	ProtocolProfile execution.RunProtocolProfile
	// Continuation is present only when fresh Segments reopen a parked executor
	// tree. It is independent from an open human interrupt: a host-settled
	// checkpoint can continue after its final external boundary was removed.
	Continuation *treeContinuation
	// admission is the pre-commit reservation Start or Resume transfers to the
	// live run immediately after its durable opening commit succeeds.
	admission *admission.RunAdmission
	// Activate crosses the executor's side-effect boundary after this segment's
	// opening write-set commits. For a fresh run it starts model/tool execution;
	// for a continuation it delivers the already-accepted user decision. Tests
	// may leave it nil when exercising an already-active synthetic executor.
	Activate func(context.Context) error
	// CommitOpening overrides the ordinary segment-opening write-set only when a
	// larger application transaction already owns that opening (waiting subtree
	// cancellation). It receives the exact reducer-built opening and must commit
	// all of it atomically before returning.
	CommitOpening func(context.Context, OpeningCommit) error
}

func (s segmentSpec) turnRef() execution.TurnRef {
	return execution.TurnRef{SessionID: s.SessionID, TurnID: s.TurnID}
}

// priorMetrics is what the Run had already consumed when this segment opened: a
// first segment starts from nothing, a continuation from what the park recorded.
func (s segmentSpec) priorMetrics() transcript.RunMetrics {
	if s.Continuation == nil {
		return transcript.RunMetrics{}
	}
	root, _ := s.Continuation.root()
	return root.Metrics
}

// effectiveLimits is the allowance in force. A continuation does not take limits
// from its resume request — the Run was admitted under one policy and keeps it,
// so answering an interrupt cannot quietly raise a budget.
func (s segmentSpec) effectiveLimits() execution.RunLimits {
	if s.Continuation == nil {
		return s.Limits
	}
	root, _ := s.Continuation.root()
	return root.Limits
}

// effectiveProfile is the protocol contract in force, by the same rule: the
// profile belongs to the Run, so a continuation reports the one the park recorded
// and never the declaration of the request that resumed it.
func (s segmentSpec) effectiveProfile() execution.RunProtocolProfile {
	if s.Continuation == nil {
		return s.ProtocolProfile
	}
	return s.Continuation.profile
}
