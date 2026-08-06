package interaction

import (
	"bytes"
	"encoding/json"
	"testing"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

type fuzzDelegateInput struct {
	Task string `json:"task"`
}

type fuzzDelegateOutput struct {
	Result string `json:"result"`
}

func FuzzExecutionStateRestore(f *testing.F) {
	definition := fuzzInteractionDefinition(f)
	for _, state := range fuzzInteractionStates(f, definition) {
		f.Add([]byte(state.Payload()))
	}
	f.Add([]byte(`null`))
	f.Add([]byte(`{"phase":"waiting_delegates"}`))
	f.Fuzz(func(t *testing.T, payload []byte) {
		state, err := agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
		if err != nil {
			return
		}
		execution, err := definition.Restore(state)
		if err != nil {
			return
		}
		captured, err := execution.Snapshot()
		if err != nil {
			t.Fatalf("restored state cannot be captured: %v", err)
		}
		restored, err := definition.Restore(captured)
		if err != nil {
			t.Fatalf("captured state cannot be restored: %v", err)
		}
		roundTrip, err := restored.Snapshot()
		if err != nil {
			t.Fatalf("round-trip state cannot be captured: %v", err)
		}
		if !bytes.Equal(captured.Payload(), roundTrip.Payload()) {
			t.Fatalf("state round trip changed payload\nfirst:  %s\nsecond: %s", captured.Payload(), roundTrip.Payload())
		}
	})
}

func fuzzInteractionDefinition(f testing.TB) *Definition {
	f.Helper()
	inputSchema, err := agent.SchemaFor[fuzzDelegateInput]()
	if err != nil {
		f.Fatal(err)
	}
	outputSchema, err := agent.SchemaFor[fuzzDelegateOutput]()
	if err != nil {
		f.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "interaction.fuzz_worker", Description: "Provide a deterministic fuzz worker contract.",
		Version: "1.0.0", InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		f.Fatal(err)
	}
	reference, err := agent.NewDeploymentRef(
		descriptor,
		agent.ComputeDigest([]byte("interaction-fuzz-worker-implementation")),
		agent.ComputeDigest([]byte("interaction-fuzz-worker-configuration")),
	)
	if err != nil {
		f.Fatal(err)
	}
	budget, _ := agent.NewBudget(10, 10, 10)
	delegate := Delegate{
		definition: chat.ToolDefinition{
			Name: "delegate_fuzz", Description: "Delegate one fuzz task to the exact worker.",
			InputSchema: inputSchema.JSON(),
		},
		deploymentRef: reference, inputSchema: inputSchema, outputSchema: outputSchema,
		budget: budget, capabilities: agent.CapabilitySet{},
	}
	definition, err := NewDefinition(DefinitionConfig{
		Name: "interaction.fuzz", Description: "Exercise strict Interaction state restoration.",
		Version: "1.0.0", MaxModelCalls: 4, Delegates: []Delegate{delegate},
	})
	if err != nil {
		f.Fatal(err)
	}
	return definition
}

func fuzzInteractionStates(f testing.TB, definition *Definition) []agent.ExecutionState {
	f.Helper()
	request := &chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("fuzz"))}}
	call := chat.ToolCall{ID: "call_fuzz", Name: "delegate_fuzz", Arguments: `{"task":"check"}`}
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	response := &chat.Response{Choices: []chat.Choice{{
		Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls,
	}}}
	key, _ := delegateChildKey(1, call)
	processID, _ := agent.ParseProcessID("process:fuzz-child")
	waitID, _ := agent.ParseWaitID("wait:fuzz-child")
	artifactOutput, _ := agent.EncodeOutput(fuzzDelegateOutput{Result: "settled"})
	states := []executionState{
		{
			Phase: phaseAwaitingDelegateStarts, WorkingContext: request.Clone(), ModelCallCount: 1,
			PendingModelResponse: response.Clone(), ActiveToolCallEndIndex: 1,
			DelegateSegment: &delegateSegmentState{Invocations: []delegateInvocationState{{ChildKey: &key}}},
		},
		{
			Phase: phaseWaitingDelegates, WorkingContext: request.Clone(), ModelCallCount: 1,
			PendingModelResponse: response.Clone(), ActiveToolCallEndIndex: 1, WaitID: &waitID,
			DelegateSegment: &delegateSegmentState{Invocations: []delegateInvocationState{{
				ChildKey: &key, ChildProcessID: &processID,
			}}},
		},
		{
			Phase: phaseAwaitingModel, WorkingContext: request.Clone(), ModelCallCount: 2,
			ArtifactRecords: []artifactRecord{{
				ModelCallSequence: 1, ToolCallIndex: 0, ToolCallID: "call_settled",
				DelegateName: "delegate_fuzz", Output: artifactOutput,
			}},
		},
	}
	encoded := make([]agent.ExecutionState, 0, len(states))
	for _, state := range states {
		if err := state.Validate(definition); err != nil {
			f.Fatal(err)
		}
		payload, err := json.Marshal(state)
		if err != nil {
			f.Fatal(err)
		}
		envelope, err := agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
		if err != nil {
			f.Fatal(err)
		}
		encoded = append(encoded, envelope)
	}
	return encoded
}
