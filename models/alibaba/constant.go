package alibaba

const (
	Provider = "Alibaba"
)

const (
	// BaseURLChina is the domestic DashScope OpenAI-compat endpoint.
	BaseURLChina = "https://dashscope.aliyuncs.com/compatible-mode/v1"

	// BaseURLIntl is the Singapore DashScope OpenAI-compat endpoint —
	// required for international (non-mainland-China) users.
	BaseURLIntl = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
)

// Chat model ids. The Qwen family is versioned aggressively; the
// constants below pin to common production picks. See
// https://help.aliyun.com/zh/model-studio/getting-started/models for
// the live catalog.
const (
	// ModelQwen3Max (qwen3-max) is the current flagship.
	ModelQwen3Max = "qwen3-max"

	// ModelQwenMax (qwen-max) is the previous-gen flagship.
	ModelQwenMax = "qwen-max"

	// ModelQwenPlus (qwen-plus) is the mid-tier general-purpose model.
	ModelQwenPlus = "qwen-plus"

	// ModelQwenTurbo (qwen-turbo) is the cheap / fast tier.
	ModelQwenTurbo = "qwen-turbo"

	// ModelQwenLong (qwen-long) targets long-context workloads.
	ModelQwenLong = "qwen-long"

	// ModelQwQ32B (qwq-32b) is the reasoning model — returns
	// chain-of-thought via reasoning_content (auto-surfaced as
	// chat.AssistantMessage.Reasoning).
	ModelQwQ32B = "qwq-32b"

	// ModelQwen3CoderPlus (qwen3-coder-plus) targets code generation
	// / completion / repair.
	ModelQwen3CoderPlus = "qwen3-coder-plus"

	// ModelQwenCoderPlus (qwen-coder-plus) is the previous-gen code
	// model.
	ModelQwenCoderPlus = "qwen-coder-plus"

	// ModelQwenVLMax (qwen-vl-max) is the multimodal vision-language
	// flagship.
	ModelQwenVLMax = "qwen-vl-max"

	// ModelQwenVLPlus (qwen-vl-plus) is the mid-tier vision-language
	// model.
	ModelQwenVLPlus = "qwen-vl-plus"

	// ModelQwenOmniTurbo (qwen-omni-turbo) handles text + image +
	// audio + video input with streaming text/audio output.
	ModelQwenOmniTurbo = "qwen-omni-turbo"
)

// Embedding model ids.
const (
	// ModelEmbeddingV4 (text-embedding-v4) is the current general-purpose
	// embedding model.
	ModelEmbeddingV4 = "text-embedding-v4"

	// ModelEmbeddingV3 (text-embedding-v3) is the previous-gen model.
	ModelEmbeddingV3 = "text-embedding-v3"
)
