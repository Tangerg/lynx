# Lynx vs Spring AI 2.0 — Reasoning 落地后能力差距快照（2026-04-29）

> **基线**：
> - Lynx HEAD = `5b86d8a`，分支 `feat/thinking-support`（含 reasoning 完整落地 + pkg 模块重构）
> - Spring AI HEAD = `b9d1c5303`（`2.0.0-SNAPSHOT`）
> - 上一份分析 `SPRING_AI_CAPABILITY_GAPS.md`（2026-04-26）的 §1 Reasoning 章节**已被本文取代**；其余章节继续生效
>
> **写作目的**：reasoning 字段刚刚落地（一周内 5 个 commits），同时 Spring AI 上游也在演进（最近 30 个 commits 含 MCP SDK 升级 / Spring Boot 4.1 / Azure OpenAI 移除等）。重新做一次定位，消除前文已经过时的判断，并对剩余真实差距重排优先级。

---

## 0. 一页式总览

| 维度 | 上一次（2026-04-26） | 当前 | 变化方向 |
|-----|------------------|------|---------|
| **Reasoning / thinking 抽象** | 🔴 双模式 magic key 抽象不统一 | ✅ `AssistantMessage.Reasoning string` 一等公民字段 | **Lynx 反超 Spring AI**（后者仍用 metadata `isThought`）|
| `Usage.ReasoningTokens` | ❌ 不暴露 | ✅ 暴露 nullable `*int64` | **Lynx 反超**（Spring AI Usage 还没有 `getReasoningTokens()`）|
| Anthropic prompt caching | ❌ | ❌ | 持续落后（Spring AI 已有完整 4 策略 + breakpoint tracker）|
| Anthropic Skills + Files API | ❌ | ❌ | 持续落后 |
| Anthropic Web Search Tool / Citation | ❌ | ❌ | **新发现** Spring AI 已支持 |
| MCP 全家桶 | ❌ | ❌ | Spring AI MCP SDK 已升级到 2.0.0-M2 |
| 持久化 ChatMemory（6 种后端）| ❌（仅 in-memory）| ❌ | 持续落后 |
| OpenAI Responses API | ❌ | ❌ | 双方都没有（Spring AI 也还没落） |
| AbstractToolCallingChatModel | – | ❌ | Spring AI 自己 P0-2 也未落地 |
| Bedrock Converse / Vertex AI Embedding | ❌ | ❌ | Spring AI 已支持 |
| Filter Converter 基类（AbstractFilterExpressionConverter）| ❌ | ❌ | 5 store × ~700 LOC 仍然 |
| RAG QueryRouter / DocumentJoiner | ❌ | ❌ | Spring AI 在 `retrieval/join/` 已成型 |

**两个反超点**说明：reasoning + reasoning_tokens 落地后，Lynx 在「推理模型语义」这一条线上**比 Spring AI 现状（2026-04-29）更前**。这不是因为 Lynx 跑得快，而是因为 Spring AI 自己 P0-1 提案还没落地——他们仍在用 `MessageAggregator` 检查 `metadata.isThought` 这套 v0 时代的 magic key 模式。

**还有一个被低估的 Lynx 设计强项**：`chat.Options.Extra` 装 provider 原生 SDK params，adapter 用「先取 Extra 再用通用 Options 覆盖」语义构造请求。这意味着任何 SDK 字段（cache_control、prefix、extra_body、provider-specific tools…）用户都能直接写上去，**不需要 Lynx 为每个 provider feature 再造一层 strategy / option 类**。本文 §3.3 / §3.9 都因为这个机制做了大幅删减——能不抄 Spring 就不抄。

---

## 1. Reasoning 落地后的代码现状

### 1.1 Lynx 当前 reasoning 表面

