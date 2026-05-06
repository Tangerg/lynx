package runtime

import "sync"

// processBudget aggregates "what did this process and its descendants
// cost?" Used by [core.BudgetPolicy] (and any custom
// [core.EarlyTerminationPolicy]) so a parent's budget governs its entire
// delegation tree.
//
// Cost / token totals are populated by integration code via
// [Process.RecordUsage]; the framework itself never invents numbers (it
// doesn't know per-model rates). The action count comes from
// [processState]'s history, threaded through Usage().
//
// Concurrency: writes are protected by AgentProcess's main mutex (held
// by RecordUsage / addChild). Reads in Usage() walk the children
// recursively under RLock — the lock graph is a tree (parent →
// children, never the reverse) so no deadlock is possible.
type processBudget struct {
	mu        *sync.RWMutex // shared with AgentProcess.mu — set by AgentProcess constructor
	children  []*AgentProcess
	ownCost   float64
	ownTokens int
}

// recordUsage adds a single LLM call's cost (USD) and token count to
// this process's running totals.
func (b *processBudget) recordUsage(cost float64, tokens int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ownCost += cost
	b.ownTokens += tokens
}

// addChild registers a child process so its Usage() rolls up.
func (b *processBudget) addChild(child *AgentProcess) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.children = append(b.children, child)
}

// usage returns the subtree-aggregated totals plus this process's own
// action count (passed in by AgentProcess.Usage which has the history).
//
// Walks children recursively while holding the parent RLock. Each
// child takes its own RLock — safe because the parent → child relation
// is acyclic (a child can't reach back to mutate parent under its own
// lock).
func (b *processBudget) usage(ownActions int) (cost float64, tokens int, actions int) {
	b.mu.RLock()
	defer b.mu.RUnlock()

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
