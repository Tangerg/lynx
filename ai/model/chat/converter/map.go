package converter

import (
	"encoding/json"
	"errors"
	"fmt"
)

var _ StructuredConverter[map[string]any] = (*MapConverter)(nil)

// MapConverter is a StructuredConverter implementation that converts
// the LLM output into a map[string]any by parsing JSON format.
type MapConverter struct{}

func NewMapConverter() *MapConverter {
	return &MapConverter{}
}

// GetFormat returns the format instructions for the LLM to output JSON data
// that matches the golang map[string]interface{} format.
func (m *MapConverter) GetFormat() string {
	return `[OUTPUT FORMAT]
JSON object only - RFC8259 compliant

[RESTRICTIONS]
• No explanations or commentary
• No markdown formatting or code blocks
• No backticks or ` + "```json```" + ` wrappers
• Must be a valid JSON object (key-value pairs)

[EXPECTED STRUCTURE]
{
  "key1": "value1",
  "key2": 123,
  "key3": true
}

[EXPECTED OUTPUT]
Raw JSON object with string keys and any valid JSON values.`
}

// Convert converts the raw LLM output string into a map[string]any by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present.
func (m *MapConverter) Convert(raw string) (map[string]any, error) {
	content := stripMarkdownCodeBlock(raw)
	rv := make(map[string]any)
	err := json.Unmarshal([]byte(content), &rv)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("cannot convert content %s to map, raw: %s", content, raw))
	}
	return rv, nil
}
