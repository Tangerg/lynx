# Message Parts 设计 —— 8 家对比 + lynx 最简 MVP 方案

> **基线**
> - lynx HEAD `a137f40`（branch `feat/message-parts-design-v3`，2026-05-18）
> - 对比对象：Spring AI / langchain4j / eino / trpc-agent-go / adk-go / Vercel AI SDK / TanStack AI / lynx
>
> **结论先行**：lynx 走最简 MVP——
>
> 1. `AssistantMessage` 内部 flat fields 改为 `Parts []OutputPart`
> 2. **第一版只 3 种 Part**：`TextPart` / `ReasoningPart` / `ToolCallPart`
> 3. **流式 = delta**——`Model.Stream` 每次 yield 携带"刚到达的 part 增量"；consumer 需要 snapshot 时用 `chat.Accumulator` 自己组装
> 4. **每个 Part 自实现 `appendDelta`**——同类型+同 identity 即 in-place 合并；Accumulator 完全类型无关
> 5. **UserMessage / SystemMessage / ToolMessage 是普通 struct**——只有 AssistantMessage 用 Part 抽象
> 6. **ToolMiddleware 放中间件链最里层**——多 turn 完全封装在中间件内部，外层 consumer 只看到一条连续 delta 流
> 7. 其它 Part 类型（Media / Source / File / Approval / Custom / ToolError）**全部 P1+ 再加**，加的时候 Accumulator 不动一行
>
> **API 表面新增类型数：3 个 Part struct + 1 个 OutputPart interface**。比 Vercel/TanStack 都简洁一个数量级。
>
> **前提**：lynx 仍在开发迭代期，可以做破坏性调整，不考虑兼容旧字段。

---

## 0. TL;DR

**问题**：现代 LLM 在单 turn 内产出"text → tool_use → text → tool_use → text"有序混排（Claude / Gemini / OpenAI Responses）。lynx 现在的 `AssistantMessage { Text; Reasoning; ToolCalls[] }` 把这三类内容打平，ordering 丢失。

**8 家观察**：

- 6 家把 Anthropic content blocks 按类型打平（Spring AI / langchain4j / eino / trpc-agent-go / lynx 当前）—— industry pattern
- 3 家有完整 Parts 模型（adk-go via `genai.Part`、Vercel via `ContentPart`、TanStack via `UIMessage.parts`）
- 2 家做到 vendor-neutral + 完整 ordering（Vercel + TanStack）
- adk-go 的 ordering 是绑死 Google genai SDK 拿到的（牺牲多 vendor）

**lynx 路线（4 步）**：

| 步骤 | 内容 |
|---|---|
| 1 | `AssistantMessage` 内部从 flat fields 改为 `Parts []OutputPart`；其它 message 类型保持简单 struct |
| 2 | OutputPart **第一版仅 3 种**：TextPart / ReasoningPart / ToolCallPart |
| 3 | `Model.Stream` 签名不变（`iter.Seq2[*Response, error]`），但语义改为 **delta**：每次 yield 携带刚到的增量 part |
| 4 | 每个 Part 自实现 `appendDelta`；`chat.Accumulator` 仅 ~15 行；多 turn 由 ToolMiddleware 完全封装 |

---

## 1. 问题陈述

### 1.1 单 turn 内的有序混排

Anthropic Claude（thinking + tool_use 交错）的 wire 形态：

```json
{
  "content": [
    {"type": "thinking", "thinking": "用户想查天气和日历，先查天气..."},
    {"type": "text", "text": "好的，我先查一下天气："},
    {"type": "tool_use", "id": "tu_1", "name": "weather", "input": {...}},
    {"type": "text", "text": "然后看看日历："},
    {"type": "tool_use", "id": "tu_2", "name": "calendar", "input": {...}},
    {"type": "text", "text": "等结果回来再总结。"}
  ]
}
```

lynx 当前接收形态：

```go
type AssistantMessage struct {
    Text      string      // "好的...\n然后...\n等结果..."
    Reasoning string      // "用户想..."
    ToolCalls []ToolCall  // 平铺
}
```

→ UI 想还原原始顺序做不到。

### 1.2 tool 执行的 wire-protocol 约束

**LLM 不会"中途"暂停去执行 tool**：

```
            ┌─────────── 一个 assistant turn ────────────┐
模型生成：   text_a → tool_use_1 → text_b → tool_use_2 → text_c    stop_reason=tool_use
                                                                                  │
                                                                                  ▼
runtime：           ┌────── 执行 tool_1 + tool_2 ───────┐
                    │                                      │
                    ▼                                      ▼
                                                  ┌── tool_results 注入 ──┐
模型再生成（新 turn）：                            text_d → ...
                                                  └────────────────────┘
```

- Anthropic / OpenAI / Google 都遵循"一个 turn 内决定所有 tool calls → 完整结束 → runtime 执行 → 下一个 turn 继续"
- text_b 是模型在 emit tool_use_2 之前自己生成的过渡文字，**不代表 tool_1 已经返回**

### 1.3 跨 turn 的事件回放

lynx 当前 ToolMiddleware 跑 N 轮只返回最后一轮 Response，中间 turn 的解释性文本被吞掉。

---

## 2. 8 家对比矩阵

