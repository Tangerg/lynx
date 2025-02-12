package converter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var _ StructuredConverter[map[string]any] = (*MapConverter)(nil)

// NewMapConverterWithExample creates a new instance of the MapConverter struct,
// initialized with a map of string keys to values of any type. This function is
// particularly useful when the generic type is `any`, as it assists in obtaining
// the specific type for conversion or processing.
//
// Parameters:
//
// v map[string]any
//   - A map where the keys are strings and the values are of any type.
//   - This map serves as the initial data for the MapConverter, allowing for
//     flexible handling and conversion of various data types.
//
// Returns:
//
// *MapConverter
//   - Returns a pointer to a newly created MapConverter instance, initialized
//     with the provided map. This instance can be used to perform type-specific
//     conversions or manipulations on the map data.
func NewMapConverterWithExample(v map[string]any) *MapConverter {
	return &MapConverter{v: v}
}

type MapConverter struct {
	v      map[string]any
	format string
}

func (m *MapConverter) getFormat() string {
	const format = `Return JSON data matching a provided example format, ensuring RFC8259 compliance and compatibility with golang map deserialization. Simply start with the opening curly brace { and end with the closing brace }. Use proper indentation and line breaks for readability, but avoid any additional formatting characters or markdown syntax.

# Steps
1. Parse the provided example JSON structure carefully
2. Validate that all mandatory fields exist in the provided example
3. Generate output data following the exact same structure
4. Verify RFC8259 compliance:
   - Use UTF-8 encoding
   - Keys must be double-quoted strings
   - String values must be double-quoted
   - Numbers must be integers or decimal literals
   - Arrays/objects must be properly terminated
   - No trailing commas allowed
5. Validate golang map compatibility:
   - All keys must be valid identifiers
   - Values must be consistent primitive types
   - Nested objects must maintain type consistency

# Output Format
Raw JSON data with no markdown formatting or code blocks. The output must:
- Match the provided example structure exactly
- Include all fields from the example
- Use identical key names
- Maintain the same nesting levels
- Use compatible data types for golang mapping
- No ` + "```" + ` or other markdown code blocks syntax, just raw JSON
- Start directly with { and end with }, using proper indentation

# Examples
Input:
{
  "number": 1.23,
  "integer": 123,
  "object": {
	"boolean": false,
    "string": "request",
    "integer_array": [1, 2, 3],
    "string_array": ["a", "b", "c"]
  }
}

Output:
{
  "number": 4.56,
  "integer": 456,
  "object": {
	"boolean": true,
    "string": "response",
    "integer_array": [4, 5, 6],
    "string_array": ["d", "e", "f"]
  }
}

# Notes
- Do not include explanatory text
- Do not use markdown formatting
- Remove any ` + "```" + ` code blocks
	- Verify all quotation marks are double quotes (")
	- Ensure no trailing commas exist
	- Maintain exact whitespace as provided example

# Here is the JSON example instance you were provided with:
%s
`

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
	rv := make(map[string]any)
	err := json.Unmarshal([]byte(raw), &rv)
	if err != nil {
		return nil, errors.Join(err, errors.Join(err, fmt.Errorf("cannot convert %s to map", raw)))
	}
	return rv, nil
}
