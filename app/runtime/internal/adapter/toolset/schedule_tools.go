package toolset

import (
	"context"
	"fmt"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
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

// ScheduleManagement is the coding tool's narrow schedule-management use case.
// It intentionally does not expose delivery's revisioned Update or firing APIs.
type ScheduleManagement interface {
	List(ctx context.Context) ([]schedule.Schedule, error)
	Create(ctx context.Context, cmd scheduleapp.CreateCommand) (schedule.Schedule, error)
	Delete(ctx context.Context, id string) error
}

type scheduleTools struct{ coordinator ScheduleManagement }

// newScheduleTools builds one tool per schedule action. Each schema therefore
// contains only fields that action can consume. nil coordinator disables the
// complete model-facing family.
func newScheduleTools(coordinator ScheduleManagement) ([]toolcontract.Tool, error) {
	if coordinator == nil {
		return nil, nil
	}
	t := &scheduleTools{coordinator: coordinator}
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
	deleteTool, err := toolcontract.NewFunc[deleteScheduleArgs, scheduleDeleteResponse](
		toolcontract.FuncConfig{
			Name:        "delete_schedule",
			Description: "Permanently delete one recurring Agent Run schedule by its exact schedule_id. Use list_schedules first when the id is uncertain. To change a schedule, delete it and create the replacement explicitly.",
		},
		t.delete,
	)
	if err != nil {
		return nil, err
	}
	return []toolcontract.Tool{list, create, deleteTool}, nil
}

func (t *scheduleTools) list(ctx context.Context, _ struct{}) (scheduleListResponse, error) {
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

func (t *scheduleTools) create(ctx context.Context, in createScheduleArgs) (scheduleResponse, error) {
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

func (t *scheduleTools) delete(ctx context.Context, in deleteScheduleArgs) (scheduleDeleteResponse, error) {
	if err := t.coordinator.Delete(ctx, in.ScheduleID); err != nil {
		return scheduleDeleteResponse{}, fmt.Errorf("delete_schedule: %w", err)
	}
	return scheduleDeleteResponse{ScheduleID: in.ScheduleID}, nil
}

func viewSchedule(sc schedule.Schedule) scheduleView {
	return scheduleView{
		ScheduleID:   sc.ID,
		Title:        sc.Title,
		Instructions: sc.Prompt,
		Workdir:      sc.Cwd,
		Provider:     sc.ModelSelection.Provider(),
		Model:        sc.ModelSelection.Model(),
		Cron:         sc.Cron,
		Enabled:      sc.Enabled,
		LastRunAt:    formatToolTime(sc.LastRunAt),
		NextRunAt:    formatToolTime(sc.NextRunAt),
		CreatedAt:    formatToolTime(sc.CreatedAt),
	}
}

func formatToolTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
