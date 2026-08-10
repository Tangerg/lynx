package server

import (
	"context"
	"errors"
	"fmt"

	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// schedules.* (API.md §7.9) — manage the cron-triggered headless runs the
// application scheduler fires. A schedule stores the final instructions
// text, so the runtime fires it without resolving a recipe.

// ListSchedules returns every schedule, newest-created first (schedules.list).
func (s *Server) ListSchedules(ctx context.Context, query protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
	page, err := s.schedules.ListPage(ctx, query.Cursor, query.Limit)
	if err != nil {
		return nil, mapScheduleErr(wirePageError(err), "schedules.list", "")
	}
	out := make([]protocol.Schedule, 0, len(page.Rows))
	for _, scheduled := range page.Rows {
		out = append(out, presentSchedule(scheduled))
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

// CreateSchedule adds an enabled schedule (schedules.create), computing its
// first due time from the cron.
func (s *Server) CreateSchedule(ctx context.Context, in protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
	selection, err := modelref.New(in.Provider, in.Model)
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.create", "")
	}
	created, err := s.schedules.Create(ctx, scheduleapp.CreateCommand{
		Title:          in.Title,
		Instructions:   in.Instructions,
		CWD:            workspaceRefPath(in.Workspace),
		ModelSelection: selection,
		Cron:           in.Cron,
		Enabled:        true,
	})
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.create", "")
	}
	wire := presentSchedule(created)
	return &wire, nil
}

// UpdateSchedule applies a revision-guarded partial patch. The schedule use case
// recomputes due time when cron or enabled changes and clears it when disabled.
func (s *Server) UpdateSchedule(ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
	updated, err := s.schedules.Update(ctx, scheduleapp.UpdateCommand{
		ID:               in.ID,
		ExpectedRevision: in.ExpectedRevision,
		Patch: schedule.Patch{
			Title:        in.Title,
			Instructions: in.Instructions,
			CWD:          workspacePathPatch(in.Workspace),
			Provider:     in.Provider,
			Model:        in.Model,
			Cron:         in.Cron,
			Enabled:      in.Enabled,
		},
	})
	if err != nil {
		return nil, mapScheduleErr(err, "schedules.update", in.ID)
	}
	wire := presentSchedule(updated)
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
	handle, err := s.scheduleFiring.RunNow(ctx, in.ID)
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
	if errors.Is(err, scheduleapp.ErrUnavailable) {
		return capabilityNotNegotiated(method)
	}
	if errors.Is(err, schedule.ErrNotFound) {
		return fmt.Errorf("%w: schedule %q not found", protocol.ErrInvalidParams, id)
	}
	if errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		return fmt.Errorf("%w: %w", protocol.ErrWorkspaceUnavailable, err)
	}
	if errors.Is(err, schedule.ErrRevisionConflict) {
		return fmt.Errorf("%w: schedule %q changed after it was read", protocol.ErrRevisionConflict, id)
	}
	if errors.Is(err, schedule.ErrIDRequired) ||
		errors.Is(err, schedule.ErrRevisionRequired) ||
		errors.Is(err, schedule.ErrInstructionsRequired) ||
		errors.Is(err, schedule.ErrCronRequired) ||
		errors.Is(err, modelref.ErrIncomplete) ||
		errors.Is(err, schedule.ErrInvalidCron) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}

// presentSchedule maps a domain schedule to its protocol shape, projecting the zero
// time (never fired / unscheduled) to an omitted field rather than a fake epoch.
func presentSchedule(scheduled schedule.Schedule) protocol.Schedule {
	presented := protocol.Schedule{
		ID:           scheduled.ID,
		Title:        scheduled.Title,
		Instructions: scheduled.Instructions,
		Workspace:    workspaceRefFromPath(scheduled.CWD),
		Provider:     scheduled.ModelSelection.Provider(),
		Model:        scheduled.ModelSelection.Model(),
		Cron:         scheduled.Cron,
		Enabled:      scheduled.Enabled,
		CreatedAt:    scheduled.CreatedAt,
		Revision:     scheduled.Revision,
	}
	if !scheduled.LastRunAt.IsZero() {
		lastRunAt := scheduled.LastRunAt
		presented.LastRunAt = &lastRunAt
	}
	if !scheduled.NextRunAt.IsZero() {
		nextRunAt := scheduled.NextRunAt
		presented.NextRunAt = &nextRunAt
	}
	return presented
}
