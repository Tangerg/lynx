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

	observer := observerFrom(pc.Options)

	// HITL R-model resume: the run parked for human input on the same process.
	// When a parked tail is present (the interrupting round's assistant
	// tool-call message + any partial results, captured below), feed it back so
	// the tool loop continues AT the still-pending (now-resolved) call — the
	// model is NOT re-invoked for that round. The system header rides as a
	// MESSAGE (not the prompt template) so the client doesn't inject a synthetic
	// user seed; the inner memory middleware splices the stored history in
	// front, and the (assistant, tool) pair persists atomically when the round
	// completes. A fresh turn (no tail) adds the user message normally.
	sysPrompt := e.SystemPrompt(ctx)
	var stream *chat.ClientStreamer
	if tail, ok := loadInflightTail(pc.Blackboard); ok {
		clearInflightTail(pc.Blackboard) // consume the tail
		msgs := append([]chat.Message{chat.NewSystemMessage(sysPrompt)}, tail...)
		stream = req.WithMessages(msgs...).Stream()
	} else {
		stream = req.
			WithSystemPrompt(sysPrompt).
			WithUserPrompt(message).
			Stream()
	}

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
		// HITL interrupt: the tool loop hands back the resumable tail (the
		// round's assistant tool-call message + any partial results) as a
		// FinishReasonInterrupt response, then a ToolHalt error (handled above).
		// Park the tail so the resuming re-tick continues AT the pending call —
		// it is not assistant text and never reaches the budget/observer below.
		if isInterruptResult(chunk) {
			recordRound()
			saveInflightTail(pc.Blackboard, chunk.Result)
			continue
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
