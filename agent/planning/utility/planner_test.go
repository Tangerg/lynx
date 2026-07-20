package utility_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/utility"
)

func mustDomain(t *testing.T, actions []core.Action, goals []*core.Goal, conditions []core.Condition) *planning.Domain {
	t.Helper()
	domain, err := planning.NewDomain(actions, goals, conditions)
	if err != nil {
		t.Fatalf("NewDomain: %v", err)
	}
	return domain
}

// fakeAction satisfies core.Action for planner-only tests.
type fakeAction struct{ meta core.ActionMetadata }

func (a *fakeAction) Metadata() core.ActionMetadata { return a.meta }
func (a *fakeAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionFailed, nil
}

// newAction builds a fakeAction with the given pre / eff / cost / value.
func newAction(name string, pre, eff core.ConditionSet, cost, value float64) core.Action {
	return &fakeAction{meta: core.ActionMetadata{
		Name:          name,
		Preconditions: pre,
		Effects:       eff,
		Cost:          core.FixedScore(cost),
		Value:         core.FixedScore(value),
	}}
}

// --- Planner (classic Utility) ----------------------------------------------

func TestUtility_NirvanaPicksHighestNetValue(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: utility.OpenEndedGoalName})

	low := newAction("low", nil, core.ConditionSet{"a": core.True}, 1, 2)    // net = 1
	high := newAction("high", nil, core.ConditionSet{"a": core.True}, 1, 10) // net = 9

	domain := mustDomain(t, []core.Action{low, high}, []*core.Goal{g}, nil)
	pl, err := utility.NewPlanner().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if pl == nil || len(pl.Actions()) != 1 || pl.Actions()[0].Metadata().Name != "high" {
		t.Fatalf("expected 'high' picked, got %#v", pl)
	}
}

func TestUtility_NirvanaWithNoActionsReturnsNil(t *testing.T) {
	g := core.NewGoal(core.GoalConfig{Name: utility.OpenEndedGoalName})
	domain := mustDomain(t, nil, []*core.Goal{g}, nil)

	pl, err := utility.NewPlanner().PlanToGoal(context.Background(), planning.NewState(nil), domain, g, planning.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if pl != nil {
		t.Errorf("expected nil plan for open-ended goal with no actions, got %#v", pl)
	}
}

func TestUtility_AlreadySatisfiedNoActions(t *testing.T) {
	start := planning.NewState(map[string]core.Truth{"goalKey": core.True})
	g := core.NewGoal(core.GoalConfig{Name: "real", Preconditions: []string{"goalKey"}})
	domain := mustDomain(t, nil, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if pl == nil || len(pl.Actions()) != 0 {
		t.Errorf("expected empty plan when already satisfied + no actions, got %#v", pl)
	}
}

func TestUtility_OneStepLookaheadEmitsPlan(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "real", Preconditions: []string{"done"}})

	a := newAction("a", nil, core.ConditionSet{"done": core.True}, 1, 5)
	domain := mustDomain(t, []core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if pl == nil || len(pl.Actions()) != 1 || pl.Actions()[0].Metadata().Name != "a" {
		t.Fatalf("expected 1-step plan via 'a', got %#v", pl)
	}
}

func TestUtility_OneStepLookaheadInsufficientReturnsNil(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "real", Preconditions: []string{"step1", "step2"}})

	a := newAction("a", nil, core.ConditionSet{"step1": core.True}, 1, 5)
	domain := mustDomain(t, []core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if pl != nil {
		t.Errorf("Utility refuses multi-step plans for real goals; got %#v", pl)
	}
}

func TestUtility_ExcludedActionsSkipped(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "real", Preconditions: []string{"done"}})
	a := newAction("a", nil, core.ConditionSet{"done": core.True}, 1, 5)
	domain := mustDomain(t, []core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, domain, g,
		planning.Options{ExcludedActions: planning.NewExclusions("a")})
	if pl != nil {
		t.Errorf("excluded action should not be picked; got %#v", pl)
	}
}

// --- GoalFirst ----------------------------------------------------------

func TestHybridUtility_SatisfiedFirstShortCircuit(t *testing.T) {
	// Goal already satisfied AND a high-value action is still
	// applicable. Hybrid returns empty plan; classic Utility would
	// pick the action.
	start := planning.NewState(map[string]core.Truth{"goalKey": core.True})
	g := core.NewGoal(core.GoalConfig{Name: "real", Preconditions: []string{"goalKey"}})

	stillRunnable := newAction("nop", nil, core.ConditionSet{"other": core.True}, 1, 99)
	domain := mustDomain(t, []core.Action{stillRunnable}, []*core.Goal{g}, nil)

	// Hybrid: empty planning.
	plH, _ := utility.NewGoalFirst().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if plH == nil || len(plH.Actions()) != 0 {
		t.Errorf("hybrid: want empty plan, got %#v", plH)
	}

	// Classic: picks the action.
	plU, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if plU == nil || len(plU.Actions()) != 1 {
		t.Errorf("classic: want 1-step plan, got %#v", plU)
	}
}

func TestHybridUtility_NirvanaSemanticsMatchClassic(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: utility.OpenEndedGoalName})

	a := newAction("a", nil, core.ConditionSet{"x": core.True}, 1, 5)
	domain := mustDomain(t, []core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewGoalFirst().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if pl == nil || len(pl.Actions()) != 1 {
		t.Fatalf("hybrid open-ended goal: want 1-step plan, got %#v", pl)
	}
}

func TestHybridUtility_OneStepReachesGoal(t *testing.T) {
	start := planning.NewState(nil)
	g := core.NewGoal(core.GoalConfig{Name: "real", Preconditions: []string{"done"}})

	a := newAction("a", nil, core.ConditionSet{"done": core.True}, 1, 5)
	domain := mustDomain(t, []core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewGoalFirst().PlanToGoal(context.Background(), start, domain, g, planning.Options{})
	if pl == nil || len(pl.Actions()) != 1 || pl.Actions()[0].Metadata().Name != "a" {
		t.Fatalf("expected 1-step plan via 'a', got %#v", pl)
	}
}

// --- helpers ---------------------------------------------------------------

func TestIsNirvana(t *testing.T) {
	if !utility.IsOpenEnded(core.NewGoal(core.GoalConfig{Name: utility.OpenEndedGoalName})) {
		t.Error("expected true for open-ended goal goal")
	}
	if utility.IsOpenEnded(core.NewGoal(core.GoalConfig{Name: "real"})) {
		t.Error("expected false for real goal")
	}
	if utility.IsOpenEnded(nil) {
		t.Error("expected false for nil goal")
	}
}

func TestPlanner_NameIsStable(t *testing.T) {
	if utility.NewPlanner().Name() != "utility" {
		t.Errorf("Planner.Name(): want utility, got %q", utility.NewPlanner().Name())
	}
	if utility.NewGoalFirst().Name() != "goal-first-utility" {
		t.Errorf("GoalFirst.Name(): want goal-first-utility, got %q", utility.NewGoalFirst().Name())
	}
}
