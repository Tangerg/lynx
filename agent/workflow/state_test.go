package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agent "github.com/Tangerg/scope/agent"
)

type stateFixture struct {
	Value int `json:"value"`
}

func TestRestoreRejectsUnknownAndContradictoryState(t *testing.T) {
	definition := stateTestDefinition(t)
	for name, payload := range map[string]json.RawMessage{
		"unknown field":            json.RawMessage(`{"phase":"ready","stage_index":0,"current_value":{"value":1},"unknown":true}`),
		"finished as ready":        json.RawMessage(`{"phase":"ready","stage_index":1,"current_value":{"value":1}}`),
		"child in transform":       json.RawMessage(`{"phase":"awaiting_child_start","stage_index":0,"current_value":{"value":1}}`),
		"Loop cursor in Transform": json.RawMessage(`{"phase":"ready","stage_index":0,"current_value":{"value":1},"loop_iteration":1}`),
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
	validPayload := json.RawMessage(`{"phase":"ready","stage_index":0,"current_value":{"value":1}}`)
	for _, envelope := range []struct {
		kind    string
		version uint16
	}{{kind: "other", version: 1}, {kind: executionStateKind, version: executionStateSchemaVersion - 1}} {
		state, err := agent.NewExecutionState(envelope.kind, envelope.version, validPayload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := definition.Restore(state); !errors.Is(err, ErrInvalidExecutionState) {
			t.Fatalf("Restore envelope error = %v", err)
		}
	}
}

func TestExecutionRejectsMissingProtocolSignals(t *testing.T) {
	callDefinition, fanoutDefinition := protocolTestDefinitions(t)
	for name, test := range map[string]struct {
		definition *Definition
		payload    string
	}{
		"child start": {
			definition: callDefinition,
			payload:    `{"phase":"awaiting_child_start","stage_index":0,"current_value":{"value":1}}`,
		},
		"child wait opening": {
			definition: callDefinition,
			payload:    `{"phase":"awaiting_child_wait_open","stage_index":0,"current_value":{"value":1},"child_process_id":"child"}`,
		},
		"child completion": {
			definition: callDefinition,
			payload:    `{"phase":"waiting_child","stage_index":0,"current_value":{"value":1},"child_process_id":"child","wait_id":"wait"}`,
		},
		"fan-out starts": {
			definition: fanoutDefinition,
			payload:    `{"phase":"awaiting_fanout_starts","stage_index":0,"current_value":{"value":1},"next_fanout_index":1,"active_fanout_window":[{"fanout_index":0}],"fanout_outputs":[null]}`,
		},
		"fan-out wait opening": {
			definition: fanoutDefinition,
			payload:    `{"phase":"awaiting_fanout_wait_open","stage_index":0,"current_value":{"value":1},"next_fanout_index":1,"active_fanout_window":[{"fanout_index":0,"child_process_id":"child"}],"fanout_outputs":[null]}`,
		},
		"fan-out completion": {
			definition: fanoutDefinition,
			payload:    `{"phase":"waiting_fanout","stage_index":0,"current_value":{"value":1},"wait_id":"wait","next_fanout_index":1,"active_fanout_window":[{"fanout_index":0,"child_process_id":"child"}],"fanout_outputs":[null]}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			state, err := agent.NewExecutionState(
				executionStateKind, executionStateSchemaVersion, json.RawMessage(test.payload),
			)
			if err != nil {
				t.Fatal(err)
			}
			execution, err := test.definition.Restore(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := execution.Step(context.Background(), nil); !errors.Is(err, ErrInvalidProtocol) {
				t.Fatalf("Step error = %v", err)
			}
		})
	}
}

func FuzzWorkflowExecutionStateRestore(f *testing.F) {
	definition := stateTestDefinition(f)
	f.Add([]byte(`{"phase":"ready","stage_index":0,"current_value":{"value":1}}`))
	f.Add([]byte(`{"phase":"completed","stage_index":1,"current_value":{"value":1}}`))
	f.Add([]byte(`{"phase":"waiting_fanout","stage_index":0,"current_value":{"value":1}}`))
	f.Add([]byte(`{"phase":"ready","stage_index":0,"current_value":{"value":1},"unknown":true}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		state, err := agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
		if err != nil {
			return
		}
		execution, err := definition.Restore(state)
		if err != nil {
			return
		}
		restored, err := execution.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := definition.Restore(restored); err != nil {
			t.Fatalf("accepted state is not restorable: %v", err)
		}
	})
}

func stateTestDefinition(t testing.TB) *Definition {
	t.Helper()
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
	return definition
}

func protocolTestDefinitions(t testing.TB) (*Definition, *Definition) {
	t.Helper()
	childDefinition := stateTestDefinition(t)
	child, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: childDefinition, Dispatcher: Dispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("workflow-state-protocol-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("workflow-state-protocol-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	budget, err := agent.NewBudget(8, 8, 8)
	if err != nil {
		t.Fatal(err)
	}
	call, err := Call(CallConfig{ID: "child", Deployment: child, Budget: budget})
	if err != nil {
		t.Fatal(err)
	}
	callDefinition, err := NewDefinition(DefinitionConfig{
		Name: "test.workflow.call_protocol", Description: "Validate Call protocol failures.",
		Version: "1.0.0", Stages: []Stage{call},
	})
	if err != nil {
		t.Fatal(err)
	}
	fanout, err := Fork(ForkConfig[stateFixture, stateFixture, stateFixture]{
		ID:         "fanout",
		Branches:   []ForkBranch{{ID: "only", Deployment: child, Budget: budget}},
		WindowSize: 1,
		Reduce: func(values []stateFixture) (stateFixture, error) {
			return values[0], nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fanoutDefinition, err := NewDefinition(DefinitionConfig{
		Name: "test.workflow.fanout_protocol", Description: "Validate fan-out protocol failures.",
		Version: "1.0.0", Stages: []Stage{fanout},
	})
	if err != nil {
		t.Fatal(err)
	}
	return callDefinition, fanoutDefinition
}
