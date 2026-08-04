package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func (s *Server) observeFileChanges(source source[workspaceapp.FileChangeNotice]) {
	source.Observe(func(change workspaceapp.FileChangeNotice) {
		s.wsHub.publish(protocol.RuntimeEvent{
			Type:      protocol.RuntimeFilesChanged,
			Workspace: workspaceRefFromPath(change.Cwd),
			Paths:     change.Paths,
		})
	})
}

func (s *Server) observeMCPStatus(source source[integrations.MCPServerStatus]) {
	source.Observe(func(status integrations.MCPServerStatus) {
		s.wsHub.publish(protocol.RuntimeEvent{
			Type: protocol.RuntimeMCPChanged, ServerIDs: []string{status.Name},
		})
	})
}

func (s *Server) observeSkillChanges(source source[struct{}]) {
	source.Observe(func(struct{}) {
		s.wsHub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged})
	})
}

func (s *Server) observeScheduleFires(source source[string]) {
	source.Observe(func(scheduleID string) {
		s.wsHub.publish(protocol.RuntimeEvent{
			Type: protocol.RuntimeSchedulesChanged, ScheduleIDs: []string{scheduleID},
		})
	})
}

func (s *Server) observeChanges(source source[change.Notice]) {
	source.Observe(func(notice change.Notice) {
		if event, ok := runtimeEventFor(notice); ok {
			s.wsHub.publish(event)
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
