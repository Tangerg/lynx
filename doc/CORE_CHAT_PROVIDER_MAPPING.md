# Core Chat 四 Provider 映射契约

> 状态：当前 conformance 契约（最初建立于 P1-07）
> 日期：2026-08-25
> 范围：OpenAI、Anthropic、Google Gemini、Ollama Native Chat

本文锁定当前 `core/chat` 协议对四个差异 provider 的表达能力。可执行契约由各 provider 自己的协议测试拥有；不存在跨 provider 的第二套实现或集中式 conformance 包。

Core Chat 的公开模型只表达一次生成：`Request.Options` 承载通用参数和 provider 扩展，`Response.Output` 承载唯一模型输出，`Response.Metadata` 承载响应身份、用量和响应级扩展。provider 原生但仍需保留的 JSON-safe 数据进入对应 metadata 的 namespaced `Extra`，不提升为 Core 公共字段。

## 1. 统一映射规则

1. Request/Response 在进入 SDK 或离开 SDK 时必须先通过递归 `Validate`。
2. provider 不支持的 role、Part 或 Media source 必须返回带 provider 名称和字段位置的错误，不再静默跳过。
3. Message.Parts 的顺序是语义；原生协议保序时逐 Part 映射，不保序时采用 provider 文档化的规范顺序，并在测试中冻结。
4. Core 只接受一个结果。provider 同步响应含多个候选，或单个流式 chunk 含多个候选时，adapter 必须在协议边界明确报错，不得截断或静默丢弃。
5. provider 原生 finish reason 归一化到 Core `FinishReason`，原值同时写入 `Result.Metadata.Extra` 的 `<provider>/native_*_reason`。
6. ToolCall.Arguments 保留 provider 原始 JSON 文本。请求侧 SDK 如果要求 JSON object，adapter 在该边界解析并报告错误；响应侧允许模型产生的 malformed/partial JSON 继续序列化。
7. 流式 tool-call delta 必须由 provider accumulator 补齐稳定 ID/Name 后再产生合法 Core Part，或缓冲到信息完整；不能把缺字段的临时对象泄露进协议。
8. 原生协议没有 tool-call ID 时，adapter 使用 `<provider>/<part-index>` 生成确定性 ID，并在同一轮 ToolResult 中沿用。不得使用全局计数器。
9. 通用 token 数进入 `Response.Metadata.Usage`；provider 原始 usage 和未提升的计数进入 `Response.Metadata.Extra` 的 `<provider>/usage`。
10. provider 特有请求参数只进入 `Request.Options.Extensions` 的 `<provider>/request`，adapter 只读取自己的 namespace。
11. 结果表示统一由 `Request.Options.OutputFormat` 表达。adapter 必须优先使用 provider 原生参数；只有原生协议无法表达该格式时才注入等价 system instruction。对应的 provider extension 字段由 Core 字段独占，不允许同时设置两套来源。

## 2. 能力矩阵

| 能力 | OpenAI | Anthropic | Google | Ollama |
|---|---|---|---|---|
| 生成结果 | 原生结果必须唯一 | 原生 Message → Result | 原生候选必须唯一 | 原生 Message → Result |
| 推理内容 | `reasoning_content` → Reasoning Part | thinking + signature；redacted block → Message Metadata | Thought + ThoughtSignature → Reasoning Part | thinking → Reasoning Part |
| 多模态输入 | image/audio/file；bytes/URI/reference 按 SDK 能力 | image/PDF；bytes/URI | inline bytes/URI | image bytes |
| 多模态输出 | audio → Media Part；transcript → Text Part | 当前无 | 当前 chat 输出按已支持 Part 映射 | 当前无 |
| ToolCall ID | 原生 ID | 原生 ID | 原生 ID 缺失时确定性合成 | 原生 ID 缺失时确定性合成 |
| ToolResult error | 原生无独立标志，结果文本保留 | `is_error` ↔ ToolResult.IsError | response object；错误语义保留在对象/结果 | 结果文本保留 |
| 缓存 usage | cached prompt tokens → CacheRead | cache read/create → CacheRead/CacheWrite | cached content → CacheRead | 当前无 |
| reasoning usage | completion details → Reasoning | 如 provider 单独报告则映射 | thoughts token count → Reasoning | 当前无 |
| OutputFormat 原生映射 | Chat `response_format`；Responses `text.format` | `output_config.format`（JSON Schema）；JSON prompt fallback | `response_mime_type` + `response_json_schema` | `format` |
| 原生请求逃生舱 | modalities、audio 等 | thinking、cache control 等 | safety、response modalities 等 | keep_alive、think、options 等 |

