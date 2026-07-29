package execution

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// ErrSessionBusy reports that admitting a Run was rejected because the Session
// already holds a non-terminal Run — the "one active/interrupted Run per
// Session" invariant (§8.2). It is the domain sentinel the durable admission
// store returns and the run coordinator + delivery match against, so the
// invariant has one name across the rings (the sqlite partial-unique-index
// violation maps onto it; delivery maps it onto the wire session-busy error).
var ErrSessionBusy = errors.New("execution: session has a non-terminal run")

// RunDraft is the fresh Run an admission records as it enters [Running]: the
// durable side of "one non-terminal Run per Session" (§8.2). It carries only the
// identity + per-run selection an admission needs; the streamed segments, usage,
// and terminal Outcome accrue afterward. Provider/Model are the run's explicit
// per-run model selection (empty ⇒ the runtime default). Executor recovery
// handles do not belong on the Run row; a parked interrupt records the actual
// process snapshot id at the point where it is known.
type RunDraft struct {
	RunID          string
	SessionID      string
	ModelSelection modelref.Selection
	// Limits is the allowance this Run is admitted under. It is recorded with the
	// admission and never changes: a resume answers an interrupt, it does not
	// renegotiate the budget the Run was accepted with.
	Limits    RunLimits
	CreatedAt time.Time
}

// RunLimits is the accumulated allowance a Run may consume before it is stopped.
// A zero field is that dimension uncapped, so the zero value is an unbounded Run.
//
// It lives beside [RunState] and [Outcome] rather than with the accrued
// accounting because it is execution POLICY — an input the admission fixes,
// which the executor enforces and a cross-process rehydrate must reapply — while
// what was actually spent is a recorded fact.
type RunLimits struct {
	MaxSteps     int
	MaxBudgetUSD float64
}

// Validate reports whether the allowance is expressible. A negative cap is not
// "no cap" — it is a cap nothing can satisfy, and admitting one would stop the
// Run before its first step.
func (l RunLimits) Validate() error {
	if l.MaxSteps < 0 || l.MaxBudgetUSD < 0 {
		return errors.New("execution: run limits must not be negative")
	}
	return nil
}

// IsZero reports whether no allowance is in force at all.
func (l RunLimits) IsZero() bool { return l == RunLimits{} }

// ResumeDraft is the durable identity of a parked Run whose next segment is
// opening. Applying it atomically consumes the Run's open interrupt and moves
// its admission state from Interrupted back to Running.
type ResumeDraft struct {
	RunID     string
	SessionID string
}
