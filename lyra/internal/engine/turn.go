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
	// Backstop a hung LLM connection (see llmCallTimeout): the deadline
	// rides the whole streaming round so a stuck read surfaces as a
	// stream error here rather than blocking the run forever.
	ctx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

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
		if chunk.IsToolResult() {
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
			if reasoning := chunk.ReasoningDelta(); reasoning != "" {
				observer.OnReasoningDelta(reasoning)
			}
		}
		delta := chunk.TextDelta()
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
