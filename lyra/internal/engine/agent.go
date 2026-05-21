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

// buildChatAgent constructs the chat agent: one action ("chat") that
// asks the LLM with the coding tool set wired in.
//
// The Action declares [ToolRoleCoding] so the runtime resolves the
// coding tool group at dispatch time; the body calls
// [core.ProcessContext.ChatWithActionTools] which composes the
// chat.NewToolMiddleware tool-loop on top of platform guardrails.
// The model can therefore call read / write / edit / glob / grep /
// bash freely within one turn — every tool call lands on the lynx
// event bus so [chat.Service] can fan it out as ToolCallStart /
// ToolCallEnd events.
func buildChatAgent() *core.Agent {
	return agent.New("lyra-chat").
		Description("single-turn LLM chat with the default coding tool set").
		Actions(agent.NewAction("chat",
			func(ctx context.Context, pc *core.ProcessContext, in ChatInput) (ChatOutput, error) {
				req, err := pc.ChatWithActionTools(ctx)
				if err != nil {
					return ChatOutput{}, err
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
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor(ToolRoleCoding),
			},
		)).
		Goals(agent.GoalProducing[ChatOutput](core.Goal{
			Description: "single-turn reply produced",
		})).
		Build()
}
