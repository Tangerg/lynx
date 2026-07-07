package runtime

import (
	"encoding/json"
	"fmt"
)

// decodeToolArguments decodes a tool argument payload into the generic type.
// Empty payloads yield the zero value of T, matching the LLM-tool contract
// where arguments may be omitted when all fields are optional.
func decodeToolArguments[T any](agentName, operation, arguments string) (T, error) {
	var args T
	if arguments == "" {
		return args, nil
	}

	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return args, fmt.Errorf("%s: parse input for %s: %w", agentName, operation, err)
	}

	return args, nil
}
