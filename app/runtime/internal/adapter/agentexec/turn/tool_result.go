package turn

import (
	"encoding/json"
	"strings"
)

func decodeToolResult(output string) any {
	if output == "" {
		return nil
	}
	var result any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return output
	}
	return result
}

func toolOutputText(toolName string, result any) string {
	if !strings.EqualFold(toolName, "shell") {
		return ""
	}
	data, err := json.Marshal(result)
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
