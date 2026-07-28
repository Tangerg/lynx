package runtime

import (
	"errors"
	"math"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

// Usage returns the resource totals for this process and its descendants.
// Admission is coordinated by one authority shared by the complete process
// tree; the recursive view remains subtree-relative for observers and custom
// policies.
func (p *Process) Usage() core.Usage {
	if p == nil {
		return core.Usage{}
	}
	return p.budget.usage()
}

// budgetAuthority is the only mutable budget decision point for a complete
// process tree. Actions are charged before execution. Model calls are reserved
// before I/O and committed with their response usage, preventing concurrent
// siblings from admitting work against the same remaining model-call capacity.
//
// This is the outer lock: a charge that touches both the tree aggregate and one
// process's own tally takes this mutex first and processBudget.mu second, never
// the reverse. One authority is shared by every process in the tree while each
// processBudget.mu is private, so a single reversed site deadlocks the whole
// tree's budget path against itself — and a deadlock is exactly what the race
// detector cannot see.
type budgetAuthority struct {
	mu sync.Mutex

	limit              core.Budget
	usage              core.Usage
	reservedModelCalls int
}

type processBudget struct {
	mu sync.RWMutex

	authority *budgetAuthority
	children  []*Process
	own       core.Usage
}

func newProcessBudget(limit core.Budget) processBudget {
	return processBudget{
		authority: &budgetAuthority{limit: limit},
	}
}

func (b *processBudget) admitAction() (bool, string, error) {
	if b == nil || b.authority == nil {
		return false, "", errors.New("runtime: process budget is not initialized")
	}
	authority := b.authority
	authority.mu.Lock()
	defer authority.mu.Unlock()

	if exhausted, reason := authority.exhaustedLocked(); exhausted {
		return false, reason, nil
	}
	if authority.usage.Actions == math.MaxInt {
		return false, "", errors.New("runtime: action usage exceeds int capacity")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.own.Actions == math.MaxInt {
		return false, "", errors.New("runtime: process action usage exceeds int capacity")
	}
	authority.usage.Actions++
	b.own.Actions++
	return true, "", nil
}

func (b *processBudget) reserveModelCall() (*modelCallReservation, interaction.StopReason, error) {
	if b == nil || b.authority == nil {
		return nil, interaction.StopNone, errors.New("runtime: process budget is not initialized")
	}
	authority := b.authority
	authority.mu.Lock()
	defer authority.mu.Unlock()

	if authority.usage.ModelCalls > math.MaxInt-authority.reservedModelCalls {
		return nil, interaction.StopNone, errors.New("runtime: model-call admission exceeds int capacity")
	}
	admittedModelCalls := authority.usage.ModelCalls + authority.reservedModelCalls
	switch {
	case authority.limit.CostLimit > 0 && authority.usage.Cost >= authority.limit.CostLimit:
		return nil, interaction.StopBudget, nil
	case authority.limit.TokenLimit > 0 && authority.usage.Tokens >= authority.limit.TokenLimit:
		return nil, interaction.StopBudget, nil
	case authority.limit.ModelCallLimit > 0 &&
		admittedModelCalls >= authority.limit.ModelCallLimit:
		return nil, interaction.StopSteps, nil
	case authority.reservedModelCalls == math.MaxInt:
		return nil, interaction.StopNone, errors.New("runtime: reserved model-call count exceeds int capacity")
	}
	authority.reservedModelCalls++
	return &modelCallReservation{
		authority: authority,
		budget:    b,
		active:    true,
	}, interaction.StopNone, nil
}

type modelCallReservation struct {
	authority *budgetAuthority
	budget    *processBudget
	active    bool
}

func (r *modelCallReservation) commit(usage core.Usage) error {
	if r == nil {
		return errors.New("runtime: model call has no budget reservation")
	}
	if err := usage.Validate(); err != nil {
		r.release()
		return err
	}
	if usage.ModelCalls != 1 || usage.Actions != 0 {
		r.release()
		return errors.New("runtime: model-call usage must contain exactly one model call and no actions")
	}

	authority := r.authority
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if !r.active {
		return errors.New("runtime: model-call budget reservation is no longer active")
	}
	defer r.finishLocked()

	nextTree, err := addUsage(authority.usage, usage)
	if err != nil {
		return err
	}
	r.budget.mu.Lock()
	defer r.budget.mu.Unlock()
	nextOwn, err := addUsage(r.budget.own, usage)
	if err != nil {
		return err
	}
	authority.usage = nextTree
	r.budget.own = nextOwn
	return nil
}

func (r *modelCallReservation) release() {
	if r == nil || r.authority == nil {
		return
	}
	r.authority.mu.Lock()
	defer r.authority.mu.Unlock()
	if r.active {
		r.finishLocked()
	}
}

func (r *modelCallReservation) finishLocked() {
	r.active = false
	r.authority.reservedModelCalls--
}

func (a *budgetAuthority) exhaustedLocked() (bool, string) {
	switch {
	case a.limit.CostLimit > 0 && a.usage.Cost >= a.limit.CostLimit:
		return true, "cost budget exhausted"
	case a.limit.TokenLimit > 0 && a.usage.Tokens >= a.limit.TokenLimit:
		return true, "token budget exhausted"
	case a.limit.ModelCallLimit > 0 &&
		saturatingInt(a.usage.ModelCalls, a.reservedModelCalls) >= a.limit.ModelCallLimit:
		return true, "model-call budget exhausted"
	case a.limit.ActionLimit > 0 && a.usage.Actions >= a.limit.ActionLimit:
		return true, "action budget exhausted"
	default:
		return false, ""
	}
}

func (b *processBudget) restore(usage core.Usage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.own = usage
}

// restoreAuthority installs the aggregate rebuilt from all restored process
// snapshots after parent-child links have been reconstructed.
func (b *processBudget) restoreAuthority(usage core.Usage) error {
	if err := usage.Validate(); err != nil {
		return err
	}
	if b == nil || b.authority == nil {
		return errors.New("runtime: process budget is not initialized")
	}
	b.authority.mu.Lock()
	defer b.authority.mu.Unlock()
	b.authority.usage = usage
	b.authority.reservedModelCalls = 0
	return nil
}

func (b *processBudget) addChild(child *Process) {
	child.budget.mu.Lock()
	child.budget.authority = b.authority
	child.budget.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	b.children = append(b.children, child)
}

func (b *processBudget) ownUsage() core.Usage {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.own
}

func (b *processBudget) usage() core.Usage {
	b.mu.RLock()
	usage := b.own
	children := append([]*Process(nil), b.children...)
	b.mu.RUnlock()

	for _, child := range children {
		usage = saturatingUsage(usage, child.Usage())
	}
	return usage
}

func addUsage(left, right core.Usage) (core.Usage, error) {
	if right.Cost > math.MaxFloat64-left.Cost {
		return core.Usage{}, errors.New("runtime: cost usage exceeds float64 capacity")
	}
	if right.Tokens > math.MaxInt64-left.Tokens {
		return core.Usage{}, errors.New("runtime: token usage exceeds int64 capacity")
	}
	if right.ModelCalls > math.MaxInt-left.ModelCalls {
		return core.Usage{}, errors.New("runtime: model-call usage exceeds int capacity")
	}
	if right.Actions > math.MaxInt-left.Actions {
		return core.Usage{}, errors.New("runtime: action usage exceeds int capacity")
	}
	return core.Usage{
		Cost:       left.Cost + right.Cost,
		Tokens:     left.Tokens + right.Tokens,
		ModelCalls: left.ModelCalls + right.ModelCalls,
		Actions:    left.Actions + right.Actions,
	}, nil
}

func usageTokenCount(values ...int64) (int64, error) {
	var total int64
	for _, value := range values {
		if value < 0 || value > math.MaxInt64-total {
			return 0, errors.New("token count exceeds int64 capacity")
		}
		total += value
	}
	return total, nil
}

func saturatingUsage(left, right core.Usage) core.Usage {
	return core.Usage{
		Cost:       saturatingCost(left.Cost, right.Cost),
		Tokens:     saturatingInt64(left.Tokens, right.Tokens),
		ModelCalls: saturatingInt(left.ModelCalls, right.ModelCalls),
		Actions:    saturatingInt(left.Actions, right.Actions),
	}
}

func saturatingCost(left, right float64) float64 {
	if right > math.MaxFloat64-left {
		return math.MaxFloat64
	}
	return left + right
}

func saturatingInt(left, right int) int {
	if right > math.MaxInt-left {
		return math.MaxInt
	}
	return left + right
}

func saturatingInt64(left, right int64) int64 {
	if right > math.MaxInt64-left {
		return math.MaxInt64
	}
	return left + right
}
