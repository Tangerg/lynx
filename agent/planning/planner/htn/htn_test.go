package htn_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/planner/htn"
)

// mustHTNPlanner is a tiny test helper for the (*Planner, error)
// shape of htn.NewPlanner — fail the test on a non-nil error.
func mustHTNPlanner(t *testing.T, lib *htn.Library) *htn.Planner {
	t.Helper()
	p, err := htn.NewPlanner(lib)
	if err != nil {
		t.Fatalf("htn.NewPlanner: %v", err)
	}
	return p
}

type fakeAction struct {
	meta core.ActionMetadata
}

func (a *fakeAction) Metadata() core.ActionMetadata { return a.meta }
func (a *fakeAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionFailed
}

func newAction(name string, eff core.Effects) core.Action {
	return &fakeAction{meta: core.ActionMetadata{
		Name:    name,
		Effects: eff,
		Cost:    core.Static(1),
		Value:   core.Static(0),
	}}
}

func names(actions []core.Action) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, a.Metadata().Name)
	}
	return out
}

func TestHTN_PrimitiveTaskEmitsAction(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "do_thing", Action: newAction("thing", core.Effects{"done": core.True})})

	g := &core.Goal{Name: "do_thing", Pre: []string{"done"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)

	pl, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.EmptyWorldState(), system, g, planning.Options{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl == nil {
		t.Fatal("expected plan, got nil")
	}
	if got := names(pl.Actions); len(got) != 1 || got[0] != "thing" {
		t.Fatalf("expected [thing], got %v", got)
	}
}

func TestHTN_CompoundTaskDecomposesIntoSubtaskOrder(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "step_a", Action: newAction("a", core.Effects{"a_done": core.True})})
	lib.MustAdd(&htn.Task{Name: "step_b", Action: newAction("b", core.Effects{"b_done": core.True})})
	lib.MustAdd(&htn.Task{Name: "build_thing", Methods: []htn.Method{
		{Name: "default", Subtasks: []string{"step_a", "step_b"}},
	}})

	g := &core.Goal{Name: "build_thing", Pre: []string{"b_done"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)
	pl, _ := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.EmptyWorldState(), system, g, planning.Options{})
	if got := names(pl.Actions); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestHTN_MethodPreconditionGate(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "fast", Action: newAction("fast", core.Effects{"served": core.True})})
	lib.MustAdd(&htn.Task{Name: "slow", Action: newAction("slow", core.Effects{"served": core.True})})
	lib.MustAdd(&htn.Task{Name: "serve", Methods: []htn.Method{
		// First method requires "ready=true" — falls through when not.
		{Name: "express", Preconditions: core.Effects{"ready": core.True}, Subtasks: []string{"fast"}},
		{Name: "fallback", Subtasks: []string{"slow"}},
	}})

	g := &core.Goal{Name: "serve", Pre: []string{"served"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)
	planner := mustHTNPlanner(t, lib)

	// Without "ready" → falls back to slow.
	pl, _ := planner.PlanToGoal(t.Context(), planning.EmptyWorldState(), system, g, planning.Options{})
	if names(pl.Actions)[0] != "slow" {
		t.Fatalf("expected fallback path 'slow', got %v", names(pl.Actions))
	}

	// With "ready" → express path picks fast.
	ready := planning.NewConditionWorldState(map[string]core.Determination{"ready": core.True})
	pl, _ = planner.PlanToGoal(t.Context(), ready, system, g, planning.Options{})
	if names(pl.Actions)[0] != "fast" {
		t.Fatalf("expected express path 'fast', got %v", names(pl.Actions))
	}
}

func TestHTN_GoalWithoutMatchingTaskReturnsNil(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "registered", Action: newAction("a", core.Effects{"x": core.True})})

	g := &core.Goal{Name: "unregistered", Pre: []string{"x"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)

	pl, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.EmptyWorldState(), system, g, planning.Options{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl != nil {
		t.Fatalf("expected nil plan when goal has no matching task, got %#v", pl)
	}
}

func TestHTN_RejectsBadTaskShapes(t *testing.T) {
	lib := htn.NewLibrary()

	if err := lib.Add(nil); err == nil {
		t.Fatal("expected error on nil task")
	}
	if err := lib.Add(&htn.Task{}); err == nil {
		t.Fatal("expected error on empty name")
	}
	if err := lib.Add(&htn.Task{Name: "x"}); err == nil {
		t.Fatal("expected error on neither Action nor Methods")
	}
	if err := lib.Add(&htn.Task{Name: "x",
		Action:  newAction("a", nil),
		Methods: []htn.Method{{Name: "m"}},
	}); err == nil {
		t.Fatal("expected error on both Action and Methods set")
	}

	lib.MustAdd(&htn.Task{Name: "ok", Action: newAction("a", nil)})
	if err := lib.Add(&htn.Task{Name: "ok", Action: newAction("b", nil)}); err == nil {
		t.Fatal("expected error on duplicate name")
	}
}

func TestHTN_BacktracksWhenFirstMethodSubtaskMissing(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "step_b", Action: newAction("b", core.Effects{"done": core.True})})
	// First method tries an unknown task — this surfaces as an error,
	// not silent fallback. Second method works.
	lib.MustAdd(&htn.Task{Name: "do", Methods: []htn.Method{
		{Name: "broken", Subtasks: []string{"missing"}},
		{Name: "good", Subtasks: []string{"step_b"}},
	}})

	g := &core.Goal{Name: "do", Pre: []string{"done"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)
	_, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.EmptyWorldState(), system, g, planning.Options{})
	if err == nil {
		t.Fatal("expected error when method references unknown subtask (no silent backtrack on missing names)")
	}
}

func TestHTN_RespectsExclusion(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "primary", Action: newAction("primary", core.Effects{"done": core.True})})
	lib.MustAdd(&htn.Task{Name: "fallback", Action: newAction("fallback", core.Effects{"done": core.True})})
	lib.MustAdd(&htn.Task{Name: "do", Methods: []htn.Method{
		{Name: "first", Subtasks: []string{"primary"}},
		{Name: "second", Subtasks: []string{"fallback"}},
	}})

	g := &core.Goal{Name: "do", Pre: []string{"done"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)
	pl, _ := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.EmptyWorldState(), system, g, planning.Options{
		ExcludedActions: map[string]struct{}{"primary": {}},
	})
	if names(pl.Actions)[0] != "fallback" {
		t.Fatalf("expected exclusion to drop 'primary', got %v", names(pl.Actions))
	}
}

func TestHTN_BestValuePlanRanksByGoalValue(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "low_goal", Action: newAction("a", core.Effects{"x": core.True})})
	lib.MustAdd(&htn.Task{Name: "high_goal", Action: newAction("b", core.Effects{"y": core.True})})

	low := &core.Goal{Name: "low_goal", Pre: []string{"x"}, Value: core.Static(2)}
	high := &core.Goal{Name: "high_goal", Pre: []string{"y"}, Value: core.Static(10)}

	system := planning.NewSystem(nil, []*core.Goal{low, high}, nil)
	pl, _ := planning.BestValuePlan(t.Context(), mustHTNPlanner(t, lib), planning.EmptyWorldState(), system, planning.Options{})
	if pl.Goal.Name != "high_goal" {
		t.Fatalf("expected high_goal, got %q", pl.Goal.Name)
	}
}
