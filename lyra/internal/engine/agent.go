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

// ChatOutput is the typed output of one turn. Reply is the assistant's
// final text. Usage / UsageByModel / CostUSD are read back from the
// process budget — the agent framework's invocation ledger — rather
// than a hand-rolled tally: the action records each LLM round via
// [core.ProcessContext.RecordLLMInvocation], and these fields are the
// rolled-up view.
type ChatOutput struct {
	Reply string
	Usage TokenUsage

	// UsageByModel breaks Usage down per served model — the lynx analog
	// of the SDK's modelUsage. One entry for a plain single-model turn;
	// several once a turn spans models (tool rounds routed elsewhere,
	// sub-agents).
	UsageByModel []ModelUsage

	// CostUSD is the turn's total dollar cost, summed from the recorded
	// invocations. Zero unless a [Pricing] func is configured (providers
	// don't return a dollar figure on the chat path); see [Config.Pricing].
	CostUSD float64

	// StoppedOnBudget is true when the turn ended because it hit
	// [ChatInput.MaxBudget] rather than the model finishing. Reply
	// holds whatever text accumulated up to the stop.
	StoppedOnBudget bool
}

// TokenUsage is a token roll-up. ReasoningTokens is the chain-of-thought
// subset of CompletionTokens (not an addition), so total counts only
// prompt + completion.
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	ReasoningTokens  int64
}

// total is prompt + completion — the figure a token budget caps.
func (t TokenUsage) total() int64 {
	return t.PromptTokens + t.CompletionTokens
}

// ModelUsage is one model's slice of a turn's tokens + cost — the lynx
// analog of an SDK modelUsage map entry.
type ModelUsage struct {
	Model string
	TokenUsage
	CostUSD float64
}

// Pricing computes the USD cost of one LLM round from the served model
// and its token usage. Supply via [Config.Pricing] to populate cost on
// invocations / ChatOutput / TurnEnd; nil leaves cost at zero. The
// per-provider rate table behind it is the caller's to maintain — the
// framework never invents cost numbers.
type Pricing func(model string, usage TokenUsage) float64

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
				return e.runChatTurn(ctx, pc, in.Message, in.MaxBudget)
			},
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor(ToolRoleCoding),
				ToolLoop:   recoverToolLoop(),
			},
		)).
		Goals(agent.GoalProducing[ChatOutput](core.Goal{
			Description: "single-turn reply produced",
		})).
		Build()
}

// TaskInput is the argument schema the model fills to call the `task`
// tool: one self-contained subtask description. lyra runs it in a fresh
// sub-agent (isolated context, the coding tools minus `task`) and hands
// back the sub-agent's final reply.
type TaskInput struct {
	Prompt string `json:"prompt"`
}

// buildSubtaskAgent constructs the agent behind the `task` delegation
// tool. Same chat body as the main agent, but: (1) named "task" so the
// derived tool is `task`; (2) declares [ToolRoleSubtask] — the coding
// tools WITHOUT `task`, so a subtask can't recurse into another
// delegation; (3) its goal produces just the reply string, so the tool
// result handed to the parent model is the answer text, not a ChatOutput
// blob. Its LLM rounds still record into the process budget, which
// aggregates up the subtree into the parent turn's usage roll-up.
func (e *Engine) buildSubtaskAgent() *core.Agent {
	return agent.New("task").
		Description("Delegate a self-contained subtask to a fresh sub-agent that has the coding " +
			"tools. Use for focused, separable work (investigate a question, draft a file) so the " +
			"main conversation stays uncluttered. Returns the sub-agent's final answer.").
		Actions(agent.NewAction("subtask",
			func(ctx context.Context, pc *core.ProcessContext, in TaskInput) (string, error) {
				// maxBudget=0: a subtask runs without its own token cap.
				// It isn't unbounded at the turn level, though — its
				// usage records into the child budget, which aggregates
				// into the parent's subtree, so the parent turn's next
				// round-boundary budget check (processTokens reads the
				// subtree total) stops further work once the subtask
				// pushes the parent over MaxBudget.
				out, err := e.runChatTurn(ctx, pc, in.Prompt, 0)
				if err != nil {
					return "", err
				}
				return out.Reply, nil
			},
			core.ActionConfig{
				ToolGroups: core.ToolRolesFor(ToolRoleSubtask),
				ToolLoop:   recoverToolLoop(),
			},
		)).
		Goals(agent.GoalProducing[string](core.Goal{
			Description: "subtask answer produced",
		})).
		Build()
}

