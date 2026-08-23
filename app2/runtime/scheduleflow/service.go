// Package scheduleflow owns recurring Run management, durable occurrence
// dispatch, Runtime invalidation, and the bounded timer worker.
package scheduleflow

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
	"github.com/Tangerg/lynx/app2/runtime/domain/schedule"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/workspacefs"
)

type Store interface {
	CreateSchedule(context.Context, schedule.Schedule) error
	GetSchedule(context.Context, string) (schedule.Schedule, error)
	ListSchedulePage(context.Context, int, *schedule.Cursor) (schedule.Page, error)
	UpdateSchedule(context.Context, schedule.Schedule, uint64) error
	DeleteSchedule(context.Context, string) (bool, error)
}

type IDs interface{ New(string) (string, error) }

type Workspaces interface {
	Resolve(context.Context, string) (workspacefs.Resolution, error)
}

type Events interface {
	Publish(protocol.RuntimeEvent)
}

type Config struct {
	Store      Store
	IDs        IDs
	Workspaces Workspaces
	Events     Events
	Clock      func() time.Time
}

type Service struct {
	store      Store
	ids        IDs
	workspaces Workspaces
	events     Events
	now        func() time.Time
}

func New(config Config) (*Service, error) {
	if config.Store == nil || config.IDs == nil || config.Workspaces == nil || config.Events == nil {
		return nil, errors.New("scheduleflow: store, ids, workspaces, and events are required")
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		store: config.Store, ids: config.IDs, workspaces: config.Workspaces,
		events: config.Events, now: clock,
	}, nil
}

func (service *Service) List(
	ctx context.Context,
	query protocol.PageQuery,
) (*protocol.Page[protocol.Schedule], error) {
	cursor, err := decodeCursor(query.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: schedule cursor is invalid", protocol.ErrInvalidParams)
	}
	page, err := service.store.ListSchedulePage(ctx, query.Limit, cursor)
	if err != nil {
		return nil, err
	}
	values := make([]protocol.Schedule, len(page.Schedules))
	for index, value := range page.Schedules {
		values[index] = present(value)
	}
	next := ""
	if page.Next != nil {
		next = encodeCursor(*page.Next)
	}
	return protocol.NewPageWithCursor(values, next), nil
}

func (service *Service) Create(
	ctx context.Context,
	request protocol.CreateScheduleRequest,
) (*protocol.Schedule, error) {
	selection, err := modelselection.New(request.Provider, request.Model)
	if err != nil {
		return nil, invalid(err)
	}
	workspace, err := service.resolveExplicitWorkspace(ctx, request.Workspace)
	if err != nil {
		return nil, err
	}
	id, err := service.ids.New("sch_")
	if err != nil {
		return nil, err
	}
	value, err := schedule.New(schedule.Create{
		ID: id, Title: request.Title, Instructions: request.Instructions,
		Workspace: workspace, Selection: selection, Cron: request.Cron, Now: service.now(),
	})
	if err != nil {
		return nil, invalid(err)
	}
	if err := service.store.CreateSchedule(ctx, value); err != nil {
		return nil, err
	}
	service.publish(value.ID())
	presented := present(value)
	return &presented, nil
}

