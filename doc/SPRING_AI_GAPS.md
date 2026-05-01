# Lynx vs Spring AI 2.0 — 能力差距快照

> **基线**：
> - Lynx HEAD = `4da6a37` main（含 reasoning 一等公民、MCP v1、Usage cache tokens）
> - Spring AI HEAD ~ `2.0.0-SNAPSHOT`（依赖 mcp 2.0.0-M2，starters 已重组到 `starters/` 子目录）
> - Embabel HEAD ~ `0.4.0-SNAPSHOT`（ToolCallInspector / NestedTool 合并，流式重构）
>
> **本文取代**原 `SPRING_AI_COMPARISON.md` / `SPRING_AI_CAPABILITY_GAPS.md` / `SPRING_AI_GAP_ANALYSIS_2026-04-29.md` / `SPRING_AI_GAP_ANALYSIS_2026-04-30.md` 四个版本快照——只保留这一份当前态。

---

## 0. TL;DR

**Lynx 当前在 6 条线上反超 Spring AI 主干**：reasoning 一等公民 / chat 包零 provider 知识 / iter.Seq2 流式 / ISP 拆接口 / MCP 桥接克制设计 / Usage cache tokens。

**P0/P1 还差的硬刚需**：(1) Anthropic Extra 通道保护让 prompt caching 真正生效（~30 行）；(2) Ollama adapter（生产硬刚需）；(3) MCP v2 反向能力 + ToolExecutionMode（agentic 编排基石）。

**Spring AI 当前 backlog 中相关提案**（P0-1 ReasoningContent / P0-2 AbstractToolCallingChatModel）仍未落地——反超窗口期保持。

---

## 1. Lynx 反超 Spring AI 的点（6 条）

### 1.1 🏆 Reasoning 一等公民

`AssistantMessage.Reasoning string` + `Usage.ReasoningTokens *int64`。Spring AI P0-1 提案至今未落地。详见 [`REASONING.md`](./REASONING.md)。

### 1.2 🏆 chat 包零 provider 知识

provider-specific metadata key 完全下沉到 `models/<provider>/metadata.go`。`grep -r "anthropic\|openai\|google" core/` 零命中——core 包对 provider 完全无知。Spring AI `MessageAggregator` 仍硬编码识别 Google `"isThought"`。

### 1.3 🏆 `iter.Seq2` 流式优势

Lynx 流式中间件就是普通 Go 1.23 迭代器；Spring AI 仍依赖 Reactor `Flux` + `contextView` ceremony。

### 1.4 🏆 ISP 拆接口

- `memory.Store = Reader + Writer + Clearer`
- `VectorStore = Creator + Retriever + Deleter + Info`

Spring AI 单接口仍是巨型接口。

### 1.5 🏆 MCP 桥接的克制设计

Lynx `mcp/` 单包 ~750 LoC 覆盖了双向 hot path——对比 Spring AI `mcp/{common,transport/{webmvc,webflux,stateless}}` + `mcp-annotations` + `auto-configurations/mcp-{client,server}-*` + 5 个 starter，估算 8-10k LoC。

| 维度 | Spring AI | Lynx |
|-----|----------|------|
| 模块数 | ~12 个 Maven module | 1 个 Go package |
| 构造方式 | Builder + autoconfig + customizer 链 | `ToolConfig{...}` / `ProviderConfig{...}` 单参 + `Validate()` |
| 测试夹具 | 无内建（需 Testcontainers / 起进程）| 直接 `mcp.NewInMemoryTransports()` |
| 错误判别 | unchecked `ToolExecutionException` + cause 链 | type-safe `*ToolCallError` + `errors.As` |
| 命名前缀 | 缩写 + 字符过滤 + 64 截断 + 跨连接幂等去重 | `<src>_<tool>` + fail-fast 重名校验 |
| Sync/Async 双轨 | 必须（每组件两份类）| 单同步路径 |

详见 [`MCP.md`](./MCP.md)。

### 1.6 🏆 Cache tokens 类型化

`Usage.CacheReadInputTokens` / `Usage.CacheWriteInputTokens *int64` 一等公民，nil/0/正数三态语义清晰，与 `ReasoningTokens` 一致。

| Spring AI 2.0 | Lynx |
|--------------|------|
| `default Long getCacheReadInputTokens() { return null; }` | `CacheReadInputTokens *int64` 字段 |
| `default Long getCacheWriteInputTokens() { return null; }` | `CacheWriteInputTokens *int64` 字段 |
| ❌ 无 reasoning_tokens | ✅ `ReasoningTokens *int64` |

Anthropic ephemeral cache 写入按 1.25× 计费、读取按 0.1×；OpenAI 自动 cache 命中按 0.5×——`PromptTokens` 反推无法识别这些 multiplier。

---

## 2. Lynx 仍然落后的能力（按严重度排序）

### 🔴 P0：Spring AI 自己也在 backlog 但有清晰提案

#### 2.1 ToolExecutionMode + ToolInvoker