| 维度 | Spring AI | langchain4j | eino | trpc-agent-go | adk-go | **Vercel AI** | **TanStack AI** | **lynx 现状** | **lynx 目标 (v1)** |
|---|---|---|---|---|---|---|---|---|---|
| 单 Message Parts 有序 | ❌ | ❌ | ✅ multimodal only | ❌ | ✅ via `genai.Part` | ✅ 完整 | ✅ UIMessage.parts | ❌ | ✅ 3 种 part |
| Reasoning 入 Parts | ❌ | ❌ | ✅ | ❌ | ✅ Thought flag | ✅ | ✅ ThinkingPart | ⚠️ flat | ✅ ReasoningPart |
| Reasoning signature | ❓ | ✅ attributes | ✅ part 字段 | ⚠️ extensions | ✅ ThoughtSignature | ✅ providerMetadata | ✅ ThinkingPart.signature | ⚠️ Metadata | ✅ ReasoningPart.Signature |
| **text↔tool_call 有序交错** | ❌ | ❌ | ❌（承认）| ❌ | ✅ | ✅ | ✅ | ❌ | ✅ |
| tool-result 入 Parts | ❌ | ❌ | ❌ | ❌ | ✅ FunctionResponse | ✅ | ✅ | ❌ | ❌ 在 ToolMessage |
| tool-error 独立 Part | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | ⚠️ state="error" | ❌ | ⏳ P1+ |
| **流式形态** | snapshot (Flux) | delta callback | snapshot + Index | 混合 | delta + Partial | event stream | event stream | delta + accumulator | **delta + Accumulator helper** |
| **流式 API 类型数** | 1 | 1 | 1 | 1 + Delta | 1 + flag | ~25 events | ~22 events | 1 | **1（Response 复用）** |
| HITL 工具确认 | ❌ | ❌ | ❌ | ❌ | ⚠️ Actions | ✅ 独立 part | ✅ 内嵌 | ❌ | ⏳ P1+ |
| Media / Source / File Part | ❌ | ❌ | ✅ Media | ⚠️ user only | ✅ all | ✅ all | ✅ all | ⚠️ Media flat | ⏳ P1+ |
| Custom Part namespace | ⚠️ | ⚠️ | ⚠️ Extra | ⚠️ | ❌ | ✅ `${prov}.${kind}` | ✅ AG-UI Custom | ⚠️ Metadata | ⏳ P1+ |
| 跨 turn intermediate 暴露 | ❌ | ✅ slice | ✅ event | ✅ channel | ✅ iter.Seq2 | ✅ 三视图 | ⚠️ AG-UI Step | ❌ | ✅ 通过 ToolMW 流 |
| 流式 wire 协议 | 内部 Reactor | 内部 callback | 内部 StreamReader | 内部 channel | 内部 iter.Seq2 | 自定义 | ✅ AG-UI 开放协议 | 内部 iter.Seq2 | 默认内部；可选 AG-UI |
| Multi-vendor provider | ✅ 20+ | ✅ 30+ | ✅ 多家 | ✅ 多家 | ❌ **Google only** | ✅ 20+ | ✅ 多家 | ✅ 22 chat | ✅ 22 chat |

**⏳ P1+** = 不在 v1 范围；未来有真实需求再加（章节 §10.2 会说明扩展策略）。

---

## 3. 各家深入分析

### 3.1 Spring AI（Java）

```java
public class AssistantMessage {
    String content;
    List<ToolCall> toolCalls;
    List<Media> media;
    Map<String, Object> properties;
}
```

Anthropic 适配器（`AnthropicChatModel.buildGenerations`）按类型打平：text → StringBuilder，tool_use → List<ToolCall>，thinking 单独 emit 成多 Generation。Tool 循环递归累计 conversationHistory 但只返回最后一个 ChatResponse。**评估**：跟 lynx 现状几乎一样的取舍。

### 3.2 langchain4j（Java）

```java
public class AiMessage {
    String text;
    String thinking;                                    // 1.2.0+
    List<ToolExecutionRequest> toolExecutionRequests;
    Map<String, Object> attributes;
}
```

亮点：流式 `onPartialThinking` callback 分通道；`ToolService.executeInferenceAndToolsLoop` 显式维护 `List<ChatResponse> intermediateResponses` —— **业务代码能拿到所有中间 turn**。**评估**：单 turn ordering 没做，但跨 turn intermediate 暴露做得最干净。

### 3.3 eino（Go，ByteDance）

```go
type Message struct {
    Role     RoleType
    Content  string
    UserInputMultiContent     []MessageInputPart
    AssistantGenMultiContent  []MessageOutputPart   // ★ 有序
    ToolCalls        []ToolCall                      // ★ 不在 Parts
    ReasoningContent string
}
type MessageStreamingMeta struct{ Index int }
```

亮点：把 multimodal + reasoning 做成有序 part；流式 Index 显式支持重组。**短板**：ToolCalls 仍在 Message 顶层不在 Parts 里 —— text↔tool ordering 没解决。eino 自己在 `react.go:166-179` 注释里承认默认实现不能很好处理 Claude。

### 3.4 trpc-agent-go（Go，Tencent）

```go
type Message struct {
    Role         Role
    Content      string
    ContentParts []ContentPart      // 仅 user 多模态侧
    ToolCalls    []ToolCall
    ReasoningContent string
}
```

ContentParts 比 eino 退一步——只有 user 输入侧。Response 直接照搬 OpenAI Choices/Delta。**Agent.Run** 返回 `<-chan *event.Event`，Event 含 `ParentInvocationID + Branch + Author + Tag + StateDelta` 完整 metadata。

### 3.5 adk-go（Go，Google ADK）

**不抽象，直接用 `genai.Content`**：

```go
type Part struct {
    Text             string
    FunctionCall     *FunctionCall            // ★ 工具调用作为 Part
    FunctionResponse *FunctionResponse        // ★ 工具结果作为 Part
    Thought          bool
    ThoughtSignature []byte
    InlineData       *Blob
    ...
}
```

**唯一真正解决 text↔tool 有序交错的设计**。**代价**：只支持 Google 系（gemini + apigee）。**Agent.Run**：`iter.Seq2[*session.Event, error]`，EventActions 完整集（StateDelta / ArtifactDelta / TransferToAgent / Escalate / SkipSummarization / RequestedToolConfirmations）。

### 3.6 Vercel AI SDK（TypeScript）

```typescript
export type ContentPart<TOOLS> =
  | { type: 'text';       text; providerMetadata? }
  | { type: 'custom';     kind: `${string}.${string}`; ... }
  | ReasoningOutput | ReasoningFileOutput
  | ({ type: 'source' } & Source)
  | { type: 'file';       file; ... }
  | ({ type: 'tool-call' }   & TypedToolCall   & { providerMetadata? })
  | ({ type: 'tool-result' } & TypedToolResult & { providerMetadata? })
  | ({ type: 'tool-error' }  & TypedToolError  & { providerMetadata? })
  | ToolApprovalRequestOutput
  | ToolApprovalResponseOutput;
```

结果数据结构暴露三视图（`content` aggregated / `steps` per-turn / `finalStep` shortcut）。流式 `fullStream`：25+ typed event，每种 part 有 start/delta/end 三段。**评估**：vendor-neutral + 完整 ordering，TypeScript 生态事实标准。

