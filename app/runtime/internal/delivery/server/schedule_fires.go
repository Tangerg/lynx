package server

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

func (s *Server) observeScheduleFires(source ScheduleFireSource) {
	source.Observe(func(scheduleID string) {
		s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
			Type:       protocol.WorkspaceEventSchedulesFired,
			ScheduleID: scheduleID,
		})
	})
}
