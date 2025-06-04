package converter

import (
	"strings"
)

var _ StructuredOutputConverter[[]string] = (*ListOutputConverter)(nil)

// ListOutputConverter is a StructuredOutputConverter implementation that converts
// the LLM output into a slice of strings.
type ListOutputConverter struct{}

// GetFormat returns the format instructions for the LLM to output a comma-separated list.
func (l *ListOutputConverter) GetFormat() string {
	return `
Respond with only a list of comma-separated values, without any leading or trailing text.
Example format: foo, bar, baz`
}

// Convert converts the raw LLM output string into a slice of strings by splitting
// on commas and trimming whitespace from each element.
func (l *ListOutputConverter) Convert(raw string) ([]string, error) {
	splits := strings.Split(raw, ",")
	for i, split := range splits {
		splits[i] = strings.TrimSpace(split)
	}
	return splits, nil
}
