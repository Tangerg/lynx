// Package schedule is the scheduled-run domain: a Schedule fires a saved prompt
// on a cron trigger as a headless run (no client present). The worker ticks,
// asks [WorkerStore.Due] for the schedules whose time has come, starts a run
// through a [Runner] port, and records the firing via [WorkerStore.MarkFired].
//
// A Schedule stores the final PROMPT text, not a recipe reference — the
// scheduler is deliberately decoupled from recipes (a client may pre-fill the
// prompt from a recipe, but a deleted/renamed recipe can't break a schedule).
package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// IDPrefix is the type prefix every schedule id carries (mirrors the session /
// run id convention).
const IDPrefix = "sch_"

// ErrNotFound is returned by [Registry.Get] / [Registry.Update] for an unknown id.
var ErrNotFound = errors.New("schedule: not found")

// ErrUnavailable is returned when scheduling is disabled for this runtime.
var ErrUnavailable = errors.New("schedule: unavailable")

// Validation sentinels returned by [Schedule.Validate]; the delivery adapter
// maps them to the protocol's invalid_params.
var (
	// ErrPromptRequired — a schedule with no prompt has nothing to fire.
	ErrPromptRequired = errors.New("schedule: prompt is required")
	// ErrCronRequired — a schedule with no cron has no trigger.
	ErrCronRequired = errors.New("schedule: cron is required")
	// ErrIncompleteModelSelection — provider and model must be set together
	// (both to pin a model, neither for the runtime default — the same rule
	// runs.start applies; provider is never inferred from model).
	ErrIncompleteModelSelection = errors.New("schedule: provider and model must be set together")
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
		return fmt.Errorf("schedule: invalid cron %q: %w", spec, err)
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
		return time.Time{}, fmt.Errorf("schedule: invalid cron %q: %w", spec, err)
	}
	return sched.Next(after), nil
}

// Registry is the schedule persistence + due-query contract. All methods are
// safe for concurrent use; the sqlite-backed implementation satisfies it.
type Registry interface {
	// List returns every schedule, newest-created first.
	List(ctx context.Context) ([]Schedule, error)
	// Get returns one schedule by id, or [ErrNotFound].
	Get(ctx context.Context, id string) (Schedule, error)
	// Create assigns an id + CreatedAt and persists s verbatim (the caller sets
	// Enabled + the computed NextRunAt). Returns the stored schedule.
	Create(ctx context.Context, s Schedule) (Schedule, error)
	// Update replaces the schedule with s.ID (full-replace of the editable
	// fields + the recomputed NextRunAt). [ErrNotFound] for an unknown id.
	Update(ctx context.Context, s Schedule) (Schedule, error)
	// Delete drops a schedule by id. Idempotent (a missing id is not an error).
	Delete(ctx context.Context, id string) error
	// Due returns the enabled schedules whose NextRunAt has come (in (0, now]),
	// newest-due first — the worker's per-tick work list.
	Due(ctx context.Context, now time.Time) ([]Schedule, error)
	// MarkFired records a scheduled firing: the run time (LastRunAt) and the
	// advanced next due time. Only the worker calls it. prevNextRunAt is the
	// NextRunAt the worker saw when it picked this schedule as due — the cursor is
	// advanced only if it still holds, so a concurrent [Registry.Update] that
	// rescheduled (new cron → new NextRunAt) between that read and now wins instead
	// of being clobbered with a value computed from the stale cron. If the guard
	// misses, the firing is still recorded (LastRunAt) without rewinding the
	// cursor. A manual run-now uses [Registry.RecordRun] instead, so the two never
	// write NextRunAt with conflicting intent.
	MarkFired(ctx context.Context, id string, ranAt, prevNextRunAt, nextRunAt time.Time) error
	// RecordRun records an off-cycle run (schedules.runNow): it updates LastRunAt
	// and leaves NextRunAt untouched. Separate from MarkFired so a manual run can
	// never rewind the cron cursor — re-stamping a cursor value read before the
	// worker advanced it would race that advance and re-fire the schedule.
	RecordRun(ctx context.Context, id string, ranAt time.Time) error
}
