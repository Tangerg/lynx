package bootstrap

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/server"
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func newOperationService(stack Stack, info protocol.ServerInfo) (*server.Server, error) {
	return server.New(server.Config{
		Sessions:   stack.Sessions,
		MCP:        stack.MCP,
		Approvals:  stack.Approvals,
		Models:     stack.Models,
		Tools:      stack.Tools,
		Codebase:   stack.Codebase,
		ServerInfo: info,
		IdempotencyLimits: protocol.IdempotencyLimits{
			RetentionSeconds: int(idempotency.Retention.Seconds()),
		},
		Runs:                   stack.Runs,
		FileChanges:            stack.FileChanges,
		Invalidations:          stack.Invalidations,
		Queries:                stack.Queries,
		Usage:                  stack.Usage,
		Feedback:               stack.Feedback,
		Schedules:              stack.Schedules,
		ScheduleFiring:         stack.ScheduleFiring,
		Goals:                  stack.Goals,
		AgentMemory:            stack.AgentMemory,
		WorkspaceFiles:         stack.WorkspaceFiles,
		WorkspaceVCS:           stack.WorkspaceVCS,
		WorkspaceDiscovery:     stack.WorkspaceDiscovery,
		WorkspaceKnowledge:     stack.WorkspaceKnowledge,
		WorkspaceSkills:        stack.WorkspaceSkills,
		WorkspaceHooks:         stack.WorkspaceHooks,
		WorkspaceWatch:         stack.WorkspaceWatch,
		WorkspaceAuthoredWatch: stack.WorkspaceAuthoredWatch,
		GitAvailable:           stack.GitAvailable,
		PlanEnabled:            stack.PlanEnabled,
	})
}
