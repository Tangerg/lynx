# Lynx vs Spring AI 2.0 — 能力差距分析（2026-04-26）

> **§1.3「Lynx 必须补齐的最小 surface」已由 `REASONING_UNIFIED_DESIGN.md` 取代落地**：
> Reasoning 不再走多 Result + magic key，而是 `AssistantMessage.Reasoning string` 单字段 + provider 包内 metadata key。其余章节（§2-§7 关于 Anthropic Prompt Caching / OpenAI Responses API / Google Cached Content API 等）仍然有效。
>
> 本文是 `SPRING_AI_COMPARISON.md`（2026-04-20 基线）的**补充快照**，专注于：
> 1. **推理模型的 think / reasoning / thinking 处理**（用户重点关注，原文档完全未覆盖）
> 2. Spring AI 2.0-SNAPSHOT 在最近两周内继续落地、Lynx 仍空白的**新能力面**
>
> Spring AI 代码：`/Users/tangerg/Desktop/spring-ai`（`2.0.0-SNAPSHOT`）
> Lynx 代码：`/Users/tangerg/Desktop/lynx`（HEAD = `69e75a0`）
>
> 原 `SPRING_AI_COMPARISON.md` 关注的是「**抽象层是否对齐**」（Advisor/ToolManager/Filter/RAG 等架构倒退点）。
> 本文关注的是「**Lynx 完全没有，但 Spring AI 已经形成成熟代码** 的能力面」。

---

## 0. 一句话结论

**Lynx 的 chat 抽象停留在「文本 + 工具调用」时代。Spring AI 2.0 已经把推理模型的「思考过程」当作消息的第一类内容处理。这是 Lynx 当前最大、最显眼、最影响实际可用性的能力空白。**

| 能力面 | Spring AI 2.0 | Lynx | 严重度 |
|-------|--------------|------|--------|
| **推理模型 think / reasoning 内容建模** | ✅ 全栈支持 5 家 | ❌ **完全无** | 🔴 **P0** |
| **流式 thinking 聚合（MessageAggregator）** | ✅ `isThought` flag | ❌ 无 | 🔴 P0 |
| **ThinkingTagCleaner 工具** | ✅ 5 种 pattern | ❌ 无 | 🟠 P1 |
| **Anthropic Prompt Caching** | ✅ 4 种策略 | ❌ 无 | 🟠 P1 |
| **Anthropic Skills + Files API** | ✅ 2.0 新增 | ❌ 无 | 🟡 P2 |
| **Google Gemini Cached Content API** | ✅ 2.0 新增 | ❌ 无 | 🟡 P2 |
| **OpenAI Audio Output（output modalities）** | ✅ | ⚠️ TTS 单独支持，未与 chat 整合 | 🟡 P2 |
| **DeepSeek 模型适配器** | ✅ 独立模块 | ❌ 无 | 🟢 P3 |
| **Ollama 模型适配器（含 think 模式）** | ✅ 独立模块 | ❌ 无 | 🟢 P3 |
| **OpenAI Responses API（o1/o3/gpt-5 reasoning）** | ✅ `extraBody` + `reasoningContent` | ❌ 无 | 🟠 P1 |

---

## 1. 推理模型 Think / Reasoning 处理（最关键的一节）

### 1.1 现状：Lynx 完全无概念

**搜索证据**（在 `/Users/tangerg/Desktop/lynx` 全仓搜索）：
- `reasoning`/`thinking`/`thought`/`reasoning_content`/`chain_of_thought` → **0 个非 trivial 命中**
- `core/model/chat/message.go:132-137` 的 `AssistantMessage` 只有 `Text / Media / ToolCalls / Metadata` 四个字段
- `core/model/chat/request.go:16-46` 的 `Options` 只有 `FrequencyPenalty / MaxTokens / PresencePenalty / Stop / Temperature / TopK / TopP`
- `core/model/chat/response_accumulator.go:72-119` 流式累积器只对 text 和 tool_calls 做拼接

