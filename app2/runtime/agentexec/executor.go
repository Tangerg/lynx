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
	"sort"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app2/runtime/domain/lifecyclehook"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type ClientResolver interface {
	ResolveClient(context.Context, string, string) (*chatclient.Client, error)
}

type ToolCatalog interface {
	ForRun(context.Context, ToolScope) ([]ExecutableTool, error)
}

// AgentDocumentSource is the consumer-owned view of Runtime-authored workspace
// guidance. The executor receives content only; discovery and filesystem
// conventions remain owned by capabilityflow.
type AgentDocumentSource interface {
	Documents(context.Context, string) ([]AgentDocument, error)
}

type AgentDocument struct {
	Path    string
	Content string
}

type KnowledgeDocumentSource interface {
	Knowledge(context.Context, string) ([]KnowledgeDocument, error)
}

type KnowledgeDocument struct {
	Path    string
	Content string
}

type MemorySource interface {
	Recall(context.Context, string) ([]MemoryItem, error)
}

type LifecycleHooks interface {
	Evaluate(context.Context, lifecyclehook.Invocation) (lifecyclehook.Decision, error)
}

type MemoryItem struct {
	ID      string
	Scope   string
	Content string
}

// ToolFactSink receives committed domain facts that cannot be reconstructed
// from a provider-facing string result. The executor owns its lifetime and
// attaches each fact to the exact observed ToolCall.
type ToolFactSink interface {
	RecordCommittedPlan(string, protocol.Plan)
	RecordEffectiveToolArguments(string, map[string]any)
}

type ToolScope struct {
	SessionID string
	RunID     string
	Workspace string
	IsRootRun bool
	Facts     ToolFactSink
}

type Executor struct {
	clients   ClientResolver
	tools     ToolCatalog
	documents AgentDocumentSource
	knowledge KnowledgeDocumentSource
	memory    MemorySource
	hooks     LifecycleHooks
}

type Config struct {
	Clients   ClientResolver
	Tools     ToolCatalog
	Documents AgentDocumentSource
	Knowledge KnowledgeDocumentSource
	Memory    MemorySource
	Hooks     LifecycleHooks
}

func New(config Config) (*Executor, error) {
	if config.Clients == nil || config.Documents == nil || config.Knowledge == nil ||
		config.Memory == nil || config.Hooks == nil {
		return nil, errors.New("agentexec: clients, context sources, memory and lifecycle hooks are required")
	}
	return &Executor{
		clients: config.Clients, tools: config.Tools,
		documents: config.Documents, knowledge: config.Knowledge,
		memory: config.Memory, hooks: config.Hooks,
	}, nil
}

type Input struct {
	Provider, Model, Workspace string
	SessionID                  string
	RunID                      string
	SegmentID                  string
	IsRootRun                  bool
	SessionStart               bool
	UserPromptSubmit           bool
	Subagents                  bool
	Delegation                 DelegationCoordinator
	Conversation               []Message
	MaxSteps                   int
	Steers                     <-chan Steer
	Cancels                    <-chan Cancel
	Live                       LiveObservationSink
}

type ResumeInput struct {
	Provider, Model, Workspace string
	SessionID, RunID, SegmentID string
	IsRootRun                  bool
	Subagents                  bool
	Delegation                 DelegationCoordinator
	MaxSteps                   int
	Checkpoint                 json.RawMessage
	Response                   json.RawMessage
	TreeMembers                []TreeResumeMember
	TreeResponses              []TreeResumeResponse
	ContinueTree               bool
	AdditionalInput            []protocol.ContentBlock
	Steers                     <-chan Steer
	Cancels                    <-chan Cancel
	Live                       LiveObservationSink
}

// TreeResumeMember rebinds one non-terminal Framework member to the exact
// product Run generation opened by runs.resume. MemberID is opaque outside the
// adapter; the root leaves it empty because the checkpoint owns that identity.
type TreeResumeMember struct {
	MemberID                                  string
	RunID, SegmentID, ParentRunID, RootRunID string
	Depth                                     uint32
}

// TreeResumeResponse is one already-validated answer addressed by source Run.
// A complete Framework tree can expose at most one external input per member.
type TreeResumeResponse struct {
	RunID   string
	Payload json.RawMessage
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
	Status     ExecutionStatus
	Detail     string
	Usage      protocol.Usage
	ModelCalls int
	ContextTokens int64
	Models     []ModelObservation
	Tools      []ToolObservation
	Waiting    *Waiting
	Children   []ChildOutput
}

type ExecutionStatus string

const (
	ExecutionCompleted ExecutionStatus = "completed"
	ExecutionFailed    ExecutionStatus = "failed"
	ExecutionCanceled  ExecutionStatus = "canceled"
	ExecutionTimedOut  ExecutionStatus = "timed_out"
	ExecutionKilled    ExecutionStatus = "killed"
)

// ChildStatus is the adapter-owned terminal vocabulary exposed to the Run
// application layer. Framework lifecycle enums do not cross this boundary.
type ChildStatus string

