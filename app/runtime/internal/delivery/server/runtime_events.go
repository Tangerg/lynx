package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
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

func (s *Server) observeChanges(source notificationSource[change.Notice]) {
	source.Observe(func(notice change.Notice) {
		if event, ok := runtimeEventFor(notice); ok {
			s.workspaceHub.publish(event)
		}
	})
}

func runtimeEventFor(notice change.Notice) (protocol.RuntimeEvent, bool) {
	switch notice.Resource {
	case change.Sessions:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeSessionsChanged, SessionIDs: notice.SessionIDs,
		}, true
	case change.Runs:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeRunsChanged, SessionIDs: notice.SessionIDs, RunIDs: notice.RunIDs,
		}, true
	case change.Interrupts:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeInterruptsChanged, SessionIDs: notice.SessionIDs, RunIDs: notice.RunIDs,
		}, true
	case change.Goals:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeGoalsChanged, SessionIDs: notice.SessionIDs,
		}, true
	case change.PlanState:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeStateChanged, Key: protocol.StatePlan, SessionIDs: notice.SessionIDs,
		}, true
	default:
		return protocol.RuntimeEvent{}, false
	}
}
