package kernel

import (
	"context"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	agentruntime "github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
)

func newAgentPlatform(cfg Config, resolver ToolResolver) (*agentruntime.Platform, error) {
	guardrails, err := newChatGuardrails(cfg)
	if err != nil {
		return nil, err
	}

	extensions := make([]core.Extension, 0, 1)
	if resolver != nil {
		extensions = append(extensions, resolver)
	}

	return agent.NewPlatform(agentruntime.PlatformConfig{
		ChatClient:   cfg.ChatClient,
		Extensions:   extensions,
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
	return agentruntime.BuildChatGuardrails(agentruntime.ChatGuardrailsConfig{
		HistoryStore: cfg.HistoryStore,
		ToolLoop: agentruntime.ToolLoopPolicy{
			FeedbackOnEmptyResponse: true,
			ParkStore:               cfg.ParkStore,
			BeforeRound:             drainSteeringBeforeRound,
		},
	})
}

func drainSteeringBeforeRound(ctx context.Context) []chat.Message {
	if s := steerSourceFrom(ctx); s != nil {
		return s()
	}
	return nil
}
