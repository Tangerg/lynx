package schedules

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// RunUseCases is the schedule application's narrow view of the complete run
// entry point. Scheduled execution never calls a Delivery handler.
type RunUseCases interface {
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// RunLauncher turns a due schedule into a headless application Run. It owns the
// schedule-specific defaults; the runs coordinator owns session creation,
// admission, execution, and lifecycle.
type RunLauncher struct {
	runs                 RunUseCases
	defaultWorkspacePath string
	fired                func(scheduleID string)
}

// NewRunLauncher builds the scheduled-run execution strategy. fired is an
// optional outward notification emitted after the run is accepted.
func NewRunLauncher(runUseCases RunUseCases, defaultWorkspacePath string, fired func(string)) RunLauncher {
	return RunLauncher{runs: runUseCases, defaultWorkspacePath: defaultWorkspacePath, fired: fired}
}

// StartScheduledRun starts one schedule through the same Application Runs entry
// point as transports, then immediately drops the unused event subscription.
func (l RunLauncher) StartScheduledRun(ctx context.Context, occurrence schedule.Occurrence) (RunHandle, error) {
	sc := occurrence.Schedule
	cwd := sc.Cwd
	if cwd == "" {
		cwd = l.defaultWorkspacePath
	}
	fireCtx, cancel := context.WithCancel(ctx)
	result, err := l.runs.Start(fireCtx, runs.StartCommand{
		RunID:                occurrence.RunID,
		NewSessionID:         occurrence.SessionID,
		ScheduleFiring:       occurrence.ID,
		DefaultWorkspacePath: cwd,
		NewSessionTitle:      sc.Title,
		ModelSelection:       sc.ModelSelection,
		Input:                []transcript.ContentBlock{{Kind: transcript.TextContent, Text: sc.Prompt}},
	})
	cancel()
	if err != nil {
		return RunHandle{}, err
	}
	if l.fired != nil {
		l.fired(sc.ID)
	}
	return RunHandle{SessionID: result.SessionID, RunID: result.RunID}, nil
}
