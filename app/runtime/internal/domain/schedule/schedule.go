// Package schedule is the scheduled-run domain: a Schedule fires a saved prompt
// on a cron trigger as a headless run (no client present). The scheduler worker
// (delivery layer) ticks, asks [Service.Due] for the schedules whose time has
// come, starts a run for each, and records the firing via [Service.MarkFired].
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

// ErrNotFound is returned by [Service.Get] / [Service.Update] for an unknown id.
var ErrNotFound = errors.New("schedule: not found")

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

// Service is the schedule persistence + due-query contract. All methods are
// safe for concurrent use; the sqlite-backed implementation satisfies it.
type Service interface {
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
	// advanced next due time. Only the worker calls it — it always advances the
	// cron cursor to the next occurrence. A manual run-now uses [Service.RecordRun]
	// instead, so the two never write NextRunAt with conflicting intent.
	MarkFired(ctx context.Context, id string, ranAt, nextRunAt time.Time) error
	// RecordRun records an off-cycle run (schedules.runNow): it updates LastRunAt
	// and leaves NextRunAt untouched. Separate from MarkFired so a manual run can
	// never rewind the cron cursor — re-stamping a cursor value read before the
	// worker advanced it would race that advance and re-fire the schedule.
	RecordRun(ctx context.Context, id string, ranAt time.Time) error
}
