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
}

// Engine is the runtime container. Holds the lynx Platform and the
// minimal Agent definition used for single-turn chat. Future
// milestones extend it (more agents, tool resolvers, blackboard
// providers, ...).
type Engine struct {
	platform *runtime.Platform
	agent    *core.Agent
}

// New constructs an engine. Returns an error when required deps
// are missing or when agent deployment fails.
func New(cfg Config) (*Engine, error) {
	if cfg.ChatClient == nil {
		return nil, errors.New("engine: ChatClient is required")
	}

	platform := agent.NewPlatform(&runtime.PlatformConfig{
		ChatClient: cfg.ChatClient,
	})

	a := buildChatAgent()
	if err := platform.Deploy(a); err != nil {
		return nil, fmt.Errorf("engine: deploy chat agent: %w", err)
	}

	return &Engine{
		platform: platform,
		agent:    a,
	}, nil
}

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
// Streaming (the more useful entry for transport adapters) lives
// in chat.Service.StartTurn — it wraps RunChat with a goroutine and
// event channel. RunChat is exposed for tests and synchronous
// callers.
func (e *Engine) RunChat(ctx context.Context, userMessage string) (string, error) {
	in := ChatInput{Message: userMessage}
	proc, err := e.platform.RunAgent(ctx, e.agent,
		map[string]any{core.DefaultBindingName: in},
		core.ProcessOptions{},
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
