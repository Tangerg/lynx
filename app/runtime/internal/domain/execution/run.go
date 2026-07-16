package execution

// RunState is the lifecycle position of a Run. It is the single source of truth
// for "where is this run" that the pre-rewrite code lacked — state was spread
// across a parked flag, registry membership, and the SDK's process status, with
// no one place enforcing the legal transitions.
//
// The state machine (see the transition methods below):
//
//	Running ──Suspend──▶ Interrupted
//	   │  ▲                   │
//	   │  └──────Resume───────┘
//	   │                      │
//	Terminate(o)       Terminate(Canceled)
//	   │                      │
//	   ▼                      ▼
//	Completed / Failed / Canceled
//
// A Run reaches exactly one terminal state. Admission ("one non-terminal Run per
// Session") keys on [RunState.IsTerminal].
type RunState uint8

const (
	// Running — a segment is actively executing.
	Running RunState = iota
	// Interrupted — parked on a HITL interrupt, awaiting Resume or Cancel. NOT
	// terminal: the run is resumable, its durable interrupt record committed.
	Interrupted
	// Completed — the model finished normally.
	Completed
	// Failed — the run stopped without completing: an error, or a budget/step
	// cap. The fine reason is the [Outcome] (Error / MaxBudget / MaxSteps).
	Failed
	// Canceled — the client canceled the run, or its context was canceled.
	Canceled
)

// IsTerminal reports whether s is an end state (Completed, Failed, or Canceled)
// — no further transition is legal, and the run no longer holds a Session's
// single-writer admission slot.
func (s RunState) IsTerminal() bool {
	return s == Completed || s == Failed || s == Canceled
}

// Suspend parks a running run on a HITL interrupt (Running → Interrupted). It
// reports false from any other state.
func (s RunState) Suspend() (RunState, bool) {
	if s == Running {
		return Interrupted, true
	}
	return s, false
}

// Resume continues a parked run (Interrupted → Running). It reports false from
// any other state.
func (s RunState) Resume() (RunState, bool) {
	if s == Interrupted {
		return Running, true
	}
	return s, false
}

// Terminate ends a run with outcome o, returning the resulting terminal state.
// It is legal from Running for any outcome, and from Interrupted only for
// [OutcomeCanceled] — a parked run can be canceled outright, but reaching any
// other terminal requires resuming first. It reports false (leaving s unchanged)
// from any other state or an illegal (Interrupted, non-cancel) pair.
func (s RunState) Terminate(o Outcome) (RunState, bool) {
	if !o.valid() {
		return s, false
	}
	switch s {
	case Running:
		return o.terminalState(), true
	case Interrupted:
		if o == OutcomeCanceled {
			return Canceled, true
		}
	}
	return s, false
}

// RecoverLost ends a non-terminal run whose executor disappeared without a
// resumable interrupt. Loss is a recovery transition rather than a normal
// executor outcome: both Running and an inconsistent orphaned Interrupted run
// become Failed, while terminal states are immutable.
func (s RunState) RecoverLost() (RunState, bool) {
	if s == Running || s == Interrupted {
		return Failed, true
	}
	return s, false
}

func (s RunState) String() string {
	switch s {
	case Running:
		return "running"
	case Interrupted:
		return "interrupted"
	case Completed:
		return "completed"
	case Failed:
		return "failed"
	case Canceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// Outcome is why a Run reached a terminal state — the single terminal-reason
// taxonomy that both the executor's terminal decision and the wire RunOutcome
// resolve against, replacing the pre-rewrite duplication between the turn
// layer's own end-reason enum and the protocol's RunOutcomeType.
//
// An interrupt is deliberately NOT an Outcome: parking is the [Interrupted]
// state, not a terminal reason. A run that ends while parked ends via
// [OutcomeCanceled].
type Outcome uint8

const (
	// OutcomeCompleted — the model returned a stop-marker normally. → Completed.
	OutcomeCompleted Outcome = iota
	// OutcomeCanceled — the client canceled, or the context was canceled. →
	// Canceled.
	OutcomeCanceled
	// OutcomeError — the run aborted on an error. → Failed.
	OutcomeError
	// OutcomeMaxBudget — the run hit its token/cost budget and stopped cleanly
	// after the current round (the partial reply already streamed). → Failed.
	OutcomeMaxBudget
	// OutcomeMaxSteps — the run hit its delegation-tree model-call cap and
	// stopped cleanly. Distinct from OutcomeMaxBudget so the wire can surface a
	// dedicated maxSteps terminal. → Failed.
	OutcomeMaxSteps
)

// terminalState maps a terminal outcome to the [RunState] it produces: normal
// completion → Completed, cancellation → Canceled, and every failure flavor
// (error, budget, steps) → Failed.
func (o Outcome) terminalState() RunState {
	switch o {
	case OutcomeCompleted:
		return Completed
	case OutcomeCanceled:
		return Canceled
	default:
		return Failed
	}
}

func (o Outcome) valid() bool {
	return o <= OutcomeMaxSteps
}

func (o Outcome) String() string {
	switch o {
	case OutcomeCompleted:
		return "completed"
	case OutcomeCanceled:
		return "canceled"
	case OutcomeError:
		return "error"
	case OutcomeMaxBudget:
		return "maxBudget"
	case OutcomeMaxSteps:
		return "maxSteps"
	default:
		return "unknown"
	}
}
