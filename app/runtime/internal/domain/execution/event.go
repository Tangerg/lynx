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
