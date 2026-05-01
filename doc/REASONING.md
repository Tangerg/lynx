# Reasoning 一等公民

> Lynx 把 reasoning（链式推理 / chain-of-thought）作为 `AssistantMessage` 的独立字段而不是 metadata 标记。设计已落地，本文是最终态文档。
>
> **状态**：已合并至 main（commit `f75f21a` Merge `feat/thinking-support`）。
>
> **取代**：原 `REASONING_UNIFIED_DESIGN.md`（设计稿）+ `SPRING_AI_THINKING_ARCHITECTURE.md`（v0 状态）。

---

## 1. TL;DR

`AssistantMessage` 加一个 `Reasoning string` 字段，和 `Text` 平级。Provider-specific 的 signature / 安全脱敏数据进 `Metadata` map——**key 常量和 helper 都定义在各自的 provider 包里，core 包零 provider 知识**。命名采用业界正在收敛的 "reasoning"。

```go
msg := resp.Result().AssistantMessage
println(msg.Text)        // 主答内容
println(msg.Reasoning)   // 推理链（任何支持 reasoning 的 provider 一致）
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

### 3.1 AssistantMessage

```go
// core/model/chat/message.go

type AssistantMessage struct {
    Text      string         `json:"text"`
    Reasoning string         `json:"reasoning,omitempty"`  // 🆕 visible reasoning text
    Media     []*media.Media `json:"media"`
    ToolCalls []*ToolCall    `json:"tool_calls"`
    Metadata  map[string]any `json:"metadata"`
}

func (a *AssistantMessage) HasReasoning() bool {
    return a != nil && a.Reasoning != ""
}
```

`MessageParams` 同步加 `Reasoning string` 字段。

### 3.2 Response 便利视图

```go
// core/model/chat/response.go

func (r *Response) Reasoning() string {
    var sb strings.Builder
    for _, result := range r.Results {
        if result == nil || result.AssistantMessage == nil { continue }
        sb.WriteString(result.AssistantMessage.Reasoning)
    }
    return sb.String()
}

func (r *Response) OutputText() string {
    var sb strings.Builder
    for _, result := range r.Results {
        if result == nil || result.AssistantMessage == nil { continue }
        sb.WriteString(result.AssistantMessage.Text)
    }
    return sb.String()
}
```

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

```go
package anthropic

const (
    // MetaReasoningSignature is the per-block continuity token Anthropic
    // returns alongside a ThinkingBlock. Must be replayed verbatim or
    // the API rejects the request.
    MetaReasoningSignature = "lynx:chat:anthropic:reasoning_signature"

    // MetaRedactedReasoning is the opaque payload of a RedactedThinkingBlock —
    // safety-redacted reasoning with no visible text but must be replayed unchanged.
    MetaRedactedReasoning = "lynx:chat:anthropic:redacted_reasoning"
)

func reasoningSignature(m *chat.AssistantMessage) string {
    if m == nil { return "" }
    v, _ := m.Metadata[MetaReasoningSignature].(string)
    return v
}
func redactedReasoning(m *chat.AssistantMessage) string {
    if m == nil { return "" }
    v, _ := m.Metadata[MetaRedactedReasoning].(string)
    return v
}
```

### 4.3 Google（前瞻设计 / `models/google/metadata.go`）

```go
package google

const (
    // MetaReasoningSignatures is the list of byte-form continuity tokens Gemini
    // emits per part. Replay rebuilds each part with its corresponding signature.
    MetaReasoningSignatures = "lynx:chat:google:reasoning_signatures"
)
```

### 4.4 OpenAI / DeepSeek

不需要 metadata key——`reasoning_content` 直接进 `AssistantMessage.Reasoning` 字段；DeepSeek 不允许回传，所以请求构建时连 metadata 都不需要看。

---

## 5. Provider 映射表

| Provider | 响应填 `Reasoning` | 响应填 `Metadata` | 多轮回放 |
|----------|----------------|------------------|---------|
| **Anthropic Thinking** | ✅ 拼接所有 thinking text | `anthropic.MetaReasoningSignature`（首个）| 必须回传，重建 ThinkingBlock |
| **Anthropic Redacted** | `""` | `anthropic.MetaRedactedReasoning` | 必须回传 RedactedThinkingBlock |
| **Anthropic Summarized** | ✅ 摘要文本 | `anthropic.MetaReasoningSignature` | 必须回传 |
| **Anthropic Omitted** | `""` | `anthropic.MetaReasoningSignature` | 必须回传（仅 signature）|
| **DeepSeek-R1** | ✅ reasoning_content 字符串 | – | **不回传**（API 返 400）|
| **OpenAI o-series Chat Completions** | `""`（API 不返）| – | 不适用 |
| **OpenAI o-series Responses API**（未来）| `output_reasoning` | – | 服务端持久化（previous_response_id）|
| **Google Gemini** | ✅ thought parts 拼接 | `google.MetaReasoningSignatures` | 必须回传 byte signatures |
| **Qwen / Nova（inline `<think>`）** | OpenAI-compat adapter 内部拆开：thinking text → Reasoning，stripped content → Text | – | 不回传 |

### 5.1 Anthropic 响应路径（实现示意）

```go
// models/anthropic/chat.go::buildAssistantMsg
var reasoningBuf strings.Builder
var firstSignature string
var redactedData string

