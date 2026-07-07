package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// Usage returns the subtree-aggregated cost / token / action totals.
// Cost and tokens come from [AgentProcess.RecordUsage] calls (zero
// unless integration code wires them up). The action count is the
// recursive sum of every History across this process and all child
// processes spawned via [Platform.CreateChildProcess]. [core.BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *AgentProcess) Usage() (cost float64, tokens int, actions int) {
	return p.budget.usage(p.state.historyLen())
}

// RecordUsage adds a single LLM call's cost (USD) and token count to
// this process's running totals. Integration code calls this from an
// LLM-client adapter that knows the per-model rate. The framework
// itself never invents numbers here.
func (p *AgentProcess) RecordUsage(cost float64, tokens int) {
	p.RecordLLMInvocation(core.LLMInvocation{CostUSD: cost, PromptTokens: int64(tokens)})
}

// RecordLLMInvocation appends a fully-attributed LLM call to this
// process's history. Integration code calls this once per LLM
// response with the model id, provider, cost, and token breakdown.
// It also publishes an [event.LLMInvocationRecorded] so listeners can
// audit per-call cost/tokens off the event stream.
func (p *AgentProcess) RecordLLMInvocation(inv core.LLMInvocation) {
	if inv.Timestamp.IsZero() {
		inv.Timestamp = core.Now()
	}
	p.budget.recordLLMInvocation(inv)
	p.publishEvent(context.Background(), event.LLMInvocationRecorded{
		BaseEvent:  p.baseEvent(),
		Invocation: inv,
	})
}

// RecordEmbeddingInvocation appends a fully-attributed embedding
// call. Mirrors RecordLLMInvocation for the embeddings path, including
// the [event.EmbeddingInvocationRecorded] publish.
func (p *AgentProcess) RecordEmbeddingInvocation(inv core.EmbeddingInvocation) {
	if inv.Timestamp.IsZero() {
		inv.Timestamp = core.Now()
	}
	p.budget.recordEmbeddingInvocation(inv)
	p.publishEvent(context.Background(), event.EmbeddingInvocationRecorded{
		BaseEvent:  p.baseEvent(),
		Invocation: inv,
	})
}

// LLMInvocations returns the subtree-aggregated LLM invocation
// history in chronological-per-process order.
func (p *AgentProcess) LLMInvocations() []core.LLMInvocation {
	return p.budget.llmHistory()
}

// EmbeddingInvocations returns the subtree-aggregated embedding
// invocation history.
func (p *AgentProcess) EmbeddingInvocations() []core.EmbeddingInvocation {
	return p.budget.embeddingHistory()
}
