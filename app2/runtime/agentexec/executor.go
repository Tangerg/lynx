// Package agentexec is the only app2 boundary that depends on the Agent
// Framework. It translates product input into an Interaction deployment and
// returns semantic output without leaking framework snapshots or identities.
package agentexec

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type ClientResolver interface {
	ResolveClient(context.Context, string, string) (*chatclient.Client, error)
}

type ToolCatalog interface {
	ForRun(context.Context, ToolScope) ([]ExecutableTool, error)
}

// ToolFactSink receives committed domain facts that cannot be reconstructed
// from a provider-facing string result. The executor owns its lifetime and
// attaches each fact to the exact observed ToolCall.
type ToolFactSink interface {
	RecordCommittedPlan(string, protocol.Plan)
}

type ToolScope struct {
	SessionID string
	RunID     string
	Workspace string
	IsRootRun bool
	Facts     ToolFactSink
}

type Executor struct {
	clients ClientResolver
	tools   ToolCatalog
}

func New(clients ClientResolver, tools ToolCatalog) (*Executor, error) {
	if clients == nil {
		return nil, errors.New("agentexec: client resolver is required")
	}
	return &Executor{clients: clients, tools: tools}, nil
}

type Input struct {
	Provider, Model, Workspace string
	SessionID                  string
	RunID                      string
	IsRootRun                  bool
	Conversation               []Message
	MaxSteps                   int
	Steers                     <-chan Steer
	Live                       LiveObservationSink
}

type ResumeInput struct {
	Provider, Model, Workspace string
	SessionID, RunID           string
	IsRootRun                  bool
	MaxSteps                   int
	Checkpoint                 json.RawMessage
	Response                   json.RawMessage
	AdditionalInput            []protocol.ContentBlock
	Steers                     <-chan Steer
	Live                       LiveObservationSink
}

type Message = chat.Message

// Steer is a one-shot ownership transfer from the Run application service to
// the live Agent process. Result is completed only after the Framework accepts
// or conclusively rejects the signal.
type Steer struct {
	Input  []protocol.ContentBlock
	Result chan<- error
}

var steerSequence atomic.Uint64

type Output struct {
	Text       string
	Usage      protocol.Usage
	ModelCalls int
	ContextTokens int64
	Models     []ModelObservation
	Tools      []ToolObservation
	Waiting    *Waiting
}

type Waiting struct {
	Prompt         json.RawMessage
	ResponseSchema json.RawMessage
	Checkpoint     json.RawMessage
}

