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
	steering    turn.SteeringSink
	maintenance turn.BoundaryMaintenance
}

func buildTurnServices(cfg Config, messages messageEnvironment, shells *exec.Shells, skillStore *skillauthoring.Store, resolveUtility func(context.Context) *chatclient.Client, embedder func(context.Context) (agentmemory.Embedder, error)) turnServices {
	services := turnServices{
		steering:    cfg.Steering,
		maintenance: cfg.Maintenance,
	}
	if services.steering == nil {
		services.steering = messages.conversation
	}
	if services.maintenance != nil {
		return services
	}
	window := 0
	if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
		window = int(info.Limits.ContextWindow)
	}
	compactor := maintenance.NewCompactor(
		messages.store,
		resolveUtility,
		maintenance.NewLiveState(shells, cfg.TodoStore),
		maintenance.CompactionConfig{ContextWindow: window},
	)
	var extractor *maintenance.Extractor
	if cfg.AgentMemoryStore != nil {
		extractor = maintenance.NewExtractor(messages.store, cfg.AgentMemoryStore, resolveUtility, embedder, maintenance.CurationConfig{})
	}
	var miner *maintenance.SkillMiner
	var curator *maintenance.SkillCurator
	if skillStore.Enabled() {
		miner = maintenance.NewSkillMiner(
			messages.store,
			skillStore,
			skillspec.Dir(cfg.SkillsGlobalDir),
			resolveUtility,
			maintenance.MinerConfig{},
		)
		curator = maintenance.NewSkillCurator(skillStore, maintenance.LifecycleConfig{})
	}
	services.maintenance = maintenance.NewSuite(compactor, extractor, miner, curator)
	return services
}
