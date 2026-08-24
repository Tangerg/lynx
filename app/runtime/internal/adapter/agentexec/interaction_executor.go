package agentexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/chatclient"
	corechat "github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

const (
	interactionDefinitionName        = "lyra.runtime.interaction"
	interactionDefinitionDescription = "Run one model-directed Lyra interaction over a frozen working context."
	interactionDefinitionVersion     = "1.0.0"
	defaultInteractionModelCalls     = 64
	interactionEventBuffer           = 64
	interactionReleaseReason         = "runtime released execution resources"
	defaultUnknownEffectPollInterval = time.Second
	defaultInteractionStatePoll      = 250 * time.Millisecond
	interactionDoomLoopThreshold     = 3
)

// InteractionChatResolver resolves the exact model client selected by a Run.
// Resolution happens during staging and must not invoke the model.
type InteractionChatResolver interface {
	ResolveChat(ctx context.Context, selection modelref.Selection) (*chatclient.Client, error)
}

// RestoreScopeValidator verifies the host facts a durable executor checkpoint
// cannot prove for itself. It must not mutate or recreate the workspace.
type RestoreScopeValidator interface {
	ValidateRestoreScope(ctx context.Context, scope runs.ExecutionScope) error
}

// InteractionExecutorConfig freezes the host-owned inputs shared by
// Interaction root executions. Identity strings must change whenever the
// executable Interaction adapter or behavior-affecting dispatcher configuration
// changes, so Agent Framework Deployment references remain honest.
type InteractionExecutorConfig struct {
	// Lifetime is the process-owned root for every Interaction staged by this
	// executor. Request contexts may bound staging and commands, but accepted
	// execution must outlive the request that created it.
	Lifetime                  context.Context
	BuildID                   string
	DefaultClient             *chatclient.Client
	DefaultSelection          modelref.Selection
	ChatResolver              InteractionChatResolver
	RestoreScopeValidator     RestoreScopeValidator
	ImplementationIdentity    string
	ConfigurationIdentity     string
	DefaultMaxModelCalls      uint32
	StreamModelResponses      bool
	DeltaBufferCapacity       int
	MaxConcurrentToolCalls    int
	ToolResolver              InteractionToolResolver
	ToolInterpreter           InteractionToolInterpreter
	ToolPresenter             InteractionToolPresenter
	ToolAuthorizer            InteractionToolAuthorizer
	ToolHooks                 InteractionToolHooks
	MCPToolAutoApproved       func(server, tool string) bool
	Maintenance               RunMaintenance
	LifecycleHooks            InteractionLifecycleHooks
	ToolResultStore           toolResultOffloader
	ToolResultThreshold       int
	ToolResultReaderName      string
	Pricing                   accounting.Pricing
	Provider                  string
	UnknownEffectPollInterval time.Duration
	StatePollInterval         time.Duration
	Delegation                InteractionDelegationConfig
}

// InteractionExecutor is the Agent Framework root execution adapter. Each staged
// root owns an independent Engine and exactly one Interaction Process; the
// Application owns durable Run state and consumes only [runs.ExecutorEvent].
type InteractionExecutor struct {
	lifetime context.Context
	config   InteractionExecutorConfig

	mu       sync.Mutex
	sessions map[string]*interactionSession
	closed   bool
	shutdown []*interactionSession
}

