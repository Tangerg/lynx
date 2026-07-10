package protocol

import (
	"context"
	"time"
)

// Schedules is the schedules.* method group (API.md §7.9) — cron-triggered
// headless runs of a saved prompt. The runtime's scheduler worker fires them
// while the server process is up; a client manages the schedule set here. A schedule
// stores the final prompt text (the client may pre-fill it from a recipe), not
// a recipe reference.
type Schedules interface {
	// ListSchedules returns every schedule, newest-created first.
	ListSchedules(ctx context.Context) (*ListSchedulesResult, error)
	// CreateSchedule adds an (enabled) schedule. prompt + cron are required;
	// provider/model are paired (both or neither). Returns the stored schedule
	// (with its id + computed nextRunAt).
	CreateSchedule(ctx context.Context, in CreateScheduleRequest) (*Schedule, error)
	// UpdateSchedule full-replaces a schedule's editable fields by id and
	// recomputes nextRunAt from the (new) cron. Disabling clears nextRunAt.
	UpdateSchedule(ctx context.Context, in UpdateScheduleRequest) (*Schedule, error)
	// DeleteSchedule removes a schedule by id. A missing id is not an error.
	DeleteSchedule(ctx context.Context, in DeleteScheduleRequest) error
	// RunScheduleNow fires a schedule immediately (a manual extra run); it
	// records the firing without shifting the schedule's next due time.
	RunScheduleNow(ctx context.Context, in RunScheduleNowRequest) error
}

// Schedule is one scheduled run (API.md §4.12). Body/Prompt is the final text
// sent as the run's input. cron is a 5-field standard expression
// ("min hour dom month dow"). lastRunAt is omitted until first fired; nextRunAt
// is omitted when the schedule is disabled.
type Schedule struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Prompt    string     `json:"prompt"`
	Cwd       string     `json:"cwd,omitempty"`
	Provider  string     `json:"provider,omitempty"`
	Model     string     `json:"model,omitempty"`
	Cron      string     `json:"cron"`
	Enabled   bool       `json:"enabled"`
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// ListSchedulesResult — the schedules.list reply.
type ListSchedulesResult struct {
	Schedules []Schedule `json:"schedules"`
}

// CreateScheduleRequest — schedules.create body. A new schedule is enabled.
type CreateScheduleRequest struct {
	Title    string `json:"title,omitempty"`
	Prompt   string `json:"prompt"`
	Cwd      string `json:"cwd,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Cron     string `json:"cron"`
}

// UpdateScheduleRequest — schedules.update body (full-replace of the editable
// fields by id).
type UpdateScheduleRequest struct {
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Prompt   string `json:"prompt"`
	Cwd      string `json:"cwd,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Cron     string `json:"cron"`
	Enabled  bool   `json:"enabled"`
}

// DeleteScheduleRequest — schedules.delete body.
type DeleteScheduleRequest struct {
	ID string `json:"id"`
}

// RunScheduleNowRequest — schedules.runNow body.
type RunScheduleNowRequest struct {
	ID string `json:"id"`
}
