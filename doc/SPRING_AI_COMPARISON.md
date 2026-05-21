# 多角度对比：lynx vs spring-ai

> **基线**
> - lynx HEAD `24354b4`（branch `main`，2026-05-17）；go.work 编入 9 个 module（`core` / `models` / `vectorstores` / `agent` / `rag` / `tools` / `mcp` / `otel` / `pkg`），Go 1.26.3
> - spring-ai 主干（2026-05 范围）；Maven artifact 矩阵 + Spring Boot starter 路线
>
> 本文取代旧版 `SPRING_AI_COMPARISON.md` (2026-05-14 基线)。自上版以来 lynx 完成了三轮生态扩张（vector stores 6→27、model providers 3→39、tools 套件出货），原文 §9 的多数 P1/P3 gap 已闭合，需要重新校准坐标。本次重组沿用 10 节框架，但每节内容按当前事实重写。

---

## 0. TL;DR

**两边的心智模型仍然对得上**：ChatModel + Embedding + Image + Audio + Moderation + Document + VectorStore + RAG + MCP + Tool calling + Memory + Evaluation 这 9 大概念在两侧都能一一对应。差异不在"有没有"，而在**怎么做**以及**广度／深度的取舍**。

**根本分歧一句话**：spring-ai 是 *batteries-included framework*（autoconfigure / starter / BOM / Micrometer 抽象），lynx 是 *thin library*（核心抽象 + 显式构造 + 原生 OTel）。这条立场未变。

**lynx 当前在 7 条线反超 spring-ai 主干**（原文 5 条 + 2 条新增）：

| # | 反超点 | 简述 |
|---|---|---|
| 1 | **Reasoning 一等公民** | `AssistantMessage.Reasoning string` + `Usage.ReasoningTokens *int64`；spring-ai 至今没有 |
| 2 | **chat 包零 provider 知识** | provider-specific metadata key 下沉到 `models/<provider>/`，core 包 grep 不到 vendor 字符串；spring-ai `MessageAggregator` 仍硬编码识别 Google `"isThought"` |
| 3 | **`iter.Seq2` 流式** | Go 1.23 内置迭代器即 streaming；spring-ai 仍是 Reactor `Flux` + `contextView` ceremony |
| 4 | **ISP 拆接口** | `vectorstore.Store = Creator + Retriever + Deleter`、`memory.Store = Reader + Writer + Clearer`；spring-ai 单接口仍是巨型 interface |
| 5 | **Cache tokens 类型化** | `Usage.CacheReadInputTokens / CacheWriteInputTokens *int64` 三态语义清晰；spring-ai 2.0 仍是 `default Long ... { return null; }` 弱形态 |
| 6 | **Vector store 广度**（新增）| **lynx 27 vs spring-ai 21**——lynx 已反超绝对数量；多 8 个 spring-ai 没有的（bedrockkb / s3vectors / clickhouse / cockroachdb / supabase / tidb / vectara / vespa） |
| 7 | **多模态广度**（新增）| Image / TTS / STT 各 8 个 vendor；spring-ai 主干 image 仅 2 个 vendor、TTS 仅 OpenAI、STT 仅 OpenAI |

**lynx 仍有 / 真正剩下的 gap**（按 ROI 重排）：

