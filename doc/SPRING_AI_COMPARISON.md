# 多角度对比：lynx vs spring-ai

> **基线**
> - lynx HEAD `6bef5eb`（branch `main`，2026-05-21）；go.work 编入 11 个 module，Go 1.26.3
> - spring-ai main HEAD `0ed060b07`（2026-05），Spring Boot 4 + Spring Framework 7 + Reactor + Micrometer 路线
>
> 本文取代历次旧版（2026-05-14 / 05-17 / 05-21 三次基线），采用当前事实直接重写——不再保留历史 strikethrough。需要历史轨迹的看 git log。

---

## 0. TL;DR

**心智模型对得上**：ChatModel + Embedding + Image + Audio + Moderation + Tool calling + Memory + Document + VectorStore + RAG + MCP + Evaluation 这 12 大概念两边都能一一对应。**差异在工程形态与边际成本**。

**根本分歧一句话**：spring-ai 是 *batteries-included framework*（autoconfigure / starter / BOM / Micrometer 抽象），lynx 是 *thin library*（核心抽象 + 显式构造 + 原生 OTel）。

**lynx 在 8 条线上反超 spring-ai 主干**：

| # | 反超点 | 简述 |
|---|---|---|
| 1 | **Reasoning 一等公民** | `AssistantMessage.Parts` 含 `ReasoningPart{Text, Signature}` + `Usage.ReasoningTokens *int64`；spring-ai 至今没有 |
| 2 | **chat 包零 provider 知识** | provider-specific metadata key 下沉到 `models/<provider>/`，core 包 grep 不到 vendor 字符串；spring-ai `MessageAggregator` 仍硬编码识别 Google `"isThought"` |
| 3 | **`iter.Seq2` 流式** | Go 1.23 内置迭代器即 streaming；spring-ai 仍是 Reactor `Flux` + `contextView` ceremony |
| 4 | **ISP 拆接口** | `vectorstore.Store = Creator + Retriever + Deleter`、`memory.Store = Reader + Writer + Clearer`；spring-ai 单接口仍是巨型 interface |
| 5 | **Cache tokens 类型化** | `Usage.CacheReadInputTokens / CacheWriteInputTokens *int64` 三态语义清晰；spring-ai 2.0 仍是 `default Long ... { return null; }` 弱形态 |
| 6 | **多模态广度** | Image / TTS / STT 各 9–10 个 vendor，spring-ai 主干 image 仅 2 / TTS 仅 1 / STT 完全没有 |
| 7 | **Vector store 广度** | lynx 27 vs spring-ai 22；多 8 个独有（bedrockkb / s3vectors / clickhouse / cockroachdb / supabase / tidb / vectara / vespa）|
| 8 | **Chat memory 广度** | lynx 6 vs spring-ai 5（lynx 多 cosmosdb，spring-ai v1.0 后已移除）|

**lynx 剩余 gap**：见 §9，只有 6 项长尾观察项，无生产硬刚需。**P0/P1/P2 全部闭合**。

---

## 1. 哲学 / 定位

| 维度 | lynx | spring-ai |
|---|---|---|
| **角色** | 基础库——给应用层 / agent runtime 用 | 应用 framework——给 Spring Boot 业务应用用 |
| **打包** | go.work 11 个独立 module | Maven artifact 矩阵 + Spring Boot starter（BOM 160 个 artifact）|
| **加 vendor 代价** | 新建 sibling dir（`models/<x>` / `vectorstores/<x>`），core 零改动 | 三件套：`spring-ai-<x>` + `spring-ai-autoconfigure-model-<x>` + `spring-ai-starter-model-<x>` |
| **依赖注入** | 没有（显式 `NewClient(model)` / `NewStore(config)`） | Spring DI 容器一等公民 |
| **配置方式** | 构造函数 + Option struct | `application.yml` + `@ConfigurationProperties` + autoconfigure |
| **开箱即用度** | 用户手动拼 model + middleware | starter 引入即生效 |
| **依赖体积** | 单 module 可零外部依赖 | 最小用例也带 Spring Framework + Boot + Micrometer + Reactor |

### 1.1 vendor 生命周期成本曲线

spring-ai 自 v1.0.0 → HEAD（HEAD `0ed060b07`）已移除 / 外迁的模块：

