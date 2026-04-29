# Lynx vs Spring AI 2.0 — MCP 落地后能力差距快照（2026-04-30）

> **基线**：
> - Lynx HEAD = `f75f21a` Merge `feat/thinking-support`，含 reasoning 完整落地 + Usage cache tokens + MCP v1
> - Spring AI HEAD = `b9d1c5303`（`2.0.0-SNAPSHOT`，距上次快照基本无新动）
> - 本文取代 `SPRING_AI_GAP_ANALYSIS_2026-04-29.md` 作为当前快照；该文档 §3.7（MCP 全家桶）和 §3.3 末尾（Cache tokens 字段）的判断已被本文修订
>
> **写作目的**：过去 24 小时 Lynx 落地了两件事——
> 1. `mcp/` 单包桥接（commit `5069ead`，~750 LOC + DESIGN.md + 对比文档）
> 2. `Usage.CacheReadInputTokens` / `Usage.CacheWriteInputTokens` 一等公民字段（commit `eefebe4`）
>
> 本文重排"还差什么"，把已经落地的两条从待办移走，并把"Lynx 反超"列表扩到 6 条。

---

## 0. 一页式总览（24 小时内变化）

| 维度 | 上一次（2026-04-29） | 当前（2026-04-30） | 变化方向 |
|-----|------------------|-----------------|---------|
| **Reasoning / thinking 抽象** | ✅ Lynx 反超 | ✅ 持续反超 | 不变 |
| `Usage.ReasoningTokens` | ✅ 已暴露 | ✅ 持续 | 不变 |
| **`Usage.CacheReadInputTokens` / `CacheWriteInputTokens`** | ❌ | ✅ 新落地（`eefebe4`）| **Lynx 追平 Spring AI 2.0** |
| Anthropic prompt caching（Extra 通道保护）| ❌ buildSystem/buildMsgs overwrite | ❌ 仍未保护 | 不变（核心方案不变）|
| Anthropic Skills + Files API | ❌ | ❌ | 持续落后 |
| Anthropic Web Search Tool / Citation | ❌ | ❌ | 持续落后 |
| **MCP 全家桶** | ❌ 完全空白 | ✅ **v1 桥接落地**（mcp/ 单包，~750 LOC）| **Lynx 在轻量级 MCP 集成上反超** |
| 持久化 ChatMemory（6 种后端）| ❌（仅 in-memory）| ❌ | 持续落后 |
| OpenAI Responses API | ❌ | ❌ | 双方都没（Spring AI 也未落）|
| AbstractToolCallingChatModel | – | ❌ | Spring AI P0-2 仍未落地 |
| Bedrock Converse / Vertex AI Embedding | ❌ | ❌ | Spring AI 已支持 |
| Filter Converter 基类 | ❌ | ❌ | 5 store × ~700 LOC 旧账仍在 |
| RAG QueryRouter / DocumentJoiner | ❌ | ❌ | 旧账仍在 |
| Ollama adapter | ❌ | ❌ | 仍是头号生产硬刚需 |

**两个新事件细节**：

1. **MCP v1**：用 `modelcontextprotocol/go-sdk@v1.5.0` 做薄壳桥接，单包 `mcp/`（5 个生产 .go + 4 个测试）。客户端（`Tool` / `Provider`）+ 服务端（`RegisterTools`）双向打通，`ToolCallError` + `errors.As` 错误判别，`WithMeta` opt-in 元数据透传，`Provider.OnToolListChanged` 直接 method value 接入 SDK 通知。详细对比见 `doc/MCP_VS_SPRING_AI.md`（1065 行）。
2. **Cache tokens**：`core/model/usage.go` 增加 `CacheReadInputTokens` / `CacheWriteInputTokens *int64` 字段，类型形态与 `ReasoningTokens` 一致；anthropic / openai adapter 在 > 0 时透传。规避了从 `PromptTokens` 反推计费成本误差。

**新的反超清单**：现在 Lynx 在 6 条线上比 Spring AI 当前 HEAD 更前——见 §2。

---

## 1. Reasoning + Cache 双字段在 Usage 上的对齐

`core/model/usage.go` 当前形态：

