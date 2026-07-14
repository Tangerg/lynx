# Core Chat 四 Provider 映射基线

> 状态：P1-07 映射验证基线
> 日期：2026-07-14
> 范围：OpenAI、Anthropic、Google Gemini、Ollama Native Chat

本文冻结新 `core/chat` 协议对四个差异 provider 的表达能力，作为 P2-07 实现新 Model/Streamer adapter 的可执行预期。当前阶段不切换生产 provider；`models/internal/chatconformance/testdata` 保存的是 adapter 迁移后必须产出的 Core wire，而不是第二套 provider 实现。

“无损”限定为：Lynx 当前公开并实际映射的能力不得丢失，同时修复旧 Response 只保留首个 choice/candidate 的缺陷；不承诺把 SDK 的每个实验字段提升为 Core 公共字段。provider 原生但仍需保留的 JSON-safe 数据进入唯一的 namespaced Extensions。

## 1. 统一映射规则

1. Request/Response 在进入 SDK 或离开 SDK 时必须先通过递归 `Validate`。
2. provider 不支持的 role、Part 或 Media source 必须返回带 provider 名称和字段位置的错误，不再静默跳过。
3. Message.Parts 的顺序是语义；原生协议保序时逐 Part 映射，不保序时采用 provider 文档化的规范顺序，并在测试中冻结。
4. 所有 choice/candidate 均保留 `Index`；不得再取 `[0]` 后静默丢弃其余结果。
5. provider 原生 finish reason 归一化到 Core `FinishReason`，原值同时写入 `<provider>/native_*_reason` Choice Extension。
6. ToolCall.Arguments 保留 provider 原始 JSON 文本。请求侧 SDK 如果要求 JSON object，adapter 在该边界解析并报告错误；响应侧允许模型产生的 malformed/partial JSON 继续序列化。
7. 流式 tool-call delta 必须由 provider accumulator 补齐稳定 ID/Name 后再产生合法 Core Part，或缓冲到信息完整；不能把缺字段的临时对象泄露进协议。
8. 原生协议没有 tool-call ID 时，adapter 使用 `<provider>/<choice-index>/<part-index>` 生成确定性 ID，并在同一轮 ToolResult 中沿用。不得使用全局计数器。
9. 通用 token 数进入 `Usage`；provider 原始 usage 和未提升的计数进入 `<provider>/usage` Response Extension。
10. provider 特有请求参数只进入 `<provider>/request`，adapter 只读取自己的 namespace。

## 2. 能力矩阵

| 能力 | OpenAI | Anthropic | Google | Ollama |
|---|---|---|---|---|
| 多结果 | 保留全部 Choices | 单 Message → Choice 0 | 保留全部 Candidates | 单 Message → Choice 0 |
| 推理内容 | `reasoning_content` → Reasoning Part | thinking + signature；redacted block → Message Metadata | Thought + ThoughtSignature → Reasoning Part | thinking → Reasoning Part |
| 多模态输入 | image/audio/file；bytes/URI/reference 按 SDK 能力 | image/PDF；bytes/URI | inline bytes/URI | image bytes |
| 多模态输出 | audio → Media Part；transcript → Text Part | 当前无 | 当前 chat 输出按已支持 Part 映射 | 当前无 |
| ToolCall ID | 原生 ID | 原生 ID | 原生 ID 缺失时确定性合成 | 原生 ID 缺失时确定性合成 |
| ToolResult error | 原生无独立标志，结果文本保留 | `is_error` ↔ ToolResult.IsError | response object；错误语义保留在对象/结果 | 结果文本保留 |
| 缓存 usage | cached prompt tokens → CacheRead | cache read/create → CacheRead/CacheWrite | cached content → CacheRead | 当前无 |
| reasoning usage | completion details → Reasoning | 如 provider 单独报告则映射 | thoughts token count → Reasoning | 当前无 |
| 原生请求逃生舱 | response format、modalities、audio 等 | thinking、cache control 等 | safety、response modalities 等 | keep_alive、format、think、options 等 |

## 3. Provider 细则

### 3.1 OpenAI

- Chat Completions 的 content、reasoning、tool_calls 原生不保留交错顺序，Core 规范顺序为 reasoning → text → tool calls → output media。
- refusal、annotations 和可重放 audio identity 属于 Message Metadata；logprobs 属于 Choice Extensions。
- created、service tier 与原始 usage 属于 Response Extensions。
- image bytes 映射为 data URL，URI 保持 URI；file reference 映射为 provider file ID；不兼容组合返回错误。

### 3.2 Anthropic

- System Message 在 provider request 层合并，但 Core conversation 保留原消息边界。
- content blocks 原生保序；thinking 的 signature 必须逐 Part 保存并在续轮原样回放。
- redacted thinking 没有可见文本，保存到 `anthropic/redacted_reasoning` Message Metadata。
- prompt cache breakpoint、extended thinking 等原生参数由 `anthropic/request` 承载；cache read/create token 分别映射到 Usage 两个可选字段。

### 3.3 Google Gemini

- Content.Parts 原生保序；Thought/ThoughtSignature 直接映射 Reasoning Part。
- Candidates 全量映射为 Choices，并保留 Candidate.Index、native finish reason 与 safety ratings。
- FunctionCall.ID 缺失时按统一规则生成稳定 ID；FunctionResponse 仍按 provider 要求以 name/object 下沉。
- model version、tool-use prompt token 和原始 usage 进入 Response Extensions。

### 3.4 Ollama Native Chat

- content、thinking、tool_calls 是分离字段，Core 规范顺序为 reasoning → text → tool calls。
- Native API 没有稳定 ID 时使用确定性合成 ID。
- keep_alive、format、think 和额外 options 保留在 `ollama/request`。
- created_at、各阶段 duration 和原始 metrics 保留在 Response Extensions。

## 4. 可执行证据与后续使用

- `models/internal/chatconformance/testdata/*.request.golden.json`：四家可映射请求边界。
- `models/internal/chatconformance/testdata/*.response.golden.json`：四家期望 Core 响应，包括多 choice、reasoning、media、tool、usage 和 provider extensions。
- `models/internal/chatconformance/mapping_test.go`：递归验证、canonical JSON fixed-point 和 provider 特有能力断言。

P2-07 实现 adapter 时必须让四家真实 SDK fixture 产出与本基线等价的 Core 值；可以调整 provider 私有 helper，但如需改变 Core wire 或上述 loss policy，必须先更新本文、golden fixture 与执行计划决策记录。
