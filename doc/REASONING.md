# Reasoning 一等公民

> Lynx 把 reasoning（链式推理 / chain-of-thought）作为 `AssistantMessage.Parts` 里的独立 [`ReasoningPart`] 类型，与 `TextPart` / `ToolCallPart` 平级、保留出现顺序。这是 Parts 模型重构后的最终态。
>
> **历史**：v0 用 `AssistantMessage.Reasoning string` 扁平字段；2026-05 的 Parts 重构后改为有序 part 列表，本文已更新到 Parts 模型。
>
> **取代**：原 `REASONING_UNIFIED_DESIGN.md`（v0 设计稿）+ `SPRING_AI_THINKING_ARCHITECTURE.md`（v0 状态）。

---

## 1. TL;DR

`AssistantMessage` 用 `Parts []OutputPart` 承载所有内容；reasoning 是其中一个 `ReasoningPart{Text, Signature}` 类型。Provider-specific 的安全脱敏 / 不可直接表达的边角数据进 `Metadata` map——**key 常量和 helper 都定义在各自的 provider 包里，core 包零 provider 知识**。命名采用业界正在收敛的 "reasoning"。

```go
msg := resp.Result().AssistantMessage
fmt.Println(msg.JoinedText())       // 拼接所有 TextPart
fmt.Println(msg.JoinedReasoning())  // 拼接所有 ReasoningPart
for r := range msg.ReasoningParts() {
    fmt.Println(r.Text, len(r.Signature))
}
```

---

## 2. 命名：`Reasoning` 而非 `Thinking`

| 维度 | `Thinking` | `Reasoning` |
|-----|-----------|------------|
| Provider 票数 | Anthropic / Google / Qwen / Ollama / Nova（5）| OpenAI / DeepSeek / Bedrock（3）|
| 行业品类名 | – | "reasoning models"（o1/o3/R1/gpt-5）|
| API 字段一致性 | – | `reasoning_effort` / `reasoning_tokens` / `reasoning_content` 几乎所有新规范 |
| 技术精确度 | 偏拟人化 | 链式推理，更准 |
| langchain4j 选择 | ✅ thinking | – |
| Spring AI 优化提案 | – | ✅ reasoning |

**决策**：`Reasoning`。industry vocabulary 已经收敛到 reasoning。langchain4j 选 thinking 是早期跟随 Anthropic 时的状态——Lynx 直接走收敛终态。

---

## 3. 数据模型

### 3.1 AssistantMessage + ReasoningPart

```go
// core/model/chat/message.go

type AssistantMessage struct {
    Parts    []OutputPart   `json:"parts"`
    Metadata map[string]any `json:"metadata,omitzero"`
}

// core/model/chat/part.go

type ReasoningPart struct {
    Text      string `json:"text"`
    Signature []byte `json:"signature,omitempty"` // continuity token (Anthropic/Google)
}
```

`ReasoningPart` 与 `TextPart` / `ToolCallPart` 实现同一 `OutputPart` 接口，保留 emission order。`MessageParams` 走 `Parts []OutputPart`。

### 3.2 便利访问器

```go
// core/model/chat/message.go

func (a *AssistantMessage) ReasoningParts() iter.Seq[*ReasoningPart]
func (a *AssistantMessage) JoinedReasoning() string
func (a *AssistantMessage) JoinedText() string
```

`JoinedReasoning` / `JoinedText` 是高频 read-only 路径的零成本封装；需要顺序 / 单个 signature 时迭代 `ReasoningParts()` 或 `Parts` 自身。

### 3.3 Usage：reasoning + cache 三态语义