**意味着**：当用户调用 Claude 3.7 Sonnet（extended thinking）、OpenAI o1/o3、Gemini 2.5 Pro（thinking）、DeepSeek-R1、Qwen3 推理模型时：
- 无法接收/保存模型的「思考过程」
- 无法控制 thinking budget / reasoning effort
- 流式输出时 thinking 块和正文混在同一个 text buffer
- 多轮工具调用时无法回传 Anthropic 的 `signature` / Google 的 `thoughtSignature`，破坏推理上下文连续性

**这不是细节问题，是「推理模型时代的能力缺位」**。

### 1.2 Spring AI 2.0 的全栈推理建模

#### 1.2.1 五家 provider 的推理控制选项（input 侧）

| Provider | Option 字段 | 类型 | 取值 | 文件 |
|----------|------------|------|------|------|
| **Anthropic** | `thinking` | `ThinkingConfigParam` | `Disabled` / `Enabled(budgetTokens)` / `Adaptive` | `AnthropicChatOptions.java:121, 267-273` |
| **Anthropic** | thinking display | enum | `SUMMARIZED` / `OMITTED`（GH-5642 新增） | `AnthropicChatOptions.java` |
| **OpenAI** | `reasoningEffort` | `String` | `"low"` / `"medium"` / `"high"`（o1/o3/gpt-5） | `OpenAiChatOptions.java:110, 926-927` |
| **Google** | `thinkingBudget` | `Integer` | tokens 数量 | `GoogleGenAiChatOptions.java:119-139` |
| **Google** | `includeThoughts` | `Boolean` | 是否在响应中返回 thinking | 同上 |
| **Google** | `thinkingLevel` | `GoogleGenAiThinkingLevel` | `MINIMAL` / `LOW` / `MEDIUM` / `HIGH`（按 model 限定） | `common/GoogleGenAiThinkingLevel.java:39-72` |
| **Ollama** | `think` | `ThinkOption`（密封接口） | `ThinkBoolean(ENABLED/DISABLED)` 或 `ThinkLevel("low"/"medium"/"high")` | `ollama/api/ThinkOption.java:43-164` |
| **DeepSeek** | （无单独开关，模型本身决定） | — | 通过 `reasoner` model 自动启用 | `DeepSeekChatModel.java` |

**注意**：Spring 没有强行把这些归一成同一个抽象——每家的语义不同（Anthropic 是 budget，OpenAI 是 effort 等级，Google 是 level + budget 双轨，Ollama 是 boolean+level 联合体）。**Lynx 设计时不应做强归一**，但应该至少：

- 让 `chat.Options` 的 provider-specific 通道（目前是 `Extra map`）变得**类型安全**：每个 provider 提供 `WithThinkingBudget(int)` / `WithReasoningEffort(string)` 的 option helper，避免 magic key。
- **更好的做法**：复用现有的「`Options struct + Provider 子结构」`模式（如果有），或让每个 provider 适配器导出强类型的选项 builder。

#### 1.2.2 五家 provider 的推理输出（response 侧）

| Provider | 输出形式 | 关键代码 |
|----------|---------|---------|
| **Anthropic** | `ThinkingBlock`（带 `signature`） + `RedactedThinkingBlock` | `AnthropicChatModel.java:47, 51, 398-404, 982-999` |
| **OpenAI** | response API 的 `reasoning_content` 字段 | `OpenAiApi.java`（`ec717c152` commit 加的 `extraBody` + `reasoningContent`） |
| **Google** | response 的 `thoughtSignatures` part（多 part 结构） | `GoogleGenAiChatModel.java`（thoughtSignature 抽取与多轮回传） |
| **DeepSeek** | `DeepSeekAssistantMessage.reasoningContent`（顶层字段） | `DeepSeekAssistantMessage.java:37-69` |
| **Ollama** | `metadata.thinking` 流式 metadata（修复见 commit `14d603214`） | `OllamaChatModel.java` |

**Spring AI 的统一处理范式**：
1. 把 thinking 输出**当作独立的 `Generation`**（Anthropic 走的路）或**独立的 metadata 字段**（DeepSeek 走的路），而不是混进主 text。
2. `Generation` 的 metadata 带 `isThought=true` 标记，下游聚合器据此分流。

#### 1.2.3 流式聚合：`MessageAggregator.isThought` 分流

`MessageAggregator.java:55-208`（核心代码）：

