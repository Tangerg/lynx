// Package schedule is the scheduled-run domain: a Schedule fires saved instructions
// on a cron trigger as a headless run (no client present). A firing claims
// schedules whose time has come, starts a run, and records the occurrence.
//
// A Schedule stores the final instruction text, not a recipe reference — the
// scheduler is deliberately decoupled from any authoring source, so deleting or
// renaming that source cannot break a schedule.
package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// IDPrefix is the type prefix every schedule id carries (mirrors the session /
// run id convention).
const IDPrefix = "sch_"

// ErrNotFound is returned when a schedule lookup cannot find an id.
var ErrNotFound = errors.New("schedule: not found")

// ErrUnavailable is returned when scheduling is disabled for this runtime.
var ErrUnavailable = errors.New("schedule: unavailable")

// ErrRevisionConflict reports that a conditional update targeted a stale
// version of the schedule.
var ErrRevisionConflict = errors.New("schedule: revision conflict")

// Validation sentinels returned by [Schedule.Validate]; callers can classify
// invalid fields without parsing error text.
var (
	// ErrIDRequired — an update target must identify a stored schedule.
	ErrIDRequired = errors.New("schedule: id is required")
	// ErrRevisionRequired — an external update must carry the version it read.
	ErrRevisionRequired = errors.New("schedule: expected revision is required")
	// ErrInstructionsRequired — a schedule with no instructions has nothing to fire.
	ErrInstructionsRequired = errors.New("schedule: instructions is required")
	// ErrCronRequired — a schedule with no cron has no trigger.
	ErrCronRequired = errors.New("schedule: cron is required")
	// ErrInvalidCron — the cron expression is not a supported five-field spec.
	ErrInvalidCron = errors.New("schedule: invalid cron")
	// ErrCWDUnavailable — the configured working directory cannot be admitted.
	ErrCWDUnavailable = errors.New("schedule: cwd unavailable")
)

// Schedule is saved instructions fired on a cron trigger. CWD anchors the headless
// run's tools (empty → the serve directory); ModelSelection is optional (its
// zero value uses the configured default).
type Schedule struct {
	ID             string
	Title          string
	Instructions   string // the final text sent as the run's input
	CWD            string
	ModelSelection modelref.Selection
	Cron           string    // 5-field standard cron: "min hour dom month dow"
	Enabled        bool      // a disabled schedule never fires (NextRunAt cleared)
	LastRunAt      time.Time // zero ⇒ never fired
	NextRunAt      time.Time // next due time, computed from Cron; zero ⇒ not scheduled (disabled)
	CreatedAt      time.Time
	Revision       uint64
}

// Occurrence is one durable cron firing intent. Its schedule snapshot and
// stable session/run identities let a worker resume an interrupted dispatch
// without recreating a second headless session or run for the same due time.
type Occurrence struct {
	ID        string
	Schedule  Schedule
	DueAt     time.Time
	FiredAt   time.Time
	NextRunAt time.Time
	SessionID string
	RunID     string
}

// Patch is a partial update to a Schedule. Nil fields keep the existing value;
// non-nil fields replace it, including replacing a string with "".
type Patch struct {
	Title        *string
	Instructions *string
	CWD          *string
	Provider     *string
	Model        *string
	Cron         *string
	Enabled      *bool
}

// Apply returns s with p applied. It does not recompute NextRunAt; call
// [Schedule.ScheduledAfter] before persisting.
func (s Schedule) Apply(p Patch) (Schedule, error) {
	if p.Title != nil {
		s.Title = *p.Title
	}
	if p.Instructions != nil {
		s.Instructions = *p.Instructions
	}
	if p.CWD != nil {
		s.CWD = *p.CWD
	}
	if p.Provider != nil || p.Model != nil {
		provider, model := s.ModelSelection.Provider(), s.ModelSelection.Model()
		if p.Provider != nil {
			provider = *p.Provider
		}
		if p.Model != nil {
			model = *p.Model
		}
		selection, err := modelref.New(provider, model)
		if err != nil {
			return Schedule{}, err
		}
		s.ModelSelection = selection
	}
	if p.Cron != nil {
		s.Cron = *p.Cron
	}
	if p.Enabled != nil {
		s.Enabled = *p.Enabled
	}
	return s, nil
}

// Validate checks a schedule draft before it is persisted: instructions and a
// parseable cron are required. Create/update call
// it so the rule lives on the entity, not at an input boundary.
func (s Schedule) Validate() error {
	if s.Instructions == "" {
		return ErrInstructionsRequired
	}
	if s.Cron == "" {
		return ErrCronRequired
	}
	return ValidateCron(s.Cron)
}

// ScheduledAfter validates s and returns a copy with NextRunAt matching its
// enabled state. Disabled schedules always have a zero NextRunAt.
func (s Schedule) ScheduledAfter(after time.Time) (Schedule, error) {
	if err := s.Validate(); err != nil {
		return Schedule{}, err
	}
	if !s.Enabled {
		s.NextRunAt = time.Time{}
		return s, nil
	}
	next, err := NextRun(s.Cron, after)
	if err != nil {
		return Schedule{}, err
	}
	s.NextRunAt = next
	return s, nil
}

// ValidateCron reports whether spec is a parseable 5-field cron expression
// (the boundary check create/update run before persisting).
func ValidateCron(spec string) error {
	if _, err := cron.ParseStandard(spec); err != nil {
		return fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	return nil
}

// NextRun returns the first time spec fires strictly after `after`. It is the
// single source of NextRunAt — create/update compute it from the new cron, and
// the worker advances it after each firing (so a schedule missed during
// downtime fires once on restart, then jumps to its next future slot rather
// than replaying every missed occurrence).
func NextRun(spec string, after time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w %q: %w", ErrInvalidCron, spec, err)
	}
	return sched.Next(after), nil
}
