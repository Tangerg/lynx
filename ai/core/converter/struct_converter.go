package converter

import (
	"encoding/json"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

var _ StructuredConverter[any] = (*StructConverter[any])(nil)

type StructConverter[T any] struct {
	v          T
	jsonSchema string
	format     string
}

// SetV If the generic type is any, assist in obtaining the specific type
func (s *StructConverter[T]) SetV(v T) {
	s.v = v
}

func (s *StructConverter[T]) getFormat() string {
	const format = `
Your response should be in JSON format.
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Do not include markdown code blocks in your response.
Remove the ` + "```" + "json markdown surrounding the output including the trailing " + "```." +
		`
Here is the JSON Schema instance your output must adhere to: %s`
	s.jsonSchema = pkgjson.StringSchemaOf(s.v)
	return fmt.Sprintf(format, s.jsonSchema)
}

func (s *StructConverter[T]) GetFormat() string {
	if s.format == "" {
		s.format = s.getFormat()
	}
	return s.format
}

func (s *StructConverter[T]) Convert(raw string) (T, error) {
	var rv T
	return rv, json.Unmarshal([]byte(raw), &rv)
}
