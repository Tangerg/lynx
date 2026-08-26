package schedules

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// RunStarter is schedule firing's narrow view of the complete Run entry point.
type RunStarter interface {
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// RunLauncher turns a due schedule into a headless application Run. It owns the
// schedule-specific defaults; the runs coordinator owns session creation,
// admission, execution, and lifecycle.
type RunLauncher struct {
	runs                 RunStarter
	defaultWorkspacePath string
}

// NewRunLauncher builds the scheduled-run execution strategy.
func NewRunLauncher(runStarter RunStarter, defaultWorkspacePath string) RunLauncher {
	return RunLauncher{runs: runStarter, defaultWorkspacePath: defaultWorkspacePath}
}

// StartScheduledRun starts one schedule through the shared Run entry point, then
// immediately drops the unused event subscription.
func (r RunLauncher) StartScheduledRun(ctx context.Context, occurrence schedule.Occurrence) (StartedRun, error) {
	scheduled := occurrence.Schedule
	workspacePath := scheduled.CWD
	if workspacePath == "" {
		workspacePath = r.defaultWorkspacePath
	}
	startCtx, cancel := context.WithCancel(ctx)
	result, err := r.runs.Start(startCtx, runs.StartCommand{
		RunID:                occurrence.RunID,
		NewSessionID:         occurrence.SessionID,
		ScheduleFiring:       occurrence.ID,
		DefaultWorkspacePath: workspacePath,
		NewSessionTitle:      scheduled.Title,
		ModelSelection:       scheduled.ModelSelection,
		Input:                []transcript.ContentBlock{{Kind: transcript.TextContent, Text: scheduled.Instructions}},
	})
	cancel()
	if err != nil {
		return StartedRun{}, err
	}
	return StartedRun{SessionID: result.SessionID, RunID: result.RunID}, nil
}