### 3.7 TanStack AI（TypeScript）

**Dual-tier Message**：

```typescript
// Tier 1: ModelMessage（贴近 wire）
interface ModelMessage { role; content; toolCalls?; thinking?; }

// Tier 2: UIMessage（UI 渲染）
interface UIMessage { id; role; parts: Array<MessagePart>; }
```

**ToolCallPart 内嵌状态机**（awaiting-input → input-streaming → input-complete → approval-requested → approval-responded）+ `approval` 字段内嵌。

**AG-UI 开放协议**：直接 import `@ag-ui/core`，22 种 typed event。MessagesSnapshot 让 client 掉线重连后能瞬时复原。

**ToolExecutionContext.emitCustomEvent**：tool 内部可发自定义 progress 事件。

### 3.8 lynx 现状

```go
type AssistantMessage struct {
    Text      string
    Reasoning string
    Media     []*media.Media
    ToolCalls []ToolCall
    Metadata  map[string]any
}
```

- ✅ Reasoning 是 first-class 字段
- ❌ Parts 模型缺失，ordering 丢
- ❌ 跨 turn intermediate 没暴露
- ✅ vendor-neutral（22 chat provider）
- ✅ Stream 已是 `iter.Seq2[*Response, error]` —— 现成 Go-idiomatic 形态

---

## 4. 取舍光谱

### 4.1 vendor-neutral × ordering

```
                  vendor-neutral                    vendor-locked
   ❌ ordering    Spring AI / langchain4j /          adk-go
                  trpc-agent-go / eino (一半)        ✅ ordering
                                                     ❌ multi-vendor

   ✅ ordering    ★ Vercel AI ★
                  ★ TanStack AI ★
                  ★ lynx 目标位置 ★
```

### 4.2 流式 snapshot × delta

```
                  snapshot                          delta
   Spring AI (Flux)  eino                 langchain4j callback
                     trpc-agent-go        adk-go (Partial)
                                          Vercel / TanStack (event stream)
                                          ★ lynx 目标 ★
```

**lynx 选 delta**：

- LLM wire 本来就是 delta；producer 无需维护"累积到当前"状态
- consumer 不需要 snapshot 时不用付组装代价
- snapshot 由 `chat.Accumulator` helper 派生（~15 行）；**严格更弱的原语用强原语派生很自然**
- consumer 想要 token-level UI（不需要 snapshot）的代码最短

### 4.3 单一 Message × dual-tier

```
                  单一 Message                       dual-tier
                  Spring AI / langchain4j /          TanStack AI
                  eino / trpc-agent-go /
                  adk-go / Vercel AI
                  ★ lynx 目标 ★
```

**lynx 选单一 Message**：单一类型简单；TanStack 双层增加调用方负担。

### 4.4 流式协议私有 × 开放

```
                  私有                                开放
   ★ lynx 默认 ★   Spring AI / langchain4j /          TanStack AI（AG-UI）
                   eino / trpc-agent-go /
                   adk-go / Vercel AI                 ★ lynx 可选 wire adapter ★
```

**lynx 默认私有**（`iter.Seq2[*Response, error]` Go-idiomatic）；HTTP/SSE 场景加 `wire/aguifmt` 子包做 AG-UI adapter，跟 CopilotKit / TanStack 等前端互通。

---

## 5. lynx 的设计决策

### 5.1 决策表

| 决策点 | 选择 | 理由 |
|---|---|---|
| **新增顶层类型数** | 1 个 interface（`OutputPart`）+ 3 个 struct（v1）| 真正的 minimal |
| **AssistantMessage 内部** | flat fields 全删，改 `Parts []OutputPart` | 唯一改动点 |
| **第一版 Part 种类** | **3 种**：`TextPart` / `ReasoningPart` / `ToolCallPart` | 覆盖 LLM 主路径 99% 场景；其它 part 类型有真实需求再加 |
| **Stream 签名** | `iter.Seq2[*Response, error]` 不变 | Go-idiomatic 形态保留 |
| **流式语义** | **delta**——每次 yield 是刚到的 part 增量 | LLM wire 本来就 delta；producer 无状态 |
| **同步 Call 实现** | 内部 = Stream + Accumulator.Build() | 一份 accumulator 服务两条路径 |
| **delta 合并机制** | 每个 Part struct 自实现 `appendDelta(OutputPart) bool` | type-local 责任；Accumulator 类型无关 |
| **`appendDelta` 同时充当 sealing** | unexported 方法 → 外部包无法实现 OutputPart | 不需要额外 sealing marker |
| **Accumulator 行数** | ~15 行（只调 interface 方法）| 加新 Part 类型零修改 |
| **UserMessage / SystemMessage / ToolMessage** | **普通 struct**（不用 Part 抽象）| 输入本来就是完整的；强行套 Part 是过度设计 |
| **Multi-turn 暴露** | ToolMiddleware 放中间件链最里层；外层只看到连续 delta 流 | turn 是中间件实现细节，不进 API |
| **ToolCallPart 状态机** | 3 个 state：`InputStreaming` / `InputComplete` / `Executed` | 静态 snapshot 也能反映 lifecycle；不含 approval state（HITL 是 P1+）|
| **Agent.Run API** | `iter.Seq2[*Event, error]`；Event 内嵌 `*chat.Response` | adk-go 路线 |
| **EventActions** | StateDelta + TransferToAgent + Escalate + SkipSummarization | adk-go 子集（v1 暂不含 ArtifactDelta / RequestedToolConfirmations）|
| **AG-UI 协议** | 可选 wire adapter（`wire/aguifmt`），核心不依赖 | TanStack 启发；保持核心 vendor-neutral |
| **OpenAI Responses API** | 独立 **P8 epic** 延后做 | 优先固化 Parts 模型，再接新 wire format |

### 5.2 命名约定

- `OutputPart` interface；`TextPart` / `ReasoningPart` / `ToolCallPart` 是 v1 实现
- Event 词留给 agent 层
- 不用 `StreamEvent` —— lynx 没有 event 概念
- 不用 `Block` / `Content` —— 用 `Part`

---

## 6. 详细数据模型

