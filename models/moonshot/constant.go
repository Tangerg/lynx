package moonshot

const (
	Provider = "Moonshot"
)

// OpenAI-compatible endpoints.
const (
	// BaseURL is the domestic OpenAI-compat endpoint.
	BaseURL = "https://api.moonshot.cn/v1"

	// BaseURLIntl is the international OpenAI-compat endpoint.
	BaseURLIntl = "https://api.moonshot.ai/v1"
)

// Anthropic-compatible endpoints. anthropic-sdk-go appends "v1/messages"
// to the configured base.
const (
	// BaseURLAnthropic is the domestic Anthropic-compat endpoint.
	BaseURLAnthropic = "https://api.moonshot.cn/anthropic"

	// BaseURLIntlAnthropic is the international Anthropic-compat endpoint.
	BaseURLIntlAnthropic = "https://api.moonshot.ai/anthropic"
)

// Chat model ids. See https://platform.moonshot.cn/docs/intro/models.
const (
	// ModelK2 (kimi-k2-0905-preview / kimi-k2-thinking) is the flagship
	// reasoning model.
	ModelK2 = "kimi-k2-0905-preview"

	// ModelK2Thinking is the K2 variant that returns chain-of-thought
	// via reasoning_content (auto-surfaced as a [chat.ReasoningPart]
	// in AssistantMessage.Parts).
	ModelK2Thinking = "kimi-k2-thinking"

	// ModelV1_8K / 32K / 128K are the legacy moonshot-v1 chat models.
	// Number is the context-window cap. New builds should prefer K2.
	ModelV1_8K   = "moonshot-v1-8k"
	ModelV1_32K  = "moonshot-v1-32k"
	ModelV1_128K = "moonshot-v1-128k"
)
