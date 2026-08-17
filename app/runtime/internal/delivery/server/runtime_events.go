package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func (s *Server) observeFileChanges(observe func(func(workspaceapp.FileChangeNotice))) {
	observe(func(change workspaceapp.FileChangeNotice) {
		s.workspaceHub.publish(protocol.RuntimeEvent{
			Type:      protocol.RuntimeFilesChanged,
			Workspace: workspaceRefFromPath(change.CWD),
			Paths:     change.Paths,
		})
	})
}

func (s *Server) observeInvalidations(observe func(func(invalidation.Notice))) {
	observe(func(notice invalidation.Notice) {
		if event, ok := runtimeEventFor(notice); ok {
			s.workspaceHub.publish(event)
		}
	})
}

func runtimeEventFor(notice invalidation.Notice) (protocol.RuntimeEvent, bool) {
	switch notice.Resource {
	case invalidation.Resync:
		return protocol.RuntimeEvent{Type: protocol.RuntimeResync, Topics: protocol.RuntimeTopics()}, true
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
	case invalidation.Knowledge:
		return protocol.RuntimeEvent{Type: protocol.RuntimeKnowledgeChanged}, true
	case invalidation.Hooks:
		return protocol.RuntimeEvent{Type: protocol.RuntimeHooksChanged}, true
	case invalidation.Skills:
		return protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged}, true
	case invalidation.MCP:
		return protocol.RuntimeEvent{
			Type: protocol.RuntimeMCPChanged, ServerIDs: notice.ServerIDs,
		}, true
	case invalidation.Models:
		return protocol.RuntimeEvent{Type: protocol.RuntimeModelsChanged}, true
	case invalidation.Approvals:
		return protocol.RuntimeEvent{Type: protocol.RuntimeApprovalsChanged}, true
	case invalidation.AgentMemory:
		return protocol.RuntimeEvent{Type: protocol.RuntimeAgentMemoryChanged}, true
	case invalidation.Codebase:
		return protocol.RuntimeEvent{Type: protocol.RuntimeCodebaseChanged}, true
	default:
		return protocol.RuntimeEvent{}, false
	}
}
