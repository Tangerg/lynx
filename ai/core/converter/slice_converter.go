package converter

import (
	"strings"
)

var _ StructuredConverter[[]string] = (*SliceConverter)(nil)

type SliceConverter struct {
}

func (s *SliceConverter) GetFormat() string {
	const format = `
Your response should be a list of comma separated values
eg: foo, bar, baz, ...
`
	return format
}

func (s *SliceConverter) Convert(raw string) ([]string, error) {
	return strings.Split(raw, ","), nil
}