// NewInteractionExecutor validates immutable host configuration. It creates no
// Engine, goroutine, model call, or tool call; those resources are per staged
// root execution.
func NewInteractionExecutor(config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if config.Lifetime == nil {
		return nil, errors.New("agentexec: Interaction lifetime is required")
	}
	if config.DefaultClient == nil && isNilInteractionCapability(config.ChatResolver) {
		return nil, errors.New("agentexec: Interaction requires a chat client or resolver")
	}
	if err := config.DefaultSelection.Validate(); err != nil {
		return nil, fmt.Errorf("agentexec: Interaction default model selection: %w", err)
	}
	if !config.DefaultSelection.Configured() {
		return nil, errors.New("agentexec: Interaction requires an exact default model selection")
	}
	for _, capability := range []struct {
		name  string
		value any
	}{
		{name: "chat resolver", value: config.ChatResolver},
		{name: "restore-scope validator", value: config.RestoreScopeValidator},
		{name: "Tool resolver", value: config.ToolResolver},
		{name: "Tool interpreter", value: config.ToolInterpreter},
		{name: "Tool presenter", value: config.ToolPresenter},
		{name: "Tool authorizer", value: config.ToolAuthorizer},
		{name: "Tool hooks", value: config.ToolHooks},
		{name: "Run maintenance", value: config.Maintenance},
		{name: "lifecycle hooks", value: config.LifecycleHooks},
		{name: "Tool-result store", value: config.ToolResultStore},
	} {
		if capability.value != nil && isNilInteractionCapability(capability.value) {
			return nil, fmt.Errorf("agentexec: Interaction %s is typed nil", capability.name)
		}
	}
	if strings.TrimSpace(config.ImplementationIdentity) == "" ||
		config.ImplementationIdentity != strings.TrimSpace(config.ImplementationIdentity) {
		return nil, errors.New("agentexec: Interaction implementation identity is required without surrounding whitespace")
	}
	if !validInteractionBuildID(config.BuildID) {
		return nil, errors.New("agentexec: Interaction build ID must use sha256:<64 lowercase hexadecimal characters>")
	}
	if strings.TrimSpace(config.ConfigurationIdentity) == "" ||
		config.ConfigurationIdentity != strings.TrimSpace(config.ConfigurationIdentity) {
		return nil, errors.New("agentexec: Interaction configuration identity is required without surrounding whitespace")
	}
	if config.DeltaBufferCapacity < 0 {
		return nil, errors.New("agentexec: Interaction delta buffer capacity must not be negative")
	}
	if config.MaxConcurrentToolCalls < 0 {
		return nil, errors.New("agentexec: Interaction Tool concurrency must not be negative")
	}
	if config.ToolResultThreshold < 0 {
		return nil, errors.New("agentexec: Interaction Tool-result threshold must not be negative")
	}
	if !isNilInteractionCapability(config.ToolResultStore) && config.ToolResultThreshold > 0 {
		if strings.TrimSpace(config.ToolResultReaderName) == "" ||
			config.ToolResultReaderName != strings.TrimSpace(config.ToolResultReaderName) {
			return nil, errors.New("agentexec: Interaction Tool-result reader name is required without surrounding whitespace when offload is enabled")
		}
	}
	if config.UnknownEffectPollInterval < 0 {
		return nil, errors.New("agentexec: Interaction unknown-Effect poll interval must not be negative")
	}
	if config.StatePollInterval < 0 {
		return nil, errors.New("agentexec: Interaction state poll interval must not be negative")
	}
	if config.Provider != strings.TrimSpace(config.Provider) {
		return nil, errors.New("agentexec: Interaction provider has surrounding whitespace")
	}
	if _, err := effectiveDelegation(config.Delegation); err != nil {
		return nil, err
	}
	if config.UnknownEffectPollInterval == 0 {
		config.UnknownEffectPollInterval = defaultUnknownEffectPollInterval
	}
	if config.StatePollInterval == 0 {
		config.StatePollInterval = defaultInteractionStatePoll
	}
	if config.DefaultMaxModelCalls == 0 {
		config.DefaultMaxModelCalls = defaultInteractionModelCalls
	}
	lifetime := config.Lifetime
	config.Lifetime = nil
	return &InteractionExecutor{
		lifetime: lifetime,
		config:   config,
		sessions: make(map[string]*interactionSession),
	}, nil
}

// ValidateRootStart rejects inputs the Interaction cannot represent.
func (executor *InteractionExecutor) ValidateRootStart(start runs.RootExecutionStart) error {
	if err := start.Validate(); err != nil {
		return err
	}
	if len(start.WorkingContext) == 0 {
		return errors.New("agentexec: Interaction requires a complete working context")
	}
	_, err := executor.maxModelCalls(start)
	return err
}

