package core_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type fakeAction struct {
	meta core.ActionMetadata
}

type fakeCondition struct {
	name string
	cost float64
}

type durableStateSample struct{ Value string }

func (c fakeCondition) Name() string                                          { return c.name }
func (c fakeCondition) Cost() float64                                         { return c.cost }
func (fakeCondition) Evaluate(context.Context, *core.ConditionEnv) core.Truth { return core.Unknown }

func (f fakeAction) Metadata() core.ActionMetadata { return f.meta }
func (f fakeAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

func TestValidateRejectsNilAction(t *testing.T) {
	a := core.NewAgent(core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{nil},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	})

	err := a.Validate()
	if err == nil || !strings.Contains(err.Error(), "action at index 0 is nil") {
		t.Fatalf("Validate error = %v, want nil action index", err)
	}
}

func TestValidateRejectsNilGoalWithIndex(t *testing.T) {
	a := core.NewAgent(core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:   []*core.Goal{nil},
	})

	err := a.Validate()
	if err == nil || !strings.Contains(err.Error(), "goal at index 0 is nil") {
		t.Fatalf("Validate error = %v, want nil goal index", err)
	}
}

func TestValidateRejectsInvalidConditions(t *testing.T) {
	base := core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	}

	base.Conditions = []core.Condition{nil}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "condition at index 0 is nil") {
		t.Fatalf("nil condition error = %v", err)
	}

	base.Conditions = []core.Condition{core.NewCondition("", nil)}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "condition at index 0 has empty name") {
		t.Fatalf("empty condition error = %v", err)
	}

	base.Conditions = []core.Condition{core.NewCondition("ready", nil), core.NewCondition("ready", nil)}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate condition name") {
		t.Fatalf("duplicate condition error = %v", err)
	}
}

func TestValidateRejectsInvalidDurableState(t *testing.T) {
	base := core.AgentConfig{
		Name:    "durable-state",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	}
	base.DurableState = []core.Binding{{Name: "state", Type: "example.State"}}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "must be constructed with NewBinding") {
		t.Fatalf("literal durable state error = %v", err)
	}
	binding := core.NewBinding[durableStateSample]("state")
	base.DurableState = []core.Binding{binding, binding}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate durable state") {
		t.Fatalf("duplicate durable state error = %v", err)
	}
}

func TestValidateRejectsInvalidToolGroupRequirement(t *testing.T) {
	config := core.AgentConfig{
		Name: "tool-policy",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{
			Name:       "act",
			ToolGroups: []core.ToolGroupRequirement{{Role: " research "}},
		}}},
		Goals: []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	}
	err := core.NewAgent(config).Validate()
	if err == nil || !strings.Contains(err.Error(), "role has surrounding whitespace") {
		t.Fatalf("invalid tool group error = %v", err)
	}
}

func TestValidateRejectsMalformedDefinitionIdentity(t *testing.T) {
	action := fakeAction{meta: core.ActionMetadata{
		Name: "act",
		Inputs: []core.Binding{{
			Name: "request:raw",
			Type: "example.Request",
		}},
		Preconditions: core.ConditionSet{" ready ": core.True},
		Effects:       core.ConditionSet{"done": core.Truth(9)},
	}}
	agent := core.NewAgent(core.AgentConfig{
		Name:    " malformed ",
		Actions: []core.Action{action},
		Goals: []*core.Goal{core.NewGoal(core.GoalConfig{
			Name:          "done",
			Preconditions: []string{" done "},
		})},
	})
	err := agent.Validate()
	if err == nil {
		t.Fatal("Validate accepted malformed identities")
	}
	for _, want := range []string{"name \" malformed \"", "contains ':'", "condition key \" ready \"", "invalid truth value 9", "condition key \" done \""} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error %q does not contain %q", err, want)
		}
	}
}

func TestValidateRejectsNonCanonicalVersion(t *testing.T) {
	for _, version := range []string{"v1.2.3", "1.2", " 1.2.3"} {
		agent := core.NewAgent(core.AgentConfig{
			Name:    "versioned",
			Version: version,
			Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
			Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
		})
		if err := agent.Validate(); err == nil || !strings.Contains(err.Error(), "version") {
			t.Errorf("Validate version %q = %v, want strict semantic-version error", version, err)
		}
	}
}

func TestValidateRejectsInvalidConditionCost(t *testing.T) {
	for _, cost := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		agent := core.NewAgent(core.AgentConfig{
			Name:       "condition-cost",
			Actions:    []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
			Goals:      []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
			Conditions: []core.Condition{fakeCondition{name: "ready", cost: cost}},
		})
		if err := agent.Validate(); err == nil || !strings.Contains(err.Error(), "must be finite and non-negative") {
			t.Errorf("Validate condition cost %v = %v", cost, err)
		}
	}
}

func TestAgentOwnsConfigurationCollections(t *testing.T) {
	action := fakeAction{meta: core.ActionMetadata{Name: "act"}}
	goal := core.NewGoal(core.GoalConfig{Name: "goal"})
	condition := core.NewCondition("ready", nil)
	actions := []core.Action{action}
	goals := []*core.Goal{goal}
	conditions := []core.Condition{condition}
	durableState := []core.Binding{core.NewBinding[durableStateSample]("state")}
	config := core.AgentConfig{
		Name:         "owned",
		Description:  "original",
		Version:      "1.2.3",
		Actions:      actions,
		Goals:        goals,
		Conditions:   conditions,
		DurableState: durableState,
	}

	agent := core.NewAgent(config)
	config.Description = "mutated"
	config.Version = "9.9.9"
	actions[0] = nil
	goals[0] = nil
	conditions[0] = nil
	durableState[0].Name = "mutated"

	returnedActions := agent.Actions()
	returnedGoals := agent.Goals()
	returnedConditions := agent.Conditions()
	returnedState := agent.DurableState()
	returnedActions[0] = nil
	returnedGoals[0] = nil
	returnedConditions[0] = nil
	returnedState[0].Name = "mutated-again"

	if agent.Description() != "original" || agent.Version() != "1.2.3" {
		t.Fatalf("scalar config leaked: description=%q version=%q", agent.Description(), agent.Version())
	}
	if agent.Actions()[0] == nil || agent.Goals()[0] != goal || agent.Conditions()[0] != condition {
		t.Fatal("Agent leaked caller or accessor slice storage")
	}
	if state := agent.DurableState(); len(state) != 1 || state[0].Name != "state" {
		t.Fatalf("Agent leaked durable state storage: %#v", state)
	}
}
