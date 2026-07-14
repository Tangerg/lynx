package chatclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidOutput reports an unusable structured-output definition.
var ErrInvalidOutput = errors.New("chatclient: invalid structured output")

// Decoder converts model text into T.
type Decoder[T any] func(string) (T, error)

// Output pairs prompt instructions with a decoding function. It is an
// ordinary value rather than a converter hierarchy, so callers can define a
// custom output with a string and function literal.
type Output[T any] struct {
	Instructions string
	Decode       Decoder[T]
}

// Validate verifies that Output has a decoder and meaningful instructions
// when instructions are present. Empty instructions are valid for callers
// that describe the response shape elsewhere.
func (o Output[T]) Validate() error {
	if o.Decode == nil {
		return fmt.Errorf("%w: nil decoder", ErrInvalidOutput)
	}
	if o.Instructions != "" && strings.TrimSpace(o.Instructions) == "" {
		return fmt.Errorf("%w: instructions contain only whitespace", ErrInvalidOutput)
	}
	return nil
}

const jsonInstructions = `Respond with only RFC 8259-compliant JSON matching the requested structure.
Do not include explanations, commentary, markdown fences, or any leading or trailing text.`

// JSON returns an Output that decodes JSON into T using encoding/json. The
// instructions do not invent a schema; use JSONSchema when one is available.
func JSON[T any]() Output[T] {
	return Output[T]{Instructions: jsonInstructions, Decode: decodeJSON[T]}
}

// JSONSchema returns a JSON Output whose instructions include a caller-owned
// JSON Schema object. Schema generation deliberately belongs to optional
// integrations above chatclient, keeping this module stdlib + Core only.
func JSONSchema[T any](schema json.RawMessage) (Output[T], error) {
	var object map[string]json.RawMessage
	if len(schema) == 0 || json.Unmarshal(schema, &object) != nil || object == nil {
		return Output[T]{}, fmt.Errorf("%w: schema must be a JSON object", ErrInvalidOutput)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, schema); err != nil {
		return Output[T]{}, fmt.Errorf("%w: compact schema: %w", ErrInvalidOutput, err)
	}
	return Output[T]{
		Instructions: jsonInstructions + "\n\nJSON Schema:\n" + compact.String(),
		Decode:       decodeJSON[T],
	}, nil
}

const commaSeparatedInstructions = `Respond with only comma-separated values.
Do not include numbering, bullets, quotes, explanations, or leading or trailing text.`

// CommaSeparated returns an Output that splits a comma-separated response and
// trims surrounding whitespace from every item. Blank output becomes an empty
// slice.
func CommaSeparated() Output[[]string] {
	return Output[[]string]{
		Instructions: commaSeparatedInstructions,
		Decode: func(raw string) ([]string, error) {
			trimmed := strings.TrimSpace(raw)
			if trimmed == "" {
				return []string{}, nil
			}
			items := strings.Split(trimmed, ",")
			for i := range items {
				items[i] = strings.TrimSpace(items[i])
			}
			return items, nil
		},
	}
}

func decodeJSON[T any](raw string) (T, error) {
	cleaned := removeMarkdownFence(raw)
	var decoded T
	if err := json.Unmarshal([]byte(cleaned), &decoded); err != nil {
		return decoded, fmt.Errorf("chatclient: decode JSON output: %w", err)
	}
	return decoded, nil
}

func removeMarkdownFence(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") || !strings.HasSuffix(trimmed, "```") || len(trimmed) < 6 {
		return trimmed
	}
	body := trimmed[3 : len(trimmed)-3]
	if newline := strings.IndexByte(body, '\n'); newline >= 0 {
		opener := strings.TrimSpace(body[:newline])
		if opener == "" || strings.EqualFold(opener, "json") {
			body = body[newline+1:]
		}
	}
	return strings.TrimSpace(body)
}