```go
// core/model/chat/message.go:148-181
type AssistantMessage struct {
    Text      string         `json:"text"`
    Reasoning string         `json:"reasoning,omitempty"`  // ← 一等公民
    Media     []*media.Media `json:"media"`
    ToolCalls []*ToolCall    `json:"tool_calls"`
    Metadata  map[string]any `json:"metadata"`
}

func (a *AssistantMessage) HasReasoning() bool {
    return a != nil && a.Reasoning != ""
}
```

```go
// core/model/usage.go:7-30
type Usage struct {
    PromptTokens     int64  `json:"prompt_tokens"`
    CompletionTokens int64  `json:"completion_tokens"`
    ReasoningTokens  *int64 `json:"reasoning_tokens,omitempty"`  // ← 一等公民
    OriginalUsage    any    `json:"original_usage,omitempty"`
}
```

Provider-specific signatures 通过文档化的 `lynx:chat:<provider>:<concept>` 命名空间放在 `Metadata` 中，定义在各 provider 自己的包内：
- `models/anthropic/metadata.go` — `MetaReasoningSignature` / `MetaRedactedReasoning`
- 未来 Google 适配器会有 `models/google/metadata.go` — `MetaReasoningSignatures`

**chat 包对 provider 零知识**——`grep -r "anthropic\|openai\|google" core/model/chat/` 零命中。

### 1.2 Spring AI 当前 reasoning 表面

```java
// spring-ai-model/.../chat/messages/AssistantMessage.java（实际）
public class AssistantMessage extends AbstractMessage implements MediaContent {
    private final List<ToolCall> toolCalls;
    protected final List<Media> media;
    // 没有 thinking 或 reasoning 字段
    // content 由父类 AbstractMessage 提供（textContent + properties Map）
}
```

```java
// spring-ai-model/.../chat/metadata/Usage.java
public interface Usage {
    Integer getPromptTokens();
    Integer getCompletionTokens();
    default Integer getTotalTokens() { ... }
    @Nullable Object getNativeUsage();
    // 2.0 新增的两个：
    default @Nullable Long getCacheReadInputTokens() { return null; }
    default @Nullable Long getCacheWriteInputTokens() { return null; }
    // ❌ 仍然没有 getReasoningTokens()
}
```

Reasoning 在 Spring AI 仍走两套并行：

1. **Anthropic 多 Generation 模式**：thinking block 成为独立 `Generation`，`signature` 进 `AssistantMessage.properties` Map（key 字符串 `"signature"`）。
2. **DeepSeek 子类化模式**：`DeepSeekAssistantMessage extends AssistantMessage` 加 `reasoningContent` 字段。
3. **MessageAggregator** 通过检查 `metadata.containsKey("isThought")` 分桶，最后写到聚合产物的 `metadata.thoughts` / `metadata.outputWithoutThoughts`。

Spring AI 自己在 `doc/spring-ai-architecture/05-optimization-suggestions.md` P0-1 中提案要把这些收敛成 `ReasoningContent` record，**但 P0-1 至今没有落地**（HEAD `b9d1c5303` 上 `AssistantMessage.java` 仍只有 toolCalls + media）。

### 1.3 直接对比

| 概念 | Lynx | Spring AI 现状 | Spring AI 提案 |
|-----|------|--------------|--------------|
| AssistantMessage 字段 | `Text + Reasoning` | content 单字段 | + `List<ReasoningContent>` |
| Usage reasoning 维度 | `ReasoningTokens *int64` | 无 | + `Long getReasoningTokens()` |
| Magic key | 无（chat 包零 magic key）| `"signature"`/`"isThought"`/`"thoughts"`/`"outputWithoutThoughts"` 等 9 个 | 计划清理 |
| 业务侧 API | `msg.Reasoning` | 遍历 generations + 看 properties + 可能 cast 子类 | `getReasoning()` |
| 命名 | `Reasoning`（行业收敛词）| `thinking` + `reasoning` 混用 | 倾向 `reasoning` |
| 多 thinking block | 拼接成单 String | `List<Generation>` 保留 | `List<ReasoningContent>` 保留 |
| ThinkingTagCleaner | ❌ 不在 chat 包（adapter 责任）| ✅ `spring-ai-model/.../converter/ThinkingTagCleaner.java` | – |

