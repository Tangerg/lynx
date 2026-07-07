package chat

import "strings"

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