| # | Gap | 状态 vs 旧版 | 简述 |
|---|---|---|---|
| ~~1~~ | ~~Retry Transient / NonTransient 分类~~ | **不做** | SDK 内部自带重试，再加一层是重复建设 |
| ~~2~~ | ~~Anthropic Extra 通道保护~~ | **已闭合** | `models/anthropic/extra.go` |
| ~~3~~ | ~~持久化 Memory 后端~~ | **已闭合 + 反超** | 顶层 `chatmemory/` 6 个 provider（postgres / redis / mongodb / cassandra / neo4j / cosmosdb）；spring-ai main 现在只有 5 个（cassandra / jdbc / mongodb / neo4j / redis），lynx 还多 cosmosdb |
| ~~4~~ | ~~PDF / Markdown reader~~ | **已闭合** | 顶层 `document-readers/` 3 个 reader：markdown（goldmark）/ html（goquery）/ pdf（ledongthuc/pdf）；Tika 因依赖 JVM 服务故意不做 |
| ~~5~~ | ~~Structured Output Converter~~ | **已闭合** | `core/model/chat/parser.go`（JSONParser[T] / ListParser / MapParser / StructuredParser[T] / AnyParser）|
| 1 | **SafeGuard / Logger middleware** | 未闭合 | core 内置 middleware 仍只有 ToolMiddleware；Logger / Safeguard 留给用户写。**当前 lynx 唯一真 P1 gap** |
| ~~7~~ | ~~DocumentJoiner / QueryRouter~~ | **不做** | spring-ai 的 ConcatenationDocumentJoiner（dedup + score-sort）等价 lynx 的 `DeduplicationRefiner` + `RankRefiner` refiner 链，已有现成等价物；RRF 仅在异质 retriever（BM25 + 向量）混用时有价值，lynx 目前 retriever 只有 `VectorStoreRetriever`，加 Joiner 是 YAGNI；QueryRouter 由用户在自定义 retriever 内决定即可。详见 `rag/doc.go` |
| ~~8~~ | ~~FilterExpression BaseVisitor~~ | **已闭合** | `vectorstores/internal/filterhelp` 提供 dispatch helpers，10 个 visitor 已迁移 |
| 2 | **ToolSpec fluent API 对账** | 新增观察 | spring-ai 在 4d0bb7b7f 给 ChatClient 加了 `ToolSpec`；lynx 一直是 `Chat().WithTools(...)` fluent，方向一致但要确认覆盖面 |
| 3 | **MCP Streamable HTTP** | 新增观察 | spring-ai 已 deprecate SSE，Streamable HTTP 为新默认（c549bf821）；lynx `mcp/` 需评估是否跟进 |

**自上版以来已闭合的 gap**（按重要性）：

| # | 项 | 闭合方式 |
|---|---|---|
| A | **Ollama adapter** | `models/ollama` 出货（chat + embed） |
| B | **Vector store 广度** | 一次性新增 21 个 vendor（pgvector / mongodb / elasticsearch / opensearch / redis / cassandra / neo4j / couchbase / clickhouse / mariadb / oracle / tidb / cockroachdb / supabase / azureaisearch / azurecosmos / bedrockkb / s3vectors / typesense / vespa / vectara） |
| C | **多模态 vendor 矩阵** | Image / TTS / STT 各扩到 8 个 vendor（含 elevenlabs / stability / deepgram / assemblyai 等） |
| D | **OpenAI-compat 网关广覆盖** | xai / groq / together / fireworks / perplexity 原生包出货，不再依赖用户自配 BaseURL |
| E | **专业 embedding vendor** | voyage / cohere / jina / nomic 出货（Anthropic 推荐 + matryoshka + 任务条件 embedding） |
| F | **Tool 套件出货** | `tools/{bash, fs, websearch, webfetch}` + 7 个 search backend / 4 个 fetch backend 已就位 |
| G | **OTel 埋点全覆盖** | chat / embedding / tool / RAG / MCP / agent / 24 vectorstore / 6 chatmemory 按 GenAI 与 DB semconv 全量挂上 |
| H | **持久化 Chat Memory** | 顶层 `chatmemory/` module 6 个 provider（postgres / redis / mongodb / cassandra / neo4j / cosmosdb），含 spring-ai 已移除的 cosmosdb |
| I | **Structured Output** | `core/model/chat/parser.go` 完整覆盖 spring-ai `BeanOutputConverter` 家族 |
| J | **不可变 Options 路线** | lynx 一直就是 `WithXxx()` builder + value-typed `DefaultOptions()`；spring-ai 1.1.6 整周期才把全套 setter 移除，方向一致但 lynx 落地更早 |