```go
type Usage struct {
    PromptTokens     int64  `json:"prompt_tokens"`
    CompletionTokens int64  `json:"completion_tokens"`
    ReasoningTokens  *int64 `json:"reasoning_tokens,omitempty"`
    CacheReadInputTokens  *int64 `json:"cache_read_input_tokens,omitempty"`
    CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`
    OriginalUsage    any    `json:"original_usage,omitempty"`
}
```

Spring AI `Usage` 接口当前形态（行 `spring-ai-model/.../chat/metadata/Usage.java`）：

```java
public interface Usage {
    Integer getPromptTokens();
    Integer getCompletionTokens();
    default @Nullable Long getCacheReadInputTokens() { return null; }
    default @Nullable Long getCacheWriteInputTokens() { return null; }
    // ❌ 仍然没有 getReasoningTokens()
}
```

Lynx 在三个维度（reasoning + cache_read + cache_write）上都已经是一等公民，Spring AI 仅 cache_read / cache_write 是 2.0 新增 default 方法，reasoning 维度仍未补齐。

> **设计对齐细节**：三个 `*int64` 字段都遵循同一约定——nil = 提供商未透出该维度，0 = 显式表示该次调用没发生（如本次无缓存命中）。Adapter 在 `> 0` 才赋值，避免 SDK 在字段缺失时把零值误传。

---

## 2. Lynx 当前反超 Spring AI 的点（6 条）

### 2.1 🏆 Reasoning 一等公民（沿用 2026-04-29）
`AssistantMessage.Reasoning string` + `Usage.ReasoningTokens *int64`，Spring AI P0-1 提案至今未落地。

### 2.2 🏆 chat 包零 provider 知识（沿用）
provider-specific metadata key 完全下沉到 `models/<provider>/metadata.go`；Spring AI `MessageAggregator` 仍硬编码识别 Google `"isThought"`。

### 2.3 🏆 iter.Seq2 流式优势（沿用）
Lynx 流式中间件就是普通迭代器；Spring AI 仍依赖 Reactor `Flux` + `contextView` ceremony。

### 2.4 🏆 ISP 拆接口（沿用）
`memory.Store = Reader + Writer + Clearer`、`VectorStore = Creator + Retriever + Deleter + Info`；Spring AI 单接口仍是巨型接口。

### 2.5 🏆 **MCP 桥接的克制设计**（**新增**）
24 小时内落地。对比 Spring AI 全家桶（`mcp/{common,transport/{webmvc,webflux},mcp-annotations}` + `auto-configurations/mcp-{client,server}-*` + 多 starter，估算 8-10k LoC），Lynx `mcp/` 单包 ~750 LoC 覆盖了双向 hot path：

| 维度 | Spring AI | Lynx |
|-----|----------|------|
| 模块数 | ~12 个 Maven module | 1 个 Go package |
| 构造方式 | Builder 模式 + autoconfig + customizer 链 | `ToolConfig{...}` / `ProviderConfig{...}` 单参 + `Validate()` |
| 测试夹具 | 无内建（需 Testcontainers / 起进程）| 直接 `mcp.NewInMemoryTransports()`（go-sdk 自带）|
| 错误判别 | unchecked `ToolExecutionException` + cause 链 | type-safe `*ToolCallError{ToolName, Message}` + `errors.As` |
| 命名前缀 | 缩写+字符过滤+64 截断+跨连接幂等去重 | `<src>_<tool>` + fail-fast 重名校验 |
| 多源 fan-out | `parallelStream` 也未做（顺序）| 顺序（同步设计）|
| Sync/Async 双轨 | 必须（每组件两份类）| 单同步路径 |

**详细对比**：`doc/MCP_VS_SPRING_AI.md`（16 章节，1065 行），含每条决策的 `file:line` 引用与代码并排。

> **取舍声明**：Lynx v1 故意没做 server 端 `exchange` 反向能力（sampling/elicit/progress/ping/logging）和注解扫描——这两块是 v2 候选。MCP 全家桶能力上 Spring AI 整体仍更全，但 Lynx 在"消费 + 暴露 chat tool"两条 hot path 上的 API 表面、构造复杂度、测试便利性都明显更轻。

### 2.6 🏆 Cache tokens 类型化（**新增**）
24 小时内落地。Anthropic ephemeral cache 写入按 1.25× 计费、读取按 0.1×；OpenAI 自动 cache 命中按 0.5× 计费——`PromptTokens` 反推无法识别这些 multiplier。Lynx 现在 nil/0/正数三态语义清晰，与 `ReasoningTokens` 一致。

---

## 3. Lynx 仍然落后的能力（按严重度排序，去重后版本）

### 🔴 P0：Spring AI 自己也在 backlog 但有清晰提案

#### 3.1 ToolExecutionMode + ToolInvoker（不变）
Spring AI 提案 `05-optimization-suggestions.md` P0-2：抽出 14 个 provider 共享的 tool-call 循环。Lynx `tool_middleware.go` 已经做到单点（commit `8e58479`），仍缺：
- `ToolExecutionMode` 枚举（Internal / External / Custom）
- `ToolInvoker` 注入点（user 提供并行 / retry 策略）
- 循环 max iterations 上限

**MCP v1 落地后这条优先级上升**——MCP 远端工具是天然的 External execution 候选，需要明确的 ExecutionMode 区分本地/远端调用语义。

### 🟠 P1：Spring AI 已有成熟代码、Lynx 完全空白

#### 3.2 Anthropic Prompt Caching（保护 Extra 通道，不抄 Spring 4 类）
**核心方案不变**：`models/anthropic/chat.go::buildApiChatRequest` 把用户预设的 `params.System` / `params.Messages` / `params.Tools` 直接 overwrite（line 191-193, 93-96）；让 `buildSystem` / `buildMsgs` / `buildToolParams` 检测 user 是否已经在 Extra params 里预填，预填则跳过。

**进度更新**：`Usage.CacheReadInputTokens` / `CacheWriteInputTokens` 在 24 小时内落地（§2.6），现在只剩 adapter 端 ~30 行的 Extra 通道保护。**这条 P1 的工作量已减半**。

#### 3.3 Anthropic Skills + Files API（不变）
Skills 4 个内置（XLSX / PPTX / DOCX / PDF）、Files API、SkillContainer。让 Claude 直接生成 Excel / PPT / Word / PDF 报告。

#### 3.4 Anthropic Web Search Tool + Citations（不变）
`AnthropicWebSearchTool.java` / `AnthropicWebSearchResult.java` / `Citation.java` / `AnthropicCitationDocument.java`。

#### 3.5 OpenAI Responses API（不变）
两边都没有，长期 P1：o1/o3/gpt-5 在 `/v1/chat/completions` 不返回 reasoning text，必须走 `/v1/responses`。DeepSeek-R1 的 OpenAI-compat 端点已被 Lynx 通过 `JSON.ExtraFields` 抓取覆盖，但 OpenAI 官方 o-series 仍待补。

### 🟡 P2：生态级缺口

#### 3.6 ~~MCP 全家桶~~ → **降级到 v2 反向能力 + 注解化**

> **2026-04-30 修订**：原 §3.7 已不再是"完全空白"。MCP v1（`mcp/` 单包桥接）落地后从 P2 表中移除主条目。

**剩余 v2 候选**（按优先级）：
1. **Server 端 `exchange` 反向能力**（高优先）：`ctx` 上挂 `*sdkmcp.ServerSession` + `ServerSessionFromContext(ctx)`，让 lynx server 端工具能反向 sampling / elicit / progress / ping。详见 `doc/MCP_VS_SPRING_AI.md §5.4` 的接入草图。
2. **`ToolFilter` 钩子**（中优先）：`ProviderConfig.Filter func(Source, *Tool) bool`，应对信任域过滤需求。
3. **多源并行 fan-out**（低优先，看实测）：`ProviderConfig.ParallelFetch bool`，多 source 时用 `errgroup`。
4. **OTel 埋点**（中优先，与 §3.16 合并）：`Tool.Call` / `makeServerHandler` 包 span，对齐 GenAI 语义规范。
5. **注解化 server tool 暴露**：低优先；Go 无 runtime annotation，需 codegen，收益不平衡。

#### 3.7 持久化 ChatMemory（不变）
Spring AI 6 种后端：JDBC / MongoDB / Neo4j / Redis / Cassandra / CosmosDB。Lynx 仅 `in_memory.go` + `message_window.go`。

**Lynx 对标方案**：建立 `memories/` 顶层 module，子包 `redis/` / `postgres/` / `mongo/`，复用 `memory.Store = Reader + Writer + Clearer` ISP 接口。

#### 3.8 模型适配器矩阵（不变）
当前 Lynx 3 个 chat adapter（anthropic / openai / google）+ 1 个 embedding（openai）。

**真实优先级**（精简后）：
1. **Ollama**：本地模型通用入口。**仍是头号生产硬刚需**——CLI/桌面应用最常见的"我想用本地 LLM 跑 agent"用例。注意不能简单走 OpenAI-compat（丢失 think option / pull-model / 模型生命周期）。
2. **Bedrock Converse**：AWS 用户重要。
3. **MiniMax / 月之暗面**：国内厂商，OpenAI-compat 即可走 OpenAI adapter。

> **DeepSeek / vLLM / Together / Anyscale / Groq / Fireworks 等**：通过 `option.WithBaseURL` + `JSON.ExtraFields` 走 OpenAI adapter，不需单独包。设计依据见 `SPRING_AI_GAP_ANALYSIS_2026-04-29.md §3.9` 的论证。

### 🟡 P2：Filter / RAG 架构倒退（旧账，不变）

#### 3.9 AbstractFilterExpressionConverter 基类
Spring AI 模板方法基类；Lynx 5 个 store visitor 各自从 0 写 600-800 LOC（共 ~3300 LOC vs Spring 的 ~750）。修复路线：`core/vectorstore/filter/visitors/BaseVisitor`。

#### 3.10 RAG QueryRouter + DocumentJoiner
Spring AI 完整 `rag/{preretrieval,retrieval,postretrieval,generation}/`。Lynx 缺 `DocumentJoiner` / `QueryRouter`，且 `pipeline.go` 5 阶段硬编码闭合。

### 🟢 P3：长尾能力

#### 3.11 OpenAI Audio Output（不变）
chat 主路径同时返回 text + audio（gpt-4o-audio-preview）。Lynx audio model 独立，不能在 chat call 直接出 audio。

#### 3.12 Google Gemini 高级特性（不变）
- CachedContent API
- Extended Usage Metadata（thoughtsTokenCount / cachedContentTokenCount / toolUsePromptTokenCount）
- ThinkingLevel enum（MINIMAL / LOW / MEDIUM / HIGH）
- thoughtSignatures 多轮恢复

#### 3.13 Embedding adapter（不变）
Spring AI 多个 embedding model；Lynx 仅 OpenAI。

#### 3.14 OpenTelemetry 集成（不变 / 与 MCP §3.6 #4 合并）
Spring AI 已有 ChatModelObservation / ChatClientObservation 体系（gen_ai semantic conventions）。Lynx `otelbridge/` 有 slog exporter skeleton 未与 chat / RAG / vector store / **MCP** 集成。

#### 3.15 SafeGuardAdvisor / 等 advisor 套件（不变）
Lynx 仅有 `ToolMiddleware` + `MemoryMiddleware`。

---

## 4. 24 小时内的双方变化

### 4.1 Lynx 端落地

| Commit | 内容 | 影响 |
|--------|-----|------|
| `5069ead` | feat(mcp): MCP 桥接 v1 | 移除 `SPRING_AI_GAP_ANALYSIS_2026-04-29.md §3.7` 全家桶整条；新增反超点 §2.5 |
| `eefebe4` | feat(usage): cache read/write input tokens | 移除 §3.3 末尾 cache tokens 子项；新增反超点 §2.6 |
| `f75f21a` | Merge `feat/thinking-support` | 把 reasoning 落地正式合到 main，巩固 §2.1 反超 |

### 4.2 Spring AI 端

距上次扫描 30 commits 后**结构性无变化**：MCP SDK 仍 2.0.0-M2、P0-1（ReasoningContent）/ P0-2（AbstractToolCallingChatModel）仍 backlog 未动。

### 4.3 反超窗口持续期

> P0-1 / P0-2 不动 + Lynx 这一天落地 mcp + cache tokens —— Lynx 反超清单从 5 条扩到 6 条（reasoning / chat 零 provider / iter.Seq2 / ISP / MCP 克制 / cache tokens）。
>
> Spring AI 真正可能反扑的方向：(a) P0-1 ReasoningContent 落地；(b) annotation-driven MCP 新功能（如 server-to-client 反向工具流的注解化）。但目前两条都没动。

---

## 5. 推荐路线图（按 ROI 重排）

### 第一阶段（~1-2 周）：填生产级"硬刚需"
1. ~~**保护 Anthropic Extra 通道不被 overwrite**~~ —— 仍未做（§3.2），优先级**最高**
2. ~~**`Usage.CacheRead/WriteInputTokens`**~~ —— ✅ 已落地（`eefebe4`）
3. **Ollama adapter**（§3.8 #1）—— 头号生产硬刚需，本地模型 + think 模式覆盖
4. **Memory middleware 文档明确链中位置**（来自 2026-04-29 §3.2 撤销决策的衍生事项）—— 补 godoc / README
5. **OpenAI adapter 接 DeepSeek/vLLM 的 README 示例** —— 防止后人重复提建独立包

### 第二阶段（~3-4 周）：解锁 agentic 场景（MCP 已就位）
6. ~~**MCP client + server**~~ —— ✅ 已落地（v1，`5069ead`）
7. **MCP v2 第一项：server 端 `exchange` 反向能力**（§3.6 #1）—— `ctx` 上挂 server session，让 lynx 工具能反向 LLM 调用
8. **ToolExecutionMode + ToolInvoker**（§3.1）—— 配合 MCP，识别本地 vs 远端执行
9. **持久化 Memory（Redis / Postgres）**（§3.7）—— 单后端先做

### 第三阶段（~4-6 周）：清理架构倒退
10. **AbstractFilterExpressionConverter / BaseVisitor**（§3.9）—— 减 ~2500 LOC
11. **RAG QueryRouter + DocumentJoiner**（§3.10）
12. **OpenAI Responses API**（§3.5）—— o-series reasoning 解锁
13. **MCP v2 第二项：ToolFilter 钩子 + OTel 埋点**（§3.6 #2 + §3.14）

### 第四阶段（~视需求）：长尾完善
14. Google Gemini 高级特性（§3.12）
15. Bedrock Converse（§3.8 #2）
16. SafeGuard / Logger / OutputValidation advisors（§3.15）
17. Anthropic Skills + Web Search + Citations（§3.3 / §3.4）

---

## 6. 与历史文档的关系

| 文档 | 状态 |
|-----|------|
| `SPRING_AI_COMPARISON.md`（2026-04-20）| §3 / §5 / §4 等结构性观察仍然有效；§2 Advisor Ordering 在 2026-04-29 §3.2 撤销 |
| `SPRING_AI_CAPABILITY_GAPS.md`（2026-04-26）| §1.3 Reasoning 步骤已被 reasoning 合并取代；§2-§7 仍生效 |
| `SPRING_AI_THINKING_ARCHITECTURE.md` | §9 已 superseded；§1-§8 作为历史参照 |
| `REASONING_UNIFIED_DESIGN.md` | reasoning 实现的契约文档，已落地完毕 |
| `SPRING_AI_GAP_ANALYSIS_2026-04-29.md` | **被本文取代**：§3.7（MCP 全家桶）/ §3.3 末尾（Cache tokens）已修订 |
| **`MCP_DESIGN.md`**（2026-04-29）| MCP 设计稿，含 Spring AI 对照表与设计取舍 |
| **`MCP_VS_SPRING_AI.md`**（2026-04-30）| MCP 模块**深度对比**（16 章 / 1065 行），含 file:line 引用 |
| **`mcp/DESIGN.md`**（模块内）| Lynx mcp 包实现 + 使用文档（含完整客户端/服务端代码示例）|
| **本文（`SPRING_AI_GAP_ANALYSIS_2026-04-30.md`）** | 当前快照，焦点在**MCP + Cache tokens 落地后还差什么** |

---

## 7. 一句话定档

> **MCP v1 + Cache tokens 落地后，Lynx 反超清单从 5 条扩到 6 条（reasoning / chat 零 provider / iter.Seq2 / ISP / MCP 克制 / cache tokens）。剩余 P0/P1 集中在三块：(1) Anthropic Extra 通道保护让 prompt caching 真正生效（~30 行）；(2) Ollama adapter（生产硬刚需）；(3) MCP v2 反向能力 + ToolExecutionMode（agentic 编排基石）。Filter Converter 基类 + RAG 阶段化是中期旧账。Spring AI 自己 P0-1（ReasoningContent）/ P0-2（AbstractToolCallingChatModel）仍未落地，反超窗口期保持。**
>
> 这一天的核心收获不是"加了多少代码"，而是验证了一个判断：**对照 Spring AI 时不照抄，照搬克制原则做薄壳——然后在文档层面把"为什么不抄"讲清楚，这套打法在 MCP 这种生态级模块上仍然成立**。MCP 桥接 750 LoC 顶 Spring AI 8-10k LoC 的关键能力，是因为 lynx 不假设 IoC 容器、不做注解魔法、不双轨 Sync/Async——三个负空间设计加在一起的复利。