for _, block := range resp.Content {
    switch block.Type {
    case "text":
        msgParams.Text += block.Text
    case "tool_use":
        rawInput, _ := json.Marshal(block.Input)
        msgParams.ToolCalls = append(msgParams.ToolCalls, &chat.ToolCall{...})
    case "thinking":
        reasoningBuf.WriteString(block.Thinking)
        if firstSignature == "" {
            firstSignature = block.Signature   // 标准模式一次响应 1 个 thinking block
        }
    case "redacted_thinking":
        redactedData = block.Data
    }
}

msgParams.Reasoning = reasoningBuf.String()
if firstSignature != "" {
    msgParams.Metadata[MetaReasoningSignature] = firstSignature
}
if redactedData != "" {
    msgParams.Metadata[MetaRedactedReasoning] = redactedData
}
```

**单一 Result**——彻底废弃 v0 的「thinking 一个 Result + text 一个 Result」双 Result 拆分。

### 5.2 Anthropic 请求重建（多轮回放）

```go
// models/anthropic/chat.go (request side)
// 顺序：thinking → text → tool_use（API 词汇）

if data := redactedReasoning(msg); data != "" {
    blocks = append(blocks, sdk.NewRedactedThinkingBlock(data))
}
sig := reasoningSignature(msg)
if msg.Reasoning != "" && sig != "" {
    blocks = append(blocks, sdk.NewThinkingBlock(sig, msg.Reasoning))
} else if sig != "" {
    // OMITTED display：仅 signature，无 text
    blocks = append(blocks, sdk.NewThinkingBlock(sig, ""))
}
// 没 signature 就跳过——多半是从外部反序列化来的、签名丢失的历史。

