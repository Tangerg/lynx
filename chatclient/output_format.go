package chatclient

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
)

// ErrInvalidOutputFormat reports an unusable output format or response stream.
var ErrInvalidOutputFormat = errors.New("chatclient: invalid output format")

// OutputFormat couples a provider-neutral request contract with the decoder
// that consumes its response stream. Pass one to [Client.Output].
type OutputFormat[T any] struct {
	contract chat.OutputFormat
	decoder  func([]byte) (T, error)
}

// outputDecoder owns the stateless algorithms used to interpret accumulated
// model output. Its zero value is ready for use.
type outputDecoder struct{}

// Text returns a plain-text result format.
func Text() OutputFormat[string] {
	decoder := outputDecoder{}
	return OutputFormat[string]{
		contract: chat.OutputFormat{Type: chat.OutputFormatText},
		decoder:  decoder.text,
	}
}

// JSON returns a result format for one JSON object.
func JSON[T any]() OutputFormat[T] {
	decoder := outputDecoder{}
	return OutputFormat[T]{contract: chat.OutputFormat{Type: chat.OutputFormatJSON}, decoder: decoder.json[T]}
}

// JSONSchema returns a result format coupled to a named JSON Schema contract.
func JSONSchema[T any](name string, schema []byte) (OutputFormat[T], error) {
	contract, err := chat.NewJSONSchemaOutputFormat(name, schema)
	if err != nil {
		return OutputFormat[T]{}, fmt.Errorf("%w: %w", ErrInvalidOutputFormat, err)
	}
	decoder := outputDecoder{}
	return OutputFormat[T]{contract: contract, decoder: decoder.json[T]}, nil
}

func (f OutputFormat[T]) validate() error {
	if err := f.contract.Validate(); err != nil {
		return fmt.Errorf("%w: contract: %w", ErrInvalidOutputFormat, err)
	}
	if f.decoder == nil {
		return fmt.Errorf("%w: nil decoder", ErrInvalidOutputFormat)
	}
	return nil
}

func (f OutputFormat[T]) decode(responses iter.Seq2[*chat.Response, error]) (T, error) {
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
			return zero, fmt.Errorf("%w: nil response", ErrInvalidOutputFormat)
		}
		if err := response.Validate(); err != nil {
			return zero, fmt.Errorf("%w: response: %w", ErrInvalidOutputFormat, err)
		}
		seen = true
		text.WriteString(response.Text())
	}
	if !seen {
		return zero, fmt.Errorf("%w: empty response sequence", ErrInvalidOutputFormat)
	}
	value, err := f.decoder([]byte(text.String()))
	if err != nil {
		return zero, fmt.Errorf("chatclient: decode result: %w", err)
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

func (d outputDecoder) json[T any](raw []byte) (T, error) {
	var lastErr error
	for _, candidate := range d.jsonCandidates(raw) {
		attempts := [][]byte{candidate}
		if repaired, changed := d.escapeStringControls(candidate); changed {
			attempts = append(attempts, repaired)
		}
		for _, attempt := range attempts {
			var decoded T
			if err := jsonv2.Unmarshal(attempt, &decoded); err == nil {
				return decoded, nil
			} else {
				lastErr = err
			}
		}
	}
	var zero T
	if lastErr == nil {
		lastErr = errors.New("no complete JSON object or array found")
	}
	return zero, fmt.Errorf("invalid JSON: %w", lastErr)
}

func (d outputDecoder) jsonCandidates(raw []byte) [][]byte {
	trimmed := bytes.TrimSpace(raw)
	candidates := make([][]byte, 0, 3)
	candidates = d.appendUnique(candidates, trimmed)
	if fenced, ok := d.stripMarkdownFence(trimmed); ok {
		candidates = d.appendUnique(candidates, fenced)
	}
	if balanced, ok := d.firstBalancedJSON(trimmed); ok {
		candidates = d.appendUnique(candidates, balanced)
	}
	return candidates
}

func (outputDecoder) appendUnique(values [][]byte, candidate []byte) [][]byte {
	if len(candidate) == 0 {
		return values
	}
	for _, value := range values {
		if bytes.Equal(value, candidate) {
			return values
		}
	}
	return append(values, candidate)
}

func (outputDecoder) stripMarkdownFence(raw []byte) ([]byte, bool) {
	if !bytes.HasPrefix(raw, []byte("```")) || !bytes.HasSuffix(raw, []byte("```")) || len(raw) < 6 {
		return nil, false
	}
	openerEnd := bytes.IndexByte(raw, '\n')
	if openerEnd < 0 {
		return nil, false
	}
	language := strings.TrimSpace(string(raw[3:openerEnd]))
	if language != "" && !strings.EqualFold(language, "json") {
		return nil, false
	}
	return bytes.TrimSpace(raw[openerEnd+1 : len(raw)-3]), true
}

func (d outputDecoder) firstBalancedJSON(raw []byte) ([]byte, bool) {
	var found []byte
	for start := 0; start < len(raw); start++ {
		value := raw[start]
		if value != '{' && value != '[' {
			continue
		}
		end, ok := d.balancedJSONEnd(raw, start)
		if !ok {
			return nil, false
		}
		if found != nil {
			return nil, false
		}
		found = bytes.TrimSpace(raw[start:end])
		start = end - 1
	}
	return found, found != nil
}

func (d outputDecoder) balancedJSONEnd(raw []byte, start int) (int, bool) {
	stack := make([]byte, 0, 8)
	inString := false
	escaped := false
	for index := start; index < len(raw); index++ {
		value := raw[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch value {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch value {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, value)
		case '}', ']':
			if len(stack) == 0 || !d.matchingDelimiters(stack[len(stack)-1], value) {
				return 0, false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				return index + 1, true
			}
		}
	}
	return 0, false
}

func (outputDecoder) matchingDelimiters(open, close byte) bool {
	return open == '{' && close == '}' || open == '[' && close == ']'
}

func (outputDecoder) escapeStringControls(raw []byte) ([]byte, bool) {
	var repaired bytes.Buffer
	repaired.Grow(len(raw))
	inString := false
	escaped := false
	changed := false
	const hexadecimal = "0123456789abcdef"
	for _, value := range raw {
		if inString && !escaped && value < 0x20 {
			changed = true
			switch value {
			case '\b':
				repaired.WriteString(`\b`)
			case '\f':
				repaired.WriteString(`\f`)
			case '\n':
				repaired.WriteString(`\n`)
			case '\r':
				repaired.WriteString(`\r`)
			case '\t':
				repaired.WriteString(`\t`)
			default:
				repaired.WriteString(`\u00`)
				repaired.WriteByte(hexadecimal[value>>4])
				repaired.WriteByte(hexadecimal[value&0x0f])
			}
			continue
		}
		repaired.WriteByte(value)
		if inString {
			if escaped {
				escaped = false
			} else if value == '\\' {
				escaped = true
			} else if value == '"' {
				inString = false
			}
		} else if value == '"' {
			inString = true
		}
	}
	return repaired.Bytes(), changed
}
