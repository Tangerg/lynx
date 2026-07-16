package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// Config describes a typed function tool. The input schema is deliberately
// absent: [New] derives it from the input type so the decoder and the
// model-visible contract cannot drift independently.
type Config struct {
	Name        string
	Description string
}

// functionTool adapts an ordinary typed function to Tool. It is immutable and
// safe for concurrent calls when function is safe for concurrent calls.
type functionTool[In, Out any] struct {
	definition chat.ToolDefinition
	function   func(context.Context, In) (Out, error)
}

// New builds a Tool from an ordinary typed function. In must be a struct or a
// pointer to a struct; use struct{} for a parameterless tool. Arguments are
// decoded strictly according to their JSON fields, so unknown fields fail
// instead of being silently ignored. A string result is returned verbatim;
// every other result is encoded as JSON.
//
// Config, the generated definition, and the function are captured at
// construction time. Runtime policy such as retries, approval, concurrency,
// and direct return belongs to the agent/toolloop layer or an explicit Tool
// decorator.
func New[In, Out any](config Config, function func(context.Context, In) (Out, error)) (Tool, error) {
	definition := chat.ToolDefinition{
		Name:        config.Name,
		Description: config.Description,
		InputSchema: json.RawMessage(`{}`),
	}
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("%w: definition: %w", ErrInvalidTool, err)
	}
	if function == nil {
		return nil, fmt.Errorf("%w: function is nil", ErrInvalidTool)
	}
	if err := validateInputType(reflect.TypeFor[In]()); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidTool, err)
	}

	var input In
	schema, err := pkgjson.StringDefSchemaOf(input)
	if err != nil {
		return nil, fmt.Errorf("%w: derive input schema: %w", ErrInvalidTool, err)
	}
	definition.InputSchema = json.RawMessage(schema)
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("%w: generated definition: %w", ErrInvalidTool, err)
	}

	return &functionTool[In, Out]{
		definition: definition.Clone(),
		function:   function,
	}, nil
}

func validateInputType(input reflect.Type) error {
	if input == nil {
		return errors.New("tools: input type is nil")
	}
	if input.Kind() == reflect.Pointer {
		input = input.Elem()
	}
	if input.Kind() != reflect.Struct {
		return fmt.Errorf("tools: input type %s must be a struct or pointer to struct", input)
	}
	return nil
}

func (t *functionTool[In, Out]) Definition() chat.ToolDefinition {
	return t.definition.Clone()
}

func (t *functionTool[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	input, err := decodeArguments[In](arguments)
	if err != nil {
		return "", fmt.Errorf("tools: decode arguments: %w", err)
	}
	output, err := t.function(ctx, input)
	if err != nil {
		return "", err
	}
	result, err := encodeResult(output)
	if err != nil {
		return "", fmt.Errorf("tools: encode result: %w", err)
	}
	return result, nil
}

func decodeArguments[In any](arguments string) (In, error) {
	var input In
	if strings.TrimSpace(arguments) == "" {
		arguments = "{}"
	}
	if !strings.HasPrefix(strings.TrimSpace(arguments), "{") {
		return input, errors.New("arguments must be a JSON object")
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, err
	}
	if err := consumeEOF(decoder); err != nil {
		return input, err
	}
	return input, nil
}

func consumeEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func encodeResult[Out any](output Out) (string, error) {
	value := reflect.ValueOf(output)
	if value.IsValid() && value.Kind() == reflect.String {
		return value.String(), nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