// recoverToolLoop is the tool-loop policy both the chat agent and the
// task sub-agent use: recover from a hallucinated tool name (feed back
// the real tool list so the model re-picks) or an empty reply (one
// nudge) instead of aborting. No-op on a well-behaved turn.
func recoverToolLoop() chat.ToolLoopConfig {
	return chat.ToolLoopConfig{FeedbackOnUnknownTool: true, FeedbackOnEmptyResponse: true}
}

// runChatTurn drives one streaming chat turn end-to-end: compose the
// system prompt + user message, run the tool-loop, stream deltas to the
// observer (when one is attached), record each LLM round into the
// process budget, and assemble the result from that ledger. Shared by
// the main chat agent and the task sub-agent — they differ only in the
// typed input wrapper, the declared tool role, and whether an observer
// is present (sub-agents run without one, so their work is opaque to the
// parent turn's event stream; only the final answer flows back).
func (e *Engine) runChatTurn(ctx context.Context, pc *core.ProcessContext, message string, maxBudget int64) (ChatOutput, error) {
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
			// Enforce the per-turn token budget at the round boundary —
			// stop before the next LLM call. The running total comes from
			// the process budget the recordRound above just updated.
			if maxBudget > 0 && processTokens(pc) >= maxBudget {
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

// invocationFrom maps a streamed round's usage + served model to the
// framework's [core.LLMInvocation]. Cost is filled from the engine's
// [Pricing] hook when configured (else zero — the chat layer gets no
// dollar figure from the provider). An empty model name (provider
// didn't report one) falls back to "unknown" so the per-model roll-up
// doesn't grow a blank-keyed entry.
func (e *Engine) invocationFrom(model string, u *chat.Usage) core.LLMInvocation {
	if model == "" {
		model = "unknown"
	}
	inv := core.LLMInvocation{
		Model:            model,
		Action:           "chat",
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
	}
	if u.ReasoningTokens != nil {
		inv.ReasoningTokens = *u.ReasoningTokens
	}
	if u.CacheReadInputTokens != nil {
		inv.CacheReadInputTokens = *u.CacheReadInputTokens
	}
	if u.CacheWriteInputTokens != nil {
		inv.CacheWriteInputTokens = *u.CacheWriteInputTokens
	}
	if e.pricing != nil {
		inv.Cost = e.pricing(model, TokenUsage{
			PromptTokens:     inv.PromptTokens,
			CompletionTokens: inv.CompletionTokens,
			ReasoningTokens:  inv.ReasoningTokens,
		})
	}
	return inv
}

// processTokens is the running prompt+completion total the process
// budget has aggregated from recorded invocations so far.
func processTokens(pc *core.ProcessContext) int64 {
	_, tokens, _ := pc.Process.Usage()
	return int64(tokens)
}

// chatOutput assembles the turn result from the process budget's
// invocation ledger: the total roll-up plus a per-model breakdown
// (insertion order preserved). Reading from the ledger — rather than a
// local tally — is the point: lyra uses the framework's accounting.
func chatOutput(pc *core.ProcessContext, reply string, stoppedOnBudget bool) ChatOutput {
	out := ChatOutput{Reply: reply, StoppedOnBudget: stoppedOnBudget}
	byModel := map[string]*ModelUsage{}
	var order []string
	for _, inv := range pc.Process.LLMInvocations() {
		out.Usage.PromptTokens += inv.PromptTokens
		out.Usage.CompletionTokens += inv.CompletionTokens
		out.Usage.ReasoningTokens += inv.ReasoningTokens
		out.CostUSD += inv.Cost
		m := byModel[inv.Model]
		if m == nil {
			m = &ModelUsage{Model: inv.Model}
			byModel[inv.Model] = m
			order = append(order, inv.Model)
		}
		m.PromptTokens += inv.PromptTokens
		m.CompletionTokens += inv.CompletionTokens
		m.ReasoningTokens += inv.ReasoningTokens
		m.CostUSD += inv.Cost
	}
	for _, model := range order {
		out.UsageByModel = append(out.UsageByModel, *byModel[model])
	}
	return out
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
