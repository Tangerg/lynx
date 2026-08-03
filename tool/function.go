package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
)

// Func adapts a typed Go function to [Tool]. It owns argument decoding and
// result encoding, but not schema derivation: callers provide the complete
// model-visible definition explicitly.
//
// Func is immutable after construction and is safe for concurrent calls when
// the wrapped function is safe for concurrent calls.
type Func[In, Out any] struct {
	definition chat.ToolDefinition
	function   func(context.Context, In) (Out, error)
}

// FuncConfig describes a typed function tool. NewFunc derives InputSchema from
// In so the decoder and model-visible contract cannot drift independently.
type FuncConfig struct {
	Name        string
	Description string
}

// NewFunc adapts function to a Tool. In must be a struct or a pointer to a
// struct; use struct{} for a tool without arguments. Arguments are decoded
// strictly, so unknown JSON fields fail. A string result is returned verbatim;
// every other result is encoded as JSON.
func NewFunc[In, Out any](config FuncConfig, function func(context.Context, In) (Out, error)) (*Func[In, Out], error) {
	if err := validateFuncInput(reflect.TypeFor[In]()); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidTool, err)
	}
	inputSchema, err := Schema[In]()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidTool, err)
	}
	definition := chat.ToolDefinition{
		Name:        config.Name,
		Description: config.Description,
		InputSchema: json.RawMessage(inputSchema),
	}
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("%w: definition: %w", ErrInvalidTool, err)
	}
	if function == nil {
		return nil, fmt.Errorf("%w: function is nil", ErrInvalidTool)
	}
	return &Func[In, Out]{
		definition: definition.Clone(),
		function:   function,
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
func (f *Func[In, Out]) Definition() chat.ToolDefinition {
	if f == nil {
		return chat.ToolDefinition{}
	}
	return f.definition.Clone()
}

// Call strictly decodes arguments, invokes the wrapped function, and encodes
// its result for a model-facing tool response.
func (f *Func[In, Out]) Call(ctx context.Context, arguments string) (string, error) {
	if f == nil || f.function == nil {
		return "", fmt.Errorf("%w: function tool is nil", ErrInvalidTool)
	}
	input, err := decodeFuncArguments[In](arguments)
	if err != nil {
		return "", fmt.Errorf("tool: decode function arguments: %w", err)
	}
	output, err := f.function(ctx, input)
	if err != nil {
		return "", err
	}
	result, err := encodeFuncResult(output)
	if err != nil {
		return "", fmt.Errorf("tool: encode function result: %w", err)
	}
	return result, nil
}

func decodeFuncArguments[In any](arguments string) (In, error) {
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
	if err := consumeFuncEOF(decoder); err != nil {
		return input, err
	}
	return input, nil
}

func consumeFuncEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func encodeFuncResult[Out any](output Out) (string, error) {
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
