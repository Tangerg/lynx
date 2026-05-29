package engine

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

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
// and its full token usage (cache breakdown included). Supply via
// [Config.Pricing] to populate cost on invocations / ChatOutput /
// TurnEnd; nil leaves cost at zero. lyra builds it from the chat model's
// [chat.ModelMetadata].Pricing (see config.BuildChatClient); the rate
// table behind that lives in the model adapters' pricing catalog — the
// engine never invents cost numbers.
type Pricing func(model string, usage *chat.Usage) float64

// turnBudget caps one turn by tokens and/or dollars. A zero field means
// no cap on that dimension; the zero value is unbounded.
type turnBudget struct {
	MaxTokens  int64
	MaxCostUSD float64
}

// exceeded reports whether the turn has hit either ceiling, reading the
// running cost / token totals the process budget has aggregated from
// recorded invocations so far (subtree-inclusive, so a sub-agent's spend
// counts toward the parent).
func (b turnBudget) exceeded(pc *core.ProcessContext) bool {
	cost, tokens, _ := pc.Process.Usage()
	return (b.MaxTokens > 0 && int64(tokens) >= b.MaxTokens) ||
		(b.MaxCostUSD > 0 && cost >= b.MaxCostUSD)
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
		inv.CostUSD = e.pricing(model, u)
	}
	return inv
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
		addUsage(&out.Usage, inv)
		out.CostUSD += inv.CostUSD
		m := byModel[inv.Model]
		if m == nil {
			m = &ModelUsage{Model: inv.Model}
			byModel[inv.Model] = m
			order = append(order, inv.Model)
		}
		addUsage(&m.TokenUsage, inv)
		m.CostUSD += inv.CostUSD
	}
	for _, model := range order {
		out.UsageByModel = append(out.UsageByModel, *byModel[model])
	}
	return out
}

// addUsage folds one invocation's token counts into a running roll-up.
// Shared by the total and per-model accumulators in chatOutput so the
// field list lives in one place.
func addUsage(dst *TokenUsage, inv core.LLMInvocation) {
	dst.PromptTokens += inv.PromptTokens
	dst.CompletionTokens += inv.CompletionTokens
	dst.ReasoningTokens += inv.ReasoningTokens
}
