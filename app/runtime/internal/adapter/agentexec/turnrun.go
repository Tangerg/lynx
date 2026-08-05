package agentexec

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// SteerSource yields user messages queued for mid-run injection. It is
// called only on continuation rounds; each call drains any pending queue
// associated with the currently running turn.
type SteerSource func() []chat.Message

// TurnRequest carries the per-turn parameters for [Engine.StartTurn].
// SessionID is non-empty to bind the turn to a chat history keyed conversation;
// Observer is non-nil to receive streaming notifications.
type TurnRequest struct {
	// SessionID anchors the turn to a chat history conversation. The
	// adapter's chat middleware projects it onto root-process model calls, then
	// loads prior history and saves the new round. Empty runs unattached: the
	// middleware uses the process ID, so one multi-round turn still keeps
	// context but nothing is shared across turns.
	SessionID string

	// Message is the user's input for this turn.
	Message string

	// ModelSelection is the explicit per-run model choice. Its zero value uses
	// the engine default; a configured value identifies the provider used for
	// per-round cost attribution and the resolved ChatClient below.
	ModelSelection modelref.Selection

	// Media carries the turn's image attachments, attached to the opening
	// user message as UserMessage.Media. Nil for a text-only turn.
	Media []*media.Media

	// CWD is the working directory the turn's filesystem + shell tools run in.
	// Runtime carries it in application context across the complete delegation
	// tree. Empty falls back to the engine's default cwd.
	CWD string

	// WorkspaceCWD is the persistent Session workspace. It remains the original
	// project directory when CWD is an isolated scratch copy.
	WorkspaceCWD string

	// Isolated marks a turn running in an isolated session: CWD is a sandbox
	// copy and the shell must be OS-jailed.
	Isolated bool

	// GoalLeaseID stamps a Goal-mode autonomous run with its goal incarnation.
	// The Goal outcome signal uses it to address only that incarnation. Empty for
	// ordinary runs.
	GoalLeaseID string

	// Limits are the immutable cumulative ceilings for the complete delegation
	// tree. Token and cost dimensions are continuation ceilings; model-call
	// admission is strict. Zero leaves a dimension unbounded.
	Limits execution.RunLimits

	// Options carries per-run generation tuning (temperature, max tokens, stop
	// sequences). Model selection stays on ModelSelection/ChatClient; these options
	// are merged over the selected client's model defaults.
	Options *chat.Options

	// ChatClient, when non-nil, overrides the model this turn runs against
	// — registered as a [core.ChatProvider] on the process so the
	// agent runtime uses it instead of the engine's default client. This
	// is how a per-run model selection reaches the turn (the caller resolves
	// the right provider+model client). nil uses the engine default.
	ChatClient *chatclient.Client

	// Observer receives streaming tool-call + text-delta
	// notifications. May be nil — the turn still runs.
	Observer executionObserver

	// Steer, when non-nil, provides user messages injected into the running
	// loop during continuation rounds. Steering messages
	// flow on the next tool loop round only, so the current assistant/tool
	// state remains the decision point. nil disables mid-run injection.
	Steer SteerSource

	// EventListener, when non-nil, is registered as a process-scope extension.
	// Values that also implement [event.Listener] receive Agent runtime events
	// for this turn. The execution controller installs one only when subagent
	// lifecycle hooks need subtree events.
	//
	// Names must be unique across the process extension slice — process
	// construction reports a collision synchronously from StartTurn.
	EventListener core.Extension

	// AdmitChild, when non-nil, is the synchronous durable-admission boundary
	// for AgentTool children. The child does not publish or execute until it
	// returns nil. Direct Agent Runtime children have no SpawnCallID and bypass
	// this application Run boundary.
	AdmitChild AdmitChildFunc
}

// snapshot returns the request-owned state used by the process launched from
// this request. Runtime collaborators keep their documented shared concurrency
// semantics; only caller-owned chat values need deep copies.
func (r TurnRequest) snapshot() TurnRequest {
	snapshot := r
	if r.Options != nil {
		options := r.Options.Clone()
		snapshot.Options = &options
	}
	if r.Media != nil {
		snapshot.Media = make([]*media.Media, len(r.Media))
		for index := range r.Media {
			snapshot.Media[index] = r.Media[index].Clone()
		}
	}
	return snapshot
}

