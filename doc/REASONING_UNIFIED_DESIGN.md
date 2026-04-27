# 统一 Reasoning 抽象 — 设计文档（v2）

> **状态**：设计提案（pre-implementation）
> **关联分支**：`feat/thinking-support`（实现 commit 时建议 rename 为 `feat/reasoning-support`）
> **取代**：`SPRING_AI_THINKING_ARCHITECTURE.md §9` + 本文 v1（ReasoningContent struct 版）+ v2 早期版（Thinking 命名版）
> **设计参照**：[`langchain4j AiMessage.java`](https://github.com/langchain4j/langchain4j/blob/main/langchain4j-core/src/main/java/dev/langchain4j/data/message/AiMessage.java)
> **拒绝采用**：[Spring AI P0-1 ReasoningContent record 提案](file:///Users/tangerg/Desktop/spring-ai/docs/spring-ai-architecture/05-optimization-suggestions.md)
>
> **一句话总结**：`AssistantMessage` 加一个 `Reasoning string` 字段，和 `Text` 平级。Provider-specific 的 signature / 安全脱敏数据进 `Metadata` map——**key 常量和 helper 都定义在各自的 provider 包里，core 包零 provider 知识**。命名采用业界正在收敛的 "reasoning"。**净改动 ~-260 行（删除 v0 的多 Result 拆分），与 langchain4j 实证可行的设计形态对齐**。

---

## 0. 设计演化记录

| 版本 | 设计 | 命名 | 评估 |
|-----|------|------|-----|
| v0（已 push） | 双模式：Anthropic 多 Result + OpenAI 元数据通道 | `IsThoughtMessage` / `MetaReasoningContent` 等 6 key + 4 访问器 | ❌ 抽象不统一，业务侧两条访问路径 |
| v1（Spring AI P0-1） | `Reasoning []*ReasoningContent`（Type enum + 多 signature 字段） | – | ⚠️ 完整但偏重，Go 无 record 写起来啰嗦，~+210 LOC |
| v2 早期 | 单 String 字段 + Metadata map | `Thinking` | 🟡 形态对，命名次优 |
| v2 中期 | 单 String + Metadata map | `Reasoning`，但 key 集中在 chat 包 | 🟡 chat 包泄漏 provider 知识 |
| **v2（本文）** | **单 String + Metadata，key 下沉到各 provider 包** | **`Reasoning`** | ✅ **采纳** |

### 0.1 命名选择：`Reasoning` 而非 `Thinking`

| 维度 | `Thinking` | `Reasoning` |
|-----|-----------|------------|
| Provider 票数 | Anthropic / Google / Qwen / Ollama / Nova（5） | OpenAI / DeepSeek / Bedrock（3） |
| 行业品类名 | – | "reasoning models"（o1/o3/R1/gpt-5） |
| API 字段一致性 | – | `reasoning_effort` / `reasoning_tokens` / `reasoning_content` 几乎所有新规范 |
| 技术精确度 | 偏拟人化 | 链式推理，更准 |
| langchain4j 选择 | ✅ 选了它 | – |
| Spring AI 优化提案 | – | ✅ 选了它（P0-1） |

**决策**：`Reasoning`。langchain4j 选择 `thinking` 是因为他们落地早（跟随 Anthropic 第一家），现在 industry vocabulary 已经收敛到 reasoning。Lynx 还在迭代期，直接走收敛终态。

---

## 1. 当前状态盘点

`feat/thinking-support` 分支已 push 的实现：

- 6 个 marker key 常量：`MetaIsThought` / `MetaThinkingSignature` / `MetaRedactedThinkingData` / `MetaReasoningContent` / `MetaThoughts` / `MetaOutputWithoutThoughts`
- 4 个访问器：`IsThoughtMessage` / `ThinkingSignature` / `RedactedThinkingData` / `ReasoningContent`
- Anthropic 适配器走**多 Result 模式**
- OpenAI 适配器走**元数据通道模式**
- `Response.Thoughts()` / `OutputText()` 同时识别两套
- `ThinkingTagCleaner`（与本设计正交，保留并 rename 为 `ReasoningTagCleaner`）

**核心痛点**：业务侧拿 reasoning 要么过 `IsThoughtMessage`、要么过 `ReasoningContent`，两套互斥访问器是 Spring AI 自己点名批评的"leaky abstraction"。Lynx 没正式 release，直接走对的设计。

---

## 2. 目标设计（v2）

### 2.1 核心原则

1. **`AssistantMessage.Reasoning string`**：单一字符串字段，跨 provider 唯一出口
2. **不引入新类型**：没有 `ReasoningContent` struct、没有 `ReasoningType` enum
3. **签名 / 脱敏数据进 `Metadata`**：用文档化 well-known key 常量，适配器封装读写细节
4. **`Usage.ReasoningTokens *int64`**：零成本可选字段（OpenAI 计费需求）
5. **保持 ISP**：不支持 reasoning 的 provider 自然 `Reasoning == ""`，无负担

### 2.2 类型改造

#### 2.2.1 `AssistantMessage`

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

`MessageParams` 同步加 `Reasoning string` 字段；泛型构造分支不变（reasoning 必须显式 `MessageParams` 入参）。

#### 2.2.2 Provider-specific Metadata key —— **下沉到各 provider 包**

**关键设计原则**：core/model/chat 包对 provider 一无所知。signature / 脱敏数据这类 provider-specific 概念的 key 常量和读写 helper 都定义在各自的 provider 包里。chat 包只规定**字符串前缀约定**：

```
lynx:chat:<provider>:<concept>
```

任何写到 `AssistantMessage.Metadata` 的 provider 内部 key 都遵守这个 namespace，避免不同 provider 之间或与未来 chat 通用 key 冲突。

**Anthropic 包**（`models/anthropic/metadata.go` 或合并进 `chat.go` 顶部）：

```go
package anthropic

// Metadata keys for Anthropic-specific reasoning continuation tokens.
// Stored on AssistantMessage.Metadata by the response side and read by
// the request side for multi-turn replay. Business code does NOT need
// to read these directly — they are exported only so other adapters or
// integrations within the Lynx ecosystem can interop if needed.
const (
    // MetaReasoningSignature is the per-block continuity token Anthropic
    // returns alongside a ThinkingBlock. Must be replayed verbatim or
    // the API rejects the request. Value type: string.
    MetaReasoningSignature = "lynx:chat:anthropic:reasoning_signature"

    // MetaRedactedReasoning is the opaque payload of a
    // RedactedThinkingBlock — safety-redacted reasoning with no visible
    // text but must be replayed unchanged. When present,
    // AssistantMessage.Reasoning will be empty. Value type: string.
    MetaRedactedReasoning = "lynx:chat:anthropic:redacted_reasoning"
)

// Internal accessors used by request/response helpers in this package.
// Returns "" when the message lacks the corresponding key — adapter
// treats this as "no replay needed".
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

**Google 包**（前瞻设计，`models/google/metadata.go`）：

```go
package google

const (
    // MetaReasoningSignatures is the list of byte-form continuity
    // tokens Gemini emits per part (Google API calls these
    // "thoughtSignatures"). Replay rebuilds each part with its
    // corresponding signature in order. Value type: [][]byte.
    MetaReasoningSignatures = "lynx:chat:google:reasoning_signatures"
)

func reasoningSignatures(m *chat.AssistantMessage) [][]byte {
    if m == nil { return nil }
    v, _ := m.Metadata[MetaReasoningSignatures].([][]byte)
    return v
}
```

**OpenAI / DeepSeek 包**：当前不需要任何 metadata key——reasoning_content 直接进 `AssistantMessage.Reasoning` 字段；DeepSeek 不允许回传，所以请求构建时连 metadata 都不需要看。

#### 2.2.3 chat 包对 reasoning 的总贡献

| 抽象 | 位置 |
|------|------|
| `AssistantMessage.Reasoning` 字段 | `core/model/chat/message.go` |
| `AssistantMessage.HasReasoning()` 方法 | 同上 |
| `Response.Reasoning()` 便利视图 | `core/model/chat/response.go` |
| 流式累积 `msg.Reasoning += other.Reasoning` | `core/model/chat/response_accumulator.go` |
| `Usage.ReasoningTokens *int64` | `core/model/usage.go` |
| `ReasoningTagCleaner` 工具 | `core/model/chat/reasoning_cleaner.go` |
| key 前缀约定（`lynx:chat:<provider>:<concept>`）| 在本设计文档说明，无代码强制 |

**chat 包不持有任何 provider 名称的代码**——`anthropic` / `google` / `openai` 这些字符串在 core 里 grep 零命中，是验收标准之一。

#### 2.2.4 业务侧便利方法

```go
// core/model/chat/response.go

// Reasoning returns the concatenation of AssistantMessage.Reasoning
// from every result in this response, separated by no delimiter.
// Empty string when no result carries reasoning.
func (r *Response) Reasoning() string {
    if r == nil { return "" }
    var sb strings.Builder
    for _, result := range r.Results {
        if result == nil || result.AssistantMessage == nil { continue }
        sb.WriteString(result.AssistantMessage.Reasoning)
    }
    return sb.String()
}

// OutputText returns AssistantMessage.Text concatenated across results.
// (Now trivial because Reasoning is a separate field — no need to skip
// "thought results" the way v0 had to.)
func (r *Response) OutputText() string {
    if r == nil { return "" }
    var sb strings.Builder
    for _, result := range r.Results {
        if result == nil || result.AssistantMessage == nil { continue }
        sb.WriteString(result.AssistantMessage.Text)
    }
    return sb.String()
}
```

> **改名**：v0 的 `Response.Thoughts()` → v2 的 `Response.Reasoning()`，与字段名对齐。`OutputText()` 保留。

#### 2.2.5 `Usage` 改造

```go
// core/model/usage.go

type Usage struct {
    PromptTokens     int64  `json:"prompt_tokens"`
    CompletionTokens int64  `json:"completion_tokens"`
    ReasoningTokens  *int64 `json:"reasoning_tokens,omitempty"`  // 🆕
    OriginalUsage    any    `json:"original_usage,omitempty"`
}

func (u *Usage) TotalTokens() int64 {
    return u.PromptTokens + u.CompletionTokens
}

func (u *Usage) HasReasoningTokens() bool {
    return u != nil && u.ReasoningTokens != nil
}
```

**`ReasoningTokens` 不重复计入 `TotalTokens()`**——OpenAI 协议里它是 `completion_tokens` 的子集（`completion_tokens_details.reasoning_tokens`），`CompletionTokens` 已经包含。

---

## 3. Provider 映射

### 3.1 Anthropic

**响应路径**（`models/anthropic/chat.go::buildAssistantMsg`）：

```go
func (r *responseHelper) buildAssistantMsg(resp *anthropicsdk.Message) *chat.AssistantMessage {
    msgParams := chat.MessageParams{ Metadata: make(map[string]any) }

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
            // Anthropic SDK 类型名是 ThinkingBlock，Lynx 抽象层叫 reasoning
            reasoningBuf.WriteString(block.Thinking)
            // 多个 thinking block 时只保留第一个 signature。Anthropic 实际
            // 标准模式（非 interleaved-thinking beta）一次响应只产 1 个
            // thinking block，所以这是无损的。interleaved-thinking 暂不支持。
            if firstSignature == "" {
                firstSignature = block.Signature
            }
        case "redacted_thinking":
            redactedData = block.Data
        }
    }

    msgParams.Reasoning = reasoningBuf.String()
    if firstSignature != "" {
        // MetaReasoningSignature 是 anthropic 包内导出常量
        msgParams.Metadata[MetaReasoningSignature] = firstSignature
    }
    if redactedData != "" {
        msgParams.Metadata[MetaRedactedReasoning] = redactedData
    }

    return chat.NewAssistantMessage(msgParams)
}
```

**单一 Result**——彻底废弃 v0 的"thinking 一个 Result + text 一个 Result"双 Result 拆分。`buildResults` / `buildThinkingResult` 一并删除。

**请求重建**（多轮回放，request 侧的 `buildAssistantMsg`，仍在 anthropic 包内）：

```go
func (r *requestHelper) buildAssistantMsg(msg *chat.AssistantMessage) anthropicsdk.MessageParam {
    blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, 2+len(msg.ToolCalls))

    // Anthropic 要求顺序：thinking → text → tool_use（API 词汇）
    // redactedReasoning / reasoningSignature 是本包内未导出 helper
    if data := redactedReasoning(msg); data != "" {
        blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(data))
    }
    sig := reasoningSignature(msg)
    if msg.Reasoning != "" && sig != "" {
        blocks = append(blocks, anthropicsdk.NewThinkingBlock(sig, msg.Reasoning))
    } else if sig != "" {
        // OMITTED display 场景：仅 signature，无 text
        blocks = append(blocks, anthropicsdk.NewThinkingBlock(sig, ""))
    }
    // 没 signature 就跳过——多半是从外部反序列化来的、签名丢失的历史。
    if msg.Text != "" {
        blocks = append(blocks, anthropicsdk.NewTextBlock(msg.Text))
    }
    for _, tc := range msg.ToolCalls {
        blocks = append(blocks, anthropicsdk.NewToolUseBlock(tc.ID, json.RawMessage(tc.Arguments), tc.Name))
    }
    return anthropicsdk.NewAssistantMessage(blocks...)
}
```

### 3.2 OpenAI 兼容（DeepSeek-R1 / vLLM）

**响应路径**：

```go
if reasoningField, ok := msg.JSON.ExtraFields["reasoning_content"]; ok && reasoningField.Valid() {
    var reasoning string
    if err := json.Unmarshal([]byte(reasoningField.Raw()), &reasoning); err == nil {
        msgParams.Reasoning = reasoning  // 🎯 直接进字段，不走 metadata
    }
}
```

**请求重建**：DeepSeek 文档明确禁止回传 reasoning_content（API 返 400）。所以 `buildAssistantMsg` 在请求侧**不读 `msg.Reasoning`**——silent skip，等同丢弃。

**reasoning_tokens**：从 `resp.Usage.CompletionTokensDetails.ReasoningTokens`（如 SDK 暴露）抽出，写 `Usage.ReasoningTokens`。当前 SDK 不暴露则留 TODO。

### 3.3 Google Gemini（前瞻设计）

```go
// 响应：每个 part 检查 thought=true，文本拼进 Reasoning
var reasoningBuf strings.Builder
var sigList [][]byte
for _, part := range resp.Candidates[0].Content.Parts {
    if part.Thought {
        reasoningBuf.WriteString(part.Text)
    } else if part.Text != "" {
        msgParams.Text += part.Text
    }
    if len(part.ThoughtSignature) > 0 {
        sigList = append(sigList, part.ThoughtSignature)
    }
}
msgParams.Reasoning = reasoningBuf.String()
if len(sigList) > 0 {
    // MetaReasoningSignatures 是 google 包内导出常量
    msgParams.Metadata[MetaReasoningSignatures] = sigList
}
```

请求重建时把 `[][]byte` 还原到对应 part。

### 3.4 兼容性约定总表

| Provider | 响应填 `Reasoning` | 响应填 `Metadata`（key 在 provider 包内）| 多轮回放 |
|----------|-----------------|----------------------------|---------|
| Anthropic Thinking | ✅ 拼接所有 thinking text | `anthropic.MetaReasoningSignature`（首个）| 必须回传，重建 ThinkingBlock |
| Anthropic Redacted | `""` | `anthropic.MetaRedactedReasoning` | 必须回传 RedactedThinkingBlock |
| Anthropic Summarized | ✅ 摘要文本 | `anthropic.MetaReasoningSignature` | 必须回传 |
| Anthropic Omitted | `""` | `anthropic.MetaReasoningSignature` | 必须回传（仅 signature）|
| DeepSeek-R1 | ✅ reasoning_content 字符串 | – | **不回传** |
| OpenAI o-series Chat Completions | `""`（API 不返）| – | 不适用 |
| OpenAI o-series Responses API | （未来）`output_reasoning` | – | 服务端持久化（previous_response_id）|
| Google Gemini | ✅ thought parts 拼接 | `google.MetaReasoningSignatures` | 必须回传 byte signatures |
| Qwen / Nova（inline `<think>`）| （由 `ReasoningTagCleaner` 在结构化输出前清掉）| – | 不回传 |

---

## 4. 流式聚合

`ResponseAccumulator.accumulateAssistantMessage` 改造极小：

```go
func (r *ResponseAccumulator) accumulateAssistantMessage(msg, other *AssistantMessage) *AssistantMessage {
    if other == nil { return msg }
    if msg == nil { msg = &AssistantMessage{} }

    msg.Text      += other.Text       // 既有
    msg.Reasoning += other.Reasoning  // 🆕 与 Text 同形——拼接

    // ToolCalls：既有逻辑不变
    if len(other.ToolCalls) > 0 { ... }

    // Metadata：last-write-wins（既有）
    for k, v := range other.Metadata {
        msg.Meta()[k] = v
    }
    return msg
}
```

**删除内容**：v0 的 `MetaReasoningContent` 拼接特例——不再需要，因为 reasoning 现在走 `Reasoning` 字段。

> **关于 Metadata last-write-wins**：流式签名 chunk 是"thinking_delta 累积 → 最后一条 signature_delta 到达"，最后一次覆写就是正确的最终签名。Google `[][]byte` 在 SDK accumulator 风格下每个 chunk 给完整列表（snapshot），覆写正确。

---

## 5. 多轮回放规则总表

| 场景 | v0 实现 | v2 设计 |
|------|---------|--------|
| Anthropic thinking + tool_use 多轮 | 历史 Result 检查 `IsThoughtMessage`，重建 ThinkingBlock | 历史 AssistantMessage 读 `Reasoning` + `Metadata[anthropic.MetaReasoningSignature]`，重建 ThinkingBlock |
| Anthropic redacted | `MetaIsThought=true` + `MetaRedactedThinkingData` | `Metadata[anthropic.MetaRedactedReasoning]`，重建 RedactedThinkingBlock |
| DeepSeek-R1 多轮 | reasoning 在 Metadata，请求构建时不读 | `Reasoning` 字段，请求构建时不读（silent skip）|
| Google Gemini 多轮 | 暂不存在 | `Metadata[google.MetaReasoningSignatures]` 还原到对应 part |

---

## 6. 边界场景与决策

### 6.1 Anthropic 多 thinking block 限制

**问题**：标准 extended thinking 一次响应 1 个 block，但 `interleaved-thinking-2025-05-14` beta 模式下可能有多个 block，各自不同 signature。

**v2 决策**：拼接所有 reasoning text 进 `Reasoning` 字段，**只保留第一个 signature**。`interleaved-thinking` beta 模式**不在初版支持范围**。如果未来要支持，再扩展为 `MetaAnthropicReasoningSignature` value 改 `[]string`（带 deprecation 周期）。

**理由**：langchain4j 也只支持单 reasoning string，没有看到他们投诉过这个限制。Anthropic 99% 用例是单 block。

### 6.2 Anthropic OMITTED display（仅 signature 无 text）

API 模式：thinking 仍发生，响应只有 signature 没有 text。

**v2 处理**：`Reasoning = ""` + `Metadata[MetaAnthropicReasoningSignature] = sig`。多轮回放时 `buildAssistantMsg` 检查到 `Reasoning == ""` 但有 signature 时，**仍然 emit** 一个 `NewThinkingBlock(sig, "")`——Anthropic 在 OMITTED 模式接受空文本 + 非空 signature。具体见 §3.1 请求重建逻辑。

### 6.3 Inline `<think>` 标签

`ReasoningTagCleaner`（旧名 `ThinkingTagCleaner`，本次一并 rename）在结构化输出解析前生效，**不**自动剥离进 `Reasoning` 字段——保持 v0/v1 的"显式优于隐式"决定。

> 可选未来增强：在 OpenAI 兼容 adapter 的 response 路径检测 content 起始 `<think>...</think>`，opt-in flag 开启时自动剥离塞 `Reasoning`。**不在初版**。

### 6.4 Reasoning 与 ToolCalls 同时存在

```go
&AssistantMessage{
    Text:      "",
    Reasoning: "let me think about which tool to call...",
    ToolCalls: []*ToolCall{ {ID: "call_1", Name: "search", ...} },
    Metadata:  map[string]any{ MetaAnthropicReasoningSignature: "sig" },
}
```

`HasReasoning() && HasToolCalls() == true`。Tool middleware 现有循环不需修改。

### 6.5 序列化

`Reasoning string` 直接 JSON marshal，`omitempty` 让无 reasoning 的消息不带这个字段。Metadata 里的 `[]byte`（Google signatures）走 base64 string 标准。

---

## 7. 迁移 checklist

### 7.1 移除 v0 实现

```
core/model/chat/thinking.go → 直接删除（无替代文件）
  原因：v0 的所有常量/访问器都被以下两个力量取代：
    1. 通用部分（Reasoning 字段、HasReasoning、Response.Reasoning）→ 进 message.go / response.go
    2. provider 专属部分（signature key、helper）→ 下沉到 models/<provider>/

  删除的符号：
  - MetaIsThought / MetaThinkingSignature / MetaRedactedThinkingData
  - MetaReasoningContent / MetaThoughts / MetaOutputWithoutThoughts
  - IsThoughtMessage() / ThinkingSignature()
  - RedactedThinkingData() / ReasoningContent()

