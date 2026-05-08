package reactive_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
	"github.com/Tangerg/lynx/agent/plan/planner/reactive"
)

// fakeAction is a planner-only Action — Execute is never called by the
// reactive planner, so we only need to satisfy the interface.
type fakeAction struct {
	meta core.ActionMetadata
}

func (a *fakeAction) Metadata() core.ActionMetadata { return a.meta }
func (a *fakeAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionFailed
}

func newAction(name string, pre, eff core.EffectSpec, cost float64) core.Action {
	return &fakeAction{meta: core.ActionMetadata{
		Name:          name,
		Preconditions: pre,
		Effects:       eff,
		Cost:          core.Static(cost),
		Value:         core.Static(0),
	}}
}

func TestReactive_AlreadySatisfiedReturnsEmptyPlan(t *testing.T) {
	start := plan.NewConditionWorldState(map[string]core.Determination{
		"goalKey": core.True,
	})
	g := &core.Goal{Name: "g", Pre: []string{"goalKey"}}
	system := plan.NewPlanningSystem(nil, []*core.Goal{g}, nil)

	pl, err := reactive.NewPlanner().PlanToGoal(context.Background(), start, system, g, plan.PlanOptions{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl == nil || len(pl.Actions) != 0 {
		t.Fatalf("expected empty plan when goal is already satisfied, got %#v", pl)
	}
}

func TestReactive_PicksHighestProgressAction(t *testing.T) {
	start := plan.EmptyWorldState()
	g := &core.Goal{Name: "g", Pre: []string{"a", "b"}}

	weak := newAction("weak", nil, core.EffectSpec{"a": core.True}, 1)
	strong := newAction("strong", nil, core.EffectSpec{"a": core.True, "b": core.True}, 5)

	system := plan.NewPlanningSystem([]core.Action{weak, strong}, []*core.Goal{g}, nil)
	pl, err := reactive.NewPlanner().PlanToGoal(context.Background(), start, system, g, plan.PlanOptions{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl == nil || len(pl.Actions) != 1 {
		t.Fatalf("expected single-action plan, got %#v", pl)
	}
	if pl.Actions[0].Metadata().Name != "strong" {
		t.Fatalf("expected highest-progress action 'strong', got %q", pl.Actions[0].Metadata().Name)
	}
}

func TestReactive_TieBreaksByLowerCost(t *testing.T) {
	start := plan.EmptyWorldState()
	g := &core.Goal{Name: "g", Pre: []string{"a"}}

	cheap := newAction("cheap", nil, core.EffectSpec{"a": core.True}, 1)
	expensive := newAction("expensive", nil, core.EffectSpec{"a": core.True}, 5)

	system := plan.NewPlanningSystem([]core.Action{expensive, cheap}, []*core.Goal{g}, nil)
	pl, _ := reactive.NewPlanner().PlanToGoal(context.Background(), start, system, g, plan.PlanOptions{})
	if pl.Actions[0].Metadata().Name != "cheap" {
		t.Fatalf("expected tie-break to cheaper action, got %q", pl.Actions[0].Metadata().Name)
	}
}

func TestReactive_SkipsInapplicable(t *testing.T) {
	start := plan.EmptyWorldState()
	g := &core.Goal{Name: "g", Pre: []string{"a"}}

	blocked := newAction("blocked",
		core.EffectSpec{"setup": core.True},   // precondition not met in start
		core.EffectSpec{"a": core.True}, 1)
	open := newAction("open", nil, core.EffectSpec{"a": core.True}, 2)

	system := plan.NewPlanningSystem([]core.Action{blocked, open}, []*core.Goal{g}, nil)
	pl, _ := reactive.NewPlanner().PlanToGoal(context.Background(), start, system, g, plan.PlanOptions{})
	if pl == nil || pl.Actions[0].Metadata().Name != "open" {
		t.Fatalf("expected applicable action 'open', got %#v", pl)
	}
}

func TestReactive_NoApplicableActionReturnsNil(t *testing.T) {
	start := plan.EmptyWorldState()
	g := &core.Goal{Name: "g", Pre: []string{"a"}}
	junk := newAction("junk", nil, core.EffectSpec{"unrelated": core.True}, 1)

	system := plan.NewPlanningSystem([]core.Action{junk}, []*core.Goal{g}, nil)
	pl, err := reactive.NewPlanner().PlanToGoal(context.Background(), start, system, g, plan.PlanOptions{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl != nil {
		t.Fatalf("expected nil plan when no action makes progress, got %#v", pl)
	}
}

func TestReactive_RespectsExclusion(t *testing.T) {
	start := plan.EmptyWorldState()
	g := &core.Goal{Name: "g", Pre: []string{"a"}}

	preferred := newAction("preferred", nil, core.EffectSpec{"a": core.True}, 1)
	fallback := newAction("fallback", nil, core.EffectSpec{"a": core.True}, 5)

	system := plan.NewPlanningSystem([]core.Action{preferred, fallback}, []*core.Goal{g}, nil)
	pl, _ := reactive.NewPlanner().PlanToGoal(context.Background(), start, system, g, plan.PlanOptions{
		ExcludedActions: map[string]struct{}{"preferred": {}},
	})
	if pl.Actions[0].Metadata().Name != "fallback" {
		t.Fatalf("expected exclusion to leave fallback, got %q", pl.Actions[0].Metadata().Name)
	}
}

func TestReactive_BestValuePlanRanksByNetValue(t *testing.T) {
	start := plan.EmptyWorldState()

	a := newAction("a", nil, core.EffectSpec{"x": core.True}, 1)
	b := newAction("b", nil, core.EffectSpec{"y": core.True}, 1)

	low := &core.Goal{Name: "low", Pre: []string{"x"}, Value: core.Static(2)}
	high := &core.Goal{Name: "high", Pre: []string{"y"}, Value: core.Static(10)}

	system := plan.NewPlanningSystem([]core.Action{a, b}, []*core.Goal{low, high}, nil)
	pl, _ := plan.BestValuePlan(context.Background(), reactive.NewPlanner(), start, system, plan.PlanOptions{})
	if pl == nil || pl.Goal.Name != "high" {
		t.Fatalf("expected high-value goal, got %#v", pl)
	}
}