// StartTurn dispatches a turn as an async agent process and
// returns the [TurnProcess] handle the caller drives. The lifecycle
// — cancel, status, awaiting completion, output extraction — runs
// against the agent runtime's [runtime.Process] rather than a
// bare goroutine, so HITL integration (plan approval, tool approval)
// drops in on the same Process via [runtime.Engine.Respond].
//
// Observer attaches a process-scope [core.ToolMiddleware]; SessionID binds the
// turn to the chat history middleware's keyed conversation.
func (e *Engine) StartTurn(ctx context.Context, request TurnRequest) (TurnProcess, error) {
	request = request.snapshot()
	scope := execution.ExecutionScope{
		SessionID:    request.SessionID,
		CWD:          request.CWD,
		WorkspaceCWD: request.WorkspaceCWD,
		Isolated:     request.Isolated,
		GoalLeaseID:  request.GoalLeaseID,
	}
	if err := scope.Validate(); err != nil {
		return nil, fmt.Errorf("engine: start chat: %w", err)
	}
	runCtx := executionctx.WithScope(ctx, scope)
	provider := request.ModelSelection.Provider()
	limits := request.Limits
	input := turnInput{Message: request.Message, Media: request.Media, Options: request.Options}
	usage := emptyUsageLedger()

	middleware, err := e.steeringChatMiddleware(request.Steer)
	if err != nil {
		return nil, fmt.Errorf("engine: build steering chat middleware: %w", err)
	}
	processOptions, err := e.turnProcessOptions(
		request.SessionID,
		provider,
		limits,
		request.Observer,
		request.EventListener,
		request.ChatClient,
		middleware,
		usage,
		request.AdmitChild,
	)
	if err != nil {
		return nil, fmt.Errorf("engine: configure chat process: %w", err)
	}
	runHandle, err := e.agentRuntime.Start(runCtx, e.turnAgent,
		core.Input(input),
		processOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("engine: start chat: %w", err)
	}
	if runHandle == nil || runHandle.Process() == nil {
		return nil, errors.New("engine: start chat: agent runtime returned an invalid run handle")
	}
	return &turnProcess{
		process:        runHandle.Process(),
		runHandle:      runHandle,
		owner:          e,
		scope:          scope,
		runCtx:         runCtx,
		usage:          usage,
		modelSelection: request.ModelSelection,
		limits:         limits,
	}, nil
}

// turnProcessOptions assembles per-process wiring: app-owned chat-history
// scoping, the observer decorator, lifecycle listener, and per-run model
// client. Shared chat middleware is built once in [New] and can be replaced
// per turn when mid-run steering is enabled.
func (e *Engine) turnProcessOptions(
	sessionID string,
	provider string,
	limits execution.RunLimits,
	observer executionObserver,
	listener core.Extension,
	client *chatclient.Client,
	middleware *core.ChatMiddleware,
	usage *usageLedger,
	admitChild AdmitChildFunc,
) (core.ProcessOptions, error) {
	dependencies := e.agentDependencies
	if dependencies == nil {
		return core.ProcessOptions{}, errors.New("agentexec: engine dependencies are required")
	}
	if usage == nil {
		return core.ProcessOptions{}, errors.New("agentexec: usage ledger is required")
	}
	if err := limits.Validate(); err != nil {
		return core.ProcessOptions{}, fmt.Errorf("agentexec: execution limits: %w", err)
	}
	scope := dependencies.Child()
	if err := core.RegisterDependency(scope, usageLedgerKey, usage); err != nil {
		return core.ProcessOptions{}, fmt.Errorf("agentexec: register usage ledger: %w", err)
	}
	options := core.ProcessOptions{
		Dependencies: scope,
		Budget: core.Budget{
			CostLimit:      limits.MaxBudgetUSD,
			ModelCallLimit: limits.MaxSteps,
			TokenLimit:     limits.MaxTotalTokens,
		},
	}
	baseMiddleware := e.chatMiddleware
	if middleware != nil {
		baseMiddleware = middleware
	}
	scopedMiddleware, err := scopeHistory(baseMiddleware, sessionID)
	if err != nil {
		return core.ProcessOptions{}, err
	}
	childMiddleware, err := scopeHistory(e.chatMiddleware, sessionID)
	if err != nil {
		return core.ProcessOptions{}, err
	}
	scopedMiddleware.StreamMiddlewares = append(
		[]chat.StreamMiddleware{streamIdleMiddleware(e.modelStreamIdleTimeout)},
		scopedMiddleware.StreamMiddlewares...,
	)
	childMiddleware.StreamMiddlewares = append(
		[]chat.StreamMiddleware{streamIdleMiddleware(e.modelStreamIdleTimeout)},
		childMiddleware.StreamMiddlewares...,
	)
	options.ChatMiddleware = scopedMiddleware
	options.ConfigureChild = childOptions(
		e,
		dependencies,
		client,
		provider,
		observer,
		e.toolResultStore,
		e.toolResultThreshold,
		e.toolResultReaderName,
		childMiddleware,
		usage,
		admitChild,
	)
	var observation *toolObservation
	if observer != nil {
		observation = newToolObservation(observer, e.toolResultStore, e.toolResultThreshold, e.toolResultReaderName)
		if err := core.RegisterDependency(scope, toolObservationKey, observation); err != nil {
			return core.ProcessOptions{}, fmt.Errorf("agentexec: register tool observation dependency: %w", err)
		}
		options.Extensions = append(options.Extensions, &toolObserverMiddleware{observation: observation})
	}
	options.Extensions = append(options.Extensions, &processProjection{
		engine:      e,
		provider:    provider,
		usage:       usage,
		observation: observation,
		observer:    observer,
	})
	if listener != nil {
		options.Extensions = append(options.Extensions, listener)
	}
	if client != nil {
		options.Extensions = append(options.Extensions, perRunChatClient{client: client})
	}
	return options, nil
}

