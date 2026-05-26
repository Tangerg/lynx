package utility_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/planner/utility"
)

// fakeAction satisfies core.Action for planner-only tests.
type fakeAction struct{ meta core.ActionMetadata }

func (a *fakeAction) Metadata() core.ActionMetadata { return a.meta }
func (a *fakeAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionFailed
}

// newAction builds a fakeAction with the given pre / eff / cost / value.
func newAction(name string, pre, eff core.Effects, cost, value float64) core.Action {
	return &fakeAction{meta: core.ActionMetadata{
		Name:          name,
		Preconditions: pre,
		Effects:       eff,
		Cost:          core.Static(cost),
		Value:         core.Static(value),
	}}
}

// --- Planner (classic Utility) ----------------------------------------------

func TestUtility_NirvanaPicksHighestNetValue(t *testing.T) {
	start := planning.EmptyWorldState()
	g := &core.Goal{Name: utility.NirvanaGoalName}

	low := newAction("low", nil, core.Effects{"a": core.True}, 1, 2)    // net = 1
	high := newAction("high", nil, core.Effects{"a": core.True}, 1, 10) // net = 9

	system := planning.NewSystem([]core.Action{low, high}, []*core.Goal{g}, nil)
	pl, err := utility.NewPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if pl == nil || len(pl.Actions) != 1 || pl.Actions[0].Metadata().Name != "high" {
		t.Fatalf("expected 'high' picked, got %#v", pl)
	}
}

func TestUtility_NirvanaWithNoActionsReturnsNil(t *testing.T) {
	g := &core.Goal{Name: utility.NirvanaGoalName}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)

	pl, err := utility.NewPlanner().PlanToGoal(context.Background(), planning.EmptyWorldState(), system, g, planning.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if pl != nil {
		t.Errorf("expected nil plan for Nirvana with no actions, got %#v", pl)
	}
}

func TestUtility_AlreadySatisfiedNoActions(t *testing.T) {
	start := planning.NewConditionWorldState(map[string]core.Determination{"goalKey": core.True})
	g := &core.Goal{Name: "real", Pre: []string{"goalKey"}}
	system := planning.NewSystem(nil, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if pl == nil || len(pl.Actions) != 0 {
		t.Errorf("expected empty plan when already satisfied + no actions, got %#v", pl)
	}
}

func TestUtility_OneStepLookaheadEmitsPlan(t *testing.T) {
	start := planning.EmptyWorldState()
	g := &core.Goal{Name: "real", Pre: []string{"done"}}

	a := newAction("a", nil, core.Effects{"done": core.True}, 1, 5)
	system := planning.NewSystem([]core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if pl == nil || len(pl.Actions) != 1 || pl.Actions[0].Metadata().Name != "a" {
		t.Fatalf("expected 1-step plan via 'a', got %#v", pl)
	}
}

func TestUtility_OneStepLookaheadInsufficientReturnsNil(t *testing.T) {
	start := planning.EmptyWorldState()
	g := &core.Goal{Name: "real", Pre: []string{"step1", "step2"}}

	a := newAction("a", nil, core.Effects{"step1": core.True}, 1, 5)
	system := planning.NewSystem([]core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if pl != nil {
		t.Errorf("Utility refuses multi-step plans for real goals; got %#v", pl)
	}
}

func TestUtility_ExcludedActionsSkipped(t *testing.T) {
	start := planning.EmptyWorldState()
	g := &core.Goal{Name: "real", Pre: []string{"done"}}
	a := newAction("a", nil, core.Effects{"done": core.True}, 1, 5)
	system := planning.NewSystem([]core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, system, g,
		planning.Options{ExcludedActions: map[string]struct{}{"a": {}}})
	if pl != nil {
		t.Errorf("excluded action should not be picked; got %#v", pl)
	}
}

// --- HybridPlanner ----------------------------------------------------------

func TestHybridUtility_SatisfiedFirstShortCircuit(t *testing.T) {
	// Goal already satisfied AND a high-value action is still
	// applicable. Hybrid returns empty plan; classic Utility would
	// pick the action.
	start := planning.NewConditionWorldState(map[string]core.Determination{"goalKey": core.True})
	g := &core.Goal{Name: "real", Pre: []string{"goalKey"}}

	stillRunnable := newAction("noop", nil, core.Effects{"other": core.True}, 1, 99)
	system := planning.NewSystem([]core.Action{stillRunnable}, []*core.Goal{g}, nil)

	// Hybrid: empty planning.
	plH, _ := utility.NewHybridPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if plH == nil || len(plH.Actions) != 0 {
		t.Errorf("hybrid: want empty plan, got %#v", plH)
	}

	// Classic: picks the action.
	plU, _ := utility.NewPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if plU == nil || len(plU.Actions) != 1 {
		t.Errorf("classic: want 1-step plan, got %#v", plU)
	}
}

func TestHybridUtility_NirvanaSemanticsMatchClassic(t *testing.T) {
	start := planning.EmptyWorldState()
	g := &core.Goal{Name: utility.NirvanaGoalName}

	a := newAction("a", nil, core.Effects{"x": core.True}, 1, 5)
	system := planning.NewSystem([]core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewHybridPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if pl == nil || len(pl.Actions) != 1 {
		t.Fatalf("hybrid Nirvana: want 1-step plan, got %#v", pl)
	}
}

func TestHybridUtility_OneStepReachesGoal(t *testing.T) {
	start := planning.EmptyWorldState()
	g := &core.Goal{Name: "real", Pre: []string{"done"}}

	a := newAction("a", nil, core.Effects{"done": core.True}, 1, 5)
	system := planning.NewSystem([]core.Action{a}, []*core.Goal{g}, nil)

	pl, _ := utility.NewHybridPlanner().PlanToGoal(context.Background(), start, system, g, planning.Options{})
	if pl == nil || len(pl.Actions) != 1 || pl.Actions[0].Metadata().Name != "a" {
		t.Fatalf("expected 1-step plan via 'a', got %#v", pl)
	}
}

// --- helpers ---------------------------------------------------------------

func TestIsNirvana(t *testing.T) {
	if !utility.IsNirvana(&core.Goal{Name: utility.NirvanaGoalName}) {
		t.Error("expected true for Nirvana goal")
	}
	if utility.IsNirvana(&core.Goal{Name: "real"}) {
		t.Error("expected false for real goal")
	}
	if utility.IsNirvana(nil) {
		t.Error("expected false for nil goal")
	}
}

func TestPlanner_NameIsStable(t *testing.T) {
	if utility.NewPlanner().Name() != "utility" {
		t.Errorf("Planner.Name(): want utility, got %q", utility.NewPlanner().Name())
	}
	if utility.NewHybridPlanner().Name() != "hybrid-utility" {
		t.Errorf("HybridPlanner.Name(): want hybrid-utility, got %q", utility.NewHybridPlanner().Name())
	}
}
