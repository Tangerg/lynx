package execution

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// ErrSessionBusy reports that admitting a root Run was rejected because the
// Session already holds a non-terminal root tree. Descendant Runs share that
// tree's admission and do not compete for a second Session slot.
var ErrSessionBusy = errors.New("execution: session has a non-terminal root run")

// RunDraft is the fresh root or child Run recorded as it enters [Running]. A
// root claims the Session's one non-terminal tree slot; a child carries all
// lineage edges and shares that claim. Streamed segments, usage, and terminal
// Outcome accrue afterward. Executor recovery handles do not belong on the Run
// row; a parked interrupt records the process snapshot id when it is known.
type RunDraft struct {
	RunID           string
	SessionID       string
	SpawnedByItemID string
	ParentRunID     string
	RootRunID       string
	// SegmentID is the first segment this Run opens with. Admission records it
	// with the Run, because a Running Run without the segment driving it is a Run
	// nothing can attach to.
	SegmentID      string
	ModelSelection modelref.Selection
	// Limits is the allowance this Run is admitted under. It is recorded with the
	// admission and never changes: a resume answers an interrupt, it does not
	// renegotiate the budget the Run was accepted with.
	Limits RunLimits
	// ProtocolProfile is the protocol contract negotiated for this Run. Like
	// Limits it is fixed here — and unlike Limits, the admission is its ONLY
	// writer: no later transition mentions it, which is how "immutable for the
	// Run's whole life" is kept by construction rather than by a check.
	ProtocolProfile RunProtocolProfile
	CreatedAt       time.Time
}

// Lineage returns the draft's immutable root/child identity as one value for
// validation and tree routing.
func (draft RunDraft) Lineage() RunLineage {
	return RunLineage{
		SpawnedByItemID: draft.SpawnedByItemID,
		ParentRunID:     draft.ParentRunID,
		RootRunID:       draft.RootRunID,
	}
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
	// SegmentID is the continuation's fresh segment, which replaces the one the
	// park cleared — in the same transaction that moves the Run back to Running.
	SegmentID string
}
