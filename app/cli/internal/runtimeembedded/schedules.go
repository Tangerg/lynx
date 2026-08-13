package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/schedule"
	"github.com/Tangerg/lynx/app/cli/internal/workspace"
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
			return nil, runtimeContractViolation("list schedules returned a nil page")
		}
		for index, value := range page.Data {
			projected := projectSchedule(value)
			if err := projected.Validate(); err != nil {
				return nil, runtimeContractViolation("list schedules item %d after cursor %q is invalid: %v", index+1, cursor, err)
			}
			if _, duplicate := seenIDs[projected.ID]; duplicate {
				return nil, runtimeContractViolation("list schedules repeats %q", projected.ID)
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
	validated := candidate
	if candidate.Workspace != "" {
		resolved, err := r.Resolve(ctx, workspace.ResolveRequest{Path: candidate.Workspace})
		if err != nil {
			return schedule.Schedule{}, fmt.Errorf("create schedule workspace: %w", err)
		}
		validated.Workspace = resolved.Path
	}
	request := protocol.CreateScheduleRequest{
		Title: validated.Title, Instructions: validated.Instructions,
		Provider: validated.Provider, Model: validated.Model, Cron: validated.Cron,
	}
	if validated.Workspace != "" {
		request.Workspace = &protocol.WorkspaceRef{Path: validated.Workspace}
	}
	created, err := r.schedules.CreateSchedule(ctx, request, options)
	projected, err := projectScheduleResult("create schedule", "", created, err)
	if err != nil {
		return schedule.Schedule{}, err
	}
	if err := validated.ValidateResult(projected); err != nil {
		return schedule.Schedule{}, runtimeContractViolation("create schedule returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
}

func (r *Runtime) Update(ctx context.Context, patch schedule.Patch) (schedule.Schedule, error) {
	if err := patch.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return schedule.Schedule{}, err
	}
	validated := patch
	if path, bound := patch.Workspace.Binding(); bound {
		resolved, err := r.Resolve(ctx, workspace.ResolveRequest{Path: path})
		if err != nil {
			return schedule.Schedule{}, fmt.Errorf("update schedule workspace: %w", err)
		}
		validated.Workspace = schedule.BindWorkspace(resolved.Path)
	}
	request := protocol.UpdateScheduleRequest{
		ID: validated.ID, ExpectedRevision: validated.ExpectedRevision,
		Title: clonePointer(validated.Title), Instructions: clonePointer(validated.Instructions),
		Provider: clonePointer(validated.Provider), Model: clonePointer(validated.Model),
		Cron: clonePointer(validated.Cron), Enabled: clonePointer(validated.Enabled),
	}
	if path, bound := validated.Workspace.Binding(); bound {
		request.Workspace = &protocol.WorkspaceRef{Path: path}
	} else if validated.Workspace.UsesDefault() {
		request.WorkspaceMode = protocol.ScheduleWorkspaceDefault
	}
	updated, err := r.schedules.UpdateSchedule(ctx, request, options)
	projected, err := projectScheduleResult("update schedule", validated.ID, updated, err)
	if err != nil {
		return schedule.Schedule{}, err
	}
	if err := validated.ValidateResult(projected); err != nil {
		return schedule.Schedule{}, runtimeContractViolation("update schedule returned an invalid acknowledgement: %v", err)
	}
	return projected, nil
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
		return schedule.RunHandle{}, runtimeContractViolation("run schedule now returned nil")
	}
	handle := schedule.RunHandle{SessionID: result.SessionID, RunID: result.RunID}
	if err := handle.Validate(); err != nil {
		return schedule.RunHandle{}, runtimeContractViolation("run schedule now returned an invalid handle: %v", err)
	}
	return handle, nil
}

func projectScheduleResult(operation, expectedID string, result *protocol.Schedule, err error) (schedule.Schedule, error) {
	if err != nil {
		return schedule.Schedule{}, classifyError(err)
	}
	if result == nil {
		return schedule.Schedule{}, runtimeContractViolation("%s returned nil", operation)
	}
	projected := projectSchedule(*result)
	if err := projected.Validate(); err != nil {
		return schedule.Schedule{}, runtimeContractViolation("%s returned an invalid schedule: %v", operation, err)
	}
	if expectedID != "" && projected.ID != expectedID {
		return schedule.Schedule{}, runtimeContractViolation("%s returned id %q for %q", operation, projected.ID, expectedID)
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
