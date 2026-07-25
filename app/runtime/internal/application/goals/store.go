package goals

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
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
	Clear(ctx context.Context, sessionID string) error
	ClearIf(ctx context.Context, sessionID string, expected goal.Version) (applied bool, err error)
	List(ctx context.Context) ([]goal.Goal, error)
}

// TurnStore records one terminal goal-owned Run exactly once. It joins the
// terminal Run transaction, rather than asking the loop to reconstruct durable
// accounting after it has observed a streamed terminal event.
type TurnStore interface {
	RecordTurn(ctx context.Context, record goal.TurnRecord) error
}

// DurableStore is the complete persistence surface the composition root gives
// to a Run terminalizer. The Driver and State consume the smaller Store slice.
type DurableStore interface {
	Store
	TurnStore
}

// State is the narrowly exposed autonomous-goal state use case. Tool adapters
// can report a terminal status and gate their manifest, but never read or
// write the persistence model directly.
type State struct {
	goals Store
	now   func() time.Time
}

// ReportCommand is a model-originated terminal status report for the active
// goal. LeaseID is the run's immutable origin stamp; empty is valid for a
// user-originated turn and targets whichever goal is currently active.
type ReportCommand struct {
	SessionID string
	LeaseID   string
	Status    goal.Status
	Reason    string
}

// ReportResult tells the adapter which truthful, recoverable outcome occurred.
// It avoids exporting persistence/CAS details into the tool layer.
type ReportResult int

const (
	ReportApplied ReportResult = iota
	ReportNoActiveGoal
	ReportSuperseded
	ReportConflict
	ReportReasonRequired
	ReportInvalidStatus
)

// NewState builds the state boundary shared by the goal loop and the tool
// environment. A nil store leaves Goal mode unavailable; callers normally omit
// the boundary rather than passing nil.
func NewState(store Store) *State {
	if store == nil {
		return nil
	}
	return &State{goals: store, now: time.Now}
}

// Active reports whether sessionID currently has a loop-driving goal.
func (s *State) Active(ctx context.Context, sessionID string) (bool, error) {
	if s == nil || s.goals == nil {
		return false, nil
	}
	g, ok, err := s.goals.Get(ctx, sessionID)
	if err != nil {
		return false, err
	}
	return ok && g.Status == goal.StatusActive, nil
}

// Report applies one model-declared terminal status through the goal use case.
// It owns the active-state, lease, validation, revision, and CAS rules so no
// delivery adapter can accidentally mutate a goal aggregate or its store.
func (s *State) Report(ctx context.Context, cmd ReportCommand) (ReportResult, error) {
	if s == nil || s.goals == nil {
		return ReportNoActiveGoal, nil
	}
	g, ok, err := s.goals.Get(ctx, cmd.SessionID)
	if err != nil {
		return 0, err
	}
	if !ok || g.Status != goal.StatusActive {
		return ReportNoActiveGoal, nil
	}
	if cmd.LeaseID != "" && cmd.LeaseID != g.LeaseID {
		return ReportSuperseded, nil
	}
	expected := g.Version()
	switch cmd.Status {
	case goal.StatusComplete:
		g.Complete(s.now())
	case goal.StatusBlocked:
		if cmd.Reason == "" {
			return ReportReasonRequired, nil
		}
		g.Block(goal.ReasonBlockedByModel, cmd.Reason, s.now())
	default:
		return ReportInvalidStatus, nil
	}
	_, applied, err := s.goals.Save(ctx, g, expected)
	if err != nil {
		return 0, err
	}
	if !applied {
		return ReportConflict, nil
	}
	return ReportApplied, nil
}
