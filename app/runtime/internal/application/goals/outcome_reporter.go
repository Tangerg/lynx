package goals

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

// ReportCommand is a model-originated terminal outcome for the active Goal.
// LeaseID is the Run's immutable origin stamp; empty targets the current Goal.
type ReportCommand struct {
	SessionID string
	LeaseID   string
	Outcome   goal.Status
	Reason    string
}

// ReportResult identifies the recoverable outcome of one report.
type ReportResult int

const (
	ReportApplied ReportResult = iota
	ReportNoActiveGoal
	ReportSuperseded
	ReportConflict
	ReportReasonRequired
	ReportInvalidOutcome
)

// OutcomeReporter owns terminal outcome validation and compare-and-swap.
type OutcomeReporter struct {
	goals Store
	now   func() time.Time
}

// NewOutcomeReporter returns the outcome boundary over store. A nil store leaves
// Goal mode unavailable, so composition should omit the reporter.
func NewOutcomeReporter(store Store) *OutcomeReporter {
	if store == nil {
		return nil
	}
	return &OutcomeReporter{goals: store, now: time.Now}
}

// Report applies one model-declared terminal outcome to the active Goal.
func (r *OutcomeReporter) Report(ctx context.Context, cmd ReportCommand) (ReportResult, error) {
	if r == nil || r.goals == nil {
		return ReportNoActiveGoal, nil
	}
	g, ok, err := r.goals.Get(ctx, cmd.SessionID)
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
	switch cmd.Outcome {
	case goal.StatusComplete:
		g.Complete(r.now())
	case goal.StatusBlocked:
		if cmd.Reason == "" {
			return ReportReasonRequired, nil
		}
		g.Block(goal.ReasonBlockedByModel, cmd.Reason, r.now())
	default:
		return ReportInvalidOutcome, nil
	}
	_, applied, err := r.goals.Save(ctx, g, expected)
	if err != nil {
		return 0, err
	}
	if !applied {
		return ReportConflict, nil
	}
	return ReportApplied, nil
}
