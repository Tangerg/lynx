package turn

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func (s *memoryDispatcher) toolActivity(toolName string) string {
	if toolName == "" {
		return ""
	}
	if s.toolPresenter != nil {
		if activity := s.toolPresenter.Activity(toolName); activity != "" {
			return activity
		}
	}
	return "Calling " + toolName
}

func decodeToolResult(presenter ToolPresenter, toolName, arguments, output string) (*tool.Result, string) {
	if output == "" {
		return nil, ""
	}
	result, err := tool.ParseResult([]byte(output))
	if err != nil {
		result = tool.StringResult(output)
	}
	if presenter == nil {
		return &result, ""
	}
	parsedArguments, err := tool.ParseArguments(arguments)
	if err != nil {
		return &result, ""
	}
	presented, outputText := presenter.Present(toolName, parsedArguments, result)
	return &presented, outputText
}
