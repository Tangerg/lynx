package turn

import (
	"encoding/json"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func decodeToolResult(toolName, arguments, output string) *tool.Result {
	if output == "" {
		return nil
	}
	result, err := tool.ParseResult([]byte(output))
	if err != nil {
		result = tool.StringResult(output)
	}
	var args map[string]any
	if json.Unmarshal([]byte(arguments), &args) != nil {
		args = nil
	}
	normalized := normalizeToolResult(toolName, args, result.Any())
	if encoded, err := json.Marshal(normalized); err == nil {
		if projected, err := tool.ParseResult(encoded); err == nil {
			result = projected
		}
	}
	return &result
}

func toolOutputText(toolName string, result *tool.Result) string {
	if !strings.EqualFold(toolName, "shell") || result == nil {
		return ""
	}
	data, err := json.Marshal(result.Any())
	if err != nil {
		return ""
	}
	var output struct {
		Output string `json:"output"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return ""
	}
	return output.Output
}