## 3. Provider 细则

### 3.1 OpenAI

- Chat Completions 的 content、reasoning、tool_calls 原生不保留交错顺序，Core 规范顺序为 reasoning → text → tool calls → output media。
- refusal、annotations 和可重放 audio identity 属于 Message Metadata；logprobs 属于 `Result.Metadata.Extra`。
- created、service tier 与原始 usage 属于 `Response.Metadata.Extra`。
- image bytes 映射为 data URL，URI 保持 URI；file reference 映射为 provider file ID；不兼容组合返回错误。
- `OutputFormat` 在 Chat Completions 映射为 `response_format`，在 Responses API 映射为 `text.format`；extension 不得重复设置 `response_format`。

### 3.2 Anthropic

- System Message 在 provider request 层合并，但 Core conversation 保留原消息边界。
- content blocks 原生保序；thinking 的 signature 必须逐 Part 保存并在续轮原样回放。
- redacted thinking 没有可见文本，保存到 `anthropic/redacted_reasoning` Message Metadata。
- prompt cache breakpoint、extended thinking 等原生参数由 `anthropic/request` 承载；原生 `input_tokens + cache_read_input_tokens + cache_creation_input_tokens` 归一化为 Core 总 `InputTokens`，两个 cache 分量分别映射到 Usage 可选 breakdown，原始计数保留在 `anthropic/usage`。
- JSON Schema `OutputFormat` 映射到 `output_config.format`；普通 JSON 或不支持该字段的 Anthropic-compatible endpoint 使用统一 prompt fallback。extension 可继续承载 output config 的其他字段，但不得重复设置 `format`。

### 3.3 Google Gemini

- Content.Parts 原生保序；Thought/ThoughtSignature 直接映射 Reasoning Part。
- 原生候选必须唯一且 index 为 0；多个候选明确报错。native finish reason 与 safety ratings 进入结果 metadata。
- FunctionCall.ID 缺失时按统一规则生成稳定 ID；FunctionResponse 仍按 provider 要求以 name/object 下沉。
- 原生 `prompt_token_count + tool_use_prompt_token_count` 归一化为 Core 总 `InputTokens`，`candidates_token_count + thoughts_token_count` 归一化为总 `OutputTokens`；cache/thoughts 作为 breakdown，原始分项进入 `google/usage`。
- model version 和 tool-use prompt token 另行进入 `Response.Metadata.Extra`。
- `OutputFormat` 映射到 `response_mime_type` 与 `response_json_schema`；extension 不得重复设置这些字段。

### 3.4 Ollama Native Chat

- content、thinking、tool_calls 是分离字段，Core 规范顺序为 reasoning → text → tool calls。
- Native API 没有稳定 ID 时使用确定性合成 ID。
- keep_alive、think 和额外 options 保留在 `ollama/request`；`format` 由 Core `OutputFormat` 独占。
- created_at、各阶段 duration 和原始 metrics 保留在 `Response.Metadata.Extra`。

## 4. 可执行证据与后续使用

- `models/protocol/openai/chat_protocol_behavior_test.go` 与 `chat_protocol_test.go`：OpenAI-compatible wire 的双向映射和行为契约。
- `models/protocol/anthropic/chat_protocol_behavior_test.go` 与 `chat_protocol_conformance_test.go`：Anthropic 映射、reasoning/tool/usage 行为契约。
- `models/google/internal/protocol/chat_protocol_behavior_test.go` 与 `chat_protocol_conformance_test.go`：Google 映射与 provider 特有能力契约。
- `models/ollama/chat_protocol_behavior_test.go` 与 `chat_protocol_conformance_test.go`：Ollama native chat 映射与行为契约。

四家 adapter 的真实 SDK fixture 必须产出与本契约等价的 Core 值；可以调整 provider 私有 helper，但如需改变 Core wire 或上述 loss policy，必须先更新本文、对应 provider 行为测试、API diff 与 release notes。
