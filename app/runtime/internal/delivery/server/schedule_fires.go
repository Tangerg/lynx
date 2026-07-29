package server

import "github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"

// observeScheduleFires signals that a schedule moved. It was `schedules.fired`, which
// named one cause; a client that has to refetch the schedule list does so for a fired
// schedule, an edited one and a deleted one alike, and one topic per cause would have
// meant three subscriptions for one read.
func (s *Server) observeScheduleFires(source Source[string]) {
	source.Observe(func(scheduleID string) {
		s.PublishRuntimeEvent(protocol.RuntimeEvent{
			Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{scheduleID},
		})
	})
}
