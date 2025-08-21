package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	pkgjson "github.com/Tangerg/lynx/pkg/json"
)

// StructuredParser defines an interface for converting unstructured LLM (Large Language Model)
// output into structured data of type T. This interface addresses the common challenge where LLMs
// generate natural language responses that need to be transformed into specific data structures.
//
// The interface follows a two-phase approach:
//  1. Instructions() provides formatting guidelines to guide the LLM's output generation
//  2. Parse() transforms the raw LLM response into the desired structured format
//
// Type parameter T represents the target data structure that the raw LLM output will be converted to.
// T can be any type including structs, slices, maps, or primitive types.
type StructuredParser[T any] interface {
	// Instructions returns a string containing detailed formatting instructions that should be
	// included in the LLM prompt to guide the model's output generation. These instructions
	// describe the expected structure, format, and constraints for the response.
	//
	// The returned string is typically appended to the user's prompt to inform the LLM
	// about the desired output format. This may include JSON schema requirements,
	// list formatting rules, field specifications, or other structural constraints.
	Instructions() string

	// Parse transforms the raw, unstructured output from an LLM into structured
	// data of type T. This method handles the parsing, validation, and type conversion
	// necessary to extract meaningful data from natural language responses.
	//
	// The implementation should be robust enough to handle variations in LLM output format,
	// including extra explanatory text, slight formatting deviations, or minor inconsistencies
	// that commonly occur in language model responses.
	//
	// Parameters:
	//   - rawLLMOutput: The unprocessed string response from the LLM, which may contain the target
	//     data mixed with additional natural language text, formatting, or explanations
	//
	// Returns:
	//   - T: The successfully parsed and converted structured data
	//   - error: An error if the conversion fails due to parsing issues, validation failures,
	//     or if the raw input doesn't contain the expected data structure
	//
	// Common error conditions include invalid format, missing required fields, type conversion
	// failures, or malformed structured data within the raw response.
	Parse(rawLLMOutput string) (T, error)
}

// stripMarkdownCodeBlock removes Markdown code block delimiters from the input string.
// It handles code blocks with various language identifiers like ```json, ```JSON, or plain ```.
//
// The function works by:
//  1. Trimming whitespace from the input
//  2. Checking for opening and closing ``` delimiters
//  3. Removing the first line (containing ```) and the trailing ```
//  4. Returning the cleaned content
func stripMarkdownCodeBlock(input string) string {
	trimmed := strings.TrimSpace(input)

	if len(trimmed) < 6 {
		return trimmed
	}

	// Check if starts with ``` and ends with ```
	if !strings.HasPrefix(trimmed, "```") ||
		!strings.HasSuffix(trimmed, "```") {
		return trimmed
	}

	// Find the first newline after ```
	newlineIdx := strings.Index(trimmed, "\n")
	if newlineIdx == -1 {
		// No newlines, treat as single line: ```content```
		return strings.TrimSpace(trimmed[3 : len(trimmed)-3])
	}

	// Multi-line case: skip first line (```json or ```), remove last ```
	content := trimmed[newlineIdx+1 : len(trimmed)-3]
	return strings.TrimSpace(content)
}

var _ StructuredParser[[]string] = (*ListParser)(nil)

// ListParser implements StructuredParser for converting LLM output into a slice of strings.
// It expects the LLM to return comma-separated values and splits them accordingly.
type ListParser struct{}

// NewListParser creates a new instance of ListParser.
func NewListParser() *ListParser {
	return &ListParser{}
}

// Instructions returns formatting instructions for the LLM to generate comma-separated values.
// The instructions emphasize simplicity and consistency in output format.
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

// Parse converts the raw LLM output string into a slice of strings by splitting
// on commas and trimming whitespace from each element.
//
// The parsing is permissive and handles various formatting variations by:
//  1. Splitting the input on comma delimiters
//  2. Trimming whitespace from each resulting element
//  3. Returning the cleaned slice of strings
func (l *ListParser) Parse(rawLLMOutput string) ([]string, error) {
	values := strings.Split(rawLLMOutput, ",")
	for i, v := range values {
		values[i] = strings.TrimSpace(v)
	}
	return values, nil
}

var _ StructuredParser[map[string]any] = (*MapParser)(nil)

// MapParser implements StructuredParser for converting LLM output into a map[string]any
// by parsing JSON format. It expects the LLM to return valid JSON objects.
type MapParser struct{}

// NewMapParser creates a new instance of MapParser.
func NewMapParser() *MapParser {
	return &MapParser{}
}

// Instructions returns formatting instructions for the LLM to generate JSON objects
// that can be parsed into Go's map[string]interface{} structure.
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

