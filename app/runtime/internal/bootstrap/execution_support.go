package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/models/catalog"
	skillspec "github.com/Tangerg/lynx/skills"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runmaintenance"
	"github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/skillauthoring"
)

func buildRunMaintenance(cfg Config, messages messageEnvironment, shells *exec.Shells, skillStore *skillauthoring.Store, skillProposals *workspace.Skills, resolveUtility func(context.Context) *chatclient.Client, embedder func(context.Context) (agentmemory.Embedder, error)) agentexec.RunMaintenance {
	if cfg.Maintenance != nil {
		return cfg.Maintenance
	}
	window := 0
	if info, ok := catalog.Lookup(cfg.Provider, cfg.Model); ok {
		window = int(info.Limits.ContextWindow)
	}
	compactor := runmaintenance.NewCompactor(
		messages.store,
		resolveUtility,
		runmaintenance.NewLiveStateSnapshotter(shells, cfg.PlanStore),
		runmaintenance.CompactionConfig{ContextWindow: window},
	)
	var consolidator *runmaintenance.MemoryConsolidator
	if cfg.AgentMemoryStore != nil {
		consolidator = runmaintenance.NewMemoryConsolidator(messages.store, cfg.AgentMemoryStore, resolveUtility, embedder, runmaintenance.MemoryCurationConfig{})
	}
	var skillMiner *runmaintenance.SkillProposalMiner
	var skillArchiver *runmaintenance.IdleSkillArchiver
	if skillStore.Enabled() {
		skillMiner = runmaintenance.NewSkillProposalMiner(
			messages.store,
			skillProposals,
			skillspec.Dir(cfg.SkillsUserDir),
			resolveUtility,
			runmaintenance.SkillMiningConfig{},
		)
		skillArchiver = runmaintenance.NewIdleSkillArchiver(skillStore, runmaintenance.SkillArchiveConfig{})
	}
	return runmaintenance.NewPipeline(compactor, consolidator, skillMiner, skillArchiver)
}
