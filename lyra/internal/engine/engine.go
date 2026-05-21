package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

// Config is the engine construction-time bundle. All fields are
// required — engine assumes its dependencies are wired before
// construction.
type Config struct {
	// ChatClient is the LLM client used by every action. Built from
	// a lynx model adapter (anthropic, openai, ...) at startup.
	ChatClient *chat.Client

	// Workdir is the filesystem root every Lyra-shipped tool is
	// scoped to. Empty string disables the scoping (LocalExecutor
	// permits any path) — fine for tests, not recommended for
	// production. Typical value: the user's project cwd.
	Workdir string
}

// Engine is the runtime container. Holds the lynx Platform and the
// minimal Agent definition used for single-turn chat. Future
// milestones extend it (more agents, tool resolvers, blackboard
// providers, ...).
type Engine struct {
	platform *runtime.Platform
	agent    *core.Agent
	tools    []chat.Tool
}

// New constructs an engine. Returns an error when required deps
// are missing or when agent deployment fails.
func New(cfg Config) (*Engine, error) {
	if cfg.ChatClient == nil {
		return nil, errors.New("engine: ChatClient is required")
	}

	resolver := buildCodingResolver(cfg.Workdir)
	tools := BuildCodingTools(cfg.Workdir)

	memStore := memory.NewInMemoryStore()
	callMW, streamMW, mwErr := memory.NewMiddleware(memStore)
	if mwErr != nil {
		return nil, fmt.Errorf("engine: build memory middleware: %w", mwErr)
	}
	platform := agent.NewPlatform(&runtime.PlatformConfig{
		ChatClient: cfg.ChatClient,
		Extensions: []core.Extension{resolver},
		Guardrails: &core.Guardrails{
			CallMiddlewares:   []chat.CallMiddleware{callMW},
			StreamMiddlewares: []chat.StreamMiddleware{streamMW},
		},
	})

	a := buildChatAgent()
	if err := platform.Deploy(a); err != nil {
		return nil, fmt.Errorf("engine: deploy chat agent: %w", err)
	}

	return &Engine{
		platform: platform,
		agent:    a,
		tools:    tools,
	}, nil
}

// Tools returns the registered coding tool set — used by
// ToolService.List to surface tool metadata to clients without
// re-running the construction.
func (e *Engine) Tools() []chat.Tool { return e.tools }

// Platform exposes the underlying lynx platform for service
// implementations that need fine-grained control (most don't —
// prefer the high-level helpers below).
func (e *Engine) Platform() *runtime.Platform { return e.platform }

// ChatAgent returns the minimal "single-turn chat" agent. M2 adds
// tools to this agent's actions; M6 adds planner-aware variants.
func (e *Engine) ChatAgent() *core.Agent { return e.agent }

// RunChatRequest carries the per-turn parameters for [Engine.RunChat].
// sessionID is non-empty to bind the turn to a chat-memory keyed
// conversation; observer is non-nil to receive streaming
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

	// Observer receives streaming tool-call + text-delta
	// notifications. May be nil — the turn still runs.
	Observer ToolObserver
}

// RunChat is the engine's lowest-level chat entry. The runtime
// schedules a process against the configured ChatAgent, blocks on
// completion, and returns the produced reply.
//
// When req.Observer is non-nil the engine attaches a process-scope
// [core.ToolDecorator] that fires OnToolCallStart / OnToolCallEnd
// for every tool the model invokes during the turn, plus
// OnMessageDelta for each streamed text chunk.
//
// When req.SessionID is non-empty the engine binds the turn to a
// chat-memory keyed conversation — the memory middleware auto-loads
// prior turns and saves new messages keyed by SessionID.
func (e *Engine) RunChat(ctx context.Context, req RunChatRequest) (string, error) {
	in := ChatInput{Message: req.Message}

	opts := core.ProcessOptions{}
	if req.SessionID != "" {
		opts.Session = &core.Session{ID: req.SessionID}
	}
	if req.Observer != nil {
		opts.Extensions = []core.Extension{
			&toolObserverDecorator{observer: req.Observer},
		}
	}

	proc, err := e.platform.RunAgent(ctx, e.agent,
		map[string]any{core.DefaultBindingName: in},
		opts,
	)
	if err != nil {
		return "", fmt.Errorf("engine: run chat: %w", err)
	}
	out, ok := core.ResultOfType[ChatOutput](proc)
	if !ok {
		return "", fmt.Errorf("engine: no ChatOutput produced; status=%s failure=%v", proc.Status(), proc.Failure())
	}
	return out.Reply, nil
}
