package converter

import (
	"encoding/json"
	"errors"
	"fmt"
)

var _ StructuredOutputConverter[map[string]any] = (*MapOutputConverter)(nil)

// MapOutputConverter is a StructuredOutputConverter implementation that converts
// the LLM output into a map[string]any by parsing JSON format.
type MapOutputConverter struct {
}

// GetFormat returns the format instructions for the LLM to output JSON data
// that matches the golang map[string]interface{} format.
func (m *MapOutputConverter) GetFormat() string {
	return `
Your response should be in JSON format.
The data structure for the JSON should match golang map[string]interface{} format.
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Remove the` + " '```json' markdown surrounding the output including the trailing '```'."
}

// Convert converts the raw LLM output string into a map[string]any by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present.
func (m *MapOutputConverter) Convert(raw string) (map[string]any, error) {
	content := stripMarkdownCodeBlock(raw)
	rv := make(map[string]any)
	err := json.Unmarshal([]byte(content), &rv)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("cannot convert content %s to map, raw: %s", content, raw))
	}
	return rv, nil
}
