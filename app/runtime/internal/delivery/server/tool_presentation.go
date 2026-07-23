package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/component/toolpresentation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// presentToolResult performs only the final protocol projection. Concrete tool
// result schemas are owned by the shared tool-presentation component.
func presentToolResult(tool transcript.ToolInvocation) any {
	if tool.Result == nil {
		return nil
	}
	return toolpresentation.Present(tool.Name, tool.Arguments.Map(), tool.Result.Any())
}

func toolActivity(name string) string { return toolpresentation.Activity(name) }
