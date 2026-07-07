package chat

import (
	"encoding/json"
	"errors"
	"fmt"
)

var _ StructuredParser[map[string]any] = (*MapParser)(nil)

// MapParser decodes a JSON object into a map[string]any. Useful when
// the schema is dynamic or when you only care about a few fields.
type MapParser struct{}

// NewMapParser returns a [MapParser]. The struct is stateless.
func NewMapParser() *MapParser { return &MapParser{} }

// Instructions returns prompt text that asks the model for a bare JSON
// object with no fences or commentary.
func (m *MapParser) Instructions() string {
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

// Parse decodes raw output into a map[string]any. Markdown code fences
// are stripped automatically; reasoning wrappers (<think>, ...) are not
// — that is the provider adapter's job, which routes reasoning into
// dedicated [ReasoningPart]s before the parser sees the text.
func (m *MapParser) Parse(rawLLMOutput string) (map[string]any, error) {
	cleaned := removeMarkdownCodeBlockDelimiters(rawLLMOutput)

	out := make(map[string]any)
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return nil, errors.Join(
			err,
			fmt.Errorf("chat.MapParser.Parse: cannot decode JSON (cleaned=%q, raw=%q)", cleaned, rawLLMOutput),
		)
	}
	return out, nil
}
