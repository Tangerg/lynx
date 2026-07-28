package runtime

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

func newTestBudget(limit core.Budget) *processBudget {
	budget := newProcessBudget(limit)
	return &budget
}

func recordTestModelUsage(t *testing.T, budget *processBudget, usage core.Usage) {
	t.Helper()
	reservation, stop, err := budget.reserveModelCall()
	if err != nil || stop != interaction.StopNone {
		t.Fatalf("reserve model call = (%v, %q, %v)", reservation, stop, err)
	}
	if err := reservation.commit(usage); err != nil {
		t.Fatalf("commit(%+v): %v", usage, err)
	}
}

func TestProcessBudgetAggregatesDirectUsage(t *testing.T) {
	budget := newTestBudget(core.Budget{})
	for _, usage := range []core.Usage{
		{Cost: 0.012, Tokens: 1500, ModelCalls: 1},
		{Cost: 0.008, Tokens: 600, ModelCalls: 1},
	} {
		recordTestModelUsage(t, budget, usage)
	}
	for range 3 {
		admitted, reason, err := budget.admitAction()
		if err != nil || !admitted {
			t.Fatalf("admit action = %t, %q, %v", admitted, reason, err)
		}
	}

	got := budget.usage()
	if math.Abs(got.Cost-0.020) > 1e-9 {
		t.Fatalf("cost = %.6f, want 0.020", got.Cost)
	}
	if got.Tokens != 2100 || got.ModelCalls != 2 || got.Actions != 3 {
		t.Fatalf("usage = %+v, want tokens=2100 modelCalls=2 actions=3", got)
	}
}

func TestModelReservationRejectsInvalidUsageAndReleasesCapacity(t *testing.T) {
	budget := newTestBudget(core.Budget{ModelCallLimit: 1})
	reservation, stop, err := budget.reserveModelCall()
	if err != nil || stop != interaction.StopNone {
		t.Fatalf("reserve = (%v, %q, %v)", reservation, stop, err)
	}
	if err := reservation.commit(core.Usage{Tokens: -1, ModelCalls: 1}); !errors.Is(err, core.ErrInvalidUsage) {
		t.Fatalf("invalid usage error = %v, want ErrInvalidUsage", err)
	}
	if usage := budget.usage(); usage != (core.Usage{}) {
		t.Fatalf("rejected usage mutated counters: %+v", usage)
	}
	reservation, stop, err = budget.reserveModelCall()
	if err != nil || stop != interaction.StopNone || reservation == nil {
		t.Fatalf("released capacity reserve = (%v, %q, %v)", reservation, stop, err)
	}
	reservation.release()
}

func TestModelReservationRejectsCostCapacityLossAtomically(t *testing.T) {
	budget := newTestBudget(core.Budget{})
	recordTestModelUsage(t, budget, core.Usage{Cost: math.MaxFloat64, ModelCalls: 1})

	reservation, stop, err := budget.reserveModelCall()
	if err != nil || stop != interaction.StopNone {
		t.Fatalf("reserve = (%v, %q, %v)", reservation, stop, err)
	}
	if err := reservation.commit(core.Usage{Cost: 1, ModelCalls: 1}); err == nil {
		t.Fatal("cost increment beyond representable capacity was accepted")
	}
	if usage := budget.usage(); usage.Cost != math.MaxFloat64 || usage.ModelCalls != 1 {
		t.Fatalf("rejected cost partially mutated usage: %+v", usage)
	}
}

func TestUsageTokenCountRejectsInvalidAndOverflowingValues(t *testing.T) {
	if _, err := usageTokenCount(-1); err == nil {
		t.Fatal("negative token count was accepted")
	}
	if _, err := usageTokenCount(math.MaxInt64, 1); err == nil {
		t.Fatal("overflowing token count was accepted")
	}
	if got, err := usageTokenCount(10, 5); err != nil || got != 15 {
		t.Fatalf("usageTokenCount = %d, %v, want 15, nil", got, err)
	}
}

func TestProcessUsageSaturatesSubtreeAggregation(t *testing.T) {
	parent := &Process{budget: newProcessBudget(core.Budget{})}
	child := &Process{budget: newProcessBudget(core.Budget{})}
	parent.budget.restore(core.Usage{
		Cost:       math.MaxFloat64,
		Tokens:     math.MaxInt64,
		ModelCalls: math.MaxInt,
		Actions:    math.MaxInt,
	})
	child.budget.restore(core.Usage{Cost: 1, Tokens: 1, ModelCalls: 1, Actions: 1})
	parent.budget.addChild(child)

	usage := parent.Usage()
	if usage.Cost != math.MaxFloat64 || usage.Tokens != math.MaxInt64 ||
		usage.ModelCalls != math.MaxInt || usage.Actions != math.MaxInt {
		t.Fatalf("subtree usage = %+v, want saturated maxima", usage)
	}
}

func TestConcurrentSiblingsShareModelCallAdmission(t *testing.T) {
	parent := &Process{budget: newProcessBudget(core.Budget{ModelCallLimit: 1})}
	children := []*Process{
		{budget: newProcessBudget(core.Budget{})},
		{budget: newProcessBudget(core.Budget{})},
	}
	for _, child := range children {
		parent.budget.addChild(child)
	}

	start := make(chan struct{})
	type result struct {
		reservation *modelCallReservation
		stop        interaction.StopReason
		err         error
	}
	results := make(chan result, len(children))
	var group sync.WaitGroup
	for _, child := range children {
		group.Go(func() {
			<-start
			reservation, stop, err := child.budget.reserveModelCall()
			results <- result{reservation: reservation, stop: stop, err: err}
		})
	}
	close(start)
	group.Wait()
	close(results)

	var admitted, stopped int
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.reservation != nil {
			admitted++
			result.reservation.release()
		}
		if result.stop == interaction.StopSteps {
			stopped++
		}
	}
	if admitted != 1 || stopped != 1 {
		t.Fatalf("admitted=%d stopped=%d, want 1/1", admitted, stopped)
	}
}

func TestConcurrentSiblingsShareActionAdmission(t *testing.T) {
	parent := &Process{budget: newProcessBudget(core.Budget{ActionLimit: 1})}
	children := []*Process{
		{budget: newProcessBudget(core.Budget{})},
		{budget: newProcessBudget(core.Budget{})},
	}
	for _, child := range children {
		parent.budget.addChild(child)
	}

	start := make(chan struct{})
	results := make(chan bool, len(children))
	var group sync.WaitGroup
	for _, child := range children {
		group.Go(func() {
			<-start
			admitted, _, err := child.budget.admitAction()
			if err != nil {
				t.Error(err)
			}
			results <- admitted
		})
	}
	close(start)
	group.Wait()
	close(results)

	var admitted int
	for ok := range results {
		if ok {
			admitted++
		}
	}
	if admitted != 1 {
		t.Fatalf("admitted=%d, want 1", admitted)
	}
	if usage := parent.Usage(); usage.Actions != 1 {
		t.Fatalf("tree action usage=%d, want 1", usage.Actions)
	}
}
