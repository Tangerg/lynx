package toolset

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type scheduleListRequest struct{}

type scheduleCreateRequest struct {
	Title    string `json:"title,omitempty" jsonschema_description:"Short display title."`
	Prompt   string `json:"prompt" jsonschema:"required" jsonschema_description:"Prompt to run when the schedule fires."`
	Cwd      string `json:"cwd,omitempty" jsonschema_description:"Working directory for the scheduled run. Empty uses the runtime default."`
	Provider string `json:"provider,omitempty" jsonschema_description:"Optional provider id. Must be set together with model."`
	Model    string `json:"model,omitempty" jsonschema_description:"Optional model id. Must be set together with provider."`
	Cron     string `json:"cron" jsonschema:"required" jsonschema_description:"Five-field cron expression: minute hour day-of-month month day-of-week."`
	Enabled  *bool  `json:"enabled,omitempty" jsonschema_description:"Whether the schedule should fire. Default true."`
}

type scheduleUpdateRequest struct {
	ID       string  `json:"id" jsonschema:"required" jsonschema_description:"Schedule id."`
	Title    *string `json:"title,omitempty" jsonschema_description:"Replace the display title."`
	Prompt   *string `json:"prompt,omitempty" jsonschema_description:"Replace the prompt."`
	Cwd      *string `json:"cwd,omitempty" jsonschema_description:"Replace the working directory. Empty clears it."`
	Provider *string `json:"provider,omitempty" jsonschema_description:"Replace provider id. Must be paired with model."`
	Model    *string `json:"model,omitempty" jsonschema_description:"Replace model id. Must be paired with provider."`
	Cron     *string `json:"cron,omitempty" jsonschema_description:"Replace cron expression."`
	Enabled  *bool   `json:"enabled,omitempty" jsonschema_description:"Enable or disable the schedule."`
}

type scheduleDeleteRequest struct {
	ID string `json:"id" jsonschema:"required" jsonschema_description:"Schedule id."`
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

func newScheduleTools(reg schedule.Registry) ([]chat.Tool, error) {
	if reg == nil {
		return nil, nil
	}
	list, err := chat.NewJSONTool[scheduleListRequest](
		chat.ToolDefinition{
			Name:        "schedule_list",
			Description: "List saved cron schedules for background agent runs.",
		},
		func(ctx context.Context, _ scheduleListRequest) (string, error) {
			items, err := reg.List(ctx)
			if err != nil {
				return "", fmt.Errorf("schedule_list: %w", err)
			}
			views := make([]scheduleView, len(items))
			for i, sc := range items {
				views[i] = viewSchedule(sc)
			}
			return encodeToolResult(scheduleListResponse{Schedules: views})
		},
	)
	if err != nil {
		return nil, err
	}

	create, err := chat.NewJSONTool[scheduleCreateRequest](
		chat.ToolDefinition{
			Name:        "schedule_create",
			Description: "Create a cron schedule that starts a background agent run with a saved prompt.",
		},
		func(ctx context.Context, in scheduleCreateRequest) (string, error) {
			enabled := true
			if in.Enabled != nil {
				enabled = *in.Enabled
			}
			sc := schedule.Schedule{
				Title:    in.Title,
				Prompt:   in.Prompt,
				Cwd:      in.Cwd,
				Provider: in.Provider,
				Model:    in.Model,
				Cron:     in.Cron,
				Enabled:  enabled,
			}
			if enabled {
				next, err := schedule.NextRun(in.Cron, time.Now())
				if err != nil {
					return "", fmt.Errorf("schedule_create: %w", err)
				}
				sc.NextRunAt = next
			}
			if err := sc.Validate(); err != nil {
				return "", fmt.Errorf("schedule_create: %w", err)
			}
			created, err := reg.Create(ctx, sc)
			if err != nil {
				return "", fmt.Errorf("schedule_create: %w", err)
			}
			return encodeToolResult(scheduleResponse{Schedule: viewSchedule(created)})
		},
	)
	if err != nil {
		return nil, err
	}

	update, err := chat.NewJSONTool[scheduleUpdateRequest](
		chat.ToolDefinition{
			Name:        "schedule_update",
			Description: "Patch an existing cron schedule. Omitted fields keep their current values.",
		},
		func(ctx context.Context, in scheduleUpdateRequest) (string, error) {
			sc, err := reg.Get(ctx, in.ID)
			if err != nil {
				return "", fmt.Errorf("schedule_update: %w", err)
			}
			applySchedulePatch(&sc, in)
			if err := sc.Validate(); err != nil {
				return "", fmt.Errorf("schedule_update: %w", err)
			}
			if sc.Enabled {
				next, err := schedule.NextRun(sc.Cron, time.Now())
				if err != nil {
					return "", fmt.Errorf("schedule_update: %w", err)
				}
				sc.NextRunAt = next
			} else {
				sc.NextRunAt = time.Time{}
			}
			updated, err := reg.Update(ctx, sc)
			if err != nil {
				return "", fmt.Errorf("schedule_update: %w", err)
			}
			return encodeToolResult(scheduleResponse{Schedule: viewSchedule(updated)})
		},
	)
	if err != nil {
		return nil, err
	}

	deleteTool, err := chat.NewJSONTool[scheduleDeleteRequest](
		chat.ToolDefinition{
			Name:        "schedule_delete",
			Description: "Delete a saved cron schedule by id.",
		},
		func(ctx context.Context, in scheduleDeleteRequest) (string, error) {
			if err := reg.Delete(ctx, in.ID); err != nil {
				return "", fmt.Errorf("schedule_delete: %w", err)
			}
			return encodeToolResult(scheduleDeleteResponse{Deleted: true})
		},
	)
	if err != nil {
		return nil, err
	}

	return []chat.Tool{list, create, update, deleteTool}, nil
}

func applySchedulePatch(sc *schedule.Schedule, in scheduleUpdateRequest) {
	if in.Title != nil {
		sc.Title = *in.Title
	}
	if in.Prompt != nil {
		sc.Prompt = *in.Prompt
	}
	if in.Cwd != nil {
		sc.Cwd = *in.Cwd
	}
	if in.Provider != nil {
		sc.Provider = *in.Provider
	}
	if in.Model != nil {
		sc.Model = *in.Model
	}
	if in.Cron != nil {
		sc.Cron = *in.Cron
	}
	if in.Enabled != nil {
		sc.Enabled = *in.Enabled
	}
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