### 6.1 OutputPart 接口 + 3 种实现

```go
// core/model/chat/part.go
package chat

type PartKind string

const (
    PartKindText      PartKind = "text"
    PartKindReasoning PartKind = "reasoning"
    PartKindToolCall  PartKind = "tool_call"
)

// OutputPart is the sealed marker for one ordered chunk in
// AssistantMessage.Parts. v1 has exactly 3 implementations:
// TextPart / ReasoningPart / ToolCallPart. New part types
// (media / source / file / approval / custom / tool-error / ...) are
// deferred to P1+ — adding them requires no change to Accumulator.
type OutputPart interface {
    Kind() PartKind

    // appendDelta tries to merge a same-type delta into this part
    // IN-PLACE. Returns true if merged, false if delta belongs to a
    // new logical part (different type, or different identity such
    // as a new tool call ID).
    //
    // Unexported — doubles as the sealed-union mechanism: only
    // implementations inside the chat package can satisfy OutputPart.
    appendDelta(delta OutputPart) bool
}

// =========================================================================

// TextPart is plain assistant-emitted text — the most common case.
// Same-type deltas concatenate.
type TextPart struct {
    Text     string
    Metadata map[string]any
}

func (p *TextPart) Kind() PartKind { return PartKindText }

func (p *TextPart) appendDelta(d OutputPart) bool {
    o, ok := d.(*TextPart)
    if !ok {
        return false
    }
    p.Text += o.Text
    mergeMeta(&p.Metadata, o.Metadata)
    return true
}

// =========================================================================

// ReasoningPart carries visible chain-of-thought (Claude thinking,
// OpenAI o-series, DeepSeek-R1, Gemini thought parts). Signature
// preserves the vendor-opaque signature so reasoning blocks can
// round-trip in subsequent requests (Anthropic thought_signature etc).
//
// Anthropic "redacted thinking" — the SDK delivers a redacted
// placeholder text; for v1 we store the placeholder in Text and mark
// metadata["redacted"]=true. A dedicated ReasoningFilePart can be
// added in P1+ if call sites prove the metadata approach is too thin.
type ReasoningPart struct {
    Text      string
    Signature []byte
    Metadata  map[string]any
}

func (p *ReasoningPart) Kind() PartKind { return PartKindReasoning }

func (p *ReasoningPart) appendDelta(d OutputPart) bool {
    o, ok := d.(*ReasoningPart)
    if !ok {
        return false
    }
    p.Text += o.Text
    if len(o.Signature) > 0 {
        p.Signature = o.Signature
    }
    mergeMeta(&p.Metadata, o.Metadata)
    return true
}

// =========================================================================

// ToolCallPart is one tool invocation request. The same ID flows into
// the matching ToolResultPart / ToolErrorPart in the following
// ToolMessage so callers can pair them by ID.
//
// Delta semantics: an empty ID on a delta means "continues the
// previous ToolCallPart"; a different non-empty ID means "new
// ToolCallPart". This handles OpenAI Chat Completions' interleaved
// streaming where multiple tool_calls grow in parallel — the
// provider adapter buffers per-vendor-index and emits each tool_call
// as a contiguous run of deltas with the same ID.
//
// "Are the arguments complete?" is encoded by stream position
// (chunks = partial, accumulated final = complete) — no separate
// state field. "Was the tool executed?" is encoded by the presence
// of a matching ToolMessage in history.
type ToolCallPart struct {
    ID        string
    Name      string
    Arguments string         // JSON-encoded; grows with deltas
    Metadata  map[string]any
}

func (p *ToolCallPart) Kind() PartKind { return PartKindToolCall }

func (p *ToolCallPart) appendDelta(d OutputPart) bool {
    o, ok := d.(*ToolCallPart)
    if !ok {
        return false
    }
    if o.ID != "" && o.ID != p.ID {
        return false // new tool call
    }
    if p.Name == "" {
        p.Name = o.Name
    }
    p.Arguments += o.Arguments
    mergeMeta(&p.Metadata, o.Metadata)
    return true
}

// =========================================================================

// mergeMeta copies entries from src into *dst, allocating *dst lazily.
// Used by all appendDelta implementations.
func mergeMeta(dst *map[string]any, src map[string]any) {
    if len(src) == 0 {
        return
    }
    if *dst == nil {
        *dst = make(map[string]any, len(src))
    }
    for k, v := range src {
        (*dst)[k] = v
    }
}
```

**总计 ~90 行**。

### 6.2 AssistantMessage 改造

```go
// core/model/chat/message.go
type AssistantMessage struct {
    // Parts is the ordered list of content chunks the model emitted.
    // Text / reasoning / tool calls live here in the order produced.
    Parts []OutputPart `json:"parts"`

    // Metadata is message-level provider metadata. Use individual
    // OutputPart.Metadata for part-level metadata.
    Metadata map[string]any `json:"metadata,omitzero"`
}
```

**Flat fields 全部移除** —— `Text` / `Reasoning` / `Media` / `ToolCalls` 都不再存在。

### 6.3 其它 Message 类型（普通 struct，不用 Part）

```go
type UserMessage struct {
    // Text is the primary text content. Most user messages are
    // text-only; this is the fast path.
    Text string

    // Media holds attached images / audio / video / files in order.
    // The framework respects this slice order when building the wire
    // format for vendors that support multimodal input.
    Media []*media.Media

    Metadata map[string]any
}

type SystemMessage struct {
    // Text is the system prompt. System prompts are almost always
    // plain text — no Part abstraction needed.
    Text     string
    Metadata map[string]any
}

type ToolMessage struct {
    // Results carries successful tool outputs. ID matches a
    // ToolCallPart.ID in the prior assistant turn.
    Results []*ToolResult

    // Errors carries failed tool outputs. ID also matches a
    // ToolCallPart.ID.
    Errors  []*ToolError

    Metadata map[string]any
}

type ToolResult struct {
    ID       string  // matches ToolCallPart.ID
    Name     string
    Result   string  // canonical JSON
    Metadata map[string]any
}

type ToolError struct {
    ID       string
    Name     string
    Error    string
    Metadata map[string]any
}
```

