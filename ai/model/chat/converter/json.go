package converter

import (
	"encoding/json"
	"errors"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

var _ StructuredConverter[any] = (*JSONConverter[any])(nil)

// JSONConverter is a generic StructuredConverter implementation that converts
// the LLM output into a structured type T by parsing JSON format with JSON Schema validation.
type JSONConverter[T any] struct {
	format string
}

func NewJSONConverter[T any]() *JSONConverter[T] {
	return &JSONConverter[T]{}
}

// genFormat generates the format instructions with JSON Schema for the LLM output.
// The schema is automatically derived from the generic type T.
func (s *JSONConverter[T]) genFormat() string {
	const template = `[OUTPUT FORMAT]
JSON only - RFC8259 compliant

[RESTRICTIONS]
• No explanations or commentary
• No markdown formatting or code blocks
• No backticks or ` + "```json```" + ` wrappers
• Exact schema compliance required

[JSON SCHEMA]
%s

[EXPECTED OUTPUT]
Raw JSON object matching the schema above.`
	var t T
	return fmt.Sprintf(template, pkgjson.StringDefSchemaOf(t))
}

// GetFormat returns the format instructions for the LLM to output JSON data
// that conforms to the JSON Schema of type T. The format is generated once and cached.
func (s *JSONConverter[T]) GetFormat() string {
	if s.format == "" {
		s.format = s.genFormat()
	}
	return s.format
}

// Convert converts the raw LLM output string into type T by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present.
func (s *JSONConverter[T]) Convert(raw string) (T, error) {
	content := stripMarkdownCodeBlock(raw)
	var t T
	err := json.Unmarshal([]byte(content), &t)
	if err != nil {
		return t, errors.Join(err, fmt.Errorf("cannot convert content %s to %T, raw: %s", content, t, raw))
	}
	return t, nil
}
