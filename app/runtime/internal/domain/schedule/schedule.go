// Package schedule is the scheduled-run domain: a Schedule fires a saved prompt
// on a cron trigger as a headless run (no client present). The application
// worker asks its persistence port for schedules whose time has come, starts a
// run, and records the firing.
//
// A Schedule stores the final PROMPT text, not a recipe reference — the
// scheduler is deliberately decoupled from recipes (a client may pre-fill the
// prompt from a recipe, but a deleted/renamed recipe can't break a schedule).
package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// IDPrefix is the type prefix every schedule id carries (mirrors the session /
// run id convention).
const IDPrefix = "sch_"

// ErrNotFound is returned when an application schedule lookup cannot find an id.
var ErrNotFound = errors.New("schedule: not found")

// ErrUnavailable is returned when scheduling is disabled for this runtime.
var ErrUnavailable = errors.New("schedule: unavailable")

// ErrRevisionConflict reports that a conditional update targeted a stale
// version of the schedule.
var ErrRevisionConflict = errors.New("schedule: revision conflict")

// Validation sentinels returned by [Schedule.Validate]; the delivery adapter
// maps them to the protocol's invalid_params.
var (
	// ErrIDRequired — an update target must identify a stored schedule.
	ErrIDRequired = errors.New("schedule: id is required")
	// ErrRevisionRequired — an external update must carry the version it read.
	ErrRevisionRequired = errors.New("schedule: expected revision is required")
	// ErrPromptRequired — a schedule with no prompt has nothing to fire.
	ErrPromptRequired = errors.New("schedule: prompt is required")
	// ErrCronRequired — a schedule with no cron has no trigger.
	ErrCronRequired = errors.New("schedule: cron is required")
	// ErrIncompleteModelSelection — provider and model must be set together
	// (both to pin a model, neither for the runtime default — the same rule
	// runs.start applies; provider is never inferred from model).
	ErrIncompleteModelSelection = errors.New("schedule: provider and model must be set together")
	// ErrInvalidCron — the cron expression is not a supported five-field spec.
	ErrInvalidCron = errors.New("schedule: invalid cron")
	// ErrCwdUnavailable — the configured working directory cannot be admitted.
	ErrCwdUnavailable = errors.New("schedule: cwd unavailable")
)

// Schedule is a saved prompt fired on a cron trigger. Cwd anchors the headless
// run's tools (empty → the serve directory); Provider/Model are optional (empty
// → the runtime default, paired — both or neither).
type Schedule struct {
	ID        string
	Title     string
	Prompt    string // the final text sent as the run's input
	Cwd       string
	Provider  string
	Model     string
	Cron      string    // 5-field standard cron: "min hour dom month dow"
	Enabled   bool      // a disabled schedule never fires (NextRunAt cleared)
	LastRunAt time.Time // zero ⇒ never fired
	NextRunAt time.Time // next due time, computed from Cron; zero ⇒ not scheduled (disabled)
	CreatedAt time.Time
	Revision  uint64
}

// Patch is a partial update to a Schedule. Nil fields keep the existing value;
// non-nil fields replace it, including replacing a string with "".
type Patch struct {
	Title    *string
	Prompt   *string
	Cwd      *string
	Provider *string
	Model    *string
	Cron     *string
	Enabled  *bool
}

// Apply returns s with p applied. It does not validate or recompute NextRunAt;
// call [Schedule.ScheduledAfter] before persisting.
func (s Schedule) Apply(p Patch) Schedule {
	if p.Title != nil {
		s.Title = *p.Title
	}
	if p.Prompt != nil {
		s.Prompt = *p.Prompt
	}
	if p.Cwd != nil {
		s.Cwd = *p.Cwd
	}
	if p.Provider != nil {
		s.Provider = *p.Provider
	}
	if p.Model != nil {
		s.Model = *p.Model
	}
	if p.Cron != nil {
		s.Cron = *p.Cron
	}
	if p.Enabled != nil {
		s.Enabled = *p.Enabled
	}
	return s
}

// Validate checks a schedule draft before it is persisted: a prompt and a
// parseable cron are required, and provider/model are paired. Returns one of the
// package's validation sentinels (or a [ValidateCron] error). Create/update call
// it so the rule lives on the entity, not in the protocol adapter.
func (s Schedule) Validate() error {
	if s.Prompt == "" {
		return ErrPromptRequired
	}
	if s.Cron == "" {
		return ErrCronRequired
	}
	if (s.Provider == "") != (s.Model == "") {
		return ErrIncompleteModelSelection
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
