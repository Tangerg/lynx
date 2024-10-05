package converter

import (
	"encoding/json"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

var _ StructuredConverter[any] = (*StructConverter[any])(nil)

type StructConverter[T any] struct {
	v      T
	format string
}

// NewStructConverterWithDefault the default value, If the generic type is any, assist in obtaining the specific type
func NewStructConverterWithDefault[T any](v T) *StructConverter[T] {
	return &StructConverter[T]{v: v}
}

func (s *StructConverter[T]) getFormat() string {
	const format = `
Your response should be in JSON format.
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Do not include markdown code blocks in your response.
Remove the ` + "```" + "json markdown surrounding the output including the trailing " + "```." +
		`
Here is the JSON Schema instance your output must adhere to: %s`

	return fmt.Sprintf(format, pkgjson.StringSchemaOf(s.v))
}

func (s *StructConverter[T]) GetFormat() string {
	if s.format == "" {
		s.format = s.getFormat()
	}
	return s.format
}

func (s *StructConverter[T]) Convert(raw string) (T, error) {
	err := json.Unmarshal([]byte(raw), &s.v)
	if err != nil {
		return s.v, err
	}
	return s.v, nil
}
