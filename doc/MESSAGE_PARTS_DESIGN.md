# Message Parts 设计 —— 7 家对比 + lynx 改造方案

> **基线**
> - lynx HEAD `5532f54`（branch `feat/message-parts-design`，2026-05-18）
> - 对比对象（按 release / 主流度排序）：
>   1. **Spring AI**（Java，Spring 生态）
>   2. **langchain4j**（Java，社区驱动）
>   3. **eino**（Go，ByteDance）
>   4. **trpc-agent-go**（Go，Tencent）
>   5. **adk-go**（Go，Google Agent Development Kit）
>   6. **Vercel AI SDK**（TypeScript，业界事实标准）
>   7. **lynx**（Go，本仓库）
>
> **结论先行**：Vercel AI SDK 是 7 家里设计最完整的。lynx 已经在"vendor-neutral + 22 chat provider"这一轴占位，**只需补完 ordering 这一轴**，就能与 Vercel 同档并领先其余 5 家。
>
> **重要前提**：lynx 仍在开发迭代期，公开 API（包括 `chat.AssistantMessage` 数据结构）**可以做破坏性调整**。本文档的方案不考虑向后兼容旧字段。

---

## 0. TL;DR

**问题**：现代 LLM 在单 turn 里可以产出"text → tool_use → text → tool_use → text"的有序混排（Claude content blocks / Gemini Parts / OpenAI Responses output items）。lynx 当前的 `AssistantMessage { Text string; Reasoning string; ToolCalls []ToolCall }` 把这三类内容**打平**，**ordering 信息丢失**。

**7 家观察**：

- **6 家把 Anthropic content blocks 按类型打平**（Spring AI / langchain4j / eino / trpc-agent-go / lynx 当前）—— 同样的 industry pattern
- **2 家有完整的 Parts 模型**（adk-go via `genai.Part`、Vercel AI SDK via `ContentPart`）
- **唯独 Vercel** 同时做到：vendor-neutral + tool-call/tool-result 入 Parts + tool-error/tool-approval 都是 first-class Part type + 每 Part 携带 providerMetadata

**lynx 路线**：抄 Vercel 的 ContentPart 模型，落到 Go-idiomatic 接口上，agent 层借鉴 adk-go 的 `iter.Seq2[*Event, error]` + EventActions。

---

## 1. 问题陈述

### 1.1 单 turn 内的有序混排

Anthropic Claude（thinking-augmented + tool use）的实际 wire 形态：

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

lynx 当前的接收形态：

```go
type AssistantMessage struct {
    Text      string      // "好的，我先查一下天气：\n然后看看日历：\n等结果回来再总结。"
    Reasoning string      // "用户想查天气和日历，先查天气..."
    ToolCalls []ToolCall  // [{ID:"tu_1", ...}, {ID:"tu_2", ...}]
    ...
}
```

→ **UI 想还原原始顺序做不到**：用户看到的应该是"text₁ → 工具占位 → text₂ → 工具占位 → text₃"，而不是"全部文本拼一起 + 两个工具调用塞底部"。

### 1.2 跨 turn 的事件回放

Tool 循环跑 N 轮（assistant₁ → tool₁ → assistant₂ → tool₂ → assistant₃）。lynx 当前只返回**最后一轮 Response**，中间 turn 的解释性文本被吞掉。业务代码没办法重放完整的"思考-行动-观察"链条。

### 1.3 流式语义

每个 SSE chunk 应该 carry 一个 part-level index，告诉消费方"这是哪个 part 的增量"。当前 lynx 的累加器把所有 chunk 的 text 拼到 `AssistantMessage.Text`，无法做 part-level 增量渲染。

---

## 2. 7 家对比矩阵

| 维度 | Spring AI | langchain4j | eino | trpc-agent-go | adk-go | **Vercel AI** | **lynx 现状** |
|---|---|---|---|---|---|---|---|
| 单 Message Parts 有序 | ❌ | ❌ | ✅ multimodal only | ❌ | ✅ via `genai.Part` | ✅ **完整** | ❌ |
| Reasoning 入 Parts | ❌ | ❌ | ✅ ReasoningPart | ❌ | ✅ Thought flag | ✅ ReasoningOutput | ⚠️ flat field |
| Reasoning signature | ❓ | ✅ attributes | ✅ part 字段 | ⚠️ extensions | ✅ ThoughtSignature | ✅ providerMetadata | ⚠️ Metadata |
| **text↔tool_call 有序交错** | ❌ | ❌ | ❌（承认） | ❌ | ✅ FunctionCall part | ✅ tool-call part | ❌ |
| tool-result 入 Parts | ❌ | ❌ | ❌ | ❌ | ✅ FunctionResponse | ✅ | ❌ |
| **tool-error 独立 Part 类型** | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| **human-in-the-loop 工具确认** | ❌ | ❌ | ❌ | ❌ | ⚠️ RequestedToolConfirmations action | ✅ approval-request/response part | ❌ |
| 代码执行 part / grounding source | ❌ | ❌ | ❌ | ❌ | ✅ ExecutableCode + Grounding | ✅ source + file | ❌ |
| 每 Part 携带 providerMetadata | ❌ | ✅ attributes | ✅ Extra | ❌ | ⚠️ | ✅ | ⚠️ message-level only |
| 跨 turn intermediate 暴露 | ❌ | ✅ slice | ✅ event | ✅ channel | ✅ iter.Seq2 | ✅ **steps + content + finalStep 三视图** | ❌ |
| 流式 typed event 数 | 1-2 | 2-3 | ~5 | ~3 | ~5 | **~25** | 2 |
| 流式 start/delta/end 三段 | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| 嵌套 agent 调用链 | ❌ | ❌ | ✅ RunPath | ✅ ParentInvocationID + Branch | ✅ Branch 点分串 | ⚠️ | ⚠️ agent 内部 |
| StateDelta / ArtifactDelta | ❌ | ❌ | ⚠️ checkpoint | ✅ StateDelta | ✅ both | ⚠️ | ❌ |
| TransferToAgent / Escalate | ❌ | ❌ | ✅ action | ❌ | ✅ both | ⚠️ | ⚠️ agent 内部 |
| Custom Part namespace | ⚠️ properties | ⚠️ attributes | ⚠️ Extra | ⚠️ Extensions | ❌ | ✅ `${prov}.${kind}` | ⚠️ Metadata |
| Multi-vendor provider | ✅ 20+ | ✅ 30+ | ✅ 多家 | ✅ 多家 | ❌ **Google only** | ✅ 20+ | ✅ 22 chat |
| 流式 API 风格 | Reactor Flux | callback | StreamReader | `<-chan *Event` | `iter.Seq2` | AsyncIterableStream | `iter.Seq2` |

