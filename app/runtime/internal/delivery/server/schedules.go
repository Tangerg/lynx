package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

// schedules.* (API.md §7.9) — manage the cron-triggered headless runs the
// scheduler worker (scheduler.go) fires. A schedule stores the final prompt
// text, so the runtime fires it without resolving a recipe.

// ListSchedules returns every schedule, newest-created first (schedules.list).
func (s *Server) ListSchedules(ctx context.Context) (*protocol.ListSchedulesResult, error) {
	scheds, err := s.scheduleList.ListSchedules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Schedule, 0, len(scheds))
	for _, sc := range scheds {
		out = append(out, scheduleToWire(sc))
	}
	return &protocol.ListSchedulesResult{Schedules: out}, nil
}

// CreateSchedule adds an enabled schedule (schedules.create), computing its
// first due time from the cron.
func (s *Server) CreateSchedule(ctx context.Context, in protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
	sc := schedule.Schedule{
		Title:    in.Title,
		Prompt:   in.Prompt,
		Provider: in.Provider,
		Model:    in.Model,
		Cron:     in.Cron,
		Enabled:  true,
	}
	if err := sc.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	cwd, err := scheduleCwdFromWire(in.Cwd)
	if err != nil {
		return nil, err
	}
	sc.Cwd = cwd
	sc.NextRunAt, _ = schedule.NextRun(in.Cron, time.Now()) // cron validated above
	created, err := s.scheduleCreation.CreateSchedule(ctx, sc)
	if err != nil {
		return nil, err
	}
	wire := scheduleToWire(created)
	return &wire, nil
}

// UpdateSchedule full-replaces a schedule's editable fields and recomputes its
// due time from the (new) cron — cleared when disabled so the worker skips it
// (schedules.update).
func (s *Server) UpdateSchedule(ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
	sc := schedule.Schedule{
		ID:       in.ID,
		Title:    in.Title,
		Prompt:   in.Prompt,
		Provider: in.Provider,
		Model:    in.Model,
		Cron:     in.Cron,
		Enabled:  in.Enabled,
	}
	if err := sc.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	cwd, err := scheduleCwdFromWire(in.Cwd)
	if err != nil {
		return nil, err
	}
	existing, err := s.scheduleRead.Schedule(ctx, in.ID)
	if err != nil {
		return nil, mapScheduleErr(err, in.ID)
	}
	sc.Cwd = cwd
	if in.Enabled {
		sc.NextRunAt, _ = schedule.NextRun(in.Cron, time.Now())
	}
	sc.LastRunAt = existing.LastRunAt
	sc.CreatedAt = existing.CreatedAt
	updated, err := s.scheduleUpdates.UpdateSchedule(ctx, sc)
	if err != nil {
		return nil, mapScheduleErr(err, in.ID)
	}
	wire := scheduleToWire(updated)
	return &wire, nil
}

// DeleteSchedule removes a schedule (schedules.delete). Idempotent.
func (s *Server) DeleteSchedule(ctx context.Context, in protocol.DeleteScheduleRequest) error {
	return s.scheduleDeletion.DeleteSchedule(ctx, in.ID)
}

// RunScheduleNow fires a schedule immediately (schedules.runNow) — a manual
// extra run that records the firing without shifting the schedule's next due
// time.
func (s *Server) RunScheduleNow(ctx context.Context, in protocol.RunScheduleNowRequest) error {
	sc, err := s.scheduleRead.Schedule(ctx, in.ID)
	if err != nil {
		return mapScheduleErr(err, in.ID)
	}
	if _, err := schedule.Fire(ctx, scheduleRunner{s}, sc); err != nil {
		return err
	}
	return s.scheduleRuns.RecordScheduleRun(ctx, sc.ID, time.Now().UTC())
}

func scheduleCwdFromWire(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	resolved, err := worktree.ResolveExistingDir(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", protocol.ErrCwdUnavailable, cwd, err)
	}
	return resolved, nil
}

// mapScheduleErr surfaces an unknown-id as invalid_params (the supplied id
// doesn't resolve), passing every other error through unchanged.
func mapScheduleErr(err error, id string) error {
	if errors.Is(err, schedule.ErrNotFound) {
		return fmt.Errorf("%w: schedule %q not found", protocol.ErrInvalidParams, id)
	}
	return err
}

// scheduleToWire maps a domain schedule to its wire shape, projecting the zero
// time (never fired / unscheduled) to an omitted field rather than a fake epoch.
func scheduleToWire(sc schedule.Schedule) protocol.Schedule {
	w := protocol.Schedule{
		ID:        sc.ID,
		Title:     sc.Title,
		Prompt:    sc.Prompt,
		Cwd:       sc.Cwd,
		Provider:  sc.Provider,
		Model:     sc.Model,
		Cron:      sc.Cron,
		Enabled:   sc.Enabled,
		CreatedAt: sc.CreatedAt,
	}
	if !sc.LastRunAt.IsZero() {
		t := sc.LastRunAt
		w.LastRunAt = &t
	}
	if !sc.NextRunAt.IsZero() {
		t := sc.NextRunAt
		w.NextRunAt = &t
	}
	return w
}