const (
	ChildCompleted ChildStatus = "completed"
	ChildFailed    ChildStatus = "failed"
	ChildCanceled  ChildStatus = "canceled"
	ChildTimedOut  ChildStatus = "timed_out"
	ChildKilled    ChildStatus = "killed"
)

// ChildOutput is the settled, source-owned material for one managed child.
// Depth-descending order lets the application commit descendants before the
// parent whose Delegate result depends on them.
type ChildOutput struct {
	RunID, SegmentID, ParentRunID, RootRunID string
	Depth                                    uint32
	Status                                   ChildStatus
	Detail, Reply                            string
	StartedAt, FinishedAt                    time.Time
	Usage                                    protocol.Usage
	ModelCalls                               int
	ContextTokens                            int64
	Models                                   []ModelObservation
	Tools                                    []ToolObservation
}

// Cancel is an exact live control-plane request. The adapter acknowledges only
// after Framework accepted the intent; child cancellation additionally returns
// authoritative subtree terminal material in descendant-first order.
type Cancel struct {
	RunID  string
	Reason string
	Result chan<- CancelResult
}

type CancelResult struct {
	Children []ChildOutput
	Err      error
}

// ErrWaitingTreeCancelUnavailable reports a checkpoint whose exact target
// cannot be transformed without guessing Framework state.
var ErrWaitingTreeCancelUnavailable = errors.New("agentexec: waiting tree cancel is unavailable")

// WaitingTreeMember binds one durable open Run to its opaque Framework member.
type WaitingTreeMember struct {
	MemberID                         string
	RunID, ParentRunID, RootRunID   string
	Depth                            uint32
}

// WaitingTreeCancelInput describes one side-effect-free checkpoint transform.
type WaitingTreeCancelInput struct {
	Provider, Model, Workspace string
	SessionID, RootRunID       string
	TargetRunID, Reason        string
	MaxSteps                   int
	Checkpoint                 json.RawMessage
	Members                    []WaitingTreeMember
	Delegation                 DelegationCoordinator
}

// WaitingTreeCancelOutput returns the replacement checkpoint and the exact
// product subtree proven canceled by Framework.
type WaitingTreeCancelOutput struct {
	Checkpoint     json.RawMessage
	CanceledRunIDs []string
}

type Waiting struct {
	Prompt         json.RawMessage
	ResponseSchema json.RawMessage
	Checkpoint     json.RawMessage
	Tree           bool
	Runs           []WaitingRun
}

type WaitingDisposition string

const (
	WaitingInterrupt WaitingDisposition = "interrupt"
	WaitingSuspended WaitingDisposition = "suspended"
)

// WaitingRun is one non-terminal product Run captured at a complete tree
// checkpoint. Interrupt material belongs only to the exact source Run; every
// other member closes its current Segment as suspended.
type WaitingRun struct {
	RunID, SegmentID, ParentRunID, RootRunID string
	Depth                                    uint32
	Disposition                              WaitingDisposition
	Prompt, ResponseSchema                   json.RawMessage
	Usage                                    protocol.Usage
	ModelCalls                               int
	ContextTokens                            int64
	Models                                   []ModelObservation
	Tools                                    []ToolObservation
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

// ErrAdmissionRejected marks a lifecycle-policy denial before a fresh root
// interaction starts. runflow projects it as an invalid-request outcome.
var ErrAdmissionRejected = errors.New("agentexec: interaction admission rejected by lifecycle policy")

func (executor *Executor) Execute(ctx context.Context, input Input) (Output, error) {
	messages, err := materializeConversation(input.Conversation)
	if err != nil {
		return Output{}, err
	}
	if input.IsRootRun {
		if input.SessionStart || input.UserPromptSubmit {
			messages, err = executor.applyPromptHooks(ctx, input, messages)
			if err != nil {
				return Output{}, err
			}
		}
		memory, err := executor.memory.Recall(ctx, input.Workspace)
		if err != nil {
			return Output{}, fmt.Errorf("agentexec: recall memory: %w", err)
		}
		memoryGuidance, err := renderMemory(memory)
		if err != nil {
			return Output{}, err
		}
		knowledge, err := executor.knowledge.Knowledge(ctx, input.Workspace)
		if err != nil {
			return Output{}, fmt.Errorf("agentexec: load knowledge: %w", err)
		}
		knowledgeGuidance, err := renderKnowledgeDocuments(knowledge)
		if err != nil {
			return Output{}, err
		}
		documents, err := executor.documents.Documents(ctx, input.Workspace)
		if err != nil {
			return Output{}, fmt.Errorf("agentexec: load agent documents: %w", err)
		}
		guidance, err := renderAgentDocuments(documents)
		if err != nil {
			return Output{}, err
		}
		workingContext := strings.Join(
			slices.DeleteFunc([]string{memoryGuidance, knowledgeGuidance, guidance}, func(value string) bool {
				return value == ""
			}),
			"\n\n",
		)
		if workingContext != "" {
			messages = append(
				[]chat.Message{chat.NewSystemMessage(workingContext)}, messages...,
			)
		}
	}
	prepared, err := executor.deployment(ctx, deploymentRequest{
		provider: input.Provider, model: input.Model, sessionID: input.SessionID,
		runID: input.RunID, segmentID: input.SegmentID, workspace: input.Workspace,
		rootRun: input.IsRootRun, subagents: input.Subagents, maxSteps: input.MaxSteps,
		delegation: input.Delegation, live: input.Live,
	})
	if err != nil {
		return Output{}, err
	}
	engine, err := agent.NewEngine(prepared.engine)
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: create engine: %w", err)
	}
	encoded, err := agent.EncodeInput(interaction.Input{Messages: messages})
	if err != nil {
		_ = engine.Close()
		return Output{}, fmt.Errorf("agentexec: encode input: %w", err)
	}
	process, err := engine.Start(ctx, prepared.root, encoded)
	if err != nil {
		_ = engine.Close()
		return Output{}, fmt.Errorf("agentexec: start interaction: %w", err)
	}
	return awaitProcess(ctx, engine, process, prepared.observer, input.RunID, input.Steers, input.Cancels)
}