// StageRoot assembles one exact Interaction Deployment and independent Engine
// without starting a Process or crossing the model/tool side-effect boundary.
func (executor *InteractionExecutor) StageRoot(
	ctx context.Context,
	start runs.RootExecutionStart,
) (runs.ExecutorRef, error) {
	if executor == nil {
		return runs.ExecutorRef{}, errors.New("agentexec: Interaction executor is nil")
	}
	if strings.TrimSpace(start.SessionID) == "" || start.SessionID != strings.TrimSpace(start.SessionID) {
		return runs.ExecutorRef{}, errors.New("agentexec: Interaction session ID is required without surrounding whitespace")
	}
	if err := executor.ValidateRootStart(start); err != nil {
		return runs.ExecutorRef{}, err
	}
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: "exec_" + uuid.NewString()}
	session, err := executor.assembleInteraction(ctx, ref, start)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	input, err := agent.EncodeInput(interaction.Input{
		Messages: cloneChatMessages(start.WorkingContext), Options: clonedOptions(start.Options),
	})
	if err != nil {
		_ = session.engine.Close()
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: encode Interaction input: %w", err)
	}
	session.input = input
	if err := executor.registerSession(session); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func (executor *InteractionExecutor) assembleInteraction(
	ctx context.Context,
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
) (*interactionSession, error) {
	start.InterruptKinds = slices.Clone(start.InterruptKinds)
	client, err := executor.resolveClient(ctx, start.ModelSelection)
	if err != nil {
		return nil, err
	}
	maxModelCalls, err := executor.maxModelCalls(start)
	if err != nil {
		return nil, err
	}
	session := newInteractionSession(executor.lifetime, ref, start, executor.config)
	observedClient, err := newObservedInteractionClient(client, session)
	if err != nil {
		return nil, fmt.Errorf("agentexec: observe Interaction client: %w", err)
	}
	deployments, err := executor.buildInteractionDeployments(
		runExecutionContext(ctx, rootExecutionScope(start), start), session, start, observedClient, maxModelCalls,
	)
	if err != nil {
		return nil, err
	}
	if err := session.installDeployments(deployments); err != nil {
		return nil, err
	}
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver:              deployments,
		ProcessAdmitter:                 agent.ProcessAdmitterFunc(session.admitProcess),
		ProcessStartOutcomeAcknowledger: agent.ProcessStartOutcomeAcknowledgerFunc(session.acknowledgeProcessStartOutcome),
		EventListeners:                  []agent.EventListener{agent.EventListenerFunc(session.observeFrameworkEvent)},
		DeltaListeners:                  []agent.DeltaListener{agent.DeltaListenerFunc(session.projectDelta)},
		DeltaBufferCapacity:             executor.config.DeltaBufferCapacity,
		Limits:                          agent.DefaultLimits(),
		TreeLimits:                      deployments.treeLimits,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: build Interaction engine: %w", err)
	}
	session.engine = engine
	return session, nil
}

func (executor *InteractionExecutor) validateInteractionTools(manifest toolset.Manifest) error {
	if len(manifest.Visible)+len(manifest.Deferred) == 0 {
		return nil
	}
	if executor.config.ToolInterpreter == nil {
		return errors.New("agentexec: Interaction Tools require a Tool interpreter")
	}
	if executor.config.ToolAuthorizer == nil {
		return errors.New("agentexec: Interaction Tools require a Tool authorizer")
	}
	for _, tools := range [][]toolcontract.Tool{manifest.Visible, manifest.Deferred} {
		for _, executable := range tools {
			name := executable.Definition().Name
			if class := executor.config.ToolInterpreter.SafetyClass(name); !class.Valid() {
				return fmt.Errorf(
					"agentexec: Interaction Tool %q has invalid safety class %q",
					name,
					class,
				)
			}
		}
	}
	return nil
}

