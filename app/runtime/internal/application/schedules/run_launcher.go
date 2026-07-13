package schedules

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// RunUseCases is the schedule application's narrow view of the complete run
// entry point. Scheduled execution never calls a Delivery handler.
type RunUseCases interface {
	Start(ctx context.Context, cmd runs.StartCommand) (runs.StartResult, error)
}

// ProjectorForSchedule supplies the temporary Batch-1 projection adapter for a
// scheduled prompt. Batch 2 removes it when runs owns canonical event reduction.
type ProjectorForSchedule func(schedule.Schedule) runs.ProjectorFactory

// RunLauncher turns a due schedule into a headless application Run. It owns the
// schedule-specific defaults; the runs coordinator owns session creation,
// admission, execution, and lifecycle.
type RunLauncher struct {
	runs       RunUseCases
	defaultCwd string
	projector  ProjectorForSchedule
	fired      func(scheduleID string)
}

// NewRunLauncher builds the scheduled-run execution strategy. fired is an
// optional outward notification emitted after the run is accepted.
func NewRunLauncher(runUseCases RunUseCases, defaultCwd string, projector ProjectorForSchedule, fired func(string)) RunLauncher {
	return RunLauncher{runs: runUseCases, defaultCwd: defaultCwd, projector: projector, fired: fired}
}

// StartScheduledRun starts one schedule through the same Application Runs entry
// point as transports, then immediately drops the unused event subscription.
func (l RunLauncher) StartScheduledRun(ctx context.Context, sc schedule.Schedule) (string, error) {
	cwd := sc.Cwd
	if cwd == "" {
		cwd = l.defaultCwd
	}
	title := sc.Title
	if title == "" {
		title = "Scheduled run"
	}
	var projector runs.ProjectorFactory
	if l.projector != nil {
		projector = l.projector(sc)
	}
	fireCtx, cancel := context.WithCancel(ctx)
	result, err := l.runs.Start(fireCtx, runs.StartCommand{
		DefaultCwd:      cwd,
		NewSessionTitle: title,
		Message:         sc.Prompt,
		Provider:        sc.Provider,
		Model:           sc.Model,
		OpeningUserText: sc.Prompt,
		NewProjector:    projector,
	})
	cancel()
	if err != nil {
		return "", err
	}
	if l.fired != nil {
		l.fired(sc.ID)
	}
	return result.SessionID, nil
}
