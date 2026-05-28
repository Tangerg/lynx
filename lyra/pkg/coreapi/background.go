package coreapi

import "context"

// BackgroundAPI is the background.* method group — long-running
// tasks the agent spawned that outlive a single run.
type BackgroundAPI interface {
	ListBackground(ctx context.Context) ([]BackgroundTask, error)
	StopBackground(ctx context.Context, taskID string) error
	SubscribeBackground(ctx context.Context, taskID string) (<-chan BackgroundUpdate, error)
}

// BackgroundStatus enumerates the wire states.
type BackgroundStatus string

const (
	BackgroundStatusRunning   BackgroundStatus = "running"
	BackgroundStatusStopped   BackgroundStatus = "stopped"
	BackgroundStatusSucceeded BackgroundStatus = "succeeded"
	BackgroundStatusFailed    BackgroundStatus = "failed"
)

// BackgroundTask is one entry in background.list (API.md §6.7).
type BackgroundTask struct {
	TaskID    string           `json:"taskId"`
	Label     string           `json:"label"`
	Status    BackgroundStatus `json:"status"`
	StartedAt string           `json:"startedAt"`         // ISO-8601
	Progress  *float64         `json:"progress,omitempty"` // 0..1
}

// BackgroundUpdate is the notifications/background/update payload
// (API.md §6.7).
type BackgroundUpdate struct {
	TaskID      string           `json:"taskId"`
	Status      BackgroundStatus `json:"status"`
	Progress    *float64         `json:"progress,omitempty"`
	OutputDelta string           `json:"outputDelta,omitempty"`
}