// ToolInputPrompt is the private mechanism contract shared by app2 tool
// adapters and the executor boundary. It is never exposed directly on Lyra's
// wire; runflow projects it into the canonical Item/Interrupt union.
type ToolInputPrompt struct {
	Kind         string                 `json:"kind"`
	ItemID       string                 `json:"itemId"`
	Tool         *ToolInputInvocation   `json:"tool,omitempty"`
	SafetyClass  protocol.SafetyClass   `json:"safetyClass,omitempty"`
	Risk         protocol.ApprovalRisk  `json:"risk,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
	Rememberable bool                   `json:"rememberable,omitempty"`
	Question     *protocol.Question     `json:"question,omitempty"`
}

type ToolInputInvocation struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func (executor *Executor) Execute(ctx context.Context, input Input) (Output, error) {
	deployment, observer, err := executor.deployment(ctx, input.Provider, input.Model, input.SessionID, input.RunID, input.Workspace, input.IsRootRun, input.MaxSteps, input.Live)
	if err != nil {
		return Output{}, err
	}
	engine, err := agent.NewEngine(engineConfig(observer))
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: create engine: %w", err)
	}
	messages, err := materializeConversation(input.Conversation)
	if err != nil {
		_ = engine.Close()
		return Output{}, err
	}
	encoded, err := agent.EncodeInput(interaction.Input{Messages: messages})
	if err != nil {
		_ = engine.Close()
		return Output{}, fmt.Errorf("agentexec: encode input: %w", err)
	}
	process, err := engine.Start(ctx, deployment, encoded)
	if err != nil {
		_ = engine.Close()
		return Output{}, fmt.Errorf("agentexec: start interaction: %w", err)
	}
	return awaitProcess(ctx, engine, process, observer, input.RunID, input.Steers)
}

func (executor *Executor) Resume(ctx context.Context, input ResumeInput) (Output, error) {
	deployment, observer, err := executor.deployment(ctx, input.Provider, input.Model, input.SessionID, input.RunID, input.Workspace, input.IsRootRun, input.MaxSteps, input.Live)
	if err != nil {
		return Output{}, err
	}
	snapshot, err := agent.ParseSnapshot(input.Checkpoint)
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: parse checkpoint: %w", err)
	}
	engine, err := agent.NewEngine(engineConfig(observer))
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: create resume engine: %w", err)
	}
	process, err := engine.Restore(ctx, deployment, snapshot)
	if err != nil {
		_ = engine.Close()
		return Output{}, fmt.Errorf("agentexec: restore interaction: %w", err)
	}
	pending, found, err := interaction.PendingToolInputFromProcess(ctx, process)
	if err != nil || !found {
		_ = stopProcess(engine, process)
		if err != nil {
			return Output{}, fmt.Errorf("agentexec: inspect restored input: %w", err)
		}
		return Output{}, errors.New("agentexec: restored checkpoint has no pending tool input")
	}
	signalID, err := agent.ParseSignalID(fmt.Sprintf("resume:%s:%d", input.RunID, steerSequence.Add(1)))
	if err != nil {
		_ = stopProcess(engine, process)
		return Output{}, err
	}
	signal, err := pending.ResponseSignal(signalID, input.Response)
	if err != nil {
		_ = stopProcess(engine, process)
		return Output{}, fmt.Errorf("agentexec: construct resume signal: %w", err)
	}
	accepted := false
	if len(input.AdditionalInput) > 0 {
		steer, steerErr := newSteerSignal(input.RunID, input.AdditionalInput)
		if steerErr != nil { _ = stopProcess(engine, process); return Output{}, steerErr }
		accepted, err = process.DeliverSignals(ctx, signal, steer)
	} else {
		accepted, err = process.DeliverSignal(ctx, signal)
	}
	if err != nil || !accepted {
		_ = stopProcess(engine, process)
		if err != nil {
			return Output{}, fmt.Errorf("agentexec: deliver resume signal: %w", err)
		}
		return Output{}, errors.New("agentexec: resume signal was not accepted")
	}
	return awaitProcess(ctx, engine, process, observer, input.RunID, input.Steers)
}

func (executor *Executor) deployment(ctx context.Context, provider, model, sessionID, runID, workspace string, rootRun bool, maxSteps int, live LiveObservationSink) (agent.Deployment, *executionObserver, error) {
	client, err := executor.clients.ResolveClient(ctx, provider, model)
	if err != nil {
		return agent.Deployment{}, nil, err
	}
	observer := newExecutionObserver(runID, model, live)
	bindings := []ExecutableTool{}
	if executor.tools != nil {
		bindings, err = executor.tools.ForRun(ctx, ToolScope{SessionID: sessionID, RunID: runID, Workspace: workspace, IsRootRun: rootRun, Facts: observer})
		if err != nil {
			return agent.Deployment{}, nil, fmt.Errorf("agentexec: resolve tools: %w", err)
		}
	}
	observer.bindTools(bindings)
	executables := make([]tool.Tool, len(bindings))
	for index, binding := range bindings {
		executables[index] = binding.Tool
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "lyra.interaction", Description: "Complete the user's request using the available tools.",
		Version: "2.0.0", MaxModelCalls: maxModelCalls(maxSteps),
	})
	if err != nil {
		return agent.Deployment{}, nil, fmt.Errorf("agentexec: define interaction: %w", err)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{Client: client, Tools: executables, StreamModelResponses: true, Observer: observer})
	if err != nil {
		return agent.Deployment{}, nil, fmt.Errorf("agentexec: create dispatcher: %w", err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("lyra-app2-interaction-v2")),
		ConfigurationDigest: agent.ComputeDigest([]byte(provider + "\x00" + model + "\x00" + workspace)),
	})
	if err != nil {
		return agent.Deployment{}, nil, fmt.Errorf("agentexec: create deployment: %w", err)
	}
	return deployment, observer, nil
}

func engineConfig(observer *executionObserver) agent.EngineConfig {
	if observer == nil || observer.live == nil {
		return agent.EngineConfig{}
	}
	return agent.EngineConfig{DeltaListeners: []agent.DeltaListener{observer}}
}

func awaitProcess(ctx context.Context, engine *agent.Engine, process *agent.Process, observer *executionObserver, runID string, steers <-chan Steer) (Output, error) {
	resultCh := make(chan agent.Result, 1)
	go func() {
		result, _ := process.Await(context.WithoutCancel(ctx))
		resultCh <- result
	}()
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	var result agent.Result
	for {
		select {
		case result = <-resultCh:
			goto completed
		case command := <-steers:
			command.Result <- deliverSteer(ctx, process, runID, command.Input)
		case <-ticker.C:
			if process.Status() != agent.StatusWaiting {
				continue
			}
			pending, found, err := interaction.PendingToolInputFromProcess(context.WithoutCancel(ctx), process)
			if err != nil || !found {
				_ = stopProcess(engine, process)
				if err != nil {
					return Output{}, fmt.Errorf("agentexec: inspect pending input: %w", err)
				}
				return Output{}, errors.New("agentexec: waiting interaction has no tool input")
			}
			snapshot, err := process.Capture(context.WithoutCancel(ctx))
			if err != nil {
				_ = stopProcess(engine, process)
				return Output{}, fmt.Errorf("agentexec: capture waiting interaction: %w", err)
			}
			waiting := &Waiting{Prompt: pending.Prompt(), ResponseSchema: pending.ResponseSchema(), Checkpoint: snapshot.JSON()}
			models, tools, usage, contextTokens := observer.snapshot()
			partial := Output{Waiting: waiting, Models: models, Tools: tools, Usage: usage, ModelCalls: len(models), ContextTokens: contextTokens}
			if err := stopProcess(engine, process); err != nil {
				return partial, fmt.Errorf("agentexec: release checkpointed interaction: %w", err)
			}
			return partial, nil
		}
	}

completed:
	models, tools, usage, contextTokens := observer.snapshot()
	partial := Output{Usage: usage, ModelCalls: len(models), Models: models, Tools: tools, ContextTokens: contextTokens}
	if err := engine.Close(); err != nil {
		return partial, fmt.Errorf("agentexec: close completed engine: %w", err)
	}
	erased, ok := result.Output()
	if !ok {
		return partial, fmt.Errorf("agentexec: interaction ended with %s", result.Status())
	}
	decoded, err := agent.DecodeOutput[interaction.Output](erased)
	if err != nil {
		return partial, fmt.Errorf("agentexec: decode output: %w", err)
	}
	if decoded.ModelResponse == nil { return partial, errors.New("agentexec: interaction produced no model response") }
	partial.Text = decoded.ModelResponse.Text()
	return partial, nil
}

func stopProcess(engine *agent.Engine, process *agent.Process) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	killErr := process.Kill(ctx, "checkpointed at an external input boundary")
	_, awaitErr := process.Await(ctx)
	return errors.Join(killErr, awaitErr, engine.Close())
}

func materializeConversation(values []Message) ([]chat.Message, error) {
	if len(values) == 0 {
		return nil, errors.New("agentexec: conversation is empty")
	}
	messages := make([]chat.Message, 0, len(values))
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("agentexec: conversation message %d: %w", index, err)
		}
		messages = append(messages, value.Clone())
	}
	return messages, nil
}

func UserMessage(blocks []protocol.ContentBlock) (chat.Message, error) {
	parts, err := contentParts(blocks)
	if err != nil { return chat.Message{}, err }
	message := chat.NewUserMessage(parts...)
	return message, message.Validate()
}

func deliverSteer(ctx context.Context, process *agent.Process, runID string, blocks []protocol.ContentBlock) error {
	signal, err := newSteerSignal(runID, blocks)
	if err != nil { return err }
	accepted, err := process.DeliverSignal(ctx, signal)
	if err != nil {
		return fmt.Errorf("agentexec: deliver steer: %w", err)
	}
	if !accepted {
		return errors.New("agentexec: duplicate steer signal")
	}
	return nil
}

func newSteerSignal(runID string, blocks []protocol.ContentBlock) (agent.SignalRequest, error) {
	parts, err := contentParts(blocks)
	if err != nil {
		return agent.SignalRequest{}, err
	}
	id, err := agent.ParseSignalID(fmt.Sprintf("steer:%s:%d", runID, steerSequence.Add(1)))
	if err != nil {
		return agent.SignalRequest{}, fmt.Errorf("agentexec: create steer identity: %w", err)
	}
	signal, err := interaction.NewSteerSignal(id, chat.NewUserMessage(parts...))
	if err != nil {
		return agent.SignalRequest{}, fmt.Errorf("agentexec: create steer signal: %w", err)
	}
	return signal, nil
}

func maxModelCalls(limit int) uint32 {
	if limit <= 0 {
		return 64
	}
	return uint32(limit)
}

func contentParts(blocks []protocol.ContentBlock) ([]chat.Part, error) {
	parts := make([]chat.Part, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case protocol.ContentBlockText:
			parts = append(parts, chat.NewTextPart(block.Text))
		case protocol.ContentBlockImage:
			data, err := base64.StdEncoding.DecodeString(block.Data)
			if err != nil {
				return nil, fmt.Errorf("agentexec: decode image: %w", err)
			}
			value, err := media.NewBytes(block.Mime, data)
			if err != nil {
				return nil, fmt.Errorf("agentexec: image: %w", err)
			}
			parts = append(parts, chat.NewMediaPart(value))
		default:
			return nil, fmt.Errorf("agentexec: unsupported content type %q", block.Type)
		}
	}
	return parts, nil
}

func presentUsage(value chat.Usage) protocol.Usage {
	usage := protocol.Usage{ModelUsage: protocol.ModelUsage{
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
	}}
	if value.ReasoningTokens != nil {
		usage.ReasoningTokens = *value.ReasoningTokens
	}
	if value.CacheReadInputTokens != nil {
		usage.CacheReadTokens = *value.CacheReadInputTokens
	}
	if value.CacheWriteInputTokens != nil {
		usage.CacheWriteTokens = *value.CacheWriteInputTokens
	}
	return usage
}
