package zhipu

const (
	Provider = "Zhipu"
)

const (
	// BaseURL is the BigModel v4 OpenAI-compatible endpoint.
	BaseURL = "https://open.bigmodel.cn/api/paas/v4"

	// BaseURLAnthropic is the Anthropic-compatible endpoint Zhipu
	// exposes for GLM-4.5 / GLM-4.6. The anthropic-sdk-go client
	// appends "v1/messages" so the full URL resolves to
	// https://open.bigmodel.cn/api/anthropic/v1/messages.
	BaseURLAnthropic = "https://open.bigmodel.cn/api/anthropic"
)

// Chat model ids. See https://docs.bigmodel.cn/api-reference/模型/模型概述.
const (
	// ModelGLM46 (glm-4.6) is the flagship, with extended context.
	ModelGLM46 = "glm-4.6"

	// ModelGLM45 (glm-4.5) is the previous-gen flagship.
	ModelGLM45 = "glm-4.5"

	// ModelGLM45Air (glm-4.5-air) is the cheaper / faster variant of 4.5.
	ModelGLM45Air = "glm-4.5-air"

	// ModelGLM4Plus (glm-4-plus) is the GLM-4 flagship.
	ModelGLM4Plus = "glm-4-plus"

	// ModelGLM4Air (glm-4-air) is the GLM-4 mid-tier.
	ModelGLM4Air = "glm-4-air"

	// ModelGLM4Flash (glm-4-flash) is the GLM-4 free / fastest tier.
	ModelGLM4Flash = "glm-4-flash"

	// ModelGLM4V (glm-4v-plus) is the multimodal vision-language model.
	ModelGLM4V = "glm-4v-plus"
)

// Embedding model ids.
const (
	// ModelEmbedding3 produces 2048-dim vectors by default; the
	// output_dimension parameter (passed through embedding.Options.Dimensions)
	// can truncate down to 256 / 512 / 1024.
	ModelEmbedding3 = "embedding-3"

	// ModelEmbedding2 is the legacy 1024-dim model. embedding-3 is
	// recommended for new builds.
	ModelEmbedding2 = "embedding-2"
)
