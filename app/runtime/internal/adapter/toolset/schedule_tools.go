package toolset

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/tools"
)

// scheduleRequest is the single `schedule` tool's argument shape — one
// op-multiplexed tool (list / create / update / delete) rather than four, so
// the model's tool surface stays small (mirrors the lsp / skill op-tools).
// Mutable fields are pointers so update patches only what's set; create requires
// prompt + cron.
type scheduleOperation string

const (
	scheduleListOperation   scheduleOperation = "list"
	scheduleCreateOperation scheduleOperation = "create"
	scheduleUpdateOperation scheduleOperation = "update"
	scheduleDeleteOperation scheduleOperation = "delete"
)

type scheduleRequest struct {
	Op       scheduleOperation `json:"op" jsonschema:"required,enum=list,enum=create,enum=update,enum=delete" jsonschema_description:"list = return all schedules; create = add one (needs prompt + cron); update = patch by id (omitted fields unchanged); delete = remove by id."`
	ID       string            `json:"id,omitempty" jsonschema_description:"Schedule id — required for update and delete."`
	Title    *string           `json:"title,omitempty" jsonschema_description:"Display title (create / update)."`
	Prompt   *string           `json:"prompt,omitempty" jsonschema_description:"Prompt to run when the schedule fires — required for create."`
	Cwd      *string           `json:"cwd,omitempty" jsonschema_description:"Working directory for the run. Empty uses the runtime default."`
	Provider *string           `json:"provider,omitempty" jsonschema_description:"Provider id. Must be paired with model."`
	Model    *string           `json:"model,omitempty" jsonschema_description:"Model id. Must be paired with provider."`
	Cron     *string           `json:"cron,omitempty" jsonschema_description:"Five-field cron expression (minute hour day-of-month month day-of-week) — required for create."`
	Enabled  *bool             `json:"enabled,omitempty" jsonschema_description:"Whether the schedule fires. Default true on create."`
}

type scheduleListResponse struct {
	Schedules []scheduleView `json:"schedules"`
}

type scheduleResponse struct {
	Schedule scheduleView `json:"schedule"`
}

type scheduleDeleteResponse struct {
	Deleted bool `json:"deleted"`
}

type scheduleView struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Prompt    string `json:"prompt"`
	Cwd       string `json:"cwd,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Cron      string `json:"cron"`
	Enabled   bool   `json:"enabled"`
	LastRunAt string `json:"last_run_at,omitempty"`
	NextRunAt string `json:"next_run_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// newScheduleTool builds the single `schedule` management tool. nil coordinator → nil
// tool (feature off, omitted). Coding role only.
func newScheduleTool(coordinator *scheduleapp.Coordinator) (tools.Tool, error) {
	if coordinator == nil {
		return nil, nil
	}
	return tools.New[scheduleRequest, string](
		tools.Config{
			Name:        "schedule",
			Description: "Manage cron schedules for background agent runs. op=list returns all; create needs prompt + cron; update patches by id (omitted fields unchanged); delete removes by id.",
		},
		func(ctx context.Context, in scheduleRequest) (string, error) {
			switch in.Op {
			case scheduleListOperation:
				return scheduleList(ctx, coordinator)
			case scheduleCreateOperation:
				return scheduleCreate(ctx, coordinator, in)
			case scheduleUpdateOperation:
				return scheduleUpdate(ctx, coordinator, in)
			case scheduleDeleteOperation:
				return scheduleDelete(ctx, coordinator, in)
			default:
				return "", fmt.Errorf("schedule: unknown op %q (want list | create | update | delete)", in.Op)
			}
		},
	)
}

func scheduleList(ctx context.Context, coordinator *scheduleapp.Coordinator) (string, error) {
	items, err := coordinator.List(ctx)
	if err != nil {
		return "", fmt.Errorf("schedule list: %w", err)
	}
	views := make([]scheduleView, len(items))
	for i, sc := range items {
		views[i] = viewSchedule(sc)
	}
	return encodeToolResult(scheduleListResponse{Schedules: views})
}

func scheduleCreate(ctx context.Context, coordinator *scheduleapp.Coordinator, in scheduleRequest) (string, error) {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	created, err := coordinator.Create(ctx, scheduleapp.CreateCommand{
		Title:    derefString(in.Title),
		Prompt:   derefString(in.Prompt),
		Cwd:      derefString(in.Cwd),
		Provider: derefString(in.Provider),
		Model:    derefString(in.Model),
		Cron:     derefString(in.Cron),
		Enabled:  enabled,
	})
	if err != nil {
		return "", fmt.Errorf("schedule create: %w", err)
	}
	return encodeToolResult(scheduleResponse{Schedule: viewSchedule(created)})
}

func scheduleUpdate(ctx context.Context, coordinator *scheduleapp.Coordinator, in scheduleRequest) (string, error) {
	updated, err := coordinator.UpdateLatest(ctx, in.ID, schedule.Patch{
		Title:    in.Title,
		Prompt:   in.Prompt,
		Cwd:      in.Cwd,
		Provider: in.Provider,
		Model:    in.Model,
		Cron:     in.Cron,
		Enabled:  in.Enabled,
	})
	if err != nil {
		return "", fmt.Errorf("schedule update: %w", err)
	}
	return encodeToolResult(scheduleResponse{Schedule: viewSchedule(updated)})
}

func scheduleDelete(ctx context.Context, coordinator *scheduleapp.Coordinator, in scheduleRequest) (string, error) {
	if err := coordinator.Delete(ctx, in.ID); err != nil {
		return "", fmt.Errorf("schedule delete: %w", err)
	}
	return encodeToolResult(scheduleDeleteResponse{Deleted: true})
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func viewSchedule(sc schedule.Schedule) scheduleView {
	return scheduleView{
		ID:        sc.ID,
		Title:     sc.Title,
		Prompt:    sc.Prompt,
		Cwd:       sc.Cwd,
		Provider:  sc.Provider,
		Model:     sc.Model,
		Cron:      sc.Cron,
		Enabled:   sc.Enabled,
		LastRunAt: formatToolTime(sc.LastRunAt),
		NextRunAt: formatToolTime(sc.NextRunAt),
		CreatedAt: formatToolTime(sc.CreatedAt),
	}
}

func formatToolTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func encodeToolResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