**关键**：只有 AssistantMessage 用 Part 抽象——它是唯一需要"流式累积 + 有序 + 多类型 + sealed union"那套机器的消息类型。其它都是普通 struct，**简单输入用简单类型**。

### 6.4 派生 helper（替代 flat fields）

```go
// TextParts iterates all TextPart in order.
func (m *AssistantMessage) TextParts() iter.Seq[*TextPart] { ... }

// ToolCalls iterates all ToolCallPart in order.
func (m *AssistantMessage) ToolCalls() iter.Seq[*ToolCallPart] { ... }

// JoinedText concatenates all TextPart bodies (no separator).
func (m *AssistantMessage) JoinedText() string { ... }

// JoinedReasoning concatenates all ReasoningPart bodies.
func (m *AssistantMessage) JoinedReasoning() string { ... }

// HasToolCalls reports whether any ToolCallPart with State >=
// InputComplete exists.
func (m *AssistantMessage) HasToolCalls() bool { ... }
```

| 旧 | 新 |
|---|---|
| `msg.Text` | `msg.JoinedText()` |
| `msg.Reasoning` | `msg.JoinedReasoning()` |
| `for _, tc := range msg.ToolCalls` | `for tc := range msg.ToolCalls()` |

### 6.5 Accumulator —— 完全类型无关

```go
// core/model/chat/accumulator.go

// Accumulator merges streaming part deltas into the final ordered
// list. Same-type adjacent deltas are merged in-place via each part's
// appendDelta; type changes (or identity changes for tool calls)
// flush the in-flight part and start a new one.
//
// Implementation is completely type-agnostic: it never type-switches
// on concrete Part types. Adding new Part types (P1+) requires no
// change here.
type Accumulator struct {
    parts   []OutputPart
    current OutputPart // in-flight
}

// Add applies one delta. Typically called once per Part in each
// streaming Response.Result.AssistantMessage.Parts.
func (a *Accumulator) Add(delta OutputPart) {
    if a.current == nil {
        a.current = delta
        return
    }
    if a.current.appendDelta(delta) {
        return
    }
    a.parts = append(a.parts, a.current)
    a.current = delta
}

// Build flushes the in-flight part and returns the final slice.
// Safe to call multiple times; idempotent once flushed.
func (a *Accumulator) Build() []OutputPart {
    if a.current != nil {
        a.parts = append(a.parts, a.current)
        a.current = nil
    }
    return a.parts
}

// AccumulateStream drains a streaming iter.Seq2 into one final
// AssistantMessage. Used by Model.Call (internally) and by call
// sites that want a snapshot rather than token-by-token deltas.
func AccumulateStream(seq iter.Seq2[*Response, error]) (*Response, error) {
    var acc Accumulator
    var lastResp *Response
    for resp, err := range seq {
        if err != nil {
            return nil, err
        }
        if resp != nil && resp.Result != nil && resp.Result.AssistantMessage != nil {
            for _, p := range resp.Result.AssistantMessage.Parts {
                acc.Add(p)
            }
        }
        lastResp = resp
    }
    if lastResp == nil {
        return nil, nil
    }
    // Replace the last response's parts with the accumulated parts.
    final := *lastResp
    final.Result = &Result{
        AssistantMessage: &AssistantMessage{Parts: acc.Build()},
        Metadata:         lastResp.Result.Metadata,
    }
    return &final, nil
}
```

**~30 行**（含注释和 stream-drainer helper）。

### 6.6 Vendor Wire Projection

Parts 是 lynx 内部 source of truth；每家 vendor adapter 负责"Parts ↔ wire format"映射。**关键约定**：provider 适配器**必须 emit 非交错的 part delta**——同一逻辑 part 的 deltas 必须连续，不被其它类型的 delta 打断。

#### 6.6.1 双向 1:1 vendors

| Vendor | wire 格式 | 映射 |
|---|---|---|
| **Anthropic** | content_block_delta 按 index 顺序到达 | 直接 1:1 翻译；Anthropic 协议本身保证非交错 |
| **Google / Vertex AI** | Content.Parts 顺序 | 1:1 翻译 |
| **AWS Bedrock Converse** | ContentBlock 顺序 | 1:1 翻译 |
| **OpenAI Responses API（P8）** | output[] 有序 items | 1:1 翻译 |

#### 6.6.2 OpenAI Chat Completions adapter（特殊：内部 demux）

OpenAI Chat Completions 的 `delta.tool_calls[]` 可以并行 streaming——同一 chunk 内多个 tool_call 的 args 都在增长。适配器内部 buffer 按 `tool_calls[i].index` 分组，**emit 时一个 ID 的全部 deltas 完整连续输出，再 emit 下一个 ID**：

```
wire 到达:
  chunk1: { content: "hello" }                                      → TextPart{"hello"}
  chunk2: { content: " world" }                                     → TextPart{" world"}
  chunk3: { tool_calls: [{index:0, id:"tc_1", function:{name:"w", arguments:"{\"c"}}] }
  chunk4: { tool_calls: [{index:0, arguments:"ity\":"}] }
  chunk5: { tool_calls: [{index:1, id:"tc_2", function:{name:"cal", arguments:"{}"}}] }
  chunk6: { tool_calls: [{index:0, arguments:"\"北京\"}"}] }
  chunk7: { finish_reason: "tool_calls" }

适配器内部 buffer 并去交错，emit:
  TextPart{"hello"}
  TextPart{" world"}
  ToolCallPart{ID:"tc_1", Name:"w", Args:"{\"c"}
  ToolCallPart{ID:"tc_1", Args:"ity\":"}
  ToolCallPart{ID:"tc_1", Args:"\"北京\"}", State:InputComplete}
  ToolCallPart{ID:"tc_2", Name:"cal", Args:"{}", State:InputComplete}
```

→ **适配器吸收 vendor 协议的交错复杂性**；lynx 公开 API 只看到非交错的 Part delta 流，Accumulator 不需要 Index 字段、不需要 map lookup。

#### 6.6.3 写侧（Parts → wire）