func (executor *Executor) applyPromptHooks(
	ctx context.Context,
	input Input,
	messages []chat.Message,
) ([]chat.Message, error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != chat.RoleUser {
		return nil, errors.New("agentexec: fresh root conversation must end with user input")
	}
	invocations := make([]lifecyclehook.Invocation, 0, 2)
	if input.SessionStart {
		invocations = append(invocations, lifecyclehook.Invocation{
			Event: lifecyclehook.SessionStart,
			SessionID: input.SessionID, RunID: input.RunID,
			Workspace: input.Workspace,
		})
	}
	if input.UserPromptSubmit {
		prompt, truncated := lifecycleText(
			messages[len(messages)-1].Text(),
			lifecyclehook.MaxPromptBytes,
		)
		invocations = append(invocations, lifecyclehook.Invocation{
			Event: lifecyclehook.UserPromptSubmit,
			SessionID: input.SessionID, RunID: input.RunID,
			Workspace: input.Workspace, Prompt: prompt, PromptTruncated: truncated,
		})
	}
	contexts := make([]lifecyclehook.Context, 0)
	for _, invocation := range invocations {
		decision, err := executor.hooks.Evaluate(ctx, invocation)
		if err != nil {
			return nil, fmt.Errorf("agentexec: evaluate %s hook: %w", invocation.Event, err)
		}
		if decision.Denied() {
			reason := strings.TrimSpace(decision.Reason)
			if reason == "" {
				reason = "blocked by lifecycle policy"
			}
			return nil, fmt.Errorf("%w: %s", ErrAdmissionRejected, reason)
		}
		contexts = append(contexts, decision.Contexts...)
	}
	if len(contexts) == 0 {
		return messages, nil
	}
	contextMessage := chat.NewSystemMessage(RenderLifecycleContext(contexts))
	result := make([]chat.Message, 0, len(messages)+1)
	result = append(result, messages[:len(messages)-1]...)
	result = append(result, contextMessage, messages[len(messages)-1])
	return result, nil
}

// RenderLifecycleContext preserves source provenance while turning trusted
// hook material into one model-visible context block.
func RenderLifecycleContext(contexts []lifecyclehook.Context) string {
	if len(contexts) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString("Trusted Lyra lifecycle context:\n")
	for _, value := range contexts {
		fmt.Fprintf(
			&result,
			"\n[event=%s source=%q]\n%s\n[end lifecycle context]\n",
			value.Event,
			value.Source,
			value.Content,
		)
	}
	return strings.TrimSpace(result.String())
}

func lifecycleText(value string, limit int) (string, bool) {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func (executor *Executor) Resume(ctx context.Context, input ResumeInput) (Output, error) {
	if input.Subagents {
		return executor.resumeTree(ctx, input)
	}
	prepared, err := executor.deployment(ctx, deploymentRequest{
		provider: input.Provider, model: input.Model, sessionID: input.SessionID,
		runID: input.RunID, segmentID: input.SegmentID, workspace: input.Workspace,
		rootRun: input.IsRootRun, maxSteps: input.MaxSteps, live: input.Live,
	})
	if err != nil {
		return Output{}, err
	}
	snapshot, err := agent.ParseSnapshot(input.Checkpoint)
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: parse checkpoint: %w", err)
	}
	engine, err := agent.NewEngine(prepared.engine)
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: create resume engine: %w", err)
	}
	process, err := engine.Restore(ctx, prepared.root, snapshot)
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
	return awaitProcess(ctx, engine, process, prepared.observer, input.RunID, input.Steers, input.Cancels)
}

