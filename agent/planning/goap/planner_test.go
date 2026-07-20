package goap

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

func mustDomain(t *testing.T, actions []core.Action, goals []*core.Goal, conditions []core.Condition) *planning.Domain {
	t.Helper()
	domain, err := planning.NewDomain(actions, goals, conditions)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	return domain
}

func TestPlannerFindsCheapestMultiEffectPlan(t *testing.T) {
	combined := newStubAction(
		"combined",
		nil,
		core.ConditionSet{"a": core.True, "b": core.True},
	)
	combined.meta.Cost = core.FixedScore(1.1)
	setA := newStubAction("set-a", nil, core.ConditionSet{"a": core.True})
	setA.meta.Cost = core.FixedScore(0.5)
	setB := newStubAction("set-b", nil, core.ConditionSet{"b": core.True})
	setB.meta.Cost = core.FixedScore(0.5)

	goal := core.NewGoal(core.GoalConfig{
		Name:          "both",
		Preconditions: []string{"a", "b"},
	})
	domain := mustDomain(t,
		[]core.Action{combined, setA, setB},
		[]*core.Goal{goal},
		nil,
	)

	plan, err := NewPlanner().PlanToGoal(
		t.Context(),
		planning.NewState(nil),
		domain,
		goal,
		planning.Options{},
	)
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if plan == nil {
		t.Fatal("PlanToGoal returned no plan")
	}
	if got := actionNames(plan.Actions()); !slices.Equal(got, []string{"set-a", "set-b"}) {
		t.Fatalf("plan actions = %v, want cheapest two-step plan", got)
	}
}

func TestPlannerEvaluatesDynamicCostAgainstTransitionSource(t *testing.T) {
	prepare := newStubAction("prepare", nil, core.ConditionSet{"ready": core.True})
	prepare.meta.Cost = core.FixedScore(0)
	finish := newStubAction(
		"finish",
		core.ConditionSet{"ready": core.True},
		core.ConditionSet{"done": core.True},
	)
	var sawReady bool
	finish.meta.Cost = func(state core.WorldState) float64 {
		sawReady = state.Conditions()["ready"] == core.True
		if sawReady {
			return 1
		}
		return 100
	}
	direct := newStubAction("direct", nil, core.ConditionSet{"done": core.True})
	direct.meta.Cost = core.FixedScore(50)
	goal := core.NewGoal(core.GoalConfig{Name: "done", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{prepare, finish, direct}, []*core.Goal{goal}, nil)

	plan, err := NewPlanner().PlanToGoal(
		t.Context(),
		planning.NewState(nil),
		domain,
		goal,
		planning.Options{},
	)
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if !sawReady {
		t.Fatal("dynamic cost did not observe the transition source state")
	}
	if got := actionNames(plan.Actions()); !slices.Equal(got, []string{"prepare", "finish"}) {
		t.Fatalf("plan actions = %v, want state-aware cheapest path", got)
	}
}

func TestPlannerPreservesActionsThatOnlyInfluenceLaterCost(t *testing.T) {
	discount := newStubAction("discount", nil, core.ConditionSet{"discounted": core.True})
	discount.meta.Cost = core.FixedScore(1)
	finish := newStubAction("finish", nil, core.ConditionSet{"done": core.True})
	finish.meta.Cost = func(state core.WorldState) float64 {
		if state.Conditions()["discounted"] == core.True {
			return 1
		}
		return 100
	}
	goal := core.NewGoal(core.GoalConfig{Name: "done", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{discount, finish}, []*core.Goal{goal}, nil)

	plan, err := NewPlanner().PlanToGoal(
		t.Context(),
		planning.NewState(nil),
		domain,
		goal,
		planning.Options{},
	)
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if got := actionNames(plan.Actions()); !slices.Equal(got, []string{"discount", "finish"}) {
		t.Fatalf("plan actions = %v, want cost-influencing prefix preserved", got)
	}
}

func TestPlannerRejectsInvalidActionCost(t *testing.T) {
	for _, test := range []struct {
		name string
		cost float64
	}{
		{name: "negative", cost: -1},
		{name: "nan", cost: math.NaN()},
		{name: "positive infinity", cost: math.Inf(1)},
		{name: "negative infinity", cost: math.Inf(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			action := newStubAction("invalid-cost", nil, core.ConditionSet{"done": core.True})
			action.meta.Cost = core.FixedScore(test.cost)
			goal := core.NewGoal(core.GoalConfig{Name: "done", Preconditions: []string{"done"}})
			domain := mustDomain(t, []core.Action{action}, []*core.Goal{goal}, nil)

			_, err := NewPlanner().PlanToGoal(
				t.Context(),
				planning.NewState(nil),
				domain,
				goal,
				planning.Options{},
			)
			if !errors.Is(err, ErrInvalidActionCost) {
				t.Fatalf("PlanToGoal error = %v, want ErrInvalidActionCost", err)
			}
		})
	}
}

type stubAction struct {
	meta core.ActionMetadata
}

func (s stubAction) Metadata() core.ActionMetadata { return s.meta }

func (stubAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

func newStubAction(name string, preconditions, effects core.ConditionSet) stubAction {
	return stubAction{
		meta: core.ActionMetadata{
			Name:          name,
			Preconditions: preconditions,
			Effects:       effects,
		},
	}
}

func actionNames(actions []core.Action) []string {
	names := make([]string, len(actions))
	for index, action := range actions {
		names[index] = action.Metadata().Name
	}
	return names
}
