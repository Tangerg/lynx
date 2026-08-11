package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func (s *Server) observeFileChanges(source notificationSource[workspaceapp.FileChangeNotice]) {
	source.Observe(func(change workspaceapp.FileChangeNotice) {
		s.workspaceHub.publish(protocol.RuntimeEvent{
			Type:      protocol.RuntimeFilesChanged,
			Workspace: workspaceRefFromPath(change.CWD),
			Paths:     change.Paths,
		})
	})
}

func (s *Server) observeMCPStatusChanges(source notificationSource[mcpapp.ServerStatus]) {
	source.Observe(func(status mcpapp.ServerStatus) {
		s.workspaceHub.publish(protocol.RuntimeEvent{
			Type: protocol.RuntimeMCPChanged, ServerIDs: []string{status.Name},
		})
	})
}

func (s *Server) observeSkillChanges(source notificationSource[struct{}]) {
	source.Observe(func(struct{}) {
		s.workspaceHub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged})
	})
}

func (s *Server) observeScheduleFires(source notificationSource[string]) {
	source.Observe(func(scheduleID string) {
		s.workspaceHub.publish(protocol.RuntimeEvent{
			Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{scheduleID},
		})
	})
}

func (s *Server) observeInvalidations(source notificationSource[invalidation.Notice]) {
	source.Observe(func(notice invalidation.Notice) {
		if event, ok := runtimeEventFor(notice); ok {
			s.workspaceHub.publish(event)
		}
	})
}

func runtimeEventFor(notice invalidation.Notice) (protocol.RuntimeEvent, bool) {
	switch notice.Resource {
	case invalidation.Sessions:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeSessionsChanged, SessionIDs: notice.SessionIDs,
		}, true
	case invalidation.Runs:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeRunsChanged, SessionIDs: notice.SessionIDs, RunIDs: notice.RunIDs,
		}, true
	case invalidation.Interrupts:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeInterruptsChanged, SessionIDs: notice.SessionIDs, RunIDs: notice.RunIDs,
		}, true
	case invalidation.Goals:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeGoalsChanged, SessionIDs: notice.SessionIDs,
		}, true
	case invalidation.PlanState:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeStateChanged, Key: protocol.StatePlan, SessionIDs: notice.SessionIDs,
		}, true
	case invalidation.Schedules:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: notice.ScheduleIDs,
		}, true
	default:
		return protocol.RuntimeEvent{}, false
	}
}