---

## 3. 各家深入分析

### 3.1 Spring AI（Java）

**Message 模型**（`AssistantMessage.java`）：

```java
public class AssistantMessage {
    String content;                  // 所有文本拼一起
    List<ToolCall> toolCalls;        // 平铺数组
    List<Media> media;
    Map<String, Object> properties;
}
```

**Anthropic 适配器**（`AnthropicChatModel.buildGenerations()`）按类型打平：text 进 `StringBuilder`，tool_use 进 `List<ToolCall>`，**thinking 单独 emit 成多 Generation**（唯一的变通）。

**Tool 循环**（`OpenAiChatModel.internalCall`）：递归调用，`conversationHistory` 累计 message，但**只返回最后一个 ChatResponse**。Usage 是 cumulative 的。

**评估**：跟 lynx 现状几乎一样的取舍。Spring AI 在 Reasoning 上不如 lynx（lynx 有显式 Reasoning 字段；Spring AI 走多 Generation workaround）。

### 3.2 langchain4j（Java）

**Message 模型**（`AiMessage.java`）：

```java
public class AiMessage {
    String text;
    String thinking;                                    // 1.2.0 加的
    List<ToolExecutionRequest> toolExecutionRequests;
    Map<String, Object> attributes;                     // 杂物袋
}
```

**Anthropic 适配器**（`DefaultAnthropicClient.build()`）同样打平：text 用 `joining("\n")` 拼，thinking 用 `joining("\n")` 拼，tool_use 进数组。

**亮点 ①**：流式有 `onPartialThinking` callback —— **partial thinking 和 partial text 分通道**。

**亮点 ②**：Tool 循环（`ToolService.executeInferenceAndToolsLoop`）显式维护 `List<ChatResponse> intermediateResponses`，最终返回 `ToolServiceResult { intermediateResponses, finalResponse, toolExecutions, aggregateTokenUsage }`。**业务代码能拿到所有中间 turn**。

**亮点 ③**：`attributes` map 是已知的杂物袋，存 `THINKING_SIGNATURE_KEY` / `REDACTED_THINKING_KEY` / `GENERATED_IMAGES_KEY` / `SERVER_TOOL_RESULTS_KEY`，类型安全但松散。

**评估**：单 turn ordering 没做，但跨 turn intermediate 暴露做得最干净（结构化数据 + 命名 keys）。**lynx 应该抄 intermediateResponses 这一招**。

### 3.3 eino（Go，ByteDance）

**Message 模型**（`schema/message.go`）：

```go
type Message struct {
    Role     RoleType
    Content  string

    // ★ 有序 parts（user 输入侧 + assistant 输出侧分开）
    UserInputMultiContent     []MessageInputPart
    AssistantGenMultiContent  []MessageOutputPart  // 这是 eino 的一大亮点

    ToolCalls        []ToolCall      // 仍然单独在 Message 顶层
    ReasoningContent string          // 仍然单独字段
    Extra            map[string]any
}

type MessageOutputPart struct {
    Type      ChatMessagePartType  // text / image / audio / video / reasoning
    Text      string
    Image     *MessageOutputImage
    Audio     *MessageOutputAudio
    Video     *MessageOutputVideo
    Reasoning *MessageOutputReasoning   // Text + Signature
    Extra     map[string]any
    StreamingMeta *MessageStreamingMeta  // ★ Index
}

type MessageStreamingMeta struct {
    Index int  // "useful for reassembling multiple reasoning/content parts in correct order"
}
```

**亮点**：把 multimodal + reasoning 做成了有序 part list。流式 `Index` 显式支持重组。

**短板**：**ToolCalls 仍在 Message 顶层不在 Parts 里** —— text↔tool ordering 没解决。

**自己承认了这个 bug**（`flow/agent/react/react.go:166-179` 的 `StreamToolCallChecker` 注释）：

> Different models have different ways of outputting tool calls in streaming mode:
>   - Some models (like OpenAI) output tool calls directly
>   - Others (like Claude) output text first, then tool calls
> ...
> The default implementation **does not work well with Claude**, which typically outputs tool calls after text content.

变通方案是让用户写自定义 checker 提前扫描 stream —— **承认问题但没在数据模型里解决**。

**跨 turn**：`adk` 包用 `AsyncIterator[*AgentEvent]`，每个 event 是 `MessageOutput` 或 `AgentAction`（transfer/exit/interrupt），完整捕获时序。

### 3.4 trpc-agent-go（Go，Tencent）

**Message 模型**（`model/request.go`）：

```go
type Message struct {
    Role         Role
    Content      string
    ContentParts []ContentPart      // ★ 但只服务 user 侧多模态（text/image/audio/file）
    ToolID       string
    ToolName     string
    ToolCalls    []ToolCall
    ReasoningContent string
}

type ContentPart struct {
    Type  ContentType    // text / image / audio / file
    Text  *string
    Image *Image
    Audio *Audio
    File  *File
}
```

