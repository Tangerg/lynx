# OpenAI-compatible providers

Lynx ships dedicated facade packages for providers with **provider-
specific surface area** worth typed support. Pure base-URL switches —
where the only difference is the endpoint hostname — are documented
here instead, since `openai.NewChatModel` already covers them with a
single `option.WithBaseURL(...)` call.

**Naming convention**: package and constant names use the **provider /
platform** brand, not the model brand. Where ambiguous, the API
platform name is preferred over the company name (parallel to "Bedrock"
vs "AWS"): `moonshot` (not `kimi`), `alibaba` (not `dashscope`, not `qwen`),
`BaseURLVolcanoArk` (not `BaseURLDouBao`), `BaseURLQianfan` (not
`BaseURLWenxin`), `BaseURLLingyiwanwu` (not `BaseURLYi`).

## Facade-packaged providers (have non-OpenAI surface)

| Provider | Package | Extra surface |
|---|---|---|
| [DeepSeek](https://api-docs.deepseek.com/) | `models/deepseek` | `/beta` endpoint for prefix-FIM completion; model id constants |
| [Zhipu](https://docs.bigmodel.cn/) | `models/zhipu` | chat (OpenAI + Anthropic) + embedding-3 dimension truncation |
| [MiniMax](https://platform.minimaxi.com/document) | `models/minimax` | chat (OpenAI + Anthropic); dual billing zones |
| [Moonshot](https://platform.moonshot.cn/docs) | `models/moonshot` | chat (OpenAI + Anthropic); dual regions (CN/Intl) |
| [Alibaba (DashScope)](https://help.aliyun.com/zh/model-studio/) | `models/alibaba` | chat + embed; region routing (CN/Intl); hosts Qwen family |
| [OpenRouter](https://openrouter.ai/docs) | `models/openrouter` | chat (OpenAI + Anthropic); HTTP-Referer / X-Title attribution headers |
| [HuggingFace](https://huggingface.co/docs/inference-providers) | `models/huggingface` | router model-id convention |
| [Mistral](https://docs.mistral.ai/) | `models/mistral` | chat + embed + custom moderation API |

## Base-URL-only providers

For these, use `openai.NewChatModel` directly with the matching
`BaseURL*` constant from `models/openaicompat`.

### Chinese providers

| Provider / Platform | Constant | BaseURL | Hosts |
|---|---|---|---|
| Volcano Ark (火山方舟) | `openaicompat.BaseURLVolcanoArk` | `https://ark.cn-beijing.volces.com/api/v3` | DouBao + 3rd-party models ([docs](https://www.volcengine.com/docs/82379)) |
| Qianfan (千帆) | `openaicompat.BaseURLQianfan` | `https://qianfan.baidubce.com/v2` | Wenxin / ERNIE ([docs](https://cloud.baidu.com/doc/qianfan-api/)) |
| Baichuan AI | `openaicompat.BaseURLBaichuan` | `https://api.baichuan-ai.com/v1` | Baichuan models ([docs](https://platform.baichuan-ai.com/docs/api)) |
| Lingyiwanwu (零一万物) | `openaicompat.BaseURLLingyiwanwu` | `https://api.lingyiwanwu.com/v1` | Yi series ([docs](https://platform.lingyiwanwu.com/docs)) |
| StepFun (阶跃星辰) | `openaicompat.BaseURLStepFun` | `https://api.stepfun.com/v1` | Step series ([docs](https://platform.stepfun.com/docs)) |
| Tencent HunYuan (混元) | `openaicompat.BaseURLHunYuan` | `https://api.hunyuan.cloud.tencent.com/v1` | HunYuan family ([docs](https://cloud.tencent.com/document/product/1729)) |
| SenseTime SenseNova | `openaicompat.BaseURLSenseNova` | `https://api.sensenova.cn/compatible-mode/v1` | SenseChat ([docs](https://platform.sensenova.cn/doc)) |
| iFlytek Spark (星火) | `openaicompat.BaseURLSpark` | `https://spark-api-open.xf-yun.com/v1` | Spark family ([docs](https://www.xfyun.cn/doc/spark/)) |
| SiliconFlow (硅基流动) | `openaicompat.BaseURLSiliconFlow` | `https://api.siliconflow.cn/v1` | aggregator of OSS models ([docs](https://docs.siliconflow.cn/)) |

### Western inference platforms

| Provider | Constant | BaseURL | Docs |
|---|---|---|---|
| Groq | `openaicompat.BaseURLGroq` | `https://api.groq.com/openai/v1` | [groq.com](https://console.groq.com/docs/openai) |
| Together AI | `openaicompat.BaseURLTogether` | `https://api.together.xyz/v1` | [together.ai](https://docs.together.ai/reference) |
| Fireworks AI | `openaicompat.BaseURLFireworks` | `https://api.fireworks.ai/inference/v1` | [fireworks.ai](https://docs.fireworks.ai/api-reference/introduction) |
| DeepInfra | `openaicompat.BaseURLDeepInfra` | `https://api.deepinfra.com/v1/openai` | [deepinfra.com](https://deepinfra.com/docs/inference) |
| Cerebras | `openaicompat.BaseURLCerebras` | `https://api.cerebras.ai/v1` | [cerebras.ai](https://inference-docs.cerebras.ai/) |
| xAI (Grok) | `openaicompat.BaseURLXAI` | `https://api.x.ai/v1` | [x.ai](https://docs.x.ai/api) |
| Perplexity | `openaicompat.BaseURLPerplexity` | `https://api.perplexity.ai` | [perplexity.ai](https://docs.perplexity.ai/) |
| Meta Llama (first-party) | `openaicompat.BaseURLMetaLlama` | `https://api.llama.com/compat/v1` | [llama.developer.meta.com](https://llama.developer.meta.com/docs) |
| GitHub Models | `openaicompat.BaseURLGitHubModels` | `https://models.github.ai/inference` | [docs.github.com](https://docs.github.com/github-models/use-github-models/) |
| NVIDIA (NIM catalog) | `openaicompat.BaseURLNVIDIA` | `https://integrate.api.nvidia.com/v1` | [docs.api.nvidia.com](https://docs.api.nvidia.com/nim/reference) |
| SambaNova Cloud | `openaicompat.BaseURLSambaNova` | `https://api.sambanova.ai/v1` | [docs.sambanova.ai](https://docs.sambanova.ai/cloud/api-reference/) |
| Novita AI | `openaicompat.BaseURLNovita` | `https://api.novita.ai/v3/openai` | [novita.ai](https://novita.ai/docs/api-reference/) |
| Hyperbolic | `openaicompat.BaseURLHyperbolic` | `https://api.hyperbolic.xyz/v1` | [docs.hyperbolic.xyz](https://docs.hyperbolic.xyz/docs/) |
| Nebius AI Studio | `openaicompat.BaseURLNebius` | `https://api.studio.nebius.ai/v1` | [docs.nebius.com](https://docs.nebius.com/studio/inference/api) |

### Self-hosted

| Provider | BaseURL |
|---|---|
| vLLM | Caller-supplied (`http://your-host:8000/v1`); start vLLM with `--api-key` and the OpenAI-compatible server flag |

## Example

```go
import (
    "github.com/Tangerg/lynx/core/model"
    "github.com/Tangerg/lynx/core/model/chat"
    "github.com/Tangerg/lynx/models/openai"
    "github.com/Tangerg/lynx/models/openaicompat"
    "github.com/openai/openai-go/v3/option"
)

opts, _ := chat.NewOptions("doubao-pro-256k")
m, err := openai.NewChatModel(&openai.ChatModelConfig{
    ApiKey:         model.NewApiKey("..."),
    DefaultOptions: opts,
    RequestOptions: []option.RequestOption{
        option.WithBaseURL(openaicompat.BaseURLVolcanoArk),
    },
})
```

## Why no dedicated facade?

A dedicated `models/<provider>/` package earns its place when it adds
non-trivial typed surface beyond a base URL switch — model id
constants, region routing, special endpoint paths, attribution headers,
or distinct request/response fields. For providers that only change
the hostname, a 50-line facade would be dead weight: the `openai`
package already handles the wire correctly and a base-URL constant is
all the discoverability lynx needs to publish.

If you find yourself wanting typed surface for one of the base-URL-only
providers above (e.g. typed model id constants, custom auth header
shapes), open an issue and we'll promote it to a facade package.
