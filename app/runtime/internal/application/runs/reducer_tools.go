package runs

import "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"

func newToolInvocation(name string, arguments tool.Arguments, result *tool.Result) *ToolInvocation {
	return &ToolInvocation{
		Name:      name,
		Arguments: arguments,
		Result:    result,
	}
}

func parseToolArguments(raw string) (tool.Arguments, error) {
	return tool.ParseArguments(raw)
}
