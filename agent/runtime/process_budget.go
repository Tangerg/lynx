package runtime

import (
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// processBudget aggregates "what did this process and its descendants
// cost?" Used by [core.BudgetPolicy] (and any custom
// [core.EarlyTerminationPolicy]) so a parent's budget governs its entire
// delegation tree.
//
// Cost / token totals are populated by integration code via
// [Process.RecordUsage] (lumped) or per-call [Process.RecordLLMInvocation]
// / [Process.RecordEmbeddingInvocation]; the framework itself never
// invents numbers (it doesn't know per-model rates). The action count
// comes from [processState]'s history, threaded through Usage().
//
// Concurrency: writes are protected by AgentProcess's main mutex (held
// by record* / addChild). Reads in Usage() walk the children
// recursively under RLock — the lock graph is a tree (parent →
// children, never the reverse) so no deadlock is possible.
type processBudget struct {
	lock                 *sync.RWMutex // points at processState.mu — set by AgentProcess constructor
	children             []*AgentProcess
	ownCost              float64
	ownTokens            int
	llmInvocations       []core.LLMInvocation
	embeddingInvocations []core.EmbeddingInvocation
}

// recordUsage adds a single flat (cost, tokens) pair without per-call
// detail. Equivalent to recordLLMInvocation with an LLMInvocation
// carrying only Cost + PromptTokens; kept as a convenience for code
// that doesn't track per-model rates.
func (b *processBudget) recordUsage(cost float64, tokens int) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += cost
	b.ownTokens += tokens
	b.llmInvocations = append(b.llmInvocations, core.LLMInvocation{
		Timestamp:    time.Now(),
		Cost:         cost,
		PromptTokens: int64(tokens),
	})
}

// recordLLMInvocation appends a fully-attributed LLM call to history
// and rolls its cost/tokens into the budget. Timestamp defaults to
// time.Now() when unset by the caller.
func (b *processBudget) recordLLMInvocation(inv core.LLMInvocation) {
	if inv.Timestamp.IsZero() {
		inv.Timestamp = time.Now()
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += inv.Cost
	b.ownTokens += int(inv.PromptTokens + inv.CompletionTokens)
	b.llmInvocations = append(b.llmInvocations, inv)
}

// recordEmbeddingInvocation appends an embedding call and rolls its
// cost/tokens into the budget.
func (b *processBudget) recordEmbeddingInvocation(inv core.EmbeddingInvocation) {
	if inv.Timestamp.IsZero() {
		inv.Timestamp = time.Now()
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += inv.Cost
	b.ownTokens += int(inv.InputTokens)
	b.embeddingInvocations = append(b.embeddingInvocations, inv)
}

// addChild registers a child process so its Usage() rolls up.
func (b *processBudget) addChild(child *AgentProcess) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = append(b.children, child)
}

// usage returns the subtree-aggregated totals plus this process's own
// action count (passed in by AgentProcess.Usage which has the history).
func (b *processBudget) usage(ownActions int) (cost float64, tokens int, actions int) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	cost = b.ownCost
	tokens = b.ownTokens
	actions = ownActions

	for _, child := range b.children {
		c, t, a := child.Usage()
		cost += c
		tokens += t
		actions += a
	}
	return
}

// llmHistory returns the subtree-aggregated LLM invocation history.
// The result is a fresh slice; the caller may sort or filter freely
// without affecting future Record* calls.
func (b *processBudget) llmHistory() []core.LLMInvocation {
	b.lock.RLock()
	defer b.lock.RUnlock()

	out := make([]core.LLMInvocation, 0, len(b.llmInvocations))
	out = append(out, b.llmInvocations...)
	for _, child := range b.children {
		out = append(out, child.LLMInvocations()...)
	}
	return out
}

// embeddingHistory mirrors llmHistory for embeddings.
func (b *processBudget) embeddingHistory() []core.EmbeddingInvocation {
	b.lock.RLock()
	defer b.lock.RUnlock()

	out := make([]core.EmbeddingInvocation, 0, len(b.embeddingInvocations))
	out = append(out, b.embeddingInvocations...)
	for _, child := range b.children {
		out = append(out, child.EmbeddingInvocations()...)
	}
	return out
}
