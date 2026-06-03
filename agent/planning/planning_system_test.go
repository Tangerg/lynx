package planning_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

type psIn struct{ X int }
type psOut struct{ Y int }

func psAgent(name, actionName string) *core.Agent {
	return agent.New(name).
		Actions(agent.NewAction(actionName,
			func(ctx context.Context, pc *core.ProcessContext, in psIn) (psOut, error) {
				return psOut{Y: in.X + 1}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[psOut](core.Goal{Description: name + " goal"})).
		Build()
}

func TestFromAgents_UnionsCapabilities(t *testing.T) {
	a := psAgent("alpha", "alpha:step")
	b := psAgent("beta", "beta:step")

	system := planning.FromAgents([]*core.Agent{a, b})
	if len(system.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(system.Actions))
	}
	if len(system.Goals) != 2 {
		t.Fatalf("goals = %d, want 2", len(system.Goals))
	}
}

func TestFromAgents_NilEntriesSkipped(t *testing.T) {
	system := planning.FromAgents([]*core.Agent{nil, psAgent("only", "only:step"), nil})
	if len(system.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(system.Actions))
	}
}

func TestFromAgents_EmptyInputProducesEmptySystem(t *testing.T) {
	system := planning.FromAgents(nil)
	if system == nil {
		t.Fatal("system is nil")
	}
	if len(system.Actions) != 0 || len(system.Goals) != 0 {
		t.Fatalf("non-empty system: actions=%d goals=%d", len(system.Actions), len(system.Goals))
	}
}
