package maintenance

import (
	"context"

	"github.com/Tangerg/lynx/models/catalog"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// Suite is the in-house composition of Run-boundary maintenance. It keeps the
// lifecycle policy beside the concrete workers: mine the full transcript, sweep
// idle skills, compact when needed, then extract facts only after compaction.
// Nil workers disable only their own operation.
type Suite struct {
	compactor *Compactor
	extractor *Extractor
	miner     *SkillMiner
	curator   *SkillCurator
}

// NewSuite composes the default maintenance workers for clean Run endings.
func NewSuite(compactor *Compactor, extractor *Extractor, miner *SkillMiner, curator *SkillCurator) *Suite {
	return &Suite{
		compactor: compactor,
		extractor: extractor,
		miner:     miner,
		curator:   curator,
	}
}

// Maintain completes one best-effort maintenance sweep. Mining precedes
// compaction so it can see the full transcript; extraction follows only a
// summary compaction, matching its cost amortization policy.
func (s *Suite) Maintain(ctx context.Context, input turn.RunMaintenanceInput) turn.RunMaintenanceResult {
	if s == nil {
		return turn.RunMaintenanceResult{}
	}
	result := turn.RunMaintenanceResult{}
	if s.miner != nil {
		if err := s.miner.MaybeMine(ctx, input.SessionID, input.Cwd, input.ToolCalls); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	if s.curator != nil {
		if err := s.curator.MaybeSweep(ctx); err != nil {
			result.Errors = append(result.Errors, err)
		}
	}
	if s.compactor == nil {
		return result
	}

	contextWindow := 0
	if info, ok := catalog.Lookup(input.ModelSelection.Provider(), input.ModelSelection.Model()); ok {
		contextWindow = int(info.Limits.ContextWindow)
	}
	compaction, err := s.compactor.MaybeCompact(ctx, input.SessionID, contextWindow, input.PreCompact)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result
	}
	result.Compaction = compaction
	if !compaction.Compacted || s.extractor == nil {
		return result
	}
	if err := s.extractor.MaybeExtract(ctx, input.SessionID, input.Cwd); err != nil {
		result.Errors = append(result.Errors, err)
	}
	return result
}