if msg.Text != "" {
    blocks = append(blocks, sdk.NewTextBlock(msg.Text))
}
```

### 5.3 OpenAI 兼容（DeepSeek-R1 / vLLM）

```go
// models/openai/chat.go (response side)
if reasoningField, ok := msg.JSON.ExtraFields["reasoning_content"]; ok && reasoningField.Valid() {
    var reasoning string
    if err := json.Unmarshal([]byte(reasoningField.Raw()), &reasoning); err == nil {
        msgParams.Reasoning = reasoning  // 直接进字段，不走 metadata
    }
}
```

请求重建时**不读 `msg.Reasoning`**——DeepSeek 文档明确禁止回传，silent skip 等同丢弃。

---

## 6. 流式聚合

```go
// core/model/chat/response_accumulator.go
func (r *ResponseAccumulator) accumulateAssistantMessage(msg, other *AssistantMessage) *AssistantMessage {
    if msg == nil { msg = &AssistantMessage{} }

    msg.Text      += other.Text
    msg.Reasoning += other.Reasoning   // 与 Text 同形——拼接

    if len(other.ToolCalls) > 0 { /* 既有逻辑 */ }

    // Metadata last-write-wins
    for k, v := range other.Metadata { msg.Meta()[k] = v }
    return msg
}
```

**关于 Metadata last-write-wins**：流式签名 chunk 是「thinking_delta 累积 → 最后一条 signature_delta 到达」，最后一次覆写就是正确的最终签名。Google `[][]byte` 在 SDK accumulator 风格下每个 chunk 给完整列表（snapshot），覆写正确。

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

标准 extended thinking 一次响应 1 个 block，但 `interleaved-thinking-2025-05-14` beta 模式下可能多个 block，各自不同 signature。

**v1 决策**：拼接所有 reasoning text 进 `Reasoning` 字段，**只保留第一个 signature**。`interleaved-thinking` beta 不在初版支持范围。如果未来要支持，再扩展为 `value` 改 `[]string`（带 deprecation 周期）。

### 8.2 OMITTED display（仅 signature 无 text）

API 模式：thinking 仍发生，响应只有 signature 没有 text。`Reasoning = ""` + `Metadata[MetaReasoningSignature] = sig`。多轮回放时 `buildAssistantMsg` 检查 `Reasoning == ""` 但有 signature 时仍 emit `NewThinkingBlock(sig, "")`——Anthropic 接受空文本 + 非空 signature。

### 8.3 Reasoning 与 ToolCalls 同时存在

```go
&AssistantMessage{
    Text:      "",
    Reasoning: "let me think about which tool to call...",
    ToolCalls: []*ToolCall{ {ID: "call_1", Name: "search", ...} },
    Metadata:  map[string]any{ MetaReasoningSignature: "sig" },
}
```

`HasReasoning() && HasToolCalls() == true`。Tool middleware 现有循环不需修改。

### 8.4 序列化

`Reasoning string` 直接 JSON marshal，`omitempty` 让无 reasoning 的消息不带这个字段。Metadata 里的 `[]byte`（Google signatures）走 base64 string 标准。

---

## 9. 与 langchain4j / Spring AI 的对照

| 维度 | Spring AI 现状 | Spring AI P0-1 提案 | langchain4j 现状 | **Lynx** |
|------|--------------|------------------|----------------|---------|
| AssistantMessage 字段 | content 单字段 | + List\<ReasoningContent\> | text + thinking | **Text + Reasoning** |
| 类型化 reasoning struct | – | record 4 字段 | – | – |
| signature 位置 | properties Map | record 字段 | attributes Map | **Metadata Map** |
| 多 block 表达 | 多 Generation | List | 单 String 拼接 | **单 String 拼接** |
| Usage.reasoningTokens | ❌ | ✅ | ❌ | **✅** |
| Inline `<think>` 清洗 | ✅ ThinkingTagCleaner（chat 包）| ✅ | – | adapter 责任，chat 包**不持有** |
| 业务侧 API | 遍历 generations + properties key | `getReasoning()` | `aiMessage.thinking()` | **`msg.Reasoning` 字段** |
| 命名 | thinking / reasoning 混用 | reasoning | thinking | **reasoning** |

### 9.1 形态对比要点

- **vs Spring AI（现状）**：Lynx 已落地、Spring AI P0-1 提案至今未实现
- **vs Spring AI P0-1 提案**：Lynx 用 single string + Metadata map 比 Spring AI 提案的 `record ReasoningContent + 4 type enum` 轻——Java record 是免费的、Go 不是
- **vs langchain4j**：形态对应（`String thinking` → `Reasoning string`，`Map attributes` → `Metadata map`），命名按行业收敛词替换

---

## 10. 验收标准

实现完成后必须满足（现已全部满足）：

1. ✅ 旧符号 `MetaIsThought` / `IsThoughtMessage` / `MetaReasoningContent` / `ThinkingTagCleaner` / `MetaThinkingSignature` 等在代码中**完全不存在**
2. ✅ `grep -r "anthropic\|openai\|google" core/` 零命中——core 包对 provider 完全无知
3. ✅ `AssistantMessage.Reasoning` 是所有 provider reasoning 内容的**唯一**字段入口
4. ✅ 业务侧拿 reasoning 一行：`resp.Result().AssistantMessage.Reasoning`
5. ✅ 跨 provider 无差别：`resp.Reasoning()` 工作于 Anthropic / OpenAI / DeepSeek / Google 一致
6. ✅ Anthropic 多轮（reasoning + tool_use）通过 signature 重建测试
7. ✅ DeepSeek-R1 reasoning 不回传到下一轮
8. ✅ `Response.Results` 在 Anthropic 响应里恢复为 1 个（无 reasoning 拆分）

---

## 11. 一句话定档

> Reasoning 是和 Text 平级的 String 字段，不是 list、不是 struct、不是 metadata key。Provider-specific signature 走 Metadata map（key 常量定义在 provider 包内，字符串值带 `lynx:chat:<provider>:<concept>` 前缀避免冲突）。`msg.Reasoning` 一行拿全部，跨 provider 行为一致。core 包对 provider 完全无知，新增 provider 不需要改 chat 抽象层。命名采用业界正在收敛的 "reasoning"——而不是 langchain4j 选择 thinking 时（早期跟随 Anthropic）那一刻的状态。

---

*配套阅读*：
- [`SPRING_AI_GAPS.md`](./SPRING_AI_GAPS.md) — 把 reasoning 列在 "Lynx 反超 Spring AI" 清单第 1 条
- langchain4j 形态参照：https://github.com/langchain4j/langchain4j/blob/main/langchain4j-core/src/main/java/dev/langchain4j/data/message/AiMessage.java