**评估**：`ContentParts` 比 eino 还退一步——只有 user 输入侧多模态，**assistant 输出侧仍然是 flat fields**。Anthropic 适配器（`convertContentBlock`）跟 Spring AI / langchain4j 一样按类型 builder 拼一起。

**Response 形态**：直接照搬 OpenAI Choices/Delta，把所有 vendor 翻译成 OpenAI 形态。**vendor 抽象上倒退**。

**跨 turn 亮点**：`Agent.Run` 返回 `<-chan *event.Event`（Go-native channel），Event 含：

```go
type Event struct {
    *model.Response                       // 嵌入
    RequestID, InvocationID string
    ParentInvocationID      string        // 嵌套 agent
    Author                  string        // 哪个 agent 发的
    Branch                  string        // multi-agent 分支
    Tag                     string
    Actions                 *EventActions { SkipSummarization bool }
    StateDelta              map[string][]byte
    Extensions              map[string]json.RawMessage
    ...
}
```

**亮点**：`ParentInvocationID + Branch + Author + Tag + StateDelta` 五件套是嵌套 agent 编排里最完整的元数据。

### 3.5 adk-go（Go，Google Agent Development Kit）

**Message 模型**：**不抽象，直接用 `genai.Content` / `genai.Part`**：

```go
// google.golang.org/genai
type Content struct {
    Parts []*Part      // ★ 有序
    Role  string
}

type Part struct {
    Text             string
    FunctionCall     *FunctionCall            // ★ 工具调用作为 Part
    FunctionResponse *FunctionResponse        // ★ 工具结果作为 Part
    Thought          bool                     // ★ reasoning 是 Part 上的 flag
    ThoughtSignature []byte
    InlineData       *Blob                    // image/audio/video
    FileData         *FileData
    ExecutableCode   *ExecutableCode          // Gemini code execution
    CodeExecutionResult *CodeExecutionResult
    VideoMetadata    *VideoMetadata
    MediaResolution  *PartMediaResolution
}
```

**亮点**：**唯一真正解决 text↔tool 有序交错的设计** ——FunctionCall / FunctionResponse 都是 Part，按时间顺序排列。

**代价**：**只支持 Google 系**（`gemini` + `apigee`），没有 OpenAI/Anthropic 适配器。强行把 OpenAI 翻译到 `genai.Part` 会丢失 vendor-specific 字段。

**Agent.Run 签名**：

```go
type Agent interface {
    Run(InvocationContext) iter.Seq2[*session.Event, error]
    ...
}

type Event struct {
    model.LLMResponse    // embed
    ID, InvocationID, Branch, Author string
    Timestamp time.Time
    Actions   EventActions
    LongRunningToolIDs []string
}

type EventActions struct {
    StateDelta                 map[string]any
    ArtifactDelta              map[string]int64       // ★ artifact 版本变更
    TransferToAgent            string                 // ★ 显式 agent 转交
    Escalate                   bool                   // ★ 上抛父 agent
    SkipSummarization          bool
    RequestedToolConfirmations map[string]ToolConfirmation  // ★ 异步工具确认
}

// 内置 helper
func (e *Event) IsFinalResponse() bool { ... }
```

**亮点 ②**：`Branch` 是点分字符串 `"agent_1.agent_2.agent_3"`，trace 时易过滤。
**亮点 ③**：`EventActions` 最完整 ——5 个 flow-control 字段。
**亮点 ④**：`IsFinalResponse()` 内置 helper，调用方一行调用就能区分中间事件 vs 最终答案。
**亮点 ⑤**：Run 用 `iter.Seq2[*Event, error]`（Go 1.23 原生），跟 lynx chat 包 stream API 同款。

### 3.6 Vercel AI SDK（TypeScript）

**Message 模型**（`packages/ai/src/generate-text/content-part.ts`）—— **7 家里最完整的 ContentPart**：

```typescript
export type ContentPart<TOOLS extends ToolSet> =
  | { type: 'text';                    text: string; providerMetadata?: ProviderMetadata }
  | { type: 'custom';                  kind: `${string}.${string}`; ... }
  | ReasoningOutput                              // type: 'reasoning'  + signature
  | ReasoningFileOutput                          // type: 'reasoning-file' (Anthropic redacted)
  | ({ type: 'source' } & Source)                // citations / grounding
  | { type: 'file';                    file: GeneratedFile; ... }
  | ({ type: 'tool-call' }   & TypedToolCall   & { providerMetadata? })
  | ({ type: 'tool-result' } & TypedToolResult & { providerMetadata? })
  | ({ type: 'tool-error' }  & TypedToolError  & { providerMetadata? })
  | ToolApprovalRequestOutput
  | ToolApprovalResponseOutput;
```

**亮点 ①**：text + reasoning + source + file + tool-call + tool-result + tool-error + tool-approval + custom 9 种 Part type 全部入有序列表。

**亮点 ②**：**`tool-error` 是独立 Part type** —— 工具失败结构化暴露，不是 text 包错误消息。

**亮点 ③**：**`tool-approval-request` / `tool-approval-response`** 把 human-in-the-loop 工具确认做成 first-class Part。

**亮点 ④**：**`custom` Part 用 `${provider}.${kind}` 命名空间** —— vendor-specific block 的类型化逃生通道。

**亮点 ⑤**：**每 Part 都有 `providerMetadata`**（不是只在 Message level）—— vendor 元数据精确定位到 Part 级别。

**结果数据结构**（`generate-text-result.ts`）—— **三视图同时给**：