Spring AI 提案 `05-optimization-suggestions.md` P0-2：抽出 14 个 provider 共享的 tool-call 循环。Lynx `tool_middleware.go` 已经做到单点（commit `8e58479`），仍缺：
- `ToolExecutionMode` 枚举（Internal / External / Custom）
- `ToolInvoker` 注入点（user 提供并行 / retry 策略）
- 循环 max iterations 上限

**MCP v1 落地后这条优先级上升**——MCP 远端工具是天然的 External execution 候选，需要明确的 ExecutionMode 区分本地/远端调用语义。

### 🟠 P1：Spring AI 已有成熟代码、Lynx 完全空白

#### 2.2 Anthropic Prompt Caching（保护 Extra 通道）

`models/anthropic/chat.go::buildApiChatRequest` 把用户预设的 `params.System` / `params.Messages` / `params.Tools` 直接 overwrite。让 `buildSystem` / `buildMsgs` / `buildToolParams` 检测 user 是否已经在 Extra params 里预填，预填则跳过——~30 行 adapter 端工作量。

> Cache tokens 字段已在 §1.6 落地。这条只剩 adapter 端的 Extra 通道保护。

#### 2.3 Anthropic Skills + Files API

Skills 4 个内置（XLSX / PPTX / DOCX / PDF）、Files API、SkillContainer。让 Claude 直接生成 Excel / PPT / Word / PDF 报告。

#### 2.4 Anthropic Web Search Tool + Citations

`AnthropicWebSearchTool.java` / `AnthropicWebSearchResult.java` / `Citation.java` / `AnthropicCitationDocument.java`。

#### 2.5 OpenAI Responses API

两边都没有，长期 P1。o1/o3/gpt-5 在 `/v1/chat/completions` 不返回 reasoning text，必须走 `/v1/responses`。DeepSeek-R1 的 OpenAI-compat 端点已被 Lynx 通过 `JSON.ExtraFields` 抓取覆盖，但 OpenAI 官方 o-series 仍待补。

### 🟡 P2：生态级缺口

#### 2.6 持久化 ChatMemory（6 种后端）

Spring AI：JDBC / MongoDB / Neo4j / Redis / Cassandra / CosmosDB。Lynx 仅 `in_memory.go` + `message_window.go`。

**对标方案**：建立 `memories/` 顶层 module，子包 `redis/` / `postgres/` / `mongo/`，复用 `memory.Store = Reader + Writer + Clearer` ISP 接口。

#### 2.7 模型适配器矩阵

当前 Lynx 3 个 chat adapter（anthropic / openai / google）+ 1 个 embedding（openai）。Spring AI 矩阵更全（Bedrock Converse、Vertex AI Embedding 等）。

**真实优先级**：

1. **Ollama**：本地模型通用入口。**头号生产硬刚需**——CLI/桌面应用最常见的「我想用本地 LLM 跑 agent」用例。注意不能简单走 OpenAI-compat（丢失 think option / pull-model / 模型生命周期）
2. **Bedrock Converse**：AWS 用户重要
3. **MiniMax / 月之暗面**：国内厂商，OpenAI-compat 即可走 OpenAI adapter

> **DeepSeek / vLLM / Together / Anyscale / Groq / Fireworks 等**：通过 `option.WithBaseURL` + `JSON.ExtraFields` 走 OpenAI adapter，不需单独包。

#### 2.8 MCP v2 反向能力 + 注解化

> MCP v1 已落地（§1.5）。这里是 v2 候选清单（按优先级）：

| 能力 | 接入点 | 优先级 |
|-----|-------|-------|
| **Server 端 `exchange` 反向调用**（sampling/elicit/progress/ping/logging）| `ctx` 上挂 `*sdkmcp.ServerSession` + `ServerSessionFromContext(ctx)` | 🔴 高 |
| `ToolFilter` 钩子 | `ProviderConfig.Filter func(Source, *Tool) bool` | 🟠 中 |
| OTel 埋点 | `Tool.Call` / `makeServerHandler` 包 span | 🟠 中（与 §2.13 合并）|
| 多源并行 fan-out | `ProviderConfig.ParallelFetch bool` | 🟡 低 |
| 注解化 server tool 暴露 | Go 无 runtime annotation，需 codegen | 🟡 低（性价比不平衡）|

详见 [`MCP.md`](./MCP.md) §13。

### 🟡 P2：Filter / RAG 架构倒退（旧账）

#### 2.9 AbstractFilterExpressionConverter 基类

Spring AI 模板方法基类。Lynx 5 个 store visitor 各自从 0 写 600-800 LOC（共 ~3300 LOC vs Spring 的 ~750）。

**修复路线**：`core/vectorstore/filter/visitors.BaseVisitor`，预计减少 ~60% 代码量。

#### 2.10 RAG QueryRouter + DocumentJoiner

Spring AI 完整 `rag/{preretrieval,retrieval,postretrieval,generation}/`。Lynx 缺 `DocumentJoiner` / `QueryRouter`，且 `pipeline.go` 5 阶段硬编码闭合。

### 🟢 P3：长尾能力

#### 2.11 OpenAI Audio Output

