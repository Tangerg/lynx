package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

const (
	defaultDelegateDepth          = 4
	defaultDelegateChildren       = 16
	defaultActiveDelegateChildren = 4
	defaultDelegateTreeProcesses  = 64
	defaultDelegateSteps          = 256
	defaultDelegateEffects        = 256
	defaultDelegateSignals        = 2048
)

// InteractionDelegationConfig bounds managed children independently of
// model/token product limits. Zero fields inherit conservative defaults. The
// values translate only into Agent Framework structural limits and a minimum per-Process
// work allocation. A delegated Process receives one allocation unit for itself
// and one for each remaining recursion level, so the configured depth is
// reachable without renewing or duplicating Framework budget.
type InteractionDelegationConfig struct {
	MaxDepth          uint32
	MaxChildren       uint32
	MaxActiveChildren uint32
	MaxTreeProcesses  uint32
	ChildSteps        uint64
	ChildEffects      uint64
	ChildSignals      uint64
}

type effectiveInteractionDelegation struct {
	treeLimits    agent.TreeLimits
	processBudget agent.Budget
}

func effectiveDelegation(config InteractionDelegationConfig) (effectiveInteractionDelegation, error) {
	if config.MaxDepth == 0 {
		config.MaxDepth = defaultDelegateDepth
	}
	if config.MaxChildren == 0 {
		config.MaxChildren = defaultDelegateChildren
	}
	if config.MaxActiveChildren == 0 {
		config.MaxActiveChildren = defaultActiveDelegateChildren
	}
	if config.MaxTreeProcesses == 0 {
		config.MaxTreeProcesses = defaultDelegateTreeProcesses
	}
	if config.ChildSteps == 0 {
		config.ChildSteps = defaultDelegateSteps
	}
	if config.ChildEffects == 0 {
		config.ChildEffects = defaultDelegateEffects
	}
	if config.ChildSignals == 0 {
		config.ChildSignals = defaultDelegateSignals
	}
	treeLimits := agent.TreeLimits{
		MaxDepth: config.MaxDepth, MaxChildren: config.MaxChildren,
		MaxActiveChildren: config.MaxActiveChildren, MaxTreeProcesses: config.MaxTreeProcesses,
	}
	if !treeLimits.Valid() {
		return effectiveInteractionDelegation{}, errors.New("agentexec: Interaction delegation tree limits are invalid")
	}
	budget, err := agent.NewBudget(config.ChildSteps, config.ChildEffects, config.ChildSignals)
	if err != nil {
		return effectiveInteractionDelegation{}, fmt.Errorf("agentexec: Interaction delegation budget: %w", err)
	}
	return effectiveInteractionDelegation{treeLimits: treeLimits, processBudget: budget}, nil
}

func delegateSubtreeBudget(base agent.Budget, processLevels uint32) (agent.Budget, error) {
	if !base.Valid() || processLevels == 0 {
		return agent.Budget{}, errors.New("agentexec: delegated subtree budget requires a positive base and depth")
	}
	scale := uint64(processLevels)
	if base.Steps > math.MaxUint64/scale || base.Effects > math.MaxUint64/scale ||
		base.Signals > math.MaxUint64/scale {
		return agent.Budget{}, errors.New("agentexec: delegated subtree budget overflows")
	}
	budget, err := agent.NewBudget(
		base.Steps*scale,
		base.Effects*scale,
		base.Signals*scale,
	)
	if err != nil {
		return agent.Budget{}, fmt.Errorf("agentexec: delegated subtree budget: %w", err)
	}
	return budget, nil
}

type delegatedTaskOutput struct {
	Reply string `json:"reply" jsonschema:"minLength=1"`
}

// delegatedInteractionDefinition is an ACL Definition: models execute an
// ordinary Interaction, while the managed Delegate boundary exposes the
// stable delegate_task input/output contract instead of Interaction's Host chat
// envelope. Snapshot interpretation remains exclusively Interaction-owned.
type delegatedInteractionDefinition struct {
	descriptor   agent.Descriptor
	inner        *interaction.Definition
	instructions []corechat.Message
}

