# Message Parts 设计 —— 8 家对比 + lynx 最简改造方案

> **基线**
> - lynx HEAD `d291eff`（branch `feat/message-parts-design-v2`，2026-05-18）
> - 对比对象：Spring AI / langchain4j / eino / trpc-agent-go / adk-go / Vercel AI SDK / TanStack AI / lynx（共 8 家）
>
> **结论先行**：lynx 选最简形态 ——**Response / Result / AssistantMessage 三层结构不动，只把 AssistantMessage 内部 flat fields（Text / Reasoning / ToolCalls）改成 `Parts []OutputPart`**。流式直接累加 Parts；同步 Call 的 Response 等于流式最后一次 yield。**API 表面零新增类型**——比 Vercel / TanStack 都简洁。
>
> **重要前提**：lynx 仍在开发迭代期，可以做破坏性调整，不考虑兼容旧字段。

---

## 0. TL;DR

**问题**：现代 LLM 在单 turn 里产出"text → tool_use → text → tool_use → text"的有序混排（Claude / Gemini / OpenAI Responses API）。lynx 当前的 `AssistantMessage { Text string; Reasoning string; ToolCalls []ToolCall }` 把这三类内容**打平**，ordering 信息丢失。

**8 家观察**：

- **6 家把 Anthropic content blocks 按类型打平**（Spring AI / langchain4j / eino / trpc-agent-go / lynx 当前）—— 同一种 industry pattern
- **3 家有完整的 Parts 模型**（adk-go via `genai.Part`、Vercel AI SDK via `ContentPart`、TanStack AI via `UIMessage.parts`）
- **2 家做到 vendor-neutral + 完整 ordering**（Vercel + TanStack）—— TanStack 还多出 **dual-tier Message** + **AG-UI 开放协议**两个独创设计
- **adk-go** 的 ordering 是通过绑死 Google genai SDK 拿到的（牺牲多 vendor）

**lynx 路线**（4 步）：

1. **数据模型**：保留 `Response → Result → AssistantMessage` 三层结构；只把 `AssistantMessage` 内部的 flat fields 换成 `Parts []OutputPart`
2. **流式协议**：**`Model.Stream` 签名不动**——`iter.Seq2[*Response, error]`；每次 yield 携带**累积**到现在的 Parts；同步 `Call` 返回的 Response 等于流式最后一次 yield
3. **Tool 循环**：Tool 执行**永远在 turn 边界**（wire protocol 强制语义）；ToolMiddleware 不引入新类型，turn 边界靠 `Result.Metadata.FinishReason` + `AssistantMessage` 指针身份判断
4. **Agent 层**：`agent.Agent.Run` 返回 `iter.Seq2[*Event, error]`，Event 内嵌 `*chat.Response` + adk-go 风格 EventActions

**API 表面新增类型**：**零**。所有新功能都落在 `OutputPart` 接口及其 11 个具体实现 + 一组 Parts 后处理 helper 函数。

---

## 1. 问题陈述

### 1.1 单 turn 内的有序混排

Anthropic Claude（thinking + tool_use 交错）的实际 wire 形态：

```json
{
  "content": [
    {"type": "thinking", "thinking": "用户想查天气和日历，先查天气..."},
    {"type": "text", "text": "好的，我先查一下天气："},
    {"type": "tool_use", "id": "tu_1", "name": "weather", "input": {"city": "北京"}},
    {"type": "text", "text": "然后看看日历："},
    {"type": "tool_use", "id": "tu_2", "name": "calendar", "input": {"date": "tomorrow"}},
    {"type": "text", "text": "等结果回来再总结。"}
  ]
}
```

lynx 当前接收形态：

```go
type AssistantMessage struct {
    Text      string      // "好的，我先查一下天气：\n然后看看日历：\n等结果回来再总结。"
    Reasoning string      // "用户想查天气和日历，先查天气..."
    ToolCalls []ToolCall  // [{ID:"tu_1", ...}, {ID:"tu_2", ...}]
}
```

→ **UI 想还原原始顺序做不到**。

### 1.2 tool 执行的 wire-protocol 约束

容易误解的关键点：**LLM 不会"中途"暂停去执行 tool**。

```
              ┌─────────── 一个 assistant turn ────────────┐
模型生成：   text_a → tool_use_1 → text_b → tool_use_2 → text_c    stop_reason = tool_use
                                                                                  │
                                                                                  ▼
agent runtime：         ┌────── 执行 tool_1 + tool_2（并行）──────┐
                        │                                            │
                        ▼                                            ▼
                                                          ┌── tool_results 注入 ──┐
模型再生成（新 turn）：                                    text_d → ...
                                                          └────────────────────┘
```

- Anthropic / OpenAI / Google 都遵循"一个 turn 内决定所有 tool calls → 完整结束 → runtime 执行 → 下一个 turn 继续"
- text_b 是模型在 emit tool_use_2 之前自己生成的过渡文字，**不代表 tool_1 已经返回**
- text_c 是模型对自己即将停顿等待结果的预告

### 1.3 跨 turn 的事件回放

ToolMiddleware 跑 N 轮（assistant₁ → tool₁ → assistant₂ → tool₂ → assistant₃）。lynx 当前只返回**最后一轮 Response**，中间 turn 的解释性文本被吞掉，业务代码看不到完整轨迹。

---

## 2. 8 家对比矩阵

| 维度 | Spring AI | langchain4j | eino | trpc-agent-go | adk-go | **Vercel AI** | **TanStack AI** | **lynx 现状** |
|---|---|---|---|---|---|---|---|---|
| 单 Message Parts 有序 | ❌ | ❌ | ✅ multimodal only | ❌ | ✅ via `genai.Part` | ✅ **完整** | ✅ UIMessage.parts | ❌ |
| Dual-tier Message（wire vs UI）| ❌ | ❌ | ❌ | ❌ | ❌ | ❌ 单一 Message | ✅ ModelMessage + UIMessage | ❌ |
| Reasoning 入 Parts | ❌ | ❌ | ✅ | ❌ | ✅ Thought flag | ✅ | ✅ ThinkingPart | ⚠️ flat field |
| Reasoning signature 保留 | ❓ | ✅ attributes | ✅ part 字段 | ⚠️ extensions | ✅ ThoughtSignature | ✅ providerMetadata | ✅ ThinkingPart.signature | ⚠️ Metadata |
| **text↔tool_call 有序交错** | ❌ | ❌ | ❌（承认）| ❌ | ✅ | ✅ | ✅ | ❌ |
| tool-result 入 Parts | ❌ | ❌ | ❌ | ❌ | ✅ FunctionResponse | ✅ | ✅ ToolResultPart | ❌ |
| tool-error 独立 Part 类型 | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | ⚠️ ToolResultPart.state="error" | ❌ |
| **流式形态** | snapshot (Flux) | delta callback | **snapshot** + Index | 混合（Message + Delta）| delta（Partial flag）| event stream | event stream | delta+accumulator |
| **流式 API 类型数** | 1 | 1 | 1 | 1 + Delta | 1 + flag | ~25 events | ~22 events (AG-UI) | 1 |
| **HITL 工具确认** | ❌ | ❌ | ❌ | ❌ | ⚠️ Actions | ✅ 独立 part | ✅ ToolCallPart.approval 内嵌 | ❌ |
| 每 Part 携带 providerMetadata | ❌ | ✅ attributes | ✅ Extra | ❌ | ⚠️ | ✅ | ✅ 泛型 `<TMeta>` | ⚠️ msg-level only |
| 跨 turn intermediate 暴露 | ❌ | ✅ slice | ✅ event | ✅ channel | ✅ iter.Seq2 | ✅ steps + finalStep + content 三视图 | ⚠️ AG-UI Step + MessagesSnapshot | ❌ |
| 嵌套 agent 调用链 | ❌ | ❌ | ✅ RunPath | ✅ ParentInvocationID + Branch | ✅ Branch 点分串 | ⚠️ | ⚠️ runId + threadId | ⚠️ agent 内部 |
| StateDelta / ArtifactDelta | ❌ | ❌ | ⚠️ checkpoint | ✅ StateDelta | ✅ both | ⚠️ | ✅ AG-UI 事件 | ❌ |
| TransferToAgent / Escalate | ❌ | ❌ | ✅ action | ❌ | ✅ both | ⚠️ | ⚠️ Custom event | ⚠️ agent 内部 |
| Custom Part / Event namespace | ⚠️ | ⚠️ | ⚠️ | ⚠️ | ❌ | ✅ `${prov}.${kind}` | ✅ AG-UI Custom | ⚠️ Metadata |
| 流式 wire 协议 | 内部 Reactor | 内部 callback | 内部 StreamReader | 内部 channel | 内部 iter.Seq2 | 自定义 | ✅ **AG-UI 开放协议** | 内部 iter.Seq2 |
| Multi-vendor provider | ✅ 20+ | ✅ 30+ | ✅ 多家 | ✅ 多家 | ❌ **Google only** | ✅ 20+ | ✅ 多家 | ✅ 22 chat |

