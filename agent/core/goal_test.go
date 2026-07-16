package core_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type goalInputSample struct {
	Topic string
	Tags  []string
}

func TestGoalOwnsConfigurationCollections(t *testing.T) {
	preconditions := []string{"ready"}
	inputs := []core.Binding{{Name: "input", Type: "example.Input"}}
	tags := []string{"routing"}
	examples := []string{"handle this"}
	tool := core.NewGoalTool[goalInputSample](core.GoalToolConfig{
		Standalone:  true,
		Description: "public description",
	})
	goal := core.NewGoal(core.GoalConfig{
		Name: "done", Description: "finish", Preconditions: preconditions, Inputs: inputs,
		Value: core.FixedScore(3), Tags: tags, Examples: examples, Tool: tool,
	})

	preconditions[0] = "mutated"
	inputs[0].Name = "mutated"
	tags[0] = "mutated"
	examples[0] = "mutated"
	tool.Description = "mutated"

	returnedPreconditions := goal.RequiredConditions()
	returnedInputs := goal.Inputs()
	returnedTags := goal.Tags()
	returnedExamples := goal.Examples()
	returnedTool := goal.Tool()
	returnedPreconditions[0] = "leaked"
	returnedInputs[0].Name = "leaked"
	returnedTags[0] = "leaked"
	returnedExamples[0] = "leaked"
	returnedTool.Description = "leaked"

	if goal.Name() != "done" || goal.Description() != "finish" || goal.Value(nil) != 3 {
		t.Fatalf("goal scalar behavior drifted: %q %q %v", goal.Name(), goal.Description(), goal.Value(nil))
	}
	if goal.RequiredConditions()[0] != "ready" || goal.Inputs()[0].Name != "input" ||
		goal.Tags()[0] != "routing" || goal.Examples()[0] != "handle this" {
		t.Fatal("Goal leaked caller or accessor slice storage")
	}
	frozenTool := goal.Tool()
	if frozenTool.Description != "public description" {
		t.Fatalf("Goal tool leaked mutation: %#v", frozenTool)
	}
	if frozenTool.InputType() != reflect.TypeFor[goalInputSample]() {
		t.Fatalf("Goal input type = %v, want %v", frozenTool.InputType(), reflect.TypeFor[goalInputSample]())
	}
}
