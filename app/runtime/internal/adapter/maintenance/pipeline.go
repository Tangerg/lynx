package maintenance

import (
	"context"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// Pipeline is the in-house composition of Run-boundary maintenance. It keeps the
// lifecycle policy beside the concrete workers: mine the full transcript, sweep
// idle skills, compact when needed, then extract facts only after compaction.
// Nil workers disable only their own operation.
type Pipeline struct {
	compactor *Compactor
	extractor *Extractor
	miner     *SkillMiner
	curator   *SkillCurator
}

// NewPipeline composes the default maintenance workers for clean Run endings.
func NewPipeline(compactor *Compactor, extractor *Extractor, miner *SkillMiner, curator *SkillCurator) *Pipeline {
	return &Pipeline{
		compactor: compactor,
		extractor: extractor,
		miner:     miner,
		curator:   curator,
	}
}

// Maintain completes one best-effort maintenance sweep. Mining precedes
// compaction so it can see the full transcript; extraction follows only a
// summary compaction, matching its cost amortization policy.
func (p *Pipeline) Maintain(ctx context.Context, input turn.MaintenanceInput) turn.MaintenanceResult {
	if p == nil {
		return turn.MaintenanceResult{}
	}
	result := turn.MaintenanceResult{}
	if p.miner != nil {
		if err := p.miner.MaybeMine(ctx, input.SessionID, input.CWD, input.ToolCalls); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	if p.curator != nil {
		if err := p.curator.MaybeSweep(ctx); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	if p.compactor == nil {
		return result
	}

	contextWindow := 0
	if info, ok := catalog.Lookup(input.ModelSelection.Provider(), input.ModelSelection.Model()); ok {
		contextWindow = int(info.Limits.ContextWindow)
	}
	compaction, err := p.compactor.MaybeCompact(ctx, input.SessionID, contextWindow, input.PreCompact)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}
	result.Compaction = compaction
	if !compaction.Compacted || p.extractor == nil {
		return result
	}
	if err := p.extractor.MaybeExtract(ctx, input.SessionID, input.CWD); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
