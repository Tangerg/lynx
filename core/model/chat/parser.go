package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// StructuredParser converts unstructured LLM output into a typed value
// of type T. Implementations pair an Instructions prompt fragment
// (telling the model the expected output shape) with a Parse function
// (decoding the model's reply). Implementations should be permissive
// on input — LLMs surround structured output with prose, code fences,
// and minor formatting drift. See [NewJSONParser] for a full example.
type StructuredParser[T any] interface {
	// Instructions is the prompt fragment that tells the LLM exactly how
	// to format its reply. Append it to the user message.
	Instructions() string

	// Parse decodes raw LLM output into the structured T.
	Parse(rawLLMOutput string) (T, error)
}

// removeMarkdownCodeBlockDelimiters strips a leading/trailing ``` fence
// from input. LLMs often wrap structured payloads in fenced code even
// when told not to; the parsers handle that quietly. Whitespace around
// the content is also trimmed.
func removeMarkdownCodeBlockDelimiters(input string) string {
	trimmed := strings.TrimSpace(input)

	if len(trimmed) < 6 {
		return trimmed
	}
	if !strings.HasPrefix(trimmed, "```") || !strings.HasSuffix(trimmed, "```") {
		return trimmed
	}

	// Single-line ``` content ``` form.
	firstNL := strings.Index(trimmed, "\n")
	if firstNL == -1 {
		return strings.TrimSpace(trimmed[3 : len(trimmed)-3])
	}

	// Multi-line: drop the opening fence (with its language tag) and
	// closing fence.
	return strings.TrimSpace(trimmed[firstNL+1 : len(trimmed)-3])
}

var _ StructuredParser[[]string] = (*ListParser)(nil)

// ListParser splits comma-separated LLM output into a string slice.
//
// Example:
//
//	parser := chat.NewListParser()
//	prompt := "List 5 fruits.\n" + parser.Instructions()
//	// model replies: "apple, banana, cherry, date, elderberry"
//	items, _ := parser.Parse(text) // ["apple","banana","cherry","date","elderberry"]
type ListParser struct{}

// NewListParser returns a [ListParser]. The struct is stateless; sharing
// one across goroutines is fine.
func NewListParser() *ListParser { return &ListParser{} }

// Instructions returns prompt text that asks the model for raw
// comma-separated values with no decoration.
func (l *ListParser) Instructions() string {
	return `[OUTPUT FORMAT]
Comma-separated list only

[RESTRICTIONS]
• No explanations or commentary
• No numbering or bullet points
• No quotes around individual items
• No leading or trailing text

[EXPECTED FORMAT]
item1, item2, item3, etc...

[EXPECTED OUTPUT]
Raw comma-separated values matching the format above.`
}

// Parse splits on commas and trims whitespace from each element. It is
// permissive — it never returns an error.
func (l *ListParser) Parse(rawLLMOutput string) ([]string, error) {
	parts := strings.Split(rawLLMOutput, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts, nil
}

var _ StructuredParser[map[string]any] = (*MapParser)(nil)

// MapParser decodes a JSON object into a map[string]any. Useful when
// the schema is dynamic or when you only care about a few fields.
type MapParser struct{}

// NewMapParser returns a [MapParser]. The struct is stateless.
func NewMapParser() *MapParser { return &MapParser{} }

// Instructions returns prompt text that asks the model for a bare JSON
// object with no fences or commentary.
func (m *MapParser) Instructions() string {
	return `[OUTPUT FORMAT]
JSON object only - RFC8259 compliant

[RESTRICTIONS]
• No explanations or commentary
• No markdown formatting or code blocks
• No backticks or ` + "```json```" + ` wrappers
• Must be a valid JSON object (key-value pairs)

[EXPECTED STRUCTURE]
{
  "key1": "value1",
  "key2": 123,
  "key3": true
}

[EXPECTED OUTPUT]
Raw JSON object with string keys and any valid JSON values.`
}

// Parse decodes raw output into a map[string]any. Markdown code fences
// are stripped automatically; reasoning wrappers (<think>, ...) are not
// — that is the provider adapter's job, which routes reasoning into
// dedicated [ReasoningPart]s before the parser sees the text.
func (m *MapParser) Parse(rawLLMOutput string) (map[string]any, error) {
	cleaned := removeMarkdownCodeBlockDelimiters(rawLLMOutput)

	out := make(map[string]any)
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return nil, errors.Join(
			err,
			fmt.Errorf("chat.MapParser.Parse: cannot decode JSON (cleaned=%q, raw=%q)", cleaned, rawLLMOutput),
		)
	}
	return out, nil
}

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

// Instructions returns the cached prompt fragment.
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

var _ StructuredParser[any] = (*AnyParser)(nil)

// AnyParser is a type-erased [StructuredParser]: it forwards
// Instructions verbatim and runs the supplied parse function. Useful
// when collecting heterogeneous parsers in one slice or storing them on
// an interface field that expects StructuredParser[any].
type AnyParser struct {
	// FormatInstructions is the prompt fragment to inject.
	FormatInstructions string

	// ParseFunction is invoked by [AnyParser.Parse]. Required.
	ParseFunction func(rawLLMOutput string) (any, error)
}

// Instructions returns FormatInstructions verbatim.
func (a *AnyParser) Instructions() string { return a.FormatInstructions }

// Parse runs ParseFunction; returns an error when it is nil.
func (a *AnyParser) Parse(rawLLMOutput string) (any, error) {
	if a.ParseFunction == nil {
		return nil, errors.New("chat.AnyParser.Parse: ParseFunction is not initialized")
	}
	return a.ParseFunction(rawLLMOutput)
}

// WrapParserAsAny adapts a typed [StructuredParser] into [*AnyParser].
// The wrapped parser's Instructions are captured at wrap time; if the
// wrapped parser regenerates instructions later, the wrapper will not
// observe the change.
func WrapParserAsAny[T any](parser StructuredParser[T]) *AnyParser {
	return &AnyParser{
		FormatInstructions: parser.Instructions(),
		ParseFunction: func(rawLLMOutput string) (any, error) {
			return parser.Parse(rawLLMOutput)
		},
	}
}