---

## 3. 各家深入分析

### 3.1 Spring AI（Java）

**Message**（`AssistantMessage.java`）：

```java
public class AssistantMessage {
    String content;                  // 所有文本拼一起
    List<ToolCall> toolCalls;        // 平铺数组
    List<Media> media;
    Map<String, Object> properties;
}
```

**Anthropic 适配器**（`AnthropicChatModel.buildGenerations`）按类型打平：text → StringBuilder，tool_use → List<ToolCall>，thinking 单独 emit 成多 Generation（唯一变通）。

**Tool 循环**：`OpenAiChatModel.internalCall` 递归调用，`conversationHistory` 累计 message，但**只返回最后一个 ChatResponse**。

**评估**：跟 lynx 现状几乎一样的取舍。Reasoning 上不如 lynx（lynx 有显式 Reasoning 字段）。

### 3.2 langchain4j（Java）

**Message**（`AiMessage.java`）：

```java
public class AiMessage {
    String text;
    String thinking;                                    // 1.2.0+
    List<ToolExecutionRequest> toolExecutionRequests;
    Map<String, Object> attributes;
}
```

**Anthropic 适配器**（`DefaultAnthropicClient.build`）同样打平：text/thinking 都用 `joining("\n")` 拼。

**亮点 ①**：流式有 `onPartialThinking` callback——partial thinking 与 partial text 分通道。

**亮点 ②**：Tool 循环（`ToolService.executeInferenceAndToolsLoop`）显式维护 `List<ChatResponse> intermediateResponses`，最终返回 `ToolServiceResult { intermediateResponses, finalResponse, toolExecutions, aggregateTokenUsage }`。**业务代码能拿到所有中间 turn**。

**评估**：单 turn ordering 没做，但跨 turn intermediate 暴露做得最干净。

### 3.3 eino（Go，ByteDance）

**Message**（`schema/message.go`）：

```go
type Message struct {
    Role     RoleType
    Content  string

    UserInputMultiContent     []MessageInputPart
    AssistantGenMultiContent  []MessageOutputPart   // ★ 有序 part

    ToolCalls        []ToolCall      // ★ 但 ToolCall 不在 Parts 里
    ReasoningContent string
    Extra            map[string]any
}

type MessageStreamingMeta struct {
    Index int  // "useful for reassembling multiple reasoning/content parts in correct order"
}
```

**亮点**：把 multimodal + reasoning 做成了有序 part list。流式 `Index` 显式支持重组。

**短板**：**ToolCalls 仍在 Message 顶层不在 Parts 里** —— text↔tool ordering 没解决。eino 自己在 `flow/agent/react/react.go:166-179` 的 `StreamToolCallChecker` 注释里直接承认了：

> The default implementation does not work well with Claude, which typically outputs tool calls after text content.

**流式形态**：`*schema.StreamReader[*Message]`——**snapshot 路线**（每帧拿到累积 Message）。

### 3.4 trpc-agent-go（Go，Tencent）

**Message**（`model/request.go`）：

```go
type Message struct {
    Role         Role
    Content      string
    ContentParts []ContentPart      // 仅 user 多模态侧（text/image/audio/file）
    ToolCalls    []ToolCall
    ReasoningContent string
}
```

**评估**：`ContentParts` 比 eino 还退一步——只有 user 输入侧，assistant 输出侧仍然 flat。Anthropic 适配器跟 Spring AI 一样打平。

**Response 形态**：直接照搬 OpenAI Choices/Delta（Choice 同时含 Message + Delta）。

**跨 turn**：`Agent.Run` 返回 `<-chan *event.Event`（Go-native channel），Event 含 `*model.Response + ParentInvocationID + Branch + Author + Tag + StateDelta + Extensions`。

### 3.5 adk-go（Go，Google Agent Development Kit）

**Message**：**不抽象，直接用 `genai.Content`**：

```go
type Part struct {
    Text             string
    FunctionCall     *FunctionCall            // ★ 工具调用作为 Part
    FunctionResponse *FunctionResponse        // ★ 工具结果作为 Part
    Thought          bool
    ThoughtSignature []byte
    InlineData       *Blob                    // image/audio/video
    FileData         *FileData
    ExecutableCode   *ExecutableCode
    CodeExecutionResult *CodeExecutionResult
    ...
}
```

**亮点**：**唯一真正解决 text↔tool 有序交错的设计**。

**代价**：**只支持 Google 系**（`gemini` + `apigee`）。

**Agent.Run**：`iter.Seq2[*session.Event, error]`——Go 1.23 原生迭代器，跟 lynx chat 包 stream API 同款。

**EventActions 完整集**：

```go
type EventActions struct {
    StateDelta                 map[string]any
    ArtifactDelta              map[string]int64
    TransferToAgent            string
    Escalate                   bool
    SkipSummarization          bool
    RequestedToolConfirmations map[string]ToolConfirmation
}
```

### 3.6 Vercel AI SDK（TypeScript）

**Message 模型**（`packages/ai/src/generate-text/content-part.ts`）：

```typescript
export type ContentPart<TOOLS> =
  | { type: 'text';       text: string; providerMetadata?: ProviderMetadata }
  | { type: 'custom';     kind: `${string}.${string}`; ... }
  | ReasoningOutput | ReasoningFileOutput
  | ({ type: 'source' } & Source)
  | { type: 'file';       file: GeneratedFile; ... }
  | ({ type: 'tool-call' }   & TypedToolCall   & { providerMetadata? })
  | ({ type: 'tool-result' } & TypedToolResult & { providerMetadata? })
  | ({ type: 'tool-error' }  & TypedToolError  & { providerMetadata? })
  | ToolApprovalRequestOutput
  | ToolApprovalResponseOutput;
```

**结果数据结构**——三视图：

```typescript
export interface GenerateTextResult<TOOLS, ...> {
  readonly content: Array<ContentPart<TOOLS>>;   // 全 step 聚合时间线
  readonly steps: Array<StepResult>;             // 每 turn 一个 StepResult
  readonly finalStep: StepResult;                // 最后一个 turn 快捷
  readonly text: string;                          // 派生 view
  readonly toolCalls: Array<TypedToolCall>;
  readonly toolResults: Array<TypedToolResult>;
  ...
}
```

