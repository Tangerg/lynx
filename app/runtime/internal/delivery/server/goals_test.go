package server

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestGoalPtrProjectsMachineReadableReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domain goal.ReasonCode
		wire   protocol.GoalReasonCode
	}{
		{goal.ReasonStoppedByUser, protocol.GoalReasonStoppedByUser},
		{goal.ReasonRuntimeRestarted, protocol.GoalReasonRuntimeRestarted},
		{goal.ReasonRunStartFailed, protocol.GoalReasonRunStartFailed},
		{goal.ReasonAwaitingInput, protocol.GoalReasonAwaitingInput},
		{goal.ReasonTerminalOutcomeMissing, protocol.GoalReasonTerminalOutcomeMissing},
		{goal.ReasonRunNotCompleted, protocol.GoalReasonRunNotCompleted},
		{goal.ReasonRunBudgetReached, protocol.GoalReasonRunBudgetReached},
		{goal.ReasonCostBudgetReached, protocol.GoalReasonCostBudgetReached},
		{goal.ReasonStepBudgetReached, protocol.GoalReasonStepBudgetReached},
		{goal.ReasonBlockedByModel, protocol.GoalReasonBlockedByModel},
	}

	for _, test := range tests {
		t.Run(string(test.domain), func(t *testing.T) {
			t.Parallel()
			presented, err := presentGoal(goal.Goal{
				SessionID: "session-1",
				Objective: "finish the migration",
				Status:    goal.StatusBlocked,
				Reason:    goal.Reason{Code: test.domain, Detail: "safe context"},
				CreatedAt: time.Unix(1, 0),
				UpdatedAt: time.Unix(2, 0),
			})
			if err != nil {
				t.Fatalf("presentGoal: %v", err)
			}
			if presented.Reason == nil || presented.Reason.Code != test.wire || presented.Reason.Detail != "safe context" {
				t.Fatalf("reason = %+v, want code %q and preserved detail", presented.Reason, test.wire)
			}
		})
	}
}

func TestGoalPtrOmitsReasonForActiveGoal(t *testing.T) {
	t.Parallel()

	presented, err := presentGoal(goal.Goal{Status: goal.StatusActive})
	if err != nil {
		t.Fatalf("presentGoal: %v", err)
	}
	if presented.Reason != nil {
		t.Fatalf("active reason = %+v, want nil", presented.Reason)
	}
}

func TestGoalPtrRejectsUnknownReasonCode(t *testing.T) {
	t.Parallel()

	_, err := presentGoal(goal.Goal{
		Status: goal.StatusPaused,
		Reason: goal.Reason{Code: goal.ReasonCode("futureReason")},
	})
	if err == nil {
		t.Fatal("presentGoal accepted an unpublished reason code")
	}
}
