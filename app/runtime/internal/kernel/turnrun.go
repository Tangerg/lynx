package kernel

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/model/chat"
)

// RunTurnRequest carries the per-turn parameters for [Engine.StartTurn] /
// [Engine.RunTurn]. SessionID is non-empty to bind the turn to a
// chat history keyed conversation; Observer is non-nil to receive streaming
// notifications.
type RunTurnRequest struct {
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
	// inherit it: Blackboard.Spawn copies protected entries to children and
	// the typed-action ClearBlackboard preserves them. Empty falls back to
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
	// — registered as a [core.ChatClientProvider] on the process so the
	// agent runtime uses it instead of the platform's default client. This
	// is how a per-run model selection reaches the turn (the caller resolves
	// the right provider+model client). nil uses the platform default.
	ChatClient core.ChatClient

	// Observer receives streaming tool-call + text-delta
	// notifications. May be nil — the turn still runs.
	Observer toolObserver

	// Steer, when non-nil, is drained before each continuation tool round and
	// its messages injected into the running loop (mid-run steering, API.md §6)
	// — so a user message sent while the turn is mid-tool-loop reaches the model
	// on the next round, not the next turn. nil disables mid-run injection.
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
	// Names must be unique across the process extension slice — the
	// runtime panics on collisions at registration time.
	EventListener core.Extension
}

// StartTurn dispatches a turn as an async agent process and
// returns the [TurnProcess] handle the caller drives. The lifecycle
// — cancel, status, awaiting completion, output extraction — runs
// against the agent runtime's [runtime.AgentProcess] rather than a
// bare goroutine, so HITL integration (plan approval, tool approval)
// drops in on the same Process via [runtime.Platform.ResumeProcess].
//
// Observer / SessionID wiring matches [Engine.RunTurn]: Observer
// attaches a process-scope [core.ToolDecorator]; SessionID binds the
// turn to the chat history middleware's keyed conversation.
func (e *Engine) StartTurn(ctx context.Context, req RunTurnRequest) TurnProcess {
	in := turnInput{Message: req.Message, Provider: req.Provider, Media: req.Media, Cwd: req.Cwd, SessionID: req.SessionID, MaxBudget: req.MaxBudget, MaxCostUSD: req.MaxCostUSD, MaxSteps: req.MaxSteps, Options: req.Options.Clone()}

	opts := turnProcessOptions(req.SessionID, req.Observer, req.EventListener, req.ChatClient)
	if req.Steer != nil {
		// Carried as a process-scope extension (not the serializable blackboard
		// — it's a live func): runTurn resolves it and stashes it on the
		// per-round context for the tool loop's BeforeRound hook.
		opts.Extensions = append(opts.Extensions, steerExtension{source: req.Steer})
	}
	proc, done := e.platform.StartAgent(ctx, e.agent,
		map[string]any{core.DefaultBindingName: in},
		opts,
	)
	return &turnProcess{proc: proc, done: done, platform: e.platform}
}

// turnProcessOptions assembles per-process wiring: the chat history Session
// binding, the observer decorator, lifecycle listener, and per-run model
// client. The chat middleware chain itself (tool loop + memory) is the
// platform default built once in [New]; the runtime stamps each request's
// conversation id from this Session, so a single shared chain serves both
// this turn and any subtask it spawns.
func turnProcessOptions(sessionID string, observer toolObserver, listener core.Extension, client core.ChatClient) core.ProcessOptions {
	opts := core.ProcessOptions{}
	if sessionID != "" {
		opts.Session = &core.Session{ID: sessionID}
	}
	if observer != nil {
		opts.Extensions = append(opts.Extensions, &toolObserverDecorator{observer: observer})
	}
	if listener != nil {
		opts.Extensions = append(opts.Extensions, listener)
	}
	if client != nil {
		opts.Extensions = append(opts.Extensions, perRunChatClient{client: client})
	}
	return opts
}

// perRunChatClient is a [core.ChatClientProvider] carrying one resolved
// client for a single turn — the seam that lets a run pick its model.
type perRunChatClient struct{ client core.ChatClient }

func (perRunChatClient) Name() string { return "lyra:per-run-chat-client" }
func (p perRunChatClient) ChatClientFor(core.Process) core.ChatClient {
	return p.client
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
	// the interrupt's persisted provider+model. nil runs on the platform default
	// (a run that didn't pick a model, or one whose provider is no longer
	// configured). Same seam as [RunTurnRequest.ChatClient] on a fresh turn.
	ChatClient core.ChatClient
}

// RestoreTurn rebuilds the agent process identified by processID from the
// configured ProcessStore snapshot and re-parks it, ready for Resume. It
// performs the first two steps of the restore-resume protocol (see
// [runtime.RestoreProcess]): RestoreProcess with the supplied wiring, then
// one ContinueProcess re-tick so the idempotent awaiting action re-issues
// AwaitInput against the restored blackboard (the handler closure does not
// round-trip). The returned [TurnProcess] is StatusWaiting with
// PendingAwaitable populated; the caller drives Resume(approved) to deliver
// the decision and run the continuation to terminal.
//
// Errors when no ProcessStore is configured, the snapshot is missing, the
// agent is not deployed under the snapshot's name, or the re-tick fails.
func (e *Engine) RestoreTurn(ctx context.Context, processID string, req RestoreTurnRequest) (TurnProcess, error) {
	// The restored continuation runs against req.ChatClient — the per-run model
	// re-resolved from the interrupt's persisted provider+model — so a restart
	// mid-run keeps the model the turn parked on. nil (no selection / provider
	// gone) falls back to the platform default.
	opts := turnProcessOptions(req.SessionID, req.Observer, req.EventListener, req.ChatClient)
	proc, err := e.platform.RestoreProcess(ctx, processID, opts)
	if err != nil {
		return nil, fmt.Errorf("engine: restore chat: %w", err)
	}
	// Re-tick: the awaitable handler closure didn't survive the snapshot,
	// so the idempotent gate action re-parks against the restored
	// blackboard, repopulating PendingAwaitable for the upcoming Resume.
	if err := e.platform.ContinueProcess(ctx, proc.ID()); err != nil {
		return nil, fmt.Errorf("engine: restore chat re-tick: %w", err)
	}
	return &turnProcess{proc: proc, platform: e.platform}, nil
}

// RunTurn is the synchronous wrapper kept for callers that don't
// need the [TurnProcess] handle (engine tests, CLI smoke runs).
// Newer call sites should use [Engine.StartTurn] directly.
func (e *Engine) RunTurn(ctx context.Context, req RunTurnRequest) (TurnOutput, error) {
	cp := e.StartTurn(ctx, req)
	if err := <-cp.Done(); err != nil {
		return TurnOutput{}, fmt.Errorf("engine: run turn: %w", err)
	}
	return cp.Output()
}
