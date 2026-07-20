package runtime

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// Usage returns the subtree-aggregated cost / token / action totals.
// Cost and tokens come from [core.ProcessContext.RecordUsage] calls (zero
// unless integration code wires them up). The action count is the
// recursive sum of every History across this process and its children.
// [core.BudgetPolicy]
// uses the result so a parent's budget governs its entire delegation
// tree.
func (p *Process) Usage() (cost float64, tokens int, actions int) {
	return p.budget.usage(p.state.historyLen())
}

type processUsage struct{ process *Process }

var _ core.UsageRecorder = processUsage{}

func (recorder processUsage) RecordUsage(ctx context.Context, cost float64, tokens int) {
	recorder.RecordModelCall(ctx, core.ModelCall{CostUSD: cost, PromptTokens: int64(tokens)})
}

func (recorder processUsage) RecordModelCall(ctx context.Context, call core.ModelCall) {
	if ctx == nil {
		ctx = context.Background()
	}
	if call.Timestamp.IsZero() {
		call.Timestamp = time.Now()
	}
	process := recorder.process
	process.budget.recordModelCall(call)
	process.publishEvent(ctx, event.ModelCallRecorded{
		Header: process.eventHeader(),
		Call:   call,
	})
}

func (recorder processUsage) RecordEmbeddingCall(ctx context.Context, call core.EmbeddingCall) {
	if ctx == nil {
		ctx = context.Background()
	}
	if call.Timestamp.IsZero() {
		call.Timestamp = time.Now()
	}
	process := recorder.process
	process.budget.recordEmbeddingCall(call)
	process.publishEvent(ctx, event.EmbeddingCallRecorded{
		Header: process.eventHeader(),
		Call:   call,
	})
}

// ModelCalls returns the subtree-aggregated model-call history in
// process order.
func (p *Process) ModelCalls() []core.ModelCall {
	return p.budget.modelCallHistory()
}

// EmbeddingCalls returns the subtree-aggregated embedding-call history.
func (p *Process) EmbeddingCalls() []core.EmbeddingCall {
	return p.budget.embeddingCallHistory()
}

// processBudget aggregates the usage of a process and its descendants. Its
// lock points at processState.mu, preserving one synchronization boundary for
// process state and accounting. Reads recurse parent-to-child only.
type processBudget struct {
	lock           *sync.RWMutex
	children       []*Process
	ownCost        float64
	ownTokens      int
	modelCalls     []core.ModelCall
	embeddingCalls []core.EmbeddingCall
}

// ownSnapshot returns one consistent copy of this process's direct usage.
func (b *processBudget) ownSnapshot() (
	cost float64,
	tokens int,
	modelCalls []core.ModelCall,
	embeddingCalls []core.EmbeddingCall,
) {
	b.lock.RLock()
	defer b.lock.RUnlock()
	return b.ownCost, b.ownTokens, slices.Clone(b.modelCalls), slices.Clone(b.embeddingCalls)
}

func (b *processBudget) recordModelCall(call core.ModelCall) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += call.CostUSD
	b.ownTokens += int(call.PromptTokens + call.CompletionTokens)
	b.modelCalls = append(b.modelCalls, call)
}

func (b *processBudget) recordEmbeddingCall(call core.EmbeddingCall) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.ownCost += call.CostUSD
	b.ownTokens += int(call.InputTokens)
	b.embeddingCalls = append(b.embeddingCalls, call)
}

// restore atomically reinstalls direct usage from a durable snapshot.
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

func (b *processBudget) addChild(child *Process) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = append(b.children, child)
}

// removeChild is the inverse of addChild for a child that did not finish
// spawning, preventing a stale process reference from living with the parent.
func (b *processBudget) removeChild(child *Process) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = slices.DeleteFunc(b.children, func(candidate *Process) bool { return candidate == child })
}

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

// modelCallHistory returns a fresh subtree-aggregated history.
func (b *processBudget) modelCallHistory() []core.ModelCall {
	b.lock.RLock()
	defer b.lock.RUnlock()

	history := slices.Clone(b.modelCalls)
	for _, child := range b.children {
		history = append(history, child.ModelCalls()...)
	}
	return history
}

// embeddingCallHistory returns a fresh subtree-aggregated history.
func (b *processBudget) embeddingCallHistory() []core.EmbeddingCall {
	b.lock.RLock()
	defer b.lock.RUnlock()

	history := slices.Clone(b.embeddingCalls)
	for _, child := range b.children {
		history = append(history, child.EmbeddingCalls()...)
	}
	return history
}
