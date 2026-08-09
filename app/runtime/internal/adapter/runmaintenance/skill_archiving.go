package runmaintenance

import (
	"context"
	"sync"
	"time"
)

const (
	defaultSkillArchiveAfter         = 30 * 24 * time.Hour
	defaultSkillArchiveCheckInterval = 6 * time.Hour
)

// SkillArchiveConfig tunes idle-Skill archival. Zero values select defaults.
type SkillArchiveConfig struct {
	// ArchiveAfter is the inactivity before an agent-authored skill is archived.
	ArchiveAfter time.Duration
	// CheckInterval is the minimum wall-clock interval between Run-boundary
	// archival checks, bounding their cost across a busy Session.
	CheckInterval time.Duration
}

func (c SkillArchiveConfig) normalized() SkillArchiveConfig {
	if c.ArchiveAfter <= 0 {
		c.ArchiveAfter = defaultSkillArchiveAfter
	}
	if c.CheckInterval <= 0 {
		c.CheckInterval = defaultSkillArchiveCheckInterval
	}
	return c
}

// idleSkillStore archives agent-authored Skills idle beyond archiveAfter and
// returns the names it archived.
type idleSkillStore interface {
	SweepIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error)
}

// IdleSkillArchiver archives inactive agent-authored Skills at Run boundaries,
// checking at most once per CheckInterval. The managed Skill library is
// user-scoped, so the check is process-wide rather than per Session. The first
// Run after start performs a check, avoiding a startup-time filesystem mutation.
type IdleSkillArchiver struct {
	store  idleSkillStore
	config SkillArchiveConfig
	now    func() time.Time

	mu        sync.Mutex
	lastCheck time.Time
}

// NewIdleSkillArchiver builds an archiver over the managed Skill store.
func NewIdleSkillArchiver(store idleSkillStore, config SkillArchiveConfig) *IdleSkillArchiver {
	return &IdleSkillArchiver{
		store:  store,
		config: config.normalized(),
		now:    time.Now,
	}
}

// ArchiveIfDue archives eligible Skills unless the previous check occurred
// within CheckInterval. The rate-limit window advances even when nothing is
// archived, so a busy Session does not query the store after every Run.
func (a *IdleSkillArchiver) ArchiveIfDue(ctx context.Context) error {
	if a == nil || a.store == nil {
		return nil
	}
	now := a.now()
	a.mu.Lock()
	if !a.lastCheck.IsZero() && now.Sub(a.lastCheck) < a.config.CheckInterval {
		a.mu.Unlock()
		return nil
	}
	a.lastCheck = now
	a.mu.Unlock()
	archived, err := a.store.SweepIdle(ctx, now, a.config.ArchiveAfter)
	recordArchivedIdleSkills(ctx, len(archived))
	return err
}
