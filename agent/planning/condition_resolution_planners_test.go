package planning_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/goap"
	"github.com/Tangerg/lynx/agent/planning/htn"
	"github.com/Tangerg/lynx/agent/planning/reactive"
	"github.com/Tangerg/lynx/agent/planning/utility"
)

func TestBuiltInPlannersResolveRequiredConditions(t *testing.T) {
	action := &planAction{metadata: core.ActionMetadata{
		Name:          "finish",
		Preconditions: core.ConditionSet{"ready": core.True},
		Effects:       core.ConditionSet{"done": core.True},
		Cost: func(state core.WorldState) float64 {
			if state.Conditions()["ready"] != core.True {
				return -1
			}
			return 1
		},
		Value: core.FixedScore(1),
	}}
	goal := core.NewGoal(core.GoalConfig{Name: "complete", RequiredConditions: []string{"done"}})
	condition := core.NewCondition(core.ConditionConfig{Name: "ready", EvaluationCost: 1})
	domain := mustDomain(t, []core.Action{action}, []*core.Goal{goal}, []core.Condition{condition})

	library := htn.NewLibrary()
	library.MustAdd(&htn.Task{Name: "finish-task", ActionName: "finish"})
	library.MustAdd(&htn.Task{Name: "complete", Methods: []htn.Method{{Name: "finish", Subtasks: []string{"finish-task"}}}})
	htnPlanner, err := htn.NewPlanner(library)
	if err != nil {
		t.Fatalf("htn.NewPlanner: %v", err)
	}

	planners := []planning.Planner{
		goap.NewPlanner(),
		htnPlanner,
		reactive.NewPlanner(),
		utility.NewPlanner(),
		utility.NewGoalFirst(),
	}
	for _, planner := range planners {
		t.Run(planner.Name(), func(t *testing.T) {
			calls := 0
			resolver := newConditionResolver(func(_ context.Context, name string) (core.Truth, error) {
				calls++
				if name != "ready" {
					t.Fatalf("resolved condition = %q, want ready", name)
				}
				return core.True, nil
			})

			plan, err := planner.PlanToGoal(
				t.Context(),
				planning.NewState(nil),
				domain,
				goal,
				planning.Options{ConditionResolver: resolver},
			)
			if err != nil {
				t.Fatalf("PlanToGoal: %v", err)
			}
			if plan == nil || len(plan.Actions()) != 1 || plan.Actions()[0].Metadata().Name != "finish" {
				t.Fatalf("plan = %#v, want finish", plan)
			}
			if calls != 1 {
				t.Fatalf("resolver calls = %d, want one", calls)
			}
		})
	}
}
