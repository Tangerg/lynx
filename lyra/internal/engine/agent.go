package engine

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
)

// ChatInput is the typed input to the M1 single-turn chat agent. It
// carries the user's message verbatim; future milestones extend with
// session context, tool selection hints, etc.
type ChatInput struct {
	Message string
}

// ChatOutput is the typed output. M1 just carries the assistant's
// reply text; later milestones add tool_calls / reasoning / sources.
type ChatOutput struct {
	Reply string
}

// buildChatAgent constructs the M1 chat agent: one action ("chat")
// that asks the LLM and returns the reply text. No tools, no planner
// flag — the framework defaults to GOAP A*, finds the one-step plan
// trivially.
func buildChatAgent() *core.Agent {
	return agent.New("lyra-chat").
		Description("single-turn LLM chat — the M1 walking-skeleton agent").
		Actions(agent.NewAction("chat",
			func(ctx context.Context, pc *core.ProcessContext, in ChatInput) (ChatOutput, error) {
				req := pc.Chat()
				if req == nil {
					return ChatOutput{}, errChatClientMissing
				}
				text, _, err := req.
					WithUserPrompt(in.Message).
					Call().
					Text(ctx)
				if err != nil {
					return ChatOutput{}, err
				}
				return ChatOutput{Reply: text}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[ChatOutput](core.Goal{
			Description: "single-turn reply produced",
		})).
		Build()
}
