package workflow

import (
	"encoding/json"
	"errors"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
)

type stateFixture struct {
	Value int `json:"value"`
}

func TestRestoreRejectsUnknownAndContradictoryState(t *testing.T) {
	stage, err := Transform("identity", func(input stateFixture) (stateFixture, error) { return input, nil })
	if err != nil {
		t.Fatal(err)
	}
	definition, err := NewDefinition(DefinitionConfig{
		Name: "test.workflow.state", Description: "Validate Workflow state restoration.",
		Version: "1.0.0", Stages: []Stage{stage},
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string]json.RawMessage{
		"unknown field":      json.RawMessage(`{"phase":"ready","stage":0,"value":{"value":1},"unknown":true}`),
		"finished as ready":  json.RawMessage(`{"phase":"ready","stage":1,"value":{"value":1}}`),
		"child in transform": json.RawMessage(`{"phase":"awaiting_child_start","stage":0,"value":{"value":1}}`),
	} {
		t.Run(name, func(t *testing.T) {
			state, err := agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := definition.Restore(state); !errors.Is(err, ErrInvalidExecutionState) {
				t.Fatalf("Restore error = %v", err)
			}
		})
	}
}