| 移除模块 | 原因 |
|---|---|
| `spring-ai-azure-openai` | 并入官方 openai-java SDK（同 SDK 直接谈 Azure endpoint）|
| `spring-ai-vertex-ai`（chat 部分）| 由 google-genai 取代 |
| `spring-ai-zhipuai` 模型 + autoconfigure | 移出主仓库 |
| `spring-ai-moonshot` / `spring-ai-qianfan` | 移到 community（"外区访问不到"）|
| `spring-ai-azure-cosmos-db-store` + chat memory | 整套 cosmos-db 砍掉 |
| `spring-ai-hanadb-store` | SAP HANA 砍掉 |
| `spring-ai-infinispan-store` | Infinispan 砍掉 |
| `spring-ai-spring-cloud-bindings` | 内部整合包砍掉 |

**这不是 provider 不好，是 framework 模块矩阵维护税**——每个 vendor 至少绑 3 个 Maven module + BOM 入口 + IT，用户量不达标就拖慢 CI。

**lynx 没有这套耦合**：`models/<x>` / `vectorstores/<x>` 各自独立 go module，core 零依赖。扩张 = 新建 sibling 目录；收缩 = 移到独立仓库改 import 路径，core 不动一行。所以 `models/azureopenai` / `models/vertexai` / `models/zhipu` / `models/moonshot` / `chatmemory/cosmosdb` 这些 spring-ai 已移除的，在 lynx 维护成本可忽略。

**结论**：扩张和收缩在 lynx 的成本曲线远低于 spring-ai。未来如果某 vendor 真要独立分发，路径是"移到 sibling 仓库"，不是"deprecate 后删"。

---

## 2. 语言范式

| 维度 | lynx (Go 1.26) | spring-ai (Java 21 / Reactor) |
|---|---|---|
| 流式 | `iter.Seq2[*Response, error]` 标准库 | `Flux<ChatResponse>` Reactor + `contextView` |
| 类型层级 | flat struct + interface（无继承）| 抽象类 + 多层 generics |
| Tool schema | `pkg/json` 显式 JSON schema 生成 | reflection + `JsonSchemaGenerator` |
| 并发模型 | goroutine + `context.Context` + `errgroup` | `CompletableFuture` + `coroutines` 混合 |
| Options | `Options` value + `WithXxx()` builder + value-typed `DefaultOptions()` | 1.1.6 起从 setter 改为 builder（spring-ai 走了 lynx 一直就在的路）|
| Extra 通道 | `Options.Extra map[string]any` + provider 直接读 | `ChatOptions` 字段预声明 + ExtraFields 受限 |

**Extra 通道的胜利**：lynx 用 `JSON.ExtraFields` + `chat.ModelMetadata.Provider` 让 22 个 chat provider 共用同一份 `openai.ChatModel`——5 行配置接 xAI / Groq / Together / Fireworks / Perplexity。spring-ai 的 `OpenAiChatOptions` 字段预声明，接 third-party endpoint 要 patch 库或绕 `@ConditionalOnProperty`。

---

## 3. 抽象形态

### 3.1 Message 模型

| 维度 | lynx | spring-ai |
|---|---|---|
| AssistantMessage shape | `Parts []OutputPart`（保序）含 `TextPart` / `ReasoningPart` / `ToolCallPart` | `content String` + `media List<Media>` + `toolCalls List<ToolCall>` + `properties Map`（平铺）|
| Reasoning | `ReasoningPart{Text, Signature}` 一等公民 + part-level 顺序 | 无；走 `properties` map（Anthropic）或硬编码 `"isThought"`（Google）|
| Multi block ordering | ✅ wire order 保留 | ❌ 按类型打平 |
| Cache tokens | `Usage.CacheReadInputTokens / CacheWriteInputTokens *int64`，三态 nil / 0 / >0 | `Long`，nil / 0 弱区分 |

### 3.2 模态对称

| 模态 | lynx vendor 数 | spring-ai vendor 数 |
|---|---|---|
| Chat | **21** | 8（含 bedrock + bedrock-converse + ollama 等）|
| Embedding | 12 | 8 |
| Image | **10** | 2（openai + stability） |
| TTS | **9** | 1（elevenlabs，v1.0 后新加）|
| STT | **9** | 0（已从主干完全移除）|
| Moderation | 2（openai + mistral）| 1（openai）|

每个 lynx vendor dir 内多模态共存（如 `models/openai/` 一夹覆盖 chat + embed + image + tts + stt + moderation 六路）；spring-ai 倾向于每个 vendor 拆多个 artifact。

---

## 4. 生态广度（vendor 矩阵）

### 4.1 Chat-capable model provider

**lynx 21 个 vs spring-ai 8 个**——2.6× 反超。

