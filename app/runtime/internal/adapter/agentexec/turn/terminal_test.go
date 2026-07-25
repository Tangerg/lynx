package turn

import (
	"testing"

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
