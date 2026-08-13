package workspace

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
)

// IdleSkillSweeper is the persistence port for automatic archival policy.
type IdleSkillSweeper interface {
	SweepIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error)
}

// SkillMaintenance owns automatic Skill-library curation. It is deliberately
// separate from Skills so interactive consumers cannot invoke scheduled work.
type SkillMaintenance struct {
	sweeper       IdleSkillSweeper
	invalidations invalidation.Publish
}

// NewSkillMaintenance builds the automatic Skill-library curation use case.
func NewSkillMaintenance(sweeper IdleSkillSweeper, invalidations invalidation.Publish) *SkillMaintenance {
	return &SkillMaintenance{sweeper: sweeper, invalidations: invalidations}
}

// Available reports whether automatic Skill curation is wired.
func (m *SkillMaintenance) Available() bool { return m != nil && m.sweeper != nil }

// ArchiveIdle applies automatic user-library curation and reports the names it
// archived. A sweep may commit some moves before returning an error, so every
// non-empty result invalidates the public Skill projections.
func (m *SkillMaintenance) ArchiveIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error) {
	if !m.Available() {
		return nil, ErrSkillLibraryUnavailable
	}
	archived, err := m.sweeper.SweepIdle(ctx, now, archiveAfter)
	if len(archived) > 0 {
		m.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
	}
	return archived, err
}