---

## 1. 哲学 / 定位

立场未变，原版分析仍成立：

| 维度 | lynx | spring-ai |
|---|---|---|
| **角色** | 基础库——给应用层 / agent runtime 用 | 应用 framework——给 Spring Boot 业务应用用 |
| **打包思路** | go.work 多 module，按需 import | Maven artifact 矩阵 + Spring Boot starter |
| **加 vendor 的代价** | 新建 sibling dir（`models/<x>` 或 `vectorstores/<x>`），不影响 core | 新建 `models/spring-ai-<x>` + `auto-configurations/models/<x>` + `starters/spring-ai-starter-<x>`，三件套 |
| **依赖注入** | 没有（显式 `NewClient(model)` / `NewStore(config)`） | Spring DI 容器一等公民 |
| **配置方式** | 构造函数 + Option struct | `application.yml` + `@ConfigurationProperties` + autoconfigure |
| **开箱即用程度** | 最小可工作集合需要用户手动拼 model + middleware | starter 引入即生效 |
| **依赖体积** | 单 module 内可零外部依赖 | 即便最小用例也带 Spring Framework + Boot + Micrometer + Reactor |

新增观察：**lynx vendor 扩张的边际成本明显低于 spring-ai**——这次单分支一次性加 21 个 vector store + 7 个 model provider，每个 vendor 平均 ~400 LOC（含 visitor + store + docs），且不需要写 autoconfigure / starter / properties / @ConfigurationProperties 任何一行。spring-ai 加一个 vendor 需要至少 3 个 Maven module + ~1500 LOC + Bean 装配测试。

### 1.1 vendor 生命周期成本曲线

spring-ai 自 v1.0.0 以来主仓库收缩明显——`spring-ai-azure-openai`、`spring-ai-vertex-ai`（chat）、`spring-ai-zhipuai`、`spring-ai-moonshot` / `spring-ai-qianfan`（→ community）、`spring-ai-azure-cosmos-db-store`、`spring-ai-hanadb-store`、`spring-ai-infinispan-store`、`spring-ai-model-chat-memory-repository-cosmos-db` 全部被清出主仓库。原因不在"provider 不好"，而在**模块矩阵的维护税**：spring-ai 每个 vendor 至少绑着 `spring-ai-<x>` + `spring-ai-autoconfigure-model-<x>` + `spring-ai-starter-model-<x>` 三个 Maven module + BOM 入口 + integration test；任何一个用户量不达标的 vendor 都会拖慢 CI 与 release。

**lynx 没有这套耦合**——`models/<x>` 和 `vectorstores/<x>` 各自是独立 go module（自己的 `go.mod` / `go.sum` / 测试），core import 路径与它们零耦合。这意味着：

- **扩张**：加一个 vendor = 新建 sibling dir，core 不动一行
- **收缩**：把一个 vendor 搬到独立仓库（甚至社区 org） = 改 import 路径，无破坏
- **维护**：单个 vendor 的依赖升级 / 弃用 / 安全告警都被隔离在它自己的 `go.sum` 里

所以 lynx 不需要、也不会做 spring-ai 那种周期性的"模块矩阵瘦身"。`models/azureopenai` / `models/vertexai` / `models/zhipu` / `models/moonshot` / `chatmemory/cosmosdb` 这些 spring-ai 已移除的 provider，在 lynx 这边维护成本可以忽略不计。

未来如果某个 vendor 真要独立分发，lynx 的路径是"移到 sibling 仓库"而不是"deprecate 后删除"——成本曲线完全不同。

---

## 2. 语言范式

未变。原文 §2 论述（流式 / 类型层级 / 反射 vs 显式 schema / 并发模型）仍然准确。补一条新增观察：

