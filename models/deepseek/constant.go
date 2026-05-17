package deepseek

const (
	Provider = "DeepSeek"
)

const (
	// BaseURL is DeepSeek's production endpoint. Equivalent to
	// "https://api.deepseek.com/v1" — DeepSeek accepts both.
	BaseURL = "https://api.deepseek.com"

	// BaseURLBeta is the beta endpoint that supports prefix completion
	// (FIM) and other experimental features.
	BaseURLBeta = "https://api.deepseek.com/beta"
)

// Production model ids. See
// https://api-docs.deepseek.com/quick_start/pricing for the current
// pricing and context-window limits.
const (
	// ModelChat (deepseek-chat) is the general-purpose model, currently
	// backed by DeepSeek-V3.2.
	ModelChat = "deepseek-chat"

	// ModelReasoner (deepseek-reasoner) returns visible chain-of-thought
	// in the reasoning_content field. The thinking is automatically
	// surfaced via [chat.AssistantMessage.Reasoning].
	ModelReasoner = "deepseek-reasoner"
)
