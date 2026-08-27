package chatclient

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	corejsonschema "github.com/Tangerg/scope/core/jsonschema"
	jsonrepair "github.com/silaswei-io/jsonrepair-go"
)

var (
	// ErrInvalidOutputFormat reports an unusable output format configuration.
	ErrInvalidOutputFormat = errors.New("chatclient: invalid output format")

	// ErrInvalidOutput reports a completed model response that cannot satisfy
	// its bound output contract.
	ErrInvalidOutput = errors.New("chatclient: invalid output")
)

// OutputFormat couples a provider-neutral request contract with the decoder
// that consumes its response stream. Pass one to [Client.Output].
type OutputFormat[T any] struct {
	contract chat.OutputFormat
	decoder  func([]byte) (T, error)
}

// outputDecoder owns the stateless algorithms used to interpret accumulated
// model output. Its zero value is ready for use.
type outputDecoder struct{}

func Text() OutputFormat[string] {
	decoder := outputDecoder{}
	return OutputFormat[string]{
		contract: chat.OutputFormat{Type: chat.OutputFormatText},
		decoder:  decoder.text,
	}
}

func JSON[T any]() OutputFormat[T] {
	decoder := outputDecoder{}
	return OutputFormat[T]{
		contract: chat.OutputFormat{Type: chat.OutputFormatJSON},
		decoder:  func(raw []byte) (T, error) { return decoder.json[T](raw, nil) },
	}
}

// JSONSchema returns a result format coupled to the named contract derived
// from T.
func JSONSchema[T any](name string) (OutputFormat[T], error) {
	schema, err := corejsonschema.For[T]()
	if err != nil {
		return OutputFormat[T]{}, fmt.Errorf("%w: derive schema: %w", ErrInvalidOutputFormat, err)
	}
	contract, err := chat.NewJSONSchemaOutputFormat(name, schema.JSON())
	if err != nil {
		return OutputFormat[T]{}, fmt.Errorf("%w: %w", ErrInvalidOutputFormat, err)
	}
	decoder := outputDecoder{}
	return OutputFormat[T]{
		contract: contract,
		decoder:  func(raw []byte) (T, error) { return decoder.json[T](raw, &schema) },
	}, nil
}

func (o OutputFormat[T]) validate() error {
	if err := o.contract.Validate(); err != nil {
		return fmt.Errorf("%w: contract: %w", ErrInvalidOutputFormat, err)
	}
	if o.decoder == nil {
		return fmt.Errorf("%w: nil decoder", ErrInvalidOutputFormat)
	}
	return nil
}

func (o OutputFormat[T]) decode(responses iter.Seq2[*chat.Response, error]) (T, error) {
	var zero T
	if responses == nil {
		return zero, fmt.Errorf("%w: nil response sequence", ErrInvalidOutputFormat)
	}

	var text strings.Builder
	seen := false
	for response, err := range responses {
		if err != nil {
			return zero, err
		}
		if response == nil {
			return zero, fmt.Errorf("%w: nil response", ErrInvalidOutput)
		}
		if err := response.Validate(); err != nil {
			return zero, fmt.Errorf("%w: response: %w", ErrInvalidOutput, err)
		}
		seen = true
		text.WriteString(response.Text())
	}
	if !seen {
		return zero, fmt.Errorf("%w: empty response sequence", ErrInvalidOutput)
	}
	value, err := o.decoder([]byte(text.String()))
	if err != nil {
		return zero, fmt.Errorf("%w: decode: %w", ErrInvalidOutput, err)
	}
	return value, nil
}

func once(response *chat.Response, err error) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		if err != nil {
			yield(nil, err)
			return
		}
		yield(response, nil)
	}
}

func (outputDecoder) text(raw []byte) (string, error) {
	return string(raw), nil
}

func (o outputDecoder) json[T any](raw []byte, schema *corejsonschema.Schema) (T, error) {
	if decoded, err := o.decodeJSON[T](raw, schema); err == nil {
		return decoded, nil
	}
	var matched T
	matches := 0
	var lastErr error
	for _, candidate := range jsonrepair.ExtractJSON(string(raw)) {
		decoded, err := o.decodeJSON[T]([]byte(candidate), schema)
		if err != nil {
			lastErr = err
			continue
		}
		matched = decoded
		matches++
	}
	if matches == 1 {
		return matched, nil
	}
	var zero T
	if matches > 1 {
		return zero, errors.New("multiple compatible JSON values in model output")
	}
	if lastErr != nil {
		return zero, fmt.Errorf("decode repaired JSON: %w", lastErr)
	}
	return zero, errors.New("no compatible JSON value in model output")
}

func (outputDecoder) decodeJSON[T any](raw []byte, schema *corejsonschema.Schema) (T, error) {
	var decoded T
	if schema != nil {
		if err := schema.Validate(raw); err != nil {
			return decoded, err
		}
	}
	if err := jsonv2.Unmarshal(raw, &decoded, jsonv2.RejectUnknownMembers(true)); err != nil {
		return decoded, err
	}
	return decoded, nil
}
