package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
)

type turnServices struct {
	steering  turn.SteeringSink
	compactor turn.Compactor
	extractor turn.Extractor
}

func buildTurnServices(cfg Config, messages messageEnvironment, resolveUtility func(context.Context) *chatclient.Client) turnServices {
	services := turnServices{
		steering:  cfg.Steering,
		compactor: cfg.Compactor,
		extractor: cfg.Extractor,
	}
	if services.steering == nil {
		services.steering = messages.conversation
	}
	if services.compactor == nil {
		window := 0
		if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
			window = int(info.Limits.ContextWindow)
		}
		services.compactor = maintenance.NewCompactor(
			messages.history,
			resolveUtility,
			maintenance.CompactionConfig{ContextWindow: window},
		)
	}
	if services.extractor == nil && cfg.Engine.Knowledge != nil {
		services.extractor = maintenance.NewExtractor(messages.history, cfg.Engine.Knowledge, resolveUtility)
	}
	return services
}