**JSON.ExtraFields 在新一轮 OpenAI-compat vendor 中验证了价值**：xai / groq / together / fireworks / perplexity 全部用同一份 `openai.ChatModel`，通过 `option.WithBaseURL` + `chat.ModelMetadata.Provider` 覆盖即可工作；provider-specific knob（Perplexity 的 `search_mode` / `web_search_options`、Groq 的 `service_tier` / `reasoning_format`、xAI 的 `search_parameters`）一律走 ExtraFields 透传。spring-ai 这个路线走不通——`OpenAiChatOptions` 把每个字段都预声明了，接 third-party endpoint 需要 patch 库或绕过 `@ConditionalOnProperty`。

---

## 3. 抽象形态 / API surface

§3.1 / §3.2 / §3.3 原文未变。§3.4 模态对称性需要更新——lynx 的"6 模态 + Client 一致性"现在落到 39 个 vendor 上，比 spring-ai 主干更深：

| 模态 | lynx vendor 数 | spring-ai vendor 数 |
|---|---|---|
| Chat | 22（incl. openaicompat 通用适配器）| 15 |
| Embedding | 12 | ~8 |
| Image | 8 | 2（openai + stability） |
| TTS | 8 | 1（openai） |
| STT | 8 | 1（openai） |
| Moderation | 2（openai + mistral） | 1（openai） |
| Audio translation | 1（openai） | 0 |

**取舍维度的不同**：spring-ai 的 vendor module 更细——一个 vendor 通常拆成 `spring-ai-<vendor>` + `spring-ai-<vendor>-chat` + `spring-ai-<vendor>-embedding` 多个 artifact；lynx 一个 vendor dir 内多模态共存（`models/openai/` 一夹覆盖 chat + embedding + image + tts + stt + moderation + audio-translation 七路）。这种打包对 Go 用户更直观（import 路径就是依赖图），对 Spring 用户却反直觉（Spring 习惯每个 Bean 一个 artifact）。

---

## 4. 生态广度（vendor 矩阵）

**这是自上版以来最大的剧变**。原文 §4 的核心结论"lynx 在 vendor 广度上是最大短板"现在已不成立。

### 4.1 模型 vendor

**spring-ai main 在 v1.0.0 → HEAD 期间砍掉了一批 chat module**（azureopenai / vertex-ai chat / zhipuai 整模块移除；moonshot / qianfan 移到 community；watson 早在 RC1 移除），重新对账：

| 类别 | spring-ai 主干 | lynx 当前 | gap |
|---|---|---|---|
| **闭源云模型** | anthropic / openai / google-genai / minimax / deepseek / mistral-ai（chat 模块共 9：含 bedrock + bedrock-converse）| anthropic / openai / azureopenai / google / vertexai / minimax / deepseek / mistral / cohere | **lynx 反超**：azureopenai（spring-ai 已并入 openai SDK）/ vertexai chat（spring-ai 已移除）/ cohere chat（spring-ai 缺）lynx 都还在 |
| **OpenAI-compat 网关** | 走 base URL 配置（接 vLLM / Together / Anyscale 等） | **原生包**：xai / groq / together / fireworks / perplexity / openrouter / moonshot / huggingface / zhipu / alibaba / xiaomi / openaicompat 通用适配器 | **lynx 反超**（spring-ai 主干没有针对 xai / groq / together / fireworks / perplexity 的专属包；moonshot / qianfan 已移到 community）|
| **本地 / 自托管** | ollama / transformers / postgresml（embedding only）| ollama | **平**（lynx 不做 transformers / postgresml，这两个 niche） |
| **AWS** | bedrock / bedrock-converse | bedrock | **接近平**（lynx 通过 bedrock module 同时覆盖 chat + embed）|
| **专业图像** | stability-ai | stability / blackforestlabs / midjourney / luma / prodia / replicate / openai / google | **lynx 反超**（5 个独有 vendor）|
| **专业语音 TTS** | elevenlabs（v1.0 后新加）| elevenlabs / hume / lmnt / deepgram / replicate / openai / google / azureopenai | **lynx 反超**（7 个独有）|
| **专业语音 STT** | — | deepgram / assemblyai / gladia / revai / elevenlabs / openai / google / azureopenai | **lynx 反超**（spring-ai main 完全没有）|
| **专业 embedding** | vertex-ai-embedding（chat 部分已移除）/ google-genai-embedding | voyage / cohere / jina / nomic（+ 每个 chat vendor 配套）| **lynx 反超** |