```go
// Anthropic write — 1:1 双向
func partsToAnthropicBlocks(parts []chat.OutputPart) []anthropic.ContentBlock {
    blocks := make([]anthropic.ContentBlock, 0, len(parts))
    for _, p := range parts {
        switch p := p.(type) {
        case *chat.TextPart:
            blocks = append(blocks, anthropic.TextBlock{Text: p.Text})
        case *chat.ToolCallPart:
            blocks = append(blocks, anthropic.ToolUseBlock{
                ID: p.ID, Name: p.Name, Input: json.RawMessage(p.Arguments),
            })
        case *chat.ReasoningPart:
            blocks = append(blocks, anthropic.ThinkingBlock{
                Thinking: p.Text, Signature: p.Signature,
            })
        }
    }
    return blocks
}

// OpenAI Chat Completions write — Parts → flat 有损投影
func partsToOpenAIAssistantMsg(parts []chat.OutputPart) openai.AssistantMsg {
    var text strings.Builder
    var toolCalls []openai.ToolCall
    for _, p := range parts {
        switch p := p.(type) {
        case *chat.TextPart:
            text.WriteString(p.Text)
        case *chat.ToolCallPart:
            toolCalls = append(toolCalls, openai.ToolCall{...})
        case *chat.ReasoningPart:
            // DeepSeek-R1 等接受 reasoning_content；OpenAI 经典 API 丢弃
        }
    }
    return openai.AssistantMsg{Content: text.String(), ToolCalls: toolCalls}
}
```

---

## 7. 流式协议

### 7.1 Stream 签名（不变）

```go
type Model interface {
    Call(ctx context.Context, req *Request) (*Response, error)
    Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error]
}
```

**Stream 语义（delta）**：每次 yield 的 Response 携带"刚到达的 part 增量"。`Response.Result.AssistantMessage.Parts` 长度通常是 1（也可能 0 或 N，取决于 chunk 边界）。

**Call 语义（snapshot）**：返回的 Response 里 Parts 是完整 final Parts。**实现上 Call 内部 = `chat.AccumulateStream(model.Stream(...))`**——一份累加器服务两条路径。

### 7.2 Provider 内的累加器（producer 端可选）

provider 适配器可以选择**直接 emit delta**（推荐）或者**内部累加再 emit snapshot**：

```go
// 推荐路径：直接 emit delta（无状态）
func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
    return func(yield func(*chat.Response, error) bool) {
        apiStream := c.api.MessageStream(ctx, ...)
        defer apiStream.Close()

        for apiStream.Next() {
            event := apiStream.Current()
            delta := wireEventToPart(event)  // 翻译单个 wire chunk → 单个 Part delta
            if delta == nil { continue }
            resp := &chat.Response{
                Result: &chat.Result{
                    AssistantMessage: &chat.AssistantMessage{Parts: []chat.OutputPart{delta}},
                },
            }
            if !yield(resp, nil) { return }
        }
    }
}
```

OpenAI Chat Completions adapter 需要内部 buffer 做 demux（见 §6.6.2）；其他 vendor 直接 1:1 emit。

### 7.3 Consumer 用法

```go
// 用法 ① ── token UI（最常见）
for resp, err := range model.Stream(ctx, req) {
    if err != nil { return err }
    for _, p := range resp.Result.AssistantMessage.Parts {
        switch p := p.(type) {
        case *chat.TextPart:
            ui.PrintToken(p.Text)
        case *chat.ReasoningPart:
            ui.PrintReasoning(p.Text)
        case *chat.ToolCallPart:
            ui.ShowToolCallProgress(p.ID, p.Name, p.Arguments)
        }
    }
}

// 用法 ② ── 想要 snapshot
final, err := chat.AccumulateStream(model.Stream(ctx, req))
// final.Result.AssistantMessage.Parts 是组装好的完整 Parts

// 用法 ③ ── 想要 snapshot 也想要 token UI
var acc chat.Accumulator
for resp, err := range model.Stream(ctx, req) {
    if err != nil { return err }
    for _, p := range resp.Result.AssistantMessage.Parts {
        acc.Add(p)
        ui.RenderDelta(p)
    }
}
finalParts := acc.Build()
```

### 7.4 AG-UI wire adapter（可选）

HTTP/SSE 暴露给浏览器 / CopilotKit / 第三方 AG-UI client 时引入 wire 子包：

```go
// core/model/chat/wire/aguifmt/aguifmt.go
package aguifmt

// StreamToAGUI maps a delta Response stream to AG-UI events.
// Each Part delta maps to the corresponding AG-UI event type.
func StreamToAGUI(seq iter.Seq2[*chat.Response, error]) iter.Seq2[AGUIEvent, error]

// EncodeSSE wraps StreamToAGUI into an io.Writer for HTTP/SSE handlers.
func EncodeSSE(seq iter.Seq2[*chat.Response, error], w io.Writer) error
```

**核心不依赖 AG-UI**——业务想直接消费 lynx Response stream 不需要引入 AG-UI 依赖。

---

## 8. ToolMiddleware（放最里层）

### 8.1 中间件链结构

```
user app → [logger MW] → [retry MW] → [ToolMW] → bare Model
                                          ↑
                              innermost：唯一懂 tool 循环的
```

**外层中间件只看到一条连续 delta 流**，不知道里面跑了几个 turn。

### 8.2 ToolMiddleware Stream 实现（简化版）

```go
// core/model/chat/tool_middleware.go
type ToolMiddleware struct {
    Tools    []CallableTool
    MaxSteps int  // 默认 10
}

func (m *ToolMiddleware) Stream(ctx context.Context, next Model, req *Request) iter.Seq2[*Response, error] {
    return func(yield func(*Response, error) bool) {
        history := slices.Clone(req.Messages)

        for step := 0; step < m.MaxSteps; step++ {
            // ① 跑当前 turn 的流，原样 yield delta 给上层
            var turnAcc Accumulator
            var turnMeta *ResultMetadata
            for resp, err := range next.Stream(ctx, &Request{Messages: history, ...}) {
                if err != nil {
                    yield(nil, err)
                    return
                }
                if !yield(resp, nil) {
                    return
                }
                for _, p := range resp.Result.AssistantMessage.Parts {
                    turnAcc.Add(p)
                }
                if resp.Result.Metadata != nil {
                    turnMeta = resp.Result.Metadata
                }
            }

            // ② turn 结束。看是否要执行 tool。
            turnParts := turnAcc.Build()
            assistantMsg := &AssistantMessage{Parts: turnParts}
            history = append(history, assistantMsg)

            if turnMeta == nil || turnMeta.FinishReason != FinishReasonToolCalls {
                return // 最终答案，已 yield 过了
            }

            // ③ 执行 tools（可并行）
            results, errs := m.executeAll(ctx, assistantMsg)
            toolMsg := &ToolMessage{Results: results, Errors: errs}
            history = append(history, toolMsg)

            // ④ Yield tool 结果作为 delta（外层看到的就是连续流）
            // ToolMessage 不是 AssistantMessage，但 Response 类型可以容纳：
            toolResp := &Response{
                Result: &Result{
                    ToolMessage: toolMsg,
                    Metadata:    &ResultMetadata{FinishReason: FinishReasonStop},
                },
            }
            if !yield(toolResp, nil) {
                return
            }
        }
        yield(nil, fmt.Errorf("tool loop exceeded MaxSteps=%d", m.MaxSteps))
    }
}
```

