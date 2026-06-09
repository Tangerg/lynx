package engine

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
)

// runChatTurn drives one streaming chat turn end-to-end: compose the
// system prompt + user message, run the tool-loop, stream deltas to the
// observer, record each LLM round into the process budget, and assemble
// the result. HITL interrupt / resume is handled by the tool
// middleware's [tool.ParkStore]; when none is configured, the engine
// intercepts [chat.FinishReasonInterrupt] chunks as a fallback.
func (e *Engine) runChatTurn(ctx context.Context, pc *core.ProcessContext, message string, budget turnBudget) (ChatOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, llmCallTimeout)
	defer cancel()

	req, err := pc.ChatWithActionTools(ctx)
	if err != nil {
		return ChatOutput{}, err
	}
	ctx = tool.WithParkKey(ctx, pc.Process.ID())

	observer := observerFrom(pc.Options)
	sysPrompt := e.SystemPrompt(ctx)
	inflightTail := inflightTailStore{bb: pc.Blackboard}
	var stream *chat.ClientStreamer
	if tail, ok := inflightTail.Load(); ok {
		inflightTail.Clear()
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
	recordRound := func() {
		if roundUsage == nil {
			return
		}
		pc.RecordLLMInvocation(e.invocationFrom(roundModel, roundUsage))
		roundUsage, roundModel = nil, ""
	}
	for chunk, streamErr := range stream.Response(ctx) {
		if streamErr != nil {
			recordRound()
			return ChatOutput{}, streamErr
		}
		// Fallback: when no ParkStore is configured, the tool middleware
		// yields FinishReasonInterrupt chunks. Intercept and save to the
		// process blackboard so resume works. With a ParkStore configured
		// the middleware never yields these — the engine does nothing.
		if isInterruptResult(chunk) {
			recordRound()
			inflightTail.Save(chunk.Result)
			continue
		}
		if chunk.IsToolResult() {
			recordRound()
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
