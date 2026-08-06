package interaction

import (
	"encoding/json"
	"errors"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

func TestArtifactStateRestoreRejectsInvalidProvenanceAndValue(t *testing.T) {
	definition := fuzzInteractionDefinition(t)
	request := &chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("validate"))}}
	validOutput, _ := agent.EncodeOutput(fuzzDelegateOutput{Result: "valid"})
	wrongOutput, _ := agent.EncodeOutput(struct {
		Other string `json:"other"`
	}{Other: "invalid"})

	cases := []struct {
		name      string
		artifacts []artifactRecord
	}{
		{
			name: "unknown Delegate",
			artifacts: []artifactRecord{{
				ModelCallSequence: 1, ToolCallIndex: 0, ToolCallID: "call_1",
				DelegateName: "missing", Output: validOutput,
			}},
		},
		{
			name: "wrong schema",
			artifacts: []artifactRecord{{
				ModelCallSequence: 1, ToolCallIndex: 0, ToolCallID: "call_1",
				DelegateName: "delegate_fuzz", Output: wrongOutput,
			}},
		},
		{
			name: "duplicate identity",
			artifacts: []artifactRecord{
				{ModelCallSequence: 1, ToolCallIndex: 0, ToolCallID: "same", DelegateName: "delegate_fuzz", Output: validOutput},
				{ModelCallSequence: 1, ToolCallIndex: 1, ToolCallID: "same", DelegateName: "delegate_fuzz", Output: validOutput},
			},
		},
		{
			name: "reversed position",
			artifacts: []artifactRecord{
				{ModelCallSequence: 1, ToolCallIndex: 1, ToolCallID: "later", DelegateName: "delegate_fuzz", Output: validOutput},
				{ModelCallSequence: 1, ToolCallIndex: 0, ToolCallID: "earlier", DelegateName: "delegate_fuzz", Output: validOutput},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			state := executionState{
				Phase: phaseAwaitingModel, WorkingContext: request.Clone(), ModelCallCount: 2,
				ArtifactRecords: test.artifacts,
			}
			payload, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := definition.Restore(envelope); !errors.Is(err, ErrInvalidExecutionState) {
				t.Fatalf("Restore error=%v, want ErrInvalidExecutionState", err)
			}
		})
	}
}

func TestInteractionDoesNotReadPriorExecutionState(t *testing.T) {
	definition := fuzzInteractionDefinition(t)
	state := executionState{
		Phase: phaseAwaitingModel,
		WorkingContext: &chat.Request{Messages: []chat.Message{
			chat.NewUserMessage(chat.NewTextPart("old state")),
		}},
		ModelCallCount: 1,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	old, err := agent.NewExecutionState(executionStateKind, executionStateSchemaVersion-1, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Restore(old); !errors.Is(err, ErrInvalidExecutionState) {
		t.Fatalf("Restore(prior version) error=%v, want ErrInvalidExecutionState", err)
	}
}
