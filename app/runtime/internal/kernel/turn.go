package kernel

import (
	"context"
	"strings"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model/chat"
)

// llmIdleTimeout is the silence window of a chat turn's hang backstop (see
// [stallContext]): as long as the stream keeps producing — reasoning tokens,
// tool rounds — the turn runs; only a provider gone SILENT this long (a real
// hang: no response, or an unparseable stream like the error body a no-access
// model returns) ends it, and the ctx cancel lets Go's HTTP stack interrupt a
// stuck tls.Read.
//
// Codex's model (stream_idle_timeout), deliberately NOT a total wall-clock cap:
// the turn wraps the WHOLE tool-loop and a delegated `task` sub-agent runs a
// full multi-round task inside one turn, so a total cap kills healthy long work
// (the earlier 2-min total cap cut reasoning models off mid-stream —
// run.outcome=errored "context deadline exceeded"). 5 min matches codex's
// default idle window. Normal work stays bounded by MaxBudget / MaxCostUSD,
// never this backstop.
const llmIdleTimeout = 5 * time.Minute

// stallContext derives a context cancelled when no progress is reported for
// idle — the silence watchdog behind [llmIdleTimeout]. keepAlive pushes the
// deadline out: call it on every unit of progress (each streamed chunk). stop
// releases the timer + context. Cancellation is idempotent, so a fired watchdog
// and an explicit stop coexist safely. Mirrors [context.WithCancel]'s
// (ctx, cancel) shape with an extra keepAlive.
func stallContext(parent context.Context, idle time.Duration) (ctx context.Context, keepAlive, stop func()) {
	ctx, cancel := context.WithCancel(parent)
	t := time.AfterFunc(idle, cancel)
	return ctx, func() { t.Reset(idle) }, func() { t.Stop(); cancel() }
}

// runChatTurn drives one streaming chat turn end-to-end: compose the
// system prompt + user message (with any image attachments), run the
// tool-loop, stream deltas to the observer, record each LLM round into the
// process budget, and assemble the result. HITL interrupt / resume is
// handled by the tool middleware's [tool.ParkStore]; when none is
// configured, the engine intercepts [chat.FinishReasonInterrupt] chunks as
// a fallback.
func (e *Engine) runChatTurn(ctx context.Context, pc *core.ProcessContext, message string, images []*media.Media, budget turnBudget) (ChatOutput, error) {
	// A silent provider ends the turn (llmIdleTimeout); every chunk below calls
	// keepAlive to push the deadline out, so a healthy long turn never trips it.
	ctx, keepAlive, stop := stallContext(ctx, llmIdleTimeout)
	defer stop()

	req, err := pc.ChatWithActionTools(ctx)
	if err != nil {
		return ChatOutput{}, err
	}

	observer := observerFrom(pc.Options)
	sysPrompt := e.SystemPrompt(ctx)
	inflightTail := inflightTailStore{bb: pc.Blackboard}
	var stream *chat.ClientStreamer
	if tail, ok := inflightTail.Load(); ok {
		inflightTail.Clear()
		msgs := append([]chat.Message{chat.NewSystemMessage(sysPrompt)}, tail...)
		stream = req.WithMessages(msgs...).Stream()
	} else {
		req = req.WithSystemPrompt(sysPrompt)
		// Images attach to the user message via a prompt template (the
		// text-only path keeps the plain WithUserPrompt). The template
		// renders the text and carries the media into UserMessage.Media,
		// which the memory middleware persists and the provider adapter
		// lowers to image content blocks.
		if len(images) > 0 {
			req = req.WithUserPromptTemplate(chat.NewPromptTemplate(message).WithMedia(images...))
		} else {
			req = req.WithUserPrompt(message)
		}
		stream = req.Stream()
	}

	var (
		accumulated    strings.Builder
		roundUsage     *chat.Usage
		roundModel     string
		cumulative     TokenUsage
		cumulativeCost float64
	)
	recordRound := func() {
		if roundUsage == nil {
			return
		}
		inv := e.invocationFrom(roundModel, roundUsage)
		pc.RecordLLMInvocation(inv)
		// Fold into the running roll-up the same way chatOutput sums the ledger,
		// so the mid-run readout matches the final TurnEnd total exactly.
		cumulative.add(inv)
		cumulativeCost += inv.CostUSD
		roundUsage, roundModel = nil, ""
		if observer != nil {
			observer.OnUsage(cumulative, cumulativeCost)
		}
	}
	for chunk, streamErr := range stream.Response(ctx) {
		keepAlive() // progress — push the silence deadline out
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
