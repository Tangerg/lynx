package protocol

import (
	"time"
)

// Schedule is one scheduled run (API.md §4.12). Instructions is the final text
// sent as the run's input. cron is a 5-field standard expression
// ("min hour dom month dow"). lastRunAt is omitted until first fired; nextRunAt
// is omitted when the schedule is disabled.
type Schedule struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	Instructions    string        `json:"instructions"`
	Workspace       *WorkspaceRef `json:"workspace,omitempty"`
	Provider        string        `json:"provider,omitempty"`
	Model           string        `json:"model,omitempty"`
	ReasoningEffort string        `json:"reasoningEffort,omitempty"`
	Cron            string        `json:"cron"`
	Enabled         bool          `json:"enabled"`
	LastRunAt       *time.Time    `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time    `json:"nextRunAt,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	Revision        uint64        `json:"revision"`
}

// CreateScheduleRequest — schedules.create body. A new schedule is enabled.
type CreateScheduleRequest struct {
	Title           string        `json:"title,omitempty"`
	Instructions    string        `json:"instructions"`
	Workspace       *WorkspaceRef `json:"workspace,omitempty"`
	Provider        string        `json:"provider,omitempty"`
	Model           string        `json:"model,omitempty"`
	ReasoningEffort string        `json:"reasoningEffort,omitempty"`
	Cron            string        `json:"cron"`
}

// ScheduleWorkspaceMode selects the Runtime-owned workspace binding for a
// schedule update. Omitting it preserves the current binding.
type ScheduleWorkspaceMode string

const (
	// ScheduleWorkspaceDefault removes an explicit binding so future firings use
	// ServerInfo.defaultWorkspace.
	ScheduleWorkspaceDefault ScheduleWorkspaceMode = "default"
)

// UpdateScheduleRequest — schedules.update body. Editable fields form a
// revision-checked partial patch. Workspace sets an explicit binding;
// WorkspaceMode="default" clears one. Omitting both preserves the binding, and
// they are mutually exclusive.
type UpdateScheduleRequest struct {
	ID               string                `json:"id"`
	ExpectedRevision uint64                `json:"expectedRevision"`
	Title            *string               `json:"title,omitempty"`
	Instructions     *string               `json:"instructions,omitempty"`
	Workspace        *WorkspaceRef         `json:"workspace,omitempty"`
	WorkspaceMode    ScheduleWorkspaceMode `json:"workspaceMode,omitempty"`
	Provider         *string               `json:"provider,omitempty"`
	Model            *string               `json:"model,omitempty"`
	ReasoningEffort  *string               `json:"reasoningEffort,omitempty"`
	Cron             *string               `json:"cron,omitempty"`
	Enabled          *bool                 `json:"enabled,omitempty"`
}

// DeleteScheduleRequest — schedules.delete body.
type DeleteScheduleRequest struct {
	ID string `json:"id"`
}

// RunScheduleNowRequest — schedules.runNow body.
type RunScheduleNowRequest struct {
	ID string `json:"id"`
}

type RunScheduleNowResponse struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}