func (executor *InteractionExecutor) interactionConfiguration(
	session *interactionSession,
	start runs.RootExecutionStart,
	maxModelCalls uint32,
	manifest toolset.Manifest,
	role string,
	depth uint32,
	delegate agent.DeploymentRef,
	delegateBudget agent.Budget,
	instructions []corechat.Message,
) ([]byte, error) {
	configuration, err := json.Marshal(struct {
		Identity               string                    `json:"identity"`
		Provider               string                    `json:"provider"`
		Model                  string                    `json:"model"`
		MaxModelCalls          uint32                    `json:"maxModelCalls"`
		Streaming              bool                      `json:"streaming"`
		MaxConcurrentToolCalls int                       `json:"maxConcurrentToolCalls"`
		ToolResultThreshold    int                       `json:"toolResultThreshold"`
		ToolResultReaderName   string                    `json:"toolResultReaderName,omitempty"`
		InteractiveApproval    bool                      `json:"interactiveApproval"`
		VisibleTools           []corechat.ToolDefinition `json:"visibleTools,omitempty"`
		DeferredTools          []corechat.ToolDefinition `json:"deferredTools,omitempty"`
		Role                   string                    `json:"role"`
		Depth                  uint32                    `json:"depth"`
		Delegate               string                    `json:"delegate,omitempty"`
		DelegateBudget         agent.Budget              `json:"delegateBudget,omitzero"`
		Instructions           []corechat.Message        `json:"instructions,omitempty"`
	}{
		Identity: executor.config.ConfigurationIdentity,
		Provider: session.accounting.providerName(), Model: start.ModelSelection.Model(),
		MaxModelCalls: maxModelCalls, Streaming: executor.config.StreamModelResponses,
		MaxConcurrentToolCalls: executor.config.MaxConcurrentToolCalls,
		ToolResultThreshold:    executor.config.ToolResultThreshold,
		ToolResultReaderName:   executor.config.ToolResultReaderName,
		InteractiveApproval:    executor.config.ToolAuthorizer != nil,
		VisibleTools:           toolDefinitions(manifest.Visible), DeferredTools: toolDefinitions(manifest.Deferred),
		Role: role, Depth: depth, Delegate: delegate.String(), DelegateBudget: delegateBudget,
		Instructions: cloneChatMessages(instructions),
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode Interaction configuration identity: %w", err)
	}
	return configuration, nil
}

func (executor *InteractionExecutor) registerSession(session *interactionSession) error {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.closed {
		_ = session.engine.Close()
		return errors.New("agentexec: Interaction executor is shutting down")
	}
	if _, duplicate := executor.sessions[session.ref.ExecutorID]; duplicate {
		_ = session.engine.Close()
		return errors.New("agentexec: duplicate Interaction executor identity")
	}
	executor.sessions[session.ref.ExecutorID] = session
	return nil
}

// BeginShutdown atomically rejects future roots and freezes the complete live
// execution set. Resource release is joined by AwaitShutdown under its caller's
// deadline so an interrupted close remains retryable.
func (executor *InteractionExecutor) BeginShutdown() {
	if executor == nil {
		return
	}
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.closed {
		return
	}
	executor.closed = true
	executor.shutdown = make([]*interactionSession, 0, len(executor.sessions))
	for _, session := range executor.sessions {
		executor.shutdown = append(executor.shutdown, session)
	}
	slices.SortFunc(executor.shutdown, func(left, right *interactionSession) int {
		return strings.Compare(left.ref.ExecutorID, right.ref.ExecutorID)
	})
}

// AwaitShutdown releases every root frozen by BeginShutdown. Successful
// targets are removed immediately; failed or timed-out targets stay owned for a
// later attempt.
func (executor *InteractionExecutor) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("agentexec: Interaction shutdown context is required")
	}
	executor.BeginShutdown()
	executor.mu.Lock()
	targets := slices.Clone(executor.shutdown)
	executor.mu.Unlock()
	var failures []error
	for _, session := range targets {
		if err := session.release(ctx); err != nil {
			failures = append(failures, fmt.Errorf(
				"agentexec: release Interaction %q: %w",
				session.ref.ExecutorID,
				err,
			))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		executor.mu.Lock()
		if executor.sessions[session.ref.ExecutorID] == session {
			delete(executor.sessions, session.ref.ExecutorID)
		}
		executor.shutdown = slices.DeleteFunc(executor.shutdown, func(candidate *interactionSession) bool {
			return candidate == session
		})
		executor.mu.Unlock()
	}
	return errors.Join(failures...)
}

