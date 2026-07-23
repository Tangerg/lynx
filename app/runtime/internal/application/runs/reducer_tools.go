package runs

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func newToolInvocation(name string, arguments tool.Arguments, result *tool.Result) *transcript.ToolInvocation {
	return &transcript.ToolInvocation{
		Name:      name,
		Arguments: arguments,
		Result:    result,
	}
}

func parseToolArguments(raw string) (tool.Arguments, error) {
	return tool.ParseArguments(raw)
}
