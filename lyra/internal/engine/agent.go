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

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. When
	// exceeded the action stops cleanly after the current round —
	// before paying for the next LLM call — and reports the partial
	// reply with [ChatOutput.StoppedOnBudget] set.
	MaxBudget int64
}

// ChatOutput is the typed output of one turn. Reply is the
// assistant's final text; Usage is the per-turn token roll-up
// summed across every LLM round in the tool loop.
type ChatOutput struct {
	Reply string
	Usage TokenUsage

	// StoppedOnBudget is true when the turn ended because it hit
	// [ChatInput.MaxBudget] rather than the model finishing. Reply
	// holds whatever text accumulated up to the stop.
	StoppedOnBudget bool
}

// TokenUsage is the per-turn token total — sum across every LLM
// round in the tool loop. ReasoningTokens stays zero for providers
// that don't report it (it's a subset of CompletionTokens, not an
// addition).
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64
}

// add folds one round's [chat.Usage] into the running per-turn
// total. nil usage is a no-op so callers don't need a guard.
func (t *TokenUsage) add(u *chat.Usage) {
	if u == nil {
		return
	}
	t.PromptTokens += u.PromptTokens
	t.CompletionTokens += u.CompletionTokens
	if u.ReasoningTokens != nil {
		t.ReasoningTokens += *u.ReasoningTokens
	}
}

// total is prompt + completion — the figure a token budget caps.
// ReasoningTokens is a subset of CompletionTokens (not an addition),
// so it's already counted.
func (t TokenUsage) total() int64 {
	return t.PromptTokens + t.CompletionTokens
}

// buildChatAgent constructs the chat agent owned by this Engine.
// The Action's closure captures `e` so it can reach the engine's
// memory service for system-prompt composition without an extra
// parameter passed through every turn.
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
func (e *Engine) buildChatAgent() *core.Agent {
	return agent.New("lyra-chat").
		Description("single-turn LLM chat with the default coding tool set").
		Actions(agent.NewAction("chat",
			func(ctx context.Context, pc *core.ProcessContext, in ChatInput) (ChatOutput, error) {
				req, err := pc.ChatWithActionTools(ctx)
				if err != nil {
					return ChatOutput{}, err
				}

				observer := ObserverFrom(pc.Options)
				stream := req.
					WithSystemPrompt(e.SystemPrompt(ctx)).
					WithUserPrompt(in.Message).
					Stream()

				var (
					accumulated strings.Builder
					totals      TokenUsage
					roundUsage  *chat.Usage
				)
				for chunk, streamErr := range stream.Response(ctx) {
					if streamErr != nil {
						return ChatOutput{}, streamErr
					}
					if isToolRoundBoundary(chunk) {
						totals.add(roundUsage)
						roundUsage = nil
						// Enforce the per-turn token budget at the round
						// boundary — stop here, before the next LLM call,
						// rather than after paying for it. Returning ends
						// the stream iterator (no further rounds run).
						if in.MaxBudget > 0 && totals.total() >= in.MaxBudget {
							return ChatOutput{
								Reply:           accumulated.String(),
								Usage:           totals,
								StoppedOnBudget: true,
							}, nil
						}
					}
					if chunk != nil && chunk.Metadata != nil && chunk.Metadata.Usage != nil {
						roundUsage = chunk.Metadata.Usage
					}
					if observer != nil {
						if reasoning := extractReasoningDelta(chunk); reasoning != "" {
							observer.OnReasoningDelta(reasoning)
						}
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
				totals.add(roundUsage)
				return ChatOutput{Reply: accumulated.String(), Usage: totals}, nil
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

// isToolRoundBoundary reports whether resp is the synthetic
// tool-result chunk the chat ToolMiddleware yields between LLM
// rounds. The middleware emits a Response with Result.ToolMessage
// set and Result.AssistantMessage nil to surface the tool return
// to stream consumers — that's our cue that the prior round is
// over and any pending Usage should be committed to the per-turn
// total before the next round overwrites it.
func isToolRoundBoundary(resp *chat.Response) bool {
	return resp != nil &&
		resp.Result != nil &&
		resp.Result.AssistantMessage == nil &&
		resp.Result.ToolMessage != nil
}

// extractReasoningDelta pulls extended-thinking text from one
// streamed chunk (its ReasoningPart bodies, joined). Returns ""
// for chunks without reasoning content — text-only or tool-only
// rounds. Mirrors [extractTextDelta] in shape but reads the
// reasoning subset instead of the final-text subset.
func extractReasoningDelta(resp *chat.Response) string {
	if resp == nil || resp.Result == nil || resp.Result.AssistantMessage == nil {
		return ""
	}
	return resp.Result.AssistantMessage.JoinedReasoning()
}