**流式 `fullStream`**：`AsyncIterableStream<TextStreamPart>`，~25 种 typed event，每种 part 类型有 `start / delta / end` 三段。

**评估**：vendor-neutral + 完整 ordering，TypeScript 生态事实标准。

### 3.7 TanStack AI（TypeScript）

**Dual-tier Message**：

```typescript
// Tier 1: ModelMessage（贴近 wire）
export interface ModelMessage {
  role: 'user' | 'assistant' | 'tool'
  content: string | null | Array<ContentPart>      // ContentPart 仅多模态
  toolCalls?: Array<ToolCall>
  thinking?: Array<{ content: string; signature?: string }>
}

// Tier 2: UIMessage（UI 渲染）
export interface UIMessage {
  id: string
  role: 'system' | 'user' | 'assistant'
  parts: Array<MessagePart>                        // ★ 完整 union 含 tool-call/tool-result/thinking
}
```

**ToolCallPart 内嵌状态机**：

```typescript
export type ToolCallState =
  | 'awaiting-input' | 'input-streaming' | 'input-complete'
  | 'approval-requested' | 'approval-responded'

export interface ToolCallPart {
  type: 'tool-call'
  id: string
  name: string
  arguments: string
  state: ToolCallState                              // ★ part 自带 lifecycle
  approval?: { id, needsApproval, approved? }       // ★ HITL 内嵌
  output?: any                                       // ★ client tool 结果内嵌
}
```

**AG-UI 开放协议**：

```typescript
// 直接 import 自 @ag-ui/core
export type AGUIEvent =
  | RunStartedEvent | RunFinishedEvent | RunErrorEvent
  | TextMessageStartEvent | TextMessageContentEvent | TextMessageEndEvent
  | ToolCallStartEvent | ToolCallArgsEvent | ToolCallEndEvent | ToolCallResultEvent
  | StepStartedEvent | StepFinishedEvent
  | MessagesSnapshotEvent
  | StateSnapshotEvent | StateDeltaEvent
  | ReasoningStartEvent | ... | CustomEvent
```

**亮点**：跨框架兼容（CopilotKit 等 AG-UI 兼容前端可直接消费）；MessagesSnapshot 让 client 掉线重连后能瞬时复原状态。

**ToolExecutionContext.emitCustomEvent**：tool 执行内部可发自定义 progress 事件（Vercel / adk-go 都没有）。

### 3.8 lynx 现状

```go
// core/model/chat/message.go
type AssistantMessage struct {
    Text      string
    Reasoning string
    Media     []*media.Media
    ToolCalls []ToolCall
    Metadata  map[string]any
}
```

- ✅ Reasoning 是 first-class 字段（比 Spring AI / trpc-agent-go 强）
- ❌ Parts 模型缺失
- ❌ text↔tool ordering 丢
- ❌ 跨 turn intermediate 没暴露
- ✅ vendor-neutral（22 chat provider 都翻译成同一份 AssistantMessage）
- ✅ 流式 API 已经是 `iter.Seq2[*Response, error]` —— **现成的 Go-idiomatic 形态**

---

## 4. 取舍光谱

### 4.1 vendor-neutral × ordering

```
                  vendor-neutral                    vendor-locked
                  ←————————————————————————————————→
                                                                
   ❌ ordering    Spring AI                                       
                 langchain4j                       adk-go       
                 trpc-agent-go                    ✅ ordering    
                 eino (一半)                       ❌ multi-vendor
                                                                  
   ✅ ordering    ★ Vercel AI ★                                    
                 ★ TanStack AI ★                                  
                 ★ lynx 目标位置 ★                                  
```

### 4.2 流式 snapshot × delta × event

```
                  snapshot                  delta              event stream
                  ←——————————————————————————————————————————————→
                                                                                  
   Spring AI ↗   eino                    langchain4j         Vercel AI         
                 trpc-agent-go (混合)    adk-go (Partial)    TanStack AI       
                 ★ lynx 目标 ★                                                   
```

**lynx 选 snapshot**——理由：
- 在 Go 生态里 eino / trpc 都已经走 snapshot；adk-go 的 delta 是绑 genai SDK 的产物
- snapshot 是严格更强的原语：snapshot 流可推 delta，反之难
- in-process API 不存在 wire bandwidth 约束
- consumer 代码最短（`for resp := range stream { render(resp) }`）

### 4.3 单一 Message × dual-tier

```
                  单一 Message（render = wire）       dual-tier Message
                  ←————————————————————————————————→
                                                                                  
                  Spring AI / langchain4j /                                      
                  eino / trpc-agent-go /                                          
                  adk-go / Vercel AI                  TanStack AI                
                  ★ lynx 目标 ★                                                    
```

**lynx 选单一 Message**——TanStack 的双层切分语义更清晰，但增加调用方负担。lynx 用单一 `AssistantMessage` + Parts，由 vendor adapter 负责向 wire 投影。

### 4.4 流式协议私有 × 开放

```
                  私有 / SDK 内部                  开放 / 跨框架
                  ←——————————————————————————————————→
                                                                                  
   ★ lynx 默认 ★   Spring AI / langchain4j /        TanStack AI（AG-UI）          
                  eino / trpc-agent-go /                                          
                  adk-go / Vercel AI / lynx         ★ lynx 可选 wire adapter ★    
```

**lynx 默认私有**（`iter.Seq2[*Response, error]` Go-idiomatic API）；HTTP/SSE 场景下可加 `wire/aguifmt` 子包做 AG-UI 协议 adapter，跟 CopilotKit / TanStack 等前端互通。

---

## 5. lynx 的设计决策

### 5.1 决策表

| 决策点 | 选择 | 理由 |
|---|---|---|
| **新增顶层类型数** | **0** | 不引入 `StreamEvent` / `StreamingMeta` / `StepResult` 任何新顶层类型；保持现有 Response/Result/AssistantMessage 三层结构 |
| **AssistantMessage 内部** | flat fields 全删，改 `Parts []OutputPart` | 唯一改动点 |
| **Stream 签名** | `iter.Seq2[*Response, error]` 不变 | 现有签名已经是 Go-idiomatic；只需让 yield 的 Response 携带累积 Parts |
| **流式语义** | **snapshot**——每次 yield 是累积到现在的完整 Parts | 严格更强原语；consumer 代码最短 |
| **同步 Call 返回** | 与最后一次 yield 等价的 Response | "完整 Call 本身就是一个 Parts"——零差异 |
| **delta 信息** | **不放在 API 顶层**——通过 `chat.DiffParts(prev, curr)` helper 派生 | 99% consumer 用不到；用到的也只是一个 helper 函数 |
| **Part 是否含 ToolCall** | ✅ 含 | 真正解决 text↔tool ordering |
| **Part 是否含 ToolResult** | ✅ 含（放在 ToolMessage 不在 AssistantMessage）| 工具结果是 runtime 注入的，不是模型输出 |
| **Part 是否含 ToolError** | ✅ 含 | 工具失败结构化暴露 |
| **ToolCallPart 状态机** | ✅ `State` 字段 | TanStack 路线；静态 snapshot 也能反映 lifecycle |
| **HITL 工具确认** | ✅ 独立 part type（`ToolApprovalRequestPart` / `ToolApprovalResponsePart`）+ ToolCallPart.State 镜像 | 双面覆盖：消息历史友好（独立 part）+ UI 渲染直观（state 字段）|
| **Source / File / Custom Part** | ✅ 全部入 Parts | Vercel 路线 |
| **每 Part 携带 providerMetadata** | ✅ 携带 | Vercel 路线 |
| **跨 turn 暴露** | turn 边界靠 `Result.Metadata.FinishReason` + `AssistantMessage` 指针身份；可选 `chat.CollectTurns` helper | 不引入 StepResult；turn 是隐含概念 |
| **Agent.Run API** | `iter.Seq2[*Event, error]`；Event 内嵌 `*chat.Response` | adk-go 路线 |
| **EventActions** | StateDelta + ArtifactDelta + TransferToAgent + Escalate + SkipSummarization + RequestedToolConfirmations | adk-go 全集 |
| **AG-UI 协议** | ✅ 可选 wire adapter（`core/model/chat/wire/aguifmt` 子包），核心不依赖 | TanStack 启发；保持核心 vendor-neutral |
| **OpenAI Responses API** | ✅ 计划接入，**独立 P8 epic 延后做** | 优先固化 Parts 模型，再接新 wire format |

