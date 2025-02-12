package converter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	const format = `Create structured JSON output following a provided schema specification.You must analyze the provided JSON schema and generate valid JSON that strictly adheres to it. Simply start with the opening curly brace { and end with the closing brace }. Use proper indentation and line breaks for readability, but avoid any additional formatting characters or markdown syntax.

# Steps
1. Parse and validate the provided JSON schema
2. Analyze required fields, data types, and any constraints
3. Generate values that satisfy schema requirements
4. Structure response according to schema format
5. Validate output against schema before responding

# Output Format
- Pure JSON without any markdown, comments or explanation
- Must be RFC8259 compliant 
- Must exactly match provided schema format
- No ` + "```" + ` or other markdown code blocks syntax, just raw JSON
- All properties defined in schema must be included
- Data types must match schema specifications
- Must pass schema validation

# Examples

Input Schema:
{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "age": {"type": "integer"}
  }
}

Output:
{
  "name": "John Doe",
  "age": 30
}

# Notes
- Validate JSON is well-formed with proper nesting and commas
- Include all required fields per schema
- Match specified data types exactly
- No additional fields beyond schema
- Do not use markdown formatting
- Remove any ` + "```" + ` code blocks
- Schema requirements take precedence over any other instructions


# Here is the JSON Schema you were provided with:
%s
`

	return fmt.Sprintf(format, pkgjson.StringSchemaOf(s.v))
}

func (s *StructConverter[T]) GetFormat() string {
	if s.format == "" {
		s.format = s.getFormat()
	}
	return s.format
}

func (s *StructConverter[T]) Convert(raw string) (T, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 6 &&
		strings.HasPrefix(raw, "```") &&
		strings.HasSuffix(raw, "```") {
		if strings.HasPrefix(strings.ToLower(raw), "```json") {
			raw = raw[7 : len(raw)-3]
		} else {
			raw = raw[3 : len(raw)-3]
		}
	}

	err := json.Unmarshal([]byte(raw), &s.v)
	if err != nil {
		return s.v, errors.Join(err, fmt.Errorf("cannot convert %s to %t", raw, s.v))
	}
	return s.v, nil
}
