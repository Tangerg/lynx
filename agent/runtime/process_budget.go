package runtime

import (
	"slices"
	"sync"

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

// recordLLMInvocation appends a fully-attributed LLM call to history
// and rolls its cost/tokens into the budget. Timestamp defaulting
// happens in [AgentProcess.RecordLLMInvocation] — the only caller —
// so the published event and the stored record carry the same stamp.
func (b *processBudget) recordLLMInvocation(inv core.LLMInvocation) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += inv.CostUSD
	b.ownTokens += int(inv.PromptTokens + inv.CompletionTokens)
	b.llmInvocations = append(b.llmInvocations, inv)
}

// recordEmbeddingInvocation appends an embedding call and rolls its
// cost/tokens into the budget. Timestamp defaulting happens in
// [AgentProcess.RecordEmbeddingInvocation], mirroring the LLM path.
func (b *processBudget) recordEmbeddingInvocation(inv core.EmbeddingInvocation) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += inv.CostUSD
	b.ownTokens += int(inv.InputTokens)
	b.embeddingInvocations = append(b.embeddingInvocations, inv)
}

// restore re-installs budget totals + invocation history from a
// snapshot. Holds the lock so [usage] / [llmHistory] / [embeddingHistory]
// observe the bulk update atomically.
func (b *processBudget) restore(
	cost float64,
	tokens int,
	llms []core.LLMInvocation,
	embeds []core.EmbeddingInvocation,
) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost = cost
	b.ownTokens = tokens
	b.llmInvocations = slices.Clone(llms)
	b.embeddingInvocations = slices.Clone(embeds)
}

// addChild registers a child process so its Usage() rolls up.
func (b *processBudget) addChild(child *AgentProcess) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = append(b.children, child)
}

// removeChild drops child from the rollup — the inverse of addChild, for when
// a child that was registered fails to fully spawn (e.g. its session link
// fails) and is unregistered: without this the parent's children slice keeps a
// stale reference for the parent's whole life. (The stale child contributes 0
// to usage since it never ran, so this is a leak fix, not an accounting fix.)
func (b *processBudget) removeChild(child *AgentProcess) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = slices.DeleteFunc(b.children, func(c *AgentProcess) bool { return c == child })
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

	out := slices.Clone(b.llmInvocations)
	for _, child := range b.children {
		out = append(out, child.LLMInvocations()...)
	}
	return out
}

// embeddingHistory mirrors llmHistory for embeddings.
func (b *processBudget) embeddingHistory() []core.EmbeddingInvocation {
	b.lock.RLock()
	defer b.lock.RUnlock()

	out := slices.Clone(b.embeddingInvocations)
	for _, child := range b.children {
		out = append(out, child.EmbeddingInvocations()...)
	}
	return out
}
