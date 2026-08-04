package goal

import "github.com/Tangerg/lynx/app/runtime/internal/application/goals"

// Prompt renders the model-facing instruction for an autonomous Goal Run. Goal
// lifecycle decisions stay in Application; this package owns the wording and
// the outcome-reporting tool contract it names.
func Prompt(input goals.PromptInput) string {
	prefix := input.Objective
	if input.Continuing {
		prefix = "Continue toward the goal: " + input.Objective
	}
	return prefix + "\n\n(You are running autonomously toward this Goal — you do not need to wait for the user. Take one concrete next step. Call report_goal_outcome(outcome=\"completed\") only when the entire objective is achieved and verified, or report_goal_outcome(outcome=\"blocked\", reason=\"...\") if progress genuinely requires the user or an external state change. Otherwise keep working; the Goal loop will provide the next Run.)"
}
