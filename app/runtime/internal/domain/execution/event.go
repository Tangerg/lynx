package execution

// Event is a run-execution event as the run pipeline classifies it: the
// lifecycle facts the application acts on — whether this event ends the run (and
// with what [Outcome]) and whether it parks the run for HITL resume. The
// concrete, data-rich event is the agent-execution adapter's own type; the
// application depends only on this narrow classification (ISP), so the run
// lifecycle owns no agent-SDK type, and the terminal/park DECISION lives on the
// domain contract rather than being re-derived from the wire projection.
type Event interface {
	// Terminal reports the terminal [Outcome] this event ends the run with, or
	// ok=false when the event does not end the run (a streaming delta, a park, a
	// mid-run signal). An interrupt is NOT a terminal Outcome — see Interrupt.
	Terminal() (Outcome, bool)
	// Interrupt reports whether this event parks the run for HITL resume — a
	// terminal-that-suspends, distinct from a run-ending Outcome: the run stays
	// resumable in the [Interrupted] state (§8.3), it does not reach a terminal.
	Interrupt() bool
}

// Durability classifies a run event by its relationship to the durable record,
// which fixes the ORDER in which the event pipeline commits and publishes it.
// It is the canonical rule the application-owned pipeline switches on:
//
//	executor event → normalize → [commit if Durable] → publish → delivery projection
//
// The pre-rewrite pipeline only honored this ordering for the interrupt record;
// every other event published first and persisted afterward on a best-effort
// basis (store errors were swallowed). The rewrite generalizes commit-before-
// publish to every Durable event.
type Durability uint8

const (
	// Live events are published to subscribers immediately and are NOT part of
	// the durable record: output/reasoning deltas, tool-argument/-output deltas,
	// progress previews, and transient state. A dropped Live event self-heals on
	// the next snapshot or a resync; it is never replayed from storage, so it
	// carries no ordering obligation against the store.
	Live Durability = iota

	// Durable events must be committed to the execution store BEFORE they are
	// published (commit-before-publish): completed items, the interrupt record,
	// and the terminal outcome. The guarantee they underwrite is that a
	// subscriber acting on a Durable event finds the committed state already
	// present — e.g. a client may resume the instant it observes the interrupt,
	// so the resumable record must exist first; and a query issued right after
	// the terminal must see the terminal state. Publication also unlocks new
	// commands, so it must not precede the commit those commands depend on.
	Durable
)

// MustCommitFirst reports whether an event of this durability must be committed
// before publication — the commit-before-publish predicate the pipeline gates
// on.
func (d Durability) MustCommitFirst() bool { return d == Durable }

func (d Durability) String() string {
	switch d {
	case Live:
		return "live"
	case Durable:
		return "durable"
	default:
		return "unknown"
	}
}