```typescript
export interface GenerateTextResult<TOOLS, ...> {
  readonly content: Array<ContentPart<TOOLS>>;      // ★ 全 step 聚合后的有序内容
  readonly text: string;                             // 派生 view
  readonly reasoning: Array<ReasoningOutput>;
  readonly reasoningText: string | undefined;
  readonly files: Array<GeneratedFile>;
  readonly sources: Array<Source>;
  readonly toolCalls: Array<TypedToolCall>;
  readonly toolResults: Array<TypedToolResult>;
  ...
  readonly steps: Array<StepResult>;                // ★ 所有中间 step 保留
  readonly finalStep: StepResult;                   // ★ 最后一步快捷访问
}
```

**亮点 ⑥**：langchain4j 给 `intermediateResponses + finalResponse` 两个字段；Vercel 给 **`steps[] + content[] + finalStep` 三视图**，覆盖所有访问 pattern。

**流式 `fullStream`**：`AsyncIterableStream<TextStreamPart>`，~25 种 typed event：

```typescript
// 文本: text-start / text-delta / text-end
// 推理: reasoning-start / reasoning-delta / reasoning-end
// 工具参数: tool-input-start / tool-input-delta / tool-input-end
// 工具事件: tool-call / tool-result / tool-error / tool-output-denied
// 工具确认: tool-approval-request / tool-approval-response
// 来源/文件: source / file / reasoning-file
// 生命周期: start / finish / start-step / finish-step
// 异常: abort / error
// 自定义: custom
```

每种语义都有独立的 start/delta/end 三段，UI 端能精确控制每种 part 的渲染生命周期。

**评估**：**Vercel 同时占据 vendor-neutral + 完整 ordering 两个最优位置**。证明了"二者兼得"是单一可达点，不是技术不可能。

### 3.7 lynx 现状

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
- ⚠️ 流式只有 2 种 event（chunk / done）
- ✅ vendor-neutral（22 chat provider 都翻译成同一份 AssistantMessage）

---

## 4. 取舍光谱

```
                  vendor-neutral                    vendor-locked
                  ←————————————————————————————————→
                                                                
   ❌ ordering    Spring AI                                       
                 langchain4j                       adk-go       
                 trpc-agent-go                    ✅ ordering    
                 eino (一半)                       ❌ multi-vendor
                                                                  
   ✅ ordering    ★ Vercel AI ★                                    
                 ★ lynx 目标位置 ★                                  
```

**Vercel 已经证明二者兼得是可行的。adk-go 选择放弃 multi-vendor 是 Google 的政治选择，不是技术必然。lynx 应该走 Vercel 路线。**

---

## 5. lynx 的设计决策

### 5.1 取舍

| 决策点 | 选择 | 理由 |
|---|---|---|
| Part 是否含 ToolCall | ✅ 含 | 真正解决 text↔tool ordering（adk-go / Vercel 都这么做）|
| Part 是否含 ToolResult | ✅ 含 | tool 执行历史回放需要；assistant turn 与 tool turn 不分离 |
| Part 是否含 ToolError | ✅ 含 | Vercel 独有的设计，工具失败结构化（避免错误信息混在 text 里）|
| 是否保留 flat `Text` / `ToolCalls` 字段 | ❌ **全部移除** | lynx 仍在迭代期；Parts 是 source of truth；不再做 flat-view shim |
| Part 是否携带 providerMetadata | ✅ 携带 | Vercel 路线；vendor 元数据精确到 Part 级别 |
| Custom Part 命名空间 | ✅ `<provider>.<kind>` | Vercel 路线；vendor-specific block 的类型化逃生通道 |
| Tool approval（human-in-the-loop）入 Part | ✅ | 这是产品需求一等公民（Vercel 验证过）|
| Source / File / ReasoningFile 入 Part | ✅ | 引用、生成文件、redacted thinking 都有结构化场景 |
| 跨 turn 暴露 | ✅ `steps + content + finalStep` 三视图 | 抄 Vercel；覆盖所有访问模式 |
| 流式 typed event 数 | ~20 个（start/delta/end 三段）| 抄 Vercel；比 lynx 现在 2 个 event 丰富一个数量级 |
| Agent.Run API | `iter.Seq2[*Event, error]` | 抄 adk-go；Go-idiomatic |
| EventActions | StateDelta + ArtifactDelta + TransferToAgent + Escalate + SkipSummarization + RequestedToolConfirmations | 抄 adk-go 全集 |

### 5.2 命名约定

- `Part` 词为基础（不用 `Block` / `Content` 避免冲突）
- 顶层类型 `OutputPart` 是 marker interface
- 具体 part 类型：`TextPart` / `ReasoningPart` / `ToolCallPart` / `ToolResultPart` / `ToolErrorPart` / `MediaPart` / `SourcePart` / `FilePart` / `ToolApprovalRequestPart` / `ToolApprovalResponsePart` / `CustomPart`
- Event 词用于 agent 层、流式块用 `StreamEvent`

---

## 6. 详细数据模型（破坏性改造）

### 6.1 OutputPart 定义

