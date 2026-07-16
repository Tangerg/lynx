package planning_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/goap"
)

type pruneAction struct{ meta core.ActionMetadata }

func (a *pruneAction) Metadata() core.ActionMetadata { return a.meta }
func (a *pruneAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionFailed, nil
}

func newPruneAction(name string, pre, eff core.ConditionSet) core.Action {
	return &pruneAction{meta: core.ActionMetadata{
		Name:          name,
		Preconditions: pre,
		Effects:       eff,
		Cost:          core.FixedScore(1),
		Value:         core.FixedScore(1),
	}}
}

// TestPrune_DropsUnreachableActions wires up a small domain in
// which one action's preconditions are impossible to satisfy from
// the start state. The reachable action stays; the dead one is
// pruned.
func TestPrune_DropsUnreachableActions(t *testing.T) {
	reachable := newPruneAction("reachable", nil, core.ConditionSet{"done": core.True})
	// Dead: requires a precondition never produced by any action.
	dead := newPruneAction("dead",
		core.ConditionSet{"never_set": core.True},
		core.ConditionSet{"done": core.True},
	)

	goal := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"done"}, Value: core.FixedScore(1)})
	domain := planning.NewDomain(
		[]core.Action{reachable, dead},
		[]*core.Goal{goal},
		nil,
	)

	pruned, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.NewState(nil),
		domain,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned.Actions()) != 1 || pruned.Actions()[0].Metadata().Name != "reachable" {
		t.Fatalf("pruned actions = %v, want [reachable]", actionNames(pruned.Actions()))
	}
	// Goals + conditions are passed through.
	if len(pruned.Goals()) != 1 || pruned.Goals()[0] != goal {
		t.Errorf("goals should be passed through unchanged")
	}
}

// TestPrune_KeepsEveryActionWhenAllReferenced — the dual case:
// every action is on the plan path, so nothing is dropped.
func TestPrune_KeepsEveryActionWhenAllReferenced(t *testing.T) {
	a := newPruneAction("a", nil, core.ConditionSet{"step1": core.True})
	b := newPruneAction("b", core.ConditionSet{"step1": core.True}, core.ConditionSet{"done": core.True})

	goal := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"done"}, Value: core.FixedScore(1)})
	domain := planning.NewDomain(
		[]core.Action{a, b},
		[]*core.Goal{goal},
		nil,
	)

	pruned, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.NewState(nil),
		domain,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned.Actions()) != 2 {
		t.Fatalf("expected both actions kept, got %v", actionNames(pruned.Actions()))
	}
}

// TestPrune_NoReachableGoalDropsEverything — when no plan exists
// at all, every action is dead and the pruned domain has an empty
// Actions slice (but still a valid, non-nil Domain).
func TestPrune_NoReachableGoalDropsEverything(t *testing.T) {
	dead := newPruneAction("a",
		core.ConditionSet{"impossible": core.True},
		core.ConditionSet{"done": core.True},
	)
	goal := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"done"}, Value: core.FixedScore(1)})
	domain := planning.NewDomain([]core.Action{dead}, []*core.Goal{goal}, nil)

	pruned, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.NewState(nil),
		domain,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if pruned == nil {
		t.Fatal("pruned domain should not be nil even when no goal is reachable")
	}
	if len(pruned.Actions()) != 0 {
		t.Fatalf("pruned actions = %v, want empty", actionNames(pruned.Actions()))
	}
}

// TestPrune_DoesNotMutateInput — Prune is pure: the input domain's
// Actions slice must be untouched after the call.
func TestPrune_DoesNotMutateInput(t *testing.T) {
	live := newPruneAction("live", nil, core.ConditionSet{"done": core.True})
	dead := newPruneAction("dead",
		core.ConditionSet{"never": core.True},
		core.ConditionSet{"done": core.True},
	)
	goal := core.NewGoal(core.GoalConfig{Name: "g", Preconditions: []string{"done"}, Value: core.FixedScore(1)})
	domain := planning.NewDomain(
		[]core.Action{live, dead},
		[]*core.Goal{goal},
		nil,
	)
	originalCount := len(domain.Actions())

	_, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.NewState(nil),
		domain,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(domain.Actions()) != originalCount {
		t.Errorf("Prune mutated input: action count went %d → %d", originalCount, len(domain.Actions()))
	}
}

func TestPrune_NilSystemRejected(t *testing.T) {
	_, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.NewState(nil),
		nil,
		planning.Options{},
	)
	if err == nil {
		t.Fatal("expected error for nil domain")
	}
}

func actionNames(actions []core.Action) []string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Metadata().Name
	}
	return names
}
