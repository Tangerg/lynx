package planning_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type nvIn struct{ X int }
type nvOut struct{ Y int }

// TestNetValueIncludesActionsValue locks in the embabel ranking heuristic
// (goal.value + actionsValue − cost). The actions-value term was previously
// dropped, so a plan whose actions carry independent value ranked the same
// as a valueless plan with equal goal value and cost.
func TestNetValueIncludesActionsValue(t *testing.T) {
	mk := func(name string, value, cost float64) core.Action {
		return agent.NewAction(name,
			func(ctx context.Context, pc *core.ProcessContext, in nvIn) (nvOut, error) {
				return nvOut{}, nil
			},
			core.ActionConfig{Value: core.Static(value), Cost: core.Static(cost)},
		)
	}

	plan := &planning.Plan{
		Actions: []core.Action{mk("a1", 3, 2), mk("a2", 4, 1)},
		Goal:    &core.Goal{Description: "g", Value: core.Static(10)},
	}

	// Static cost/value ignore the world state, so nil is a valid sample.
	if got := plan.ActionsValue(nil); got != 7 {
		t.Fatalf("ActionsValue = %v, want 7 (3 + 4)", got)
	}
	if got := plan.Cost(nil); got != 3 {
		t.Fatalf("Cost = %v, want 3 (2 + 1)", got)
	}
	// goal value (10) + actionsValue (7) − cost (3) = 14
	if got := plan.NetValue(nil); got != 14 {
		t.Fatalf("NetValue = %v, want 14 (goal 10 + actions 7 − cost 3)", got)
	}
}

// TestNetValueZeroActionsValueByDefault confirms the common case is
// unchanged: actions leave Value at Static(0), so NetValue still reduces to
// goal value − cost.
func TestNetValueZeroActionsValueByDefault(t *testing.T) {
	a := agent.NewAction("a",
		func(ctx context.Context, pc *core.ProcessContext, in nvIn) (nvOut, error) {
			return nvOut{}, nil
		},
		core.ActionConfig{Cost: core.Static(2)}, // Value defaults to Static(0)
	)
	plan := &planning.Plan{
		Actions: []core.Action{a},
		Goal:    &core.Goal{Description: "g", Value: core.Static(5)},
	}

	if got := plan.ActionsValue(nil); got != 0 {
		t.Fatalf("ActionsValue = %v, want 0", got)
	}
	if got := plan.NetValue(nil); got != 3 {
		t.Fatalf("NetValue = %v, want 3 (goal 5 − cost 2)", got)
	}
}
