package metadata

// Usage interface defines methods for tracking token usage in a model operation.
type Usage interface {
	// PromptTokens returns the number of tokens used in the input prompt.
	PromptTokens() int64

	// CompletionTokens returns the number of tokens generated in the completion.
	CompletionTokens() int64

	// TotalTokens returns the total number of tokens used, including both prompt and completion tokens.
	TotalTokens() int64
}
