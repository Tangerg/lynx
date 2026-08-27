package bootstrap

import (
	"context"

	"github.com/Tangerg/scope/core/chatclient"
	"github.com/Tangerg/scope/models/catalog"
	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/runmaintenance"
	"github.com/Tangerg/scope/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/infra/exec"
)

func buildRunMaintenance(
	cfg Config,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills *workspace.Skills,
	skillMaintenance *workspace.SkillMaintenance,
	memoryCuration *agentmemory.Curation,
	resolveUtility func(context.Context) *chatclient.Client,
) (agentexec.RunMaintenance, agentexec.ModelContextCompactor) {
	window := 0
	if info, ok := catalog.Default.Lookup(cfg.Provider, cfg.Model); ok {
		window = int(info.Limits.ContextWindow)
	}
	compactor := runmaintenance.NewCompactor(
		conversationServices.messages,
		resolveUtility,
		runmaintenance.NewLiveStateSnapshotter(shells, cfg.PlanStore),
		runmaintenance.CompactionConfig{ContextWindow: window},
	)
	if cfg.Maintenance != nil {
		return cfg.Maintenance, compactor
	}
	var consolidator *runmaintenance.MemoryConsolidator
	if memoryCuration.Available() {
		consolidator = runmaintenance.NewMemoryConsolidator(conversationServices.store, memoryCuration, resolveUtility, runmaintenance.MemoryCurationConfig{})
	}
	var skillMiner *runmaintenance.SkillProposalMiner
	var skillArchiver *runmaintenance.IdleSkillArchiver
	if skillMaintenance.Available() {
		skillMiner = runmaintenance.NewSkillProposalMiner(
			conversationServices.store,
			skills,
			skillspec.NewDirectoryRepository(cfg.SkillsUserDir),
			resolveUtility,
			runmaintenance.SkillMiningConfig{},
		)
		skillArchiver = runmaintenance.NewIdleSkillArchiver(skillMaintenance, runmaintenance.SkillArchiveConfig{})
	}
	return runmaintenance.NewPipeline(compactor, consolidator, skillMiner, skillArchiver), compactor
}