| 类别 | spring-ai 主干 | lynx |
|---|---|---|
| 闭源云模型 | anthropic / openai / google-genai / minimax / deepseek / mistral-ai | + azureopenai / vertexai / cohere / xiaomi |
| AWS | bedrock + bedrock-converse | bedrock（双路径合一）|
| 本地 / 自托管 | ollama | ollama |
| OpenAI-compat 网关 | 走 base URL 配置 | **原生包**：xai / groq / together / fireworks / perplexity / openrouter / moonshot / huggingface / zhipu / alibaba + `openaicompat` 通用适配器 |

**5 个 spring-ai 没有的 OpenAI-compat 专属包** + **3 个 spring-ai 已移除但 lynx 保留**（azureopenai / vertexai chat / zhipu）+ **2 个 spring-ai 缺**（cohere chat / xiaomi）。

### 4.2 Vector store

**lynx 27 vs spring-ai 22**——绝对数反超。

| spring-ai 主干（22）| lynx（27） |
|---|---|
| azure / **bedrock-knowledgebase** / cassandra / chroma / **coherence** / couchbase / elasticsearch / **gemfire** / mariadb / milvus / mongodb-atlas / neo4j / opensearch / oracle / pgvector / pinecone / qdrant / redis (+ redis-semantic-cache) / **s3-vector** / typesense / weaviate | azureaisearch / azurecosmos / **bedrockkb** / cassandra / chroma / **clickhouse** / **cockroachdb** / couchbase / elasticsearch / inmemory / mariadb / milvus / mongodb / neo4j / opensearch / oracle / pgvector / pinecone / qdrant / redis / **s3vectors** / **supabase** / **tidb** / typesense / **vectara** / **vespa** / weaviate |

- lynx 覆盖 spring-ai 22 个中的 19 个，缺 **gemfire / coherence**（Java 强生态，Go SDK 缺位）
- lynx 多出 **8 个**：bedrockkb / s3vectors / clickhouse / cockroachdb / supabase / tidb / vectara / vespa
- lynx 还保留 **azurecosmos**（spring-ai v1.0 后已移除）—— 不是 gap，是 §1.1 vendor 生命周期成本曲线的直接体现

### 4.3 Chat memory backend

**lynx 6 vs spring-ai 5**。

| spring-ai | lynx |
|---|---|
| jdbc / redis / mongodb / neo4j / cassandra | postgres / redis / mongodb / neo4j / cassandra / **cosmosdb** |

spring-ai v1.0 后移除了 cosmos-db，lynx 保留。两边都没有 vector-store-backed memory（语义检索式长期记忆）。

### 4.4 Document reader

**lynx 3 vs spring-ai 4**。spring-ai 多一个 **Tika**（universal parser，但依赖 JVM 服务）。lynx 的 markdown（goldmark）/ html（goquery）/ pdf（ledongthuc/pdf）覆盖 95% RAG 输入；Tika 故意不做（要求用户跑独立 JVM 服务）。

### 4.5 Image / TTS / STT

**这是 lynx 反超最大的领域**。

| 模态 | lynx | spring-ai |
|---|---|---|
| Image | 10：azureopenai / blackforestlabs / google / luma / midjourney / openai / prodia / replicate / stability / vertexai | 2：openai + stability |
| TTS | 9：azureopenai / deepgram / elevenlabs / google / hume / lmnt / openai / replicate / vertexai | 1：elevenlabs |
| STT | 9：assemblyai / azureopenai / deepgram / elevenlabs / gladia / google / openai / revai / vertexai | 0 |

### 4.6 Tool 实现

| 类别 | spring-ai | lynx |
|---|---|---|
| 内置 tool 实现 | 仅 `ToolCallback` SPI | 出货 `tools/{bash, fs, websearch, webfetch}` 4 大类 |
| Websearch backend | — | 7 个：brave / exa / firecrawl / jina / perplexity / serper / tavily |
| Webfetch backend | — | 4 个：exa / firecrawl / jina / tavily |

### 4.7 MCP 模块化

| | spring-ai | lynx |
|---|---|---|
| 模块数 | ~12 个 Maven module | 1 个 Go package |
| LOC | 8k–10k | ~1500（含 reverse capabilities + Streamable HTTP）|
| Sync/Async | 双轨 | 仅 sync + `iter.Seq2` 流式 |
| Server transport | WebMvc / WebFlux / stateless | modelcontextprotocol/go-sdk（InProc / stdio / SSE / **Streamable HTTP** lynx 已包装）|
| 反向能力 | Sampling / Elicit / Progress / Log / Ping 全开 | 全开（`mcp/notify.go` + `mcp/session.go`）|

---

## 5. 中间件 / Advisor 体系