// CancelWaitingTree projects Framework's exact Strategy-safe subtree
// cancellation into a replacement checkpoint. The ephemeral restored Engine is
// discarded; only the returned opaque snapshot may cross the application
// transaction boundary.
func (executor *Executor) CancelWaitingTree(
	ctx context.Context,
	input WaitingTreeCancelInput,
) (WaitingTreeCancelOutput, error) {
	const waitingGeneration = "waiting-tree-cancel"
	prepared, err := executor.deployment(ctx, deploymentRequest{
		provider: input.Provider, model: input.Model, sessionID: input.SessionID,
		runID: input.RootRunID, segmentID: waitingGeneration, workspace: input.Workspace,
		rootRun: true, subagents: true, maxSteps: input.MaxSteps,
		delegation: input.Delegation,
	})
	if err != nil {
		return WaitingTreeCancelOutput{}, err
	}
	tree, err := agent.ParseTreeSnapshot(input.Checkpoint)
	if err != nil {
		return WaitingTreeCancelOutput{}, fmt.Errorf("agentexec: parse waiting tree checkpoint: %w", err)
	}
	bindings := make([]TreeResumeMember, 0, len(input.Members))
	for _, member := range input.Members {
		bindings = append(bindings, TreeResumeMember{
			MemberID: member.MemberID, RunID: member.RunID, SegmentID: waitingGeneration,
			ParentRunID: member.ParentRunID, RootRunID: member.RootRunID, Depth: member.Depth,
		})
	}
	if prepared.observer.delegation == nil {
		return WaitingTreeCancelOutput{}, errors.New("agentexec: waiting tree cancel has no delegation bridge")
	}
	if err := prepared.observer.delegation.restoreBindings(tree, bindings); err != nil {
		return WaitingTreeCancelOutput{}, err
	}
	engine, err := agent.NewEngine(prepared.engine)
	if err != nil {
		return WaitingTreeCancelOutput{}, fmt.Errorf("agentexec: create waiting tree cancel engine: %w", err)
	}
	root, err := engine.RestoreTree(ctx, prepared.root, tree)
	if err != nil {
		_ = engine.Close()
		return WaitingTreeCancelOutput{}, fmt.Errorf("agentexec: restore waiting tree for cancel: %w", err)
	}
	targetID, found := prepared.observer.delegation.processForRun(input.TargetRunID)
	if !found {
		_ = stopProcess(engine, root)
		return WaitingTreeCancelOutput{}, ErrWaitingTreeCancelUnavailable
	}
	target, found := engine.Process(targetID)
	if !found || target.Status() != agent.StatusWaiting {
		_ = stopProcess(engine, root)
		return WaitingTreeCancelOutput{}, ErrWaitingTreeCancelUnavailable
	}
	projected, err := engine.PrepareWaitingSubtreeCancellation(
		ctx, root.ID(), targetID, input.Reason,
	)
	if err != nil {
		_ = stopProcess(engine, root)
		if errors.Is(err, agent.ErrWaitingSubtreeCancellationUnavailable) {
			return WaitingTreeCancelOutput{}, ErrWaitingTreeCancelUnavailable
		}
		return WaitingTreeCancelOutput{}, fmt.Errorf("agentexec: project waiting subtree cancel: %w", err)
	}
	resulting := projected.ResultingSnapshot()
	canceled := make([]string, 0, len(projected.CanceledProcessIDs()))
	for _, processID := range projected.CanceledProcessIDs() {
		binding, found := prepared.observer.delegation.bindingProcess(processID)
		if !found || binding.runID == input.RootRunID {
			_ = projected.Discard()
			_ = stopProcess(engine, root)
			return WaitingTreeCancelOutput{}, errors.New("agentexec: canceled Process has no child Run binding")
		}
		canceled = append(canceled, binding.runID)
	}
	if err := projected.Discard(); err != nil {
		_ = stopProcess(engine, root)
		return WaitingTreeCancelOutput{}, fmt.Errorf("agentexec: discard projected waiting cancel engine: %w", err)
	}
	if err := stopProcess(engine, root); err != nil {
		return WaitingTreeCancelOutput{}, fmt.Errorf("agentexec: release waiting cancel probe: %w", err)
	}
	return WaitingTreeCancelOutput{Checkpoint: resulting.JSON(), CanceledRunIDs: canceled}, nil
}

