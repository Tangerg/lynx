// Package schedule exposes recurring Agent Run schedule management tools.
package schedule

import (
	"context"
	"fmt"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	scheduledomain "github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type createScheduleArgs struct {
	Title        string `json:"title,omitempty" jsonschema_description:"Optional concise name for this recurring automation."`
	Instructions string `json:"instructions" jsonschema:"minLength=1" jsonschema_description:"Complete self-contained instructions for each scheduled Agent Run."`
	Workdir      string `json:"workdir,omitempty" jsonschema_description:"Workspace directory for each Run. Omit to use the configured default."`
	Provider     string `json:"provider,omitempty" jsonschema_description:"Model provider id. Set together with model only when the user explicitly chose both; otherwise omit both."`
	Model        string `json:"model,omitempty" jsonschema_description:"Model id. Set together with provider only when the user explicitly chose both; otherwise omit both."`
	Cron         string `json:"cron" jsonschema:"minLength=1" jsonschema_description:"Five-field cron expression: minute hour day-of-month month day-of-week."`
}

type deleteScheduleArgs struct {
	ScheduleID string `json:"schedule_id" jsonschema:"minLength=1" jsonschema_description:"Exact id returned by list_schedules or create_schedule."`
}

type scheduleListResponse struct {
	Schedules []scheduleView `json:"schedules"`
}

type scheduleResponse struct {
	Schedule scheduleView `json:"schedule"`
}

type scheduleDeleteResponse struct {
	ScheduleID string `json:"schedule_id"`
}

type scheduleView struct {
	ScheduleID   string `json:"schedule_id"`
	Title        string `json:"title,omitempty"`
	Instructions string `json:"instructions"`
	Workdir      string `json:"workdir,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	Cron         string `json:"cron"`
	Enabled      bool   `json:"enabled"`
	LastRunAt    string `json:"last_run_at,omitempty"`
	NextRunAt    string `json:"next_run_at,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
}

// Management is the schedule family's narrow application use case.
// It intentionally excludes revisioned updates and firing operations.
type Management interface {
	List(ctx context.Context) ([]scheduledomain.Schedule, error)
	Create(ctx context.Context, cmd scheduleapp.CreateCommand) (scheduledomain.Schedule, error)
	Delete(ctx context.Context, id string) error
}

type family struct{ coordinator Management }

// Build constructs one tool per schedule action. Each schema therefore contains
// only fields that action can consume. A nil coordinator disables the family.
func Build(coordinator Management) ([]toolcontract.Tool, error) {
	if coordinator == nil {
		return nil, nil
	}
	t := &family{coordinator: coordinator}
	list, err := toolcontract.NewFunc[struct{}, scheduleListResponse](
		toolcontract.FuncConfig{
			Name:        "list_schedules",
			Description: "List recurring Agent Run schedules and their ids, instructions, cron expressions, model choices, and next-run state. Use this before deleting or replacing a schedule when its exact id is unknown.",
		},
		t.list,
	)
	if err != nil {
		return nil, err
	}
	create, err := toolcontract.NewFunc[createScheduleArgs, scheduleResponse](
		toolcontract.FuncConfig{
			Name: "create_schedule",
			Description: "Create an enabled recurring schedule that starts a new Agent Run from self-contained instructions at each five-field cron occurrence. " +
				"Use only when the user explicitly asks for recurring automated work; do not use for the current request, a one-off future action, or an autonomous Goal.",
		},
		t.create,
	)
	if err != nil {
		return nil, err
	}
	deleteSchedule, err := toolcontract.NewFunc[deleteScheduleArgs, scheduleDeleteResponse](
		toolcontract.FuncConfig{
			Name:        "delete_schedule",
			Description: "Permanently delete one recurring Agent Run schedule by its exact schedule_id. Use list_schedules first when the id is uncertain. To change a schedule, delete it and create the replacement explicitly.",
		},
		t.delete,
	)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{list, create, deleteSchedule}, nil
}

func (t *family) list(ctx context.Context, _ struct{}) (scheduleListResponse, error) {
	items, err := t.coordinator.List(ctx)
	if err != nil {
		return scheduleListResponse{}, fmt.Errorf("list_schedules: %w", err)
	}
	views := make([]scheduleView, len(items))
	for i, sc := range items {
		views[i] = viewSchedule(sc)
	}
	return scheduleListResponse{Schedules: views}, nil
}

func (t *family) create(ctx context.Context, in createScheduleArgs) (scheduleResponse, error) {
	selection, err := modelref.New(in.Provider, in.Model)
	if err != nil {
		return scheduleResponse{}, fmt.Errorf("create_schedule: %w", err)
	}
	created, err := t.coordinator.Create(ctx, scheduleapp.CreateCommand{
		Title:          in.Title,
		Prompt:         in.Instructions,
		Cwd:            in.Workdir,
		ModelSelection: selection,
		Cron:           in.Cron,
		Enabled:        true,
	})
	if err != nil {
		return scheduleResponse{}, fmt.Errorf("create_schedule: %w", err)
	}
	return scheduleResponse{Schedule: viewSchedule(created)}, nil
}

func (t *family) delete(ctx context.Context, in deleteScheduleArgs) (scheduleDeleteResponse, error) {
	if err := t.coordinator.Delete(ctx, in.ScheduleID); err != nil {
		return scheduleDeleteResponse{}, fmt.Errorf("delete_schedule: %w", err)
	}
	return scheduleDeleteResponse(in), nil
}

func viewSchedule(sc scheduledomain.Schedule) scheduleView {
	return scheduleView{
		ScheduleID:   sc.ID,
		Title:        sc.Title,
		Instructions: sc.Prompt,
		Workdir:      sc.Cwd,
		Provider:     sc.ModelSelection.Provider(),
		Model:        sc.ModelSelection.Model(),
		Cron:         sc.Cron,
		Enabled:      sc.Enabled,
		LastRunAt:    formatScheduleTime(sc.LastRunAt),
		NextRunAt:    formatScheduleTime(sc.NextRunAt),
		CreatedAt:    formatScheduleTime(sc.CreatedAt),
	}
}

func formatScheduleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
