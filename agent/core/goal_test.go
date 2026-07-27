package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
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

	if goal.Name() != "done" || goal.Description() != "finish" || goal.Value(nil) != 3 {
		t.Fatalf("goal scalar behavior drifted: %q %q %v", goal.Name(), goal.Description(), goal.Value(nil))
	}
	if goal.RequiredConditions()[0] != "ready" || goal.Inputs()[0].Name != "input" {
		t.Fatal("Goal leaked caller or accessor slice storage")
	}
}
