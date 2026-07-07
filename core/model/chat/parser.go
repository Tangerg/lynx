package chat

import "strings"

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