### 5.2 命名约定

- `Part` 一词作为基础词（不用 `Block` / `Content` 避免冲突）
- 输出侧统一 `OutputPart` interface；输入侧 `InputPart` interface
- 具体 Part 类型命名：`TextPart` / `ReasoningPart` / `ToolCallPart` / `ToolResultPart` / `ToolErrorPart` / `MediaPart` / `SourcePart` / `FilePart` / `ToolApprovalRequestPart` / `ToolApprovalResponsePart` / `CustomPart`
- 不用 `StreamEvent` —— lynx 没有 event 概念
- Event 词留给 agent 层

---

## 6. 详细数据模型

### 6.1 OutputPart 定义

```go
// core/model/chat/part.go
package chat

import "github.com/Tangerg/lynx/core/media"

// OutputPart is the marker interface for one ordered chunk in an
// assistant's reply. Concrete impls are sealed: TextPart /
// ReasoningPart / ToolCallPart / ToolResultPart / ToolErrorPart /
// MediaPart / SourcePart / FilePart / ToolApprovalRequestPart /
// ToolApprovalResponsePart / CustomPart.
//
// Parts of an AssistantMessage live in [AssistantMessage.Parts] in
// the order the model emitted them. Renderers and tool middlewares
// MUST respect this order.
type OutputPart interface {
    Kind() PartKind
    ProviderMetadata() map[string]any
    sealedOutputPart()  // unexported — keeps the union closed
}

type PartKind string

const (
    PartKindText                 PartKind = "text"
    PartKindReasoning            PartKind = "reasoning"
    PartKindToolCall             PartKind = "tool_call"
    PartKindToolResult           PartKind = "tool_result"
    PartKindToolError            PartKind = "tool_error"
    PartKindMedia                PartKind = "media"
    PartKindSource               PartKind = "source"
    PartKindFile                 PartKind = "file"
    PartKindToolApprovalRequest  PartKind = "tool_approval_request"
    PartKindToolApprovalResponse PartKind = "tool_approval_response"
    PartKindCustom               PartKind = "custom"
)

type TextPart struct {
    Text     string
    Metadata map[string]any
}

// ReasoningPart carries visible chain-of-thought. Signature preserves
// Anthropic thought_signature / OpenAI encrypted reasoning so reasoning
// blocks can round-trip in subsequent requests. Redacted=true means
// Text is the SDK's redacted placeholder.
type ReasoningPart struct {
    Text      string
    Signature []byte
    Redacted  bool
    Metadata  map[string]any
}

// ToolCallState tracks the lifecycle of a single ToolCallPart. Mirrored
// from streaming state so static snapshots also expose lifecycle.
type ToolCallState string

const (
    ToolCallStateAwaitingInput     ToolCallState = "awaiting_input"
    ToolCallStateInputStreaming    ToolCallState = "input_streaming"
    ToolCallStateInputComplete     ToolCallState = "input_complete"
    ToolCallStateApprovalRequested ToolCallState = "approval_requested"
    ToolCallStateApprovalResponded ToolCallState = "approval_responded"
    ToolCallStateExecuted          ToolCallState = "executed"
)

type ToolCallPart struct {
    ID        string
    Name      string
    Arguments string         // JSON-encoded
    State     ToolCallState  // lifecycle marker
    Metadata  map[string]any
}

// ToolResultPart belongs in ToolMessage.Results, NOT in
// AssistantMessage.Parts. It represents a tool execution result the
// runtime fed back into the conversation.
type ToolResultPart struct {
    ID       string
    Name     string
    Result   string  // canonical JSON; non-string outputs base64 + content-type via Metadata
    Metadata map[string]any
}

type ToolErrorPart struct {
    ID       string
    Name     string
    Error    string
    Metadata map[string]any
}

type MediaPart struct {
    Media    *media.Media
    Metadata map[string]any
}

type SourcePart struct {
    ID, URL, Title, Snippet string
    Metadata                map[string]any
}

type FilePart struct {
    Name, URI, MIME string
    Size            int64
    Metadata        map[string]any
}

type ToolApprovalRequestPart struct {
    RequestID, ToolName, Arguments, Reason string
    Metadata                               map[string]any
}

type ToolApprovalResponsePart struct {
    RequestID string
    Approved  bool
    Reason    string  // optional user rationale
    Metadata  map[string]any
}

// CustomPart is the typed escape hatch for vendor-specific blocks.
// Namespace+Kind scopes the payload to a vendor; consumers switch on
// (Namespace, Kind) to decode Payload.
type CustomPart struct {
    Namespace string  // e.g. "anthropic", "openai", "google"
    Kind      string  // e.g. "server_tool_use", "code_execution_result"
    Payload   any
    Metadata  map[string]any
}
```

### 6.2 AssistantMessage 改造（破坏性）

```go
// core/model/chat/message.go
type AssistantMessage struct {
    // Parts is the ordered list of content chunks the model emitted.
    // Text / reasoning / tool calls / media / etc. live here in the
    // order the model produced them.
    Parts []OutputPart `json:"parts"`

    // Metadata is message-level provider metadata. Use individual
    // OutputPart.Metadata for part-level metadata.
    Metadata map[string]any `json:"metadata,omitzero"`
}
```

**Flat fields 全部移除**——`Text` / `Reasoning` / `ToolCalls` / `Media` 都不再存在。调用方查文本/工具/推理改用 helper（§6.4）。

### 6.3 ToolMessage / UserMessage

```go
// ToolMessage carries the runtime-synthesized response back to the
// model for tool calls in the previous assistant turn. Each Result
// matches one ToolCallPart by ID.
type ToolMessage struct {
    Results  []ToolResultPart  // 成功结果
    Errors   []ToolErrorPart   // 失败结果（与 Results 配对，ID 对应同一 ToolCallPart）
    Metadata map[string]any
}

type UserMessage struct {
    Parts    []InputPart
    Metadata map[string]any
}

type InputPart interface {
    InputKind() PartKind
    sealedInputPart()
}
type UserTextPart  struct { Text string; Metadata map[string]any }
type UserMediaPart struct { Media *media.Media; Metadata map[string]any }
type UserFilePart  struct { URI, MIME, Name string; Metadata map[string]any }

type SystemMessage struct {
    Text     string  // 系统提示通常是纯文本
    Metadata map[string]any
}
```

### 6.4 派生 helper（替代 flat fields）

