package execution

import (
	"errors"
	"fmt"
	"strings"
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
// row; a parked interrupt records the executor process ID when it is known.
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

// RunResumeDraft is one parked Run whose fresh continuation Segment is opening.
// It is always applied as part of a root-owned [TreeResumeDraft]; a descendant
// cannot resume independently of the barrier that suspended the complete tree.
type RunResumeDraft struct {
	RunID string
	// SegmentID is the continuation's fresh segment, which replaces the one the
	// park cleared — in the same transaction that moves the Run back to Running.
	SegmentID string
}

// TreeResumeDraft is the complete durable identity set reopened by one accepted
// answer barrier. Runs is canonical postorder (descendants before ancestors,
// siblings by Run ID, root last), matching the Pending continuation set it
// consumes in the same transaction.
type TreeResumeDraft struct {
	RootRunID string
	SessionID string
	// ResumedAt is the single tree-opening timestamp used by every Run row.
	// Recording it on the draft lets the application return the exact committed
	// root snapshot instead of approximating a store-owned clock.
	ResumedAt time.Time
	Runs      []RunResumeDraft
}

// Validate checks the tree-resume identity frame. Topology and exact postorder
// correspondence are checked against the consumed Pending set by the aggregate
// transaction, which owns both values.
func (draft TreeResumeDraft) Validate() error {
	switch {
	case strings.TrimSpace(draft.RootRunID) == "":
		return errors.New("execution: tree resume root run id is required")
	case strings.TrimSpace(draft.SessionID) == "":
		return errors.New("execution: tree resume session id is required")
	case draft.ResumedAt.IsZero():
		return errors.New("execution: tree resume time is required")
	case len(draft.Runs) == 0:
		return errors.New("execution: tree resume has no Runs")
	}
	seen := make(map[string]struct{}, len(draft.Runs))
	for index, run := range draft.Runs {
		if strings.TrimSpace(run.RunID) == "" || strings.TrimSpace(run.SegmentID) == "" {
			return fmt.Errorf("execution: tree resume Run[%d] has incomplete identity", index)
		}
		if _, duplicate := seen[run.RunID]; duplicate {
			return fmt.Errorf("execution: tree resume repeats Run %q", run.RunID)
		}
		seen[run.RunID] = struct{}{}
	}
	if draft.Runs[len(draft.Runs)-1].RunID != draft.RootRunID {
		return fmt.Errorf(
			"execution: tree resume root %q must be the final Run, got %q",
			draft.RootRunID,
			draft.Runs[len(draft.Runs)-1].RunID,
		)
	}
	return nil
}