chat 主路径同时返回 text + audio（gpt-4o-audio-preview）。Lynx audio model 独立，不能在 chat call 直接出 audio。

#### 2.12 Google Gemini 高级特性

- CachedContent API
- Extended Usage Metadata（thoughtsTokenCount / cachedContentTokenCount / toolUsePromptTokenCount）
- ThinkingLevel enum（MINIMAL / LOW / MEDIUM / HIGH）
- thoughtSignatures 多轮恢复

#### 2.13 OpenTelemetry 集成

Spring AI 已有 `ChatModelObservation` / `ChatClientObservation` 体系（gen_ai semantic conventions）。Lynx `otelbridge/` 有 slog/log exporter，**但核心代码尚未挂上任何 `otel.Tracer(...)` 埋点**。

详见 [`OBSERVABILITY.md`](./OBSERVABILITY.md) §8。

#### 2.14 SafeGuard / Logger / OutputValidation advisors 套件

Lynx 仅有 `ToolMiddleware` + `MemoryMiddleware`。Spring AI 提供 SafeGuardAdvisor、LoggerAdvisor、OutputValidationAdvisor 等成套。

#### 2.15 Embedding adapter

Spring AI 多个 embedding model；Lynx 仅 OpenAI。

---

## 3. Embabel 的最近变化（参考）

> Embabel 是 Spring AI 生态外的 agent 框架，Lynx `agent/` 设计参照其 GOAP + 黑板模式。

| 维度 | Embabel 0.4.0-SNAPSHOT | Lynx 应做 |
|-----|----------------------|----------|
| **ToolCallInspector** SPI | 拦截/观察 tool 执行流；`StreamingLlmOperationsFactory` + `LlmMessageStreamer` 流式重构 | Lynx 的 `iter.Seq2` 流式 + `StreamMiddleware` 已天然覆盖此能力，agent 框架落地时把 ToolCallInspector 语义映射到 middleware 链 + 事件总线即可 |
| **NestedTool** 接口合并 | Progressive 与 Playbook tool 层级统一为 `NestedTool` 超接口 | Lynx 当前 tool 层级扁平；agent 框架引入 sub-agent / nested action 时同步合并设计 |
| `Budget` 参数下传 | 跨 nested subprocess 的 budget 聚合 | Lynx agent 框架的 cost/budget 字段需要从 `Usage` 累积往上传 |
| `ReplanRequestedException` 扩展到 ConcurrentAgentProcess | 并发 agent 也支持 replanning | Lynx 用 `error` + `errors.As` 即可表达 |
| TokenCounter SPI | 细粒度 cost tracking | Lynx `core/tokenizer/` 是天然实现入口 |

---

## 4. 推荐路线图

### 第一阶段（~1-2 周）：填生产级硬刚需

1. **保护 Anthropic Extra 通道不被 overwrite**（§2.2）—— 优先级**最高**，~30 行
2. **Ollama adapter**（§2.7 #1）—— 头号生产硬刚需，本地模型 + think 模式覆盖
3. **OpenAI adapter 接 DeepSeek/vLLM 的 README 示例** —— 防止后人重复提建独立包
4. **核心代码挂 OTel 埋点**（§2.13）—— 接收侧已就绪，按 [`OBSERVABILITY.md`](./OBSERVABILITY.md) §4 清单逐点

### 第二阶段（~3-4 周）：解锁 agentic 场景

5. **MCP v2 反向能力**（§2.8 #1）—— `ServerSessionFromContext`，让本地工具能反向 LLM 调用
6. **ToolExecutionMode + ToolInvoker**（§2.1）—— 配合 MCP，识别本地 vs 远端执行
7. **持久化 Memory（Redis / Postgres）**（§2.6）—— 单后端先做

### 第三阶段（~4-6 周）：清理架构倒退

8. **AbstractFilterExpressionConverter / BaseVisitor**（§2.9）—— 减 ~2500 LOC
9. **RAG QueryRouter + DocumentJoiner**（§2.10）
10. **OpenAI Responses API**（§2.5）—— o-series reasoning 解锁
11. **MCP v2 ToolFilter 钩子 + OTel 埋点**（§2.8）

### 第四阶段（~视需求）：长尾完善

12. Google Gemini 高级特性（§2.12）
13. Bedrock Converse（§2.7 #2）
14. SafeGuard / Logger / OutputValidation advisors（§2.14）
15. Anthropic Skills + Web Search + Citations（§2.3 / §2.4）

---

## 5. 一句话定档

> 对照 Spring AI 时不照抄，照搬克制原则做薄壳——然后在文档层面把「为什么不抄」讲清楚。这套打法在 reasoning（不抄 P0-1 的 record 重设计）、MCP（不抄全家桶 12 module 拆分）、observability（不抄 Micrometer 抽象层）、cache tokens（不抄 default null Long）四条线上都已经验证成立。下一阶段的 reaim 是把「反超清单」从字段/抽象级别推进到生产级别——Ollama adapter + Anthropic Extra 通道保护是最近 ROI 最高的两条。