### 8.3 Call 实现（复用 Stream + Accumulator）

```go
func (m *ToolMiddleware) Call(ctx context.Context, next Model, req *Request) (*Response, error) {
    return chat.AccumulateStream(m.Stream(ctx, next.Stream, req))
}
```

→ Call 是 Stream 的 trivial wrapper；**一份 ToolMiddleware 逻辑服务两条路径**。

### 8.4 Result 类型升级

```go
type Result struct {
    // 二者只有一个非 nil:
    AssistantMessage *AssistantMessage  // 模型输出（当前 chunk 是 assistant delta）
    ToolMessage      *ToolMessage        // runtime 注入的 tool 结果

    Metadata *ResultMetadata
}
```

**Discriminated union**：消费方根据非 nil 字段分支处理。

### 8.5 外层 consumer 视角

```go
// 外层完全不关心是几个 turn
for resp, err := range pipelineModel.Stream(ctx, req) {
    if err != nil { return err }
    switch {
    case resp.Result.AssistantMessage != nil:
        // assistant delta
        for _, p := range resp.Result.AssistantMessage.Parts {
            handleAssistantPart(p)
        }
    case resp.Result.ToolMessage != nil:
        // tool 执行结果（runtime 注入）
        handleToolResults(resp.Result.ToolMessage)
    }
}
```

UI 想知道"轮次边界"，监听 `Result.Metadata.FinishReason == FinishReasonToolCalls` 即可——但**多数 consumer 不需要知道**。

---

## 9. Agent 层 Event 流

### 9.1 Agent.Run 签名

```go
type Agent interface {
    Call(ctx context.Context, req *Request) (*Response, error)
    Run(ctx context.Context, req *Request) iter.Seq2[*Event, error]
    Info() Info
    SubAgents() []Agent
}
```

### 9.2 Event 结构

```go
type Event struct {
    Response *chat.Response  // 一帧 delta（assistant 或 tool message）

    ID                 string
    InvocationID       string
    ParentInvocationID string    // 嵌套 agent
    Branch             string    // "root.planner.executor" 点分路径
    Author             string    // 哪个 agent 发的
    Timestamp          time.Time

    Actions *EventActions

    Err error
}

type EventActions struct {
    StateDelta        map[string]any
    TransferToAgent   string
    Escalate          bool
    SkipSummarization bool
    // ArtifactDelta + RequestedToolConfirmations 推迟到 P1+
}

func (e *Event) IsFinal() bool { ... }
```

---

## 10. 实施路线

### 10.1 阶段

| 阶段 | 范围 | 工作量 | 是否破坏 API |
|---|---|---|---|
| **P0** — 3 种 Part + Accumulator | `core/model/chat/part.go`（3 个 struct + `appendDelta`）；`accumulator.go`（~30 行）；`message.go` 改造（AssistantMessage.Parts；UserMessage/SystemMessage/ToolMessage 简化）；helper（`JoinedText` / `ToolCalls()` iter）| 中（~250 LOC） | ✅ 破坏 |
| **P1** — Provider builder 改造 | 22 个 chat provider 的 `buildChatResponse` 改产 delta Parts；anthropic / google / vertexai / bedrock 走 1:1 native；openai Chat Completions 走有损投影 + 内部 demux；17 个 OpenAI-compat shim 自动继承 | 中（每 vendor ~25 LOC，主要在 anthropic / google / openai）| ✅ 破坏 |
| **P2** — Tool middleware 改造 | ToolMiddleware 改用 Parts；Stream 内部循环；Call = AccumulateStream(Stream) | 低（~150 LOC）| ✅ 破坏 |
| **P3** — Agent Event 流 | `agent.Agent.Run iter.Seq2[*Event, error]`；Event{Response, Actions}；保留 `Agent.Call` | 中（~400 LOC）| ⚠️ 破坏 agent API |
| **P4** — AG-UI wire adapter | `core/model/chat/wire/aguifmt`；`StreamToAGUI` / `EncodeSSE` | 中（~300 LOC，opt-in）| ❌ 不破坏 |
| **P5** — 文档 + 测试 | 改 `doc/REASONING.md` / `doc/MIDDLEWARE.md`；加 `doc/PARTS_RENDERING.md`；端到端 mock 测试 Claude 7 种 content block / OpenAI 流式 demux / 多 turn tool loop | 中 | — |
| **P8** — OpenAI Responses API adapter | **独立 epic**——`models/openai/chat_responses.go`；Parts ↔ output[] 1:1；与现有 `NewChatModel` 并存 | 中（~500 LOC + 测试）| ❌ 不破坏 |

**P0-P5 核心总工作量估计**：**~1100-1400 LOC + 测试**，~2 周（一个人）。

### 10.2 加新 Part 类型的策略（P1+）

当出现真实需求时（生成媒体 / grounding source / 文件输出 / HITL approval / vendor-specific custom），按以下步骤加：

1. 新建 struct，例如 `type MediaPart struct { ... }`
2. 实现 `Kind()` 和 `appendDelta()`（如果 atomic 不可合并，`appendDelta` 直接 `return false`）
3. 在 `PartKind` 常量加一项
4. 相关 provider adapter 加映射代码

**Accumulator 一行不动。其它 Part 类型不动。已有 consumer 代码用 type switch 时遇到不识别的 Part 类型可以 ignore 或 fallback 到 default case**。

