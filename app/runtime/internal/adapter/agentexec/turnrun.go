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
	// runtime stamps it onto each request under the chat conversation-id key,
	// which the history middleware reads to pull prior history before the
	// model call and save the new round afterwards. Empty string runs the
	// turn unattached (the runtime falls back to the process id, so a
	// single multi-round turn still keeps context, but nothing persists
	// across turns).
	SessionID string

	// Message is the user's input for this turn.
	Message string

	// Provider is the turn's provider id (the per-run selection; empty for a
	// default turn). Carried only so per-round cost pricing attributes spend to
	// the right provider — the client itself is supplied via ChatClient below.
	Provider string

	// Media carries the turn's image attachments, attached to the opening
	// user message as UserMessage.Media. Nil for a text-only turn.
	Media []*media.Media

	// Cwd is the working directory the turn's filesystem + shell tools run
	// in — the session's project directory. The chat action binds it onto
	// the process blackboard (turnctx.CwdBindingKey) as a protected entry so
	// the tool resolver anchors the tools there, and so `task` sub-agents
	// inherit it: Blackboard.Clone copies protected entries to children and
	// the action's ClearWorkingState policy preserves them. Empty falls back to
	// the engine's default workdir.
	Cwd string

	// Isolated marks a turn running in an isolated session: Cwd is a sandbox
	// copy and the shell must be OS-jailed. The chat action binds it protected
	// (turnctx.IsolatedBindingKey) so tools and task sub-agents see the isolation.
	Isolated bool

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. See
	// [turnInput.MaxBudget] for the stop semantics.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost (0 = no cap). See
	// [turnInput.MaxCostUSD] — requires a [Config.Pricing] hook.
	MaxCostUSD float64

	// MaxSteps caps cumulative model calls across the root and child delegation
	// tree (0 = no cap). See [turnInput.MaxSteps]; surfaces as the maxSteps run
	// outcome.
	MaxSteps int

	// Options carries per-run generation tuning (temperature, max tokens, stop
	// sequences). Model selection stays on Provider/ChatClient; these options
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
	Observer toolObserver

	// Steer, when non-nil, provides user messages injected into the running
	// loop during continuation rounds (mid-run steering, API.md §6). Messages
	// flow on the next tool loop round only, so the current assistant/tool
	// state remains the decision point. nil disables mid-run injection.
	Steer SteerSource

	// EventListener, when non-nil, is registered as a process-scope
	// extension. Values that also implement [event.Listener] (i.e.
	// have OnEvent) receive every agent runtime event for this turn
	// — process lifecycle (Created / Completed / Failed / Killed /
	// Stuck / Terminated), action execution, ready-to-plan, etc.
	// The canonical wrapper is [event.NamedListener]; turn.Dispatcher
	// uses one to map process terminal events onto TurnEnd reasons
	// without re-deriving status from the run loop's error.
	//
	// Names must be unique across the process extension slice — process
	// construction reports a collision through the run's error channel.
	EventListener core.Extension
}

// snapshot returns the protocol-value state owned by the process launched from
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
// drops in on the same Process via [runtime.Engine.Resume].
//
// Observer attaches a process-scope [core.ToolMiddleware]; SessionID binds the
// turn to the chat history middleware's keyed conversation.
func (e *Engine) StartTurn(ctx context.Context, request TurnRequest) (TurnProcess, error) {
	request = request.snapshot()
	input := turnInput{Message: request.Message, Provider: request.Provider, Media: request.Media, Cwd: request.Cwd, Isolated: request.Isolated, SessionID: request.SessionID, MaxBudget: request.MaxBudget, MaxCostUSD: request.MaxCostUSD, MaxSteps: request.MaxSteps, Options: request.Options}

	guardrails, err := e.steeringGuardrails(request.Steer)
	if err != nil {
		return nil, fmt.Errorf("engine: build steering guardrails: %w", err)
	}
	processOptions, err := e.turnProcessOptions(request.SessionID, request.Observer, request.EventListener, request.ChatClient, guardrails)
	if err != nil {
		return nil, fmt.Errorf("engine: configure chat process: %w", err)
	}
	process, done, err := e.runtime.Start(ctx, e.agent,
		core.Input(input),
		processOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("engine: start chat: %w", err)
	}
	return &turnProcess{process: process, done: done, engine: e.runtime}, nil
}

