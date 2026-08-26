package workspace

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
)

// IdleSkillSweeper is the persistence port for automatic archival policy. The
// second result contains every public file identity changed before return, even
// when a later step fails.
type IdleSkillSweeper interface {
	SweepIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, []string, error)
}

// SkillMaintenance owns automatic Skill-library curation. It is deliberately
// separate from Skills so interactive consumers cannot invoke scheduled work.
type SkillMaintenance struct {
	sweeper       IdleSkillSweeper
	observations  *AuthoredWatch
	invalidations invalidation.Publish
}

// NewSkillMaintenance builds the automatic Skill-library curation use case.
func NewSkillMaintenance(sweeper IdleSkillSweeper, observations *AuthoredWatch, invalidations invalidation.Publish) *SkillMaintenance {
	return &SkillMaintenance{sweeper: sweeper, observations: observations, invalidations: invalidations}
}

// Available reports whether automatic Skill curation is wired.
func (s *SkillMaintenance) Available() bool { return s != nil && s.sweeper != nil }

// ArchiveIdle applies automatic user-library curation and reports the names it
// archived. A sweep may commit file changes before returning an error, so every
// non-empty identity set invalidates the public Skill projections.
func (s *SkillMaintenance) ArchiveIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error) {
	if !s.Available() {
		return nil, ErrSkillLibraryUnavailable
	}
	archived, identities, err := s.sweeper.SweepIdle(ctx, now, archiveAfter)
	if len(identities) > 0 {
		if s.observations != nil {
			s.observations.Accept(AuthoredChange{Resource: AuthoredSkills, Identities: identities})
		}
		s.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
	}
	return archived, err
}
