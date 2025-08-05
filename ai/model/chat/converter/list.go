package converter

import (
	"strings"
)

var _ StructuredConverter[[]string] = (*ListConverter)(nil)

// ListConverter is a StructuredConverter implementation that converts
// the LLM output into a slice of strings.
type ListConverter struct{}

func NewListConverter() *ListConverter {
	return &ListConverter{}
}

// GetFormat returns the format instructions for the LLM to output a comma-separated list.
func (l *ListConverter) GetFormat() string {
	return `[OUTPUT FORMAT]
Comma-separated list only

[RESTRICTIONS]
• No explanations or commentary
• No numbering or bullet points
• No quotes around individual items
• No leading or trailing text

[EXPECTED FORMAT]
item1, item2, item3, etc...

[EXPECTED OUTPUT]
Raw comma-separated values matching the format above.`
}

// Convert converts the raw LLM output string into a slice of strings by splitting
// on commas and trimming whitespace from each element.
func (l *ListConverter) Convert(raw string) ([]string, error) {
	splits := strings.Split(raw, ",")
	for i, split := range splits {
		splits[i] = strings.TrimSpace(split)
	}
	return splits, nil
}
