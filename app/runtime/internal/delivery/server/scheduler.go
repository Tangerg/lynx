package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// RunScheduler starts the scheduled-run worker until ctx is canceled.
func (s *Server) RunScheduler(ctx context.Context) {
	s.schedules.RunWorker(ctx, s.scheduledRunLauncher())
}

func (s *Server) scheduledRunLauncher() schedules.RunLauncher {
	return schedules.NewRunLauncher(
		s.coordinator,
		s.serverInfo.Cwd,
		func(sc schedule.Schedule) runs.ProjectorFactory {
			input := []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: sc.Prompt}}
			return s.segmentProjector(input)
		},
		func(scheduleID string) {
			s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
				Type:       protocol.WorkspaceEventSchedulesFired,
				ScheduleID: scheduleID,
			})
		},
	)
}
