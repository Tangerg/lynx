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
	CreatedAt      time.Time
}

// ResumeDraft is the durable identity of a parked Run whose next segment is
// opening. Applying it atomically consumes the Run's open interrupt and moves
// its admission state from Interrupted back to Running.
type ResumeDraft struct {
	RunID     string
	SessionID string
}

// AdmittedRun is what the durable admission record can answer about a Run
// without loading its transcript: identity, where it sits in its lifecycle, and
// the selection it runs under.
//
// It is the authoritative answer to "which Runs exist and what are they doing" —
// a live in-process registry only knows the segments THIS process is streaming,
// so a Run parked on an interrupt, or one whose process died, is absent from it
// while very much still being a Run.
type AdmittedRun struct {
	RunID          string
	SessionID      string
	State          RunState
	ModelSelection modelref.Selection
	StartedAt      time.Time
}
