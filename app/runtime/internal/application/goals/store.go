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

// RunRecorder records one terminal goal-owned Run exactly once. It joins the
// terminal Run transaction, rather than asking the loop to reconstruct durable
// accounting after it has observed a streamed terminal event.
type RunRecorder interface {
	RecordRun(ctx context.Context, record goal.RunRecord) error
}

// DurableStore is the complete persistence surface required by a Run
// terminalizer. The Driver and State consume the smaller Store slice.
type DurableStore interface {
	Store
	RunRecorder
}

// State is the narrowly exposed autonomous-goal state use case. Tool callers
// can read the current aggregate, report a terminal outcome, and gate their
// manifest, but never receive persistence or compare-and-swap operations.
type State struct {
	goals Store
	now   func() time.Time
}

// ReportCommand is a model-originated terminal status report for the active
// goal. LeaseID is the run's immutable origin stamp; empty is valid for a
// user-originated Run and targets whichever goal is currently active.
type ReportCommand struct {
	SessionID string
	LeaseID   string
	Status    goal.Status
	Reason    string
}

// ReportResult identifies the truthful, recoverable outcome of one report.
// It keeps persistence and compare-and-swap details private to this use case.
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

// Get returns the session's current Goal without exposing its Store.
func (s *State) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	if s == nil || s.goals == nil {
		return goal.Goal{}, false, nil
	}
	return s.goals.Get(ctx, sessionID)
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
// It owns the active-state, lease, validation, revision, and CAS rules so callers
// cannot accidentally mutate a Goal aggregate or its store.
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