```go
// core/model/chat/part.go
package chat

import "github.com/Tangerg/lynx/core/media"

// OutputPart is the marker interface for one ordered chunk of an
// assistant's reply. Concrete implementations are sealed: TextPart /
// ReasoningPart / ToolCallPart / ToolResultPart / ToolErrorPart /
// MediaPart / SourcePart / FilePart / ToolApprovalRequestPart /
// ToolApprovalResponsePart / CustomPart.
//
// All parts of an AssistantMessage live in [AssistantMessage.Parts]
// in temporal order — the order the model emitted them. Renderers and
// tool middlewares MUST respect this order.
type OutputPart interface {
    // Kind returns the part type for switch-style discrimination.
    Kind() PartKind
    // ProviderMetadata returns vendor-specific extras attached to this
    // single part (e.g. Anthropic cache control, OpenAI request_id).
    ProviderMetadata() map[string]any

    sealedOutputPart()  // unexported: keeps the union closed
}

// PartKind enumerates concrete part types.
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

// TextPart is plain assistant-emitted text.
type TextPart struct {
    Text     string
    Metadata map[string]any  // providerMetadata
}

// ReasoningPart carries visible chain-of-thought (OpenAI o*, Anthropic
// thinking, Gemini Thought parts, DeepSeek reasoner). Signature
// preserves Anthropic thought_signature / OpenAI encrypted reasoning;
// when Redacted is true the Text is the SDK's redacted placeholder.
type ReasoningPart struct {
    Text      string
    Signature []byte
    Redacted  bool
    Metadata  map[string]any
}

// ToolCallPart is one tool invocation request. The same ID flows into
// the matching ToolResultPart / ToolErrorPart so callers can pair them.
type ToolCallPart struct {
    ID        string
    Name      string
    Arguments string  // JSON-encoded
    Metadata  map[string]any
}

// ToolResultPart is the (successful) result of executing a ToolCallPart.
// It only ever appears INSIDE an assistant turn when the loop has
// already executed the call — i.e. the agent's tool middleware
// inlines the result instead of emitting a separate Tool message.
// In the classic "two-message dance" (assistant→tool→assistant) this
// part is absent; the tool round-trip is captured at the Steps level.
type ToolResultPart struct {
    ID         string
    Name       string
    Result     string  // canonical JSON; tools that return non-string
                       // bytes should base64 + content-type via Metadata
    Metadata   map[string]any
}

// ToolErrorPart marks a tool execution failure. Errors are first-class
// so callers can branch on them without parsing text.
type ToolErrorPart struct {
    ID       string
    Name     string
    Error    string  // human-readable; structured details via Metadata
    Metadata map[string]any
}

// MediaPart is an inline media artifact (image / audio / video) the
// model returned as part of its response.
type MediaPart struct {
    Media    *media.Media
    Metadata map[string]any
}

// SourcePart is a citation / grounding source surfaced by the model
// (Perplexity citations, Anthropic web_search results, Gemini grounding).
type SourcePart struct {
    ID       string  // optional source identifier
    URL      string
    Title    string
    Snippet  string  // model-supplied excerpt
    Metadata map[string]any
}

// FilePart represents a generated file artifact (Anthropic Files API
// output, OpenAI generated file, Gemini Imagen output stored as URI).
type FilePart struct {
    Name     string
    URI      string
    MIME     string
    Size     int64
    Metadata map[string]any
}

// ToolApprovalRequestPart asks the user to confirm a tool invocation
// before the agent actually runs it. Used in human-in-the-loop flows.
type ToolApprovalRequestPart struct {
    RequestID string
    ToolName  string
    Arguments string
    Reason    string  // why the model wants this approved
    Metadata  map[string]any
}

// ToolApprovalResponsePart records the user's approve/deny decision.
type ToolApprovalResponsePart struct {
    RequestID string
    Approved  bool
    Reason    string  // user-supplied rationale (optional)
    Metadata  map[string]any
}

// CustomPart is the typed escape hatch for vendor-specific blocks
// that don't fit any of the typed kinds. The Namespace+Kind pair
// scopes the block to a vendor; consumers switch on (Namespace, Kind)
// to decode Payload.
type CustomPart struct {
    Namespace string  // e.g. "anthropic", "openai", "google"
    Kind      string  // e.g. "server_tool_use", "code_execution_result"
    Payload   any     // vendor-specific shape
    Metadata  map[string]any
}
```

每个具体 part 类型实现 `Kind()` / `ProviderMetadata()` / `sealedOutputPart()` 方法。`sealedOutputPart()` 是未导出方法 —— 防止外部新增 part type 破坏 switch 闭包性（Go 的 closed union 模式）。

### 6.2 AssistantMessage 改造

```go
// core/model/chat/message.go (改造后)
type AssistantMessage struct {
    // Parts is the ordered list of content chunks the model emitted —
    // text / reasoning / tool calls / media / etc. The order matches
    // what the model produced and MUST be respected by renderers.
    Parts []OutputPart `json:"parts"`

    // Metadata is message-level provider metadata. Use individual
    // OutputPart.Metadata for part-level metadata.
    Metadata map[string]any `json:"metadata,omitzero"`
}
```

**flat fields 全部移除**。调用方查文本/工具/推理改用 helper 方法（见 §6.3）。

### 6.3 派生 helper

不再保留 `Text string` / `ToolCalls []ToolCall` 之类的 flat 字段，但提供方法迭代：

```go
// TextParts iterates all TextPart in this message, in order.
func (m *AssistantMessage) TextParts() iter.Seq[*TextPart] { ... }

// ToolCalls iterates all ToolCallPart in this message, in order.
func (m *AssistantMessage) ToolCalls() iter.Seq[*ToolCallPart] { ... }

// JoinedText concatenates all TextPart bodies with no separator. Use
// when downstream just needs "the final string the user sees".
func (m *AssistantMessage) JoinedText() string { ... }

// JoinedReasoning concatenates all ReasoningPart bodies.
func (m *AssistantMessage) JoinedReasoning() string { ... }
```

→ 调用方代码从 `msg.Text` 改成 `msg.JoinedText()`，工具调用列表从 `msg.ToolCalls` 改成 `slices.Collect(msg.ToolCalls())`。**破坏性，但代码迁移直观**。

### 6.4 Tool / User Message 同样改

```go
type ToolMessage struct {
    // ToolResults is the ordered list of results for tool calls the
    // assistant requested in the prior turn. ID matches ToolCallPart.ID.
    Results  []ToolResultPart  // 复用 part 类型
    Errors   []ToolErrorPart   // 工具失败也是结构化
    Metadata map[string]any
}

type UserMessage struct {
    Parts    []InputPart
    Metadata map[string]any
}

// InputPart enumerates user-input parts.
type InputPart interface {
    InputKind() PartKind
    sealedInputPart()
}
type UserTextPart   struct { Text string; Metadata map[string]any }
type UserMediaPart  struct { Media *media.Media; Metadata map[string]any }
type UserFilePart   struct { URI, MIME, Name string; Metadata map[string]any }
```

