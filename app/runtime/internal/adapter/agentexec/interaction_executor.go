package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
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
	interactionDoomLoopThreshold     = 3
)

// InteractionChatResolver resolves the exact model client selected by a Run.
// Resolution happens during staging and must not invoke the model.
type InteractionChatResolver interface {
	ResolveChat(ctx context.Context, selection modelref.Selection) (*chatclient.Client, error)
}

// InteractionExecutorConfig freezes the host-owned inputs shared by native
// Interaction root executions. Identity strings must change whenever the
// executable Interaction adapter or behavior-affecting dispatcher configuration
// changes, so Agent2 Deployment references remain honest.
type InteractionExecutorConfig struct {
	DefaultClient             *chatclient.Client
	ChatResolver              InteractionChatResolver
	ImplementationIdentity    string
	ConfigurationIdentity     string
	DefaultMaxModelCalls      uint32
	StreamModelResponses      bool
	DeltaBufferCapacity       int
	MaxConcurrentToolCalls    int
	ToolResolver              InteractionToolResolver
	ToolInterpreter           InteractionToolInterpreter
	ToolPresenter             InteractionToolPresenter
	ToolAuthorizer            AutomaticToolAuthorizer
	ToolHooks                 InteractionToolHooks
	ToolResultStore           toolResultOffloader
	ToolResultThreshold       int
	ToolResultReaderName      string
	Pricing                   accounting.Pricing
	Provider                  string
	UnknownEffectPollInterval time.Duration
}

// InteractionExecutor is the native Agent2 root execution adapter. Each staged
// root owns an independent Engine and exactly one Interaction Process; the
// Application owns durable Run state and consumes only [runs.ExecutorEvent].
type InteractionExecutor struct {
	config InteractionExecutorConfig

	mu       sync.Mutex
	sessions map[string]*interactionSession
}

// NewInteractionExecutor validates immutable host configuration. It creates no
// Engine, goroutine, model call, or tool call; those resources are per staged
// root execution.
func NewInteractionExecutor(config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if config.DefaultClient == nil && isNilInteractionCapability(config.ChatResolver) {
		return nil, errors.New("agentexec: native Interaction requires a chat client or resolver")
	}
	for _, capability := range []struct {
		name  string
		value any
	}{
		{name: "chat resolver", value: config.ChatResolver},
		{name: "Tool resolver", value: config.ToolResolver},
		{name: "Tool interpreter", value: config.ToolInterpreter},
		{name: "Tool presenter", value: config.ToolPresenter},
		{name: "Tool authorizer", value: config.ToolAuthorizer},
		{name: "Tool hooks", value: config.ToolHooks},
		{name: "Tool-result store", value: config.ToolResultStore},
	} {
		if capability.value != nil && isNilInteractionCapability(capability.value) {
			return nil, fmt.Errorf("agentexec: native Interaction %s is typed nil", capability.name)
		}
	}
	if strings.TrimSpace(config.ImplementationIdentity) == "" ||
		config.ImplementationIdentity != strings.TrimSpace(config.ImplementationIdentity) {
		return nil, errors.New("agentexec: native Interaction implementation identity is required without surrounding whitespace")
	}
	if strings.TrimSpace(config.ConfigurationIdentity) == "" ||
		config.ConfigurationIdentity != strings.TrimSpace(config.ConfigurationIdentity) {
		return nil, errors.New("agentexec: native Interaction configuration identity is required without surrounding whitespace")
	}
	if config.DeltaBufferCapacity < 0 {
		return nil, errors.New("agentexec: native Interaction delta buffer capacity must not be negative")
	}
	if config.MaxConcurrentToolCalls < 0 {
		return nil, errors.New("agentexec: native Interaction Tool concurrency must not be negative")
	}
	if config.ToolResultThreshold < 0 {
		return nil, errors.New("agentexec: native Interaction Tool-result threshold must not be negative")
	}
	if !isNilInteractionCapability(config.ToolResultStore) && config.ToolResultThreshold > 0 {
		if strings.TrimSpace(config.ToolResultReaderName) == "" ||
			config.ToolResultReaderName != strings.TrimSpace(config.ToolResultReaderName) {
			return nil, errors.New("agentexec: native Interaction Tool-result reader name is required without surrounding whitespace when offload is enabled")
		}
	}
	if config.UnknownEffectPollInterval < 0 {
		return nil, errors.New("agentexec: native Interaction unknown-Effect poll interval must not be negative")
	}
	if config.Provider != strings.TrimSpace(config.Provider) {
		return nil, errors.New("agentexec: native Interaction provider has surrounding whitespace")
	}
	if config.UnknownEffectPollInterval == 0 {
		config.UnknownEffectPollInterval = defaultUnknownEffectPollInterval
	}
	if config.DefaultMaxModelCalls == 0 {
		config.DefaultMaxModelCalls = defaultInteractionModelCalls
	}
	return &InteractionExecutor{config: config, sessions: make(map[string]*interactionSession)}, nil
}

