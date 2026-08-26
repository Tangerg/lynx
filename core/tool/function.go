package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/core/chat"
)

// Func adapts a typed Go function to [Tool]. It owns the derived input
// contract, strict argument decoding, invocation, and result encoding.
//
// Func is immutable after construction and is safe for concurrent calls when
// the wrapped function is safe for concurrent calls.
type Func[In, Out any] struct {
	config   FuncConfig
	input    schemaContract
	function func(context.Context, In) (Out, error)
}

// FuncConfig describes a typed function tool. NewFunc derives InputSchema from
// In so the decoder and model-visible contract cannot drift independently.
type FuncConfig struct {
	Name        string
	Description string
}

// NewFunc adapts function to a Tool. In must be a struct or a pointer to a
// struct; use struct{} for a tool without arguments. Arguments are validated
// against the derived schema and decoded strictly, so missing required fields,
// constraint violations, and unknown JSON fields fail before invocation. A
// string result is returned verbatim; every other result is encoded as JSON.
func NewFunc[In, Out any](config FuncConfig, function func(context.Context, In) (Out, error)) (Func[In, Out], error) {
	var zero Func[In, Out]
	if function == nil {
		return zero, fmt.Errorf("%w: function is nil", ErrInvalidTool)
	}
	inputType := reflect.TypeFor[In]()
	if err := validateFuncInput(inputType); err != nil {
		return zero, fmt.Errorf("%w: %w", ErrInvalidTool, err)
	}
	input, err := newSchemaContract(inputType)
	if err != nil {
		return zero, fmt.Errorf("%w: %w", ErrInvalidTool, err)
	}
	definition := chat.ToolDefinition{
		Name:        config.Name,
		Description: config.Description,
		InputSchema: input.JSON(),
	}
	if err := definition.Validate(); err != nil {
		return zero, fmt.Errorf("%w: definition: %w", ErrInvalidTool, err)
	}
	return Func[In, Out]{
		config:   config,
		input:    input,
		function: function,
	}, nil
}

func validateFuncInput(input reflect.Type) error {
	if input == nil {
		return errors.New("tool: function input type is nil")
	}
	if input.Kind() == reflect.Pointer {
		input = input.Elem()
	}
	if input.Kind() != reflect.Struct {
		return fmt.Errorf("tool: function input type %s must be a struct or pointer to struct", input)
	}
	return nil
}

// Definition returns an independent snapshot of the function's tool contract.
func (f Func[In, Out]) Definition() chat.ToolDefinition {
	if f.function == nil {
		return chat.ToolDefinition{}
	}
	return chat.ToolDefinition{
		Name:        f.config.Name,
		Description: f.config.Description,
		InputSchema: f.input.JSON(),
	}
}

// Call strictly decodes arguments, invokes the wrapped function, and encodes
// its result for a model-facing tool response.
func (f Func[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	if f.function == nil {
		return "", fmt.Errorf("%w: function tool is nil", ErrInvalidTool)
	}
	input, err := f.input.decode[In](arguments)
	if err != nil {
		return "", fmt.Errorf("tool: decode function arguments: %w", err)
	}
	output, err := f.function(ctx, input)
	if err != nil {
		return "", err
	}
	result, err := f.encodeResult(output)
	if err != nil {
		return "", fmt.Errorf("tool: encode function result: %w", err)
	}
	return result, nil
}

func (Func[In, Out]) encodeResult(output Out) (string, error) {
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
