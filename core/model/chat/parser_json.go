package chat

import (
	"encoding/json"
	"errors"
	"fmt"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

var _ StructuredParser[any] = (*JSONParser[any])(nil)

// JSONParser decodes JSON into a user-supplied generic type T, using a
// JSON Schema derived from T to instruct the model. The schema is
// generated once at construction; subsequent calls reuse the cached
// instructions string.
//
// Example:
//
//	type Recipe struct {
//	    Title string   `json:"title"`
//	    Steps []string `json:"steps,omitzero"`
//	}
//	parser := chat.NewJSONParser[Recipe]()
//	r, err := parser.Parse(`{"title":"pasta","steps":["boil","drain"]}`)
type JSONParser[T any] struct {
	cachedInstructions string
}

// NewJSONParser returns a [JSONParser] for T. The schema is generated
// eagerly so the first Instructions call is free.
func NewJSONParser[T any]() *JSONParser[T] {
	p := &JSONParser[T]{}
	p.cachedInstructions = p.buildInstructions()
	return p
}

// buildInstructions assembles the prompt fragment using a JSON Schema
// derived from T via reflection.
func (j *JSONParser[T]) buildInstructions() string {
	const tmpl = `[OUTPUT FORMAT]
JSON only - RFC8259 compliant

[RESTRICTIONS]
• No explanations or commentary
• No markdown formatting or code blocks
• No backticks or ` + "```json```" + ` wrappers
• Exact schema compliance required

[JSON SCHEMA]
%s

[EXPECTED OUTPUT]
Raw JSON object matching the schema above.`

	var zero T
	schema, _ := pkgjson.StringDefSchemaOf(zero)
	return fmt.Sprintf(tmpl, schema)
}

func (j *JSONParser[T]) Instructions() string { return j.cachedInstructions }

// Parse decodes raw output into T. Markdown fences are stripped first;
// reasoning wrappers are not — see [MapParser.Parse] for rationale.
func (j *JSONParser[T]) Parse(rawLLMOutput string) (T, error) {
	cleaned := removeMarkdownCodeBlockDelimiters(rawLLMOutput)

	var out T
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return out, errors.Join(
			err,
			fmt.Errorf("chat.JSONParser.Parse: cannot decode JSON into %T (cleaned=%q, raw=%q)",
				out, cleaned, rawLLMOutput),
		)
	}
	return out, nil
}
