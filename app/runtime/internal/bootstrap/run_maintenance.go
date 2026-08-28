package bootstrap

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chatclient"
	skillspec "github.com/Tangerg/scope/skills"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/modelcatalog"
	"github.com/Tangerg/scope/app/runtime/internal/adapter/runmaintenance"
	"github.com/Tangerg/scope/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/scope/app/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/infra/exec"
)

func buildRunMaintenance(
	cfg Config,
	defaultSelection modelref.Selection,
	conversationServices conversationEnvironment,
	shells *exec.Shells,
	skills *workspace.Skills,
	skillMaintenance *workspace.SkillMaintenance,
	memoryCuration *agentmemory.Curation,
	resolveUtility func(context.Context) *chatclient.Client,
) (agentexec.RunMaintenance, agentexec.ModelContextCompactor, error) {
	fallbackLimits := modelref.TokenLimits{}
	limits, found, err := modelcatalog.LookupTokenLimits(defaultSelection)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime: default model token limits: %w", err)
	}
	if found {
		fallbackLimits = limits
	}
	compactor := runmaintenance.NewCompactor(
		conversationServices.messages,
		resolveUtility,
		runmaintenance.NewLiveStateSnapshotter(shells),
		runmaintenance.CompactionConfig{FallbackTokenLimits: fallbackLimits},
	)
	if cfg.Maintenance != nil {
		return cfg.Maintenance, compactor, nil
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
	return runmaintenance.NewPipeline(compactor, consolidator, skillMiner, skillArchiver), compactor, nil
}