```go
// core/model/usage.go

type Usage struct {
    PromptTokens          int64  `json:"prompt_tokens"`
    CompletionTokens      int64  `json:"completion_tokens"`
    ReasoningTokens       *int64 `json:"reasoning_tokens,omitempty"`
    CacheReadInputTokens  *int64 `json:"cache_read_input_tokens,omitempty"`
    CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`
    OriginalUsage         any    `json:"original_usage,omitempty"`
}
```

三个 `*int64` 字段都遵循同一约定：
- `nil` = provider 未透出该维度
- `0` = 显式表示该次调用没发生（如本次无缓存命中）
- `> 0` = 实际值

Adapter 在 `> 0` 才赋值，避免 SDK 在字段缺失时把零值误传。

**`ReasoningTokens` 不重复计入 `TotalTokens()`**——OpenAI 协议里它是 `completion_tokens` 的子集（`completion_tokens_details.reasoning_tokens`），`CompletionTokens` 已经包含。

---

## 4. Provider-specific Metadata

### 4.1 关键设计原则：core 零 provider 知识

`core/model/chat` 包对 provider 一无所知。signature / 脱敏数据这类 provider-specific 概念的 key 常量和读写 helper 都定义在各自 provider 包里。chat 包只规定**字符串前缀约定**：

```
lynx:chat:<provider>:<concept>
```

**验收标准**：`grep -r "anthropic\|openai\|google" core/model/` 零命中。

### 4.2 Anthropic（`models/anthropic/metadata.go`）

Visible thinking 的 signature 直接挂在 `ReasoningPart.Signature` 上（part-level），不再走 message-level metadata。只有"无对应 part"的 RedactedThinkingBlock 仍需要 metadata 通道：

```go
package anthropic

