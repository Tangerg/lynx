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

// NewStructConverterWithDefault creates a new instance of the StructConverter struct,
// initialized with a default value of a generic type T. This function is particularly
// useful when the generic type is `any`, as it assists in obtaining the specific type
// for conversion or processing.
//
// Type Parameters:
//
// T any
//   - Represents the generic type parameter, allowing for flexibility in handling
//     various data types. The type can be any valid Go type.
//
// Parameters:
//
// v T
//   - The default value of type T used to initialize the StructConverter.
//   - This value serves as the initial data for the StructConverter, enabling
//     type-specific conversions or manipulations.
//
// Returns:
//
// *StructConverter[T]
//   - Returns a pointer to a newly created StructConverter instance, initialized
//     with the provided default value of type T. This instance can be used to
//     perform type-specific conversions or manipulations on the data.
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
