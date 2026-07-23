package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/models/catalog"
	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

type turnServices struct {
	steering  turn.SteeringSink
	compactor turn.Compactor
	extractor turn.Extractor
	miner     turn.SkillMiner
	curator   turn.SkillCurator
}

func buildTurnServices(cfg Config, messages messageEnvironment, shells *exec.Shells, skillStore *skillauthoring.Store, resolveUtility func(context.Context) *chatclient.Client, embedder func(context.Context) (agentmemory.Embedder, error)) turnServices {
	services := turnServices{
		steering:  cfg.Steering,
		compactor: cfg.Compactor,
		extractor: cfg.Extractor,
		miner:     cfg.Miner,
		curator:   cfg.SkillCurator,
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
			messages.store,
			resolveUtility,
			maintenance.NewLiveState(shells, cfg.TodoStore),
			maintenance.CompactionConfig{ContextWindow: window},
		)
	}
	if services.extractor == nil && cfg.AgentMemoryStore != nil {
		services.extractor = maintenance.NewExtractor(messages.store, cfg.AgentMemoryStore, resolveUtility, embedder, maintenance.CurationConfig{})
	}
	if services.miner == nil && skillStore.Enabled() {
		services.miner = maintenance.NewSkillMiner(
			messages.store,
			skillStore,
			skillspec.Dir(cfg.SkillsGlobalDir),
			resolveUtility,
			maintenance.MinerConfig{},
		)
	}
	if services.curator == nil && skillStore.Enabled() {
		services.curator = maintenance.NewSkillCurator(skillStore, maintenance.LifecycleConfig{})
	}
	return services
}