→ **"Open/Closed" 在数据模型层得到完美体现**：对扩展开放（加新 Part），对修改关闭（Accumulator / 现有 Part 不动）。

### 10.3 OpenAI Responses API 单独成 P8

- Responses API 增量很大（新请求 schema / 新流式格式 / background mode / file_search / web_search 内置工具）
- 同时维护 ChatCompletions + Responses 等于翻倍测试矩阵
- Parts 模型未稳定就接 Responses 会反复重构
- **17 个 OpenAI-compat shim 永远走 Chat Completions**——vendor 本身不支持 ordering

---

## 11. 风险与待定项

### 11.1 已知风险

1. **OpenAI Chat Completions 反推 Parts 顺序的约定**
   read 路径上从 `{content, tool_calls}` 反推 Parts，约定"先 text 后 tool_calls"。lossy 转换。OpenAI Responses API（P8）天然有序，接入后这条约定自动作废。

2. **OpenAI Chat Completions 流式 tool_calls 交错**
   适配器内部 buffer 按 `tool_calls[i].index` 分组，**一个 ID 的全部 deltas 完整连续输出后再 emit 下一个 ID**。需要充分测试覆盖。

3. **Anthropic content_block_delta 的顺序保证**
   Anthropic 文档承诺 content blocks 按 `index` 顺序到达，但实际是否绝对一致需要测试验证。

### 11.2 待定项

- [ ] `ReasoningPart.Signature` 的合并规则：多 thinking block 是合并 signatures 还是只保留最后一个？v1 实现取最后一个非空。
- [ ] Anthropic redacted thinking：v1 走 `metadata["redacted"]=true`；如果真实场景多到需要独立类型，P1+ 再加 `ReasoningFilePart`。
- [ ] Streaming abort + resume 时的 part identity 一致性。

---

## 12. 验证清单

实施完成后需要的端到端验证：

| 用例 | 验证点 |
|---|---|
| Anthropic Claude 流式（thinking + 2 tool calls + text）| Accumulator 组装出 `[ReasoningPart, TextPart, ToolCallPart, TextPart, ToolCallPart, TextPart]` |
| OpenAI Chat Completions（text + 2 tool calls 并行 streaming）| 适配器 demux 出非交错 delta；Accumulator 组装出 `[TextPart, ToolCallPart_tc1, ToolCallPart_tc2]` |
| Gemini 2.0 Flash（thinking + functions）| Parts 含 ReasoningPart + ToolCallPart |
| ToolMiddleware 3-turn 循环 | 外层 consumer 收到一连续 delta 流；history 中含 3 个 AssistantMessage + 2 个 ToolMessage |
| `chat.AccumulateStream` | Call 内部用法 ≡ 外部 helper 调用结果 |
| token UI 直接消费 delta | 不调 Accumulator，逐 chunk 渲染 token |
| 加新 Part 类型不破坏 Accumulator | mock 一个 `FuturePart`，与现有 3 种 Part 混合流，Accumulator 行为不变 |
| Stream cancel mid-turn | iter.Seq2 提前 return，server-side 连接关闭 |
| Agent transfer | `Event.Actions.TransferToAgent = "executor"` → 下一个 Event 的 `Author = "executor"` |
| AG-UI wire round-trip | StreamToAGUI 派发的 events 经反向 unmarshal 后能还原 Parts |

---

## 13. 后续可扩展点（P1+）

按真实需求出现的优先级排序：

| 项 | 触发条件 |
|---|---|
| **MediaPart**（模型生成图像/音频/视频）| gpt-4o-audio / Gemini Imagen-via-chat 等"chat-modality 一并产出 media"的场景实际使用 |
| **SourcePart**（grounding / citations）| Perplexity Sonar / Anthropic web_search 等真实接入到 chat 包，且 message-level Metadata 难表达 |
| **FilePart**（生成文件）| Anthropic Files API 接入 |
| **ToolErrorPart**（独立 part type）| 当前 `ToolMessage.Errors` 满足不了的场景（如 tool 执行失败仍想保留在 assistant turn 内）|
| **ToolApprovalRequestPart / ToolApprovalResponsePart**（HITL）| 实际 HITL flow 上线 |
| **CustomPart**（vendor-specific 逃生通道）| 真出现某 vendor 的 block 无法映射到现有 3 种 part |
| **OpenAI Responses API adapter** | P8 单独 epic |
| **AG-UI Server transport** | HTTP endpoint 暴露 lynx agent |
| **Vercel UI Message Stream Protocol** 互通 | Next.js / useChat 接 lynx |
| **Trace 整合** | 每个 yield 自动产 OTel span（`gen_ai.*`），part-level child span |
| **MCP 协议事件桥接** | lynx Response stream 映射到 MCP progress notification |

**重点**：上述每一项都是"加新 Part 类型 + 新 vendor 映射 + 可选新 helper"，**核心数据模型（OutputPart interface + Accumulator）零修改**。

---

## 14. 一句话定档

**lynx v1 走最简 MVP：AssistantMessage 内部 flat fields 改为 `Parts []OutputPart`，只 3 种 Part（Text / Reasoning / ToolCall）；流式 = delta，每个 Part 自实现 `appendDelta`，Accumulator 完全类型无关；UserMessage / SystemMessage / ToolMessage 是普通 struct；ToolMiddleware 放中间件链最里层封装多 turn 复杂度。**

**API 表面新增类型**：1 个 OutputPart interface + 3 个 Part struct（+ 1 个 Accumulator helper struct）。比 Vercel/TanStack 都简洁一个数量级。

**8 家对比里 lynx 同时占据**：vendor-neutral（22 chat provider）+ 完整 ordering（Parts 模型）+ delta 流（in-process Go-idiomatic）+ 最简 API 表面（3 种 Part v1）+ Open/Closed（新 Part 类型扩展不动 Accumulator）。

**优先级**：P0 + P1 → 反超 4 家；P2 + P3 → 持平 Vercel + adk-go；P4 → 持平 TanStack；P8（OpenAI Responses API）独立 epic 延后做。

---

*文档结束。lynx HEAD `a137f40`，对比基线日期 2026-05-18。*