**取舍说明**：
- 多 block 表达力 Lynx 主动放弃（`interleaved-thinking` beta 不支持），换来抽象简洁
- 业界 Lynx 的设计形态最接近 langchain4j（`AiMessage.thinking string` + `attributes map`）；命名采用 reasoning 是 Lynx 比 langchain4j 走得更远的一步

---

## 2. Lynx 当前**反超** Spring AI 的点（罕见但真实）

### 2.1 🏆 Reasoning 一等公民
原因：langchain4j 形态 + reasoning 命名收敛 = Spring AI 还在 P0-1 backlog 里讨论的设计在 Lynx 已经是 `git push origin feat/thinking-support` 的 HEAD 现状。

### 2.2 🏆 `Usage.ReasoningTokens` 类型化
Spring AI 文档（`doc/spring-ai-architecture/04-reasoning-thinking.md` §4.9.3）列出的"成本计算错误"痛点（OpenAI 的 reasoning_tokens 实际计费但 `Usage` 看不到）在 Lynx 已修复。

### 2.3 🏆 chat 包零 provider 知识
Lynx 把 provider-specific metadata key 完全下沉到 `models/<provider>/metadata.go`。Spring AI 的 `MessageAggregator` 还硬编码识别 Google 的 `"isThought"` key。

### 2.4 🏆 iter.Seq2 优势保持
仍然适用——Spring AI 仍依赖 Reactor `Flux` + `contextView`/`ObservationThreadLocalAccessor` 这一套 ceremony；Lynx 流式中间件就是普通迭代器（`SPRING_AI_COMPARISON.md §7.1` 已记录，不变）。

### 2.5 🏆 ISP 拆接口
`memory.Store = Reader + Writer + Clearer`、`VectorStore = Creator + Retriever + Deleter + Info`——Spring AI 单接口仍是巨型接口，没变。

---

## 3. Lynx 仍然落后的能力（按严重度排序）

### 🔴 P0：Spring AI 自己也在 backlog 但有清晰提案

#### 3.1 ToolExecutionMode + ToolInvoker（Lynx 已避开 Spring AI P0-2 大坑）
Spring AI 提案 `05-optimization-suggestions.md` P0-2：抽出 14 个 provider 共享的 tool-call 循环到 `AbstractToolCallingChatModel`。

Lynx 现状：`core/model/chat/tool_middleware.go` 已经把 tool 循环抽成单一 `ToolMiddleware`（commit `8e58479`），**14-vs-1 这个坑 Lynx 直接避开**——工具循环代码只有一份，所有 provider 共用。

仍缺：
- `ToolExecutionMode` 枚举（Internal / External / Custom）
- `ToolInvoker` 注入点（user 提供自定义并行 / retry 策略）
- 工具循环 max iterations 上限（防失控）

**收益**：解锁 agentic 编排（external execution 转给 MCP server / 其他 agent），且加保险防 prompt 攻击账单失控。

#### ~~3.2 Middleware Ordering~~ — **撤销**

