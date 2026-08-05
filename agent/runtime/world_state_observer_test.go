package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type namespacedAction struct {
	metadata core.ActionMetadata
}

func (a namespacedAction) Metadata() core.ActionMetadata { return a.metadata }

func (namespacedAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

func TestWorldStateObservesNamespacedActionSuccessCondition(t *testing.T) {
	metadata := core.ActionMetadata{Name: "checkout:authorize"}
	metadata.Preconditions = core.ConditionSet{metadata.SuccessCondition(): core.False}
	metadata.Effects = core.ConditionSet{metadata.SuccessCondition(): core.True}
	action := namespacedAction{metadata: metadata}
	domain, err := planning.NewDomain([]core.Action{action}, nil, nil)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	blackboard := newInMemoryBlackboard()
	if err := blackboard.StoreCondition(action.metadata.SuccessCondition(), true); err != nil {
		t.Fatal(err)
	}

	state, err := newWorldStateObserver(domain, blackboard, nil).observe(t.Context())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := state.Conditions()[action.metadata.SuccessCondition()]; got != core.True {
		t.Fatalf("success condition = %v, want true", got)
	}
}

func TestWorldStateDefersAndCachesNamedConditionEvaluation(t *testing.T) {
	evaluations := 0
	condition := core.NewCondition(core.ConditionConfig{Name: "remote_ready", EvaluationCost: 5, Evaluate: func(context.Context, *core.ConditionEnv) core.Truth {
		evaluations++
		return core.True
	}})
	domain, err := planning.NewDomain(nil, nil, []core.Condition{condition})
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	observer := newWorldStateObserver(domain, newInMemoryBlackboard(), nil)

	state, err := observer.observe(t.Context())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if evaluations != 0 {
		t.Fatalf("evaluations after read = %d, want deferred", evaluations)
	}
	if got := state.Conditions()["remote_ready"]; got != core.Unknown {
		t.Fatalf("unresolved state = %s, want unknown", got)
	}
	for range 2 {
		truth, err := observer.Resolve(t.Context(), "remote_ready")
		if err != nil || truth != core.True {
			t.Fatalf("Resolve = %s, %v; want true", truth, err)
		}
	}
	if evaluations != 1 {
		t.Fatalf("evaluations after two resolves = %d, want one", evaluations)
	}
	resolved := observer.ResolvedConditions()
	if resolved["remote_ready"] != core.True {
		t.Fatalf("resolved conditions = %v, want remote_ready=true", resolved)
	}
	resolved["remote_ready"] = core.False
	if observer.ResolvedConditions()["remote_ready"] != core.True {
		t.Fatal("ResolvedConditions exposed mutable cache storage")
	}

	if _, err := observer.observe(t.Context()); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if _, err := observer.Resolve(t.Context(), "remote_ready"); err != nil {
		t.Fatalf("Resolve after second read: %v", err)
	}
	if evaluations != 2 {
		t.Fatalf("evaluations after next tick = %d, want two", evaluations)
	}
}
