package agentexec

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
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

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. See
	// [turnInput.MaxBudget] for the stop semantics.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost (0 = no cap). See
	// [turnInput.MaxCostUSD] — requires a [Config.Pricing] hook.
	MaxCostUSD float64

	// MaxSteps caps the turn's tool-call rounds (0 = no cap). See
	// [turnInput.MaxSteps]; surfaces as the maxSteps run outcome.
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

// StartTurn dispatches a turn as an async agent process and
// returns the [TurnProcess] handle the caller drives. The lifecycle
// — cancel, status, awaiting completion, output extraction — runs
// against the agent runtime's [runtime.Process] rather than a
// bare goroutine, so HITL integration (plan approval, tool approval)
// drops in on the same Process via [runtime.Engine.Resume].
//
// Observer attaches a process-scope [core.ToolMiddleware]; SessionID binds the
// turn to the chat history middleware's keyed conversation.
func (e *Engine) StartTurn(ctx context.Context, request TurnRequest) TurnProcess {
	var options *chat.Options
	if request.Options != nil {
		copy := cloneChatOptions(*request.Options)
		options = &copy
	}
	input := turnInput{Message: request.Message, Provider: request.Provider, Media: request.Media, Cwd: request.Cwd, SessionID: request.SessionID, MaxBudget: request.MaxBudget, MaxCostUSD: request.MaxCostUSD, MaxSteps: request.MaxSteps, Options: options}

	processOptions := turnProcessOptions(e.dependencies, request.SessionID, request.Observer, request.EventListener, request.ChatClient, e.steeringGuardrails(request.Steer))
	process, done := e.turnStarter.Start(ctx, e.agent,
		map[string]any{core.DefaultBindingName: input},
		processOptions,
	)
	return &turnProcess{process: process, done: done, engine: e.turnControl}
}

// turnProcessOptions assembles per-process wiring: the chat history Session
// binding, the observer decorator, lifecycle listener, and per-run model
// client. The chat middleware chain itself (tool loop + memory) is the
// engine default built once in [New]; the chat middleware chain can be
// overridden per turn by supplying [core.ProcessOptions.Guardrails] when
// mid-run steering is enabled. The runtime stamps each request's conversation
// id from this Session, so one shared chain can still serve both this turn and
// any spawned subtask unless explicitly overridden.
func turnProcessOptions(dependencies *core.Dependencies, sessionID string, observer toolObserver, listener core.Extension, client *chatclient.Client, guardrails *core.ChatGuardrails) core.ProcessOptions {
	options := core.ProcessOptions{}
	if sessionID != "" {
		options.Session = &core.Session{ID: sessionID}
	}
	if observer != nil {
		options.Extensions = append(options.Extensions, &toolObserverMiddleware{observer: observer})
		if dependencies != nil {
			options.Dependencies = dependencies.Child()
			if err := core.RegisterDependency(options.Dependencies, toolObserverKey, observer); err != nil {
				panic(err)
			}
		}
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
	return options
}

func (e *Engine) steeringGuardrails(steer SteerSource) *core.ChatGuardrails {
	if steer == nil {
		return nil
	}

	guardrails, err := newChatGuardrailsWithBeforeRound(
		e.historyStore,
		func(_ context.Context) []chat.Message {
			return steer()
		},
	)
	if err != nil {
		return nil
	}
	return guardrails
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
	options := turnProcessOptions(e.dependencies, request.SessionID, request.Observer, request.EventListener, request.ChatClient, nil)
	process, err := e.turnRestorer.Restore(ctx, processID, options)
	if err != nil {
		return nil, fmt.Errorf("engine: restore chat: %w", err)
	}
	return &turnProcess{process: process, engine: e.turnControl}, nil
}
