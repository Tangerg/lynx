package openaicompat

const (
	// Chinese providers (OpenAI-compatible mode).

	// BaseURLVolcanoArk targets ByteDance's Volcano Ark platform
	// (火山方舟, hosts DouBao and several third-party models). Model
	// ids on Volcano are "endpoint ids" (ep-2024xxxx-xxxx) rather
	// than bare model names — see the console for the id of your
	// deployed model.
	// Docs: https://www.volcengine.com/docs/82379
	BaseURLVolcanoArk = "https://ark.cn-beijing.volces.com/api/v3"

	// BaseURLQianfan targets Baidu's Qianfan platform v2 (千帆, hosts
	// Wenxin / ERNIE plus third-party models). Qianfan v1's bespoke
	// access-token flow is deprecated; v2 uses IAM API keys with a
	// standard Bearer header.
	// Docs: https://cloud.baidu.com/doc/qianfan-api/
	BaseURLQianfan = "https://qianfan.baidubce.com/v2"

	// BaseURLBaichuan targets Baichuan AI (the company name and model
	// family share a brand).
	// Docs: https://platform.baichuan-ai.com/docs/api
	BaseURLBaichuan = "https://api.baichuan-ai.com/v1"

	// BaseURLLingyiwanwu targets 01.AI / Lingyiwanwu (零一万物, hosts
	// the Yi series).
	// Docs: https://platform.lingyiwanwu.com/docs
	BaseURLLingyiwanwu = "https://api.lingyiwanwu.com/v1"

	// BaseURLStepFun targets StepFun (阶跃星辰).
	// Docs: https://platform.stepfun.com/docs
	BaseURLStepFun = "https://api.stepfun.com/v1"

	// BaseURLHunYuan targets Tencent HunYuan (混元, the official
	// platform brand — the same name is shared with the model family).
	// Docs: https://cloud.tencent.com/document/product/1729
	BaseURLHunYuan = "https://api.hunyuan.cloud.tencent.com/v1"

	// BaseURLSenseNova targets SenseTime's SenseNova platform.
	// Docs: https://platform.sensenova.cn/doc
	BaseURLSenseNova = "https://api.sensenova.cn/compatible-mode/v1"

	// BaseURLSpark targets iFlytek's Spark platform (the official
	// platform brand — shared with the model family).
	// Docs: https://www.xfyun.cn/doc/spark/
	BaseURLSpark = "https://spark-api-open.xf-yun.com/v1"

	// BaseURLSiliconFlow targets SiliconFlow's inference platform.
	// Docs: https://docs.siliconflow.cn/
	BaseURLSiliconFlow = "https://api.siliconflow.cn/v1"
)

const (
	// Western OpenAI-compatible inference platforms.

	// BaseURLGroq targets Groq's LPU inference platform.
	// Docs: https://console.groq.com/docs/openai
	BaseURLGroq = "https://api.groq.com/openai/v1"

	// BaseURLTogether targets Together AI.
	// Docs: https://docs.together.ai/reference
	BaseURLTogether = "https://api.together.xyz/v1"

	// BaseURLFireworks targets Fireworks AI.
	// Docs: https://docs.fireworks.ai/api-reference/introduction
	BaseURLFireworks = "https://api.fireworks.ai/inference/v1"

	// BaseURLDeepInfra targets DeepInfra's OpenAI-compatible path.
	// Docs: https://deepinfra.com/docs/inference
	BaseURLDeepInfra = "https://api.deepinfra.com/v1/openai"

	// BaseURLCerebras targets Cerebras' inference platform.
	// Docs: https://inference-docs.cerebras.ai/
	BaseURLCerebras = "https://api.cerebras.ai/v1"

	// BaseURLXAI targets xAI (Grok).
	// Docs: https://docs.x.ai/api
	BaseURLXAI = "https://api.x.ai/v1"

	// BaseURLPerplexity targets Perplexity's online LLMs.
	// Docs: https://docs.perplexity.ai/
	BaseURLPerplexity = "https://api.perplexity.ai"

	// BaseURLMetaLlama targets Meta's first-party Llama API
	// (OpenAI-compatible mode). Access is currently invite-based; for
	// general availability the same Llama models are reachable through
	// Groq, Together, Fireworks, Cerebras, OpenRouter, AWS Bedrock,
	// HuggingFace, or self-hosted via Ollama / vLLM — all already
	// supported by the framework.
	// Docs: https://llama.developer.meta.com/docs
	BaseURLMetaLlama = "https://api.llama.com/compat/v1"

	// BaseURLGitHubModels targets GitHub Models — Microsoft's free
	// catalog of foundation models (OpenAI GPT-4 / 4o / 5, Llama,
	// Mistral, Phi, Cohere Command, AI21 Jamba, ...) reachable with
	// a GitHub personal access token. Useful for getting started
	// without a paid account.
	// Docs: https://docs.github.com/github-models/use-github-models/
	BaseURLGitHubModels = "https://models.github.ai/inference"

	// BaseURLNVIDIA targets NVIDIA's hosted inference catalog
	// (NIM-backed). Hosts Llama, Mistral, Mixtral, NVIDIA Nemotron,
	// DeepSeek and Qwen variants tuned for NVIDIA hardware.
	// Docs: https://docs.api.nvidia.com/nim/reference
	BaseURLNVIDIA = "https://integrate.api.nvidia.com/v1"

	// BaseURLSambaNova targets SambaNova Cloud's OpenAI-compat
	// endpoint — extremely low-latency Llama / Qwen hosted on
	// SambaNova RDU chips.
	// Docs: https://docs.sambanova.ai/cloud/api-reference/
	BaseURLSambaNova = "https://api.sambanova.ai/v1"

	// BaseURLNovita targets Novita AI, an aggregator hosting Llama,
	// Mistral, Qwen, DeepSeek and others at competitive pricing.
	// Docs: https://novita.ai/docs/api-reference/
	BaseURLNovita = "https://api.novita.ai/v3/openai"

	// BaseURLHyperbolic targets Hyperbolic's OpenAI-compat surface —
	// hosts Llama, Qwen, DeepSeek-V3, Pixtral, image / audio models.
	// Docs: https://docs.hyperbolic.xyz/docs/
	BaseURLHyperbolic = "https://api.hyperbolic.xyz/v1"

	// BaseURLNebius targets Nebius AI Studio's OpenAI-compat
	// endpoint — Llama, Qwen, Mistral, DeepSeek hosted in the EU.
	// Docs: https://docs.nebius.com/studio/inference/api
	BaseURLNebius = "https://api.studio.nebius.ai/v1"
)

// Anthropic's and Google Gemini's first-party OpenAI-compat endpoints
// live in their respective facade packages: see
// [github.com/Tangerg/lynx/models/anthropic.BaseURLOpenAI] and
// [github.com/Tangerg/lynx/models/google.BaseURLOpenAI]. Constructing
// against those bridges goes through models/anthropic.NewOpenAIChatModel
// or models/google.NewOpenAIChatModel rather than openai.NewChatModel
// with a hardcoded BaseURL.

// vLLM is self-hosted — there is no shared base URL. Configure with
// the daemon address you control:
//
//	option.WithBaseURL("http://your-vllm-host:8000/v1")
//
// vLLM exposes the same OpenAI-compatible /chat/completions and
// /embeddings paths once started with the OpenAI-compatible server.
//
// See https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html.