```go
// AssistantMessage 上的便利方法 —— 直接 walk Parts 派生。

// TextParts iterates all TextPart in this message, in order.
func (m *AssistantMessage) TextParts() iter.Seq[*TextPart] { ... }

// ToolCalls iterates all ToolCallPart in this message, in order.
func (m *AssistantMessage) ToolCalls() iter.Seq[*ToolCallPart] { ... }

// JoinedText concatenates all TextPart bodies (no separator).
// Use when downstream just needs "the final string the user sees".
func (m *AssistantMessage) JoinedText() string { ... }

// JoinedReasoning concatenates all ReasoningPart bodies.
func (m *AssistantMessage) JoinedReasoning() string { ... }

// HasToolCalls reports whether any ToolCallPart with State >=
// InputComplete exists.
func (m *AssistantMessage) HasToolCalls() bool { ... }
```

→ 调用方代码迁移：

| 旧 | 新 |
|---|---|
| `msg.Text` | `msg.JoinedText()` |
| `msg.Reasoning` | `msg.JoinedReasoning()` |
| `for _, tc := range msg.ToolCalls` | `for tc := range msg.ToolCalls()` |
| `len(msg.ToolCalls) > 0` | `msg.HasToolCalls()` |

### 6.5 Parts 后处理 helper（ordering 红利）

`Parts []OutputPart` 是切片，所有后处理都是纯切片操作——**这是 Vercel/TanStack 都没做成系统化 helper 的红利**：

```go
// core/model/chat/parts_normalize.go

// Normalize runs the standard pipeline:
//   1. drop empty Text/Reasoning parts
//   2. merge consecutive Text parts
//   3. merge consecutive Reasoning parts (signatures concatenated)
//   4. drop ToolCallPart with State < InputComplete (streaming residue)
// Preserves ordering of unchanged parts.
func Normalize(parts []OutputPart, opts ...NormalizeOpt) []OutputPart

type NormalizeOpt func(*normalizeConfig)
func WithKeepEmpty()       NormalizeOpt
func WithKeepPartialCalls() NormalizeOpt

// 单独 helper（按需组合）：
func MergeConsecutiveText(parts []OutputPart) []OutputPart
func MergeConsecutiveReasoning(parts []OutputPart) []OutputPart
func FilterEmpty(parts []OutputPart) []OutputPart
func FilterByKind(parts []OutputPart, kinds ...PartKind) []OutputPart

// PartDelta describes how parts changed between two snapshots.
// Used by wire adapters (AG-UI) and token-by-token UI consumers.
type PartDelta struct {
    Index     int
    Kind      PartKind
    NewPart   bool   // true if Index == len(prev) (a new part appeared)
    TextDelta string // populated for TextPart growth
    ReasoningDelta  string
    ToolInputDelta  string
    ToolCallID      string
}

// DiffParts computes the incremental changes between prev and curr.
// Always O(1) because:
//   - prev's first len(prev)-1 parts are unchanged (immutable shared ptrs)
//   - only the last part of prev may have grown
//   - curr may have one or more new parts appended
func DiffParts(prev, curr []OutputPart) []PartDelta

// PairToolCallsWithResults walks parts + a corresponding ToolMessage
// and pairs each ToolCallPart with its ToolResultPart / ToolErrorPart
// by ID, for downstream rendering.
type PartPair struct {
    Call   *ToolCallPart
    Result *ToolResultPart  // nil if errored
    Error  *ToolErrorPart   // nil if succeeded
}
func PairToolCallsWithResults(callParts []OutputPart, toolMsg *ToolMessage) []PartPair
```

**经典用法**：

```go
// 流式收到的 Parts 可能很碎（OpenAI token-by-token 模式下尤其）
finalParts := chat.Normalize(resp.Result.AssistantMessage.Parts)

// token UI 想要 delta：
prev := []OutputPart{}
for resp, err := range model.Stream(ctx, req) {
    if err != nil { return err }
    curr := resp.Result.AssistantMessage.Parts
    for _, d := range chat.DiffParts(prev, curr) {
        applyDeltaToUI(d)
    }
    prev = curr
}
```

### 6.6 Vendor Wire Projection

**关键 invariant**：Parts 是 lynx 内部 source of truth；每家 vendor adapter 负责"Parts ↔ wire format"双向映射。映射可能是 **1:1** 或 **有损投影**，取决于 vendor 协议本身的能力。

#### 6.6.1 双向 1:1 vendors（保留完整 ordering）

| Vendor | wire 格式 | 映射 |
|---|---|---|
| **Anthropic Messages API** | `content: []ContentBlock` 有序数组 | Parts ↔ ContentBlocks 双向 1:1，含 thinking_signature 等 round-trip |
| **Google Gemini / Vertex AI** | `Content.Parts []Part` 有序 | Parts ↔ genai.Part 双向 1:1 |
| **AWS Bedrock Converse** | `ContentBlock` 有序数组 | Parts ↔ ContentBlock 双向 1:1 |
| **OpenAI Responses API（未来 P8）** | `output: []OutputItem` 有序 | Parts ↔ output items 双向 1:1 |

→ 这 4 类立刻享受完整 ordering 红利。

**写 Anthropic 示例**：

```go
// models/anthropic/build_request.go
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
    return blocks  // ordering 完整保留
}
```

#### 6.6.2 有损投影 vendors（协议本身不支持 ordering）

| Vendor | wire 格式 | 映射方式 |
|---|---|---|
| **OpenAI Chat Completions API** | `content: string + tool_calls: []` 分离字段 | **Parts → flat 单向有损**；read 时按"text 优先于 tool_calls"约定反推 Parts |
| **17 个 OpenAI-compat shim** | 继承 OpenAI Chat Completions | 同上 |
| **Cohere v2** | `content + tool_calls` 分离 | 同上 |

**写 OpenAI Chat Completions 示例**（有损）：

```go
// models/openai/build_request.go
func partsToOpenAIAssistantMsg(parts []chat.OutputPart) openai.AssistantMsg {
    var text strings.Builder
    var toolCalls []openai.ToolCall
    for _, p := range parts {
        switch p := p.(type) {
        case *chat.TextPart:
            text.WriteString(p.Text)             // 文本全拼成一个 string
        case *chat.ToolCallPart:
            toolCalls = append(toolCalls, openai.ToolCall{...})
        case *chat.ReasoningPart:
            // DeepSeek-R1 等接受 reasoning_content 字段
            // OpenAI 经典 API 丢弃
        case *chat.ToolApprovalRequestPart, *chat.ToolApprovalResponsePart:
            // OpenAI Chat Completions 不支持 HITL；走 Custom Extra
        }
    }
    return openai.AssistantMsg{
        Content: text.String(), ToolCalls: toolCalls,
    }
}
```

**读 OpenAI Chat Completions 示例**（按约定反推 Parts）：

```go
// 流式累加完成后，整条消息长这样：
//   choice.message = { content: "hello world", tool_calls: [{id, name, args}, ...] }
//
// 反推 Parts（约定：text 先，tool_calls 后）：
func openAIMsgToParts(msg openai.AssistantMessage) []chat.OutputPart {
    var parts []chat.OutputPart
    if msg.Content != "" {
        parts = append(parts, &chat.TextPart{Text: msg.Content})
    }
    if msg.ReasoningContent != "" {  // DeepSeek-R1 等
        parts = append(parts, &chat.ReasoningPart{Text: msg.ReasoningContent})
    }
    for _, tc := range msg.ToolCalls {
        parts = append(parts, &chat.ToolCallPart{
            ID: tc.ID, Name: tc.Function.Name,
            Arguments: tc.Function.Arguments,
            State: chat.ToolCallStateInputComplete,
        })
    }
    return parts
}
```

#### 6.6.3 为什么这是好设计

