package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/agenttest"
)

type externalInput struct {
	Value string `json:"value"`
}

type externalOutput struct {
	Value string `json:"value"`
}

type externalDefinition struct {
	descriptor agent.Descriptor
}

type externalState struct {
	Phase string `json:"phase"`
	Value string `json:"value"`
}

func (definition externalDefinition) Descriptor() agent.Descriptor { return definition.descriptor }

func (externalDefinition) Start(input agent.Input) (agent.Execution, error) {
	value, err := agent.DecodeInput[externalInput](input)
	if err != nil {
		return nil, err
	}
	return &externalExecution{state: externalState{Phase: "ready", Value: value.Value}}, nil
}

func (externalDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "external.direct" || state.SchemaVersion() != 1 {
		return nil, agent.ErrInvalidExecutionState
	}
	var value externalState
	if err := json.Unmarshal(state.Payload(), &value); err != nil {
		return nil, err
	}
	return &externalExecution{state: value}, nil
}

type externalExecution struct {
	state externalState
}

func (execution *externalExecution) Step(_ context.Context, signals []agent.Signal) (agent.Transition, error) {
	switch execution.state.Phase {
	case "ready":
		payload, err := json.Marshal(externalInput{Value: execution.state.Value})
		if err != nil {
			return agent.Transition{}, err
		}
		effect, err := agent.NewDispatcherEffect(payload)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.Phase = "dispatched"
		return agent.Continue(0, effect)
	case "dispatched":
		if len(signals) != 1 {
			return agent.Transition{}, agent.ErrInvalidSignal
		}
		var result externalOutput
		if err := json.Unmarshal(signals[0].Payload(), &result); err != nil {
			return agent.Transition{}, err
		}
		output, err := agent.EncodeOutput(result)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.Phase = "completed"
		return agent.Complete(1, output)
	default:
		return agent.Transition{}, agent.ErrInvalidExecutionState
	}
}

func (execution *externalExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(execution.state)
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("external.direct", 1, payload)
}

func TestExternalPackageCanComposeAndRunDefinition(t *testing.T) {
	inputSchema, err := agent.SchemaFor[externalInput]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := agent.SchemaFor[externalOutput]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: "external.direct", Description: "Completes a direct external API example.", Version: "0.1.0",
		InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedPayload, err := json.Marshal(externalInput{Value: "done"})
	if err != nil {
		t.Fatal(err)
	}
	expectedEffect, err := agent.NewDispatcherEffect(expectedPayload)
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := agenttest.NewScriptedDispatcher(agenttest.ScriptedDispatcherConfig{
		ReplayPolicy: agent.ReplayPolicyNever,
		Steps: []agenttest.DispatchStep{{
			ExpectedEffect:    &expectedEffect,
			Deltas:            []json.RawMessage{json.RawMessage(`{"text":"do"}`)},
			SettlementStatus:  agent.SettlementStatusSucceeded,
			SettlementPayload: json.RawMessage(`{"value":"done"}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: externalDefinition{descriptor: descriptor}, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("external-direct-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("external-direct-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	observations := &agenttest.ObservationRecorder{}
	preparedSteps := agenttest.NewPreparedStepRecorder(nil)
	engine, err := agent.NewEngine(agent.EngineConfig{
		PreparedStepAcknowledger: preparedSteps,
		EventListeners:           []agent.EventListener{observations},
		DeltaListeners:           []agent.DeltaListener{observations},
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodeInput(externalInput{Value: "done"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	output, ok := result.Output()
	if !ok {
		t.Fatal("completed Result has no Output")
	}
	value, err := agent.DecodeOutput[externalOutput](output)
	if err != nil || value.Value != "done" {
		t.Fatalf("output=%+v err=%v", value, err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if dispatcher.Remaining() != 0 || len(dispatcher.Requests()) != 1 {
		t.Fatalf("remaining dispatches=%d requests=%d", dispatcher.Remaining(), len(dispatcher.Requests()))
	}
	if len(preparedSteps.Snapshots()) != 1 {
		t.Fatalf("prepared snapshots=%d, want 1", len(preparedSteps.Snapshots()))
	}
	if len(observations.Events()) == 0 || len(observations.Deltas()) != 1 {
		t.Fatalf("events=%d deltas=%d", len(observations.Events()), len(observations.Deltas()))
	}
	waitContext, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	finished, err := observations.AwaitEvent(waitContext, func(event agent.Event) bool {
		return event.Name() == agent.EventProcessFinished
	})
	if err != nil || finished.ProcessID() != result.ProcessID() {
		t.Fatalf("finished event=%+v error=%v", finished, err)
	}
}
