package builtin

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
)

func TestRunInstructionsUseOutcomeReportingContract(t *testing.T) {
	instructions := RunInstructions(goals.RunInstructionInput{Objective: "finish migration", Continuing: true})
	for _, want := range []string{
		"Continue toward the goal: finish migration",
		`report_goal_outcome(outcome="completed")`,
		`report_goal_outcome(outcome="blocked", reason="...")`,
	} {
		if !strings.Contains(instructions, want) {
			t.Errorf("RunInstructions() = %q, missing %q", instructions, want)
		}
	}
	for _, legacy := range []string{"update_goal", `status="complete"`} {
		if strings.Contains(instructions, legacy) {
			t.Errorf("RunInstructions() = %q, contains legacy contract %q", instructions, legacy)
		}
	}
}
