package reactive_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/reactive"
)

// fakeAction is a planner-only Action — Execute is never called by the
// reactive planner, so we only need to satisfy the interface.
type fakeAction struct {
	meta core.ActionMetadata
}

func (a *fakeAction) Metadata() core.ActionMetadata { return a.meta }
func (a *fakeAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionFailed, nil
}

func newAction(name string, pre, eff core.ConditionSet, cost float64) core.Action {
	return &fakeAction{meta: core.ActionMetadata{
		Name:          name,
		Preconditions: pre,
		Effects:       eff,
		Cost:          core.FixedScore(cost),
		Value:         core.FixedScore(0),
	}}
}

func TestReactive_AlreadySatisfiedReturnsEmptyPlan(t *testing.T) {
	start := planning.NewState(map[string]core.Truth{
		"goalKey": core.True,
	})
	g := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"goalKey"}})
	domain := planning.NewDomain(nil, []*core.Goal{g}, nil)

	pl, err := reactive.NewPlanner().PlanToGoal(t.Context(), start, domain, g, planning.Options{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl == nil || len(pl.Actions()) != 0 {
		t.Fatalf("expected empty plan when goal is already satisfied, got %#v", pl)
	}
}

func TestReactive_PicksHighestProgressAction(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"a", "b"}})

	weak := newAction("weak", nil, core.ConditionSet{"a": core.True}, 1)
	strong := newAction("strong", nil, core.ConditionSet{"a": core.True, "b": core.True}, 5)

	domain := planning.NewDomain([]core.Action{weak, strong}, []*core.Goal{g}, nil)
	pl, err := reactive.NewPlanner().PlanToGoal(t.Context(), start, domain, g, planning.Options{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl == nil || len(pl.Actions()) != 1 {
		t.Fatalf("expected single-action plan, got %#v", pl)
	}
	if pl.Actions()[0].Metadata().Name != "strong" {
		t.Fatalf("expected highest-progress action 'strong', got %q", pl.Actions()[0].Metadata().Name)
	}
}

func TestReactive_TieBreaksByLowerCost(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"a"}})

	cheap := newAction("cheap", nil, core.ConditionSet{"a": core.True}, 1)
	expensive := newAction("expensive", nil, core.ConditionSet{"a": core.True}, 5)

	domain := planning.NewDomain([]core.Action{expensive, cheap}, []*core.Goal{g}, nil)
	pl, _ := reactive.NewPlanner().PlanToGoal(t.Context(), start, domain, g, planning.Options{})
	if pl.Actions()[0].Metadata().Name != "cheap" {
		t.Fatalf("expected tie-break to cheaper action, got %q", pl.Actions()[0].Metadata().Name)
	}
}

func TestReactive_SkipsInapplicable(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"a"}})

	blocked := newAction("blocked",
		core.ConditionSet{"setup": core.True}, // precondition not met in start
		core.ConditionSet{"a": core.True}, 1)
	open := newAction("open", nil, core.ConditionSet{"a": core.True}, 2)

	domain := planning.NewDomain([]core.Action{blocked, open}, []*core.Goal{g}, nil)
	pl, _ := reactive.NewPlanner().PlanToGoal(t.Context(), start, domain, g, planning.Options{})
	if pl == nil || pl.Actions()[0].Metadata().Name != "open" {
		t.Fatalf("expected applicable action 'open', got %#v", pl)
	}
}

func TestReactive_NoApplicableActionReturnsNil(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"a"}})
	junk := newAction("junk", nil, core.ConditionSet{"unrelated": core.True}, 1)

	domain := planning.NewDomain([]core.Action{junk}, []*core.Goal{g}, nil)
	pl, err := reactive.NewPlanner().PlanToGoal(t.Context(), start, domain, g, planning.Options{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl != nil {
		t.Fatalf("expected nil plan when no action makes progress, got %#v", pl)
	}
}

func TestReactive_RespectsExclusion(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"a"}})

	preferred := newAction("preferred", nil, core.ConditionSet{"a": core.True}, 1)
	fallback := newAction("fallback", nil, core.ConditionSet{"a": core.True}, 5)

	domain := planning.NewDomain([]core.Action{preferred, fallback}, []*core.Goal{g}, nil)
	pl, _ := reactive.NewPlanner().PlanToGoal(t.Context(), start, domain, g, planning.Options{
		ExcludedActions: planning.NewExclusions("preferred"),
	})
	if pl.Actions()[0].Metadata().Name != "fallback" {
		t.Fatalf("expected exclusion to leave fallback, got %q", pl.Actions()[0].Metadata().Name)
	}
}

func TestReactive_BestValuePlanRanksByNetValue(t *testing.T) {
	start := planning.NewState(nil)

	a := newAction("a", nil, core.ConditionSet{"x": core.True}, 1)
	b := newAction("b", nil, core.ConditionSet{"y": core.True}, 1)

	low := core.NewGoal(core.GoalConfig{Name: "low", Preconditions: []string{"x"}, Value: core.FixedScore(2)})
	high := core.NewGoal(core.GoalConfig{Name: "high", Preconditions: []string{"y"}, Value: core.FixedScore(10)})

	domain := planning.NewDomain([]core.Action{a, b}, []*core.Goal{low, high}, nil)
	pl, _ := domain.BestPlan(t.Context(), reactive.NewPlanner(), start, planning.Options{})
	if pl == nil || pl.Goal().Name() != "high" {
		t.Fatalf("expected high-value goal, got %#v", pl)
	}
}
