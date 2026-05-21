package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
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

	platform := agent.NewPlatform(&runtime.PlatformConfig{
		ChatClient: cfg.ChatClient,
		Extensions: []core.Extension{resolver},
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

// RunChat is the engine's lowest-level chat entry. The runtime
// schedules a process against the configured ChatAgent, blocks on
// completion, and returns the produced reply.
//
// When observer is non-nil the engine attaches a process-scope
// [core.ToolDecorator] that fires OnToolCallStart / OnToolCallEnd
// for every tool the model invokes during the turn — used by
// chat.Service to surface ToolCallStart / ToolCallEnd events.
//
// Streaming (the more useful entry for transport adapters) lives
// in chat.Service.StartTurn — it wraps RunChat with a goroutine and
// event channel. RunChat is exposed for tests and synchronous
// callers.
func (e *Engine) RunChat(ctx context.Context, userMessage string, observer ToolObserver) (string, error) {
	in := ChatInput{Message: userMessage}

	opts := core.ProcessOptions{}
	if observer != nil {
		opts.Extensions = []core.Extension{
			&toolObserverDecorator{observer: observer},
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
		return "", fmt.Errorf("engine: no ChatOutput produced; status=%s", proc.Status())
	}
	return out.Reply, nil
}
