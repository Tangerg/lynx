package scheduleflow

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

type ScheduledRunStarter interface {
	StartScheduled(context.Context, runflow.ScheduledStart) (*runflow.ScheduledRun, error)
}

// Launcher resolves the workspace at firing time and adapts a Schedule
// occurrence to Run admission. A default-bound Schedule therefore follows the
// Runtime's current default without storing a second workspace identity.
type Launcher struct {
	workspaces Workspaces
	runs       ScheduledRunStarter
}

func NewLauncher(workspaces Workspaces, runs ScheduledRunStarter) (*Launcher, error) {
	if workspaces == nil || runs == nil {
		return nil, fmt.Errorf("scheduleflow: workspace resolver and Run starter are required")
	}
	return &Launcher{workspaces: workspaces, runs: runs}, nil
}

func (launcher *Launcher) StartScheduledRun(
	ctx context.Context,
	request RunRequest,
) (StartedRun, error) {
	resolved, err := launcher.workspaces.Resolve(ctx, request.Schedule.Workspace())
	if err != nil {
		if ctx.Err() != nil {
			return StartedRun{}, ctx.Err()
		}
		return StartedRun{}, fmt.Errorf(
			"%w: schedule %s workspace resolution failed: %v",
			protocol.ErrWorkspaceUnavailable,
			request.Schedule.ID(),
			err,
		)
	}
	if !resolved.Available {
		return StartedRun{}, fmt.Errorf(
			"%w: schedule %s workspace is unavailable",
			protocol.ErrWorkspaceUnavailable,
			request.Schedule.ID(),
		)
	}
	started, err := launcher.runs.StartScheduled(ctx, runflow.ScheduledStart{
		ScheduleID: request.Schedule.ID(), OccurrenceID: request.OccurrenceID,
		SessionID: request.SessionID, RunID: request.RunID,
		Title: request.Schedule.Title(), Workspace: resolved.Workspace.Path(),
		Selection: request.Schedule.Selection(), Instruction: request.Schedule.Instructions(),
		FiredAt: request.FiredAt, AllowMissingSchedule: request.AllowMissingSchedule,
	})
	if err != nil {
		return StartedRun{}, err
	}
	return StartedRun{SessionID: started.SessionID, RunID: started.RunID}, nil
}

var _ Runner = (*Launcher)(nil)
