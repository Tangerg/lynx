package htn_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/htn"
)

func mustDomain(t *testing.T, actions []core.Action, goals []*core.Goal, conditions []core.Condition) *planning.Domain {
	t.Helper()
	domain, err := planning.NewDomain(actions, goals, conditions)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	return domain
}

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
func (a *fakeAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionFailed, nil
}

func newAction(name string, eff core.ConditionSet) core.Action {
	return &fakeAction{meta: core.ActionMetadata{
		Name:    name,
		Effects: eff,
		Cost:    core.FixedScore(1),
		Value:   core.FixedScore(0),
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
	action := newAction("thing", core.ConditionSet{"done": core.True})
	lib.MustAdd(&htn.Task{Name: "do_thing", Action: action})

	g := core.NewGoal(core.GoalConfig{Name: "do_thing", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{action}, []*core.Goal{g}, nil)

	pl, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.NewState(nil), domain, g, planning.Options{})
	if err != nil {
		t.Fatalf("PlanToGoal: %v", err)
	}
	if pl == nil {
		t.Fatal("expected plan, got nil")
	}
	if got := names(pl.Actions()); len(got) != 1 || got[0] != "thing" {
		t.Fatalf("expected [thing], got %v", got)
	}
}

func TestHTN_CompoundTaskDecomposesIntoSubtaskOrder(t *testing.T) {
	lib := htn.NewLibrary()
	actionA := newAction("a", core.ConditionSet{"a_done": core.True})
	actionB := newAction("b", core.ConditionSet{"b_done": core.True})
	lib.MustAdd(&htn.Task{Name: "step_a", Action: actionA})
	lib.MustAdd(&htn.Task{Name: "step_b", Action: actionB})
	lib.MustAdd(&htn.Task{Name: "build_thing", Methods: []htn.Method{
		{Name: "default", Subtasks: []string{"step_a", "step_b"}},
	}})

	g := core.NewGoal(core.GoalConfig{Name: "build_thing", Preconditions: []string{"b_done"}})
	domain := mustDomain(t, []core.Action{actionA, actionB}, []*core.Goal{g}, nil)
	pl, _ := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.NewState(nil), domain, g, planning.Options{})
	if got := names(pl.Actions()); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestHTN_MethodPreconditionGate(t *testing.T) {
	lib := htn.NewLibrary()
	fast := newAction("fast", core.ConditionSet{"served": core.True})
	slow := newAction("slow", core.ConditionSet{"served": core.True})
	lib.MustAdd(&htn.Task{Name: "fast", Action: fast})
	lib.MustAdd(&htn.Task{Name: "slow", Action: slow})
	lib.MustAdd(&htn.Task{Name: "serve", Methods: []htn.Method{
		// First method requires "ready=true" — falls through when not.
		{Name: "express", Preconditions: core.ConditionSet{"ready": core.True}, Subtasks: []string{"fast"}},
		{Name: "fallback", Subtasks: []string{"slow"}},
	}})

	g := core.NewGoal(core.GoalConfig{Name: "serve", Preconditions: []string{"served"}})
	domain := mustDomain(t, []core.Action{fast, slow}, []*core.Goal{g}, nil)
	planner := mustHTNPlanner(t, lib)

	// Without "ready" → falls back to slow.
	pl, _ := planner.PlanToGoal(t.Context(), planning.NewState(nil), domain, g, planning.Options{})
	if names(pl.Actions())[0] != "slow" {
		t.Fatalf("expected fallback path 'slow', got %v", names(pl.Actions()))
	}

	// With "ready" → express path picks fast.
	ready := planning.NewState(map[string]core.Truth{"ready": core.True})
	pl, _ = planner.PlanToGoal(t.Context(), ready, domain, g, planning.Options{})
	if names(pl.Actions())[0] != "fast" {
		t.Fatalf("expected express path 'fast', got %v", names(pl.Actions()))
	}
}

func TestHTN_GoalWithoutMatchingTaskReturnsNil(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "registered", Action: newAction("a", core.ConditionSet{"x": core.True})})

	g := core.NewGoal(core.GoalConfig{Name: "unregistered", Preconditions: []string{"x"}})
	domain := mustDomain(t, nil, []*core.Goal{g}, nil)

	pl, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.NewState(nil), domain, g, planning.Options{})
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

func TestHTN_RejectsUnknownSubtaskAtConstruction(t *testing.T) {
	lib := htn.NewLibrary()
	lib.MustAdd(&htn.Task{Name: "step_b", Action: newAction("b", core.ConditionSet{"done": core.True})})
	// First method tries an unknown task — this surfaces as an error,
	// not silent fallback. Second method works.
	lib.MustAdd(&htn.Task{Name: "do", Methods: []htn.Method{
		{Name: "broken", Subtasks: []string{"missing"}},
		{Name: "good", Subtasks: []string{"step_b"}},
	}})

	_, err := htn.NewPlanner(lib)
	if err == nil {
		t.Fatal("expected unknown subtask to reject planner construction")
	}
}

