package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/delegation"
	corechat "github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
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
	inputSchema, err := runtimeContractSchema[delegation.Input]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task input schema: %w", err)
	}
	outputSchema, err := runtimeContractSchema[delegatedTaskOutput]()
	if err != nil {
		return nil, fmt.Errorf("agentexec: delegated task output schema: %w", err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: name, Description: delegation.Description,
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

// runtimeContractSchema preserves Runtime's established jsonschema tag
// vocabulary at the Agent Framework ACL. Agent Framework accepts the resulting neutral JSON
// Schema and remains independent of Runtime's tool-contract implementation.
func runtimeContractSchema[T any]() (agent.Schema, error) {
	raw, err := toolcontract.Schema[T]()
	if err != nil {
		return agent.Schema{}, err
	}
	return agent.ParseSchema([]byte(raw))
}

func (definition *delegatedInteractionDefinition) Descriptor() agent.Descriptor {
	if definition == nil {
		return agent.Descriptor{}
	}
	return definition.descriptor
}

func (definition *delegatedInteractionDefinition) Start(input agent.Input) (agent.Execution, error) {
	if definition == nil || definition.inner == nil || !definition.descriptor.Valid() {
		return nil, errors.New("agentexec: delegated Interaction definition is invalid")
	}
	task, err := agent.DecodeInput[delegation.Input](input)
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode delegated task: %w", err)
	}
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("agentexec: invalid delegated task: %w", err)
	}
	messages := cloneChatMessages(definition.instructions)
	messages = append(messages, corechat.NewUserMessage(corechat.NewTextPart(task.Instructions)))
	adapted, err := agent.EncodeInput(interaction.Input{Messages: messages})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode delegated Interaction input: %w", err)
	}
	execution, err := definition.inner.Start(adapted)
	if err != nil {
		return nil, err
	}
	return &delegatedInteractionExecution{inner: execution}, nil
}

func (definition *delegatedInteractionDefinition) Restore(
	state agent.ExecutionState,
) (agent.Execution, error) {
	if definition == nil || definition.inner == nil || !definition.descriptor.Valid() {
		return nil, errors.New("agentexec: delegated Interaction definition is invalid")
	}
	execution, err := definition.inner.Restore(state)
	if err != nil {
		return nil, err
	}
	return &delegatedInteractionExecution{inner: execution}, nil
}

type delegatedInteractionExecution struct{ inner agent.Execution }

func (execution *delegatedInteractionExecution) Step(
	ctx context.Context,
	signals []agent.Signal,
) (agent.Transition, error) {
	if execution == nil || execution.inner == nil {
		return agent.Transition{}, errors.New("agentexec: delegated Interaction execution is invalid")
	}
	transition, err := execution.inner.Step(ctx, signals)
	if err != nil || transition.Kind() != agent.TransitionKindComplete {
		return transition, err
	}
	erased, _ := transition.Output()
	output, err := agent.DecodeOutput[interaction.Output](erased)
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

func (execution *delegatedInteractionExecution) Snapshot() (agent.ExecutionState, error) {
	if execution == nil || execution.inner == nil {
		return agent.ExecutionState{}, errors.New("agentexec: delegated Interaction execution is invalid")
	}
	return execution.inner.Snapshot()
}

func delegatedInteractionReply(output interaction.Output) (string, error) {
	if err := output.Validate(); err != nil {
		return "", err
	}
	switch output.Source {
	case interaction.CompletionSourceModelResponse:
		choice := output.ModelResponse.First()
		if choice == nil || choice.Message == nil || choice.Message.Text() == "" {
			return "", errors.New("agentexec: delegated Interaction completed without a textual answer")
		}
		return choice.Message.Text(), nil
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