func (e *Engine) steeringChatMiddleware(steer SteerSource) (*core.ChatMiddleware, error) {
	if steer == nil {
		return nil, nil
	}
	if e.chatMiddlewareBuilder == nil {
		return nil, errors.New("engine: steering chat middleware builder is nil")
	}

	middleware, err := e.chatMiddlewareBuilder(
		e.historyStore,
		func(_ context.Context) []chat.Message {
			return steer()
		},
	)
	if err != nil {
		return nil, err
	}
	return middleware, nil
}

// perRunChatClient is a [core.ChatProvider] carrying one resolved
// client for a single turn — the seam that lets a run pick its model.
type perRunChatClient struct{ client *chatclient.Client }

func (perRunChatClient) Name() string { return "lyra:per-run-chat-client" }
func (p perRunChatClient) Chat(core.ProcessView) core.ChatCapability {
	return core.ChatCapability{Model: p.client, Streamer: p.client}
}

// RestoreTurnRequest carries the live collaborators to re-attach to a turn
// rebuilt from a checkpoint. Durable host scope comes from that checkpoint.
type RestoreTurnRequest struct {
	// SessionID is the expected owner supplied by the application Run. It must
	// exactly match the checkpoint's non-empty application Session, so a
	// continuation can never cross product-session boundaries.
	SessionID string

	// ModelSelection is the immutable provider/model pair admitted for the Run.
	// It must match the application checkpoint exactly.
	ModelSelection modelref.Selection

	// CWD, WorkspaceCWD, and Isolated are the Session facts independently resolved by the
	// application. They must match the checkpoint so executor tools, lifecycle
	// hooks, and delegated work cannot rehydrate into different workspaces.
	CWD          string
	WorkspaceCWD string
	Isolated     bool
	// GoalLeaseID binds autonomous-goal tool context to the same application
	// lease whose terminal accounting will consume the resumed run.
	GoalLeaseID string

	// Limits are the immutable tree-wide ceilings admitted by the application
	// Run. Restore rejects a checkpoint that carries a different policy.
	Limits execution.RunLimits

	// Observer receives the continuation's streaming tool-call + text
	// deltas, exactly as on a fresh turn. May be nil.
	Observer executionObserver

	// EventListener receives restored-process subtree events for subagent
	// lifecycle hooks. May be nil.
	EventListener core.Extension

	// AdmitChild reattaches the synchronous child Run admission boundary when a
	// parked process tree is restored.
	AdmitChild AdmitChildFunc

	// ChatClient, when non-nil, overrides the model the restored continuation
	// runs against — the per-run model the parked turn used, re-resolved from
	// the interrupt's persisted provider+model. nil runs on the engine default
	// (a run that didn't pick a model). Same seam as [TurnRequest.ChatClient] on
	// a fresh turn.
	ChatClient *chatclient.Client
}