func newDelegatedInteractionDefinition(
	name string,
	inner *interaction.Definition,
	instructions []corechat.Message,
) (*delegatedInteractionDefinition, error) {
	if inner == nil {
		return nil, errors.New("agentexec: delegated Interaction definition is nil")
	}
	inputSchema, err := runtimeContractSchema[delegateInput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task input schema: %w", err)
	}
	outputSchema, err := runtimeContractSchema[delegatedTaskOutput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task output schema: %w", err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: delegateDescription,
		Version: interactionDefinitionVersion, InputSchema: inputSchema, OutputSchema: outputSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated Interaction descriptor: %w", err)
	}
	return &delegatedInteractionDefinition{
		descriptor: descriptor, inner: inner,
		instructions: cloneChatMessages(instructions),
	}, nil
}

// runtimeContractSchema derives the delegated contract through the Agent
// Framework's schema owner. Both sides use Core's canonical JSON Schema
// implementation, so the wire vocabulary has no adapter-local representation.
func runtimeContractSchema[T any]() (agent.Schema, error) {
	return agent.SchemaFor[T]()
}

func (d *delegatedInteractionDefinition) Descriptor() agent.Descriptor {
	if d == nil {
		return agent.Descriptor{}
	}
	return d.descriptor
}

func (d *delegatedInteractionDefinition) Start(input agent.Input) (agent.Execution, error) {
	if d == nil || d.inner == nil || !d.descriptor.Valid() {
		return nil, errors.New("agentexec: delegated Interaction definition is invalid")
	}
	task, err := input.Decode[delegateInput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode delegated task: %w", err)
	}
	if validateErr := task.Validate(); validateErr != nil {
		return nil, fmt.Errorf("agentexec: invalid delegated task: %w", validateErr)
	}
	messages := cloneChatMessages(d.instructions)
	messages = append(messages, corechat.NewUserMessage(corechat.NewTextPart(task.Instructions)))
	adapted, err := agent.EncodeInput(interaction.Input{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode delegated Interaction input: %w", err)
	}
	execution, err := d.inner.Start(adapted)
	if err != nil {
		return nil, err
	}
	return &delegatedInteractionExecution{inner: execution}, nil
}

func (d *delegatedInteractionDefinition) Restore(
	state agent.ExecutionState,
) (agent.Execution, error) {
	if d == nil || d.inner == nil || !d.descriptor.Valid() {
		return nil, errors.New("agentexec: delegated Interaction definition is invalid")
	}
	execution, err := d.inner.Restore(state)
	if err != nil {
		return nil, err
	}
	return &delegatedInteractionExecution{inner: execution}, nil
}

type delegatedInteractionExecution struct{ inner agent.Execution }

func (d *delegatedInteractionExecution) Step(
	ctx context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	if d == nil || d.inner == nil {
		return agent.Transition{}, errors.New("agentexec: delegated Interaction execution is invalid")
	}
	transition, err := d.inner.Step(ctx, signals)
	if err != nil || transition.Kind() != agent.TransitionKindComplete {
		return transition, err
	}
	erased, _ := transition.Output()
	output, err := erased.Decode[interaction.Output]()
	if err != nil {
		return agent.Transition{}, fmt.Errorf("agentexec: decode delegated Interaction output: %w", err)
	}
	reply, err := delegatedInteractionReply(output)
	if err != nil {
		return agent.Transition{}, err
	}
	adapted, err := agent.EncodeOutput(delegatedTaskOutput{Reply: reply})
	if err != nil {
		return agent.Transition{}, fmt.Errorf("agentexec: encode delegated task output: %w", err)
	}
	return agent.Complete(transition.ConsumedSignals(), adapted)
}

func (d *delegatedInteractionExecution) Snapshot() (agent.ExecutionState, error) {
	if d == nil || d.inner == nil {
		return agent.ExecutionState{}, errors.New("agentexec: delegated Interaction execution is invalid")
	}
	return d.inner.Snapshot()
}

func delegatedInteractionReply(output interaction.Output) (string, error) {
	if err := output.Validate(); err != nil {
		return "", err
	}
	switch output.Source {
	case interaction.CompletionSourceModelResponse:
		modelOutput := output.ModelResponse.Output
		if modelOutput == nil || modelOutput.Message == nil || modelOutput.Message.Text() == "" {
			return "", errors.New("agentexec: delegated Interaction completed without a textual answer")
		}
		return modelOutput.Message.Text(), nil
	case interaction.CompletionSourceDirectToolResults:
		encoded, err := json.Marshal(output.DirectToolResults)
		if err != nil {
			return "", fmt.Errorf("agentexec: encode delegated direct Tool results: %w", err)
		}
		return string(encoded), nil
	default:
		return "", fmt.Errorf("agentexec: unsupported delegated Interaction completion source %q", output.Source)
	}
}
