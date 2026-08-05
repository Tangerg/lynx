package core_test

import (
	"context"
	"errors"
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

type snapshotStateSample struct{ Value string }

type pointerStuckPolicy struct{}

func (*pointerStuckPolicy) Recover(context.Context, core.ProcessView, core.BlackboardWriter) (core.StuckResult, error) {
	return core.StuckResult{}, nil
}

type metadataAction struct {
	metadata core.ActionMetadata
	calls    *int
	cause    error
}

func (a metadataAction) Metadata() core.ActionMetadata {
	if a.calls != nil {
		*a.calls++
	}
	if a.cause != nil {
		panic(a.cause)
	}
	return a.metadata
}

func (metadataAction) Execute(context.Context, *core.ProcessContext) (core.ActionStatus, error) {
	return core.ActionSucceeded, nil
}

type metadataCondition struct {
	name      string
	cost      float64
	nameCalls *int
	costCalls *int
	namePanic error
	costPanic error
}

func (c metadataCondition) Name() string {
	if c.nameCalls != nil {
		*c.nameCalls++
	}
	if c.namePanic != nil {
		panic(c.namePanic)
	}
	return c.name
}

func (c metadataCondition) Cost() float64 {
	if c.costCalls != nil {
		*c.costCalls++
	}
	if c.costPanic != nil {
		panic(c.costPanic)
	}
	return c.cost
}

func (metadataCondition) Evaluate(context.Context, *core.ConditionEnv) core.Truth {
	return core.Unknown
}

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

func TestValidateRejectsTypedNilCapabilitiesWithoutPanicking(t *testing.T) {
	var action *fakeAction
	var condition *fakeCondition
	var stuckPolicy *pointerStuckPolicy
	agent := core.NewAgent(core.AgentConfig{
		Name:        "typed-nil",
		StuckPolicy: stuckPolicy,
		Actions:     []core.Action{action},
		Goals:       []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
		Conditions:  []core.Condition{condition},
	})

	_ = agent.Descriptor()
	err := agent.Validate()
	for _, want := range []string{"action at index 0 is nil", "condition at index 0 is nil", "stuck policy is typed nil"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate error = %v, want %q", err, want)
		}
	}
}

func TestValidateContainsMetadataPanics(t *testing.T) {
	for _, test := range []struct {
		name       string
		action     core.Action
		conditions []core.Condition
		contains   string
		cause      error
	}{
		{
			name: "action metadata", cause: errors.New("action metadata panic"), contains: "Metadata panicked",
		},
		{
			name: "condition name", action: fakeAction{meta: core.ActionMetadata{Name: "act"}},
			cause: errors.New("condition name panic"), contains: "Name panicked",
		},
		{
			name: "condition cost", action: fakeAction{meta: core.ActionMetadata{Name: "act"}},
			cause: errors.New("condition cost panic"), contains: "Cost panicked",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			action := test.action
			conditions := test.conditions
			switch test.name {
			case "action metadata":
				action = metadataAction{cause: test.cause}
			case "condition name":
				conditions = []core.Condition{metadataCondition{namePanic: test.cause}}
			case "condition cost":
				conditions = []core.Condition{metadataCondition{name: "ready", costPanic: test.cause}}
			}
			definition := core.NewAgent(core.AgentConfig{
				Name: "metadata-panic", Actions: []core.Action{action},
				Goals: []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})}, Conditions: conditions,
			})
			_ = definition.Descriptor()
			err := definition.Validate()
			if !errors.Is(err, test.cause) || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Validate error = %v, want attributed metadata panic", err)
			}
		})
	}
}

func TestValidateReadsCapabilityMetadataOnce(t *testing.T) {
	var actionCalls, conditionNameCalls, conditionCostCalls int
	definition := core.NewAgent(core.AgentConfig{
		Name: "metadata-snapshot",
		Actions: []core.Action{metadataAction{
			metadata: core.ActionMetadata{Name: "act"}, calls: &actionCalls,
		}},
		Goals: []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
		Conditions: []core.Condition{metadataCondition{
			name: "ready", nameCalls: &conditionNameCalls, costCalls: &conditionCostCalls,
		}},
	})
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	if actionCalls != 1 || conditionNameCalls != 1 || conditionCostCalls != 1 {
		t.Fatalf("metadata calls = action %d, condition name/cost %d/%d; want one each", actionCalls, conditionNameCalls, conditionCostCalls)
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

	base.Conditions = []core.Condition{core.NewCondition(core.ConditionConfig{})}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "condition at index 0 has empty name") {
		t.Fatalf("empty condition error = %v", err)
	}

	base.Conditions = []core.Condition{
		core.NewCondition(core.ConditionConfig{Name: "ready"}),
		core.NewCondition(core.ConditionConfig{Name: "ready"}),
	}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate condition name") {
		t.Fatalf("duplicate condition error = %v", err)
	}
}

