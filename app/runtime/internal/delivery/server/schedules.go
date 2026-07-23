package server

import (
	"context"
	"errors"
	"fmt"

	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// schedules.* (API.md §7.9) — manage the cron-triggered headless runs the
// application scheduler fires. A schedule stores the final prompt
// text, so the runtime fires it without resolving a recipe.

// ListSchedules returns every schedule, newest-created first (schedules.list).
func (s *Server) ListSchedules(ctx context.Context, query protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
	scheds, err := s.schedules.List(ctx)
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.list", "")
	}
	page, next, err := pageByCursor(scheds, func(sc schedule.Schedule) string { return sc.ID }, query.Cursor, query.Limit, 100)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Schedule, 0, len(page))
	for _, sc := range page {
		out = append(out, scheduleToWire(sc))
	}
	return &protocol.Page[protocol.Schedule]{Data: out, NextCursor: next}, nil
}

// CreateSchedule adds an enabled schedule (schedules.create), computing its
// first due time from the cron.
func (s *Server) CreateSchedule(ctx context.Context, in protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
	created, err := s.schedules.Create(ctx, scheduleapp.CreateCommand{
		Title:    in.Title,
		Prompt:   in.Prompt,
		Cwd:      in.Cwd,
		Provider: in.Provider,
		Model:    in.Model,
		Cron:     in.Cron,
		Enabled:  true,
	})
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.create", "")
	}
	wire := scheduleToWire(created)
	return &wire, nil
}

// UpdateSchedule applies a revision-guarded partial patch. The coordinator
// recomputes due time when cron or enabled changes and clears it when disabled.
func (s *Server) UpdateSchedule(ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
	updated, err := s.schedules.Update(ctx, scheduleapp.UpdateCommand{
		ID:               in.ID,
		ExpectedRevision: in.ExpectedRevision,
		Patch: schedule.Patch{
			Title:    in.Title,
			Prompt:   in.Prompt,
			Cwd:      in.Cwd,
			Provider: in.Provider,
			Model:    in.Model,
			Cron:     in.Cron,
			Enabled:  in.Enabled,
		},
	})
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.update", in.ID)
	}
	wire := scheduleToWire(updated)
	return &wire, nil
}

// DeleteSchedule removes a schedule (schedules.delete). Idempotent.
func (s *Server) DeleteSchedule(ctx context.Context, in protocol.DeleteScheduleRequest) error {
	return mapScheduleErr(s.schedules.Delete(ctx, in.ID), "schedules.delete", in.ID)
}

// RunScheduleNow fires a schedule immediately (schedules.runNow) — a manual
// extra run that records the firing without shifting the schedule's next due
// time.
func (s *Server) RunScheduleNow(ctx context.Context, in protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
	handle, err := s.schedules.RunNow(ctx, in.ID)
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.runNow", in.ID)
	}
	return &protocol.RunScheduleNowResponse{SessionID: handle.SessionID, RunID: handle.RunID}, nil
}

// mapScheduleErr surfaces an unknown-id as invalid_params (the supplied id
// doesn't resolve), passing every other error through unchanged.
func mapScheduleErr(err error, method, id string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, schedule.ErrUnavailable) {
		return capabilityNotNegotiated(method)
	}
	if errors.Is(err, schedule.ErrNotFound) {
		return fmt.Errorf("%w: schedule %q not found", protocol.ErrInvalidParams, id)
	}
	if errors.Is(err, schedule.ErrCwdUnavailable) {
		return fmt.Errorf("%w: %w", protocol.ErrCwdUnavailable, err)
	}
	if errors.Is(err, schedule.ErrRevisionConflict) {
		return fmt.Errorf("%w: schedule %q changed after it was read", protocol.ErrRevisionConflict, id)
	}
	if errors.Is(err, schedule.ErrIDRequired) ||
		errors.Is(err, schedule.ErrRevisionRequired) ||
		errors.Is(err, schedule.ErrPromptRequired) ||
		errors.Is(err, schedule.ErrCronRequired) ||
		errors.Is(err, schedule.ErrIncompleteModelSelection) ||
		errors.Is(err, schedule.ErrInvalidCron) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
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
		Revision:  sc.Revision,
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
