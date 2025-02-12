package converter

import (
	"strings"
)

var _ StructuredConverter[[]string] = (*SliceConverter)(nil)

type SliceConverter struct{}

func NewSliceConverter() *SliceConverter {
	return &SliceConverter{}
}

func (s *SliceConverter) GetFormat() string {
	const format = `
Generate a comma-separated list of values.

# Output Format
- Output should be only comma-separated values, with no leading text, trailing text or comments
- Each value should be followed by a comma and a single space
- The list should not end with a comma
- The output should be a single line
- No surrounding quotes, brackets, or other characters

# Examples

Input: List three colors
Output: red, blue, green

Input: List two animals
Output: cat, dog

# Notes
- Do not add any explanatory text or commentary
- Do not use quotes around values unless they are part of the value itself
- Do not add a period at the end
- Do not add line breaks
`
	return format
}

func (s *SliceConverter) Convert(raw string) ([]string, error) {
	splits := strings.Split(raw, ",")
	for i, split := range splits {
		splits[i] = strings.TrimSpace(split)
	}
	return splits, nil
}