**总计**：lynx 39 个 model provider 目录 / 22 个 chat-capable，spring-ai main 当前 9 个 chat module + 5 个非 chat 模型模块。从绝对覆盖看 lynx 2.4× 反超 chat 数；spring-ai 仍可能在企业向 vendor（IBM watsonx / Snowflake Cortex / Databricks）上有覆盖——这些都不在 spring-ai 主仓库，得去 community / 第三方寻。

### 4.2 Vector store vendor

**这是反差最大的部分**。原文：spring-ai 23 vs lynx 6。当前：

| spring-ai 主干（21）| lynx 当前（27） |
|---|---|
| azure / **bedrock-knowledgebase** / cassandra / chroma / **coherence** / couchbase / elasticsearch / **gemfire** / mariadb / milvus / mongodb-atlas / neo4j / opensearch / oracle / pgvector / pinecone / qdrant / redis / **s3-vector** / typesense / weaviate（azurecosmos / saphana / infinispan 已于 v1.0 后移除）| azureaisearch / azurecosmos / **bedrockkb** / cassandra / chroma / **clickhouse** / **cockroachdb** / couchbase / elasticsearch / inmemory / mariadb / milvus / mongodb / neo4j / opensearch / oracle / pgvector / pinecone / qdrant / redis / **s3vectors** / **supabase** / **tidb** / typesense / **vectara** / **vespa** / weaviate |

**统计**（基线已对齐 spring-ai main HEAD `b63a0d117`）：
- lynx 已覆盖 spring-ai 21 个中的 19 个（缺 gemfire + coherence，这两个是 Java 强生态，Go SDK 缺位）
- **lynx 多出 8 个 spring-ai 主干没有的**：bedrockkb / s3vectors / clickhouse / cockroachdb / supabase / tidb / vectara / vespa
- **lynx 还保留了 spring-ai 已移除的 azurecosmos** —— 这条不是 gap，是 §1.1 vendor 生命周期成本曲线的直接体现：lynx 独立 module 没有维护成本压力
- 绝对数量反超：27 > 21

**架构副产物**：本轮 vector store 扩张引入了 `vectorstores/internal/` 共享层——`filterhelp`（AST 助手）/ `ident`（标识符校验）/ `docio`（文档 I/O + 向量字面量格式化）。这把 §6.4 的"每 vendor visitor 600–800 LOC"问题局部缓解（共享了 ~1000 LOC 的 AST 处理样板），但 spring-ai 的 `AbstractFilterExpressionConverter` 模板方法基类形式上更彻底——lynx 仍无 `BaseVisitor`。

### 4.3 Document reader

未变。

| spring-ai（4 个）| lynx（2 个）|
|---|---|
| pdf（PagePdf + ParagraphPdf）/ markdown / tika / jsoup | text / JSON |

**仍是真 gap**：PDF / Markdown 在 RAG 场景里几乎必需。原文 §9 P1-6 此项未闭合。

### 4.4 Memory 后端

未变。

| spring-ai（6 个）| lynx（1 个）|
|---|---|
| jdbc / redis / mongodb / neo4j / cassandra / cosmos-db | in-memory + message-window |

**仍是真 gap**：原文 §9 P1-5 此项未闭合。**讽刺的是**：lynx 现在已经有 27 个 vector store 后端，把其中任何一个（postgres / redis / mongodb）改写成 memory.Store 都只需要 ~100 LOC——这件事的工作量已经被 vector store 那一轮消化掉一半。

