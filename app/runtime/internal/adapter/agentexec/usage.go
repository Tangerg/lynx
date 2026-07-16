package agentexec

import (
	"cmp"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/chat"
)

// modelAttribution supplies provider identity and pricing. The agent
// runtime owns model, token, action, duration, and timestamp fields.
func (e *Engine) modelAttribution(provider string) core.ModelAttributionFunc {
	return func(response *chat.Response) core.ModelAttribution {
		resolvedProvider := cmp.Or(provider, e.defaultProvider)
		model := cmp.Or(response.Model, "unknown")
		attribution := core.ModelAttribution{Provider: resolvedProvider}
		if e.pricing != nil {
			attribution.CostUSD = e.pricing(resolvedProvider, model, &response.Usage)
		}
		return attribution
	}
}

// tokenUsageOf maps one SDK invocation's token counts onto the domain
// [accounting.TokenUsage] value, so the accounting domain never imports the
// agent SDK (§16 rule 2).
func tokenUsageOf(call core.ModelCall) accounting.TokenUsage {
	return accounting.TokenUsage{
		PromptTokens:     call.PromptTokens,
		CompletionTokens: call.CompletionTokens,
		ReasoningTokens:  call.ReasoningTokens,
		CacheReadTokens:  call.CacheReadInputTokens,
		CacheWriteTokens: call.CacheWriteInputTokens,
	}
}

// turnOutput assembles the turn result from the process budget's
// invocation ledger: the total roll-up plus a per-model breakdown
// (insertion order preserved). Reading from the ledger — rather than a
// local tally — is the point: lyra uses the framework's accounting.
func turnOutput(pc *core.ProcessContext, reply string, stopReason StopReason) TurnOutput {
	output := TurnOutput{Reply: reply, StopReason: stopReason}
	byModel := map[string]*accounting.ModelUsage{}
	var order []string
	for _, call := range pc.Process().ModelCalls() {
		usage := tokenUsageOf(call)
		output.Usage.Add(usage)
		output.CostUSD += call.CostUSD
		modelUsage := byModel[call.Model]
		if modelUsage == nil {
			modelUsage = &accounting.ModelUsage{Model: call.Model}
			byModel[call.Model] = modelUsage
			order = append(order, call.Model)
		}
		modelUsage.TokenUsage.Add(usage)
		modelUsage.CostUSD += call.CostUSD
	}
	for _, model := range order {
		output.UsageByModel = append(output.UsageByModel, *byModel[model])
	}
	return output
}