| 角度 | 评价 |
|---|---|
| **lynx 内部代码** | 永远操作 Parts；ordering 完整 |
| **Anthropic / Google / Bedrock 用户** | 立刻享受完整 ordering 红利 |
| **OpenAI Chat Completions 用户** | wire 层丢序是 vendor 协议天生限制，lynx 无能为力；但 lynx 内部 Parts 仍然有序 |
| **OpenAI Responses API 接入后（P8）** | 同一份 Parts → 接 Responses API adapter → unlock ordering，旧 ChatCompletions adapter 不动 |
| **17 个 OpenAI-compat shim** | vendor 本身就不支持 ordering，lynx 自动继承 OpenAI Chat Completions adapter 的有损投影；零额外工作 |

---

## 7. 流式 = 累积 snapshot

### 7.1 Stream 签名（不变）

```go
type Model interface {
    Call(ctx context.Context, req *Request) (*Response, error)
    Stream(ctx context.Context, req *Request) iter.Seq2[*Response, error]
    ...
}
```

每次 yield 的 Response 携带**累积到当前的完整 AssistantMessage**——其中 `Parts` 是从 0 个 part 增长到 N 个 part 的过程中某一帧。

**完整 Call** 等价于 **流式最后一次 yield 的 Response**——零差异。

### 7.2 Provider 内累加器

每家 provider 的 `chat.go` 在 Stream 路径内部维护累加器：

```go
// models/anthropic/chat.go（简化）
func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
    return func(yield func(*chat.Response, error) bool) {
        apiStream := c.api.MessageStream(ctx, ...)
        defer apiStream.Close()

        acc := newPartAccumulator()   // ★ 持续累积的 part builder
        for apiStream.Next() {
            event := apiStream.Current()
            acc.Apply(event)           // ★ 内部 mutate accumulator state

            resp := acc.Snapshot()     // ★ 当前累积的完整 Response（fresh 指针）
            if !yield(resp, nil) { return }
        }
    }
}
```

**`acc.Snapshot()` 内部约定**：
- 返回 fresh `*Response`
- 内嵌的 `AssistantMessage.Parts` 是新的切片头
- 切片元素：早期已 finalized 的 part 是**共享指针**（immutable）
- **正在增长的最后一个 part**：copy-on-grow（每次替换最后一个指针，新 Part 含累积内容）
- → 每次 yield 仅 O(1) 额外分配

**共享指针的内存安全保证**：consumer 持有旧 Response 时，看到的 `Parts[i]` 内容**永远不会改变**（pointer 不变 = 内容不变）。

### 7.3 Anthropic 累加器示例

Anthropic stream events 映射到 part accumulator：

```go
// content_block_start { index: 0, content_block: { type: "thinking" } }
//   → acc.ensurePart(0, &ReasoningPart{})
//
// content_block_delta { index: 0, delta: { type: "thinking_delta", thinking: "..." } }
//   → acc.parts[0].(*ReasoningPart).Text += delta.thinking
//   → State 隐式 (Reasoning 没有状态机)
//
// content_block_start { index: 1, content_block: { type: "text" } }
//   → acc.ensurePart(1, &TextPart{})
//
// content_block_start { index: 2, content_block: { type: "tool_use", id, name } }
//   → acc.ensurePart(2, &ToolCallPart{ID, Name, State: AwaitingInput})
//
// content_block_delta { index: 2, delta: { type: "input_json_delta", partial_json: "..." } }
//   → acc.parts[2].(*ToolCallPart).Arguments += delta.partial_json
//   → State = InputStreaming
//
// content_block_stop { index: 2 }
//   → acc.parts[2].(*ToolCallPart).State = InputComplete
//
// message_delta { stop_reason: "tool_use", usage: {...} }
//   → acc.metadata.FinishReason = FinishReasonToolCalls
//   → acc.metadata.Usage = ...
```

### 7.4 Wire adapter 派生 events（可选）

不依赖 in-process API 的 wire 协议（AG-UI / Vercel UI Message Stream / SSE）走独立 adapter 包：

```go
// core/model/chat/wire/aguifmt/aguifmt.go
package aguifmt

import (
    "iter"
    "github.com/Tangerg/lynx/core/model/chat"
)

// StreamToAGUI converts a Go-native snapshot Response stream into an
// AG-UI delta event stream. The conversion is done by DiffParts on
// adjacent snapshots; each detected change emits the matching AG-UI
// event(s).
func StreamToAGUI(seq iter.Seq2[*chat.Response, error]) iter.Seq2[AGUIEvent, error] {
    return func(yield func(AGUIEvent, error) bool) {
        var prev []chat.OutputPart
        for resp, err := range seq {
            if err != nil { yield(nil, err); return }
            curr := resp.Result.AssistantMessage.Parts
            for _, delta := range chat.DiffParts(prev, curr) {
                for _, ev := range deltaToAGUIEvents(delta) {
                    if !yield(ev, nil) { return }
                }
            }
            prev = curr
        }
    }
}

// EncodeSSE wraps StreamToAGUI into an io.Writer for HTTP/SSE handlers.
func EncodeSSE(seq iter.Seq2[*chat.Response, error], w io.Writer) error { ... }
```

**关键决策**：
- `wire/aguifmt` 是独立 sub-package；core 不依赖 AG-UI
- 业务想直接消费 lynx Response stream，不需要引入 AG-UI 依赖
- 想跟 AG-UI 生态（CopilotKit / TanStack）互通的业务，import `aguifmt`
- diff 由 `chat.DiffParts` 完成；wire 层只负责把 PartDelta 翻译成 AG-UI event 类型

---

## 8. Tool 循环（turn 边界语义）

### 8.1 wire protocol 强制语义

**所有支持 tool use 的 LLM API 都遵循"turn 内决定 → 完整结束 → runtime 执行 → 下一个 turn"**：

| Vendor | turn 内 ordering | 下一轮 tool_results 注入方式 |
|---|---|---|
| Anthropic | ✅ content blocks 有序 | `role: user` message + `tool_result` content blocks |
| Google Gemini | ✅ Parts 有序 | `role: user` content + `function_response` Parts |
| Bedrock Converse | ✅ ContentBlock 有序 | `role: user` + `toolResult` blocks |
| OpenAI Chat Completions | ❌ flat（text + tool_calls 分离）| 每个 tool_call 一个 `role: tool` message |
| OpenAI Responses API（未来）| ✅ output[] 有序 | `function_call_output` items |

→ **tool 永远在 turn 结束后才执行**。"text → tool → text" 这种 interleaving 是同一个 turn 内的相对位置，**不代表 tool 已经返回**。

### 8.2 ToolMiddleware 简化设计

不引入 StepResult；逻辑就这么直白：

```go
// core/model/chat/tool_middleware.go
type ToolMiddleware struct {
    Tools     []CallableTool
    MaxSteps  int                                          // 默认 10
    OnStep    func(resp *Response)                         // 可选：每完成一个 turn 调一次
}

func (m *ToolMiddleware) Call(ctx context.Context, next Model, req *Request) (*Response, error) {
    history := slices.Clone(req.Messages)
    for step := 0; step < m.MaxSteps; step++ {
        resp, err := next.Call(ctx, &Request{Messages: history, ...})
        if err != nil { return nil, err }
        if m.OnStep != nil { m.OnStep(resp) }

        // turn 是否要执行 tool？
        if resp.Result.Metadata.FinishReason != FinishReasonToolCalls {
            return resp, nil  // 不需要——这就是最终答案
        }

        // 提取所有 ToolCallPart（必须 State == InputComplete）
        msg := resp.Result.AssistantMessage
        var calls []*ToolCallPart
        for tc := range msg.ToolCalls() {
            if tc.State == ToolCallStateInputComplete {
                calls = append(calls, tc)
            }
        }

        // 执行所有 tool（可并行）
        results, errs := m.executeAll(ctx, calls)

        // 把当前 assistant turn + tool 结果加进 history，进入下一轮
        history = append(history, msg)
        history = append(history, &ToolMessage{Results: results, Errors: errs})
    }
    return nil, fmt.Errorf("tool loop exceeded MaxSteps=%d", m.MaxSteps)
}
```

