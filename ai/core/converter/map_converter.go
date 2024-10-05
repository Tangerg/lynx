package converter

import (
	"encoding/json"
	"fmt"
	"strings"
)

var _ StructuredConverter[map[string]any] = (*MapConverter)(nil)

type MapConverter struct {
	v      map[string]any
	format string
}

// NewMapConverterWithExample If the generic type is any, assist in obtaining the specific type
func NewMapConverterWithExample(v map[string]any) *MapConverter {
	return &MapConverter{v: v}
}

func (m *MapConverter) getFormat() string {
	const format = `
Your response should be in JSON format.
The data structure for the JSON should match this example: %s
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Remove the ` + "```" + "json markdown surrounding the output including the trailing " + "```."

	marshal, _ := json.Marshal(m.v)
	return fmt.Sprintf(format, string(marshal))
}

func (m *MapConverter) GetFormat() string {
	if m.format == "" {
		m.format = m.getFormat()
	}
	return m.format
}

func (m *MapConverter) Convert(raw string) (map[string]any, error) {
	if strings.HasPrefix(raw, "```json") &&
		strings.HasSuffix(raw, "```") {
		raw = raw[7 : len(raw)-3]
	}
	rv := make(map[string]any)
	err := json.Unmarshal([]byte(raw), &rv)
	if err != nil {
		return nil, err
	}
	return rv, nil
}
