package minimax

const (
	Provider = "MiniMax"
)

// OpenAI-compatible endpoints.
const (
	// BaseURLIntl is the international OpenAI-compat endpoint
	// (USD billing).
	BaseURLIntl = "https://api.minimaxi.com/v1"

	// BaseURLChina is the domestic OpenAI-compat endpoint (RMB billing).
	BaseURLChina = "https://api.minimax.io/v1"
)

// Anthropic-compatible endpoints. The anthropic-sdk-go client appends
// "v1/messages" to the supplied BaseURL so the full request URL ends
// at, e.g., https://api.minimaxi.com/anthropic/v1/messages.
const (
	// BaseURLIntlAnthropic is the international Anthropic-compat
	// endpoint (USD billing).
	BaseURLIntlAnthropic = "https://api.minimaxi.com/anthropic"

	// BaseURLChinaAnthropic is the domestic Anthropic-compat endpoint
	// (RMB billing).
	BaseURLChinaAnthropic = "https://api.minimax.io/anthropic"
)

// Current Chat model ids. See
// https://platform.minimaxi.com/docs/guides/models-intro.
const (
	ModelM3           = "MiniMax-M3"
	ModelM27          = "MiniMax-M2.7"
	ModelM27HighSpeed = "MiniMax-M2.7-highspeed"
	ModelM25          = "MiniMax-M2.5"
	ModelM25HighSpeed = "MiniMax-M2.5-highspeed"
	ModelM21          = "MiniMax-M2.1"
	ModelM21HighSpeed = "MiniMax-M2.1-highspeed"
	ModelM2           = "MiniMax-M2"
)