System message 保持简单：

```go
type SystemMessage struct {
    Text     string  // 系统提示通常是纯文本
    Metadata map[string]any
}
```

---

## 7. 流式协议

### 7.1 StreamEvent discriminated union

```go
// core/model/chat/stream.go
type StreamEvent interface {
    EventKind() StreamEventKind
    sealedStreamEvent()
}

type StreamEventKind string

const (
    // Lifecycle.
    StreamEventStart      StreamEventKind = "start"
    StreamEventFinish     StreamEventKind = "finish"
    StreamEventStartStep  StreamEventKind = "start_step"   // tool-loop 内每个 turn
    StreamEventFinishStep StreamEventKind = "finish_step"

    // Text.
    StreamEventTextStart StreamEventKind = "text_start"
    StreamEventTextDelta StreamEventKind = "text_delta"
    StreamEventTextEnd   StreamEventKind = "text_end"

    // Reasoning.
    StreamEventReasoningStart StreamEventKind = "reasoning_start"
    StreamEventReasoningDelta StreamEventKind = "reasoning_delta"
    StreamEventReasoningEnd   StreamEventKind = "reasoning_end"

    // Tool input accumulation.
    StreamEventToolInputStart StreamEventKind = "tool_input_start"
    StreamEventToolInputDelta StreamEventKind = "tool_input_delta"
    StreamEventToolInputEnd   StreamEventKind = "tool_input_end"

    // Tool results.
    StreamEventToolCall    StreamEventKind = "tool_call"
    StreamEventToolResult  StreamEventKind = "tool_result"
    StreamEventToolError   StreamEventKind = "tool_error"

    // Tool approval (HITL).
    StreamEventToolApprovalRequest  StreamEventKind = "tool_approval_request"
    StreamEventToolApprovalResponse StreamEventKind = "tool_approval_response"

    // Media / sources / files.
    StreamEventMedia  StreamEventKind = "media"
    StreamEventSource StreamEventKind = "source"
    StreamEventFile   StreamEventKind = "file"

    // Anomalies.
    StreamEventAbort StreamEventKind = "abort"
    StreamEventError StreamEventKind = "error"

    // Custom escape hatch.
    StreamEventCustom StreamEventKind = "custom"
)
```

每个事件有自己的结构体。例：

```go
type TextDeltaEvent struct {
    PartIndex int     // 这是 Parts 数组里的第 N 个 part
    Delta     string
}

type ToolInputDeltaEvent struct {
    PartIndex int
    CallID    string
    Delta     string  // JSON-fragment 增量
}

type FinishStepEvent struct {
    StepIndex int
    Reason    FinishReason
    Usage     *Usage
}

type FinishEvent struct {
    Reason FinishReason
    Usage  *Usage         // 整轮累计
    Steps  []*StepResult  // 所有 step 的快照（也可走 Result API 拿）
}
```

### 7.2 Model.Stream 签名

```go
type Model interface {
    Call(ctx context.Context, req *Request) (*Response, error)
    Stream(ctx context.Context, req *Request) iter.Seq2[StreamEvent, error]
    ...
}
```

事件按 part 维度成 start/delta/end 三段。`PartIndex` 用于消费方组装 part 索引到 final `AssistantMessage.Parts`。

### 7.3 累加器

`response_accumulator.go` 改造：维护 `parts []OutputPart` 数组，按 `PartIndex` 写入：

```go
type Accumulator struct {
    parts []OutputPart
    steps []*StepResult
    // ...
}

func (a *Accumulator) Apply(ev StreamEvent) {
    switch e := ev.(type) {
    case *TextStartEvent:
        a.ensurePart(e.PartIndex, &TextPart{})
    case *TextDeltaEvent:
        p := a.parts[e.PartIndex].(*TextPart)
        p.Text += e.Delta
    case *ToolInputStartEvent:
        a.ensurePart(e.PartIndex, &ToolCallPart{ID: e.CallID, Name: e.Name})
    case *ToolInputDeltaEvent:
        p := a.parts[e.PartIndex].(*ToolCallPart)
        p.Arguments += e.Delta
    // ... 等等
    }
}
```

---

## 8. 跨 turn API（Tool Middleware）

### 8.1 StepResult

```go
// core/model/chat/step.go
type StepResult struct {
    StepNumber int

    // AssistantMessage is the assistant turn that produced this step.
    AssistantMessage *AssistantMessage

    // ToolMessage is the executed tool round-trip following this step.
    // Nil when this step did not request tools (i.e. the final step).
    ToolMessage *ToolMessage

    // FinishReason is the reason this step's assistant generation stopped.
    FinishReason FinishReason

    // Metadata carries step-level metadata (per-turn usage, request id, ...).
    Metadata *ResponseMetadata

    // RequestSnapshot is the request actually sent for this step
    // (useful for debugging / replay).
    RequestSnapshot *Request
}
```

### 8.2 Response 三视图

```go
type Response struct {
    // Steps is the ordered list of every assistant turn in this
    // generation — including all tool-loop intermediate turns.
    Steps []*StepResult

    // FinalStep is the last step (Steps[len(Steps)-1]) — most callers
    // only need this. Nil when Steps is empty.
    FinalStep *StepResult

    // AggregatedContent flattens every step's AssistantMessage.Parts
    // into one ordered timeline — interleaved text → tool-call →
    // tool-result → text → ... — for renderers that show the full
    // conversation as a single stream.
    AggregatedContent []OutputPart

    // Metadata is generation-level metadata: cumulative usage across
    // all steps, model id, response id, etc.
    Metadata *ResponseMetadata
}
```

