package agentexec

import "github.com/Tangerg/lynx/app/runtime/internal/application/goals"

// GoalPrompt renders the model-facing instruction for an autonomous goal turn.
// Goal lifecycle decisions stay in application/goals; this execution adapter
// owns the wording and the update_goal tool contract presented to the model.
func GoalPrompt(input goals.PromptInput) string {
	prefix := input.Objective
	if input.Continuing {
		prefix = "Continue toward the goal: " + input.Objective
	}
	return prefix + "\n\n(You are running autonomously toward this goal — you do not need to wait for the user. Take one concrete next step. Call update_goal(status=\"complete\") only when the whole goal is done and verified, or update_goal(status=\"blocked\", reason=\"...\") if you genuinely cannot proceed without the user. Otherwise just keep working.)"
}