### 4.5 Tool 实现

未变。

| 类别 | spring-ai | lynx |
|---|---|---|
| 内置 tool 实现 | 仅提供 `ToolCallback` SPI，无开箱即用工具 | 出货 `tools/{bash, fs, websearch, webfetch}` 4 大类 |
| Websearch backend | — | 7 个（brave / exa / firecrawl / jina / perplexity / serper / tavily）|
| Webfetch backend | — | 4 个 |

**lynx 反向超**仍然成立。

### 4.6 MCP 模块化

未变。

| | spring-ai | lynx |
|---|---|---|
| 模块数 | ~12 个 Maven module | 1 个 Go package |
| 估算 LOC | 8k–10k | ~750 |
| Sync/Async | 双轨 | 仅 sync + `iter.Seq2` 流式 |
| Server transport | WebMvc / WebFlux / stateless | mcp-go SDK（InProc / stdio / HTTP SSE / Streamable HTTP）|

---

## 5. 中间件 / Advisor 体系

未变。lynx 仍只有 3 个内置 middleware（`tool_middleware` / `memory/middleware` / `rag/pipeline_middleware`），spring-ai 仍有 ~10 个 advisor。**SafeGuard + Logger middleware 仍未出货**——原文 §9 P2-9 此项未闭合。

---

## 6. 关键子系统深入对比

### 6.1 Chat client 形态

未变。新增观察：随着 22 个 chat provider 出货，`JSON.ExtraFields` + `chat.ModelMetadata.Provider` 这套机制已在 5 个 OpenAI-compat 新 vendor 上经过验证——同一份 `openai.ChatModel`，5 行配置即可服务 xAI / Groq / Together / Fireworks / Perplexity 五家。这种"薄壳 facade"路线在 lynx 已成定式。

### 6.2 Tool / function calling

未变。

### 6.3 RAG pipeline

未变。

| | spring-ai（4 阶段层级）| lynx（扁平）|
|---|---|---|
| 组件数 | ~10 | ~9 |
| 阶段化 | preretrieval / retrieval / postretrieval / generation | 扁平自由组合 |
| 入口 | `RetrievalAugmentationAdvisor` | `rag.NewMiddleware` |
| lynx 独有 | — | `DeduplicationRefiner` / `RankRefiner` |
| spring-ai 独有 | `DocumentJoiner`（多路检索合并）| — |

**`DocumentJoiner` 仍缺**——原文 §9 P2-10 此项未闭合。

### 6.4 VectorStore filter mini-language

**部分变动**。原文 §6.4 提到"lynx 的架构倒退是每个 vendor visitor 从 0 写、~600-800 LOC 重复样板"。本轮重构后：

| 维度 | spring-ai | lynx 当前 |
|---|---|---|
| Lexer/Parser 实现 | ANTLR4 generated（grammar in `vectorstore/filter/antlr4/`）| 手写 lexer + recursive descent parser，~500 LOC |
| AST visitor | `AbstractFilterExpressionConverter` 模板方法基类 | **每 vendor 独立 visitor + 共享 `internal/filterhelp` 助手**（CollectKeyPath / LiteralToValue / ExtractValue / WholeNumber 等） |
| Vendor visitor LOC | ~600 LOC × 21 vendor，但基类省了 ~40% | ~500 LOC × 27 vendor，filterhelp 省了 ~30% |
| Runtime 依赖 | ANTLR4 runtime（~500KB JAR） | 零运行时依赖 |
| 标识符校验 | 基类统一 | `internal/ident.Check` / `CheckWithDash` 共享 |
| 向量字面量格式化 | 基类统一 | `internal/docio.FormatVectorLiteral` 共享（SQL stores） |