// RestoreTurn rebuilds the agent process identified by rootProcessID from the
// configured executor checkpoint. The framework snapshot already contains
// the exact JSON-safe Suspension and tool checkpoint, so the returned process
// can be answered immediately without a replay tick.
//
// Errors when no checkpoint reader is configured, the snapshot is missing, the
// agent is not deployed under the snapshot's name, or the re-tick fails.
func (e *Engine) RestoreTurn(ctx context.Context, rootProcessID string, request RestoreTurnRequest) (TurnProcess, error) {
	if request.Observer != nil && executionObserverIsNil(request.Observer) {
		return nil, fmt.Errorf("engine: configure restored chat process: observer: %w", core.ErrNilDependency)
	}
	// The restored continuation runs against request.ChatClient — the per-run model
	// re-resolved from the interrupt's persisted provider+model — so a restart
	// mid-run keeps the model the turn parked on. nil (no selection / provider
	// gone) falls back to the engine default.
	if e.agentRuntime == nil {
		return nil, errors.New("engine: restore chat: agent runtime is required")
	}
	if e.checkpoints == nil {
		return nil, errors.New("engine: restore chat: checkpoint reader is required")
	}
	checkpoint, err := e.checkpoints.LoadCheckpoint(ctx, rootProcessID)
	if err != nil {
		if isExecutorCheckpointLoss(err) {
			return nil, executorCheckpointLost("restore", err)
		}
		return nil, fmt.Errorf("engine: load process tree: %w", err)
	}
	if err := checkpoint.ValidateFor(execution.ExecutorCheckpointExpectation{
		RootProcessID:  rootProcessID,
		SessionID:      request.SessionID,
		CWD:            request.CWD,
		WorkspaceCWD:   request.WorkspaceCWD,
		Isolated:       request.Isolated,
		GoalLeaseID:    request.GoalLeaseID,
		ModelSelection: request.ModelSelection,
		Limits:         request.Limits,
	}); err != nil {
		return nil, executorCheckpointLost("restore ownership", err)
	}
	tree, err := decodeValidatedProcessTree(checkpoint)
	if err != nil {
		return nil, executorCheckpointLost("decode", err)
	}
	if checkpoint.BuildID != e.buildID {
		return nil, executorCheckpointLost(
			"restore",
			fmt.Errorf("checkpoint build %q does not match runtime build %q", checkpoint.BuildID, e.buildID),
		)
	}
	if err := validateCheckpointUsage(tree, checkpoint.Usage); err != nil {
		return nil, executorCheckpointLost("restore usage", err)
	}
	usage, err := newUsageLedger(checkpoint.Usage)
	if err != nil {
		return nil, executorCheckpointLost("restore usage", err)
	}
	options, err := e.turnProcessOptions(
		checkpoint.Scope.SessionID,
		checkpoint.ModelSelection.Provider(),
		checkpoint.Limits,
		request.Observer,
		request.EventListener,
		request.ChatClient,
		nil,
		usage,
		request.AdmitChild,
	)
	if err != nil {
		return nil, fmt.Errorf("engine: configure restored chat process: %w", err)
	}
	root, ok := tree.Root()
	if !ok {
		return nil, executorCheckpointLost("restore", core.ErrInvalidSnapshot)
	}
	if err := agentruntime.ValidateResumableSnapshot(root); err != nil {
		return nil, executorCheckpointLost("restore", err)
	}
	runCtx := executionctx.WithScope(ctx, checkpoint.Scope)
	process, err := e.agentRuntime.RestoreTree(runCtx, tree, options)
	if err != nil {
		if isExecutorCheckpointLoss(err) {
			return nil, executorCheckpointLost("restore", err)
		}
		return nil, fmt.Errorf("engine: restore process tree: %w", err)
	}
	if process == nil {
		return nil, errors.New("engine: restore chat: agent runtime returned nil process without an error")
	}
	return &turnProcess{
		process:        process,
		owner:          e,
		scope:          checkpoint.Scope,
		runCtx:         runCtx,
		usage:          usage,
		modelSelection: checkpoint.ModelSelection,
		limits:         checkpoint.Limits,
	}, nil
}

func isExecutorCheckpointLoss(err error) bool {
	return errors.Is(err, execution.ErrExecutorCheckpointNotFound) ||
		errors.Is(err, execution.ErrInvalidExecutorCheckpoint) ||
		errors.Is(err, core.ErrUnsupportedSnapshotSchema) ||
		errors.Is(err, core.ErrInvalidSnapshot) ||
		errors.Is(err, agentruntime.ErrDeploymentNotFound)
}
