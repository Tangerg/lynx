package protocol

import (
	"context"
	"time"
)

// Background is the background.* method group — long-running tasks that
// outlive a single run (API.md §7.7). Gated on features.background.
// Subscribe output flows via notifications.background.update.
type Background interface {
	ListBackground(ctx context.Context, q PageQuery) (*Page[BackgroundTask], error)
	SubscribeBackground(ctx context.Context, taskID string) (<-chan BackgroundTask, error)
	CancelBackground(ctx context.Context, taskID string) error
}

// BackgroundStatus enumerates the wire states (API.md §4.10).
type BackgroundStatus string

const (
	BackgroundStatusRunning   BackgroundStatus = "running"
	BackgroundStatusCompleted BackgroundStatus = "completed"
	BackgroundStatusFailed    BackgroundStatus = "failed"
	BackgroundStatusCanceled  BackgroundStatus = "canceled"
)

// BackgroundTask is one entry in background.list + the
// notifications.background.update payload (API.md §4.10). Category is an
// open classification string (task kind, §2.6) — named `category`, not
// `kind`: `kind` never appears on the wire (§2.1).
type BackgroundTask struct {
	ID        string           `json:"id"`
	Category  string           `json:"category"`
	Status    BackgroundStatus `json:"status"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt *time.Time       `json:"updatedAt,omitempty"`
	Result    any              `json:"result,omitempty"`
	Error     *ProblemData     `json:"error,omitempty"`
}
