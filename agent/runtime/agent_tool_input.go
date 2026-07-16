package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
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

// decodeDynamicToolArguments decodes a tool argument payload into a
// newly allocated value of inputType and returns it as [any].
//
// Empty payloads yield the zero value for the target type. When
// inputType is nil, decoding targets [any] and follows the same empty-input
// behavior as [decodeToolArguments].
func decodeDynamicToolArguments(agentName, operation string, inputType reflect.Type, arguments string) (any, error) {
	if inputType == nil {
		return decodeToolArguments[any](agentName, operation, arguments)
	}

	value := reflect.New(inputType)
	if arguments == "" {
		return value.Elem().Interface(), nil
	}

	if err := json.Unmarshal([]byte(arguments), value.Interface()); err != nil {
		return nil, fmt.Errorf("%s: parse input as %s for %s: %w", agentName, inputType.String(), operation, err)
	}

	return value.Elem().Interface(), nil
}