> **2026-04-29 修订**：本条移除。
>
> 原诉求来自 `SPRING_AI_COMPARISON.md §2`，把 Spring 的 `Advisor.getOrder()` + `DEFAULT_CHAT_MEMORY_PRECEDENCE_ORDER` 当作 Lynx 中间件该补的能力。这个判断对 Java/Spring 成立、对 Go **不成立**。
>
> **理由**：Spring 之所以需要 `Order` 元数据，是因为 bean 容器**自动发现** advisor 实例（@Component / @Bean），调用方对加载顺序失控，必须靠数字 / 注解强制排序。Go 没有这个问题——中间件链是调用方**显式**用 `Use(a, b, c)` 组合，注册顺序就是执行顺序，没有第二个真相源头。
>
> **Go 生态共识**：`net/http.Handler` 装饰器、Gin `r.Use`、Echo `e.Use`、Fiber `app.Use`、Chi `r.Use`——都是注册顺序即执行顺序。Lynx `client.WithMiddlewares(NewMemoryMiddleware(), NewToolMiddleware())` 完全对齐这个范式。
>
> 真正可能出问题的是「文档没说清 memory middleware 应该靠外」——这是文档问题，不是抽象问题。`SPRING_AI_COMPARISON.md §2` 的对应条目也应当一并修订（已计入 §6 历史文档关系表）。

### 🟠 P1：Spring AI 已经形成成熟代码、Lynx 完全空白

#### 3.3 Anthropic Prompt Caching（精简实现，不抄 Spring 4 类）
Spring AI 在 `models/spring-ai-anthropic/` 用 4 个类 + 1 个 enum 实现：
```
AnthropicCacheOptions.java
AnthropicCacheStrategy.java     (NONE / ADAPTIVE / LAST_N / AFTER_CHUNK)
AnthropicCacheTtl.java          (5m / 1h)
CacheBreakpointTracker.java
CacheEligibilityResolver.java
```

**Lynx 不需要走这条路**——本项目设计里有更轻的解：`chat.Options.Extra` 装 provider 原生 SDK params（`getOptionsParams[anthropicsdk.MessageNewParams](opts)`），用户可以直接在 SDK 类型上设 `cache_control`。Anthropic SDK 的 `TextBlockParam` / `ToolParam` / `ToolResultBlockParam` 等都有 `CacheControl` 字段，用户预设即可生效。

**当前阻塞点**：`models/anthropic/chat.go::buildApiChatRequest` 把用户预设的 `params.System` / `params.Messages` / `params.Tools` **直接 overwrite** 掉（line 191-193, line 93-96）。意味着用户在 Extra 里精心预填的 `cache_control` 会被通用 builder 清掉。

**修复方案**（~30 行改动）：
- 让 `buildSystem` / `buildMsgs` / `buildToolParams` 检测 user 是否已经在 Extra params 里预填——预填则跳过、不预填则按现有逻辑构建
- 或更简单：把"预设兜底"语义改成 merge——adapter 构建后做 cache_control 透传

**生产痛点照旧成立**：长 system prompt + RAG context + 多轮对话不开 cache 成本 5-10 倍，是商用刚需。但解决方式是**保护现有 Extra 通道不被 overwrite**，不是再造一个 Spring 风格的 strategy 抽象层。

> 顺带，`Usage.getCacheReadInputTokens()` / `getCacheWriteInputTokens()` 类似 ReasoningTokens 的处理——给 `core/model/usage.go` 加 `CacheReadInputTokens` / `CacheWriteInputTokens *int64` 即可，~10 行。Anthropic adapter 从 `resp.Usage.CacheCreationInputTokens` / `CacheReadInputTokens` 抽出。

#### 3.4 Anthropic Skills + Files API
Spring AI 文件（同模块）：
```
AnthropicSkill.java              (XLSX / PPTX / DOCX / PDF 4 个内置)
AnthropicSkillType.java          (ANTHROPIC / CUSTOM)
AnthropicSkillContainer.java
AnthropicSkillRecord.java
AnthropicSkillsResponseHelper.java
```

让 Claude 直接生成 Excel / PPT / Word / PDF 报告——电商分析、财报生成等场景刚需。

#### 3.5 Anthropic Web Search Tool + Citations（**新发现**）
本次扫描发现 Spring AI 还有：
- `AnthropicWebSearchTool.java` / `AnthropicWebSearchResult.java`
- `Citation.java` / `AnthropicCitationDocument.java`

这是 Anthropic Beta API（`web_search_20250305`）的客户端封装。Lynx 完全没有。

