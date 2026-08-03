package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/goals"
)

func TestGoalPromptUsesOutcomeReportingContract(t *testing.T) {
	prompt := GoalPrompt(goals.PromptInput{Objective: "finish migration", Continuing: true})
	for _, want := range []string{
		"Continue toward the goal: finish migration",
		`report_goal_outcome(outcome="completed")`,
		`report_goal_outcome(outcome="blocked", reason="...")`,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("GoalPrompt() = %q, missing %q", prompt, want)
		}
	}
	for _, legacy := range []string{"update_goal", `status="complete"`} {
		if strings.Contains(prompt, legacy) {
			t.Errorf("GoalPrompt() = %q, contains legacy contract %q", prompt, legacy)
		}
	}
}
