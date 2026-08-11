package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/schedule"
)

const schedulePageLimit = 100

type scheduleBinding interface {
	ListSchedules(context.Context, protocol.PageQuery, embedded.CallOptions) (*protocol.Page[protocol.Schedule], error)
	CreateSchedule(context.Context, protocol.CreateScheduleRequest, embedded.CommandOptions) (*protocol.Schedule, error)
	UpdateSchedule(context.Context, protocol.UpdateScheduleRequest, embedded.CommandOptions) (*protocol.Schedule, error)
	DeleteSchedule(context.Context, protocol.DeleteScheduleRequest, embedded.CommandOptions) error
	RunScheduleNow(context.Context, protocol.RunScheduleNowRequest, embedded.CommandOptions) (*protocol.RunScheduleNowResponse, error)
}

var _ schedule.Service = (*Runtime)(nil)

func (r *Runtime) Schedules(ctx context.Context) ([]schedule.Schedule, error) {
	var schedules []schedule.Schedule
	seenIDs := make(map[string]struct{})
	cursors := newCursorTraversal("list schedules", "")
	for {
		cursor := cursors.Current()
		page, err := r.schedules.ListSchedules(ctx, protocol.PageQuery{Cursor: cursor, Limit: schedulePageLimit}, r.callOptions())
		if err != nil {
			return nil, classifyError(err)
		}
		if page == nil {
			return nil, errors.New("list schedules: runtime returned nil")
		}
		for index, value := range page.Data {
			projected := projectSchedule(value)
			if err := projected.Validate(); err != nil {
				return nil, fmt.Errorf("list schedules item %d after cursor %q: %w", index+1, cursor, err)
			}
			if _, duplicate := seenIDs[projected.ID]; duplicate {
				return nil, fmt.Errorf("list schedules repeats %q", projected.ID)
			}
			seenIDs[projected.ID] = struct{}{}
			schedules = append(schedules, projected)
		}
		more, err := cursors.Advance(page.NextCursor)
		if err != nil {
			return nil, err
		}
		if !more {
			return schedules, nil
		}
	}
}

func (r *Runtime) Create(ctx context.Context, candidate schedule.Candidate) (schedule.Schedule, error) {
	if err := candidate.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return schedule.Schedule{}, err
	}
	request := protocol.CreateScheduleRequest{
		Title: candidate.Title, Instructions: candidate.Instructions,
		Provider: candidate.Provider, Model: candidate.Model, Cron: candidate.Cron,
	}
	if candidate.Workspace != "" {
		request.Workspace = &protocol.WorkspaceRef{Path: candidate.Workspace}
	}
	created, err := r.schedules.CreateSchedule(ctx, request, options)
	return projectScheduleResult("create schedule", created, err)
}

func (r *Runtime) Update(ctx context.Context, patch schedule.Patch) (schedule.Schedule, error) {
	if err := patch.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return schedule.Schedule{}, err
	}
	request := protocol.UpdateScheduleRequest{
		ID: patch.ID, ExpectedRevision: patch.ExpectedRevision,
		Title: clonePointer(patch.Title), Instructions: clonePointer(patch.Instructions),
		Provider: clonePointer(patch.Provider), Model: clonePointer(patch.Model),
		Cron: clonePointer(patch.Cron), Enabled: clonePointer(patch.Enabled),
	}
	if patch.Workspace != nil {
		request.Workspace = &protocol.WorkspaceRef{Path: *patch.Workspace}
	}
	updated, err := r.schedules.UpdateSchedule(ctx, request, options)
	return projectScheduleResult("update schedule", updated, err)
}

func (r *Runtime) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("delete schedule: id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(r.schedules.DeleteSchedule(ctx, protocol.DeleteScheduleRequest{ID: id}, options))
}

func (r *Runtime) RunNow(ctx context.Context, id string) (schedule.RunHandle, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return schedule.RunHandle{}, errors.New("run schedule now: id is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return schedule.RunHandle{}, err
	}
	result, err := r.schedules.RunScheduleNow(ctx, protocol.RunScheduleNowRequest{ID: id}, options)
	if err != nil {
		return schedule.RunHandle{}, classifyError(err)
	}
	if result == nil {
		return schedule.RunHandle{}, errors.New("run schedule now: runtime returned nil")
	}
	handle := schedule.RunHandle{SessionID: result.SessionID, RunID: result.RunID}
	if err := handle.Validate(); err != nil {
		return schedule.RunHandle{}, fmt.Errorf("run schedule now: %w", err)
	}
	return handle, nil
}

func projectScheduleResult(operation string, result *protocol.Schedule, err error) (schedule.Schedule, error) {
	if err != nil {
		return schedule.Schedule{}, classifyError(err)
	}
	if result == nil {
		return schedule.Schedule{}, fmt.Errorf("%s: runtime returned nil", operation)
	}
	projected := projectSchedule(*result)
	if err := projected.Validate(); err != nil {
		return schedule.Schedule{}, fmt.Errorf("%s: %w", operation, err)
	}
	return projected, nil
}

func projectSchedule(value protocol.Schedule) schedule.Schedule {
	projected := schedule.Schedule{
		ID: value.ID, Title: value.Title, Instructions: value.Instructions,
		Provider: value.Provider, Model: value.Model, Cron: value.Cron, Enabled: value.Enabled,
		LastRunAt: clonePointer(value.LastRunAt), NextRunAt: clonePointer(value.NextRunAt),
		CreatedAt: value.CreatedAt, Revision: value.Revision,
	}
	if value.Workspace != nil {
		projected.Workspace = value.Workspace.Path
	}
	return projected
}