// Parse converts the raw LLM output string into a map[string]any by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present before parsing.
//
// The method handles common LLM formatting issues by:
//  1. Stripping Markdown code block delimiters
//  2. Unmarshaling the JSON into a map[string]any
//  3. Returning detailed error information if parsing fails
func (m *MapParser) Parse(rawLLMOutput string) (map[string]any, error) {
	clean := stripMarkdownCodeBlock(rawLLMOutput)
	result := make(map[string]any)
	err := json.Unmarshal([]byte(clean), &result)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("failed to parse JSON content: %s (original input: %s)", clean, rawLLMOutput))
	}
	return result, nil
}

var _ StructuredParser[any] = (*JSONParser[any])(nil)

// JSONParser is a generic StructuredParser implementation that converts LLM output
// into a structured type T by parsing JSON format with automatic JSON Schema generation.
//
// The parser leverages Go's type system to automatically generate appropriate JSON Schema
// instructions for the LLM, ensuring the output matches the expected structure.
type JSONParser[T any] struct {
	cachedInstructions string // Cached format instructions
}

// NewJSONParser creates a new instance of JSONParser for type T.
func NewJSONParser[T any]() *JSONParser[T] {
	j := &JSONParser[T]{}
	j.cachedInstructions = j.generateInstructions()
	return j
}

// generateInstructions generates format instructions with JSON Schema for the LLM output.
// The schema is automatically derived from the generic type T using reflection.
func (j *JSONParser[T]) generateInstructions() string {
	const template = `[OUTPUT FORMAT]
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
	var instance T
	return fmt.Sprintf(template, pkgjson.StringDefSchemaOf(instance))
}

// Instructions returns formatting instructions for the LLM to generate JSON data
// that conforms to the JSON Schema of type T. The format is generated once and cached
// for performance optimization.
func (j *JSONParser[T]) Instructions() string {
	return j.cachedInstructions
}

// Parse converts the raw LLM output string into type T by parsing JSON.
// It automatically strips Markdown code blocks (```json and ```) if present before parsing.
//
// The parsing process includes:
//  1. Stripping any Markdown code block formatting
//  2. Unmarshaling the JSON into the target type T
//  3. Providing detailed error information including both processed content and raw input
func (j *JSONParser[T]) Parse(rawLLMOutput string) (T, error) {
	clean := stripMarkdownCodeBlock(rawLLMOutput)
	var result T
	err := json.Unmarshal([]byte(clean), &result)
	if err != nil {
		return result, errors.Join(err, fmt.Errorf("failed to parse JSON content to type %T: %s (original input: %s)", result, clean, rawLLMOutput))
	}
	return result, nil
}

var _ StructuredParser[any] = (*AnyParser)(nil)

// AnyParser adapts any StructuredParser[T] to StructuredParser[any] by wrapping
// the original parser and performing type erasure. This enables uniform handling
// of different parser types through a common interface.
type AnyParser struct {
	FormatInstructions string
	ParseFunction      func(rawLLMOutput string) (any, error)
}

// Instructions returns the format instructions from the wrapped parser.
func (parser *AnyParser) Instructions() string {
	return parser.FormatInstructions
}

// Parse delegates to the wrapped parse function and returns the result as any.
// Returns an error if the parse function is not properly initialized.
func (parser *AnyParser) Parse(rawLLMOutput string) (any, error) {
	if parser.ParseFunction == nil {
		return nil, errors.New("parse function cannot be nil")
	}
	return parser.ParseFunction(rawLLMOutput)
}

// ParserAsAny wraps any StructuredParser[T] and converts it to StructuredParser[any].
// The adapter preserves format instructions and parsing behavior while performing
// type erasure to enable uniform handling of different parser types.
//
// This is useful when you need to store different parser types in a common collection
// or pass them through interfaces that expect StructuredParser[any].
func ParserAsAny[T any](original StructuredParser[T]) *AnyParser {
	return &AnyParser{
		FormatInstructions: original.Instructions(),
		ParseFunction: func(rawLLMOutput string) (any, error) {
			result, err := original.Parse(rawLLMOutput)
			return result, err
		},
	}
}

// ListParserAsAny creates a StructuredParser[any] that parses comma-separated values.
// This is a convenience function equivalent to ParserAsAny(NewListParser()).
func ListParserAsAny() *AnyParser {
	return ParserAsAny(NewListParser())
}

// MapParserAsAny creates a StructuredParser[any] that parses JSON objects into maps.
// This is a convenience function equivalent to ParserAsAny(NewMapParser()).
func MapParserAsAny() *AnyParser {
	return ParserAsAny(NewMapParser())
}

// JSONParserAsAnyOf creates a StructuredParser[any] that parses JSON into type T
// and returns it as any. This is a convenience function equivalent to
// ParserAsAny(NewJSONParser[T]()).
func JSONParserAsAnyOf[T any]() *AnyParser {
	return ParserAsAny(NewJSONParser[T]())
}
