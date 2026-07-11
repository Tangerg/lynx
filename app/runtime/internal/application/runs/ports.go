package runs

import (
	"context"
	"iter"
	"time"
)

// The ports this package consumes to run a segment. They are defined here — on
// the consumer side — and satisfied structurally by the runtime / delivery /
// adapter implementations the composition root injects.
//
// The application drives execution through an engine-neutral [Executor]: both
// the events it observes ([EngineEvent]) and the handle it drives ([Handle]) are
// opaque to it, so the run lifecycle owns no agent-SDK type. The pump is a pure
// conduit — it shuttles each EngineEvent from the Executor to the [Projector]
// without inspecting it, the same opacity the durable side-effect payload
// already has (see ProjectedEvent.Effect). The agentexec adapter produces the
// concrete event + handle; the delivery Projector asserts the event back to the
// shape it emitted.

// EngineEvent is one event the [Executor] emits for a live run segment. It is
// opaque to the application: the pump forwards each to the [Projector] verbatim.
type EngineEvent = any

// Handle is the opaque per-segment execution handle the [Executor] returns and
// the application hands back to it to observe and cancel a live turn. Opaque for
// the same reason as [EngineEvent]: the application owns the run's lifecycle, not
// the executor's internal representation of a live turn.
type Handle = any

// Executor is what the run pump needs to drive, observe, and cancel the agent
// turn backing a run segment. The concrete agent-execution adapter implements it.
type Executor interface {
	TurnEvents(ctx context.Context, handle Handle) (iter.Seq[EngineEvent], error)
	CancelTurn(ctx context.Context, handle Handle) error
}

// Projector turns the executor's events into projected events: the pump feeds it
// a lead Open(), then each turn.Event via Translate, and a SynthesizeTerminal()
// when the stream ended without a terminal. It hands back the durable side
// effect to commit and the opaque wire payload to publish.
//
// It is stateful per segment (open-item tracking, step ordinal, error
// classification), so the delivery layer builds one per segment; the application
// never sees the wire shape it produces.
type Projector interface {
	// Open leads every segment (root + continuation) with its run.started-class
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

// ProjectedEvent is one event on its way to both the durable store and the live
// journal. Payload and Effect are BOTH opaque to the application: it shuttles
// each from the delivery-side Projector to its consumer without inspecting it.
// Payload is the wire event (published to the journal); Effect is the durable
// side effect (handed to the Effects port). The projector builds Effect as a
// concrete adapter type; only that adapter reads it back.
type ProjectedEvent struct {
	Durable   bool // retained for replay (mirrors the wire's IsDurable)
	Terminal  bool // ends the run's stream (the run.finished)
	Interrupt bool // terminal that PARKS — takes the commit-before-publish path
	Abort     bool // projection failed; cancel the turn and terminalize as error
	Payload   any  // opaque wire payload (a protocol StreamEvent)
	Effect    any  // opaque durable side effect (an adapter side-effect record)
}

// SegmentView exposes late-bound segment state the Projector reads at terminal
// time — notably the human cancel reason, which the cancel path sets AFTER the
// segment has started, so it must be read live, not captured at open.
type SegmentView interface {
	CancelReason() string
}

// Effects is the durable side of a run segment: commit the interrupt record
// before its event is published (BeforeLive), commit item/run/files after
// publication (AfterLive), and run terminal boundary maintenance (Finish). The
// side-effect payload is opaque (see ProjectedEvent.Effect); only [Finish] —
// which the pump builds from run-boundary facts it owns — is a concrete type.
// The adapter/runsegment.Effects satisfies this structurally.
type Effects interface {
	BeforeLive(ctx context.Context, effect any) error
	AfterLive(ctx context.Context, effect any)
	Finish(ctx context.Context, fin Finish)
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

// CursorMinter supplies the monotonic, fixed-width, lexically-ordered cursor
// stamped on each [Event]. Minting stays with the delivery layer (it is wire
// framing — the evt_ id); the application only needs opaque monotonic strings.
type CursorMinter interface {
	Mint() string
}

// StartSpec is the protocol-free description of a run segment to open. User
// input and resume bindings are deliberately NOT here — they live in the
// Projector the caller supplies, so the application never sees wire content.
type StartSpec struct {
	RunID       string
	ParentRunID string
	SessionID   string
	Cwd         string
	// TurnID is the executor's durable turn identity recorded on the live run —
	// supplied alongside the opaque Handle so the application never reaches into
	// the executor's handle representation.
	TurnID          string
	Handle          Handle
	Provider        string
	Model           string
	CreatedAt       time.Time
	OpeningUserText string
}