// MetaRedactedReasoning carries the opaque payload of a
// RedactedThinkingBlock — safety-redacted reasoning with no visible
// text but must be replayed unchanged.
const MetaRedactedReasoning = "lynx:chat:anthropic:redacted_reasoning"
```

### 4.3 Google / Bedrock

同样：visible reasoning 的 signature 走 `ReasoningPart.Signature`（`[]byte`，与 Google byte-form 原生兼容）。Bedrock Converse 也走同一路径——参见 `models/bedrock/chat.go`。

### 4.4 OpenAI / DeepSeek

不需要 metadata key——`reasoning_content` 直接被 adapter 转成 `ReasoningPart` 追加进 `Parts`；DeepSeek 不允许回传，所以请求构建时跳过即可。

---

## 5. Provider 映射表

| Provider | 响应填 `ReasoningPart` | 响应填 `Metadata` | 多轮回放 |
|----------|----------------|------------------|---------|
| **Anthropic Thinking** | 每个 ThinkingBlock 一个 part，Signature 落 part.Signature | – | 每个 part 重建 ThinkingBlock（带 signature）|
| **Anthropic Redacted** | – | `anthropic.MetaRedactedReasoning` | 必须回传 RedactedThinkingBlock |
| **Anthropic Summarized** | 摘要文本 → part.Text + part.Signature | – | 单 ThinkingBlock 回放 |
| **Anthropic Omitted** | part.Text="" + part.Signature 非空 | – | 必须回传（仅 signature 的 ThinkingBlock）|
| **DeepSeek-R1** | reasoning_content → part.Text，无 signature | – | **不回传**（API 返 400）|
| **OpenAI o-series Chat Completions** | API 不返 reasoning text | – | 不适用 |
| **OpenAI Responses API** | output_reasoning 段 → part | – | 服务端持久化（previous_response_id）|
| **Google Gemini** | 每个 thought part → ReasoningPart + byte Signature | – | 每个 part 带 signature 回放 |
| **Bedrock Converse** | ReasoningContentBlock → part；Signature 取自 ReasoningTextBlock.Signature | – | 每个 part 重建 ContentBlockMemberReasoningContent |
| **Qwen / Nova（inline `<think>`）** | OpenAI-compat adapter 拆开：thinking 段 → ReasoningPart，剩余 → TextPart | – | 不回传 |

### 5.1 Anthropic 响应路径（实现示意）

```go
// models/anthropic/chat.go::buildAssistantMsg
for _, block := range resp.Content {
    switch b := block.(type) {
    case *sdk.TextBlock:
        parts = append(parts, &chat.TextPart{Text: b.Text})
    case *sdk.ThinkingBlock:
        parts = append(parts, &chat.ReasoningPart{
            Text:      b.Thinking,
            Signature: []byte(b.Signature),
        })
    case *sdk.RedactedThinkingBlock:
        metadata[MetaRedactedReasoning] = b.Data
    case *sdk.ToolUseBlock:
        parts = append(parts, &chat.ToolCallPart{...})
    }
}
```

**Part 顺序与 wire 一致**——多个 thinking block 各占一个 ReasoningPart，每个保留自己的 signature。Redacted thinking 没有 visible part 对应，所以走 message metadata。

### 5.2 Anthropic 请求重建（多轮回放）

```go
// models/anthropic/chat.go (request side)
if data := redactedReasoning(msg); data != "" {
    blocks = append(blocks, sdk.NewRedactedThinkingBlock(data))
}
for p := range msg.Parts {
    switch v := p.(type) {
    case *chat.ReasoningPart:
        // OMITTED display = Text="" + Signature 非空，依然 emit
        if v.Text != "" || len(v.Signature) > 0 {
            blocks = append(blocks, sdk.NewThinkingBlock(string(v.Signature), v.Text))
        }
    case *chat.TextPart:
        if v.Text != "" {
            blocks = append(blocks, sdk.NewTextBlock(v.Text))
        }
    case *chat.ToolCallPart:
        blocks = append(blocks, sdk.NewToolUseBlock(...))
    }
}
```

### 5.3 OpenAI 兼容（DeepSeek-R1 / vLLM）

```go
// models/openai/chat.go (response side)
if reasoningField, ok := msg.JSON.ExtraFields["reasoning_content"]; ok && reasoningField.Valid() {
    var reasoning string
    if err := json.Unmarshal([]byte(reasoningField.Raw()), &reasoning); err == nil {
        parts = append(parts, &chat.ReasoningPart{Text: reasoning})
    }
}
```

请求重建时跳过 `ReasoningPart`——DeepSeek 文档明确禁止回传，silent skip 等同丢弃。

---

## 6. 流式聚合

流式 delta 走 part-级别合并：每个 chunk 携带"刚到达的 part 增量"，`ResponseAccumulator` 按 `(kind, identity)` 调用 `OutputPart.appendDelta` 做 in-place 合并。`ReasoningPart` 的合并规则：

- `Text` 拼接（thinking_delta 累积）
- `Signature` 取后到的非空值覆写（signature 单次终态传输，与 thinking text 同一 part）

`TextPart` 同形拼接，`ToolCallPart` 按 ID 路由合并 `Arguments`。详见 `core/model/chat/response_accumulator.go` 和各 part 的 `appendDelta`。

---

## 7. Inline `<think>` 标签处理

某些 OpenAI 兼容 API 暴露的开源推理模型（Qwen3、Nova、DeepSeek-R1 蒸馏版部分实现）会把 `<think>...</think>` 直接拼在 content 字符串里，不走结构化字段。

**决策**：拆解逻辑放在 **provider 适配器**里，不是 chat 包。
- OpenAI-compat adapter 在 `buildAssistantMsg` 中检测 `msg.Content` 是否以 `<think>` 开头
- 命中则用本包内的正则把 thinking 段剥离 → `Reasoning` 字段
- 剩余文本进 `Text`

这样 chat 包的 `StructuredParser.Parse(text)` 收到的 `text` 已经干净，无需再做 reasoning-tag 兜底。**v0/v1 在 chat 包提供的 `ReasoningTagCleaner` 已删除**——它的存在前提是「reasoning 跟主文本混在一起」，把拆解责任交给 adapter 后这层兜底就是死代码。

> 当前 OpenAI-compat adapter 还没实现 inline 拆解；现有 `JSON.ExtraFields["reasoning_content"]` 路径已覆盖 DeepSeek-R1 / vLLM 等结构化暴露 reasoning 的模型。当真正需要支持 Qwen / Nova 时再在 openai 包内加未导出的 splitter（~30 行正则）。

---

## 8. 边界场景

### 8.1 多 thinking block

标准 extended thinking 一次响应 1 个 block，但 `interleaved-thinking-2025-05-14` beta 模式下可能多个 block 各自带不同 signature。Parts 模型天然支持：每个 block 一个 `ReasoningPart`，保留 emission order，回放时逐个 emit ThinkingBlock。

### 8.2 OMITTED display（仅 signature 无 text）

API 模式：thinking 仍发生，响应只有 signature 没有 text。emit 一个 `ReasoningPart{Text:"", Signature:sig}`，回放时 `NewThinkingBlock(string(sig), "")`——Anthropic 接受空文本 + 非空 signature。

### 8.3 Reasoning 与 ToolCalls 同时存在

```go
msg := &chat.AssistantMessage{
    Parts: []chat.OutputPart{
        &chat.ReasoningPart{Text: "let me think about which tool to call...", Signature: sig},
        &chat.ToolCallPart{ID: "call_1", Name: "search", Arguments: `{"q":"..."}`},
    },
}
```

`msg.HasToolCalls() == true`，`msg.JoinedReasoning()` 拼出全部推理。Tool middleware 现有循环不需修改。

### 8.4 序列化

`Parts []OutputPart` 走 discriminator-based JSON：每个 part 序列化时带 `"kind":"reasoning"` / `"text"` / `"tool_call"`，`chat.UnmarshalMessage` 据此反序列化为具体 part 类型。`ReasoningPart.Signature []byte` 走 base64 标准。

---

## 9. 与 langchain4j / Spring AI 的对照

| 维度 | Spring AI 现状 | Spring AI P0-1 提案 | langchain4j 现状 | **Lynx** |
|------|--------------|------------------|----------------|---------|
| AssistantMessage 字段 | content 单字段 | + List\<ReasoningContent\> | text + thinking | **Parts []OutputPart 含 ReasoningPart** |
| 类型化 reasoning struct | – | record 4 字段 | – | **ReasoningPart{Text, Signature}** |
| signature 位置 | properties Map | record 字段 | attributes Map | **Part 字段** |
| 多 block 表达 | 多 Generation | List | 单 String 拼接 | **多 ReasoningPart 保序** |
| Usage.reasoningTokens | ❌ | ✅ | ❌ | **✅** |
| Inline `<think>` 清洗 | ✅ ThinkingTagCleaner（chat 包）| ✅ | – | adapter 责任，chat 包**不持有** |
| 业务侧 API | 遍历 generations + properties key | `getReasoning()` | `aiMessage.thinking()` | **`msg.JoinedReasoning()` / `msg.ReasoningParts()`** |
| 命名 | thinking / reasoning 混用 | reasoning | thinking | **reasoning** |

### 9.1 形态对比要点

- **vs Spring AI（现状）**：Lynx 已落地、Spring AI P0-1 提案至今未实现
- **vs Spring AI P0-1 提案**：Lynx 的 `ReasoningPart` 是 `OutputPart` 接口的一个 case，与 `TextPart` / `ToolCallPart` 同等地位，保留 emission order；Spring AI 提案的 `record ReasoningContent + 4 type enum` 是单独维护一个并行集合，多 block 时丢失顺序
- **vs langchain4j**：langchain4j `String thinking` 是单字符串扁平字段；Lynx 走 Parts 模型可以保留多 thinking block 顺序 + signature，命名按行业收敛词替换

---

## 10. 验收标准

实现完成后必须满足（现已全部满足）：

1. ✅ 旧符号 `MetaIsThought` / `IsThoughtMessage` / `MetaReasoningContent` / `ThinkingTagCleaner` / `MetaThinkingSignature` / `MetaReasoningSignature` 等在代码中**完全不存在**
2. ✅ `grep -r "anthropic\|openai\|google" core/` 零命中——core 包对 provider 完全无知
3. ✅ `chat.ReasoningPart` 是所有 provider reasoning 内容的**唯一**类型入口
4. ✅ 业务侧拿 reasoning 一行：`resp.Result.AssistantMessage.JoinedReasoning()`
5. ✅ 跨 provider 无差别：`JoinedReasoning()` 在 Anthropic / OpenAI / DeepSeek / Google / Bedrock 一致
6. ✅ Anthropic 多轮（reasoning + tool_use）通过 part-level signature 重建测试
7. ✅ DeepSeek-R1 reasoning 不回传到下一轮
8. ✅ Reasoning + Text + ToolCall 在同一 AssistantMessage 内保序

---

## 11. 一句话定档

> Reasoning 是 `OutputPart` 接口的一个具体 case（`ReasoningPart{Text, Signature}`），与 TextPart / ToolCallPart 同等地位，保留 emission order。`JoinedReasoning()` 一行拿全部，跨 provider 行为一致；需要顺序 / 单个 signature 时迭代 `ReasoningParts()`。core 包对 provider 完全无知，新增 provider 不需要改 chat 抽象层。命名采用业界正在收敛的 "reasoning"——而不是 langchain4j 选择 thinking 时那一刻的状态。

---

*配套阅读*：
- [`SPRING_AI_COMPARISON.md`](./SPRING_AI_COMPARISON.md) — 把 reasoning 列在 "Lynx 反超 Spring AI" 章节
- langchain4j 形态参照：https://github.com/langchain4j/langchain4j/blob/main/langchain4j-core/src/main/java/dev/langchain4j/data/message/AiMessage.java
