package response

// Usage encapsulates metadata on the usage of an AI provider's API per AI request.
type Usage struct {
	promptTokens     int
	completionTokens int
	nativeUsage      interface{}
}

// PromptTokens returns the number of tokens used in the prompt of the AI request.
func (u *Usage) PromptTokens() int {
	return u.promptTokens
}

// CompletionTokens returns the number of tokens returned in the generation (aka completion)
// of the AI's response.
func (u *Usage) CompletionTokens() int {
	return u.completionTokens
}

// TotalTokens returns the total number of tokens from both the prompt of an AI request
// and generation of the AI's response.
func (u *Usage) TotalTokens() int {
	return u.promptTokens + u.completionTokens
}

// NativeUsage returns the usage data from the underlying model API response.
func (u *Usage) NativeUsage() interface{} {
	return u.nativeUsage
}

// UsageBuilder provides a builder pattern for creating Usage instances.
type UsageBuilder struct {
	promptTokens     int
	completionTokens int
	nativeUsage      interface{}
}

// NewUsageBuilder creates a new UsageBuilder instance.
func NewUsageBuilder() *UsageBuilder {
	return &UsageBuilder{}
}

// WithPromptTokens sets the number of prompt tokens.
func (u *UsageBuilder) WithPromptTokens(promptTokens int) *UsageBuilder {
	u.promptTokens = promptTokens
	return u
}

// WithCompletionTokens sets the number of completion tokens.
func (u *UsageBuilder) WithCompletionTokens(completionTokens int) *UsageBuilder {
	u.completionTokens = completionTokens
	return u
}

// WithNativeUsage sets the native usage data.
func (u *UsageBuilder) WithNativeUsage(nativeUsage interface{}) *UsageBuilder {
	if nativeUsage != nil {
		u.nativeUsage = nativeUsage
	}
	return u
}

// Build creates a new Usage instance with the configured values.
func (u *UsageBuilder) Build() *Usage {
	return &Usage{
		promptTokens:     u.promptTokens,
		completionTokens: u.completionTokens,
		nativeUsage:      u.nativeUsage,
	}
}