func (executor *Executor) resumeTree(ctx context.Context, input ResumeInput) (Output, error) {
	prepared, err := executor.deployment(ctx, deploymentRequest{
		provider: input.Provider, model: input.Model, sessionID: input.SessionID,
		runID: input.RunID, segmentID: input.SegmentID, workspace: input.Workspace,
		rootRun: input.IsRootRun, subagents: true, maxSteps: input.MaxSteps,
		delegation: input.Delegation, live: input.Live,
	})
	if err != nil {
		return Output{}, err
	}
	tree, err := agent.ParseTreeSnapshot(input.Checkpoint)
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: parse tree checkpoint: %w", err)
	}
	if prepared.observer.delegation == nil {
		return Output{}, errors.New("agentexec: tree resume has no delegation bridge")
	}
	if err := prepared.observer.delegation.restoreBindings(tree, input.TreeMembers); err != nil {
		return Output{}, err
	}
	engine, err := agent.NewEngine(prepared.engine)
	if err != nil {
		return Output{}, fmt.Errorf("agentexec: create tree resume engine: %w", err)
	}
	root, err := engine.RestoreTree(ctx, prepared.root, tree)
	if err != nil {
		_ = engine.Close()
		return Output{}, fmt.Errorf("agentexec: restore interaction tree: %w", err)
	}
	responses := make(map[string]json.RawMessage, len(input.TreeResponses))
	for _, response := range input.TreeResponses {
		if response.RunID == "" || len(response.Payload) == 0 || responses[response.RunID] != nil {
			_ = stopProcess(engine, root)
			return Output{}, errors.New("agentexec: tree response set is invalid")
		}
		responses[response.RunID] = response.Payload
	}
	type preparedResponse struct {
		process *agent.Process
		signal  agent.SignalRequest
	}
	preparedResponses := make([]preparedResponse, 0, len(responses))
	paused := make([]*agent.Process, 0)
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Status() == agent.StatusPaused {
			process, found := engine.Process(snapshot.ProcessID())
			if !found {
				_ = stopProcess(engine, root)
				return Output{}, fmt.Errorf("agentexec: paused Process %s is unavailable", snapshot.ProcessID())
			}
			paused = append(paused, process)
		}
		pending, found, pendingErr := interaction.PendingToolInputFromSnapshot(snapshot)
		if pendingErr != nil {
			_ = stopProcess(engine, root)
			return Output{}, fmt.Errorf("agentexec: inspect restored Process %s input: %w", snapshot.ProcessID(), pendingErr)
		}
		if !found {
			continue
		}
		if input.ContinueTree {
			continue
		}
		binding, bound := prepared.observer.delegation.bindingProcess(snapshot.ProcessID())
		payload, answered := responses[binding.runID]
		if !bound || !answered {
			_ = stopProcess(engine, root)
			return Output{}, errors.New("agentexec: restored input has no exact product answer")
		}
		signalID, err := agent.ParseSignalID(fmt.Sprintf("resume:%s:%d", binding.runID, steerSequence.Add(1)))
		if err != nil {
			_ = stopProcess(engine, root)
			return Output{}, err
		}
		signal, err := pending.ResponseSignal(signalID, payload)
		if err != nil {
			_ = stopProcess(engine, root)
			return Output{}, fmt.Errorf("agentexec: construct tree resume signal: %w", err)
		}
		process, available := engine.Process(snapshot.ProcessID())
		if !available {
			_ = stopProcess(engine, root)
			return Output{}, fmt.Errorf("agentexec: answered Process %s is unavailable", snapshot.ProcessID())
		}
		preparedResponses = append(preparedResponses, preparedResponse{process: process, signal: signal})
		delete(responses, binding.runID)
	}
	if input.ContinueTree {
		if len(responses) != 0 || len(input.AdditionalInput) != 0 {
			_ = stopProcess(engine, root)
			return Output{}, errors.New("agentexec: automatic tree continuation carries user input")
		}
	} else if len(responses) != 0 || len(preparedResponses) == 0 {
		_ = stopProcess(engine, root)
		return Output{}, errors.New("agentexec: tree answers do not exactly cover restored inputs")
	}
	if len(input.AdditionalInput) > 0 && len(preparedResponses) != 1 {
		_ = stopProcess(engine, root)
		return Output{}, errors.New("agentexec: additional tree input requires one exact interrupted Process")
	}
	for index, response := range preparedResponses {
		accepted := false
		if len(input.AdditionalInput) > 0 && len(preparedResponses) == 1 {
			steer, steerErr := newSteerSignal(input.RunID, input.AdditionalInput)
			if steerErr != nil {
				_ = stopProcess(engine, root)
				return Output{}, steerErr
			}
			accepted, err = response.process.DeliverSignals(ctx, response.signal, steer)
		} else {
			accepted, err = response.process.DeliverSignal(ctx, response.signal)
		}
		if err != nil || !accepted {
			_ = stopProcess(engine, root)
			if err != nil {
				return Output{}, fmt.Errorf("agentexec: deliver tree resume response %d: %w", index, err)
			}
			return Output{}, errors.New("agentexec: tree resume response was already accepted")
		}
	}
	for _, process := range paused {
		if process.Status() != agent.StatusPaused {
			_ = stopProcess(engine, root)
			return Output{}, fmt.Errorf("agentexec: Process %s left its paused boundary", process.ID())
		}
		if err := process.Resume(ctx); err != nil {
			_ = stopProcess(engine, root)
			return Output{}, fmt.Errorf("agentexec: resume Process %s: %w", process.ID(), err)
		}
	}
	return awaitProcess(ctx, engine, root, prepared.observer, input.RunID, input.Steers, input.Cancels)
}

type deploymentRequest struct {
	provider, model, sessionID, runID, segmentID, workspace string
	rootRun, subagents                                  bool
	maxSteps                                            int
	delegation                                          DelegationCoordinator
	live                                                LiveObservationSink
}

