package together

// Exported identifiers keep provider-owned names and defaults out of caller literals.
const (
	Provider = "Together"

	BaseURL = "https://api.together.xyz/v1"
)

// A representative slice of Together's current serverless catalog. See
// https://docs.together.ai/docs/serverless-models for the full list.
const (
	ModelMiniMaxM27        = "MiniMaxAI/MiniMax-M2.7"
	ModelQwen37Max         = "Qwen/Qwen3.7-Max"
	ModelQwen36Plus        = "Qwen/Qwen3.6-Plus"
	ModelKimiK26           = "moonshotai/Kimi-K2.6"
	ModelGLM51             = "zai-org/GLM-5.1"
	ModelGPTOSS120B        = "openai/gpt-oss-120b"
	ModelGPTOSS20B         = "openai/gpt-oss-20b"
	ModelDeepSeekV4Pro     = "deepseek-ai/DeepSeek-V4-Pro"
	ModelRnj1Instruct      = "essentialai/rnj-1-instruct"
	ModelLlama33Instruct   = "meta-llama/Llama-3.3-70B-Instruct-Turbo"
	ModelQwen3Instruct2507 = "Qwen/Qwen3-235B-A22B-Instruct-2507-tput"
	ModelQwen35_397BA17B   = "Qwen/Qwen3.5-397B-A17B"
)