**这套就是 Vercel 三视图的 Go 版本**：

- `Response.FinalStep.AssistantMessage.JoinedText()` —— 业务代码只关心最后答案
- `Response.AggregatedContent` —— UI 想还原完整对话时间线
- `Response.Steps` —— 调试 / 训练数据收集 / agent 编排

### 8.3 ToolMiddleware 改造

```go
// core/model/chat/tool_middleware.go
type ToolMiddleware struct {
    Tools []CallableTool
    // OnIntermediate is called once per step BEFORE the tool round-trip
    // (i.e. with the assistant turn that requested tools). Use it for
    // streaming logs / observability hooks.
    OnIntermediate func(step *StepResult)
    // MaxSteps caps the loop. 0 selects DefaultMaxSteps (10).
    MaxSteps int
}
```

返回的 `Response` 含完整 `Steps[]`。**业务代码默认看 `FinalStep`；想看轨迹的代码看 `Steps[]`；想看时间线的代码看 `AggregatedContent`**。

---

## 9. Agent 层 Event 流

### 9.1 Agent.Run 签名

```go
// agent/agent.go
type Agent interface {
    // Call runs the agent and returns the final result.
    Call(ctx context.Context, req *Request) (*Response, error)

    // Run runs the agent and streams every event — including
    // intermediate assistant turns, tool calls, tool results, sub-agent
    // transfers, and state delta changes. Use this for agentic UIs.
    Run(ctx context.Context, req *Request) iter.Seq2[*Event, error]

    Info() Info
    SubAgents() []Agent
}
```

`iter.Seq2[*Event, error]` 抄 adk-go ——Go 1.23 原生，跟 lynx chat 包 stream API 同款。

### 9.2 Event 结构

```go
// agent/event.go
type Event struct {
    // Step is the assistant-turn snapshot when this event fires (nil for
    // pure action events like transfer/escalate).
    Step *chat.StepResult

    ID                 string
    InvocationID       string
    ParentInvocationID string   // 嵌套 agent
    Branch             string   // "root.planner.executor" 点分串
    Author             string   // 哪个 agent 发的
    Timestamp          time.Time

    Actions *EventActions

    Err error
}

type EventActions struct {
    // StateDelta carries session-state changes the agent wants to
    // commit as part of this event.
    StateDelta map[string]any

    // ArtifactDelta tracks artifact-version bumps (filename → version).
    ArtifactDelta map[string]int64

    // TransferToAgent names the agent control should hand off to.
    TransferToAgent string

    // Escalate bubbles control up to the parent agent.
    Escalate bool

    // SkipSummarization tells the flow not to summarize after this event.
    SkipSummarization bool

    // RequestedToolConfirmations is the set of tool calls waiting for
    // user approval before they can execute.
    RequestedToolConfirmations map[string]*chat.ToolApprovalRequestPart
}

// IsFinalResponse reports whether this event represents the agent's
// final answer (no further turns / tool calls / interrupts).
func (e *Event) IsFinalResponse() bool { ... }
```

→ **adk-go 的 EventActions 全集 + lynx 自己的 chat.StepResult**。

---

## 10. 实施路线

| 阶段 | 范围 | 工作量 | 是否破坏 API |
|---|---|---|---|
| **P0** — Part 模型 | `core/model/chat/part.go` 新增；`AssistantMessage` 改造（移除 flat fields）；helper 方法（`JoinedText` / `ToolCalls` iter）| 中（~400 LOC） | ✅ 破坏 |
| **P1** — Provider builder | 22 个 chat provider 的 `buildChatResponse` 改成产 `Parts`：anthropic / openai / google / vertexai / bedrock 是 native path；其余 17 个 OpenAI-compat shim 复用 openai builder | 中（每家 ~50 LOC，主要工作集中在 anthropic / google）| ✅ 破坏（外部代码读 `msg.Text` 全要改）|
| **P2** — 流式重构 | `StreamEvent` discriminated union 替换现有 `iter.Seq2[*Response, error]`；累加器按 PartIndex 重组 | 中（~300 LOC + 测试）| ✅ 破坏 |
| **P3** — Response 三视图 | `Response { Steps, FinalStep, AggregatedContent, Metadata }`；ToolMiddleware 维护 Steps[] | 低（~150 LOC）| ✅ 破坏 |
| **P4** — Agent Event 流 | `agent.Agent.Run iter.Seq2[*Event, error]`；`Event{Step, Actions{StateDelta, ArtifactDelta, TransferToAgent, Escalate, SkipSummarization, RequestedToolConfirmations}}`；保留 `Agent.Call` 同步入口 | 中-高（~500 LOC + 现有 agent 包重构）| ⚠️ 破坏 agent API |
| **P5** — 文档 + 例子 | 改 `doc/REASONING.md` / `doc/MIDDLEWARE.md`；加 `doc/PARTS_RENDERING.md` 教 UI 怎么消费 Parts；加端到端例子（Claude 交错 thinking + tool）| 低 | — |
| **P6** — 测试覆盖 | mock 测试覆盖 Anthropic 7 个 content block 类型的有序场景；Vercel-style approval flow 端到端测试 | 中 | — |

**总工作量估计**：~1500-2000 LOC 改动 + 测试，~2-3 周（一个人）。

---

## 11. 风险与待定项

### 11.1 已知风险

1. **OpenAI Chat Completions API 的 ordering 推断**
   OpenAI 经典 `/chat/completions` 不像 Anthropic 那样给 ordered content blocks，而是 `message.content: string + message.tool_calls: []` 的平铺形态。要还原顺序需要从 streaming delta 里观察 chunk 到达次序——通常 OpenAI 是 "text 先到，tool_calls 在最后一起出"，所以 Parts 顺序就是 `[TextPart, ToolCallPart_1, ToolCallPart_2, ...]`。**这是已知 lossy 转换**——OpenAI 经典 API 本身就丢了顺序信息。
   缓解：文档明确标注；想要真正的 ordering 用 OpenAI Responses API 或 Anthropic / Google。