type preparedDeployment struct {
	root     agent.Deployment
	observer *executionObserver
	engine   agent.EngineConfig
}

func (executor *Executor) deployment(ctx context.Context, request deploymentRequest) (preparedDeployment, error) {
	client, err := executor.clients.ResolveClient(ctx, request.provider, request.model)
	if err != nil {
		return preparedDeployment{}, err
	}
	var bridge *delegationBridge
	if request.subagents {
		bridge, err = newDelegationBridge(request.runID, request.segmentID, request.delegation)
		if err != nil {
			return preparedDeployment{}, err
		}
	}
	observer := newExecutionObserver(request.runID, request.segmentID, request.model, request.live, bridge)
	bindings := []ExecutableTool{}
	if executor.tools != nil {
		bindings, err = executor.tools.ForRun(ctx, ToolScope{
			SessionID: request.sessionID, RunID: request.runID, Workspace: request.workspace,
			IsRootRun: request.rootRun, Facts: observer,
		})
		if err != nil {
			return preparedDeployment{}, fmt.Errorf("agentexec: resolve tools: %w", err)
		}
	}
	observer.bindTools(bindings)
	if request.subagents {
		childManifest, manifestErr := executor.childToolManifest(ctx, request.sessionID, request.workspace, observer)
		if manifestErr != nil {
			return preparedDeployment{}, fmt.Errorf("agentexec: resolve delegated Tool manifest: %w", manifestErr)
		}
		observer.bindTools(childManifest)
		router, routerErr := newRunToolRouter(executor.tools, bridge, ToolScope{
			SessionID: request.sessionID, Workspace: request.workspace,
		}, observer, childManifest)
		if routerErr != nil {
			return preparedDeployment{}, routerErr
		}
		family, familyErr := newDeploymentFamily(familyConfig{
			client: client, provider: request.provider, model: request.model, workspace: request.workspace,
			maxModelCalls: maxModelCalls(request.maxSteps), rootTools: bindings,
			childManifest: childManifest, toolRouter: router, observer: observer,
		})
		if familyErr != nil {
			return preparedDeployment{}, familyErr
		}
		bridge.installFamily(family.root.DeploymentRef(), family.targets)
		return preparedDeployment{
			root: family.root, observer: observer,
			engine: agent.EngineConfig{
				DeploymentResolver: family, ProcessAdmitter: bridge,
				ProcessStartOutcomeAcknowledger: bridge,
				DeltaListeners: liveDeltaListeners(observer), TreeLimits: family.limits,
			},
		}, nil
	}
	visible, deferred := partitionToolManifest(bindings)
	toolDigest, err := toolManifestDigest(bindings)
	if err != nil {
		return preparedDeployment{}, err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "lyra.interaction", Description: "Complete the user's request using the available tools.",
		Version: "3.0.0", MaxModelCalls: maxModelCalls(request.maxSteps),
	})
	if err != nil {
		return preparedDeployment{}, fmt.Errorf("agentexec: define interaction: %w", err)
	}
	dispatcher, err := interaction.NewDispatcher(
		definition,
		interaction.DispatcherConfig{
			Client: client, Tools: visible, DeferredTools: deferred,
			StreamModelResponses: true, Observer: observer,
		},
	)
	if err != nil {
		return preparedDeployment{}, fmt.Errorf("agentexec: create dispatcher: %w", err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("lyra-app2-interaction-v3")),
		ConfigurationDigest: agent.ComputeDigest([]byte(
			request.provider + "\x00" + request.model + "\x00" + request.workspace +
				"\x00tools:" + toolDigest.String(),
		)),
	})
	if err != nil {
		return preparedDeployment{}, fmt.Errorf("agentexec: create deployment: %w", err)
	}
	return preparedDeployment{root: deployment, observer: observer, engine: engineConfig(observer)}, nil
}

func engineConfig(observer *executionObserver) agent.EngineConfig {
	return agent.EngineConfig{DeltaListeners: liveDeltaListeners(observer)}
}

func liveDeltaListeners(observer *executionObserver) []agent.DeltaListener {
	if observer == nil || observer.live == nil {
		return nil
	}
	return []agent.DeltaListener{observer}
}

func awaitProcess(
	ctx context.Context,
	engine *agent.Engine,
	process *agent.Process,
	observer *executionObserver,
	runID string,
	steers <-chan Steer,
	cancels <-chan Cancel,
) (Output, error) {
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
		case command := <-cancels:
			if command.Result != nil {
				command.Result <- cancelRunTreeMember(ctx, engine, process, observer, command)
			}
		case <-ticker.C:
			if process.Status() != agent.StatusWaiting {
				continue
			}
			select {
			case command := <-cancels:
				if command.Result != nil {
					command.Result <- cancelRunTreeMember(ctx, engine, process, observer, command)
				}
				continue
			default:
			}
			if observer.delegation != nil {
				waiting, children, err := captureWaitingTree(context.WithoutCancel(ctx), engine, process, observer)
				models, tools, usage, contextTokens := observer.snapshot(runID)
				partial := Output{
					Waiting: waiting, Children: children,
					Models: models, Tools: tools, Usage: usage,
					ModelCalls: len(models), ContextTokens: contextTokens,
				}
				if err != nil {
					_ = stopProcess(engine, process)
					return partial, err
				}
				if err := stopProcess(engine, process); err != nil {
					return partial, fmt.Errorf("agentexec: release checkpointed tree: %w", err)
				}
				return partial, nil
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
			models, tools, usage, contextTokens := observer.snapshot(runID)
			partial := Output{Waiting: waiting, Models: models, Tools: tools, Usage: usage, ModelCalls: len(models), ContextTokens: contextTokens}
			if err := stopProcess(engine, process); err != nil {
				return partial, fmt.Errorf("agentexec: release checkpointed interaction: %w", err)
			}
			return partial, nil
		}
	}

