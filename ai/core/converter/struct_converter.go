package converter

import (
	"encoding/json"
)

var _ StructuredConverter[any] = (*StructConverter[any])(nil)

type StructConverter[T any] struct {
}

func (s *StructConverter[T]) GetFormat() string {
	const format = `
Your response should be in JSON format.
Do not include any explanations, only provide a RFC8259 compliant JSON response following this format without deviation.
Do not include markdown code blocks in your response.
Remove the ` + "```" + "json markdown surrounding the output including the trailing " + "```." +
		`
Here is the JSON Schema instance your output must adhere to: %s`

	return format
}

func (s *StructConverter[T]) Convert(raw string) (T, error) {
	var rv T
	return rv, json.Unmarshal([]byte(raw), &rv)
}
