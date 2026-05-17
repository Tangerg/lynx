package anthropic

const (
	Provider = "Anthropic"
)

const (
	OptionsKey = "lynx:ai:model:anthropic_options"
)

// BaseURLOpenAI is Anthropic's first-party OpenAI-compatible endpoint
// (https://docs.claude.com/en/api/openai-sdk). Use it via
// [NewOpenAIChatModel] to keep an OpenAI-SDK integration while
// targeting Claude models.
const BaseURLOpenAI = "https://api.anthropic.com/v1"