#### 3.6 OpenAI Responses API
两边都没有，但**长期 P1**：o1/o3/gpt-5 在 Chat Completions API 不返回 reasoning text，只能走 `/v1/responses`。Lynx 现在 `models/openai/chat.go` 仅走 `/v1/chat/completions`，意味着调用 OpenAI 推理模型拿不到思考过程。

DeepSeek-R1 走 OpenAI-compatible 端点暴露 `reasoning_content`，已经被 Lynx OpenAI adapter 通过 `JSON.ExtraFields` 抓出来——这覆盖了主要的开源推理模型场景。但 OpenAI 官方 o-series 仍待补 Responses API 适配。

### 🟡 P2：生态级缺口

#### 3.7 MCP 全家桶
Spring AI 有：
```
mcp/mcp-annotations          (注解驱动 tool/resource/prompt)
mcp/transport/mcp-spring-webmvc + mcp-spring-webflux
mcp/common
auto-configurations/mcp/...
spring-ai-spring-boot-starters/spring-ai-starter-mcp-client[-webflux]
spring-ai-spring-boot-starters/spring-ai-starter-mcp-server[-webflux|-webmvc]
```

最近一次 commit `5a9335b71` 升级到 MCP SDK 2.0.0-M2，包含 breaking changes。

Lynx：`doc/agent/README.md` 规划过 `agents/mcp/` 目录但**没有任何代码落地**。这是 agentic 应用的关键集成点。

#### 3.8 持久化 ChatMemory（6 种后端）
Spring AI 有：
```
memory/repository/spring-ai-model-chat-memory-repository-jdbc
memory/repository/spring-ai-model-chat-memory-repository-mongodb
memory/repository/spring-ai-model-chat-memory-repository-neo4j
memory/repository/spring-ai-model-chat-memory-repository-redis
memory/repository/spring-ai-model-chat-memory-repository-cassandra
memory/repository/spring-ai-model-chat-memory-repository-cosmos-db
```

每个都有对应的 Spring Boot starter。

Lynx：仅 `core/model/chat/memory/in_memory.go` + `message_window.go`——纯内存，重启即失。

**Lynx 对标方案**：建立 `memories/` 顶层 module，子包 `redis/` / `postgres/` / `mongo/`，复用现有 `memory.Store = Reader + Writer + Clearer` ISP 接口（这是 Lynx 已经赢的设计点，对接后端友好）。

#### 3.9 模型适配器矩阵
Spring AI 当前 15 个模型 adapter：
```
spring-ai-anthropic / spring-ai-bedrock / spring-ai-bedrock-converse
spring-ai-deepseek / spring-ai-elevenlabs / spring-ai-google-genai
spring-ai-google-genai-embedding / spring-ai-minimax / spring-ai-mistral-ai
spring-ai-ollama / spring-ai-openai / spring-ai-postgresml
spring-ai-stability-ai / spring-ai-transformers / spring-ai-vertex-ai-embedding
```

**注意 1**：Spring AI **2.0 移除了独立的 Azure OpenAI 模块**（commits `3f5255f71` / `888292c07` / `40122da17`），把 Azure 用例合并到 `spring-ai-openai`——和我们下面对 DeepSeek 的判断同源。

**注意 2（重大撤销）**：~~原来在第一阶段路线图列出的 "DeepSeek adapter"~~ **不需要**单独建包。Lynx 的 OpenAI adapter 通过 `option.WithBaseURL("https://api.deepseek.com/v1")` 加上现有的 `JSON.ExtraFields["reasoning_content"]` 抽取已经 100% 覆盖 DeepSeek 的 chat 用例。`prefix` 等 DeepSeek 独有字段也能直接通过 `chat.Options.Extra` 装 `*openai.ChatCompletionNewParams` 时预填走 ExtraBody——不需要单独的包。**vLLM / Together AI / Anyscale / Groq / Fireworks / DeepInfra 等所有 OpenAI-compat 端点同理**——Lynx 一个 OpenAI adapter 就够了。Spring AI 之所以单独建 spring-ai-deepseek 是因为他们的 ToolCallback / response parsing 都要 hard-code，Lynx 的 ExtraFields 通用读取已经把这条路打通。

