package goals

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

type stateStore struct {
	goal     goal.Goal
	present  bool
	conflict bool
}

func (s *stateStore) Get(context.Context, string) (goal.Goal, bool, error) {
	return s.goal, s.present, nil
}
func (s *stateStore) Save(_ context.Context, next goal.Goal, expected goal.Version) (bool, error) {
	if s.conflict || !s.present || s.goal.Version() != expected {
		return false, nil
	}
	s.goal = next
	return true, nil
}
func (s *stateStore) Clear(context.Context, string) error { s.present = false; return nil }
func (s *stateStore) ClearIf(context.Context, string, goal.Version) (bool, error) {
	return false, nil
}
func (s *stateStore) List(context.Context) ([]goal.Goal, error) { return nil, nil }

func TestStateReportOwnsTerminalGoalTransition(t *testing.T) {
	now := time.Date(2026, time.July, 23, 9, 0, 0, 0, time.UTC)
	g, err := goal.New("ses_1", "finish", "", "", goal.Budget{}, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	g.RenewLease("lease-current")
	store := &stateStore{goal: g, present: true}
	state := NewState(store)
	state.now = func() time.Time { return now }

	active, err := state.Active(t.Context(), "ses_1")
	if err != nil || !active {
		t.Fatalf("Active = %v, %v, want true, nil", active, err)
	}

	result, err := state.Report(t.Context(), ReportCommand{
		SessionID: "ses_1", LeaseID: "lease-stale", Status: goal.StatusComplete,
	})
	if err != nil || result != ReportSuperseded {
		t.Fatalf("stale Report = %v, %v, want superseded, nil", result, err)
	}
	if store.goal.Status != goal.StatusActive {
		t.Fatalf("stale report changed status to %q", store.goal.Status)
	}

	result, err = state.Report(t.Context(), ReportCommand{
		SessionID: "ses_1", LeaseID: "lease-current", Status: goal.StatusBlocked,
	})
	if err != nil || result != ReportReasonRequired {
		t.Fatalf("reasonless blocked Report = %v, %v, want reason-required, nil", result, err)
	}

	result, err = state.Report(t.Context(), ReportCommand{
		SessionID: "ses_1", LeaseID: "lease-current", Status: goal.StatusBlocked, Reason: "needs credentials",
	})
	if err != nil || result != ReportApplied {
		t.Fatalf("blocked Report = %v, %v, want applied, nil", result, err)
	}
	if store.goal.Status != goal.StatusBlocked || store.goal.Reason != (goal.Reason{Cause: goal.ReasonBlockedByModel, Detail: "needs credentials"}) || !store.goal.UpdatedAt.Equal(now) {
		t.Fatalf("stored goal = %+v, want blocked state at %s", store.goal, now)
	}
}
