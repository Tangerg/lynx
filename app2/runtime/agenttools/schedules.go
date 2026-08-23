package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type listSchedulesRequest struct {
	Cursor string `json:"cursor,omitempty" jsonschema_description:"Opaque next_cursor returned by a previous list_schedules call."`
}

type createScheduleRequest struct {
	Title         string `json:"title,omitempty" jsonschema_description:"Optional concise name for the recurring automation."`
	Instructions  string `json:"instructions" jsonschema:"minLength=1" jsonschema_description:"Complete self-contained instructions for every scheduled Run."`
	WorkspacePath string `json:"workspace_path,omitempty" jsonschema_description:"Absolute workspace path. Omit to use the Runtime default at firing time."`
	Provider      string `json:"provider,omitempty" jsonschema_description:"Provider id; set together with model only when the user chose both."`
	Model         string `json:"model,omitempty" jsonschema_description:"Model id; set together with provider only when the user chose both."`
	Cron          string `json:"cron" jsonschema:"minLength=1" jsonschema_description:"Standard five-field cron expression: minute hour day-of-month month day-of-week."`
}

type deleteScheduleRequest struct {
	ScheduleID string `json:"schedule_id" jsonschema:"minLength=1" jsonschema_description:"Exact id returned by list_schedules or create_schedule."`
}

type scheduleToolPage struct {
	Schedules  []scheduleToolView `json:"schedules"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

type scheduleToolResult struct {
	Schedule scheduleToolView `json:"schedule"`
}

type scheduleDeleteResult struct {
	ScheduleID string `json:"schedule_id"`
}

type scheduleToolView struct {
	ScheduleID    string `json:"schedule_id"`
	Title         string `json:"title"`
	Instructions  string `json:"instructions"`
	WorkspacePath string `json:"workspace_path,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	Cron          string `json:"cron"`
	Enabled       bool   `json:"enabled"`
	LastRunAt     string `json:"last_run_at,omitempty"`
	NextRunAt     string `json:"next_run_at,omitempty"`
	CreatedAt     string `json:"created_at"`
	Revision      uint64 `json:"revision"`
}

func (catalog *Catalog) scheduleTools() ([]scopedTool, error) {
	list, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name:        "list_schedules",
		Description: "List recurring Lyra Run schedules, their exact ids, instructions, cadence, workspace and next-run state. Pass next_cursor to continue when the result is paged.",
	}, func(ctx context.Context, request listSchedulesRequest) (string, error) {
		page, err := catalog.schedules.List(ctx, protocol.PageQuery{Limit: 100, Cursor: request.Cursor})
		if err != nil {
			return "", err
		}
		result := scheduleToolPage{Schedules: make([]scheduleToolView, len(page.Data)), NextCursor: page.NextCursor}
		for index, value := range page.Data {
			result.Schedules[index] = presentScheduleTool(value)
		}
		return encodeScheduleToolResult(result)
	})
	if err != nil {
		return nil, fmt.Errorf("agenttools: list Schedules tool: %w", err)
	}

	create, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name:        "create_schedule",
		Description: "Create an enabled recurring Lyra Run schedule. Use only when the user explicitly requests recurring automated work; do not use it for the current request, a one-off action, or an autonomous Goal.",
	}, func(ctx context.Context, request createScheduleRequest) (string, error) {
		var workspace *protocol.WorkspaceRef
		if strings.TrimSpace(request.WorkspacePath) != "" {
			workspace = &protocol.WorkspaceRef{Path: request.WorkspacePath}
		}
		value, err := catalog.schedules.Create(ctx, protocol.CreateScheduleRequest{
			Title: request.Title, Instructions: request.Instructions, Workspace: workspace,
			Provider: request.Provider, Model: request.Model, Cron: request.Cron,
		})
		if err != nil {
			return "", err
		}
		return encodeScheduleToolResult(scheduleToolResult{Schedule: presentScheduleTool(*value)})
	})
	if err != nil {
		return nil, fmt.Errorf("agenttools: create Schedule tool: %w", err)
	}

	deleteTool, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name:        "delete_schedule",
		Description: "Delete one recurring Lyra Run schedule by its exact schedule_id. Use list_schedules first when the identity is uncertain. Deletion does not cancel a Run already admitted from an occurrence.",
	}, func(ctx context.Context, request deleteScheduleRequest) (string, error) {
		if err := catalog.schedules.Delete(ctx, protocol.DeleteScheduleRequest{ID: request.ScheduleID}); err != nil {
			return "", err
		}
		return encodeScheduleToolResult(scheduleDeleteResult(request))
	})
	if err != nil {
		return nil, fmt.Errorf("agenttools: delete Schedule tool: %w", err)
	}
	return []scopedTool{
		{tool: list, safety: protocol.SafetyClassSafe, deferred: true},
		{tool: create, safety: protocol.SafetyClassWrite, deferred: true},
		{tool: deleteTool, safety: protocol.SafetyClassWrite, deferred: true},
	}, nil
}

func presentScheduleTool(value protocol.Schedule) scheduleToolView {
	workspace := ""
	if value.Workspace != nil {
		workspace = value.Workspace.Path
	}
	return scheduleToolView{
		ScheduleID: value.ID, Title: value.Title, Instructions: value.Instructions,
		WorkspacePath: workspace, Provider: value.Provider, Model: value.Model,
		Cron: value.Cron, Enabled: value.Enabled, LastRunAt: scheduleToolTime(value.LastRunAt),
		NextRunAt: scheduleToolTime(value.NextRunAt), CreatedAt: value.CreatedAt.UTC().Format(time.RFC3339),
		Revision: value.Revision,
	}
}

func scheduleToolTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func encodeScheduleToolResult(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("agenttools: encode Schedule result: %w", err)
	}
	return string(encoded), nil
}
