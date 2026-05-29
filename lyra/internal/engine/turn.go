package engine

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// runChatTurn drives one streaming chat turn end-to-end: compose the
// system prompt + user message, run the tool-loop, stream deltas to the
// observer (when one is attached), record each LLM round into the
// process budget, and assemble the result from that ledger. Shared by
// the main chat agent and the task sub-agent — they differ only in the
// typed input wrapper, the declared tool role, and whether an observer
// is present (sub-agents run without one, so their work is opaque to the
// parent turn's event stream; only the final answer flows back).
func (e *Engine) runChatTurn(ctx context.Context, pc *core.ProcessContext, message string, budget turnBudget) (ChatOutput, error) {
	req, err := pc.ChatWithActionTools(ctx)
	if err != nil {
		return ChatOutput{}, err
	}

	observer := ObserverFrom(pc.Options)
	stream := req.
		WithSystemPrompt(e.SystemPrompt(ctx)).
		WithUserPrompt(message).
		Stream()

	var (
		accumulated strings.Builder
		roundUsage  *chat.Usage
		roundModel  string
	)
	// recordRound commits the just-finished LLM round to the process
	// budget via the framework's invocation ledger; usage is read back
	// from there, not tallied locally.
	recordRound := func() {
		if roundUsage == nil {
			return
		}
		pc.RecordLLMInvocation(e.invocationFrom(roundModel, roundUsage))
		roundUsage, roundModel = nil, ""
	}
	for chunk, streamErr := range stream.Response(ctx) {
		if streamErr != nil {
			// Best-effort accounting: if the round's usage already
			// arrived before the error, record it (tokens were spent).
			// No-op when nothing arrived yet.
			recordRound()
			return ChatOutput{}, streamErr
		}
		if isToolRoundBoundary(chunk) {
			recordRound()
			// Enforce the per-turn budget at the round boundary — stop
			// before the next LLM call. The running totals come from the
			// process budget the recordRound above just updated.
			if budget.exceeded(pc) {
				return chatOutput(pc, accumulated.String(), true), nil
			}
		}
		if chunk != nil && chunk.Metadata != nil {
			if chunk.Metadata.Usage != nil {
				roundUsage = chunk.Metadata.Usage
			}
			if chunk.Metadata.Model != "" {
				roundModel = chunk.Metadata.Model
			}
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
	recordRound()
	return chatOutput(pc, accumulated.String(), false), nil
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