func (service *Service) Update(
	ctx context.Context,
	request protocol.UpdateScheduleRequest,
) (*protocol.Schedule, error) {
	value, err := service.store.GetSchedule(ctx, request.ID)
	if err != nil {
		return nil, projectError(err)
	}
	patch := schedule.Patch{
		Title: request.Title, Instructions: request.Instructions,
		Cron: request.Cron, Enabled: request.Enabled,
	}
	if request.Workspace != nil && request.WorkspaceMode != "" {
		return nil, fmt.Errorf("%w: workspace and workspaceMode are mutually exclusive", protocol.ErrInvalidParams)
	}
	if request.WorkspaceMode != "" && request.WorkspaceMode != protocol.ScheduleWorkspaceDefault {
		return nil, fmt.Errorf("%w: unknown workspaceMode %q", protocol.ErrInvalidParams, request.WorkspaceMode)
	}
	if request.Workspace != nil {
		workspace, err := service.resolveExplicitWorkspace(ctx, request.Workspace)
		if err != nil {
			return nil, err
		}
		patch.Workspace = &workspace
	} else if request.WorkspaceMode == protocol.ScheduleWorkspaceDefault {
		workspace := ""
		patch.Workspace = &workspace
	}
	selection, err := selectionPatch(request.Provider, request.Model)
	if err != nil {
		return nil, err
	}
	patch.Selection = selection
	updated, changed, err := value.Update(request.ExpectedRevision, patch, service.now())
	if err != nil {
		return nil, projectError(err)
	}
	if !changed {
		presented := present(value)
		return &presented, nil
	}
	if err := service.store.UpdateSchedule(ctx, updated, value.Revision()); err != nil {
		return nil, projectError(err)
	}
	service.publish(updated.ID())
	presented := present(updated)
	return &presented, nil
}

func (service *Service) Delete(ctx context.Context, request protocol.DeleteScheduleRequest) error {
	changed, err := service.store.DeleteSchedule(ctx, request.ID)
	if err != nil {
		return err
	}
	if changed {
		service.publish(request.ID)
	}
	return nil
}

func (service *Service) resolveExplicitWorkspace(
	ctx context.Context,
	workspace *protocol.WorkspaceRef,
) (string, error) {
	if workspace == nil {
		return "", nil
	}
	resolved, err := service.workspaces.Resolve(ctx, workspace.Path)
	if err != nil || !resolved.Available {
		return "", fmt.Errorf("%w: workspace %q is unavailable", protocol.ErrWorkspaceUnavailable, workspace.Path)
	}
	return resolved.Workspace.Path(), nil
}

func selectionPatch(provider, model *string) (*modelselection.Selection, error) {
	if provider == nil && model == nil {
		return nil, nil
	}
	if provider == nil || model == nil {
		return nil, fmt.Errorf("%w: provider and model must be changed together", protocol.ErrInvalidParams)
	}
	selection, err := modelselection.New(*provider, *model)
	if err != nil {
		return nil, invalid(err)
	}
	return &selection, nil
}

func present(value schedule.Schedule) protocol.Schedule {
	result := protocol.Schedule{
		ID: value.ID(), Title: value.Title(), Instructions: value.Instructions(),
		Provider: value.Selection().Provider(), Model: value.Selection().Model(),
		Cron: value.Cron(), Enabled: value.Enabled(), CreatedAt: value.CreatedAt(), Revision: value.Revision(),
	}
	if value.Workspace() != "" {
		result.Workspace = &protocol.WorkspaceRef{Path: value.Workspace()}
	}
	if !value.LastRunAt().IsZero() {
		last := value.LastRunAt()
		result.LastRunAt = &last
	}
	if !value.NextRunAt().IsZero() {
		next := value.NextRunAt()
		result.NextRunAt = &next
	}
	return result
}

func invalid(err error) error {
	return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
}

func projectError(err error) error {
	switch {
	case errors.Is(err, schedule.ErrNotFound):
		return protocol.ErrItemNotFound
	case errors.Is(err, schedule.ErrRevisionConflict):
		return protocol.ErrRevisionConflict
	case errors.Is(err, schedule.ErrInvalid):
		return invalid(err)
	default:
		return err
	}
}

func (service *Service) publish(id string) {
	service.events.Publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{id},
	})
}

func encodeCursor(value schedule.Cursor) string {
	raw := value.CreatedAt.UTC().Format(time.RFC3339Nano) + "\n" + value.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(value string) (*schedule.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "\n", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "sch_") ||
		len(parts[1]) > 512 || strings.TrimSpace(parts[1]) != parts[1] ||
		strings.IndexFunc(parts[1], func(value rune) bool { return value < 0x20 || value == 0x7f }) >= 0 {
		return nil, errors.New("invalid schedule cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	return &schedule.Cursor{CreatedAt: createdAt, ID: parts[1]}, nil
}
