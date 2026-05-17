package xiaomi

const (
	Provider = "Xiaomi"
)

// OpenAI-compatible endpoint.
const (
	// BaseURL is the MiMo OpenAI-compatible endpoint.
	BaseURL = "https://api.xiaomimimo.com/v1"
)

// Anthropic-compatible endpoint. anthropic-sdk-go appends
// "v1/messages" to the supplied BaseURL so the full URL ends at
// https://api.xiaomimimo.com/anthropic/v1/messages.
const (
	// BaseURLAnthropic is the MiMo Anthropic-compatible endpoint.
	BaseURLAnthropic = "https://api.xiaomimimo.com/anthropic"
)

// Chat model ids. The MiMo lineup is versioned aggressively; consult
// https://platform.xiaomimimo.com/docs/en-US/api/chat for the live
// catalog.
const (
	// ModelV25Pro (mimo-v2.5-pro) is the current flagship — tuned
	// for Agent / Coding workloads, tops the open-source leaderboard.
	// Supports the thinking mode for visible chain-of-thought.
	ModelV25Pro = "mimo-v2.5-pro"

	// ModelV25 (mimo-v2.5) is the V2.5 multimodal model with 1M-token
	// context.
	ModelV25 = "mimo-v2.5"

	// ModelV2Pro (mimo-v2-pro) is the V2-series 1T flagship; precedes
	// V2.5-pro.
	ModelV2Pro = "mimo-v2-pro"

	// ModelV2Flash (mimo-v2-flash) is the 309B-param MoE open-source
	// model — cheap and fast.
	ModelV2Flash = "mimo-v2-flash"

	// ModelV2Omni (mimo-v2-omni) handles text + image + audio + video
	// input.
	ModelV2Omni = "mimo-v2-omni"
)