```java
// 在 doOnNext 中
if (metadata != null && metadata.containsKey("isThought")) {
    var isThought = Boolean.parseBoolean(metadata.get("isThought").toString());
    if (isThought) {
        thoughtsRef.get().append(chatResponse.getResult().getOutput().getText());
    } else {
        outputWithoutThoughtsRef.get().append(chatResponse.getResult().getOutput().getText());
    }
}

// 在 doOnComplete 中
if (!thoughtsRef.get().isEmpty()) {
    messageMetadata.put("thoughts", thoughtsRef.get().toString());
    messageMetadata.put("outputWithoutThoughts", outputWithoutThoughtsRef.get().toString());
}
```

**关键设计**：
- `messageTextContentRef`（**所有** text，包括 thinking）+ `thoughtsRef`（**只有** thinking）+ `outputWithoutThoughtsRef`（**只有**主回答）三套 buffer 并行积累。
- 最终 `AssistantMessage.content` 仍然是「全文（含 thinking）」（向后兼容老调用方），但 metadata 提供了 `thoughts` 和 `outputWithoutThoughts` 两个干净的 view。
- 调用方可选择：渲染时只显示 `outputWithoutThoughts`，调试/审计时拿 `thoughts`。

**Lynx `core/model/chat/response_accumulator.go` 现在做的是**（从前面探索得到）：把所有 text 一股脑拼成一个 `Text` 字段，无任何分流。

#### 1.2.4 ThinkingTagCleaner（在线 fallback 工具）

某些**通过 OpenAI 兼容 API 调用的开源推理模型**（Qwen3、DeepSeek-R1 蒸馏版）会把 thinking 直接嵌在 text 里以 `<think>...</think>` 包围，而不走结构化的 reasoning 字段。Spring AI 提供了 `ThinkingTagCleaner.java:45-187` 处理这种情况：

```java
public class ThinkingTagCleaner implements ResponseTextCleaner {
    private static final List<Pattern> DEFAULT_PATTERNS = Arrays.asList(
        Pattern.compile("(?s)<thinking>.*?</thinking>\\s*", Pattern.CASE_INSENSITIVE),  // Amazon Nova
        Pattern.compile("(?s)<think>.*?</think>\\s*", Pattern.CASE_INSENSITIVE),         // Qwen
        Pattern.compile("(?s)<reasoning>.*?</reasoning>\\s*", Pattern.CASE_INSENSITIVE),
        Pattern.compile("(?s)```thinking.*?```\\s*", Pattern.CASE_INSENSITIVE),         // Markdown
        Pattern.compile("(?s)<!--\\s*thinking:.*?-->\\s*", Pattern.CASE_INSENSITIVE)    // Comment
    );
    // fast-path：如果 text 不含 < 和 `，直接返回（pkg.strings 无开销）
    // 对 BeanOutputConverter 生效——结构化输出前自动清洗
}
```

**Lynx 应做的最小克隆**：在 `core/model/chat/converter/` 或类似位置加一个 `ThinkingTagCleaner` interface + 默认实现，并让 `StructuredParser` 在解析前调用它。**这是结构化输出在推理模型上能跑通的前提**。

### 1.3 Lynx 必须补齐的最小 surface

按依赖关系排序：

#### Step 1：Message 抽象层加 thinking 字段（不可避免的破坏性变更）

```go
// core/model/chat/message.go
type AssistantMessage struct {
    Text             string         `json:"text"`
    Thinking         string         `json:"thinking,omitempty"`           // 🆕 推理过程纯文本
    ThinkingSignature string        `json:"thinking_signature,omitempty"` // 🆕 Anthropic / Google 多轮场景必需
    Media            []*media.Media `json:"media"`
    ToolCalls        []*ToolCall    `json:"tool_calls"`
    Metadata         map[string]any `json:"metadata"`
}
```

或更结构化（推荐）：把 `Text` 改成 `Content []ContentBlock`，每个 block 有 `Type`（text / thinking / tool_use / image），但这是更大的重构，可放到第二阶段。

#### Step 2：`Options` 加 thinking 控制位（每个 provider 各自暴露）

不需要在 `core/model/chat/request.go` 的通用 `Options` 里加（Spring AI 也没归一），而是在每个 provider 的 `XxxOptions` 加：

```go
// models/anthropic/options.go
type Options struct {
    chat.Options
    Thinking *ThinkingConfig // budget 模式
}