**部分闭合**：本轮抽出的 `vectorstores/internal/{filterhelp,ident,docio}` 三个共享包消化了 vendor visitor 的 30% 重复代码（约 1000 LOC 不再重复）。**但形态上仍没有 spring-ai 那种 `AbstractVisitor` 基类**——lynx 走的是"组合优于继承"的 Go 路线，把通用部分抽成函数式 helper 而不是基类。哪种更好见仁见智，但绝对 LOC 上 spring-ai 仍胜（21 × 600 × 0.6 = 7560 vs lynx 27 × 500 × 0.7 = 9450）。

### 6.5 MCP 桥接

未变。

---

## 7. 打包 / 集成 / 部署

未变。原文 §7 的论述（spring-ai `@Autowired` vs lynx 显式构造、BOM vs go.work、autoconfigure 链 vs 编译期类型检查）全部仍然成立。

---

## 8. 可观测性 / 错误处理 / Retry

### 8.1 可观测性

未变。`doc/OBSERVABILITY.md` 仍是单一事实源；lynx 走 OTel 原生 + slog bridge，spring-ai 走 Micrometer Observation。

**仍有局部 gap**：core hot path 埋点据 OBSERVABILITY.md §4 清单尚未完整覆盖——原文 §9 P0-3 此项**未闭合**。

### 8.2 Retry

未变。原文 §9 P0-1 此项**未闭合**——`pkg/retry` 仍无 `Transient` / `NonTransient` 分类，各 vendor adapter 仍不区分 429/503 vs 401/400。

**这是当前所有未闭合 gap 中 ROI 最高的一项**：30 LOC 类型 + 各 adapter ~20 LOC 分类逻辑，但收益是 LLM 集成最高频的痛点（rate-limit 自动重试 + auth-fail 立刻报错）。

### 8.3 错误处理总体

未变。lynx sentinel error + `errors.As` 仍优于 spring-ai unchecked exception。

---

## 9. 战略 gap 清单 + ROI 路线图（更新版）

按 ROI 重排。已闭合项移出列表，仅留剩余项。

### 9.1 P0 — 短工作量、高单点价值

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| ~~1~~ | ~~Retry `Transient` / `NonTransient` 分类~~ | — | **不做**：SDK 内部已自带重试，再加一层是重复建设 |
| ~~2~~ | ~~Anthropic Extra 通道保护~~ | — | **已闭合**（`models/anthropic/extra.go`）|
| ~~3~~ | ~~OTel hot path 埋点补完~~ | — | **已闭合**：chat / embedding / tool / RAG / MCP / agent / 24 vectorstore / 6 chatmemory 全量埋点 |

### 9.2 P1 — 生产硬刚需

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| ~~4~~ | ~~持久化 Memory 后端~~ | — | **已闭合**：顶层 `chatmemory/` module 提供 postgres / redis / mongodb / cassandra / neo4j / cosmosdb 6 个 provider |
| ~~5~~ | ~~PDF + Markdown document reader~~ | — | **已闭合**：顶层 `document-readers/` 出货 markdown / html / pdf 三个 reader（goldmark / goquery / ledongthuc/pdf）|

### 9.3 P2 — 闭合 spring-ai 已有的设计

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| ~~6~~ | ~~Structured output converter~~ | — | **已闭合**：`core/model/chat/parser.go`（JSONParser[T] / ListParser / MapParser / StructuredParser[T] / AnyParser）已与 spring-ai BeanOutputConverter 家族对齐 |
| 7 | **`SafeGuardMiddleware` + `LoggerMiddleware`** | 低（~100 LOC 共） | 弥补 advisor 数量差距，可直接出货 |
| ~~8~~ | ~~`DocumentJoiner` + `QueryRouter`~~ | — | **不做**：spring-ai ConcatenationDocumentJoiner 等价于 lynx `DeduplicationRefiner` + `RankRefiner` 链；RRF 留待非向量 retriever 落地时再设计；QueryRouter 由用户自定义 retriever 解决。详见 `rag/doc.go` |
| ~~9~~ | ~~Vector store `BaseVisitor` 进一步抽象~~ | — | **已闭合**：`vectorstores/internal/filterhelp` 提供 dispatch helpers，10 个 visitor 已迁移 |