| | spring-ai | lynx |
|---|---|---|
| 数量 | ~10 个 advisor | 4 个 middleware（`ToolMiddleware` + `memory.Middleware` + `LoggerMiddleware` + `SafeguardMiddleware`）+ `rag.PipelineMiddleware` |
| 形态 | `CallAroundAdvisor` / `StreamAroundAdvisor` | `CallMiddleware` / `StreamMiddleware`（双轨但共用底层逻辑）|
| Logger | `SimpleLoggerAdvisor` 硬编码 slf4j | **依赖倒置**：`Logger` 接口 + `NewSlogLogger` 默认实现，用户可换 zap/zerolog |
| Safeguard | `SafeGuardAdvisor` 硬编码字符串列表 | **依赖倒置**：`Matcher` 接口 + `NewSubstringMatcher` 默认实现，用户可换 regex/ML 分类器 |
| Tool loop | `ToolCallingManager` | `ToolMiddleware`（recursion + accumulator）|

依赖倒置形态是 lynx 比 spring-ai 在 middleware 设计上的小胜——但 spring-ai 的 advisor 数量上仍领先（QuestionAnswer / RetrievalAugmentation / Refusal / TokenCount 等场景化 advisor lynx 缺）。

---

## 6. 关键子系统

### 6.1 Chat client

| | spring-ai | lynx |
|---|---|---|
| Fluent API | `ChatClient.builder().defaultXxx(...).build()` + `.prompt().user(...).call()` | `client.Chat().WithMessages(...).WithTools(...).Call().Response(ctx)` |
| ToolSpec 风格 | 4d0bb7b7f 引入 `ToolSpec` fluent | lynx 一直就是 fluent（`Chat().WithTools(...)`）|
| Multi-vendor 复用 | 各 vendor 一个 ChatModel 子类 | 5 个 OpenAI-compat vendor 共用 `openai.ChatModel`，BaseURL 切换即可 |

### 6.2 Tool / function calling

| | spring-ai | lynx |
|---|---|---|
| 注册 | `@Tool` 注解 / `MethodToolCallback` / `FunctionToolCallback` 多种 | 单一 `Tool` 接口（去年 `Tool` + `CallableTool` 二分已合并）|
| Reflection schema | 自动 + 注解约束 | `pkg/json` 显式生成；可手写 |
| Loop driver | `ToolCallingManager` | `ToolMiddleware`（recursion + ShouldReturnDirect 短路）|

### 6.3 RAG pipeline

| | spring-ai（4 阶段层级）| lynx（扁平）|
|---|---|---|
| 入口 | `RetrievalAugmentationAdvisor` | `rag.NewMiddleware` |
| 组件 | `QueryTransformer` / `QueryExpander` / `DocumentRetriever` / `DocumentJoiner` / `QueryAugmenter` | `QueryTransformer` / `QueryExpander` / `DocumentRetriever` / `DocumentRefiner` / `QueryAugmenter` |
| Joiner | `ConcatenationDocumentJoiner`（dedup + sort）| `DeduplicationRefiner` → `RankRefiner` refiner 链等价 |
| Refiner | — | `DeduplicationRefiner` / `RankRefiner` lynx 独有 |
| 多路 RRF | — | YAGNI：lynx 当前 retriever 全部向量同尺度，RRF 无对象 |

### 6.4 VectorStore filter mini-language

| | spring-ai | lynx |
|---|---|---|
| Lexer/Parser | ANTLR4 generated | 手写 lexer + recursive descent，~500 LOC |
| AST visitor | `AbstractFilterExpressionConverter` 模板方法基类 | 每 vendor 独立 visitor + `internal/filterhelp` 共享 helper（dispatch / literal / wholeNumber 等）|
| Runtime 依赖 | ANTLR4 runtime（~500KB JAR）| 零 |
| 标识符 / 字面量校验 | 基类统一 | `internal/ident` + `internal/docio` 共享 |
| 形态 | 继承基类 | 组合 helper（Go 风）|

---

## 7. 打包 / 集成 / 部署

| | spring-ai | lynx |
|---|---|---|
| 模块矩阵 | 160 个 BOM artifact | 11 个 go module |
| 配置发现 | Spring autoconfigure 链 | 编译期 import + 显式构造 |
| 部署 | 主要进 Spring Boot 应用 | 直接编译进 Go 二进制 |
| OS / 平台 | JVM | Go 原生交叉编译 |

---

## 8. 可观测性 / 错误处理 / Retry

### 8.1 Observability

lynx 走 **OTel 原生** + slog bridge（OTLP 全家桶 + `otel/slog` SpanExporter）；spring-ai 走 **Micrometer Observation** 抽象层。