Lynx 当前 3 个 chat adapter（anthropic / openai / google）+ 0 个独立 embedding adapter。

**真实优先级**（精简后）：
1. **Ollama**：本地模型通用入口。注意它**不能**简单走 OpenAI-compat——Ollama 有自己的 think option / pull-model / embed / 模型生命周期管理，用 OpenAI 端点丢失这些。建议独立 module。
2. **Bedrock Converse**：AWS 用户重要，独有 SDK 体系。
3. **MiniMax / ZhiPu / Moonshot**：国内厂商。其中 MiniMax / 月之暗面已经 OpenAI-compat → 走 OpenAI adapter 即可；ZhiPu 自己 SDK → 视情况

### 🟡 P2：Filter / RAG 架构倒退（旧账，未变）

#### 3.10 AbstractFilterExpressionConverter 基类（`SPRING_AI_COMPARISON.md §5` 旧账）
Spring AI 有 `AbstractFilterExpressionConverter` 模板方法基类。Lynx 5 个 store visitor 各自从 0 写 600-800 LOC，~3300 LOC vs Spring 的 ~750。

修复方案不变：`core/vectorstore/filter/visitors/BaseVisitor`。

#### 3.11 RAG QueryRouter + DocumentJoiner
Spring AI 有完整模块：
```
rag/preretrieval/query/expansion/MultiQueryExpander.java
rag/preretrieval/query/transformation/Compression/Rewrite/Translation
rag/retrieval/search/VectorStoreDocumentRetriever
rag/retrieval/join/ConcatenationDocumentJoiner.java  ← Lynx 缺
rag/postretrieval/document/DocumentPostProcessor.java
rag/generation/augmentation/ContextualQueryAugmenter
```

Lynx 缺 `DocumentJoiner` / `QueryRouter`，且 `pipeline.go` 把 5 个阶段硬编码闭合（旧账）。修复路线在 `SPRING_AI_COMPARISON.md §4.4`。

### 🟢 P3：长尾能力

#### 3.12 OpenAI Audio Output（output modalities 集成）
Spring AI 在 chat 主路径上支持同时返回 text + audio（gpt-4o-audio-preview）。Lynx 的 `audio_tts.go` / `audio_transcription.go` 是独立 model，无法在 chat call 里直接拿 audio 输出。

#### 3.13 Google Gemini 高级特性
- CachedContent API（cachedContentName / autoCacheThreshold / autoCacheTtl）
- Extended Usage Metadata（thoughtsTokenCount / cachedContentTokenCount / toolUsePromptTokenCount）
- ThinkingLevel enum（MINIMAL / LOW / MEDIUM / HIGH，per-model）
- thoughtSignatures 多轮恢复（function calling 必需）

Lynx Google adapter 当前是基础 chat，全部缺失。

#### 3.14 OpenAI Image / TTS / STT / Moderation 已有
Lynx 对齐情况：✅ Image / ✅ TTS / ✅ STT / ✅ Moderation 都有。

#### 3.15 Embedding adapter
Spring AI 有 OpenAI / Google / Vertex / PostgresML / Transformers 多个 embedding model。Lynx 仅 OpenAI embedding。

#### 3.16 OpenTelemetry 集成
Spring AI 已有完整的 ChatModelObservation / ChatClientObservation 体系（gen_ai semantic conventions）。Lynx：`otelbridge/` 目录有 slog exporter skeleton，未与 chat / RAG / vector store 集成。