core/model/chat/thinking_cleaner.go → 重命名为 reasoning_cleaner.go
  - ThinkingTagCleaner → ReasoningTagCleaner
  - NewThinkingTagCleaner → NewReasoningTagCleaner
  - DefaultThinkingTagPatterns → DefaultReasoningTagPatterns
  - CleanThinkingTags → CleanReasoningTags
  - 默认 pattern 不变（仍然识别 <think> / <thinking> / <reasoning> 等）

core/model/chat/response_accumulator.go:
  - MetaReasoningContent 拼接特例（~10 行）

core/model/chat/response.go:
  - Thoughts() 中检查 IsThoughtMessage 的逻辑

core/model/chat/thinking_test.go → 重命名为 reasoning_test.go
  - 全部基于 metadata key 的断言重写

models/anthropic/chat.go:
  - buildResults() / buildThinkingResult() 整段删除
  - request 侧 buildAssistantMsg 中 IsThoughtMessage 分支
  
models/openai/chat.go:
  - reasoning_content 写进 Metadata[MetaReasoningContent] 改写进 Reasoning 字段
```

### 7.2 新增 / 改造

| 步骤 | 文件 | 改动 |
|-----|------|------|
| 1 | `core/model/chat/thinking.go` 删除 | v0 的常量/访问器整体下线 |
| 2 | `core/model/chat/message.go` | `AssistantMessage.Reasoning string` 字段 + `HasReasoning()`；`MessageParams.Reasoning` |
| 3 | `core/model/chat/response.go` | `Response.Reasoning()` + 简化 `OutputText()` |
| 4 | `core/model/chat/response_accumulator.go` | `msg.Reasoning += other.Reasoning` 一行；删 MetaReasoningContent 特例 |
| 5 | `core/model/usage.go` | `ReasoningTokens *int64` + `HasReasoningTokens()` |
| 6 | `core/model/chat/reasoning_cleaner.go`（rename from thinking_cleaner.go）| 类型与函数全部 rename，保留默认 patterns |
| 7 | `core/model/chat/parser.go` | `CleanThinkingTags` 调用改 `CleanReasoningTags` |
| 8 | `models/anthropic/metadata.go`（**新文件**）| 定义 `MetaReasoningSignature` / `MetaRedactedReasoning` 常量 + 包内 helper |
| 9 | `models/anthropic/chat.go` | 响应单 Result 写 Reasoning + Metadata；请求从 Reasoning + Metadata 重建 |
| 10 | `models/openai/chat.go` | reasoning_content 改写 Reasoning；抽 ReasoningTokens 进 Usage（无 metadata key 需求）|
| 11 | `core/model/chat/reasoning_test.go`（rename）| 重写为 Reasoning 字段测试 |
| 12 | `doc/SPRING_AI_THINKING_ARCHITECTURE.md` | §9 重写 |
| 13 | `doc/SPRING_AI_CAPABILITY_GAPS.md` | §1.3 标 superseded by 本文 |

> **未来 google adapter 落地时**：新建 `models/google/metadata.go` 定义 `MetaReasoningSignatures`，与 Anthropic 同样的位置局部性原则。本次实现暂不涉及 google。

### 7.3 估计代码量

| 类别 | 行数变化 |
|------|--------|
| `thinking.go` 整体删除 | -160 |
| `message.go`（加字段 + Has）| +20 |
| `response.go`（Reasoning 方法 + OutputText 简化）| +10 / -25 净 -15 |
| `response_accumulator.go` | +1 / -10 净 -9 |
| `usage.go` | +10 |
| `reasoning_cleaner.go` 文件改名 + 类型改名 | ±0 净（机械替换） |
| `parser.go` 调用改名 | ±0 净 |
| `models/anthropic/metadata.go`（新文件，2 const + 2 helper）| +30 |
| Anthropic `chat.go`（buildAssistantMsg 改动）| +25 / -75 净 -50 |
| OpenAI adapter | ±0（仅一处 metadata key 改字段名）|
| 测试重写 | +60 / -150 净 -90 |
| **合计** | **~+156 / -420，净 -264** |

🎯 **比 v1 的 +210 净增更轻**——因为 v0 的多 Result 拆分代码反而被简化掉了。chat 包减少更多（-160），而 anthropic 包小幅增长（+30 metadata.go），总账等价。

---

## 8. 与备选方案的最终对比

| 方案 | 内部一致性 | 多轮回放 | 表达力 | 改动量 | 总评 |
|------|----------|---------|-------|-------|------|
| v0（双模式） | ❌ leaky | ⚠️ 走 metadata key | – | – | ❌ 替代目标 |
| A（全部多 Result） | ✅ | DeepSeek 强行拆，破坏不回传语义 | 中 | 中 | ❌ |
| v1（ReasoningContent struct）| ✅ | ✅ 完整 | 高（多 block + 多 type）| 净 +210 | 🟡 重 |
| **v2（langchain4j 风格 + reasoning 命名）** | ✅ | ✅ 单 block 场景完整覆盖 | 中（多 block 损失）| 净 -264 | ✅ **采纳** |

---

## 9. 与 langchain4j / Spring AI 的对比

### 9.1 langchain4j AiMessage（参照原型）

```java
public class AiMessage implements ChatMessage {
    private final String text;
    private final String thinking;                       // ← 单 String，命名 thinking
    private final List<ToolExecutionRequest> toolExecutionRequests;
    private final Map<String, Object> attributes;        // ← signatures 等
}

