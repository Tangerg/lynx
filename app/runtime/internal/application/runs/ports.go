package runs

import (
	"context"
	"iter"
	"time"

	"github.com/Tangerg/lynx/core/media"
	corechat "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// The ports this package consumes to run a segment. They are defined here — on
// the consumer side — and satisfied structurally by the runtime / delivery /
// adapter implementations the composition root injects.
//
// The application drives execution through engine-neutral [SegmentExecutor]
// and [TurnControl] ports: it observes the application-owned [EngineEvent] sum
// type and drives an opaque [Handle], so neither lifecycle nor Delivery depends
// on agent-SDK types.

// Handle is the per-segment execution handle the turn adapter returns and the
// application hands back to observe and cancel a live turn. It stays opaque (any)
// on purpose: unlike an [EngineEvent] it carries no lifecycle semantics the
// application acts on — it is an inert token the executor recovers its turn from
// — so typing it would be an empty-interface ceremony.
type Handle = any

// SegmentExecutor is what the run pump needs to observe and cancel the agent
// turn backing a run segment. The concrete agent-execution adapter implements it.
type SegmentExecutor interface {
	TurnEvents(ctx context.Context, handle Handle) (iter.Seq[EngineEvent], error)
	CancelTurn(ctx context.Context, handle Handle) error
}

// SessionLifecycle is the run use cases' narrow view of session persistence,
// open interrupts, the atomic parked-run abandon write-set, and the in-process
// working-tree admission gate. It is implemented by application/sessions; runs
// owns the ordering in which these capabilities are used.
type SessionLifecycle interface {
	Get(ctx context.Context, id string) (session.Session, error)
	Create(ctx context.Context, title, cwd string) (session.Session, error)
	SetModel(ctx context.Context, id, model string) error
	ListOpenInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
	GetOpenInterrupt(ctx context.Context, runID string) (interrupts.Pending, bool, error)
	ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, finishedAt time.Time) error
	AcquireWorkingTreeRun(cwd string) (release func(), ok bool)
}

// TurnRef is the engine-neutral durable address of a turn. Delivery never
// rebuilds an adapter handle from it; the driven turn adapter does.
type TurnRef struct {
	SessionID string
	TurnID    string
}

// Turn is the result of starting, preparing, or rehydrating an executor turn.
// The identity is application-visible; Handle remains an opaque token used only
// by the segment executor and turn-control adapter.
type Turn struct {
	SessionID string
	TurnID    string
	Handle    Handle
}

// StartTurn is the protocol-neutral command the run use case sends to the
// executor adapter after resolving the session and its working directory.
type StartTurn struct {
	SessionID      string
	Message        string
	Media          []*media.Media
	Cwd            string
	Provider       string
	Model          string
	MaxBudget      int64
	MaxCostUSD     float64
	MaxSteps       int
	Options        *corechat.Options
	InterruptKinds []string
}

// RehydrateTurn describes rebuilding a parked executor turn from its durable
// process snapshot after process-local state was lost.
type RehydrateTurn struct {
	SessionID string
	TurnID    string
	ProcessID string
	Provider  string
	Model     string
}

// TurnControl is the run use cases' engine-neutral control surface. Validation
// happens before session creation; all opaque-handle recovery remains inside
// the adapter implementation.
type TurnControl interface {
	ValidateStart(StartTurn) error
	Start(ctx context.Context, req StartTurn) (Turn, error)
	Prepare(ctx context.Context, ref TurnRef) (Turn, error)
	Resume(ctx context.Context, turn Turn, resolution interrupts.Resolution, interruptKinds []string) error
	Rehydrate(ctx context.Context, req RehydrateTurn) (Turn, error)
	Cancel(ctx context.Context, ref TurnRef) error
	Steer(ctx context.Context, ref TurnRef, message string) error
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
// workspace notification; Finish runs terminal boundary maintenance (checkpoint
// snapshot, title) off the live path. The adapter/runsegment.Effects satisfies it.
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
	// Nudge publishes a non-durable live workspace change to subscribers.
	Nudge(cwd string, paths []string)
	// Finish runs terminal boundary maintenance off the live path. A parked run is
	// resumable, not a boundary, so Finish no-ops for it (fin.Parked).
	Finish(ctx context.Context, fin Finish)
}

// OpeningCommit is the single atomic acceptance commit for a segment. Exactly
// one of Admit or Resume is set. Events contains the durable transcript
// canonical records produced by the reducer; applying admission and all opening
// facts in one transaction prevents a successful start response from naming a
// Run whose opening record does not exist.
type OpeningCommit struct {
	Admit  *execution.RunDraft
	Resume *execution.ResumeDraft
	Events []EventCommit
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
	// TurnID is the executor's durable turn identity recorded on the live run —
	// supplied alongside the opaque Handle so the application never reaches into
	// the executor's handle representation.
	TurnID          string
	Handle          Handle
	Provider        string
	Model           string
	CreatedAt       time.Time
	OpeningUserText string
	Input           []ContentBlock
	Pending         *interrupts.Pending
	// Activate distinguishes a continuation from a fresh Run and delivers its
	// already-durably-accepted decision to the executor. It is nil for a fresh
	// Run. Start establishes the event owner before calling it; an activation
	// error becomes the segment's streamed error terminal.
	Activate func(context.Context) error
}