completed:
	models, tools, usage, contextTokens := observer.snapshot(runID)
	partial := Output{Usage: usage, ModelCalls: len(models), Models: models, Tools: tools, ContextTokens: contextTokens}
	children, childErr := collectChildOutputs(ctx, engine, observer, nil)
	partial.Children = children
	if childErr != nil {
		_ = engine.Close()
		return partial, childErr
	}
	if err := engine.Close(); err != nil {
		return partial, fmt.Errorf("agentexec: close completed engine: %w", err)
	}
	partial.Status, partial.Detail = executionResult(result)
	if partial.Status != ExecutionCompleted {
		return partial, nil
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

func cancelRunTreeMember(
	ctx context.Context,
	engine *agent.Engine,
	root *agent.Process,
	observer *executionObserver,
	command Cancel,
) CancelResult {
	if command.RunID == "" || command.Result == nil {
		return CancelResult{Err: errors.New("agentexec: cancel identity is incomplete")}
	}
	if command.RunID == observer.runID {
		if err := root.RequestCancellation(ctx, command.Reason); err != nil &&
			!errors.Is(err, agent.ErrProcessFinished) {
			return CancelResult{Err: fmt.Errorf("agentexec: cancel root Process: %w", err)}
		}
		return CancelResult{}
	}
	if observer.delegation == nil {
		return CancelResult{Err: errors.New("agentexec: cancel target is not in this Run tree")}
	}
	target, found := observer.delegation.processForRun(command.RunID)
	if !found {
		return CancelResult{Err: errors.New("agentexec: cancel target has no live Process")}
	}
	process, found := engine.Process(target)
	if !found {
		return CancelResult{Err: errors.New("agentexec: cancel target Process is unavailable")}
	}
	if err := process.RequestCancellation(ctx, command.Reason); err != nil &&
		!errors.Is(err, agent.ErrProcessFinished) {
		return CancelResult{Err: fmt.Errorf("agentexec: cancel child Process: %w", err)}
	}
	children, err := collectChildOutputs(
		context.WithoutCancel(ctx), engine, observer, observer.delegation.subtree(command.RunID),
	)
	return CancelResult{Children: children, Err: err}
}

func executionResult(result agent.Result) (ExecutionStatus, string) {
	status := ExecutionFailed
	switch result.Status() {
	case agent.StatusCompleted:
		status = ExecutionCompleted
	case agent.StatusFailed:
		status = ExecutionFailed
	case agent.StatusCanceled:
		status = ExecutionCanceled
	case agent.StatusTimedOut:
		status = ExecutionTimedOut
	case agent.StatusKilled:
		status = ExecutionKilled
	}
	detail := ""
	if status != ExecutionCompleted {
		termination := result.Termination()
		detail = termination.Cause().String()
		if termination.Reason() != "" {
			detail += ": " + termination.Reason()
		}
	}
	return status, detail
}

func captureWaitingTree(
	ctx context.Context,
	engine *agent.Engine,
	root *agent.Process,
	observer *executionObserver,
) (*Waiting, []ChildOutput, error) {
	tree, err := parkTreeMembers(ctx, engine, root.ID())
	if err != nil {
		return nil, nil, fmt.Errorf("agentexec: park waiting tree: %w", err)
	}
	terminal := make(map[agent.ProcessID]bool)
	runs := make([]WaitingRun, 0, len(tree.ProcessSnapshots()))
	interrupts := 0
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Status().Terminal() {
			terminal[snapshot.ProcessID()] = true
			continue
		}
		binding, found := observer.delegation.bindingProcess(snapshot.ProcessID())
		if !found {
			return nil, nil, fmt.Errorf("agentexec: checkpointed Process %s has no product Run", snapshot.ProcessID())
		}
		value := WaitingRun{
			RunID: binding.runID, SegmentID: binding.segmentID,
			ParentRunID: binding.parentRunID, RootRunID: binding.rootRunID,
			Depth: binding.depth, Disposition: WaitingSuspended,
		}
		pending, found, pendingErr := interaction.PendingToolInputFromSnapshot(snapshot)
		if pendingErr != nil {
			return nil, nil, fmt.Errorf("agentexec: inspect waiting Process %s: %w", snapshot.ProcessID(), pendingErr)
		}
		if found {
			value.Disposition = WaitingInterrupt
			value.Prompt = pending.Prompt()
			value.ResponseSchema = pending.ResponseSchema()
			interrupts++
		} else if snapshot.Status() != agent.StatusPaused && snapshot.Status() != agent.StatusWaiting {
			return nil, nil, fmt.Errorf("agentexec: checkpointed Process %s remained %s", snapshot.ProcessID(), snapshot.Status())
		}
		value.Models, value.Tools, value.Usage, value.ContextTokens = observer.snapshot(binding.runID)
		value.ModelCalls = len(value.Models)
		runs = append(runs, value)
	}
	if interrupts == 0 {
		return nil, nil, errors.New("agentexec: waiting tree has no external Tool input")
	}
	sort.Slice(runs, func(left, right int) bool {
		if runs[left].Depth != runs[right].Depth {
			return runs[left].Depth > runs[right].Depth
		}
		return runs[left].RunID < runs[right].RunID
	})
	children, err := collectChildOutputs(ctx, engine, observer, terminal)
	if err != nil {
		return nil, children, err
	}
	return &Waiting{Tree: true, Checkpoint: tree.JSON(), Runs: runs}, children, nil
}

