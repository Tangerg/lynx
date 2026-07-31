package huggingface

const (
	Provider = "HuggingFace"
)

const (
	// DefaultBaseURL targets the HuggingFace router which proxies to a
	// curated set of inference providers (together, fireworks, nebius,
	// sambanova, hf-inference, ...). The router exposes an
	// OpenAI-compatible /chat/completions endpoint.
	DefaultBaseURL = "https://router.huggingface.co/v1"
)

const ModelGPTOSS120B = "openai/gpt-oss-120b"
