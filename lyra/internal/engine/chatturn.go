package engine

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
)

// RunChatRequest carries the per-turn parameters for [Engine.StartChat] /
// [Engine.RunChat]. SessionID is non-empty to bind the turn to a
// chat-memory keyed conversation; Observer is non-nil to receive streaming
// notifications.
type RunChatRequest struct {
	// SessionID anchors the turn to a chat-memory conversation. The
	// memory middleware reads this via [memory.ConversationIDKey] to
	// pull prior history before the model call and to save the new
	// round afterwards. Empty string runs the turn unattached (each
	// call starts fresh).
	SessionID string

	// Message is the user's input for this turn.
	Message string

	// MaxBudget caps the total tokens (prompt + completion) the turn
	// may spend across its tool-loop rounds. 0 means unlimited. See
	// [ChatInput.MaxBudget] for the stop semantics.
	MaxBudget int64

	// MaxCostUSD caps the turn's dollar cost (0 = no cap). See
	// [ChatInput.MaxCostUSD] — requires a [Config.Pricing] hook.
	MaxCostUSD float64

	// PlanMode runs the turn behind plan approval — see
	// [ChatInput.PlanMode]. The process parks on AwaitInput after
	// drafting a plan; drive it back with [ChatProcess.Resume].
	PlanMode bool

	// Observer receives streaming tool-call + text-delta
	// notifications. May be nil — the turn still runs.
	Observer ToolObserver

	// EventListener, when non-nil, is registered as a process-scope
	// extension. Values that also implement [event.Listener] (i.e.
	// have OnEvent) receive every agent runtime event for this turn
	// — process lifecycle (Created / Completed / Failed / Killed /
	// Stuck / Terminated), action execution, ready-to-plan, etc.
	// The canonical wrapper is [event.NamedListener]; chat.Service
	// uses one to map process terminal events onto TurnEnd reasons
	// without re-deriving status from the run loop's error.
	//
	// Names must be unique across the process extension slice — the
	// runtime panics on collisions at registration time.
	EventListener core.Extension
}

// StartChat dispatches a chat turn as an async agent process and
// returns the [ChatProcess] handle the caller drives. The lifecycle
// — cancel, status, awaiting completion, output extraction — runs
// against the agent runtime's [runtime.AgentProcess] rather than a
// bare goroutine, so HITL integration (plan approval, tool approval)
// drops in on the same Process via [runtime.Platform.ResumeProcess].
//
// Observer / SessionID wiring matches [Engine.RunChat]: Observer
// attaches a process-scope [core.ToolDecorator]; SessionID binds the
// turn to the chat-memory middleware's keyed conversation.
func (e *Engine) StartChat(ctx context.Context, req RunChatRequest) ChatProcess {
	in := ChatInput{Message: req.Message, MaxBudget: req.MaxBudget, MaxCostUSD: req.MaxCostUSD, PlanMode: req.PlanMode}

	proc, done := e.platform.StartAgent(ctx, e.agent,
		map[string]any{core.DefaultBindingName: in},
		chatProcessOptions(req.SessionID, req.Observer, req.EventListener),
	)
	return &chatProcess{proc: proc, done: done, platform: e.platform}
}

// chatProcessOptions assembles the per-process wiring a chat turn runs
// with — its chat-memory Session binding plus the session-scoped
// extensions (the tool observer decorator + the lifecycle event listener).
// StartChat and RestoreChat share it so a resumed turn is wired exactly
// like a fresh one; if they diverged, the continuation would observe /
// persist differently than the original turn.
func chatProcessOptions(sessionID string, observer ToolObserver, listener core.Extension) core.ProcessOptions {
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
	return opts
}

// RestoreChatRequest carries the per-process wiring to re-attach to a
// turn rebuilt from a snapshot — the same Observer + Session a fresh turn
// gets from [Engine.StartChat], so the resumed continuation streams and
// keys chat-memory to the right conversation.
type RestoreChatRequest struct {
	// SessionID rebinds the restored process to its chat-memory
	// conversation (so the continuation's LLM round loads + saves the
	// right history). Empty runs unattached.
	SessionID string

	// Observer receives the continuation's streaming tool-call + text
	// deltas, exactly as on a fresh turn. May be nil.
	Observer ToolObserver

	// EventListener captures the restored process's terminal event so the
	// resumed turn can map it onto a TurnEnd reason. May be nil.
	EventListener core.Extension
}

// RestoreChat rebuilds the agent process identified by processID from the
// configured ProcessStore snapshot and re-parks it, ready for Resume. It
// performs the first two steps of the restore-resume protocol (see
// [runtime.RestoreProcess]): RestoreProcess with the supplied wiring, then
// one ContinueProcess re-tick so the idempotent awaiting action re-issues
// AwaitInput against the restored blackboard (the handler closure does not
// round-trip). The returned [ChatProcess] is StatusWaiting with
// PendingAwaitable populated; the caller drives Resume(approved) to deliver
// the decision and run the continuation to terminal.
//
// Errors when no ProcessStore is configured, the snapshot is missing, the
// agent is not deployed under the snapshot's name, or the re-tick fails.
func (e *Engine) RestoreChat(ctx context.Context, processID string, req RestoreChatRequest) (ChatProcess, error) {
	opts := chatProcessOptions(req.SessionID, req.Observer, req.EventListener)
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
	return &chatProcess{proc: proc, platform: e.platform}, nil
}

// RunChat is the synchronous wrapper kept for callers that don't
// need the [ChatProcess] handle (engine tests, CLI smoke runs).
// Newer call sites should use [Engine.StartChat] directly.
func (e *Engine) RunChat(ctx context.Context, req RunChatRequest) (ChatOutput, error) {
	cp := e.StartChat(ctx, req)
	if err := <-cp.Done(); err != nil {
		return ChatOutput{}, fmt.Errorf("engine: run chat: %w", err)
	}
	return cp.Output()
}