func TestHTN_RespectsExclusion(t *testing.T) {
	lib := htn.NewLibrary()
	primary := newAction("primary", core.ConditionSet{"done": core.True})
	fallback := newAction("fallback", core.ConditionSet{"done": core.True})
	lib.MustAdd(&htn.Task{Name: "primary", Action: primary})
	lib.MustAdd(&htn.Task{Name: "fallback", Action: fallback})
	lib.MustAdd(&htn.Task{Name: "do", Methods: []htn.Method{
		{Name: "first", Subtasks: []string{"primary"}},
		{Name: "second", Subtasks: []string{"fallback"}},
	}})

	g := core.NewGoal(core.GoalConfig{Name: "do", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{primary, fallback}, []*core.Goal{g}, nil)
	pl, _ := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.NewState(nil), domain, g, planning.Options{
		ExcludedActions: planning.NewExclusions("primary"),
	})
	if names(pl.Actions())[0] != "fallback" {
		t.Fatalf("expected exclusion to drop 'primary', got %v", names(pl.Actions()))
	}
}

func TestHTN_BestValuePlanRanksByGoalValue(t *testing.T) {
	lib := htn.NewLibrary()
	actionA := newAction("a", core.ConditionSet{"x": core.True})
	actionB := newAction("b", core.ConditionSet{"y": core.True})
	lib.MustAdd(&htn.Task{Name: "low_goal", Action: actionA})
	lib.MustAdd(&htn.Task{Name: "high_goal", Action: actionB})

	low := core.NewGoal(core.GoalConfig{Name: "low_goal", Preconditions: []string{"x"}, Value: core.FixedScore(2)})
	high := core.NewGoal(core.GoalConfig{Name: "high_goal", Preconditions: []string{"y"}, Value: core.FixedScore(10)})

	domain := mustDomain(t, []core.Action{actionA, actionB}, []*core.Goal{low, high}, nil)
	pl, _ := domain.BestPlan(t.Context(), mustHTNPlanner(t, lib), planning.NewState(nil), planning.Options{})
	if pl.Goal().Name() != "high_goal" {
		t.Fatalf("expected high_goal, got %q", pl.Goal().Name())
	}
}

func TestHTN_AlreadySatisfiedGoalReturnsEmptyPlan(t *testing.T) {
	lib := htn.NewLibrary()
	action := newAction("work", core.ConditionSet{"done": core.True})
	lib.MustAdd(&htn.Task{Name: "goal", Action: action})
	goal := core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{action}, []*core.Goal{goal}, nil)

	plan, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.NewState(core.ConditionSet{"done": core.True}), domain, goal, planning.Options{})
	if err != nil || plan == nil || !plan.Complete() {
		t.Fatalf("PlanToGoal() = %#v, %v; want complete plan", plan, err)
	}
}

func TestHTN_PrimitivePreconditionsMustHold(t *testing.T) {
	lib := htn.NewLibrary()
	action := newAction("work", core.ConditionSet{"done": core.True})
	action.(*fakeAction).meta.Preconditions = core.ConditionSet{"ready": core.True}
	lib.MustAdd(&htn.Task{Name: "goal", Action: action})
	goal := core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{action}, []*core.Goal{goal}, nil)

	plan, err := mustHTNPlanner(t, lib).PlanToGoal(t.Context(), planning.NewState(nil), domain, goal, planning.Options{})
	if err != nil || plan != nil {
		t.Fatalf("PlanToGoal() = %#v, %v; want nil plan", plan, err)
	}
}

func TestHTN_PlannerOwnsLibrarySnapshot(t *testing.T) {
	lib := htn.NewLibrary()
	action := newAction("work", core.ConditionSet{"done": core.True})
	task := &htn.Task{Name: "goal", Action: action}
	lib.MustAdd(task)
	planner := mustHTNPlanner(t, lib)

	task.Name = "mutated"
	lib.MustAdd(&htn.Task{Name: "later", Action: newAction("later", core.ConditionSet{"later": core.True})})
	goal := core.NewGoal(core.GoalConfig{Name: "goal", Preconditions: []string{"done"}})
	domain := mustDomain(t, []core.Action{action}, []*core.Goal{goal}, nil)
	plan, err := planner.PlanToGoal(t.Context(), planning.NewState(nil), domain, goal, planning.Options{})
	if err != nil || plan == nil || len(plan.Actions()) != 1 {
		t.Fatalf("snapshot planner result = %#v, %v", plan, err)
	}
}
