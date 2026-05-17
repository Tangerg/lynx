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
	// The legacy api.minimax.chat host redirects here.
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

// Chat model ids. See
// https://platform.minimaxi.com/document/algorithm-concept.
const (
	// ModelM2 (MiniMax-M2) is the flagship reasoning model.
	ModelM2 = "MiniMax-M2"

	// ModelText01 (MiniMax-Text-01) is the predecessor general-purpose
	// 1M-context model.
	ModelText01 = "MiniMax-Text-01"

	// ModelAbab65SChat (abab6.5s-chat) is the legacy abab-series chat
	// model; new builds should prefer ModelM2 or ModelText01.
	ModelAbab65SChat = "abab6.5s-chat"
)
