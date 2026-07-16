package toolport

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/tools"
)

const (
	// ToolRoleCoding is the role the main chat agent declares: the full coding
	// tool set plus the task delegation tool.
	ToolRoleCoding = "coding"
	// ToolRoleSubtask is the role a delegated sub-agent declares: the same
	// coding tools without task, so delegation cannot recurse.
	ToolRoleSubtask = "subtask"
)

// ToolResolver resolves role-specific tool groups and accepts the task tool
// that can only be built after the agent engine exists.
type ToolResolver interface {
	core.ToolGroupResolver
	UseTaskTool(tools.Tool)
}