func validInteractionBuildID(value string) bool {
	digest, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(digest) != sha256.Size*2 || digest != strings.ToLower(digest) {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func toolDefinitions(tools []toolcontract.Tool) []corechat.ToolDefinition {
	definitions := make([]corechat.ToolDefinition, len(tools))
	for index, executable := range tools {
		definitions[index] = executable.Definition().Clone()
	}
	return definitions
}

// Observe attaches the single Application Run pump before Process start.
// Streaming facts are best-effort; authoritative completion and termination
// are always emitted from Process.Await.
func (executor *InteractionExecutor) Observe(
	ctx context.Context,
	ref runs.ExecutorRef,
) (iter.Seq[runs.ExecutorEvent], error) {
	session, err := executor.session(ref)
	if err != nil {
		return nil, err
	}
	if !session.state.attachObserver() {
		return nil, errors.New("agentexec: Interaction execution already has an observer")
	}
	stopDetach := context.AfterFunc(ctx, session.state.detachObserver)
	return func(yield func(runs.ExecutorEvent) bool) {
		defer func() {
			stopDetach()
			session.state.detachObserver()
		}()
		for {
			select {
			case event, open := <-session.lifetime.events:
				if !open || !yield(event) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

// BeginRoot starts the staged Process exactly once. The accepted Run uses the
// staged session lifecycle rather than the caller's request context, so a client
// disconnect cannot cancel durable execution. Agent Framework owns every step;
// this adapter only awaits and translates the immutable terminal result.
func (executor *InteractionExecutor) BeginRoot(_ context.Context, ref runs.ExecutorRef) error {
	session, err := executor.session(ref)
	if err != nil {
		return err
	}
	if !session.state.begin() {
		return errors.New("agentexec: Interaction execution was already begun")
	}
	if !session.state.observerAttached() {
		session.failStart()
		return errors.New("agentexec: Interaction execution must be observed before begin")
	}
	process, err := session.engine.Start(
		runExecutionContext(session.lifetime.context, session.scope, session.start),
		session.deployment,
		session.input,
	)
	if err != nil {
		session.failStart()
		return fmt.Errorf("agentexec: start Interaction: %w", err)
	}
	session.state.setProcess(process)
	session.startWorkers()
	return nil
}

// StageContinuation claims the exact process-local waiting boundary without
// advancing it. A missing live owner is rebuilt only from the supplied exact
// TreeSnapshot; a mismatch is rejected instead of silently recapturing state.
func (executor *InteractionExecutor) StageContinuation(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (runs.ExecutorRef, error) {
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if continuation.Checkpoint.BuildID != executor.config.BuildID {
		return runs.ExecutorRef{}, fmt.Errorf(
			"%w: checkpoint build %q does not match %q",
			runs.ErrExecutorStateLost,
			continuation.Checkpoint.BuildID,
			executor.config.BuildID,
		)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	session, err := executor.session(ref)
	if err == nil {
		if err := session.stageContinuation(continuation.Checkpoint); err != nil {
			return runs.ExecutorRef{}, err
		}
		return ref, nil
	}
	if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	if err := executor.restoreWaitingTree(
		ctx,
		ref,
		continuation,
		interactionBoundaryContinuationStaged,
	); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

// RestoreWaitingExecution reconstructs an exact committed waiting tree without
// claiming it for continuation. An existing owner is rejected: recovery must
// first prove that the obsolete execution was released.
func (executor *InteractionExecutor) RestoreWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (runs.ExecutorRef, error) {
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if continuation.Checkpoint.BuildID != executor.config.BuildID {
		return runs.ExecutorRef{}, fmt.Errorf(
			"%w: checkpoint build %q does not match %q",
			runs.ErrExecutorStateLost,
			continuation.Checkpoint.BuildID,
			executor.config.BuildID,
		)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if _, err := executor.session(ref); err == nil {
		return runs.ExecutorRef{}, runs.ErrExecutionClaimed
	} else if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	if err := executor.restoreWaitingTree(
		ctx,
		ref,
		continuation,
		interactionBoundaryWaiting,
	); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func (executor *InteractionExecutor) restoreWaitingTree(
	ctx context.Context,
	ref runs.ExecutorRef,
	continuation runs.WaitingContinuation,
	boundary interactionBoundary,
) error {
	if err := executor.validateRestoreScope(ctx, continuation.Checkpoint.Scope); err != nil {
		return err
	}
	checkpoint, err := decodeInteractionCheckpointPayload(continuation.Checkpoint.Payload)
	if err != nil {
		return fmt.Errorf("%w: parse Interaction checkpoint: %w", runs.ErrExecutorStateLost, err)
	}
	rootID, err := agent.ParseProcessID(continuation.Checkpoint.RootMemberID)
	if err != nil || checkpoint.tree.RootID() != rootID {
		return fmt.Errorf("%w: checkpoint root differs from its tree", runs.ErrExecutorStateLost)
	}
	processSnapshots := checkpoint.tree.ProcessSnapshots()
	if len(processSnapshots) == 0 || processSnapshots[0].ProcessID() != rootID ||
		!isInteractionWaitingBoundary(processSnapshots[0].Status()) {
		return fmt.Errorf("%w: Interaction restore requires a product waiting boundary", runs.ErrExecutorStateLost)
	}
	start := runs.RootExecutionStart{
		SessionID: continuation.SessionID,
		CWD:       continuation.Checkpoint.Scope.CWD, WorkspaceCWD: continuation.Checkpoint.Scope.WorkspaceCWD,
		Isolated: continuation.Checkpoint.Scope.Isolated, GoalIncarnationID: continuation.Checkpoint.Scope.GoalIncarnationID,
		ModelSelection: continuation.Checkpoint.ModelSelection, Limits: continuation.Checkpoint.Limits,
		InterruptKinds:           continuation.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: continuation.ChildRunAdmissionEnabled,
		WorkingContext:           cloneChatMessages(checkpoint.instructions),
	}
	session, err := executor.assembleInteraction(ctx, ref, start)
	if err != nil {
		return err
	}
	process, err := session.engine.RestoreTree(
		runExecutionContext(session.lifetime.context, session.scope, session.start),
		session.deployment,
		checkpoint.tree,
	)
	if err != nil {
		_ = session.engine.Close()
		return fmt.Errorf("%w: restore exact Interaction tree: %w", runs.ErrExecutorStateLost, err)
	}
	if err := session.initializeRestoredContinuation(process, continuation, checkpoint, boundary); err != nil {
		discardRestoredInteraction(session, process)
		return err
	}
	unknown, err := session.unknownEffectIDs(ctx)
	if err != nil {
		discardRestoredInteraction(session, process)
		return fmt.Errorf("%w: inspect restored Interaction effects: %v", runs.ErrExecutorStateLost, err)
	}
	if len(unknown) > 0 {
		discardRestoredInteraction(session, process)
		return fmt.Errorf("%w: restored Interaction has unresolved effects", runs.ErrExecutorStateLost)
	}
	interruptions, err := session.pendingInterruptions(checkpoint.tree)
	if err != nil || len(interruptions) == 0 {
		discardRestoredInteraction(session, process)
		if err != nil {
			return fmt.Errorf("%w: inspect restored Interaction input: %v", runs.ErrExecutorStateLost, err)
		}
		return fmt.Errorf("%w: restored Interaction tree has no pending input", runs.ErrExecutorStateLost)
	}
	if err := executor.registerSession(session); err != nil {
		discardRestoredInteraction(session, process)
		return err
	}
	session.startWorkers()
	return nil
}

func (executor *InteractionExecutor) validateRestoreScope(
	ctx context.Context,
	scope runs.ExecutionScope,
) error {
	if scope.Isolated {
		return fmt.Errorf("%w: isolated workspaces are not restorable after executor loss", runs.ErrExecutorStateLost)
	}
	if executor.config.RestoreScopeValidator != nil {
		if err := executor.config.RestoreScopeValidator.ValidateRestoreScope(ctx, scope); err != nil {
			return fmt.Errorf("%w: validate restore scope: %v", runs.ErrExecutorStateLost, err)
		}
		return nil
	}
	for _, path := range []string{scope.CWD, scope.WorkspaceCWD} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%w: restore workspace path is empty", runs.ErrExecutorStateLost)
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%w: restore workspace %q is unavailable", runs.ErrExecutorStateLost, path)
		}
	}
	return nil
}

func discardRestoredInteraction(session *interactionSession, process *agent.Process) {
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(session.lifetime.context),
		authoritativeProjectionTimeout,
	)
	defer cancel()
	_ = process.Kill(cleanupCtx, interactionReleaseReason)
	_, _ = process.Await(cleanupCtx)
	_ = session.engine.Close()
}

// BeginContinuation converts every validated application answer into an
// Interaction response Signal. It is called only after the fresh Segment
// opening commits, so accepting the Signal cannot start model/tool work ahead
// of the product's durable lifecycle boundary.
func (executor *InteractionExecutor) BeginContinuation(
	ctx context.Context,
	ref runs.ExecutorRef,
	answers []runs.InterruptAnswer,
	allowedInterrupts []interrupt.Kind,
) error {
	session, err := executor.session(ref)
	if err != nil {
		return err
	}
	if err := session.beginContinuation(allowedInterrupts); err != nil {
		return err
	}
	paused, err := session.pausedProcessIDs()
	if err != nil {
		return fmt.Errorf("agentexec: inspect paused Interaction members: %w", err)
	}
	prepared, err := session.prepareContinuationAnswers(ctx, answers)
	if err != nil {
		return err
	}
	// The previous Agent Process lifetime includes the human wait. Reset the
	// executor's Segment clock before any answer can make that Process runnable.
	session.segmentClock.start()
	if err := session.deliverContinuationAnswers(ctx, prepared); err != nil {
		return err
	}
	if err := session.resumePausedProcesses(ctx, paused); err != nil {
		return err
	}
	session.continuationAccepted()
	return nil
}

type preparedInteractionAnswer struct {
	process *agent.Process
	signal  agent.SignalRequest
}

func (session *interactionSession) prepareContinuationAnswers(
	ctx context.Context,
	answers []runs.InterruptAnswer,
) ([]preparedInteractionAnswer, error) {
	session.state.mu.Lock()
	checkpoint := session.state.waitingCheckpoint.Clone()
	session.state.mu.Unlock()
	checkpointState, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return nil, fmt.Errorf("agentexec: decode staged Interaction checkpoint: %w", err)
	}
	interruptions, err := session.pendingInterruptions(checkpointState.tree)
	if err != nil {
		return nil, err
	}
	orderedAnswers := slices.Clone(answers)
	slices.SortFunc(orderedAnswers, func(left, right runs.InterruptAnswer) int {
		if order := strings.Compare(left.MemberID, right.MemberID); order != 0 {
			return order
		}
		return strings.Compare(left.RequestID, right.RequestID)
	})
	if len(orderedAnswers) != len(interruptions) {
		return nil, fmt.Errorf(
			"agentexec: %d Interaction answers do not match %d pending inputs",
			len(orderedAnswers), len(interruptions),
		)
	}
	prepared := make([]preparedInteractionAnswer, 0, len(orderedAnswers))
	for index, answer := range orderedAnswers {
		expected := interruptions[index]
		if answer.MemberID != expected.MemberID || answer.RequestID != expected.RequestID {
			return nil, errors.New("agentexec: interrupt answer set differs from the staged Interaction inputs")
		}
		processID, err := agent.ParseProcessID(answer.MemberID)
		if err != nil {
			return nil, fmt.Errorf("agentexec: parse answered Interaction member: %w", err)
		}
		process, found := session.engine.Process(processID)
		if !found {
			return nil, errors.New("agentexec: answered Interaction member is unavailable")
		}
		pending, found, err := interaction.PendingToolInputFromProcess(ctx, process)
		if err != nil {
			return nil, fmt.Errorf("agentexec: inspect pending Interaction input: %w", err)
		}
		if !found || answer.RequestID != pending.WaitID().String() {
			return nil, errors.New("agentexec: interrupt answer does not address the active Interaction input")
		}
		response, err := interactioninput.EncodeResolution(answer.Resolution)
		if err != nil {
			return nil, err
		}
		signalID, err := interactionAnswerSignalID(answer, response)
		if err != nil {
			return nil, err
		}
		signal, err := pending.ResponseSignal(signalID, response)
		if err != nil {
			return nil, fmt.Errorf("agentexec: construct Interaction answer Signal: %w", err)
		}
		prepared = append(prepared, preparedInteractionAnswer{process: process, signal: signal})
	}
	return prepared, nil
}

func (session *interactionSession) deliverContinuationAnswers(
	ctx context.Context,
	answers []preparedInteractionAnswer,
) error {
	for _, answer := range answers {
		accepted, err := answer.process.DeliverSignal(
			runExecutionContext(ctx, session.scope, session.start), answer.signal,
		)
		if err != nil {
			return fmt.Errorf("agentexec: deliver Interaction answer Signal: %w", err)
		}
		if !accepted {
			return errors.New("agentexec: Interaction answer Signal was already accepted")
		}
	}
	return nil
}

// SubmitSteer queues one user message for the next Interaction safe boundary.
// Agent Framework rejects it while the Process is waiting; accepted content is projected
// immediately before the model request that can first observe it.
func (executor *InteractionExecutor) SubmitSteer(
	ctx context.Context,
	ref runs.ExecutorRef,
	input []transcript.ContentBlock,
) error {
	session, err := executor.session(ref)
	if err != nil {
		return err
	}
	message, err := runs.MaterializeUserMessage(input)
	if err != nil {
		return err
	}
	return session.submitSteer(ctx, message, input)
}

func interactionAnswerSignalID(
	answer runs.InterruptAnswer,
	response json.RawMessage,
) (agent.SignalID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(answer.InterruptItemID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.MemberID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(answer.RequestID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(response)
	return agent.ParseSignalID("answer:" + hex.EncodeToString(digest.Sum(nil)))
}

func rootExecutionScope(start runs.RootExecutionStart) runs.ExecutionScope {
	return runs.ExecutionScope{
		SessionID: start.SessionID, CWD: start.CWD, WorkspaceCWD: start.WorkspaceCWD,
		Isolated: start.Isolated, GoalIncarnationID: start.GoalIncarnationID,
	}
}

func runExecutionContext(
	ctx context.Context,
	scope runs.ExecutionScope,
	start runs.RootExecutionStart,
) context.Context {
	capabilities := run.Capabilities{
		ChildRuns:      start.ChildRunAdmissionEnabled,
		InterruptKinds: slices.Clone(start.InterruptKinds),
	}.Normalized()
	return executionctx.WithRunCapabilities(executionctx.WithScope(ctx, scope), capabilities)
}

// Release tears down one staged or terminal per-root Engine. It is idempotent
// and does not decide the product Run outcome.
func (executor *InteractionExecutor) Release(ctx context.Context, ref runs.ExecutorRef) error {
	if executor == nil {
		return nil
	}
	executor.mu.Lock()
	session := executor.sessions[ref.ExecutorID]
	if session != nil && session.ref.SessionID != ref.SessionID {
		executor.mu.Unlock()
		return runs.ErrInvalidExecutorRef
	}
	if session == nil {
		executor.mu.Unlock()
		return nil
	}
	executor.mu.Unlock()

	err := session.release(ctx)
	if err == nil {
		executor.mu.Lock()
		if executor.sessions[ref.ExecutorID] == session {
			delete(executor.sessions, ref.ExecutorID)
		}
		executor.shutdown = slices.DeleteFunc(executor.shutdown, func(candidate *interactionSession) bool {
			return candidate == session
		})
		executor.mu.Unlock()
	}
	return err
}

// RequestRootCancellation submits the Application's accepted cancellation to
// Agent Framework without deciding the product outcome or releasing the tree.
// Success means the request entered Engine's queue. The adapter then cancels
// its cooperative in-flight model/Tool dispatches so they can settle promptly;
// Agent Framework remains the sole lifecycle owner and applies the accepted
// intent only after that safe settlement boundary.
func (executor *InteractionExecutor) RequestRootCancellation(
	ctx context.Context,
	ref runs.ExecutorRef,
	reason string,
) error {
	session, err := executor.session(ref)
	if err != nil {
		return err
	}
	process := session.state.processHandle()
	if process == nil {
		return runs.ErrExecutorNotLive
	}
	if err := process.RequestCancellation(ctx, reason); err != nil &&
		!errors.Is(err, agent.ErrProcessFinished) {
		return fmt.Errorf("agentexec: submit Interaction cancellation intent: %w", err)
	}
	session.cancelAllDispatches()
	return nil
}

func (executor *InteractionExecutor) resolveClient(
	ctx context.Context,
	selection modelref.Selection,
) (*chatclient.Client, error) {
	if err := selection.Validate(); err != nil {
		return nil, fmt.Errorf("agentexec: Interaction model selection: %w", err)
	}
	if !selection.Configured() {
		return nil, errors.New("agentexec: Interaction requires an exact model selection")
	}
	if selection == executor.config.DefaultSelection && executor.config.DefaultClient != nil {
		return executor.config.DefaultClient, nil
	}
	if executor.config.ChatResolver == nil {
		return nil, errors.New("agentexec: Interaction model selection requires a chat resolver")
	}
	client, err := executor.config.ChatResolver.ResolveChat(ctx, selection)
	if err != nil {
		return nil, fmt.Errorf("agentexec: resolve Interaction chat client: %w", err)
	}
	if client == nil {
		return nil, errors.New("agentexec: Interaction chat resolver returned nil")
	}
	return client, nil
}

func (executor *InteractionExecutor) maxModelCalls(start runs.RootExecutionStart) (uint32, error) {
	if start.Limits.MaxSteps == 0 {
		return executor.config.DefaultMaxModelCalls, nil
	}
	if uint64(start.Limits.MaxSteps) > math.MaxUint32 {
		return 0, fmt.Errorf("%w: max steps exceeds Interaction model-call range", runs.ErrInvalidRunLimit)
	}
	return uint32(start.Limits.MaxSteps), nil
}

func (executor *InteractionExecutor) session(ref runs.ExecutorRef) (*interactionSession, error) {
	if executor == nil {
		return nil, errors.New("agentexec: Interaction executor is nil")
	}
	if err := ref.ValidateFor(ref.SessionID); err != nil {
		return nil, err
	}
	executor.mu.Lock()
	session := executor.sessions[ref.ExecutorID]
	executor.mu.Unlock()
	if session == nil || session.ref.SessionID != ref.SessionID {
		return nil, fmt.Errorf("%w: Interaction execution %q", runs.ErrExecutorNotLive, ref.ExecutorID)
	}
	return session, nil
}

func cloneChatMessages(messages []corechat.Message) []corechat.Message {
	cloned := make([]corechat.Message, len(messages))
	for index := range messages {
		cloned[index] = messages[index].Clone()
	}
	return cloned
}

func clonedOptions(options *corechat.Options) corechat.Options {
	if options == nil {
		return corechat.Options{}
	}
	return options.Clone()
}

var (
	_ runs.RootExecutionStarter             = (*InteractionExecutor)(nil)
	_ runs.ExecutionObserver                = (*InteractionExecutor)(nil)
	_ runs.ExecutionReleaser                = (*InteractionExecutor)(nil)
	_ runs.RunningRootCancellationRequester = (*InteractionExecutor)(nil)
	_ runs.WaitingExecutionContinuer        = (*InteractionExecutor)(nil)
	_ runs.RunningExecutionSteerer          = (*InteractionExecutor)(nil)
)
