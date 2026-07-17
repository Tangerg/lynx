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

	// ToolNameReadToolResult is the model-facing name of the tool that reads an
	// offloaded tool result back by id. It is shared vocabulary across the
	// engine↔toolset boundary: the toolset registers the tool under this name and
	// the engine's tool-result eviction middleware excludes it from offloading
	// (evicting the read-back tool's own output would loop), so the two can never
	// drift apart.
	ToolNameReadToolResult = "read_tool_result"
)

// ToolResolver resolves role-specific tool groups and accepts the task tool
// that can only be built after the agent engine exists.
type ToolResolver interface {
	core.ToolGroupResolver
	UseTaskTool(tools.Tool)
}
