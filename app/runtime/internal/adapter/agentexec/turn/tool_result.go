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
	raw, _ := result.(map[string]any)
	stdout, _ := raw["stdout"].(string)
	stderr, _ := raw["stderr"].(string)
	switch {
	case stderr == "":
		return stdout
	case stdout == "":
		return stderr
	default:
		return stdout + "\n" + stderr
	}
}