@Experimental
public String thinking() { return thinking; }

@Experimental
public Builder thinking(String thinking) { ... }
```

Lynx v2 的形态对应、命名替换：

| langchain4j | Lynx v2 |
|-------------|---------|
| `String thinking` | `Reasoning string`（命名按行业收敛词替换）|
| `Map<String, Object> attributes` | `Metadata map[string]any` |
| `@Experimental since 1.2.0` | （Lynx 没正式 release，不需要 experimental 标记）|

**形态差异**：
- langchain4j 把 attributes 当**通用 provider extension 通道**——不只装 reasoning signature，还装 `generated_images` 等其他东西。Lynx 的 `Metadata` map 已经是同一角色，复用即可。
- langchain4j 没暴露 `ReasoningTokens`——Lynx 加这个字段，OpenAI o-series 计费需求。

### 9.2 Spring AI P0-1（拒绝采用）

Spring AI 提案的 `ReasoningContent` record 太重：4 字段 + 4 type enum。优势是表达力强（多 block + 多 type 显式），但复杂度对应 Java record 是免费的、对 Go 不是。**Lynx 用 v2 等于站在更后期、langchain4j 已经实证可行的简化设计上**，命名上比 langchain4j 更进一步——选了行业正在收敛的 "reasoning"。

### 9.3 三方对照表

| 维度 | Spring AI 现状 | Spring AI P0-1 提案 | langchain4j 现状 | **Lynx v2** |
|------|--------------|------------------|----------------|------------|
| AssistantMessage 字段 | content 单字段 | + List\<ReasoningContent\> | text + thinking | **Text + Reasoning** |
| 类型化 reasoning struct | – | record 4 字段 | – | – |
| signature 位置 | properties Map | record 字段 | attributes Map | **Metadata Map** |
| 多 block 表达 | 多 Generation | List | 单 String 拼接 | **单 String 拼接** |
| Usage.reasoningTokens | ❌ | ✅ | ❌ | **✅** |
| Inline `<think>` 清洗 | ✅ ThinkingTagCleaner | ✅ | – | ✅ ReasoningTagCleaner |
| 业务侧 API | 遍历 generations + properties key | `getReasoning()` | `aiMessage.thinking()` | **`msg.Reasoning` 字段** |
| 命名 | thinking + reasoning 混用 | reasoning | thinking | **reasoning（统一）** |

---

## 10. 决策记录

### 10.1 命名 Thinking vs Reasoning
**Reasoning**。industry vocabulary 已经收敛到 reasoning（API 字段一致 + 品类名 + Spring AI 提案选择），langchain4j 选 thinking 是因为他们落地早、跟随 Anthropic。Lynx 还在迭代期，直接走收敛终态。

### 10.2 多 reasoning block 损失是否可接受
✅ **接受**。`interleaved-thinking` beta 不在初版范围；标准 extended thinking 一次 1 个 block，无损。

### 10.3 signature 放 Metadata 还是第一类字段
**Metadata**。理由：
- 用户从不直接读 signature
- well-known key + 适配器封装 = 没有真正的 magic key 暴露
- 跨 provider signature 形态不同（Anthropic string、Google bytes）——分别用前缀 key 比强行归一更清晰

### 10.4 是否保留 v0 的 marker 常量做兼容
**不保留**。Lynx 没正式 release，无外部用户，零兼容包袱。

### 10.5 是否同时把已 push 的 3 个 commit force-push 重做
**不**。在分支上加新 commit `refactor: switch to single-string reasoning design (v2)` 演化过去。保留决策路径有利于评审，反正是个人 feature 分支无人协作。

### 10.6 ReasoningTokens 是否落地
**是**。零成本，OpenAI o-series 计费的实质需求。

### 10.7 ChatOptions 是否加通用 reasoning 入参
**否**。provider-specific options via Extra 已经够用，跨 provider 强行归一没好处。

### 10.8 是否自动剥离 inline `<think>` 进 Reasoning 字段
**否**。显式优于隐式，需要时后续 opt-in。

### 10.9 ReasoningTagCleaner 是否一并 rename
**是**。用户确认"现有代码不必考虑（疯狂迭代期）"，全包 rename 保持命名一致。默认 pattern 不变。

### 10.10 Provider-specific metadata key 放哪个包
**各自的 provider 包**（`models/anthropic/metadata.go` / 未来 `models/google/metadata.go`），不放 chat 包。理由：
- chat 包是抽象层，不应该有 "anthropic" / "google" 这种 provider 名称
- 新增 provider 时不需要改 core
- 同 provider 内的 key + helper 高度内聚，命名更短（`MetaReasoningSignature` 在 anthropic 包内一目了然，无需 `MetaAnthropic*` 前缀）
- key 字符串值仍然带 `lynx:chat:<provider>:<concept>` 前缀，避免序列化时跨 provider 冲突

---

## 11. 落地路线

按依赖顺序：

**Step A**（核心抽象，~2-3 小时）：
1. `core/model/chat/thinking.go` 整体删除（v0 常量/访问器全部下线）
2. `core/model/chat/message.go` 加 `Reasoning` 字段 + `HasReasoning()`
3. `core/model/chat/response.go` 加 `Response.Reasoning()`，简化 `OutputText()`
4. `core/model/chat/response_accumulator.go` 加 `Reasoning` 拼接，删 MetaReasoningContent 特例
5. `core/model/usage.go` 加 `ReasoningTokens`
6. `core/model/chat/thinking_cleaner.go` 重命名为 `reasoning_cleaner.go`，类型/函数同步 rename
7. `core/model/chat/parser.go` 更新 `CleanThinkingTags` → `CleanReasoningTags` 调用
8. `core/model/chat/thinking_test.go` 删除；新写 `core/model/chat/message_reasoning_test.go`（围绕新字段）
9. `core/model/chat/thinking_cleaner_test.go` 重命名为 `reasoning_cleaner_test.go`
10. `go build` + `go test` 验证 `core/`，确认 `grep -r "anthropic\|openai\|google" core/model/` 零命中

**Step B**（Anthropic 适配器，~2 小时）：
11. **新建** `models/anthropic/metadata.go`：定义 `MetaReasoningSignature` / `MetaRedactedReasoning` 常量 + 包内 `reasoningSignature` / `redactedReasoning` helper
12. `models/anthropic/chat.go` 响应路径：单 Result + 写 Reasoning + Metadata（用本包内常量）
13. 请求路径：从 Reasoning + Metadata 重建 Block；处理 OMITTED 场景
14. 验证 build

**Step C**（OpenAI 适配器，~30 分钟）：
15. `models/openai/chat.go` reasoning_content 改写 Reasoning 字段
16. 抽 ReasoningTokens 进 Usage（如 SDK 暴露则做、不暴露留 TODO）
17. 验证 build

**Step D**（文档与提交，~1 小时）：
18. 更新 `SPRING_AI_THINKING_ARCHITECTURE.md` §9
19. 更新 `SPRING_AI_CAPABILITY_GAPS.md` §1.3
20. 拆分实现 commit（建议 3 个：core abstraction → anthropic（含新 metadata.go） → openai+docs+rename）
21. push

总计 ~半个工作日。

---

## 12. 验收标准

实现完成后必须满足：

1. ✅ 旧符号 `MetaIsThought` / `IsThoughtMessage` / `MetaReasoningContent` / `ReasoningContent`（旧函数版本）/ `ThinkingTagCleaner` / `MetaThinkingSignature` 等在代码中**完全不存在**（grep 零命中）
2. ✅ **`grep -r "anthropic\|openai\|google" core/` 零命中**——core 包对 provider 完全无知
3. ✅ `AssistantMessage.Reasoning` 是所有 provider reasoning 内容的**唯一**字段入口
4. ✅ 业务侧拿 reasoning 一行：`resp.Result().AssistantMessage.Reasoning`
5. ✅ 跨 provider 无差别：`resp.Reasoning()` 工作于 Anthropic / OpenAI / DeepSeek 一致
6. ✅ Anthropic 多轮（reasoning + tool_use）通过 signature 重建测试
7. ✅ DeepSeek-R1 reasoning 不回传到下一轮
8. ✅ `Response.Results` 在 Anthropic 响应里恢复为 1 个（无 reasoning 拆分）
9. ✅ `go test ./...` 全绿
10. ✅ 文档与代码一致

---

## 13. 一句话定档

> **Reasoning 是和 Text 平级的 String 字段，不是 list、不是 struct、不是 metadata key。Provider-specific signature 走 Metadata map（key 常量定义在 provider 包内，字符串值带 `lynx:chat:<provider>:<concept>` 前缀避免冲突）。`msg.Reasoning` 一行拿全部，跨 provider 行为一致。core 包对 provider 完全无知，新增 provider 不需要改 chat 抽象层。命名采用业界正在收敛的 "reasoning"——而不是 langchain4j 选择 thinking 时（早期跟随 Anthropic）那一刻的状态。**

---

*配套阅读*：
- 当前实现痛点：`SPRING_AI_THINKING_ARCHITECTURE.md`（v0 描述）
- Spring AI 架构剖析：`/Users/tangerg/Desktop/spring-ai/docs/spring-ai-architecture/04-reasoning-thinking.md`
- Spring AI P0-1 ReasoningContent 提案（已**拒绝**用于 Lynx）：`/Users/tangerg/Desktop/spring-ai/docs/spring-ai-architecture/05-optimization-suggestions.md`
- langchain4j AiMessage 形态参照：https://github.com/langchain4j/langchain4j/blob/main/langchain4j-core/src/main/java/dev/langchain4j/data/message/AiMessage.java
