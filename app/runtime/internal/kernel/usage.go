package kernel

import (
	"cmp"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/accounting"
	"github.com/Tangerg/lynx/core/model/chat"
)

// turnBudget caps one turn by tokens, dollars, and/or tool-call rounds. A zero
// field means no cap on that dimension; the zero value is unbounded.
type turnBudget struct {
	MaxTokens  int64
	MaxCostUSD float64
	MaxSteps   int
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
// [accounting.Pricing] hook when configured (else zero — the chat layer gets no
// dollar figure from the provider). provider is the round's provider (the
// turn's selection), falling back to the engine default for a default /
// subtask turn that named none — pricing needs it to disambiguate a model id
// shared across providers. An empty model name (provider didn't report one)
// falls back to "unknown" so the per-model roll-up doesn't grow a blank-keyed
// entry.
func (e *Engine) invocationFrom(provider, model string, u *chat.Usage) core.LLMInvocation {
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
		inv.CostUSD = e.pricing(cmp.Or(provider, e.defaultProvider), model, u)
	}
	return inv
}

// turnOutput assembles the turn result from the process budget's
// invocation ledger: the total roll-up plus a per-model breakdown
// (insertion order preserved). Reading from the ledger — rather than a
// local tally — is the point: lyra uses the framework's accounting.
func turnOutput(pc *core.ProcessContext, reply string, stoppedOnBudget bool) TurnOutput {
	out := TurnOutput{Reply: reply, StoppedOnBudget: stoppedOnBudget}
	byModel := map[string]*accounting.ModelUsage{}
	var order []string
	for _, inv := range pc.Process.LLMInvocations() {
		out.Usage.AddInvocation(inv)
		out.CostUSD += inv.CostUSD
		m := byModel[inv.Model]
		if m == nil {
			m = &accounting.ModelUsage{Model: inv.Model}
			byModel[inv.Model] = m
			order = append(order, inv.Model)
		}
		m.TokenUsage.AddInvocation(inv)
		m.CostUSD += inv.CostUSD
	}
	for _, model := range order {
		out.UsageByModel = append(out.UsageByModel, *byModel[model])
	}
	return out
}
