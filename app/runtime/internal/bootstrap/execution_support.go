package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/models/catalog"
	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/maintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

type executionSupport struct {
	steering    turn.SteeringSink
	maintenance turn.RunMaintenance
}

func buildExecutionSupport(cfg Config, messages messageEnvironment, shells *exec.Shells, skillStore *skillauthoring.Store, skillProposals *workspace.Skills, resolveUtility func(context.Context) *chatclient.Client, embedder func(context.Context) (agentmemory.Embedder, error)) executionSupport {
	support := executionSupport{
		steering:    cfg.Steering,
		maintenance: cfg.Maintenance,
	}
	if support.steering == nil {
		support.steering = messages.conversation
	}
	if support.maintenance != nil {
		return support
	}
	window := 0
	if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
		window = int(info.Limits.ContextWindow)
	}
	compactor := maintenance.NewCompactor(
		messages.store,
		resolveUtility,
		maintenance.NewLiveState(shells, cfg.PlanStore),
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
			skillProposals,
			skillspec.Dir(cfg.SkillsUserDir),
			resolveUtility,
			maintenance.MinerConfig{},
		)
		curator = maintenance.NewSkillCurator(skillStore, maintenance.LifecycleConfig{})
	}
	support.maintenance = maintenance.NewSuite(compactor, extractor, miner, curator)
	return support
}
