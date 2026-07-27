package core_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

func TestGoalOwnsConfigurationCollections(t *testing.T) {
	preconditions := []string{"ready"}
	inputs := []core.Binding{{Name: "input", Type: "example.Input"}}
	tags := []string{"routing"}
	examples := []string{"handle this"}
	goal := core.NewGoal(core.GoalConfig{
		Name: "done", Description: "finish", Preconditions: preconditions, Inputs: inputs,
		Value: core.FixedScore(3), Tags: tags, Examples: examples,
	})

	preconditions[0] = "mutated"
	inputs[0].Name = "mutated"
	tags[0] = "mutated"
	examples[0] = "mutated"

	returnedPreconditions := goal.RequiredConditions()
	returnedInputs := goal.Inputs()
	returnedTags := goal.Tags()
	returnedExamples := goal.Examples()
	returnedPreconditions[0] = "leaked"
	returnedInputs[0].Name = "leaked"
	returnedTags[0] = "leaked"
	returnedExamples[0] = "leaked"

	if goal.Name() != "done" || goal.Description() != "finish" || goal.Value(nil) != 3 {
		t.Fatalf("goal scalar behavior drifted: %q %q %v", goal.Name(), goal.Description(), goal.Value(nil))
	}
	if goal.RequiredConditions()[0] != "ready" || goal.Inputs()[0].Name != "input" ||
		goal.Tags()[0] != "routing" || goal.Examples()[0] != "handle this" {
		t.Fatal("Goal leaked caller or accessor slice storage")
	}
}
