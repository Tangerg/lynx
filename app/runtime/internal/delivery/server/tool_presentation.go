package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// presentToolResult performs only the final protocol projection. Tool results
// are normalized by the executor adapter before Application persists them.
func presentToolResult(tool transcript.ToolInvocation) any {
	if tool.Result == nil {
		return nil
	}
	return tool.Result.Any()
}
