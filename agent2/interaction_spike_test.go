package agent2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type interactionSpikeInput struct {
	Prompt string `json:"prompt"`
}

type interactionSpikeOutput struct {
	Message string `json:"message"`
}

type interactionSpikeState struct {
	Phase    string   `json:"phase"`
	Prompt   string   `json:"prompt"`
	Steering []string `json:"steering,omitempty"`
	WaitID   string   `json:"wait_id,omitempty"`
}

type interactionSpikeEffect struct {
	Version uint16 `json:"version"`
	Kind    string `json:"kind"`
	Prompt  string `json:"prompt,omitempty"`
	Tool    string `json:"tool,omitempty"`
}

type interactionSpikeSignal struct {
	Version          uint16 `json:"version"`
	Kind             string `json:"kind"`
	Text             string `json:"text,omitempty"`
	RequiresApproval bool   `json:"requires_approval,omitempty"`
	Approved         bool   `json:"approved,omitempty"`
}

type interactionSpikeDefinition struct {
	descriptor Descriptor
}

func newInteractionSpikeDefinition(t *testing.T) *interactionSpikeDefinition {
	t.Helper()
	inputSchema, err := SchemaFor[interactionSpikeInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[interactionSpikeOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name:         "interaction.spike",
		Description:  "Validates the candidate Interaction execution contracts.",
		Version:      "0.1.0",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &interactionSpikeDefinition{descriptor: descriptor}
}

func (definition *interactionSpikeDefinition) Descriptor() Descriptor { return definition.descriptor }

func (definition *interactionSpikeDefinition) Start(input Input) (Execution, error) {
	if err := definition.descriptor.ValidateInput(input); err != nil {
		return nil, err
	}
	value, err := DecodeInput[interactionSpikeInput](input)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(value.Prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	return &interactionSpikeExecution{state: interactionSpikeState{Phase: "model", Prompt: value.Prompt}}, nil
}

func (definition *interactionSpikeDefinition) Restore(state ExecutionState) (Execution, error) {
	if state.Kind() != definition.descriptor.Name() || state.SchemaVersion() != 1 {
		return nil, errors.New("unsupported interaction state")
	}
	value, err := decodeJSON[interactionSpikeState](state.Payload())
	if err != nil {
		return nil, err
	}
	if !validInteractionSpikePhase(value.Phase) || strings.TrimSpace(value.Prompt) == "" {
		return nil, errors.New("invalid interaction state")
	}
	return &interactionSpikeExecution{state: value}, nil
}

func validInteractionSpikePhase(phase string) bool {
	switch phase {
	case "model", "model_result", "tool_result", "wait_id", "approval", "final_result":
		return true
	default:
		return false
	}
}

type interactionSpikeExecution struct {
	state interactionSpikeState
}

func (execution *interactionSpikeExecution) Step(ctx context.Context, signals []Signal) (Transition, error) {
	if err := ctx.Err(); err != nil {
		return Transition{}, err
	}
	switch execution.state.Phase {
	case "model":
		if len(signals) != 0 {
			return Transition{}, errors.New("model phase does not accept signals")
		}
		execution.state.Phase = "model_result"
		return interactionSpikeContinue(0, interactionSpikeEffect{Version: 1, Kind: "model", Prompt: execution.state.Prompt})

	case "model_result":
		if len(signals) != 1 {
			return Transition{}, errors.New("model result phase requires one signal")
		}
		result, err := decodeInteractionSpikeSignal(signals[0])
		if err != nil || result.Kind != "model_result" {
			return Transition{}, errors.New("expected model result")
		}
		execution.state.Phase = "tool_result"
		return interactionSpikeContinue(1, interactionSpikeEffect{Version: 1, Kind: "tool", Tool: result.Text})

	case "tool_result":
		var toolResult *interactionSpikeSignal
		for _, signal := range signals {
			value, err := decodeInteractionSpikeSignal(signal)
			if err != nil {
				return Transition{}, err
			}
			switch value.Kind {
			case "steer":
				execution.state.Steering = append(execution.state.Steering, value.Text)
			case "tool_result":
				copyOfValue := value
				toolResult = &copyOfValue
			default:
				return Transition{}, fmt.Errorf("unexpected interaction signal %q", value.Kind)
			}
		}
		if toolResult == nil || !toolResult.RequiresApproval {
			return Transition{}, errors.New("tool result did not request approval")
		}
		key, err := ParseWaitKey("tool.approval")
		if err != nil {
			return Transition{}, err
		}
		waitSignal, err := json.Marshal(interactionSpikeSignal{Version: 1, Kind: "wait_id"})
		if err != nil {
			return Transition{}, err
		}
		waitEffect, err := RequestWait(key, waitSignal)
		if err != nil {
			return Transition{}, err
		}
		execution.state.Phase = "wait_id"
		return Continue(uint32(len(signals)), waitEffect)

	case "wait_id":
		if len(signals) != 1 {
			return Transition{}, errors.New("wait identity phase requires one signal")
		}
		value, err := decodeInteractionSpikeSignal(signals[0])
		if err != nil || value.Kind != "wait_id" {
			return Transition{}, errors.New("expected wait identity signal")
		}
		waitID, ok := signals[0].WaitID()
		if !ok {
			return Transition{}, errors.New("Engine did not attach a WaitID")
		}
		execution.state.WaitID = waitID.String()
		execution.state.Phase = "approval"
		return Wait(1, waitID)

	case "approval":
		if len(signals) != 1 {
			return Transition{}, errors.New("approval phase requires one signal")
		}
		waitID, ok := signals[0].WaitID()
		if !ok || waitID.String() != execution.state.WaitID {
			return Transition{}, errors.New("approval addressed the wrong wait")
		}
		answer, err := decodeInteractionSpikeSignal(signals[0])
		if err != nil || answer.Kind != "human_answer" || !answer.Approved {
			return Transition{}, errors.New("tool approval was denied or malformed")
		}
		execution.state.Phase = "final_result"
		prompt := execution.state.Prompt + "\nsteering: " + strings.Join(execution.state.Steering, ", ") + "\napproval: granted"
		return interactionSpikeContinue(1, interactionSpikeEffect{Version: 1, Kind: "model", Prompt: prompt})

	case "final_result":
		if len(signals) != 1 {
			return Transition{}, errors.New("final result phase requires one signal")
		}
		result, err := decodeInteractionSpikeSignal(signals[0])
		if err != nil || result.Kind != "model_result" {
			return Transition{}, errors.New("expected final model result")
		}
		output, err := EncodeOutput(interactionSpikeOutput{Message: result.Text})
		if err != nil {
			return Transition{}, err
		}
		return Complete(1, output)

	default:
		return Transition{}, errors.New("invalid interaction phase")
	}
}

func interactionSpikeContinue(consumedSignals uint32, value interactionSpikeEffect) (Transition, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return Transition{}, err
	}
	effect, err := NewDispatcherEffect(payload)
	if err != nil {
		return Transition{}, err
	}
	return Continue(consumedSignals, effect)
}

func decodeInteractionSpikeSignal(signal Signal) (interactionSpikeSignal, error) {
	value, err := decodeJSON[interactionSpikeSignal](signal.Payload())
	if err != nil {
		return interactionSpikeSignal{}, err
	}
	if value.Version != 1 {
		return interactionSpikeSignal{}, errors.New("unsupported interaction signal version")
	}
	return value, nil
}

func (execution *interactionSpikeExecution) Snapshot() (ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return ExecutionState{}, err
	}
	return NewExecutionState("interaction.spike", 1, payload)
}

type interactionSpikeDispatcher struct {
	modelCalls int
}

func (dispatcher *interactionSpikeDispatcher) Dispatch(effectID EffectID, effect Effect, emit func(Delta)) (Settlement, error) {
	value, err := decodeJSON[interactionSpikeEffect](effect.Payload())
	if err != nil || value.Version != 1 {
		return Settlement{}, errors.New("invalid interaction effect")
	}
	var result interactionSpikeSignal
	switch value.Kind {
	case "model":
		dispatcher.modelCalls++
		for sequence, text := range []string{"stream-1", "stream-2"} {
			delta, err := newDelta(mustProcessID("process:interaction"), effectID, uint64(sequence+1), time.Unix(int64(sequence+1), 0), json.RawMessage(fmt.Sprintf(`{"text":%q}`, text)))
			if err != nil {
				return Settlement{}, err
			}
			emit(delta)
		}
		if dispatcher.modelCalls == 1 {
			result = interactionSpikeSignal{Version: 1, Kind: "model_result", Text: "shell"}
		} else {
			if !strings.Contains(value.Prompt, "prefer concise output") || !strings.Contains(value.Prompt, "approval: granted") {
				return Settlement{}, errors.New("steering or approval was not applied before the next model Effect")
			}
			result = interactionSpikeSignal{Version: 1, Kind: "model_result", Text: "final semantic output"}
		}
	case "tool":
		result = interactionSpikeSignal{Version: 1, Kind: "tool_result", Text: value.Tool, RequiresApproval: true}
	default:
		return Settlement{}, fmt.Errorf("unknown interaction Effect %q", value.Kind)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return Settlement{}, err
	}
	return NewSettlement(effectID, SettlementStatusSucceeded, payload)
}

func TestInteractionSpikeValidatesModelToolStreamHITLAndSteer(t *testing.T) {
	definition := newInteractionSpikeDefinition(t)
	typed, err := NewTyped[interactionSpikeInput, interactionSpikeOutput](definition)
	if err != nil {
		t.Fatal(err)
	}
	input, err := typed.EncodeInput(interactionSpikeInput{Prompt: "inspect repository"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := definition.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &interactionSpikeDispatcher{}

	model := mustStep(t, execution, nil, TransitionKindContinue)
	modelSettlement, streamed := dispatchInteractionSpike(t, dispatcher, 1, model.Effects()[0], true)
	if streamed != 2 {
		t.Fatalf("first model streamed %d deltas, want 2", streamed)
	}
	modelSignal := mustSignal(t, "signal:model", WaitID{}, time.Unix(3, 0), modelSettlement.Payload())

	tool := mustStep(t, execution, []Signal{modelSignal}, TransitionKindContinue)
	toolSettlement, _ := dispatchInteractionSpike(t, dispatcher, 2, tool.Effects()[0], true)
	steerPayload, _ := json.Marshal(interactionSpikeSignal{Version: 1, Kind: "steer", Text: "prefer concise output"})
	steer := mustSignal(t, "signal:steer", WaitID{}, time.Unix(4, 0), steerPayload)
	toolSignal := mustSignal(t, "signal:tool", WaitID{}, time.Unix(5, 0), toolSettlement.Payload())

	waitRequest := mustStep(t, execution, []Signal{steer, toolSignal}, TransitionKindContinue)
	key, waitSignalPayload, err := decodeWaitRequest(waitRequest.Effects()[0])
	if err != nil || key.String() != "tool.approval" {
		t.Fatalf("wait request = key %v error %v", key, err)
	}
	waitID, _ := ParseWaitID("wait:approval:1")
	waitOpened := mustSignal(t, "signal:wait", waitID, time.Unix(6, 0), waitSignalPayload)
	waiting := mustStep(t, execution, []Signal{waitOpened}, TransitionKindWait)
	if got, ok := waiting.WaitID(); !ok || got != waitID {
		t.Fatalf("waiting WaitID = %v, %t", got, ok)
	}

	waitingState, err := execution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	execution, err = definition.Restore(waitingState)
	if err != nil {
		t.Fatal(err)
	}
	answerPayload, _ := json.Marshal(interactionSpikeSignal{Version: 1, Kind: "human_answer", Approved: true})
	answer := mustSignal(t, "signal:answer", waitID, time.Unix(7, 0), answerPayload)
	finalModel := mustStep(t, execution, []Signal{answer}, TransitionKindContinue)
	finalSettlement, streamed := dispatchInteractionSpike(t, dispatcher, 3, finalModel.Effects()[0], false)
	if streamed != 2 {
		t.Fatalf("discarded final stream reported %d deltas, want 2 attempted", streamed)
	}
	finalSignal := mustSignal(t, "signal:final", WaitID{}, time.Unix(8, 0), finalSettlement.Payload())
	completed := mustStep(t, execution, []Signal{finalSignal}, TransitionKindComplete)
	output, ok := completed.Output()
	if !ok {
		t.Fatal("completed Interaction has no Output")
	}
	decoded, err := typed.DecodeOutput(output)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Message != "final semantic output" {
		t.Fatalf("final Output = %+v", decoded)
	}
}

func dispatchInteractionSpike(t *testing.T, dispatcher *interactionSpikeDispatcher, step uint64, effect Effect, retainDelta bool) (Settlement, int) {
	t.Helper()
	effectID := mustEffectID(fmt.Sprintf("process:interaction:step:%d:effect:0", step))
	count := 0
	settlement, err := dispatcher.Dispatch(effectID, effect, func(delta Delta) {
		count++
		if retainDelta {
			_ = delta.Payload()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return settlement, count
}

func mustStep(t *testing.T, execution Execution, signals []Signal, want TransitionKind) Transition {
	t.Helper()
	transition, err := execution.Step(context.Background(), signals)
	if err != nil {
		t.Fatal(err)
	}
	if transition.Kind() != want {
		t.Fatalf("Step transition = %s, want %s", transition.Kind(), want)
	}
	return transition
}

func mustSignal(t *testing.T, id string, waitID WaitID, receivedAt time.Time, payload json.RawMessage) Signal {
	t.Helper()
	signalID, err := ParseSignalID(id)
	if err != nil {
		t.Fatal(err)
	}
	signal, err := newSignal(signalID, waitID, receivedAt, payload)
	if err != nil {
		t.Fatal(err)
	}
	return signal
}

func mustProcessID(value string) ProcessID {
	id, err := ParseProcessID(value)
	if err != nil {
		panic(err)
	}
	return id
}

func mustEffectID(value string) EffectID {
	id, err := ParseEffectID(value)
	if err != nil {
		panic(err)
	}
	return id
}
