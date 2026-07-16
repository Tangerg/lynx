package runs

import "encoding/json"

func newToolInvocation(name, argumentsJSON string, result any) *ToolInvocation {
	return &ToolInvocation{
		Name:      name,
		Arguments: parseArgs(argumentsJSON),
		Result:    result,
	}
}

func parseArgs(raw string) map[string]any {
	arguments := map[string]any{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &arguments)
	}
	return arguments
}
