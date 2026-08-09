package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
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

func (definition externalDefinition) Descriptor() agent.Descriptor { return definition.descriptor }

func (externalDefinition) Start(input agent.Input) (agent.Execution, error) {
	value, err := agent.DecodeInput[externalInput](input)
	if err != nil {
		return nil, err
	}
	return &externalExecution{value: value.Value}, nil
}

func (externalDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if state.Kind() != "external.direct" || state.SchemaVersion() != 1 {
		return nil, agent.ErrInvalidExecutionState
	}
	var value externalInput
	if err := json.Unmarshal(state.Payload(), &value); err != nil {
		return nil, err
	}
	return &externalExecution{value: value.Value}, nil
}

type externalExecution struct {
	value string
}

func (execution *externalExecution) Step(context.Context, []agent.Signal) (agent.Transition, error) {
	output, err := agent.EncodeOutput(externalOutput{Value: execution.value})
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Complete(0, output)
}

func (execution *externalExecution) Snapshot() (agent.ExecutionState, error) {
	payload, err := json.Marshal(externalInput{Value: execution.value})
	if err != nil {
		return agent.ExecutionState{}, err
	}
	return agent.NewExecutionState("external.direct", 1, payload)
}

type unusedDispatcher struct{}

func (unusedDispatcher) Dispatch(
	context.Context,
	agent.EffectRequest,
	agent.DeltaEmitter,
) (agent.Settlement, error) {
	return agent.Settlement{}, errors.New("direct execution declared an unexpected effect")
}

func (unusedDispatcher) ReplayPolicy(agent.Effect) agent.ReplayPolicy { return agent.ReplayPolicyNever }

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
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: externalDefinition{descriptor: descriptor}, Dispatcher: unusedDispatcher{},
		ImplementationDigest: agent.ComputeDigest([]byte("external-direct-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("external-direct-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	engine, err := agent.NewEngine(agent.EngineConfig{})
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
}
