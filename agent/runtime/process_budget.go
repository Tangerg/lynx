package runtime

import (
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// processBudget aggregates "what did this process and its descendants
// cost?" Used by [core.BudgetPolicy] (and any custom
// [core.StopPolicy]) so a parent's budget governs its entire
// delegation tree.
//
// Cost / token totals are populated by integration code via
// [core.UsageRecorder.RecordUsage] (lumped) or per-call
// [core.UsageRecorder.RecordModelCall] /
// [core.UsageRecorder.RecordEmbeddingCall]; the framework itself never
// invents numbers (it doesn't know per-model rates). The action count
// comes from [processState]'s history, threaded through Usage().
//
// Concurrency: writes are protected by Process's main mutex (held
// by record* / addChild). Reads in Usage() walk the children
// recursively under RLock — the lock graph is a tree (parent →
// children, never the reverse) so no deadlock is possible.
type processBudget struct {
	lock           *sync.RWMutex // points at processState.mu — set by Process constructor
	children       []*Process
	ownCost        float64
	ownTokens      int
	modelCalls     []core.ModelCall
	embeddingCalls []core.EmbeddingCall
}

// recordModelCall appends a fully-attributed LLM call to history
// and rolls its cost/tokens into the budget. Timestamp defaulting
// happens in [Process.RecordModelCall] — the only caller —
// so the published event and the stored record carry the same stamp.
func (b *processBudget) recordModelCall(call core.ModelCall) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += call.CostUSD
	b.ownTokens += int(call.PromptTokens + call.CompletionTokens)
	b.modelCalls = append(b.modelCalls, call)
}

// recordEmbeddingCall appends an embedding call and rolls its
// cost/tokens into the budget. Timestamp defaulting happens in
// [Process.RecordEmbeddingCall], mirroring the LLM path.
func (b *processBudget) recordEmbeddingCall(call core.EmbeddingCall) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += call.CostUSD
	b.ownTokens += int(call.InputTokens)
	b.embeddingCalls = append(b.embeddingCalls, call)
}

// restore re-installs budget totals and call history from a
// snapshot. Holds the lock so [usage] / [modelCallHistory] / [embeddingCallHistory]
// observe the bulk update atomically.
func (b *processBudget) restore(
	cost float64,
	tokens int,
	modelCalls []core.ModelCall,
	embeddingCalls []core.EmbeddingCall,
) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost = cost
	b.ownTokens = tokens
	b.modelCalls = slices.Clone(modelCalls)
	b.embeddingCalls = slices.Clone(embeddingCalls)
}

// addChild registers a child process so its Usage() rolls up.
func (b *processBudget) addChild(child *Process) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = append(b.children, child)
}

// removeChild drops child from the rollup — the inverse of addChild, for when
// a child that was registered fails to fully spawn (e.g. its session link
// fails) and is unregistered: without this the parent's children slice keeps a
// stale reference for the parent's whole life. (The stale child contributes 0
// to usage since it never ran, so this is a leak fix, not an accounting fix.)
func (b *processBudget) removeChild(child *Process) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = slices.DeleteFunc(b.children, func(candidate *Process) bool { return candidate == child })
}

// usage returns the subtree-aggregated totals plus this process's own
// action count (passed in by Process.Usage which has the history).
func (b *processBudget) usage(ownActions int) (cost float64, tokens int, actions int) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	cost = b.ownCost
	tokens = b.ownTokens
	actions = ownActions

	for _, child := range b.children {
		childCost, childTokens, childActions := child.Usage()
		cost += childCost
		tokens += childTokens
		actions += childActions
	}
	return
}

// modelCallHistory returns the subtree-aggregated model call history.
// The result is a fresh slice; the caller may sort or filter freely
// without affecting future Record* calls.
func (b *processBudget) modelCallHistory() []core.ModelCall {
	b.lock.RLock()
	defer b.lock.RUnlock()

	history := slices.Clone(b.modelCalls)
	for _, child := range b.children {
		history = append(history, child.ModelCalls()...)
	}
	return history
}

// embeddingCallHistory mirrors modelCallHistory for embeddings.
func (b *processBudget) embeddingCallHistory() []core.EmbeddingCall {
	b.lock.RLock()
	defer b.lock.RUnlock()

	history := slices.Clone(b.embeddingCalls)
	for _, child := range b.children {
		history = append(history, child.EmbeddingCalls()...)
	}
	return history
}
