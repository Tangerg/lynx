package goals

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
)

// Store is the autonomous-goal use case's durable state. It is deliberately
// owned here: the domain owns goal values and invariants, while application
// workflows decide when those values are read, persisted, cleared, or
// reconciled.
type Store interface {
	Get(ctx context.Context, sessionID string) (goal.Goal, bool, error)
	// Save atomically assigns the next durable revision and returns the saved
	// snapshot. Revision on g is ignored: expected is the sole CAS authority.
	// A lost compare-and-swap returns the zero Goal with applied=false.
	Save(ctx context.Context, g goal.Goal, expected goal.Version) (saved goal.Goal, applied bool, err error)
	ClearIf(ctx context.Context, sessionID string, expected goal.Version) (applied bool, err error)
	List(ctx context.Context) ([]goal.Goal, error)
}

// RunRecorder records one terminal goal-owned Run exactly once. It joins the
// terminal Run transaction, rather than asking the drive to reconstruct durable
// accounting after it has observed a streamed terminal event.
type RunRecorder interface {
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

// DurableStore is the complete persistence surface required by a Run
// terminalizer. The Driver, Reader, and OutcomeReporter consume Store.
type DurableStore interface {
	Store
	RunRecorder
	// Clear is reserved for the session aggregate's atomic delete write-set.
	// Goal lifecycle and boot recovery use versioned ClearIf instead.
	Clear(ctx context.Context, sessionID string) error
}
