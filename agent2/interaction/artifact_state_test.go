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
				ModelCall: 1, CallIndex: 0, CallID: "call_1",
				DelegateName: "missing", Output: validOutput,
			}},
		},
		{
			name: "wrong schema",
			artifacts: []artifactRecord{{
				ModelCall: 1, CallIndex: 0, CallID: "call_1",
				DelegateName: "delegate_fuzz", Output: wrongOutput,
			}},
		},
		{
			name: "duplicate identity",
			artifacts: []artifactRecord{
				{ModelCall: 1, CallIndex: 0, CallID: "same", DelegateName: "delegate_fuzz", Output: validOutput},
				{ModelCall: 1, CallIndex: 1, CallID: "same", DelegateName: "delegate_fuzz", Output: validOutput},
			},
		},
		{
			name: "reversed position",
			artifacts: []artifactRecord{
				{ModelCall: 1, CallIndex: 1, CallID: "later", DelegateName: "delegate_fuzz", Output: validOutput},
				{ModelCall: 1, CallIndex: 0, CallID: "earlier", DelegateName: "delegate_fuzz", Output: validOutput},
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			state := executionState{
				Phase: phaseAwaitingModel, Request: request.Clone(), ModelCalls: 2,
				Artifacts: test.artifacts,
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

func TestArtifactStateDoesNotReadExecutionStateV2(t *testing.T) {
	definition := fuzzInteractionDefinition(t)
	state := executionState{
		Phase: phaseAwaitingModel,
		Request: &chat.Request{Messages: []chat.Message{
			chat.NewUserMessage(chat.NewTextPart("old state")),
		}},
		ModelCalls: 1,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	old, err := agent.NewExecutionState(executionStateKind, 2, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Restore(old); !errors.Is(err, ErrInvalidExecutionState) {
		t.Fatalf("Restore(v2) error=%v, want ErrInvalidExecutionState", err)
	}
}