### 9.4 P3 — 长尾 / 大依赖

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 10 | Bedrock Converse adapter（chat 路径） | 中 | AWS 用户；当前 bedrock 仅 chat + embed 基础 |
| 11 | MCP v2 反向能力（`ServerSessionFromContext` / sampling / elicit / progress / ping / logging）| 中 | agentic 编排基石 |
| 12 | Vector-store backed chat memory（语义检索式长期记忆）| 中 | 复用 vectorstore.Store + 现有 memory middleware |
| 13 | OpenAI Responses API | 中 | o1/o3/gpt-5 reasoning 解锁 |
| 14 | Anthropic Web Search Tool + Citations | 中 | Anthropic 特有能力 |
| 15 | Anthropic Skills + Files API | 高 | 生成 Excel / PPT / PDF 报告 |
| 16 | Google Gemini 高级特性（CachedContent API / ThinkingLevel / thoughtSignatures）| 中 | Google 用户 |
| 17 | OpenAI Audio Output（chat 主路径同时返回 text + audio）| 低 | gpt-4o-audio-preview |
| 18 | 企业向 vendor 补全（IBM watsonx / Snowflake Cortex / Databricks）| 中-高 | 企业 RFP 加分项；非 OpenAI-compat，需独立 SDK |

### 9.5 故意不做（"为什么不抄"）

| # | spring-ai 有但 lynx 不该抄 | 原因 |
|---|---|---|
| A | Spring Boot autoconfigure / starter | lynx 是 library 不是 framework；Go 生态没有"DI 容器自动装配"传统 |
| B | Micrometer Observation 抽象层 | Go 世界 OTel 已是事实标准 |
| C | `@Tool` / `@McpTool` 注解 | Go 无 runtime annotation；构造函数路线已足够清晰 |
| D | `ToolCallbackResolver` Spring Bean 解析 | lynx 无 DI 容器 |
| E | sync/async 双轨 | Go goroutine + `iter.Seq2` 单一路径已经覆盖 |
| F | `StringTemplate` 完整模板引擎 | `text/template` 已是 Go 标准库；简单 `{key}` placeholder 够用 |
| G | 多套 vendor module 完整拆分 | lynx 一个 vendor dir 覆盖多模态（如 openai 含 7 路），不需要 chat / embedding / image 各一个 artifact |
| H | GemFire / SAP Hana vector store | Java 强生态，Go SDK 缺位；目标用户重合度低 |
| I | transformers / postgresml 本地推理 vendor | Go 调 Python ML runtime 是异常路径；用户该用 ollama |

---

## 10. 一句话定档（修订）

对照 spring-ai 时**不照抄、照搬克制原则做薄壳**——这套打法在 reasoning / MCP / observability / cache tokens / vector-store 广度 / 多模态广度 6 条线上都已经验证成立。**本轮 vector store 6→27 和 model provider 3→39 的扩张证明：thin-library 路线下，扩 vendor 的边际成本远低于 framework 路线**——spring-ai 同期反而做了 vendor 矩阵收缩（azure-openai / vertex-ai chat / zhipuai / moonshot / qianfan / cosmosdb / hanadb / infinispan 全部移出主仓库），是 framework 路线必须缴的"模块矩阵维护税"。

**当前剩余 ROI 只剩 SafeGuard/Logger middleware 一项（P2）**。P0 三件套 + P1 持久化 Memory + P1 document readers + P2 结构化输出 / BaseVisitor 都已闭合；DocumentJoiner / QueryRouter 经深度分析判定为 YAGNI（refiner 链已等价覆盖），不做。lynx 在"生产可用度"上对 spring-ai 的硬差距已基本补齐，同时保留 thin library 哲学不动摇。

---

*对比结束。lynx HEAD `d236959`，2026-05-21；对照 spring-ai main HEAD `b63a0d117`。*
