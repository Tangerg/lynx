package turn

import (
	"encoding/json"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func decodeToolResult(output string) *tool.Result {
	if output == "" {
		return nil
	}
	result, err := tool.ParseResult([]byte(output))
	if err != nil {
		result = tool.StringResult(output)
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
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return ""
	}
	switch {
	case output.Stderr == "":
		return output.Stdout
	case output.Stdout == "":
		return output.Stderr
	default:
		return output.Stdout + "\n" + output.Stderr
	}
}
