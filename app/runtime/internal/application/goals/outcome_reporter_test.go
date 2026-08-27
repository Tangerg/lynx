package goals

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/goal"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
)

type reportingStore struct {
	goal     goal.Goal
	present  bool
	conflict bool
}

func (r *reportingStore) Get(context.Context, string) (goal.Goal, bool, error) {
	return r.goal, r.present, nil
}
func (r *reportingStore) Save(_ context.Context, next goal.Goal, expected goal.Version) (goal.Goal, bool, error) {
	if r.conflict || !r.present || r.goal.Version() != expected {
		return goal.Goal{}, false, nil
	}
	next.Revision++
	r.goal = next
	return next, true, nil
}
func (r *reportingStore) Clear(context.Context, string) error { r.present = false; return nil }
func (r *reportingStore) ClearIf(context.Context, string, goal.Version) (bool, error) {
	return false, nil
}
func (r *reportingStore) List(context.Context) ([]goal.Goal, error) { return nil, nil }

func TestOutcomeReporterOwnsTerminalGoalTransition(t *testing.T) {
	now := time.Date(2026, time.July, 23, 9, 0, 0, 0, time.UTC)
	g, err := goal.New("ses_1", "finish", modelref.Selection{}, goal.Budget{}, run.Capabilities{}, "lease-current", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	store := &reportingStore{goal: g, present: true}
	reader := NewReader(store)
	reporter := NewOutcomeReporter(store)
	reporter.now = func() time.Time { return now }

	active, err := reader.Active(t.Context(), "ses_1")
	if err != nil || !active {
		t.Fatalf("Active = %v, %v, want true, nil", active, err)
	}

	result, err := reporter.Report(t.Context(), ReportCommand{
		SessionID: "ses_1", IncarnationID: "lease-stale", Outcome: goal.StatusComplete,
	})
	if err != nil || result != ReportSuperseded {
		t.Fatalf("stale Report = %v, %v, want superseded, nil", result, err)
	}
	if store.goal.Status != goal.StatusActive {
		t.Fatalf("stale report changed status to %q", store.goal.Status)
	}

	result, err = reporter.Report(t.Context(), ReportCommand{
		SessionID: "ses_1", IncarnationID: "lease-current", Outcome: goal.StatusBlocked,
	})
	if err != nil || result != ReportReasonRequired {
		t.Fatalf("reasonless blocked Report = %v, %v, want reason-required, nil", result, err)
	}

	result, err = reporter.Report(t.Context(), ReportCommand{
		SessionID: "ses_1", IncarnationID: "lease-current", Outcome: goal.StatusBlocked, Reason: "needs credentials",
	})
	if err != nil || result != ReportApplied {
		t.Fatalf("blocked Report = %v, %v, want applied, nil", result, err)
	}
	if store.goal.Status != goal.StatusBlocked || store.goal.Reason != (goal.Reason{Code: goal.ReasonBlockedByModel, Detail: "needs credentials"}) || !store.goal.UpdatedAt.Equal(now) {
		t.Fatalf("stored goal = %+v, want blocked state at %s", store.goal, now)
	}
}
