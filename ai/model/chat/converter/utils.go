package converter

import (
	"strings"
)

// stripMarkdownCodeBlock removes Markdown code block delimiters from the input string.
// It handles ```json, ```JSON, ``` or any other language identifier after ```.
func stripMarkdownCodeBlock(input string) string {
	input = strings.TrimSpace(input)

	if len(input) < 6 {
		return input
	}

	// Check if starts with ``` and ends with ```
	if !strings.HasPrefix(input, "```") ||
		!strings.HasSuffix(input, "```") {
		return input
	}

	// Find the first newline after ```
	firstNewline := strings.Index(input, "\n")
	if firstNewline == -1 {
		// No newlines, treat as single line: ```content```
		return strings.TrimSpace(input[3 : len(input)-3])
	}

	// Multi-line case: skip first line (```json or ```), remove last ```
	content := input[firstNewline+1 : len(input)-3]
	return strings.TrimSpace(content)
}
