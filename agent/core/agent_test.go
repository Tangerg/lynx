package core_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type fakeAction struct {
	meta core.ActionMetadata
}

func (f fakeAction) Metadata() core.ActionMetadata { return f.meta }
func (f fakeAction) Execute(context.Context, *core.ProcessContext) core.ActionStatus {
	return core.ActionSucceeded
}

func TestValidateAgentRejectsNilAction(t *testing.T) {
	a := core.NewAgent(core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{nil},
		Goals:   []*core.Goal{{Name: "goal"}},
	})

	err := core.ValidateAgent(a)
	if err == nil || !strings.Contains(err.Error(), "action at index 0 is nil") {
		t.Fatalf("ValidateAgent error = %v, want nil action index", err)
	}
}

func TestValidateAgentRejectsNilGoalWithIndex(t *testing.T) {
	a := core.NewAgent(core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:   []*core.Goal{nil},
	})

	err := core.ValidateAgent(a)
	if err == nil || !strings.Contains(err.Error(), "goal at index 0 is nil") {
		t.Fatalf("ValidateAgent error = %v, want nil goal index", err)
	}
}

func TestValidateAgentRejectsInvalidConditions(t *testing.T) {
	base := core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:   []*core.Goal{{Name: "goal"}},
	}

	base.Conditions = []core.Condition{nil}
	if err := core.ValidateAgent(core.NewAgent(base)); err == nil || !strings.Contains(err.Error(), "condition at index 0 is nil") {
		t.Fatalf("nil condition error = %v", err)
	}

	base.Conditions = []core.Condition{core.NewCondition("", nil)}
	if err := core.ValidateAgent(core.NewAgent(base)); err == nil || !strings.Contains(err.Error(), "condition at index 0 has empty name") {
		t.Fatalf("empty condition error = %v", err)
	}

	base.Conditions = []core.Condition{core.NewCondition("ready", nil), core.NewCondition("ready", nil)}
	if err := core.ValidateAgent(core.NewAgent(base)); err == nil || !strings.Contains(err.Error(), "duplicate condition name") {
		t.Fatalf("duplicate condition error = %v", err)
	}
}

func TestKnownConditionsSkipsNilEntries(t *testing.T) {
	conditions := core.KnownConditions(
		[]core.Action{nil, fakeAction{meta: core.ActionMetadata{
			Name:          "act",
			Preconditions: core.Effects{"need": core.True},
			Effects:       core.Effects{"have": core.True},
		}}},
		[]*core.Goal{nil, {Name: "goal", Pre: []string{"done"}}},
		[]core.Condition{nil},
	)

	for _, key := range []string{"need", "have", "done"} {
		if _, ok := conditions[key]; !ok {
			t.Fatalf("missing condition %q in %#v", key, conditions)
		}
	}
}