### 8.3 流式 ToolMiddleware

签名同样保持 `iter.Seq2[*Response, error]`：

```go
func (m *ToolMiddleware) Stream(ctx context.Context, next Model, req *Request) iter.Seq2[*Response, error] {
    return func(yield func(*Response, error) bool) {
        history := slices.Clone(req.Messages)
        for step := 0; step < m.MaxSteps; step++ {
            var finalResp *Response
            for resp, err := range next.Stream(ctx, &Request{Messages: history, ...}) {
                if err != nil { yield(nil, err); return }
                if !yield(resp, nil) { return }
                finalResp = resp  // 最后一次 yield 就是这个 turn 的最终累积状态
            }

            if finalResp.Result.Metadata.FinishReason != FinishReasonToolCalls {
                return  // 最终答案，已经 yield 过了
            }

            // 执行 tools + 进入下一轮
            msg := finalResp.Result.AssistantMessage
            results, errs := m.executeAll(ctx, msg)
            history = append(history, msg)
            history = append(history, &ToolMessage{Results: results, Errors: errs})
        }
        yield(nil, fmt.Errorf("tool loop exceeded MaxSteps=%d", m.MaxSteps))
    }
}
```

→ **流式从外部看**：consumer 收到一连串 yields。每个 yield 是某个 turn 内的累积 snapshot。当一个 turn 结束（FinishReason = ToolCalls），下一个 yield 的 `AssistantMessage` 是**新的指针**（不同 turn 的 fresh AssistantMessage）。

### 8.4 turn 边界检测（consumer 侧）

业务代码可以靠两个无歧义信号判别 turn 边界：

```go
var lastTurn *chat.AssistantMessage
for resp, err := range middleware.Stream(ctx, req) {
    if err != nil { return err }
    curr := resp.Result.AssistantMessage
    if curr != lastTurn {
        // 新 turn 开始
        startNewTurnInUI()
        lastTurn = curr
    }
    render(curr)
}
```

或用 helper：

```go
// chat.CollectTurns 阻塞跑完 stream，返回每个 turn 的 final snapshot 列表。
// 适合不关心 token-level 增长、只要"完整对话轨迹"的业务。
turns, err := chat.CollectTurns(middleware.Stream(ctx, req))
// turns = []*chat.Response{ turn1Final, turn2Final, ..., turnNFinal }
```

### 8.5 HITL 工具确认流程

当 ToolMiddleware 检测到工具被标记"需要确认"：

```
turn N: Model 输出 ToolCallPart(state=InputComplete)
        Middleware 检测 "weather" 工具 needs approval
        Middleware 把 ToolCallPart.State 改成 ApprovalRequested
        Middleware 同时往 Parts 追加 ToolApprovalRequestPart{RequestID, ToolName, Args}
        yield Response (FinishReason = ApprovalRequested)
        ↓
        【stream 暂停】
        ↓
Consumer 收集用户决定，构造新 Request（含 ToolApprovalResponsePart）→ 调 Stream
        ↓
turn N+1: Middleware 看到 ApprovalResponse → Approved=true 才执行 tool → 进入正常 turn 循环
```

→ **不需要新增 StreamEvent 类型**。HITL 完全靠 `FinishReason + ToolApprovalRequestPart in Parts` 表达。

---

## 9. Agent 层 Event 流

### 9.1 Agent.Run 签名

```go
// agent/agent.go
type Agent interface {
    // Call runs the agent and returns the final result.
    Call(ctx context.Context, req *Request) (*Response, error)

    // Run streams every event — including intermediate assistant turns,
    // tool executions, sub-agent transfers, state-delta changes.
    Run(ctx context.Context, req *Request) iter.Seq2[*Event, error]

    Info() Info
    SubAgents() []Agent
}
```

### 9.2 Event 结构（嵌 chat.Response）

```go
// agent/event.go
type Event struct {
    // Response is the chat-layer snapshot for this event. Nil for
    // pure action events (transfer/escalate without an LLM call).
    Response *chat.Response

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
    StateDelta                 map[string]any
    ArtifactDelta              map[string]int64
    TransferToAgent            string
    Escalate                   bool
    SkipSummarization          bool
    RequestedToolConfirmations map[string]*chat.ToolApprovalRequestPart
}

// IsFinalResponse reports whether this event is the agent's final answer.
func (e *Event) IsFinalResponse() bool { ... }
```

→ Event 直接复用 `*chat.Response`，**不需要 StepResult 中介**。

---

## 10. 实施路线

| 阶段 | 范围 | 工作量 | 是否破坏 API |
|---|---|---|---|
| **P0** — Part 模型 | `core/model/chat/part.go` 新增（11 种 part type）；`AssistantMessage` / `ToolMessage` / `UserMessage` 改造；helper（`JoinedText` / `ToolCalls()` iter）| 中（~350 LOC） | ✅ 破坏 |
| **P1** — Provider builder | 22 个 chat provider 的 `buildChatResponse` 改产 Parts：anthropic / google / vertexai / bedrock 走 1:1 native 映射；openai 走有损投影；17 个 OpenAI-compat shim 自动继承 openai adapter | 中（每 vendor ~50 LOC，主要工作在 anthropic / google / bedrock）| ✅ 破坏 |
| **P2** — 流式 accumulator | provider 内累加器改产 snapshot Response（不是 single-chunk delta）；调用方 API 形态不变（仍是 `iter.Seq2[*Response, error]`）| 低（~150 LOC + 测试）| ❌ 不破坏（外部 API 形态不变）|
| **P3** — Parts 后处理 helper | `Normalize` / `Merge*` / `Filter*` / `DiffParts` / `PairToolCallsWithResults` | 低（~200 LOC + 测试）| — |
| **P4** — Tool middleware 改造 | ToolMiddleware 改用 Parts；MaxSteps + OnStep 回调；HITL flow | 低（~150 LOC）| ✅ 破坏 ToolMiddleware API |
| **P5** — Agent Event 流 | `agent.Agent.Run iter.Seq2[*Event, error]`；Event{Response, Actions{StateDelta, ArtifactDelta, TransferToAgent, Escalate, SkipSummarization, RequestedToolConfirmations}}；保留 `Agent.Call` 同步入口 | 中-高（~500 LOC + agent 包重构）| ⚠️ 破坏 agent API |
| **P6** — AG-UI wire adapter | `core/model/chat/wire/aguifmt` 子包；`StreamToAGUI` / `EncodeSSE` / 反向 `UnmarshalEvent` | 中（~400 LOC + AG-UI 依赖仅子包内）| ❌ 不破坏（opt-in）|
| **P7** — 文档 + 测试 | 改 `doc/REASONING.md` / `doc/MIDDLEWARE.md`；加 `doc/PARTS_RENDERING.md`；端到端 mock 测试 Claude 7 种 content block / OpenAI 有损往返 / HITL flow | 中 | — |
| **P8** — OpenAI Responses API adapter | **独立 epic**——`models/openai/chat_responses.go`：新的 `NewResponsesChatModel`；Parts ↔ output[] 1:1 双向映射；流式支持；与现有 `NewChatModel` 并存 | 中-高（~500 LOC + 测试）| ❌ 不破坏（新建 adapter）|

