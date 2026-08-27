package runs

import (
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
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