func parkTreeMembers(ctx context.Context, engine *agent.Engine, rootID agent.ProcessID) (agent.TreeSnapshot, error) {
	for attempts := 0; attempts < 64; attempts++ {
		tree, err := engine.CaptureTree(ctx, rootID)
		if err != nil {
			return agent.TreeSnapshot{}, err
		}
		running := make([]agent.ProcessID, 0)
		for _, snapshot := range tree.ProcessSnapshots() {
			if snapshot.Status() == agent.StatusRunning {
				running = append(running, snapshot.ProcessID())
			}
		}
		if len(running) == 0 {
			return tree, nil
		}
		for _, processID := range running {
			member, found := engine.Process(processID)
			if !found {
				return agent.TreeSnapshot{}, fmt.Errorf("Process %s disappeared while parking", processID)
			}
			if err := member.Pause(ctx, "checkpointing a waiting Lyra Run tree"); err != nil &&
				!errors.Is(err, agent.ErrProcessFinished) {
				return agent.TreeSnapshot{}, err
			}
		}
	}
	return agent.TreeSnapshot{}, errors.New("agentexec: waiting tree did not reach a stable checkpoint boundary")
}

func collectChildOutputs(
	ctx context.Context,
	engine *agent.Engine,
	observer *executionObserver,
	only map[agent.ProcessID]bool,
) ([]ChildOutput, error) {
	if observer == nil || observer.delegation == nil {
		return nil, nil
	}
	bindings := observer.delegation.children()
	children := make([]ChildOutput, 0, len(bindings))
	for _, child := range bindings {
		if only != nil && !only[child.processID] {
			continue
		}
		process, found := engine.Process(child.processID)
		if !found {
			return children, fmt.Errorf("agentexec: delegated Process %s is unavailable", child.processID)
		}
		result, err := process.Await(context.WithoutCancel(ctx))
		if err != nil {
			return children, fmt.Errorf("agentexec: await delegated Process %s: %w", child.processID, err)
		}
		models, tools, usage, contextTokens := observer.snapshot(child.binding.runID)
		status, statusErr := childStatus(result.Status())
		if statusErr != nil {
			return children, fmt.Errorf("agentexec: project delegated Process %s status: %w", child.processID, statusErr)
		}
		output := ChildOutput{
			RunID: child.binding.runID, SegmentID: child.binding.segmentID,
			ParentRunID: child.binding.parentRunID, RootRunID: child.binding.rootRunID,
			Depth: child.binding.depth, Status: status,
			StartedAt: result.StartedAt(), FinishedAt: result.FinishedAt(),
			Usage: usage, ModelCalls: len(models), ContextTokens: contextTokens,
			Models: models, Tools: tools,
		}
		if status != ChildCompleted {
			termination := result.Termination()
			output.Detail = termination.Cause().String()
			if termination.Reason() != "" {
				output.Detail += ": " + termination.Reason()
			}
		}
		if erased, present := result.Output(); present {
			decoded, decodeErr := agent.DecodeOutput[delegateTaskResult](erased)
			if decodeErr != nil {
				return children, fmt.Errorf("agentexec: decode delegated Process %s: %w", child.processID, decodeErr)
			}
			output.Reply = decoded.Reply
		}
		children = append(children, output)
	}
	return children, nil
}

func childStatus(status agent.Status) (ChildStatus, error) {
	switch status {
	case agent.StatusCompleted:
		return ChildCompleted, nil
	case agent.StatusFailed:
		return ChildFailed, nil
	case agent.StatusCanceled:
		return ChildCanceled, nil
	case agent.StatusTimedOut:
		return ChildTimedOut, nil
	case agent.StatusKilled:
		return ChildKilled, nil
	default:
		return "", fmt.Errorf("unexpected terminal status %s", status)
	}
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