**总工作量估计**：P0-P7 约 **~1200-1500 LOC + 测试**，~2-3 周；P8 单独 ~500 LOC，~1 周。

### 10.1 优先级建议

- **P0 + P1 + P3** 是核心 —— 做完已经反超 Spring AI / langchain4j / eino / trpc-agent-go 四家
- **P2 + P4 + P5** 持平 Vercel + adk-go
- **P6** 持平 TanStack，并打开"接入 AG-UI 生态前端"能力
- **P8** 是 OpenAI 阵营的现代化升级，**延后做**——不阻塞 P0-P7

### 10.2 为什么 OpenAI Responses API 单独成 P8

| 风险 | 影响 |
|---|---|
| Responses API 增量很大 | 新请求 schema / 新流式格式 / background mode / stored 模式 / file_search / web_search 内置工具 —— 任何一个单独都值得独立 epic |
| OpenAI 客户端 SDK 双轨 | openai-go v3 同时维护 ChatCompletions + Responses；lynx 同时支持俩 endpoint 等于翻倍测试矩阵 |
| Parts 模型未稳定就接 Responses 会反复重构 | Parts 设计本身先固化，再对接新的 wire format 更稳 |

**17 个 OpenAI-compat shim 永远走 Chat Completions**——它们 vendor 本身就不支持 ordering，无须特殊处理。Responses API 接入后，**只有 OpenAI 自家用户能切换到新 adapter**。

---

## 11. 风险与待定项

### 11.1 已知风险

1. **OpenAI Chat Completions 反推 Parts 顺序的约定**
   read 路径上从 `{content, tool_calls}` 反推 Parts，约定"先 text 后 tool_calls"。这是 lossy 转换——但 OpenAI Chat Completions 协议本身就丢序，**lynx 无能为力**。OpenAI Responses API（P8）天然有序，接入后这条约定自动作废。

2. **`AssistantMessage` 指针身份判定 turn 边界**
   ToolMiddleware 在每个新 turn 创建 fresh `*AssistantMessage`。consumer 用 `curr != lastTurn` 比较是 Go 指针比较——可靠但隐式。**建议**：在 doc 里明确这条约定，加 testcase 验证。

3. **共享指针 + copy-on-grow 内存模型**
   流式累加器只 mutate 最后一个 part 的 pointer（替换为新 pointer 含累积内容），保持早期 parts immutable。**写实现时要小心**：永远不在已 yield 的 part 上做 in-place mutation，否则 consumer 持有旧 Response 会看到内容飘动。**单元测试**：consumer 持有第 N 个 yield 的 Parts[i]，跑到第 N+M 个 yield 时验证 Parts[i] 内容不变。

4. **HITL 流的 multi-request 协议**
   approval 流要求：stream 暂停 → consumer 用 ApprovalResponse 发新 request → middleware 继续。**这是 multi-request 协议**，状态在 Request.Messages 历史里。**待定**：`ToolApprovalRequest.RequestID` 的生成 / 验证规则；超时清理。

### 11.2 待定项

- [ ] `chat.DiffParts` 在 abort + retry 场景下的 index 一致性
- [ ] `CustomPart.Payload any` 的 JSON 序列化策略（Marshaler 接口要求 vs reflect-based）
- [ ] `MediaPart` 在 OpenAI Chat Completions 的 round-trip（OpenAI Chat 不支持 assistant 端 inline media）
- [ ] Anthropic 多 thinking signature 在 Parts 内的合并规则（每个 reasoning block 一个 signature；合并后保留所有还是只保留最后一个）

---

## 12. 验证清单

实施完成后需要的端到端验证：

| 用例 | 验证点 |
|---|---|
| Anthropic Claude 流式（thinking + 2 tool calls + final text）| Parts 累积出 `[ReasoningPart, TextPart, ToolCallPart, TextPart, ToolCallPart, TextPart]`，时序正确，FinishReason=ToolCalls |
| OpenAI Chat Completions（仅 tool calls）| Parts = `[ToolCallPart, ToolCallPart]` |
| OpenAI Chat Completions（text + tool calls 混合）| Parts = `[TextPart, ToolCallPart, ToolCallPart]`（按"先 text 后 tool"约定）|
| Gemini 2.0 Flash（thinking + functions + grounding）| Parts 含 ReasoningPart + ToolCallPart + SourcePart |
| ToolMiddleware 3-turn 循环 | consumer 用指针比较检测到 3 个不同 AssistantMessage；最终 yield 含最终答案 |
| `chat.CollectTurns` helper | 返回 3 个 Response（每 turn 一个 final snapshot）|
| Stream cancel mid-turn | iter.Seq2 提前 return，server-side 连接关闭 |
| HITL approval | 第 1 个 stream yield ToolApprovalRequestPart + FinishReason=ApprovalRequested → 业务发新 request 带 ApprovalResponse → 第 2 个 stream 继续执行工具 |
| Agent transfer | `Event.Actions.TransferToAgent = "executor"` → 下一个 Event 的 `Author = "executor"` |
| `DiffParts` 增量 | 连续两次 yield 之间，diff 出最后一个 part 的增量；O(1) 复杂度 |
| `Normalize` pipeline | 100 个微小 TextPart 合并成 1 个 |
| AG-UI wire round-trip | `StreamToAGUI` 派发的 events 经过 `UnmarshalEvent` 反向后能还原 Parts |
| 共享指针内存模型 | 持有第 N yield 的 Parts[i]，跑到第 N+M yield 时 Parts[i] 内容未变 |

---

## 13. 后续可扩展点

- **AG-UI Server transport**：`server/aguifmt` 子包提供 HTTP handler，把 lynx agent 暴露成 AG-UI endpoint（CopilotKit React UI 直接接）
- **AG-UI Client adapter**：作为 AG-UI client 消费 remote agent stream（lynx 充当 agent-of-agents 编排者）
- **Vercel UI Message Stream Protocol** 互通：第二个 wire adapter（Vercel SDK 私有协议）—— 给 Next.js / `useChat` 用户用
- **Trace 整合**：每个 yield 自动产 OTel span attribute（`gen_ai.*`），part-level child span
- **Replay / time-travel**：保存中间 turn 的 Response → 可以从某一点 replay
- **MCP 协议事件桥接**：把 lynx Response stream 映射到 MCP 的 progress notification（与 `mcp/` 包整合）

---

## 14. 一句话定档

**lynx 走最简形态——Response/Result/AssistantMessage 三层结构不动，只把 AssistantMessage 内部 flat fields 换成 `Parts []OutputPart`；流式 `iter.Seq2[*Response, error]` 签名不动，yield 携带累积 Parts；同步 Call 的 Response 等于流式最后一次 yield。**

**API 表面零新增类型**——所有新功能落在 `OutputPart` 接口 + 11 种具体 Part + 一组 Parts 后处理 helper（Normalize / DiffParts / ...）。

**8 家对比里 lynx 同时占据**：
- vendor-neutral（22 chat provider）
- 完整 ordering（Parts 模型）
- snapshot 流（in-process Go-idiomatic）
- 最简 API 表面（零新增顶层类型）

**优先级**：P0 + P1 + P3 → 反超 4 家；P2 + P4 + P5 → 持平 Vercel + adk-go；P6 → 持平 TanStack；P8（OpenAI Responses API）独立 epic 延后做。

---

*文档结束。lynx HEAD `d291eff`，对比基线日期 2026-05-18。*