// models/openai/options.go
type Options struct {
    chat.Options
    ReasoningEffort string // "low" / "medium" / "high"
}

// models/google/options.go
type Options struct {
    chat.Options
    ThinkingBudget   *int
    IncludeThoughts  bool
    ThinkingLevel    string // "minimal" / "low" / "medium" / "high"
}
```

#### Step 3：流式累积器加 thinking 分流

`response_accumulator.go` 当前的拼接逻辑加上对 `Result.Metadata.Extra["isThought"]` 的判断（或直接通过 `AssistantMessage.Thinking` 字段累积）：

```go
// 简化版
if chunk.AssistantMessage.Thinking != "" {
    thinkingBuf.WriteString(chunk.AssistantMessage.Thinking)
}
if chunk.AssistantMessage.Text != "" {
    textBuf.WriteString(chunk.AssistantMessage.Text)
}
```

#### Step 4：每个 provider adapter 的 build/parse 实现 thinking 通道

具体到代码（按工作量从低到高）：
- **Anthropic** (`models/anthropic/chat.go:219-234` 的 `buildAssistantMsg`)：识别 `type == "thinking"` / `type == "redacted_thinking"` block，提取 text 和 signature。Stream 同理。
- **Google** (`models/google/chat.go:187-211`)：从 response parts 中识别 `thought` / `thoughtSignature`，分流到 `Thinking` 字段。
- **OpenAI** (`models/openai/chat.go:222-258`)：response API 的 `reasoning_content` 字段（如果用 chat completions API 不需要；但 o1/o3 需要走 Responses API，需要新增端点适配——见 §3）。
- **新增 DeepSeek** (`models/deepseek/`)：实现整个 adapter，把 `reasoning_content` 字段映射到 `AssistantMessage.Thinking`。

#### Step 5：`ThinkingTagCleaner` 工具

`pkg/text/` 或 `core/model/chat/converter/` 下新增一个 ~80 行的 Go 等价实现。`StructuredParser` 调用前自动清洗。

#### Step 6（可选）：多轮工具调用回传 signature

Anthropic / Google 在 thinking + 工具调用混合场景，要求**下一轮请求把上一轮的 thinking block（含 signature）原样回传**，否则会因为推理上下文断裂报错（Claude 会拒绝请求）。这是 **§Step 1 的 `ThinkingSignature` 字段必须存在的原因**。

实现：
- `request.go::buildMessages` 把 history 里的 `AssistantMessage.Thinking + ThinkingSignature` 重建成 `thinking_block`。
- Tool middleware 在递归调用时确保 thinking block 不丢失。

---

## 2. Anthropic 2.0 新增能力

### 2.1 Prompt Caching（Lynx 完全无）

Spring AI 2.0 把 Anthropic 的 prompt caching 抽象成可配置策略：

```
AnthropicCacheOptions.java        — 总配置入口
AnthropicCacheStrategy.java       — NONE / ADAPTIVE / LAST_N / AFTER_CHUNK
AnthropicCacheTtl.java            — 5m / 1h
CacheEligibilityResolver.java     — 决定哪些 message 该打 cache_control
CacheBreakpointTracker.java       — 跟踪已打的 breakpoints（Anthropic 限制最多 4 个）
```

**为什么重要**：长 system prompt + 大量 RAG context + 多轮对话场景，prompt cache 能省 90% input token 成本。这是**生产环境的硬刚需**。

**Lynx 现状**：`models/anthropic/chat.go` 的 message build 完全没有 cache_control 概念。

**Lynx 对标方案**：
- 在 `core/media/` 或 `core/model/chat/` 增加 `CacheControl` 元信息（type + ttl），可挂在 `Message.Meta()` 或独立字段。
- Anthropic adapter 在 build 阶段把它转成 `cache_control: {type: "ephemeral"}` block 注入。
- 提供 `AdaptiveCacheStrategy` 默认实现：检测 system + history + RAG 段长度，自动在合适位置打 breakpoint。

### 2.2 Anthropic Skills + Files API（Lynx 无）

Spring AI 2.0 集成了 Claude Skills（XLSX/PPTX/DOCX/PDF 生成）：

```
AnthropicSkill.java              — 4 个内置 skill 枚举
AnthropicSkillType.java          — ANTHROPIC（内置）/ CUSTOM（用户上传）
AnthropicSkillContainer.java     — Skill 容器
AnthropicSkillsResponseHelper.java — 从响应中下载生成的文件
```

加上 Files API 集成（`files-api-2025-04-14` beta）：上传/下载 Claude 生成的二进制文件。

**Lynx 影响**：这是 Claude 专属能力，不必硬要泛化。但如果 Lynx 要支持「让 Claude 生成 Excel/PPT 报告」这种场景，需要：
- 在 `models/anthropic/` 新增 `skills.go`、`files.go`
- ChatOptions 加 `Skills []SkillID` 字段
- Response 加 `GeneratedFiles []File` 元信息

**优先级 P2**：商用场景才需要，可放到第二轮。

### 2.3 Code Execution Tool（Lynx 无）

Anthropic 的 `code-execution-2025-08-25` beta（Anthropic 托管的 Python 沙箱）。Spring AI 通过 beta header 已开通。

**Lynx 影响**：这是「服务端执行的 builtin tool」，与 Lynx 的 ToolMiddleware 模型完全不同（后者是客户端执行）。需要在 Anthropic adapter 层做特例支持。**可暂缓**。

---

## 3. OpenAI 2.0 新增能力

### 3.1 OpenAI Responses API（o1/o3/gpt-5 必经路径）

OpenAI 的 Responses API（`/v1/responses`）是 chat completions 的下一代，专为推理模型设计：
- 内置多轮推理状态保持（不需要 client 回传 reasoning blocks）
- 原生支持 `reasoning_effort`、`reasoning_summary`、`reasoning_content` 字段

Spring AI 通过 `OpenAiApi` 的 `extraBody` + `reasoningContent` 字段已支持（commit `ec717c152`）。

**Lynx 现状**：`models/openai/` 目前只走 `/v1/chat/completions`，没有 Responses API 适配。**这意味着 Lynx 用户调 o1/o3 时拿不到 reasoning content**，且无法用最新 reasoning 控制参数。

**修复方案**：
- 在 `models/openai/internal/api/` 加 `responses_api.go`
- 提供 `OpenAIChatModel.WithResponsesAPI(true)` 切换端点
- 把 `reasoning_content` 映射到 `AssistantMessage.Thinking`（依赖 §1.3 Step 1）

### 3.2 Audio Output（output modalities）

Spring AI 2.0 在 chat 主路径上支持 audio 输出：

```java
// OpenAiChatOptions.java:82-84
private List<String> outputModalities;     // ["text", "audio"]
private OpenAiAudio.AudioParameters outputAudio;  // voice, format
```

模型在同一个 chat completion call 中**同时生成 text + audio**（gpt-4o-audio-preview）。

**Lynx 现状**：`models/openai/audio_tts.go` 和 `audio_transcription.go` 是**独立的 TTS / STT 模型**，与 chat 解耦。无法在 chat 调用里直接拿到 audio output。

**修复方案**：`OpenAIChatOptions` 加 `OutputModalities []string` + `OutputAudio AudioConfig`，response 的 `Result` 携带 `Audio []byte` 字段（或走 `Media` 通道）。

### 3.3 Batch API（Lynx 无）

OpenAI Batch API（异步批量推理，50% 折扣）。Spring AI 有相应的 helper（`OpenAiApi.uploadFile / createBatch / retrieveBatch`）。

**Lynx 现状**：`models/openai/` 无 `batch.go`。

**Lynx 影响**：离线评估、大规模数据标注场景才需要。**P3 可暂缓**。

---

## 4. Google Gemini 2.0 新增能力

### 4.1 Cached Content API（Lynx 无）

```java
// GoogleGenAiChatOptions.java
private String cachedContentName;     // 引用已缓存的 content
private Boolean useCachedContent;
private Long autoCacheThreshold;      // 自动缓存阈值（tokens）
private Long autoCacheTtl;            // TTL
```

类似 Anthropic prompt caching 但走显式 `CachedContent` 资源——**长 system prompt 多次复用时的硬刚需**。

**Lynx 现状**：`models/google/` 无任何缓存机制。

### 4.2 Extended Usage Metadata（Lynx 无）

`GoogleGenAiUsage.java:30-156` 把 usage 拆得非常细：
- `thoughtsTokenCount`（thinking 消耗）
- `cachedContentTokenCount`（cache 命中）
- `toolUsePromptTokenCount`（工具调用消耗）
- `promptTokensDetails` / `candidatesTokensDetails` / `cacheTokensDetails`（按 modality 拆解：text / image / video / audio）
- `trafficType`（PAYG vs Provisioned）

**Lynx 现状**：`core/model/usage.go` 只有 `PromptTokens / CompletionTokens / TotalTokens`。

**修复方案**：让 `Usage` 接口/结构开放 `Extra map[string]any` 通道（如果还没有），Google adapter 把这些字段塞进去；同时核心 Usage 加 nullable 的 `ThoughtsTokens *int`、`CachedTokens *int`。

### 4.3 Thought Signatures（多轮工具调用必需）

`GoogleGenAiChatModel.java` 中处理：Gemini 3 Pro 推理模型在 function calling 多轮中要求把 `thoughtSignature` 原样回传。Spring AI 已实现完整生命周期（见 `GoogleGenAiThoughtSignatureLifecycleIT.java`）。

**Lynx 现状**：`models/google/chat.go:187-211` 完全没有 signature 概念，**多轮 + 推理 + 工具调用场景必坏**。

**修复方案**：见 §1.3 Step 6。

---

## 5. 跨 Provider 的 Cross-cutting 能力

### 5.1 BeanOutputConverter 与 thinking 兼容（Lynx 需补）

Spring AI 2.0 的 `BeanOutputConverter` 在解析 JSON/POJO 前会调用 `ThinkingTagCleaner` 清洗输入文本。这样 Qwen3 / DeepSeek-R1 的输出（含 `<think>...</think>` 包裹）能直接做结构化解析。

**Lynx 现状**：`core/model/chat/structured_parser.go`（如果存在）应**默认接 `ThinkingTagCleaner`**——否则结构化输出在主流推理模型上会全部失败。

### 5.2 ChatClientMessageAggregator（Lynx 需补）

`ChatClientMessageAggregator.java` 是 `MessageAggregator` 的 ChatClient 层包装，把流式响应聚合成单个 `ChatClientResponse`，**保留 advisor context**。

**Lynx 现状**：流式中间件链没有终点聚合工具——用户要从 `iter.Seq2` 拿到等价于 call 模式的最终 `*Response` 必须自己写循环。

**修复方案**：在 `core/model/chat/client.go` 提供 `(streamer ClientStreamer).Aggregate(ctx) (*Response, error)` 便利方法，内部调用 `response_accumulator`。

---

## 6. 模型适配器矩阵差距

Spring AI 2.0 的 model adapter 列表（与 Lynx 对比）：

| Provider | Spring AI 2.0 | Lynx | 备注 |
|----------|--------------|------|------|
| Anthropic | ✅ 含 thinking + skills + caching | ⚠️ 基础 chat，缺 thinking | §1, §2 |
| OpenAI（chat completions） | ✅ | ✅ | OK |
| OpenAI（Responses API） | ✅ | ❌ | §3.1 |
| Google Gemini | ✅ 含 thinking + cache | ⚠️ 基础 chat，缺 thinking + cache | §1, §4 |
| **DeepSeek** | ✅ 独立模块（reasoning_content） | ❌ **无适配器** | §1.2.2 |
| **Ollama** | ✅ 独立模块（含 ThinkOption） | ❌ **无适配器** | §1.2.1 |
| Azure OpenAI | ✅ | ❌ | 可暂缓 |
| Bedrock | ✅ | ❌ | 可暂缓 |
| MistralAI | ✅ | ❌ | 可暂缓 |
| Vertex AI | ✅ | ❌ | 与 Google Gemini 不同入口 |
| Watsonx | ✅ | ❌ | 可暂缓 |
| ZhiPu / MiniMax / Moonshot | ✅ | ❌ | 国内厂商可考虑 |

**Lynx 优先级建议**：
1. **必加**：DeepSeek（国内推理模型旗舰）+ Ollama（本地推理通用入口）
2. **可选**：Azure OpenAI（企业用户多）、ZhiPu（国内）
3. **不加**：剩余的等真有用户需求

---

## 7. 落地路线图（与原 SPRING_AI_COMPARISON.md §10 不冲突）

把本文新增的能力面**插入**原文档的优先级序列：

### 第一阶段（紧急，推理模型时代刚需，~2-3 周）
- **§1.3 Step 1-4**：`AssistantMessage.Thinking` + `ThinkingSignature` 字段、Options 推理控制、流式 thinking 分流、Anthropic/Google/OpenAI adapter thinking 通道
- **§5.1 ThinkingTagCleaner**：~80 行 Go 实现 + 接到 StructuredParser
- **§3.1 OpenAI Responses API**：补 o1/o3 必经路径
- **§4.3 Google thoughtSignature 多轮**：保证多轮工具调用不坏

### 第二阶段（生产硬需求，~3-4 周）
- **§2.1 Anthropic Prompt Caching**：CacheStrategy + CacheBreakpointTracker
- **§4.1 Google Cached Content API**
- **§4.2 Extended Usage Metadata**（thinking tokens / cache tokens）
- **§5.2 ChatClientMessageAggregator** 等价品
- **§1.3 Step 5-6**：signature 多轮回传完整链路

### 第三阶段（覆盖面拓展，~4-6 周）
- DeepSeek adapter（§1.2.2，含 reasoning_content）
- Ollama adapter（§1.2.1，含 ThinkOption 双模式）
- §3.2 OpenAI Audio Output 整合到 chat 主路径

### 第四阶段（与原 §10 第四阶段合并）
- ObservationRegistry / OTel
- 双向 AdvisorContext

---

## 8. 与原 `SPRING_AI_COMPARISON.md` 的关系

| 原文档章节 | 本文新增 | 关系 |
|----------|---------|------|
| §0-§10（架构对齐） | — | 仍然有效，本文不覆盖 |
| §11（Spring AI 2.0 新生态） | **本文 §1-§5 全部** | 本文是 §11 的深度展开 |
| §12（总评） | — | 本文重申同一判断：**Lynx 抓骨架但缺新能力** |

**两文档配套阅读**：原文档讲「**结构哪里设计倒退**」，本文讲「**功能哪里直接缺失**」。修复路径正交，可并行推进。

---

## 9. 一页式总结

| Lynx 现状 | Spring AI 2.0 现状 | 差距严重度 |
|----------|-------------------|----------|
| `AssistantMessage` 只有 Text/Media/ToolCalls/Metadata | thinking + signature 一等公民，5 家 provider 全栈支持 | 🔴 P0 |
| `Options` 无 reasoning 控制 | 5 家 provider 各自暴露 budget/effort/level 类型化选项 | 🔴 P0 |
| 流式累积只拼 text | `MessageAggregator` 用 `isThought` 三 buffer 分流 | 🔴 P0 |
| 无结构化输出前清洗 | `ThinkingTagCleaner` 5 种 pattern + fast-path | 🟠 P1 |
| OpenAI 只走 chat completions | Responses API 已就位（o1/o3 必经） | 🟠 P1 |
| 无 prompt caching | Anthropic 4 种策略 + Google CachedContent | 🟠 P1 |
| 无 DeepSeek / Ollama adapter | 都有独立模块 | 🟢 P3 |
| Usage 只有 3 字段 | thinking/cache/tool/modality 拆解齐全 | 🟡 P2 |

**最值得回填的一行字结论**：

> **先把 thinking 通道在 Message + Options + Aggregator + 4 个 adapter 上打通，整个推理模型时代的可用性就回来了。这个改动的代码量在 600-1000 行内，但是 Lynx 是否值得用的分水岭。**

---

*配套阅读：`SPRING_AI_COMPARISON.md`（架构层倒退点）、`IMPROVEMENTS.md`（其他战术清单）、`ARCHITECTURE.md`（整体架构）。*
