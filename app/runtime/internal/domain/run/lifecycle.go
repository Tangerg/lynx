package run

// State is the lifecycle position of a Run and the single authority for its
// legal transitions.
//
// The state machine (see the transition methods below):
//
//	Running ──Suspend──▶ Waiting
//	   │  ▲                   │
//	   │  └──────Resume───────┘
//	   │                      │
//	Terminate(o)       Terminate(Canceled)
//	   │                      │
//	   ▼                      ▼
//	Completed / Failed / Canceled
//
// A Run reaches exactly one terminal state. Root admission ("one non-terminal
// root Run per Session") keys on [State.IsTerminal]; descendants share their
// root tree's admission.
type State string

const (
	// Running — a segment is actively executing.
	Running State = "running"
	// Waiting — parked on a HITL interrupt, awaiting Resume or Cancel. NOT
	// terminal: the run is resumable, its durable interrupt record committed.
	Waiting State = "waiting"
	// Completed — the model finished normally.
	Completed State = "completed"
	// Failed — the Run stopped without completing. The exact reason remains in
	// its Outcome: TimedOut, Failed, MaxBudget, MaxSteps, or Lost.
	Failed State = "failed"
	// Canceled — the caller canceled the run, or its context was canceled.
	Canceled State = "canceled"
)

func (s State) Valid() bool {
	return s == Running || s == Waiting || s == Completed || s == Failed || s == Canceled
}

// IsTerminal reports whether s is an end state (Completed, Failed, or Canceled)
// — no further transition is legal, and the run no longer holds a Session's
// single-writer admission slot.
func (s State) IsTerminal() bool {
	return s == Completed || s == Failed || s == Canceled
}

// Suspend parks a running run on a HITL interrupt (Running → Waiting). It
// reports false from any other state.
func (s State) Suspend() (State, bool) {
	if s == Running {
		return Waiting, true
	}
	return s, false
}

// Resume continues a parked run (Waiting → Running). It reports false from
// any other state.
func (s State) Resume() (State, bool) {
	if s == Waiting {
		return Running, true
	}
	return s, false
}

// Terminate ends a run with outcome o, returning the resulting terminal state.
// It is legal from Running for any outcome, and from Waiting only for
// [OutcomeCanceled] — a parked run can be canceled outright, but reaching any
// other terminal requires resuming first. It reports false (leaving s unchanged)
// from any other state or an illegal (Waiting, non-cancel) pair.
func (s State) Terminate(o Outcome) (State, bool) {
	if !o.valid() {
		return s, false
	}
	switch s {
	case Running:
		return o.terminalState(), true
	case Waiting:
		if o == OutcomeCanceled {
			return Canceled, true
		}
	}
	return s, false
}

// RecoverLost ends a non-terminal run whose executor disappeared without a
// resumable interrupt. Loss is a recovery transition rather than a normal
// executor outcome: both Running and an inconsistent orphaned Waiting run
// become Failed, while terminal states are immutable.
func (s State) RecoverLost() (State, bool) {
	if s == Running || s == Waiting {
		return Failed, true
	}
	return s, false
}

func (s State) String() string {
	if !s.Valid() {
		return "unknown"
	}
	return string(s)
}

// Outcome is why a Run reached a terminal state. Persistence and presentation
// both project from this single terminal-reason taxonomy.
//
// An interrupt is deliberately NOT an Outcome: parking is the [Waiting]
// state, not a terminal reason. A run that ends while parked ends via
// [OutcomeCanceled].
type Outcome string

const (
	// OutcomeCompleted — the model returned a stop-marker normally. → Completed.
	OutcomeCompleted Outcome = "completed"
	// OutcomeCanceled — the caller canceled, or the context was canceled. →
	// Canceled.
	OutcomeCanceled Outcome = "canceled"
	// OutcomeTimedOut — the Run exceeded its governing deadline. This is not a
	// generic failure: callers may apply a distinct retry and alerting policy. →
	// Failed.
	OutcomeTimedOut Outcome = "timedOut"
	// OutcomeFailed — the run aborted on an error. → Failed.
	OutcomeFailed Outcome = "failed"
	// OutcomeMaxBudget — the run hit its token/cost budget and stopped cleanly
	// after the current round (the partial reply already streamed). → Failed.
	OutcomeMaxBudget Outcome = "maxBudget"
	// OutcomeMaxSteps — the run hit its delegation-tree model-call cap and
	// stopped cleanly. Distinct from OutcomeMaxBudget because the exhausted
	// allowance is a different terminal fact. → Failed.
	OutcomeMaxSteps Outcome = "maxSteps"
	// OutcomeLost — recovery proved that no live executor or valid checkpoint
	// can continue the Run. It is produced by recovery, never by an executor. →
	// Failed.
	OutcomeLost Outcome = "lost"
)

// terminalState maps a terminal outcome to the [State] it produces: normal
// completion → Completed, cancellation → Canceled, and every non-success
// terminal reason → Failed.
func (o Outcome) terminalState() State {
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
	switch o {
	case OutcomeCompleted, OutcomeCanceled, OutcomeTimedOut, OutcomeFailed,
		OutcomeMaxBudget, OutcomeMaxSteps, OutcomeLost:
		return true
	default:
		return false
	}
}

// ParseOutcome maps an outcome's [Outcome.String] form back to the value,
// reporting false for anything else. It sits next to String because they are one
// mapping read in two directions: a durable record must come back as the same
// terminal reason it was written as, and a second hand-written table somewhere
// downstream would be free to disagree with this one.
func ParseOutcome(s string) (Outcome, bool) {
	outcome := Outcome(s)
	if !outcome.valid() {
		return "", false
	}
	return outcome, true
}

func (o Outcome) String() string {
	if !o.valid() {
		return "unknown"
	}
	return string(o)
}
