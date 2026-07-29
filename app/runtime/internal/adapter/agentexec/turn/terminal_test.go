package turn

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestPlanTurnEndRejectsCompletedProcessWithoutOutput(t *testing.T) {
	plan := planTurnEnd(agentexec.TurnCompletion{Status: core.StatusCompleted})
	if plan.reason != execution.OutcomeError || plan.problem == nil || plan.problem.Kind != transcript.InternalProblem {
		t.Fatalf("planTurnEnd = %+v, want internal error", plan)
	}
}

func TestPlanTurnEndUsesJoinedKilledStatus(t *testing.T) {
	plan := planTurnEnd(agentexec.TurnCompletion{Status: core.StatusKilled})
	if plan.reason != execution.OutcomeCanceled || plan.problem != nil {
		t.Fatalf("planTurnEnd = %+v, want canceled", plan)
	}
}

func TestPlanChildTurnEndDoesNotDisguiseProjectionFailureAsCancellation(t *testing.T) {
	at := time.Date(2026, time.July, 30, 10, 0, 0, 0, time.UTC)
	plan := planChildTurnEnd(agentexec.ChildCompletion{
		Process: agentexec.ChildProcess{
			ProcessRef: agentexec.ProcessRef{
				ID:          "child",
				ParentID:    "root",
				SpawnCallID: "call-task",
			},
			StartedAt: at,
		},
		Status:      core.StatusKilled,
		Err:         errors.New("usage projection failed"),
		CompletedAt: at.Add(time.Second),
	})
	if plan.reason != execution.OutcomeError ||
		plan.problem == nil ||
		plan.problem.Kind != transcript.InternalProblem {
		t.Fatalf("planChildTurnEnd = %+v, want internal error", plan)
	}
}
