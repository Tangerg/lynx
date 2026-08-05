package turn

import "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"

// ToolPresenter owns tool-specific activity and result projection.
type ToolPresenter interface {
	Activity(toolName string, arguments tool.Arguments) string
	Present(toolName string, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}
