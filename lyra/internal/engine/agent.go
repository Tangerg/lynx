package engine

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
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
// bash freely within one turn.
//
// The body uses Stream rather than Call so each text chunk surfaces
// to [ToolObserver.OnMessageDelta] as it arrives — transport
// adapters get a real streaming experience instead of one
// pre-buffered MessageDelta. Tool-call rounds still go through the
// same ToolMiddleware loop; tool events surface via the
// ToolDecorator path independently of the text-delta path.
func buildChatAgent() *core.Agent {
	return agent.New("lyra-chat").
		Description("single-turn LLM chat with the default coding tool set").
		Actions(agent.NewAction("chat",
			func(ctx context.Context, pc *core.ProcessContext, in ChatInput) (ChatOutput, error) {
				req, err := pc.ChatWithActionTools(ctx)
				if err != nil {
					return ChatOutput{}, err
				}

				observer := ObserverFrom(pc.Options)
				stream := req.WithUserPrompt(in.Message).Stream()

				var accumulated strings.Builder
				for chunk, streamErr := range stream.Response(ctx) {
					if streamErr != nil {
						return ChatOutput{}, streamErr
					}
					delta := extractTextDelta(chunk)
					if delta == "" {
						continue
					}
					accumulated.WriteString(delta)
					if observer != nil {
						observer.OnMessageDelta(delta)
					}
				}
				return ChatOutput{Reply: accumulated.String()}, nil
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

// extractTextDelta pulls the text the model emitted in this chunk
// (its TextPart bodies, joined). Returns "" for chunks that don't
// carry assistant text — tool-call rounds (AssistantMessage has
// only ToolCallParts), tool-injection rounds (Result.AssistantMessage
// is nil and only Result.ToolMessage is populated), and any
// reasoning-only or empty chunk the provider sends.
func extractTextDelta(resp *chat.Response) string {
	if resp == nil || resp.Result == nil || resp.Result.AssistantMessage == nil {
		return ""
	}
	return resp.Result.AssistantMessage.JoinedText()
}
