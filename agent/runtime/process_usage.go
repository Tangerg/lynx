package runtime

import (
	"context"
	"errors"
	"math"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// Usage returns subtree-aggregated resource and action totals. Resource values
// come from [core.ProcessContext.RecordUsage]; Actions is the recursive sum of
// completed action history. [core.BudgetPolicy] uses this aggregate so a
// parent's budget governs its entire delegation tree.
func (p *Process) Usage() core.ProcessUsage {
	return p.budget.usage(p.state.historyLen())
}

type processUsage struct{ process *Process }

var _ core.UsageRecorder = processUsage{}

func (recorder processUsage) RecordUsage(_ context.Context, usage core.Usage) error {
	if recorder.process == nil {
		return core.ErrUsageUnavailable
	}
	return recorder.process.budget.record(usage)
}

// processBudget aggregates the usage of a process and its descendants. Its
// lock points at processState.mu, preserving one synchronization boundary for
// process state and accounting. Reads recurse parent-to-child only.
type processBudget struct {
	lock     *sync.RWMutex
	children []*Process
	own      core.Usage
}

func (b *processBudget) record(usage core.Usage) error {
	if err := usage.Validate(); err != nil {
		return err
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	nextCost := b.own.Cost + usage.Cost
	if math.IsInf(nextCost, 0) {
		return errors.New("record usage: cost total exceeds float64 capacity")
	}
	if usage.Tokens > math.MaxInt-b.own.Tokens {
		return errors.New("record usage: token total exceeds int capacity")
	}
	if usage.ModelCalls > math.MaxInt-b.own.ModelCalls {
		return errors.New("record usage: model-call total exceeds int capacity")
	}
	b.own.Cost = nextCost
	b.own.Tokens += usage.Tokens
	b.own.ModelCalls += usage.ModelCalls
	return nil
}

func usageTokenCount(values ...int64) (int, error) {
	var total int64
	for _, value := range values {
		if value < 0 || value > int64(math.MaxInt)-total {
			return 0, errors.New("token count exceeds int capacity")
		}
		total += value
	}
	return int(total), nil
}

// restore atomically reinstalls direct usage from a process snapshot.
func (b *processBudget) restore(usage core.Usage) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.own = usage
}

func (b *processBudget) addChild(child *Process) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.children = append(b.children, child)
}

func (b *processBudget) usage(ownActions int) core.ProcessUsage {
	b.lock.RLock()
	defer b.lock.RUnlock()

	usage := core.ProcessUsage{Usage: b.own, Actions: ownActions}
	for _, child := range b.children {
		childUsage := child.Usage()
		usage.Cost = saturatingCost(usage.Cost, childUsage.Cost)
		usage.Tokens = saturatingCount(usage.Tokens, childUsage.Tokens)
		usage.ModelCalls = saturatingCount(usage.ModelCalls, childUsage.ModelCalls)
		usage.Actions = saturatingCount(usage.Actions, childUsage.Actions)
	}
	return usage
}

func saturatingCost(left, right float64) float64 {
	if right > math.MaxFloat64-left {
		return math.MaxFloat64
	}
	return left + right
}

func saturatingCount(left, right int) int {
	if right > math.MaxInt-left {
		return math.MaxInt
	}
	return left + right
}
