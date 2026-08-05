package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

func TestGoalOwnsConfigurationCollections(t *testing.T) {
	preconditions := []string{"ready"}
	inputs := []core.Binding{{Name: "input", Type: "example.Input"}}
	goal := core.NewGoal(core.GoalConfig{
		Name: "done", Description: "finish", Preconditions: preconditions, Inputs: inputs,
		Value: core.FixedScore(3),
	})

	preconditions[0] = "mutated"
	inputs[0].Name = "mutated"

	returnedPreconditions := goal.RequiredConditions()
	returnedInputs := goal.Inputs()
	returnedPreconditions[0] = "leaked"
	returnedInputs[0].Name = "leaked"
	descriptor := goal.Descriptor()
	descriptorConditions := descriptor.RequiredConditions()
	descriptorInputs := descriptor.Inputs()
	descriptorConditions[0] = "descriptor-leaked"
	descriptorInputs[0].Name = "descriptor-leaked"

	if goal.Name() != "done" || goal.Description() != "finish" || goal.Value(nil) != 3 {
		t.Fatalf("goal scalar behavior drifted: %q %q %v", goal.Name(), goal.Description(), goal.Value(nil))
	}
	if goal.RequiredConditions()[0] != "ready" || goal.Inputs()[0].Name != "input" {
		t.Fatal("Goal leaked caller or accessor slice storage")
	}
	if descriptor.Name() != "done" || descriptor.Description() != "finish" ||
		descriptor.RequiredConditions()[0] != "ready" || descriptor.Inputs()[0].Name != "input" {
		t.Fatal("GoalDescriptor leaked accessor slice storage")
	}
}

func TestGoalRejectsTypedNilWorldState(t *testing.T) {
	goal := core.NewGoal(core.GoalConfig{Name: "done"})
	var state *planning.State
	if goal.SatisfiedBy(state) {
		t.Fatal("goal was satisfied by a typed nil world state")
	}
}