// turnProcessOptions assembles per-process wiring: the chat history Session
// binding, the observer decorator, lifecycle listener, and per-run model
// client. Shared chat guardrails are built once in [New] and can be overridden
// per turn when mid-run steering is enabled. The runtime stamps each request's
// conversation id from this Session, so one shared history chain can still
// serve both this turn and spawned subtasks unless explicitly overridden.
func (e *Engine) turnProcessOptions(sessionID string, observer toolObserver, listener core.Extension, client *chatclient.Client, guardrails *core.ChatGuardrails) (core.ProcessOptions, error) {
	dependencies := e.dependencies
	options := core.ProcessOptions{}
	if dependencies != nil {
		options.ChildOptions = childOptions(dependencies, client, observer, e.toolResultStore, e.toolResultThreshold)
	}
	if sessionID != "" {
		options.Session = &core.Session{ID: sessionID}
	}
	if observer != nil {
		if dependencies == nil {
			return core.ProcessOptions{}, errors.New("agentexec: dependencies are required when a tool observer is configured")
		}
		observation := newToolObservation(observer, e.toolResultStore, e.toolResultThreshold)
		scope := dependencies.Child()
		if err := core.RegisterDependency(scope, toolObservationKey, observation); err != nil {
			return core.ProcessOptions{}, fmt.Errorf("agentexec: register tool observation dependency: %w", err)
		}
		options.Dependencies = scope
		options.Extensions = append(options.Extensions, &toolObserverMiddleware{observation: observation})
	}
	if listener != nil {
		options.Extensions = append(options.Extensions, listener)
	}
	if client != nil {
		options.Extensions = append(options.Extensions, perRunChatClient{client: client})
	}
	if guardrails != nil {
		options.Guardrails = guardrails
	}
	return options, nil
}

func (e *Engine) steeringGuardrails(steer SteerSource) (*core.ChatGuardrails, error) {
	if steer == nil {
		return nil, nil
	}
	if e.guardrailsBuilder == nil {
		return nil, errors.New("engine: steering guardrails builder is nil")
	}

	guardrails, err := e.guardrailsBuilder(
		e.historyStore,
		func(_ context.Context) []chat.Message {
			return steer()
		},
	)
	if err != nil {
		return nil, err
	}
	return guardrails, nil
}

// perRunChatClient is a [core.ChatProvider] carrying one resolved
// client for a single turn — the seam that lets a run pick its model.
type perRunChatClient struct{ client *chatclient.Client }

func (perRunChatClient) Name() string { return "lyra:per-run-chat-client" }
func (p perRunChatClient) Chat(core.ProcessView) core.ChatCapability {
	return core.ChatCapability{Model: p.client, Streamer: p.client}
}

// RestoreTurnRequest carries the per-process wiring to re-attach to a
// turn rebuilt from a snapshot — the same Observer + Session a fresh turn
// gets from [Engine.StartTurn], so the resumed continuation streams and
// keys chat history to the right conversation.
type RestoreTurnRequest struct {
	// SessionID rebinds the restored process to its chat history
	// conversation (so the continuation's LLM round loads + saves the
	// right history). Empty runs unattached.
	SessionID string

	// Observer receives the continuation's streaming tool-call + text
	// deltas, exactly as on a fresh turn. May be nil.
	Observer toolObserver

	// EventListener captures the restored process's terminal event so the
	// resumed turn can map it onto a TurnEnd reason. May be nil.
	EventListener core.Extension

	// ChatClient, when non-nil, overrides the model the restored continuation
	// runs against — the per-run model the parked turn used, re-resolved from
	// the interrupt's persisted provider+model. nil runs on the engine default
	// (a run that didn't pick a model, or one whose provider is no longer
	// configured). Same seam as [TurnRequest.ChatClient] on a fresh turn.
	ChatClient *chatclient.Client
}

// RestoreTurn rebuilds the agent process identified by processID from the
// configured ProcessStore snapshot. The framework snapshot already contains
// the exact JSON-safe Suspension and tool checkpoint, so the returned process
// can be answered immediately without a replay tick.
//
// Errors when no ProcessStore is configured, the snapshot is missing, the
// agent is not deployed under the snapshot's name, or the re-tick fails.
func (e *Engine) RestoreTurn(ctx context.Context, processID string, request RestoreTurnRequest) (TurnProcess, error) {
	// The restored continuation runs against request.ChatClient — the per-run model
	// re-resolved from the interrupt's persisted provider+model — so a restart
	// mid-run keeps the model the turn parked on. nil (no selection / provider
	// gone) falls back to the engine default.
	options, err := e.turnProcessOptions(request.SessionID, request.Observer, request.EventListener, request.ChatClient, nil)
	if err != nil {
		return nil, fmt.Errorf("engine: configure restored chat process: %w", err)
	}
	if e.runtime == nil {
		return nil, errors.New("engine: restore chat: agent runtime is required")
	}
	process, err := e.runtime.RestoreResumable(ctx, processID, options)
	if err != nil {
		if errors.Is(err, agentruntime.ErrResumableSnapshotLost) {
			return nil, processSnapshotLost("restore", err)
		}
		return nil, fmt.Errorf("engine: restore chat: %w", err)
	}
	if process == nil {
		return nil, errors.New("engine: restore chat: agent runtime returned nil process without an error")
	}
	return &turnProcess{process: process, engine: e.runtime}, nil
}
