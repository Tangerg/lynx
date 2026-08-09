package run

import (
	"errors"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// ErrSessionBusy reports that admitting a root Run was rejected because the
// Session already holds a non-terminal root tree. Descendant Runs share that
// tree's admission and do not compete for a second Session slot.
var ErrSessionBusy = errors.New("run: session has a non-terminal root Run")

// Draft is the fresh root or child Run recorded as it enters [Running]. A
// root claims the Session's one non-terminal tree slot; a child carries all
// lineage edges and shares that claim. Streamed segments, usage, and terminal
// Outcome accrue afterward. Executor recovery handles do not belong on the Run
// row; a parked interrupt records its opaque executor member identity when known.
type Draft struct {
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
	// GoalLeaseID is the autonomous-goal incarnation that admitted this root
	// Run. It is durable admission provenance so crash recovery can terminalize
	// the Run and charge the exact lease in one transaction. Child Runs leave it
	// empty because the root is the single Goal Run.
	GoalLeaseID string
	// Limits is the allowance this Run is admitted under. It is recorded with the
	// admission and never changes: a resume answers an interrupt, it does not
	// renegotiate the budget the Run was accepted with.
	Limits Limits
	// Capabilities is the optional behavior enabled for this Run. Like Limits it
	// is fixed here — and unlike Limits, admission is its ONLY
	// writer: no later transition mentions it, which is how "immutable for the
	// Run's whole life" is kept by construction rather than by a check.
	Capabilities Capabilities
	CreatedAt    time.Time
}

// Lineage returns the draft's immutable root/child identity as one value for
// validation and tree routing.
func (draft Draft) Lineage() Lineage {
	return Lineage{
		SpawnedByItemID: draft.SpawnedByItemID,
		ParentRunID:     draft.ParentRunID,
		RootRunID:       draft.RootRunID,
	}
}
