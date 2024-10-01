package converter

import (
	"encoding/json"
)

var _ StructuredConverter[map[string]any] = (*MapConverter)(nil)

type MapConverter struct {
}

func (m *MapConverter) GetFormat() string {
	const format = `
Your response should be in JSON format.
The data structure for the JSON should match this example: %s
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Remove the ` + "```" + "json markdown surrounding the output including the trailing " + "```."

	return format
}

func (m *MapConverter) Convert(raw string) (map[string]any, error) {
	rv := make(map[string]any)
	return rv, json.Unmarshal([]byte(raw), &rv)
}
