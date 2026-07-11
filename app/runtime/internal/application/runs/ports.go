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
// journal. Payload is opaque to the application — it shuttles the wire event to
// the journal without inspecting it. Commit is the event's atomic durable
// side-effect ([execution.EventCommit], nil when the event persists nothing);
// Nudge is a non-durable live workspace notification (nil when none). The
// application reads neither Commit's records nor Nudge's paths — it forwards them
// to the [Effects] port — but Commit is a domain value (not an opaque adapter
// type) so the run pump can commit it without an agent-SDK dependency.
type ProjectedEvent struct {
	Durable   bool                   // retained for replay (mirrors the wire's IsDurable)
	Terminal  bool                   // ends the run's stream (the run.finished)
	Interrupt bool                   // terminal that PARKS — takes the commit-before-publish path
	Abort     bool                   // projection failed; cancel the turn and terminalize as error
	Payload   any                    // opaque wire payload (a protocol StreamEvent)
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
// transaction — the pump calls it BEFORE publishing an interrupt (so a client
// can't resume ahead of the durable record) and AFTER publishing every other
// event (so a slow store can't delay live delivery), the commit-before-publish
// vs live-first ordering the pump owns. Nudge is a non-durable live workspace
// notification; Finish runs terminal boundary maintenance (checkpoint snapshot,
// title) off the live path. The adapter/runsegment.Effects satisfies this.
type Effects interface {
	// CommitEvent applies commit's set parts (interrupt open, transcript item/run,
	// run-state transition) in one transaction. The pump checks the returned error
	// only for a park (an interrupt commit that fails aborts the run); item/run
	// commits are best-effort (a lost projection self-heals, reconcile sweeps a
	// stranded admission row).
	CommitEvent(ctx context.Context, commit execution.EventCommit) error
	// Nudge publishes a non-durable live workspace change to subscribers.
	Nudge(cwd string, paths []string)
	// Finish runs terminal boundary maintenance off the live path. A parked run is
	// resumable, not a boundary, so Finish no-ops for it (fin.Parked).
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