#### 3.17 SafeGuardAdvisor / StructuredOutputValidationAdvisor 等
Spring AI advisor 模块默认提供：
```
SafeGuardAdvisor.java
SimpleLoggerAdvisor.java
StructuredOutputValidationAdvisor.java
ToolCallAdvisor.java
LastMaxTokenSizeContentPurger.java
PromptChatMemoryAdvisor.java + MessageChatMemoryAdvisor.java
```

Lynx 仅有 `ToolMiddleware` + `MemoryMiddleware`。可以补的常用 middleware 列表清晰。

---

## 4. Spring AI 自 2026-04-26 以来的新动向

### 4.1 已落地变化（最近 30 个 commits）

| Commit | 内容 | Lynx 影响 |
|--------|-----|---------|
| `b9d1c5303` | Remove Azure OpenAI from BOM | – Lynx 没有 Azure adapter |
| `5a9335b71` | MCP SDK 升级 2.0.0-M2 + breaking changes 文档 | 影响未来 Lynx 接 MCP 的 SDK 选型 |
| `12e1e3d92` | VectorStoreChatMemoryAdvisor 加 conversationId filter 处理 | Lynx 没有 vector-store memory advisor |
| `029173fba` | Filter expression converter key handling 修复 | 提醒：Lynx 5 个 visitor 是否同样有 bug 需要排查 |
| `dd820e697` | StopSequences defensive copy | 检查 `chat.MergeOptions` 对 `Stop` slice 的处理 |
| `e5383b219` | 自定义 StructuredOutputConverter 参与 Native Structured Output | Lynx 的 `StructuredParser` 当前不与 Native 对接 |
| `7538183b5` | 升级 Spring Boot 4.1.0-RC1 | – |
| `0f9340fbb` | Anthropic 模块迁移文档 | 参考价值：本次 reasoning 重构后 Lynx 也该写迁移文档 |
| `4ebf453d4` / `d4c6389cd` | Bedrock 模块 null-safety | – Lynx 不影响 |
| `40122da17` / `3f5255f71` / `888292c07` | Azure OpenAI 模块整体移除 | 设计参考：consolidation > 重复 |

### 4.2 仍未落地的 Spring AI P0-1 / P0-2

> Spring AI 自己 `doc/spring-ai-architecture/05-optimization-suggestions.md` 列出的 **P0-1（统一 ReasoningContent）** 和 **P0-2（AbstractToolCallingChatModel）** 至今未落地——HEAD 上代码与 1 个月前没有结构性变化。这意味着：
>
> - Lynx 的 reasoning 设计可以**长期保持反超**（除非 Spring AI 突然推 P0-1）
> - Lynx 不必紧急抄 AbstractToolCallingChatModel——Lynx 的 ToolMiddleware 单点设计已经避开这个坑

---

## 5. 推荐路线图（按 ROI 排序）

### 第一阶段（~1-2 周）：填生产级"硬刚需"
1. **保护 Anthropic adapter 的 Extra 通道不被 overwrite**（§3.3）—— ~30 行让 user 预填 `cache_control` 真正生效
2. **`Usage.CacheReadInputTokens` / `CacheWriteInputTokens`**（§3.3 末尾）—— ~10 行加字段 + adapter 抽取
3. **Ollama adapter**（§3.9 #1）—— 本地模型 + think 模式覆盖（注意：不能简单 OpenAI-compat）
4. **Memory middleware 文档明确链中位置**（来自 §3.2 撤销决策的衍生事项）—— 补 godoc / README 一段
5. **OpenAI adapter 接 DeepSeek/vLLM 的 README 示例**（来自 §3.9 注意 2）—— 不写代码、写一段「这是怎么用 OpenAI adapter 接其他 OpenAI-compat 端点」的文档，避免后人重复提"我们要不要单独建一个 DeepSeek adapter"

