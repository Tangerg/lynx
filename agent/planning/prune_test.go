package planning_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
	"github.com/Tangerg/lynx/agent/planning/planner/goap"
)

type pruneAction struct{ meta core.ActionMetadata }

func (a *pruneAction) Metadata() core.ActionMetadata { return a.meta }
func (a *pruneAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionFailed
}

func newPruneAction(name string, pre, eff core.Effects) core.Action {
	return &pruneAction{meta: core.ActionMetadata{
		Name:          name,
		Preconditions: pre,
		Effects:       eff,
		Cost:          core.Static(1),
		Value:         core.Static(1),
	}}
}

// TestPrune_DropsUnreachableActions wires up a small system in
// which one action's preconditions are impossible to satisfy from
// the start state. The reachable action stays; the dead one is
// pruned.
func TestPrune_DropsUnreachableActions(t *testing.T) {
	reachable := newPruneAction("reachable", nil, core.Effects{"done": core.True})
	// Dead: requires a precondition never produced by any action.
	dead := newPruneAction("dead",
		core.Effects{"never_set": core.True},
		core.Effects{"done": core.True},
	)

	goal := &core.Goal{Name: "g", Pre: []string{"done"}, Value: core.Static(1)}
	system := planning.NewSystem(
		[]core.Action{reachable, dead},
		[]*core.Goal{goal},
		nil,
	)

	pruned, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.EmptyWorldState(),
		system,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned.Actions) != 1 || pruned.Actions[0].Metadata().Name != "reachable" {
		t.Fatalf("pruned actions = %v, want [reachable]", actionNames(pruned.Actions))
	}
	// Goals + conditions are passed through.
	if len(pruned.Goals) != 1 || pruned.Goals[0] != goal {
		t.Errorf("goals should be passed through unchanged")
	}
}

// TestPrune_KeepsEveryActionWhenAllReferenced — the dual case:
// every action is on the plan path, so nothing is dropped.
func TestPrune_KeepsEveryActionWhenAllReferenced(t *testing.T) {
	a := newPruneAction("a", nil, core.Effects{"step1": core.True})
	b := newPruneAction("b", core.Effects{"step1": core.True}, core.Effects{"done": core.True})

	goal := &core.Goal{Name: "g", Pre: []string{"done"}, Value: core.Static(1)}
	system := planning.NewSystem(
		[]core.Action{a, b},
		[]*core.Goal{goal},
		nil,
	)

	pruned, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.EmptyWorldState(),
		system,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned.Actions) != 2 {
		t.Fatalf("expected both actions kept, got %v", actionNames(pruned.Actions))
	}
}

// TestPrune_NoReachableGoalDropsEverything — when no plan exists
// at all, every action is dead and the pruned system has an empty
// Actions slice (but still a valid, non-nil System).
func TestPrune_NoReachableGoalDropsEverything(t *testing.T) {
	dead := newPruneAction("a",
		core.Effects{"impossible": core.True},
		core.Effects{"done": core.True},
	)
	goal := &core.Goal{Name: "g", Pre: []string{"done"}, Value: core.Static(1)}
	system := planning.NewSystem([]core.Action{dead}, []*core.Goal{goal}, nil)

	pruned, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.EmptyWorldState(),
		system,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if pruned == nil {
		t.Fatal("pruned system should not be nil even when no goal is reachable")
	}
	if len(pruned.Actions) != 0 {
		t.Fatalf("pruned actions = %v, want empty", actionNames(pruned.Actions))
	}
}

// TestPrune_DoesNotMutateInput — Prune is pure: the input system's
// Actions slice must be untouched after the call.
func TestPrune_DoesNotMutateInput(t *testing.T) {
	live := newPruneAction("live", nil, core.Effects{"done": core.True})
	dead := newPruneAction("dead",
		core.Effects{"never": core.True},
		core.Effects{"done": core.True},
	)
	goal := &core.Goal{Name: "g", Pre: []string{"done"}, Value: core.Static(1)}
	system := planning.NewSystem(
		[]core.Action{live, dead},
		[]*core.Goal{goal},
		nil,
	)
	originalCount := len(system.Actions)

	_, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.EmptyWorldState(),
		system,
		planning.Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(system.Actions) != originalCount {
		t.Errorf("Prune mutated input: action count went %d → %d", originalCount, len(system.Actions))
	}
}

func TestPrune_NilSystemRejected(t *testing.T) {
	_, err := planning.Prune(
		context.Background(),
		goap.NewPlanner(),
		planning.EmptyWorldState(),
		nil,
		planning.Options{},
	)
	if err == nil {
		t.Fatal("expected error for nil system")
	}
}

func actionNames(actions []core.Action) []string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Metadata().Name
	}
	return names
}
