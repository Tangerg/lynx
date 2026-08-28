package runmaintenance

import (
	"context"

	"github.com/Tangerg/scope/models/catalog"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/agentexec"
)

// Pipeline composes the post-Run maintenance workers. It keeps the lifecycle
// policy beside the concrete workers: mine the full transcript for Skill
// proposals, archive idle Skills, compact when needed, then consolidate memory
// only after compaction. Nil workers disable only their own operation.
type Pipeline struct {
	compactor     *Compactor
	consolidator  *MemoryConsolidator
	skillMiner    *SkillProposalMiner
	skillArchiver *IdleSkillArchiver
}

// NewPipeline composes the default maintenance workers for clean Run endings.
func NewPipeline(compactor *Compactor, consolidator *MemoryConsolidator, skillMiner *SkillProposalMiner, skillArchiver *IdleSkillArchiver) *Pipeline {
	return &Pipeline{
		compactor:     compactor,
		consolidator:  consolidator,
		skillMiner:    skillMiner,
		skillArchiver: skillArchiver,
	}
}

// Maintain completes one best-effort maintenance pass. Skill mining precedes
// compaction so it can see the full transcript; memory consolidation follows
// only a summary compaction, matching its cost-amortization policy.
func (p *Pipeline) Maintain(ctx context.Context, input agentexec.RunMaintenanceInput) agentexec.RunMaintenanceResult {
	if p == nil {
		return agentexec.RunMaintenanceResult{}
	}
	result := agentexec.RunMaintenanceResult{}
	if p.skillMiner != nil {
		if err := p.skillMiner.MineIfDue(ctx, input.SessionID, input.CWD, input.ToolCalls); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	if p.skillArchiver != nil {
		if err := p.skillArchiver.ArchiveIfDue(ctx); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	if p.compactor == nil {
		return result
	}

	contextWindow := 0
	maxInputTokens := 0
	if info, ok := catalog.Default.Lookup(input.ModelSelection.Provider(), input.ModelSelection.Model()); ok {
		contextWindow = int(info.Limits.ContextWindow)
		maxInputTokens = int(info.Limits.MaxInputTokens)
	}
	compaction, err := p.compactor.CompactIfNeeded(
		ctx,
		input.SessionID,
		contextWindow,
		maxInputTokens,
		input.PreCompact,
	)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}
	result.Compaction = agentexec.CompactionResult{
		Compacted:      compaction.Compacted,
		MessagesBefore: compaction.MessagesBefore,
		MessagesAfter:  compaction.MessagesAfter,
	}
	if !compaction.Compacted || p.consolidator == nil {
		return result
	}
	if err := p.consolidator.Consolidate(ctx, input.SessionID, input.CWD); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
