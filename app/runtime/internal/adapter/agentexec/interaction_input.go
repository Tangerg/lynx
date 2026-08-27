package agentexec

import (
	"context"

	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/interrupt"
)

// RequireToolInput is the execution ACL between Runtime Tools and Agent
// Interaction input. It enriches question prompts with the same stable Tool
// invocation identity used by ToolCallStarted before delegating the wait and
// restore protocol to interactioninput.
//
// Built-in Tools deliberately do not depend on Agent attribution. Keeping this
// join here lets the Run reducer correlate semantically normalized arguments
// (for example omitted empty option descriptions) without guessing identity.
func RequireToolInput(
	ctx context.Context,
	key string,
	prompt runs.Interrupt,
) (interrupt.Resolution, error) {
	if prompt.Question != nil && prompt.Question.CallID == "" {
		if invocation, present := interaction.ToolInvocationFromContext(ctx); present {
			question := *prompt.Question
			question.CallID = toolInvocationID(invocation)
			prompt.Question = &question
		}
	}
	return interactioninput.Require(ctx, key, prompt)
}
