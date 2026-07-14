package agentexec

import (
	"cmp"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
)

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
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
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

// tokenUsageOf maps one SDK invocation's token counts onto the domain
// [accounting.TokenUsage] value, so the accounting domain never imports the
// agent SDK (§16 rule 2).
func tokenUsageOf(inv core.LLMInvocation) accounting.TokenUsage {
	return accounting.TokenUsage{
		PromptTokens:     inv.PromptTokens,
		CompletionTokens: inv.CompletionTokens,
		ReasoningTokens:  inv.ReasoningTokens,
		CacheReadTokens:  inv.CacheReadInputTokens,
		CacheWriteTokens: inv.CacheWriteInputTokens,
	}
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
		usage := tokenUsageOf(inv)
		out.Usage.Add(usage)
		out.CostUSD += inv.CostUSD
		m := byModel[inv.Model]
		if m == nil {
			m = &accounting.ModelUsage{Model: inv.Model}
			byModel[inv.Model] = m
			order = append(order, inv.Model)
		}
		m.TokenUsage.Add(usage)
		m.CostUSD += inv.CostUSD
	}
	for _, model := range order {
		out.UsageByModel = append(out.UsageByModel, *byModel[model])
	}
	return out
}
