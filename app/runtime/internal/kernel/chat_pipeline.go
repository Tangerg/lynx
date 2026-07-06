package kernel

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"
	historymw "github.com/Tangerg/lynx/core/model/chat/middleware/history"
)

func toolResolverOrEmpty(resolver ToolResolver) ToolResolver {
	if resolver != nil {
		return resolver
	}
	return &emptyToolResolver{}
}

func newAgentPlatform(cfg Config, resolver ToolResolver) (*agentruntime.Platform, error) {
	guardrails, err := newChatGuardrails(cfg)
	if err != nil {
		return nil, err
	}

	return agent.NewPlatform(agentruntime.PlatformConfig{
		ChatClient:   cfg.ChatClient,
		Extensions:   []core.Extension{resolver},
		Guardrails:   guardrails,
		ProcessStore: cfg.ProcessStore,
		AutoSnapshot: cfg.ProcessStore != nil,
		SessionStore: cfg.SessionStore,
	}), nil
}

// newChatGuardrails composes the shared chat pipeline for every top-level
// turn and subtask. The tool loop stays outermost and the history middleware
// stays model-adjacent, so each loop round persists only the genuinely-new
// messages for that conversation id.
func newChatGuardrails(cfg Config) (*core.Guardrails, error) {
	historyStore := cfg.HistoryStore
	if historyStore == nil {
		historyStore = history.NewInMemoryStore()
	}
	historyCallMW, historyStreamMW, err := historymw.NewMiddleware(historyStore)
	if err != nil {
		return nil, fmt.Errorf("engine: build history middleware: %w", err)
	}

	toolCallMW, toolStreamMW := toolloop.NewMiddleware(toolLoopConfig(cfg))
	return &core.Guardrails{
		CallMiddlewares:   []chat.CallMiddleware{toolCallMW, historyCallMW},
		StreamMiddlewares: []chat.StreamMiddleware{toolStreamMW, historyStreamMW},
	}, nil
}

// toolLoopConfig captures Lyra's agent-loop policy: retry an empty model reply
// once, park HITL tool interrupts when a store exists, stop fixed-point tool
// repetition before the iteration cap, and drain mid-run steering before each
// continuation round.
func toolLoopConfig(cfg Config) toolloop.Config {
	return toolloop.Config{
		FeedbackOnEmptyResponse: true,
		ParkStore:               cfg.ParkStore,
		LoopDetection:           &toolloop.LoopDetectionConfig{},
		BeforeRound:             drainSteeringBeforeRound,
	}
}

func drainSteeringBeforeRound(ctx context.Context) []chat.Message {
	if s := steerSourceFrom(ctx); s != nil {
		return s()
	}
	return nil
}