func TestValidateRejectsInvalidSnapshotState(t *testing.T) {
	base := core.AgentConfig{
		Name:    "snapshot-state",
		Actions: []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	}
	base.SnapshotState = []core.Binding{{Name: "state", Type: "example.State"}}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "must be constructed with NewBinding") {
		t.Fatalf("literal snapshot state error = %v", err)
	}
	binding := core.NewBinding[snapshotStateSample]("state")
	base.SnapshotState = []core.Binding{binding, binding}
	if err := core.NewAgent(base).Validate(); err == nil || !strings.Contains(err.Error(), "duplicate snapshot state") {
		t.Fatalf("duplicate snapshot state error = %v", err)
	}
}

func TestValidateRejectsInvalidToolGroupRole(t *testing.T) {
	for _, test := range []struct {
		name  string
		roles []string
		want  string
	}{
		{name: "whitespace", roles: []string{" research "}, want: "role has surrounding whitespace"},
		{name: "duplicate", roles: []string{"research", "research"}, want: "duplicate role \"research\""},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := core.AgentConfig{
				Name: "tool-policy",
				Actions: []core.Action{fakeAction{meta: core.ActionMetadata{
					Name:       "act",
					ToolGroups: test.roles,
				}}},
				Goals: []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
			}
			err := core.NewAgent(config).Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid tool group error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsWhitespacePlannerName(t *testing.T) {
	definition := core.NewAgent(core.AgentConfig{
		Name:        "planner-name",
		PlannerName: " goap ",
		Actions:     []core.Action{fakeAction{meta: core.ActionMetadata{Name: "act"}}},
		Goals:       []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	})
	if err := definition.Validate(); err == nil || !strings.Contains(err.Error(), "planner name \" goap \" has surrounding whitespace") {
		t.Fatalf("Validate error = %v, want planner-name whitespace error", err)
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
	condition := core.NewCondition(core.ConditionConfig{Name: "ready"})
	actions := []core.Action{action}
	goals := []*core.Goal{goal}
	conditions := []core.Condition{condition}
	snapshotState := []core.Binding{core.NewBinding[snapshotStateSample]("state")}
	config := core.AgentConfig{
		Name:          "owned",
		Description:   "original",
		Version:       "1.2.3",
		Actions:       actions,
		Goals:         goals,
		Conditions:    conditions,
		SnapshotState: snapshotState,
	}

	agent := core.NewAgent(config)
	config.Description = "mutated"
	config.Version = "9.9.9"
	actions[0] = nil
	goals[0] = nil
	conditions[0] = nil
	snapshotState[0].Name = "mutated"

	returnedActions := agent.Actions()
	returnedGoals := agent.Goals()
	returnedConditions := agent.Conditions()
	returnedState := agent.SnapshotState()
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
	if state := agent.SnapshotState(); len(state) != 1 || state[0].Name != "state" {
		t.Fatalf("Agent leaked snapshot state storage: %#v", state)
	}
}

func TestAgentDescriptorOwnsItsNonExecutableProjection(t *testing.T) {
	action := fakeAction{meta: core.ActionMetadata{
		Name:          "act",
		Description:   "perform work",
		Inputs:        []core.Binding{core.NewBinding[snapshotStateSample]("input")},
		Preconditions: core.ConditionSet{"ready": core.True},
		Cost: func(core.WorldState) float64 {
			t.Fatal("descriptor invoked action scoring policy")
			return 0
		},
	}}
	agent := core.NewAgent(core.AgentConfig{
		Name:          "described",
		Description:   "description",
		Version:       "1.2.3",
		Actions:       []core.Action{action},
		Goals:         []*core.Goal{core.NewGoal(core.GoalConfig{Name: "done", Preconditions: []string{"ready"}})},
		Conditions:    []core.Condition{fakeCondition{name: "ready", cost: 2}},
		SnapshotState: []core.Binding{core.NewBinding[snapshotStateSample]("state")},
		PlannerName:   "goap",
	})

	descriptor := agent.Descriptor()
	actions := descriptor.Actions()
	actionInputs := actions[0].Inputs()
	actionPreconditions := actions[0].Preconditions()
	goals := descriptor.Goals()
	goalConditions := goals[0].RequiredConditions()
	conditions := descriptor.Conditions()
	state := descriptor.SnapshotState()
	actions[0] = core.ActionDescriptor{}
	actionInputs[0].Name = "leaked"
	actionPreconditions["ready"] = core.False
	goals[0] = core.GoalDescriptor{}
	goalConditions[0] = "leaked"
	conditions[0] = core.ConditionDescriptor{}
	state[0].Name = "leaked"

	if descriptor.Name() != "described" ||
		descriptor.Description() != "description" ||
		descriptor.Version() != "1.2.3" ||
		descriptor.PlannerName() != "goap" {
		t.Fatal("AgentDescriptor lost scalar definition data")
	}
	if descriptor.Actions()[0].Name() != "act" ||
		descriptor.Actions()[0].Inputs()[0].Name != "input" ||
		descriptor.Actions()[0].Preconditions()["ready"] != core.True ||
		descriptor.Goals()[0].Name() != "done" ||
		descriptor.Goals()[0].RequiredConditions()[0] != "ready" ||
		descriptor.Conditions()[0].Name() != "ready" ||
		descriptor.Conditions()[0].Cost() != 2 ||
		descriptor.SnapshotState()[0].Name != "state" {
		t.Fatal("AgentDescriptor leaked accessor storage")
	}
}
