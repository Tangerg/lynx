# CLAUDE.md — models module

> 38 个 LLM / embedding / image / audio provider 的统一适配层。
> 项目级约定见 `../lyra/CLAUDE.md`。

---

## 一句话定位

每个 provider 一个独立子包，全部实现 `core/model/{chat,embedding,image,audio}` 的 Model 接口。**新加 provider = 复制现有结构 + 改 SDK 调用**，不修改 lynx 协议。

## 技术栈

- Go 1.23+（用 `iter.Seq2` 流迭代）
- 各家 SDK 直接依赖：
  - `anthropics/anthropic-sdk-go`（Messages API + token counting）
  - `openai/openai-go` v3（chat / embedding / image / audio 全套）
  - `googleapis/google-cloud-go/auth` + `google.golang.org/genai`（Gemini）
  - `aws/aws-sdk-go-v2`（Bedrock）/ Azure SDK（azureopenai）
  - 其他走原生 SDK 或自写 REST 客户端
- ~15k LOC / 187 文件 / 44 个子目录

## 核心架构

每 provider 一个一级目录（如 `anthropic/` / `openai/` / `google/`），固定结构：

- **`api.go`** —— `APIConfig` + API 客户端包装
- **`chat.go`** —— `ChatModelConfig` / `ChatModel` / `requestHelper`（消息格式转换）/ `responseHelper`（结果累积）
- **`embedding.go`** —— `EmbeddingModelConfig` / `EmbeddingModel`
- **`image.go` / `audio_tts.go` / `audio_transcription.go` / `audio_translation.go`** —— 各模态（按 provider 支持情况）
- 部分 provider 有兼容层：`chat_openai.go`（OpenAI 形状）+ `chat_anthropic.go`（Anthropic 形状）—— 同一 provider 双 API（moonshot / openrouter / openaicompat）

## 关键接口/类型

1. **`chat.Model`** —— `Call(ctx, req) (*Response, error)` + `Stream(ctx, req) iter.Seq2[*Response, error]`
2. **`<Provider>ChatModelConfig`** —— 固定字段：`APIKey model.APIKey` / `DefaultOptions *chat.Options` / `RequestOptions []option.RequestOption` / `Metadata *chat.ModelMetadata`（可选）
3. **`requestHelper`** —— `buildParams()` 合并默认 + 请求级 options；`buildXxxMsg()` 消息格式转换（user / assistant / tool）
4. **`responseHelper`** —— `buildResult()` / `buildMeta()` / `newChunkAccumulator()` 流式响应累积
5. **`embedding.Model`** / **`image.Model`** —— 同样的 Config + Model + helpers 结构

## 强约定

- **`Config.Validate()` 必检**：APIKey / DefaultOptions 缺失返 `errors.New("<provider>: XYZ is required")`
- **`New<Provider>ChatModel(cfg) (*ChatModel, error)`** 工厂：Validate → 创 API client → 初始化 Model struct
- **Options 合并走 `chat.MergeOptions(defaultOptions, requestOptions)`**；之后 `options.GetParams[T](opts, OptionsKey)` 取 provider 专属参数
- **Stream 用 `iter.Seq2`**：内部 accumulator 逐 SSE 事件累积；每个事件只 yield 该事件的 delta
- **工具调用统一形状**：`chat.ToolCallPart{ID, Name, Arguments(JSON string)}`，JSON parse 错误 **skip 不传染**
- **Metadata 覆盖**（facade 模式）：`Config.Metadata` 非 nil 用它，否则用包级默认 Provider 字符串
- **错误透传**：网络 / SDK error 直接 wrap 上浮；业务级错误（"max_tokens required" 等）包成有意义 message
- **Reasoning signature**：Anthropic / Google 有签名（续流必需），OpenAI o-series 无；适配层用 `[]byte` 兼容，按 provider 填空

## 关键目录 / 成熟度

| 类别 | provider | 模式 | 状态 |
|---|---|---|---|
| 原生 SDK | **anthropic** / **openai** / **google** | 自己跟 SDK | ✅ production |
| OpenAI 兼容 | azureopenai / alibaba / deepseek / xai / perplexity / groq / together | 委托给 openai + WithBaseURL | ✅ production |
| 双兼容 | moonshot / openrouter | 同时 NewOpenAIChatModel + NewAnthropicChatModel | ✅ production |
| 托管平台 | bedrock / vertexai | IAM auth（无 APIKey） | ✅ production |
| 混合 | mistral / minimax / zhipu / qwen | 部分模态 / 部分兼容 | ⚙️ stable |
| 本地 | ollama / openaicompat | 开源模型 / 通用容器 | ⚙️ stable |
| 语音 | lmnt / elevenlabs / deepgram / gladia / assemblyai / revai | TTS / 转录 | ⚙️ stable |
| Embedding 专用 | cohere / jina / nomic / voyage / huggingface | 只做 embedding | ⚙️ stable |
| 图像专用 | stability / prodia / blackforestlabs / luma / midjourney / replicate | text-to-image | ⚙️ stable |
| 实验性 | hume / xai | 协议或能力不稳定 | 🔬 experimental |

## 特殊点

- **chunkAccumulator**：每 provider 的 stream loop 用自己的 accumulator 把 SSE delta 拼成 Response chunk；上层 `chat.ResponseAccumulator` 再 stitch 完整消息
- **Prompt Caching**（Anthropic 独有）：API 返 `CacheReadInputTokens` / `CacheWriteInputTokens` → 映射到 lynx `Usage` 同名字段
- **Vision / Audio 多部分消息**：OpenAI `ChatCompletionContentPartImageParam` / `InputAudioParam` / `FileParam` 一条消息混合 text + media
- **内部 `options` 包**：`options.GetParams[T](opts, OptionsKey)` 通用提取器，避免每 provider 手动 type-assert `opts.Extra`
- **测试**：`internal/testutil` 有 SSE mock stream / embedding 数据生成 / 契约测试 fixtures

## 常用命令

```bash
go build ./...
go test ./...
go test ./openai/... -run TestStream  # 单 provider 流式
```

## 修改任何东西之前

- **加新 provider**：先看 `openai/` 当 reference（最全），复制结构，改 SDK 调用。Config / helpers / Model 三件套不要变形状
- **改 `core/model/chat` 接口**：所有 provider 都受影响；改之前先评估 38 处适配成本
- **改 chunkAccumulator 逻辑**：每 provider 一份，跑各 provider 的 stream 测试

## 强反向不变量

- ❌ **provider 间共享 helpers**：每 provider 自己写 requestHelper / responseHelper，别强行抽公共基类（不同 SDK shape 差异大于相似度）
- ❌ **加 retry layer**：SDK 自己有重试，框架不再加（见 lyra/CLAUDE.md 反向不变量）
- ❌ **加 OAuth / token refresh**：用户填 API key + 401 重填，OAuth 是 Claude Code 复杂度（同 lyra）
- ❌ **DefaultOptions 返指针**：`*Options` 会破坏 immutability 保证，必须返值（同 lyra 的 user feedback memory）
