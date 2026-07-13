package runs

import (
	"context"
	"iter"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// The ports this package consumes to run a segment. They are defined here — on
// the consumer side — and satisfied structurally by the runtime / delivery /
// adapter implementations the composition root injects.
//
// The application drives execution through an engine-neutral [Executor]: it
// observes the application-owned [EngineEvent] sum type and drives an opaque
// [Handle], so neither the lifecycle nor delivery depends on agent-SDK types.

// Handle is the per-segment execution handle the [Executor] returns and the
// application hands back to observe and cancel a live turn. It stays opaque (any)
// on purpose: unlike an [EngineEvent] it carries no lifecycle semantics the
// application acts on — it is an inert token the executor recovers its turn from
// — so typing it would be an empty-interface ceremony.
type Handle = any

// Executor is what the run pump needs to drive, observe, and cancel the agent
// turn backing a run segment. The concrete agent-execution adapter implements it.
type Executor interface {
	TurnEvents(ctx context.Context, handle Handle) (iter.Seq[EngineEvent], error)
	CancelTurn(ctx context.Context, handle Handle) error
}

// Projector turns normalized executor events into projected events: the pump
// feeds it a lead Open(), then each EngineEvent via Translate, and a SynthesizeTerminal()
// when the stream ended without a terminal. It hands back the durable side
// effect to commit and the opaque wire payload to publish.
//
// It is stateful per segment (open-item tracking, step ordinal, error
// classification), so the delivery layer builds one per segment; the application
// never sees the wire shape it produces.
type Projector interface {
	// Open leads every segment (root + continuation) with its segment.started-class
	// events, independent of any executor event.
	Open() []ProjectedEvent
	// Translate projects one executor event into zero or more projected events.
	Translate(ev EngineEvent) []ProjectedEvent
	// SynthesizeTerminal builds the terminal the pump needs when the executor
	// stream ended without one (canceled mid-flight / drained iterator). The
	// Projector decides error-vs-canceled from its own recorded state.
	SynthesizeTerminal() []ProjectedEvent
	// Abort records a projection/commit-failure message so a subsequent
	// SynthesizeTerminal reports the run as errored.
	Abort(msg string)
}

// Projection is the payload family a delivery adapter may place on the run
// journal. The marker removes the unbounded any seam while keeping the concrete
// transport value outside the application package.
type Projection interface {
	RunProjection()
}

// ProjectedEvent is one event on its way to both the durable store and the live
// journal. Payload is opaque to the application — it shuttles the wire event to
// the journal without inspecting it. Commit is the event's atomic durable
// side-effect ([execution.EventCommit], nil when the event persists nothing);
// Nudge is a non-durable live workspace notification (nil when none). The
// application reads neither Commit's records nor Nudge's paths — it forwards them
// to the [Effects] port — but Commit is a domain value (not an opaque adapter
// type) so the run pump can commit it without an agent-SDK dependency.
type ProjectedEvent struct {
	Durable   bool                   // retained for replay (mirrors the wire's IsDurable)
	Terminal  bool                   // ends the run's stream (the segment.finished)
	Interrupt bool                   // terminal that PARKS — takes the commit-before-publish path
	Abort     bool                   // projection failed; cancel the turn and terminalize as error
	Payload   Projection             // delivery-owned payload (a protocol StreamEvent)
	Commit    *execution.EventCommit // atomic durable commit (nil = nothing to persist)
	Nudge     *Nudge                 // non-durable live workspace nudge (nil = none)
}

// Nudge is a non-durable live workspace change notification the pump forwards to
// subscribers after a file-mutating tool item — deliberately path-only, so the
// wire WorkspaceEvent shape stays in the delivery adapter.
type Nudge struct {
	Cwd   string
	Paths []string
}

// SegmentView exposes late-bound segment state the Projector reads at terminal
// time — notably the human cancel reason, which the cancel path sets AFTER the
// segment has started, so it must be read live, not captured at open.
type SegmentView interface {
	CancelReason() string
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
	CommitEvent(ctx context.Context, commit execution.EventCommit) error
	// Nudge publishes a non-durable live workspace change to subscribers.
	Nudge(cwd string, paths []string)
	// Finish runs terminal boundary maintenance off the live path. A parked run is
	// resumable, not a boundary, so Finish no-ops for it (fin.Parked).
	Finish(ctx context.Context, fin Finish)
}

// OpeningCommit is the single atomic acceptance commit for a segment. Exactly
// one of Admit or Resume is set. Events contains the durable transcript
// projections produced by Projector.Open; applying the admission transition and
// all projections in one transaction prevents a successful start response from
// naming a Run whose opening record does not exist.
type OpeningCommit struct {
	Admit  *execution.RunDraft
	Resume *execution.ResumeDraft
	Events []execution.EventCommit
}

// Finish describes the terminal run-boundary maintenance the Effects port runs
// after the live stream has closed. The pump builds it from run-boundary facts
// it already owns; a parked run is resumable, not a boundary.
type Finish struct {
	SessionID       string
	RunID           string
	Parked          bool
	OpeningUserText string
}

// StartSpec is the protocol-free description of a run segment to open. User
// input and resume bindings are deliberately NOT here — they live in the
// Projector the caller supplies, so the application never sees wire content.
type StartSpec struct {
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
	// Activate distinguishes a continuation from a fresh Run and delivers its
	// already-durably-accepted decision to the executor. It is nil for a fresh
	// Run. Start establishes the event owner before calling it; an activation
	// error becomes the segment's streamed error terminal.
	Activate func(context.Context) error
}