### 第二阶段（~3-4 周）：解锁 agentic 场景
5. **MCP client + server**（§3.7）—— 高优先，独立 module 走 `agents/mcp/`
6. **ToolExecutionMode + ToolInvoker**（§3.1）—— 配合 MCP 落地
7. **持久化 Memory（Redis / Postgres）**（§3.8）—— 单后端先做

### 第三阶段（~4-6 周）：清理架构倒退
8. **AbstractFilterExpressionConverter / BaseVisitor**（§3.10）—— 减 ~2500 LOC
9. **RAG QueryRouter + DocumentJoiner**（§3.11）
10. **OpenAI Responses API**（§3.6）—— o-series reasoning 解锁

### 第四阶段（~视需求）：长尾完善
11. Google Gemini 高级特性（§3.13）
12. Bedrock Converse（§3.9 #3）
13. OpenTelemetry 全面接入（§3.16）
14. SafeGuard / Logger / OutputValidation advisors（§3.17）
15. Anthropic Skills + Web Search + Citations（§3.4 / §3.5）

---

## 6. 与历史文档的关系

| 文档 | 状态 |
|-----|------|
| `SPRING_AI_COMPARISON.md`（2026-04-20）| §3（ToolCallingManager）/ §5（Filter Visitor）/ §4（RAG Pipeline）等结构性观察**仍然有效**；**§2（Advisor Ordering）已被本文 §3.2 撤销**——Go 中间件链显式组合即顺序，与 Spring IoC 容器的自动发现模型无关，原判断把 Java 范式硬套到 Go 了 |
| `SPRING_AI_CAPABILITY_GAPS.md`（2026-04-26）| §1.3（Reasoning 实施步骤）**被本文 §1 + REASONING_UNIFIED_DESIGN.md 取代**；§2-§7（Anthropic caching / OpenAI Responses / Google CachedContent / 模型适配器矩阵）**仍然有效** |
| `SPRING_AI_THINKING_ARCHITECTURE.md` | §9 已标 superseded；§1-§8 关于 Spring AI 内部多 Generation / metadata channel 模式的剖析作为**历史参照**仍有价值 |
| `REASONING_UNIFIED_DESIGN.md` | Reasoning 实现的契约文档，已落地完毕 |
| **本文（`SPRING_AI_GAP_ANALYSIS_2026-04-29.md`）** | 当前快照，焦点在**还差什么** |

---

## 7. 一句话定档

> **Reasoning 字段落地后，Lynx 在「推理模型语义」这条线上反超 Spring AI 当前 HEAD（后者 P0-1 仍未落地）。剩下的差距集中在三块：(1) 商用刚需的 Anthropic Prompt Caching；(2) 生态级 MCP + 持久化 Memory；(3) 旧账上的 Filter Converter 基类 + RAG 阶段化。前两块决定能不能上生产，第三块决定能不能加 store / 加 retriever 不痛苦。**
>
> Spring AI 自己也没静止——MCP SDK 在升级、Spring Boot 在升级、Azure OpenAI 在被合并、Anthropic 在加 Web Search Tool / Citations。但**结构性 P0 提案（ReasoningContent / AbstractToolCallingChatModel）依然在 backlog 里**，给 Lynx 留了反超窗口期。
>
> **不要照搬不属于 Go 的差距**：Spring 的 `Advisor.getOrder()` 是 IoC 容器自动发现导致的补丁，Go 中间件用注册顺序就够了——这一条已从清单撤销。把 Java 框架的解决方案翻译成 Go 时，要先确认 Java 在解决什么问题、Go 有没有这个问题。
>
> **不要重造 Lynx 已经有的逃生通道**：`chat.Options.Extra` 装 SDK 原生 params 这个机制，已经覆盖了 Spring AI 用专用类做的多数 provider-specific feature——cache_control、reasoning_effort、prefix、extra_body 等用户全能直接写上去。Lynx 需要做的是**保护这条通道在 adapter 里不被通用 builder 清掉**，而不是再造一层 strategy / option 类。