**lynx 埋点覆盖（已全量）**：chat / embedding / tool / RAG 五阶段 / MCP client+server / agent runtime（含 HTN / Reactive / GOAP planner）/ 24 个 vectorstore / 6 个 chatmemory provider 全部按 GenAI + DB semconv 严格挂上。详见 `OBSERVABILITY.md`。

### 8.2 Retry

lynx 不做 vendor-agnostic retry 层——SDK 内部已自带（openai-go / anthropic-sdk-go / bedrockruntime / pinecone-go 等）。spring-ai 的 `RetryTemplate` 也基本是 wrap SDK 重试，再造一层属重复建设。

### 8.3 错误处理

| | spring-ai | lynx |
|---|---|---|
| 形态 | unchecked exception | sentinel error + `errors.As` |
| Wrap | `NestedRuntimeException` | `fmt.Errorf("...: %w", err)` |
| Type assertion | `instanceof` | `errors.As` |

---

## 9. 剩余 gap 清单

P0 / P1 / P2 全部闭合，剩余仅长尾观察项。

| # | 项 | 类别 | 说明 |
|---|---|---|---|
| 1 | Vector-store backed chat memory | P3 / 中 | 语义检索式长期记忆，复用 vectorstore.Store + memory middleware；spring-ai 也未做 |
| 2 | OpenAI Responses API（chat 路径）| P3 / 中 | o1/o3/gpt-5 reasoning 路径已通过 `models/openai/chat_responses.go` 落地（earlier PR），但 Tools / Built-in Tools / streaming 部分细节仍可深化 |
| 3 | Anthropic Web Search Tool + Citations | P3 / 中 | Anthropic 特有能力 |
| 4 | Anthropic Skills + Files API | P3 / 高 | 生成 Excel / PPT / PDF 报告 |
| 5 | Google Gemini 高级特性 | P3 / 中 | CachedContent API / ThinkingLevel / thoughtSignatures |
| 6 | 企业向 vendor 补全 | P3 / 中-高 | IBM watsonx / Snowflake Cortex / Databricks 等非 OpenAI-compat，需独立 SDK；spring-ai 也未做 |

### 9.1 故意不做（"为什么不抄"）

| # | spring-ai 有但 lynx 不做 | 原因 |
|---|---|---|
| A | Spring Boot autoconfigure / starter | lynx 是 library；Go 生态没有"DI 容器自动装配"传统 |
| B | Micrometer Observation 抽象层 | Go 世界 OTel 已是事实标准 |
| C | `@Tool` / `@McpTool` 注解 | Go 无 runtime annotation；构造函数路线已足够清晰 |
| D | `ToolCallbackResolver` Spring Bean 解析 | lynx 无 DI 容器 |
| E | sync/async 双轨 | Go goroutine + `iter.Seq2` 单一路径已覆盖 |
| F | `StringTemplate` 完整模板引擎 | `text/template` 已是 Go 标准库；简单 `{key}` placeholder 够用 |
| G | 多套 vendor module 完整拆分 | lynx 一个 vendor dir 覆盖多模态 |
| H | GemFire / Coherence vector store | Java 强生态，Go SDK 缺位 |
| I | transformers / postgresml 本地推理 vendor | Go 调 Python ML runtime 是异常路径；用户该用 ollama |
| J | Tika document reader | 要求独立 JVM 服务；Go 用户体验差 |
| K | Retry `Transient` / `NonTransient` 分类 | SDK 内部已自带重试；再加一层重复 |
| L | RRF DocumentJoiner | 当前所有 retriever 都是向量同尺度，RRF 无对象；待非向量 retriever 落地再做 |

---

## 10. 一句话定档

**lynx 走 thin-library 路线扩 vendor 的边际成本远低于 spring-ai 的 framework 路线——21 chat / 27 vector / 6 chatmemory / 10 image / 9 TTS / 9 STT 已经验证；spring-ai 同期反而做了一轮主仓库收缩（azure-openai / vertex-ai chat / zhipuai / moonshot / cosmosdb / hanadb / infinispan 全部移出），是 framework 路线必须缴的"模块矩阵维护税"**。

P0 / P1 / P2 全部闭合后，lynx 在"生产可用度"上对 spring-ai 的硬差距已实质补齐：reasoning 一等公民 / OTel 全量埋点 / 持久化 memory 反超 / document readers / Structured Output / SafeGuard + Logger middleware（依赖倒置形态优于 spring-ai 硬编码版）/ Bedrock Converse 深化 / MCP v2 反向能力 / Streamable HTTP——全部到位。剩余 6 项 P3 都是长尾创新（spring-ai 也未做的）。

---

*对比结束。lynx HEAD `6bef5eb`，对照 spring-ai main HEAD `0ed060b07`，2026-05-21。*
