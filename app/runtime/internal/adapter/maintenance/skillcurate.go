package maintenance

import (
	"context"
	"sync"
	"time"
)

const (
	defaultSkillArchiveAfter = 30 * 24 * time.Hour
	defaultSkillSweepEvery   = 6 * time.Hour
)

// LifecycleConfig tunes the idle-skill curator. Zero values select defaults.
type LifecycleConfig struct {
	// ArchiveAfter is the inactivity before an agent-authored skill is archived.
	ArchiveAfter time.Duration
	// SweepEvery is the minimum wall-clock between turn-boundary sweeps, bounding
	// the sweep cost across a busy session.
	SweepEvery time.Duration
}

func (c LifecycleConfig) normalized() LifecycleConfig {
	if c.ArchiveAfter <= 0 {
		c.ArchiveAfter = defaultSkillArchiveAfter
	}
	if c.SweepEvery <= 0 {
		c.SweepEvery = defaultSkillSweepEvery
	}
	return c
}

// skillSweeper archives agent-authored skills idle beyond archiveAfter and
// returns the names it archived.
type skillSweeper interface {
	SweepIdle(ctx context.Context, now time.Time, archiveAfter time.Duration) ([]string, error)
}

// SkillCurator runs the idle-lifecycle sweep at the turn boundary, rate-limited
// to at most once per SweepEvery. The skill library is global, so the sweep is
// global — not per session; the first turn after start triggers it (lastSweep is
// zero), which stands in for an explicit boot sweep without a startup-time
// filesystem mutation.
type SkillCurator struct {
	sweeper skillSweeper
	config  LifecycleConfig
	now     func() time.Time

	mu        sync.Mutex
	lastSweep time.Time
}

// NewSkillCurator builds the idle-skill curator over the authoring store's
// idle-sweep.
func NewSkillCurator(sweeper skillSweeper, config LifecycleConfig) *SkillCurator {
	return &SkillCurator{
		sweeper: sweeper,
		config:  config.normalized(),
		now:     time.Now,
	}
}

// MaybeSweep runs one idle-lifecycle sweep unless the last one was within
// SweepEvery. The rate-limit window advances whether or not the sweep archives
// anything, so a busy session doesn't re-sweep every turn.
func (c *SkillCurator) MaybeSweep(ctx context.Context) error {
	if c == nil || c.sweeper == nil {
		return nil
	}
	now := c.now()
	c.mu.Lock()
	if !c.lastSweep.IsZero() && now.Sub(c.lastSweep) < c.config.SweepEvery {
		c.mu.Unlock()
		return nil
	}
	c.lastSweep = now
	c.mu.Unlock()
	archived, err := c.sweeper.SweepIdle(ctx, now, c.config.ArchiveAfter)
	recordArchivedSkills(ctx, len(archived))
	return err
}
