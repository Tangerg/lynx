package runs

import (
	"context"
	"iter"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// The ports this package consumes to run a segment. They are defined here — on
// the consumer side — and satisfied structurally by the runtime / delivery /
// kernel implementations the composition root injects.
//
// Executor and Projector currently reference kernel types (turn.Event,
// turn.TurnHandle) and kernel/runsegment.Event: the application depends inward
// on them for now. A later batch inverts the executor edge behind an
// engine-neutral event (rewrite Batch 5) and relocates runsegment (Batch 3).

// Executor is what the run pump needs to drive, observe, and cancel the agent
// turn backing a run segment. The concrete runtime implements it.
type Executor interface {
	TurnEvents(ctx context.Context, handle turn.TurnHandle) (iter.Seq[turn.Event], error)
	CancelTurn(ctx context.Context, handle turn.TurnHandle) error
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
	Translate(ev turn.Event) []ProjectedEvent
	// SynthesizeTerminal builds the terminal the pump needs when the executor
	// stream ended without one (canceled mid-flight / drained iterator). The
	// Projector decides error-vs-canceled from its own recorded state.
	SynthesizeTerminal() []ProjectedEvent
	// Abort records a projection/commit-failure message so a subsequent
	// SynthesizeTerminal reports the run as errored.
	Abort(msg string)
}

// ProjectedEvent is one event on its way to both the durable store and the live
// journal. Payload is the opaque wire event (the application never imports the
// wire package); Effect is the durable domain record to commit.
type ProjectedEvent struct {
	Durable   bool             // retained for replay (mirrors the wire's IsDurable)
	Terminal  bool             // ends the run's stream (the run.finished)
	Interrupt bool             // terminal that PARKS — takes the commit-before-publish path
	Abort     bool             // projection failed; cancel the turn and terminalize as error
	Payload   any              // opaque wire payload (a protocol StreamEvent)
	Effect    runsegment.Event // durable side effect to commit (item / run / interrupt / files)
}

// SegmentView exposes late-bound segment state the Projector reads at terminal
// time — notably the human cancel reason, which the cancel path sets AFTER the
// segment has started, so it must be read live, not captured at open.
type SegmentView interface {
	CancelReason() string
}

// Effects is the durable side of a run segment: commit the interrupt record
// before its event is published (BeforeLive), commit item/run/files after
// publication (AfterLive), and run terminal boundary maintenance (Finish).
// *kernel/runsegment.Effects satisfies this structurally.
type Effects interface {
	BeforeLive(ctx context.Context, ev runsegment.Event) error
	AfterLive(ctx context.Context, ev runsegment.Event)
	Finish(ctx context.Context, fin runsegment.Finish)
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
	RunID           string
	ParentRunID     string
	SessionID       string
	Cwd             string
	Handle          turn.TurnHandle
	Provider        string
	Model           string
	CreatedAt       time.Time
	OpeningUserText string
}