2. **OpenAI Responses API（output[] 有序）的接入**
   `Responses API` 的 `output[]` 是 ordered list，含 `message` / `function_call` / `reasoning` 等条目，**天然映射 lynx 的 Parts**。但 lynx 当前只接了 `chat/completions`。
   建议：P1 阶段顺手加 Responses API 适配。

3. **Tool middleware 行为变化**
   当前 ToolMiddleware 把 tool 结果 wrap 成新的 user message 再发回。改造后 `ToolResultPart` 可以直接放在 assistant turn 的 Parts 里（同一个 assistant message 包含 call + result），或继续走两-message 模式。两条路并存的话语义混乱。
   **建议**：默认两-message 模式（保持 OpenAI/Anthropic 协议兼容），`ToolResultPart` 仅用于"agent runtime 内部回放 / 数据持久化"场景。

4. **AggregatedContent 的语义边界**
   当 tool middleware 跑 N 轮时，AggregatedContent 是把每个 step 的 `AssistantMessage.Parts` 直接拼起来吗？是否插入 `ToolResultPart` 表示 tool 执行？
   **建议**：插入。`AggregatedContent` 是 `[Step_1.Assistant.Parts..., Step_1.Tool.ToolResults_as_Parts..., Step_2.Assistant.Parts..., ...]` 一个扁平 timeline，UI 一遍 walk 就能渲染完整时间线。

5. **Provider metadata 命名空间冲突**
   `OutputPart.Metadata` 是 vendor-specific extras。不同 vendor 用相同 key 怎么办？
   **建议**：约定 key 前缀（`anthropic.`, `openai.`, `google.`），并在 doc 里列已知的 Metadata key。

### 11.2 待定项

- [ ] **Tool approval flow 的 lifecycle**：approval 是 sync wait 还是 async event？Vercel 是后者（client 收到 approval-request → 用户决定 → client 发 approval-response 进新 request）。
- [ ] **Reasoning signature 的传递规则**：Anthropic 要求把 thought_signature 回写到下一轮 request 才能续聊。Provider builder 需要从历史 message 重建。
- [ ] **CustomPart 的序列化**：`Payload any` 走 JSON marshal 时，反序列化端怎么知道 concrete type？建议要求 `Payload` 实现 `json.Marshaler`/`Unmarshaler`，namespace+kind 作为 type discriminator。
- [ ] **Stream event PartIndex 在 mid-stream 重启时如何重置**：abort + retry 场景的 index 连续性。

---

## 12. 验证清单

实施完成后需要的端到端验证：

| 用例 | 验证点 |
|---|---|
| Anthropic Claude 流式（thinking + 2 tool calls + final text）| 累加器产出 5 个 Parts：`[ReasoningPart, TextPart, ToolCallPart, TextPart, ToolCallPart, TextPart]`，时序正确 |
| OpenAI Chat Completions（仅 tool calls）| `[ToolCallPart, ToolCallPart]`（无 text）|
| OpenAI Chat Completions（text + tool calls 混合）| `[TextPart, ToolCallPart, ToolCallPart]`（OpenAI 经典约定：text 先 tool 后）|
| OpenAI Responses API（混合多 output）| 按 `output[]` 顺序 1:1 映射到 Parts |
| Gemini 2.0 Flash（thinking + functions + grounding）| Parts 含 ReasoningPart + ToolCallPart + SourcePart（grounding）|
| Tool middleware 3-step 循环 | `Response.Steps` 长度 3，`Response.AggregatedContent` 包含 3 个 Assistant Parts + 2 个 ToolResult round-trip |
| Stream cancel mid-step | `iter.Seq2` 提前 return，server-side 连接关闭 |
| Tool approval 流程 | `ToolApprovalRequestPart` 出现 → 调用方 emit response → 下一轮 request 含 `ToolApprovalResponsePart` → agent 继续执行 |
| Agent transfer | `Event.Actions.TransferToAgent = "executor"` → 下一个 Event 的 `Author = "executor"` |
| StateDelta 持久化 | Event 携带 `StateDelta` → session store 应用 → 下一个 Event 读取已更新状态 |

---

## 13. 后续可扩展点（不在 P0-P6 范围）

- **Streaming over WebSocket / SSE 协议**：把 StreamEvent 标准化成 wire 格式，复用 Vercel UI Message Stream Protocol 让前端 SDK 通用
- **Trace 整合**：每个 StreamEvent / Event 自动产 OTel span attribute（`gen_ai.*`）
- **Replay / time-travel**：Response.Steps 完整保留请求快照 → 可以 replay 单个 step
- **Multi-modal output**（ImagePart inline）：当前 MediaPart 已经支持；OpenAI gpt-4o-audio / Gemini Imagen-via-chat 接入时直接复用

---

## 14. 一句话定档

**Vercel AI SDK 证明了"vendor-neutral + 完整 ordering"是单一可达点。lynx 已经在 vendor-neutral 这一轴占位（22 chat provider），只需把 ContentPart 模型抽到自己的 Go-native part type 上，配合 adk-go 风格的 iter.Seq2 event stream，就能与 Vercel 同档并领先其余 5 家。**

**优先级**：P0+P1（数据模型 + provider builder）→ P3（三视图）→ P2（流式重构）→ P4（agent event 流）。P0+P1+P3 做完已经反超 Spring AI / langchain4j / eino / trpc-agent-go 四家；P2+P4 持平 Vercel；P6 测试封口。

---

*文档结束。lynx HEAD `5532f54`，对比基线日期 2026-05-18。*