// ValidateRootStart rejects inputs the native Interaction cannot represent.
func (executor *InteractionExecutor) ValidateRootStart(start runs.RootExecutionStart) error {
	if err := start.Validate(); err != nil {
		return err
	}
	if len(start.WorkingContext) == 0 {
		return errors.New("agentexec: native Interaction requires a complete working context")
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
		return runs.ExecutorRef{}, errors.New("agentexec: native Interaction executor is nil")
	}
	if strings.TrimSpace(start.SessionID) == "" || start.SessionID != strings.TrimSpace(start.SessionID) {
		return runs.ExecutorRef{}, errors.New("agentexec: native Interaction session ID is required without surrounding whitespace")
	}
	if err := executor.ValidateRootStart(start); err != nil {
		return runs.ExecutorRef{}, err
	}
	client, err := executor.resolveClient(ctx, start.ModelSelection)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	maxModelCalls, err := executor.maxModelCalls(start)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: interactionDefinitionName, Description: interactionDefinitionDescription,
		Version: interactionDefinitionVersion, MaxModelCalls: maxModelCalls,
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: build native Interaction definition: %w", err)
	}
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: "exec_" + uuid.NewString()}
	session := newInteractionSession(ref, start, executor.config)
	executionCtx := executionctx.WithScope(ctx, rootExecutionScope(start))
	manifest := toolset.Manifest{}
	if executor.config.ToolResolver != nil {
		manifest, err = executor.config.ToolResolver.Manifest(executionCtx, domaintool.GroupRoot)
		if err != nil {
			return runs.ExecutorRef{}, fmt.Errorf("agentexec: resolve native Interaction Tools: %w", err)
		}
		manifest = manifest.Clone()
	}
	if err := validateToolManifest(manifest); err != nil {
		return runs.ExecutorRef{}, err
	}
	if len(manifest.Visible)+len(manifest.Deferred) > 0 {
		if executor.config.ToolInterpreter == nil {
			return runs.ExecutorRef{}, errors.New("agentexec: native Interaction Tools require a Tool interpreter")
		}
		if executor.config.ToolAuthorizer == nil {
			return runs.ExecutorRef{}, errors.New("agentexec: native Interaction Tools require an automatic authorizer")
		}
		for _, tools := range [][]toolcontract.Tool{manifest.Visible, manifest.Deferred} {
			for _, executable := range tools {
				name := executable.Definition().Name
				if class := executor.config.ToolInterpreter.SafetyClass(name); !class.Valid() {
					return runs.ExecutorRef{}, fmt.Errorf(
						"agentexec: native Interaction Tool %q has invalid safety class %q",
						name,
						class,
					)
				}
			}
		}
	}
	observedClient, err := newObservedInteractionClient(client, session)
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: observe native Interaction client: %w", err)
	}
	visibleTools, deferredTools := wrapInteractionTools(manifest, session, executor.config, start)
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{
		Client: observedClient, Tools: visibleTools, DeferredTools: deferredTools,
		MaxConcurrentToolCalls: executor.config.MaxConcurrentToolCalls,
		StreamModelResponses:   executor.config.StreamModelResponses,
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: build native Interaction dispatcher: %w", err)
	}
	trackedDispatcher := &interactionDispatcher{inner: dispatcher, session: session}
	configuration, err := json.Marshal(struct {
		Identity               string                    `json:"identity"`
		Provider               string                    `json:"provider"`
		Model                  string                    `json:"model"`
		MaxModelCalls          uint32                    `json:"maxModelCalls"`
		Streaming              bool                      `json:"streaming"`
		MaxConcurrentToolCalls int                       `json:"maxConcurrentToolCalls"`
		ToolResultThreshold    int                       `json:"toolResultThreshold"`
		ToolResultReaderName   string                    `json:"toolResultReaderName,omitempty"`
		VisibleTools           []corechat.ToolDefinition `json:"visibleTools,omitempty"`
		DeferredTools          []corechat.ToolDefinition `json:"deferredTools,omitempty"`
	}{
		Identity: executor.config.ConfigurationIdentity,
		Provider: session.provider, Model: start.ModelSelection.Model(),
		MaxModelCalls: maxModelCalls, Streaming: executor.config.StreamModelResponses,
		MaxConcurrentToolCalls: executor.config.MaxConcurrentToolCalls,
		ToolResultThreshold:    executor.config.ToolResultThreshold,
		ToolResultReaderName:   executor.config.ToolResultReaderName,
		VisibleTools:           toolDefinitions(manifest.Visible), DeferredTools: toolDefinitions(manifest.Deferred),
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: encode native Interaction configuration identity: %w", err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition: definition, Dispatcher: trackedDispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte(executor.config.ImplementationIdentity)),
		ConfigurationDigest:  agent.ComputeDigest(configuration),
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: build native Interaction deployment: %w", err)
	}
	input, err := agent.EncodeInput(interaction.Input{
		Messages: cloneChatMessages(start.WorkingContext), Options: clonedOptions(start.Options),
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: encode native Interaction input: %w", err)
	}

	session.deployment = deployment
	session.input = input
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver:  exactDeploymentResolver{deployment: deployment},
		ProcessAdmitter:     agent.ProcessAdmitterFunc(session.admitRoot),
		DeltaListeners:      []agent.DeltaListener{agent.DeltaListenerFunc(session.projectDelta)},
		DeltaBufferCapacity: executor.config.DeltaBufferCapacity,
		Limits:              agent.DefaultLimits(),
		TreeLimits: agent.TreeLimits{
			MaxDepth: 1, MaxChildren: 1, MaxActiveChildren: 1, MaxTreeProcesses: 1,
		},
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: build native Interaction engine: %w", err)
	}
	session.engine = engine

	executor.mu.Lock()
	defer executor.mu.Unlock()
	if _, duplicate := executor.sessions[ref.ExecutorID]; duplicate {
		_ = engine.Close()
		return runs.ExecutorRef{}, errors.New("agentexec: duplicate native Interaction executor identity")
	}
	executor.sessions[ref.ExecutorID] = session
	return ref, nil
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
	if !session.attachObserver() {
		return nil, errors.New("agentexec: native Interaction execution already has an observer")
	}
	return func(yield func(runs.ExecutorEvent) bool) {
		for {
			select {
			case event, open := <-session.events:
				if !open || !yield(event) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

// BeginRoot starts the staged Process exactly once. Agent2 owns every execution
// step; this adapter only awaits the immutable terminal result and translates it.
func (executor *InteractionExecutor) BeginRoot(ctx context.Context, ref runs.ExecutorRef) error {
	session, err := executor.session(ref)
	if err != nil {
		return err
	}
	if !session.begin() {
		return errors.New("agentexec: native Interaction execution was already begun")
	}
	if !session.observerAttached() {
		session.failStart()
		return errors.New("agentexec: native Interaction execution must be observed before begin")
	}
	process, err := session.engine.Start(
		executionctx.WithScope(ctx, session.scope),
		session.deployment,
		session.input,
	)
	if err != nil {
		session.failStart()
		return fmt.Errorf("agentexec: start native Interaction: %w", err)
	}
	session.setProcess(process)
	session.startWorkers()
	return nil
}

func rootExecutionScope(start runs.RootExecutionStart) runs.ExecutionScope {
	return runs.ExecutionScope{
		SessionID: start.SessionID, CWD: start.CWD, WorkspaceCWD: start.WorkspaceCWD,
		Isolated: start.Isolated, GoalLeaseID: start.GoalLeaseID,
	}
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
		executor.mu.Unlock()
	}
	return err
}

func (executor *InteractionExecutor) resolveClient(
	ctx context.Context,
	selection modelref.Selection,
) (*chatclient.Client, error) {
	if selection.Configured() {
		if executor.config.ChatResolver == nil {
			return nil, errors.New("agentexec: explicit native Interaction model selection requires a chat resolver")
		}
		client, err := executor.config.ChatResolver.ResolveChat(ctx, selection)
		if err != nil {
			return nil, fmt.Errorf("agentexec: resolve native Interaction chat client: %w", err)
		}
		if client == nil {
			return nil, errors.New("agentexec: native Interaction chat resolver returned nil")
		}
		return client, nil
	}
	if executor.config.DefaultClient == nil {
		return nil, errors.New("agentexec: native Interaction has no default chat client")
	}
	return executor.config.DefaultClient, nil
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
		return nil, errors.New("agentexec: native Interaction executor is nil")
	}
	if err := ref.ValidateFor(ref.SessionID); err != nil {
		return nil, err
	}
	executor.mu.Lock()
	session := executor.sessions[ref.ExecutorID]
	executor.mu.Unlock()
	if session == nil || session.ref.SessionID != ref.SessionID {
		return nil, fmt.Errorf("%w: native Interaction execution %q", runs.ErrExecutorNotLive, ref.ExecutorID)
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

type exactDeploymentResolver struct{ deployment agent.Deployment }

func (resolver exactDeploymentResolver) Resolve(reference agent.DeploymentRef) (agent.Deployment, error) {
	if resolver.deployment.DeploymentRef() != reference {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	return resolver.deployment, nil
}

var (
	_ runs.RootExecutionStarter = (*InteractionExecutor)(nil)
	_ runs.ExecutionObserver    = (*InteractionExecutor)(nil)
	_ runs.ExecutionReleaser    = (*InteractionExecutor)(nil)
)
