package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

type delegateTaskResult struct {
	Reply string `json:"reply"`
}

// delegatedDefinition adapts Lyra's narrow DelegateTask contract to an
// ordinary Interaction without exposing the root chat envelope to the model.
// Interaction remains the sole owner of its snapshot representation.
type delegatedDefinition struct {
	descriptor agent.Descriptor
	inner      *interaction.Definition
}

func newDelegatedDefinition(name string, inner *interaction.Definition) (*delegatedDefinition, error) {
	if inner == nil {
		return nil, errors.New("agentexec: delegated Interaction is required")
	}
	inputSchema, err := agent.SchemaFor[DelegateTask]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated input schema: %w", err)
	}
	outputSchema, err := agent.SchemaFor[delegateTaskResult]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated output schema: %w", err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: "Complete one isolated delegated task and return a concise result.",
		Version: "1.0.0", InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: define delegated task: %w", err)
	}
	return &delegatedDefinition{descriptor: descriptor, inner: inner}, nil
}

func (definition *delegatedDefinition) Descriptor() agent.Descriptor {
	if definition == nil {
		return agent.Descriptor{}
	}
	return definition.descriptor
}

func (definition *delegatedDefinition) Start(input agent.Input) (agent.Execution, error) {
	if definition == nil || definition.inner == nil {
		return nil, errors.New("agentexec: delegated definition is invalid")
	}
	task, err := agent.DecodeInput[DelegateTask](input)
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode delegated task: %w", err)
	}
	if err := task.Validate(); err != nil {
		return nil, err
	}
	adapted, err := agent.EncodeInput(interaction.Input{Messages: []chat.Message{
		chat.NewUserMessage(chat.NewTextPart(task.Instructions)),
	}})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode delegated conversation: %w", err)
	}
	execution, err := definition.inner.Start(adapted)
	if err != nil {
		return nil, err
	}
	return &delegatedExecution{inner: execution}, nil
}

func (definition *delegatedDefinition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if definition == nil || definition.inner == nil {
		return nil, errors.New("agentexec: delegated definition is invalid")
	}
	execution, err := definition.inner.Restore(state)
	if err != nil {
		return nil, err
	}
	return &delegatedExecution{inner: execution}, nil
}

type delegatedExecution struct{ inner agent.Execution }

func (execution *delegatedExecution) Step(ctx context.Context, signals []agent.Signal) (agent.Transition, error) {
	if execution == nil || execution.inner == nil {
		return agent.Transition{}, errors.New("agentexec: delegated execution is invalid")
	}
	transition, err := execution.inner.Step(ctx, signals)
	if err != nil || transition.Kind() != agent.TransitionKindComplete {
		return transition, err
	}
	output, _ := transition.Output()
	decoded, err := agent.DecodeOutput[interaction.Output](output)
	if err != nil {
		return agent.Transition{}, fmt.Errorf("agentexec: decode delegated completion: %w", err)
	}
	reply, err := delegatedReply(decoded)
	if err != nil {
		return agent.Transition{}, err
	}
	encoded, err := agent.EncodeOutput(delegateTaskResult{Reply: reply})
	if err != nil {
		return agent.Transition{}, fmt.Errorf("agentexec: encode delegated completion: %w", err)
	}
	return agent.Complete(transition.ConsumedSignals(), encoded)
}

func (execution *delegatedExecution) Snapshot() (agent.ExecutionState, error) {
	if execution == nil || execution.inner == nil {
		return agent.ExecutionState{}, errors.New("agentexec: delegated execution is invalid")
	}
	return execution.inner.Snapshot()
}

func delegatedReply(output interaction.Output) (string, error) {
	if err := output.Validate(); err != nil {
		return "", err
	}
	switch output.Source {
	case interaction.CompletionSourceModelResponse:
		choice := output.ModelResponse.First()
		if choice == nil || choice.Message == nil || choice.Message.Text() == "" {
			return "", errors.New("agentexec: delegated task completed without a reply")
		}
		return choice.Message.Text(), nil
	case interaction.CompletionSourceDirectToolResults:
		encoded, err := json.Marshal(output.DirectToolResults)
		if err != nil {
			return "", fmt.Errorf("agentexec: encode delegated tool results: %w", err)
		}
		return string(encoded), nil
	default:
		return "", errors.New("agentexec: unsupported delegated completion")
	}
}

var _ agent.Definition = (*delegatedDefinition)(nil)
var _ agent.Execution = (*delegatedExecution)(nil)
