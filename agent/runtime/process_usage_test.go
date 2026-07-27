package runtime

import (
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func newTestBudget() *processBudget {
	return &processBudget{lock: new(sync.RWMutex)}
}

func TestProcessBudgetAggregatesResourceUsage(t *testing.T) {
	budget := newTestBudget()
	for _, usage := range []core.Usage{
		{Cost: 0.012, Tokens: 1500, ModelCalls: 1},
		{Cost: 0.008, Tokens: 600, ModelCalls: 1},
	} {
		if err := budget.record(usage); err != nil {
			t.Fatalf("record(%+v): %v", usage, err)
		}
	}

	got := budget.usage(3)
	if math.Abs(got.Cost-0.020) > 1e-9 {
		t.Fatalf("cost = %.6f, want 0.020", got.Cost)
	}
	if got.Tokens != 2100 || got.ModelCalls != 2 || got.Actions != 3 {
		t.Fatalf("usage = %+v, want tokens=2100 modelCalls=2 actions=3", got)
	}
}

func TestProcessBudgetRejectsInvalidAndOverflowingUsageAtomically(t *testing.T) {
	budget := newTestBudget()
	if err := budget.record(core.Usage{Tokens: -1}); !errors.Is(err, core.ErrInvalidUsage) {
		t.Fatalf("invalid usage error = %v, want ErrInvalidUsage", err)
	}
	if err := budget.record(core.Usage{Cost: math.MaxFloat64, Tokens: math.MaxInt, ModelCalls: math.MaxInt}); err != nil {
		t.Fatalf("record maxima: %v", err)
	}
	before := budget.usage(0)
	for name, delta := range map[string]core.Usage{
		"cost":        {Cost: math.MaxFloat64},
		"tokens":      {Tokens: 1},
		"model_calls": {ModelCalls: 1},
	} {
		if err := budget.record(delta); err == nil {
			t.Errorf("%s overflow was accepted", name)
		}
		if got := budget.usage(0); got != before {
			t.Errorf("%s overflow mutated usage: got %+v, want %+v", name, got, before)
		}
	}
}

func TestUsageTokenCountRejectsInvalidAndOverflowingValues(t *testing.T) {
	if _, err := usageTokenCount(-1); err == nil {
		t.Fatal("negative token count was accepted")
	}
	if _, err := usageTokenCount(int64(math.MaxInt), 1); err == nil {
		t.Fatal("overflowing token count was accepted")
	}
	if got, err := usageTokenCount(10, 5); err != nil || got != 15 {
		t.Fatalf("usageTokenCount = %d, %v, want 15, nil", got, err)
	}
}

func TestProcessUsageSaturatesSubtreeAggregation(t *testing.T) {
	parent := &Process{state: newProcessState()}
	parent.budget.lock = &parent.state.mu
	child := &Process{state: newProcessState()}
	child.budget.lock = &child.state.mu
	if err := parent.budget.record(core.Usage{
		Cost:       math.MaxFloat64,
		Tokens:     math.MaxInt,
		ModelCalls: math.MaxInt,
	}); err != nil {
		t.Fatalf("record parent maxima: %v", err)
	}
	if err := child.budget.record(core.Usage{Cost: 1, Tokens: 1, ModelCalls: 1}); err != nil {
		t.Fatalf("record child usage: %v", err)
	}
	parent.budget.addChild(child)

	usage := parent.Usage()
	if usage.Cost != math.MaxFloat64 || usage.Tokens != math.MaxInt || usage.ModelCalls != math.MaxInt {
		t.Fatalf("subtree usage = %+v, want saturated maxima", usage)
	}
}
