# GitNexus（Akon Labs）—— Agent 上下文供给侧

> 实测：TypeScript monorepo，943 个 `.ts`/`.tsx` 文件，**301,972 行**（CLI + shared + web）。
> 结构：`gitnexus/`（npm 包：CLI + MCP server + HTTP API + 摄取管线 + LadybugDB 图 + embeddings）、`gitnexus-web/`（Vite+React 瘦客户端）、`gitnexus-shared/`（共享类型）。
> 许可：PolyForm Noncommercial。

---

## 0. 一句话定位

**GitNexus 与其余六个项目不在同一条轴上。** 它不是 Agent 框架，而是 **Agent 的上下文供给侧**：把一个代码库索引成知识图谱（符号、调用、继承、DI、路由、社区、执行流、PDG 数据流），然后通过 MCP 暴露成 16+ 个工具，让 Cursor / Claude Code / Codex 这类编码 Agent "不再瞎改"。

对 scope 而言，它的可比维度有三条，且都很实：

1. **检索质量的另一条路** —— 结构化图谱检索 vs 向量 RAG（对应 scope 的 `rag` + `vectorstores`）
2. **MCP 工具的服务端设计** —— 输出预算、只读模式、陈旧性建模（对应 scope 的 `mcp` + `tools`）
3. **管线架构与工程治理** —— 静态 DAG、无插件、显式失败建模（对应 scope 的 `etl` + 全仓治理）

---

## 1. 维度一：检索 —— 结构图谱 vs 向量 RAG

### 1.1 GitNexus 的答案

19 个阶段的静态 DAG，产出一张知识图谱：

```
scan → structure → [springConfig, markdown, cobol] → parse → [routes, tools, orm]
  → crossFile → scopeResolution → [springAutoConfiguration, springAop]
  → pruneLocalSymbols → mro → springAopInheritance → di → communities → processes
```

`--pdg` 再加两个阶段（`taintSummaries`、`callSummaries`）。

图上的边类型（实测）：`CONTAINS` / `DEFINES` / `IMPORTS` / `CALLS` / `EXTENDS` / `METHOD_OVERRIDES` / `METHOD_IMPLEMENTS` / `INJECTS` / `QUERIES` / `HANDLES_ROUTE` / `HANDLES_TOOL` / `ADVISED_BY` / `MEMBER_OF` / `STEP_IN_PROCESS` / `CONTRACT_LINK` / `CDG` / `REACHING_DEF`。

检索是**混合的**：BM25 + 向量 + Reciprocal Rank Fusion，再叠图查询（`context` 取调用者/被调用者、`impact` 走爆炸半径、`trace` 求两个符号间的最短有向路径）。

**关键洞察**：对"这个函数改了会影响谁"这类问题，**向量相似度是错的工具**。答案不在"语义相近的代码"里，而在"调用图的上游闭包"里。GitNexus 把这个洞察做成了产品。

### 1.2 与 scope 的对照

scope 的 `rag` module 定位是**通用 RAG 的组合原语**：

```
Retriever 是窄腰
  ├─ QueryTransformer（rewrite / translation / compression）
  ├─ QueryExpander（multi）
  ├─ Retriever（vectorstore / fusion / tool）
  └─ CandidateRefiner（dedup / topk / model）
```

`rag/CLAUDE.md`：

> **RAG 是一组小接口的自由组合,不是一条框架化流水线**
> **Retriever 是窄腰**:围绕它用组合函数（叠加 transformer / expander / refiner）显式表达能力

**两者是互补而非竞争**：

| | GitNexus | scope `rag` |
|---|---|---|
| 语料 | 单一领域（代码），schema 固定 | 任意文档 |
| 检索单位 | 符号 + 关系边 | `document.Document` + score |
| 主检索信号 | 图拓扑（调用/继承/DI） | 向量相似度 + BM25 |
| 融合 | RRF（跨 repo 成员） | `retriever_fusion.go`（多路 Retriever 合并，同 ID 保最高分） |
| 是否可插入对方 | GitNexus 可以是 scope 的一个 `Retriever` 实现 | scope 可以是 GitNexus 的 embedding 后端 |

**scope 的 `rag.Retriever` 完全可以由一个"图谱检索器"实现** —— 这正是 `rag/CLAUDE.md` 说的「路由写成自定义 Retriever，合并写成 Refiner」。GitNexus 证明了这个窄腰选对了：一个和向量完全无关的检索策略，能无损地塞进同一个接口。

### 1.3 一条 scope 应当吸收的原则

ARCHITECTURE.md 里关于路由抽取器的这句话，是整个文档最有价值的一行：

> That extractor is deliberately **precision-weighted**: `route_map` presents its output as fact, so a `startsWith` namespace test, a bare `pathname === '/'` without a verb, and any regex it cannot translate exactly are all dropped rather than guessed at.
> **A missing route is a coverage limit; an invented one is a lie.**

以及 CLAUDE.md 里对 `risk: UNKNOWN` 的处理：

> **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index. `impact` pairs `UNKNOWN` with a `riskNote` saying so.

**把"我不知道"和"没有"分成两个可区分的答案。** 这是喂给 LLM 的数据必须遵守的第一原则 —— 模型不会怀疑一个空数组，它会当作事实。

scope 的对应条款分散在几处，且**没有统一表述**：

- `rag/CLAUDE.md`：「并发结果按声明顺序归并，任一分支失败则本次检索失败，**不把不完整结果伪装成完整命中**」✅ 同一原则
- `rag/CLAUDE.md`：「❌ 把 nil/空输出静默降级成 identity」✅
- `core/CLAUDE.md`：「❌ 让探测错误以 0/空值静默返回」✅
- `tools/CLAUDE.md`：「输出超限截断而非报错：带 truncated 标记，LLM 据此决定下一步」✅

**结论**：scope 已经有这条原则的四个分身，但没有一句统一的判词。GitNexus 的两句话（"an invented one is a lie" / "a confident empty answer is worse than a failure"）值得作为 `DESIGN_PHILOSOPHY.md` 的一条显式原则收录 —— 因为它是所有喂 LLM 的能力（tool 结果、retrieval 结果、model 元数据探测）的共同不变量。

---

## 2. 维度二：MCP 服务端设计

### 2.1 输出预算

```typescript
const BUDGETED_TOOLS = new Set(['query', 'context', 'impact']);
export const MCP_TOKEN_ESTIMATE_BYTES = 4;
export const MCP_TRUNCATION_MARKER = '\n…';

export function resolveMcpMaxTokens(toolName, args, env): number | undefined
export function applyMcpMaxTokens(text, maxTokens): string
function utf8Prefix(text: string, maxBytes: number): string  // ← 按 code point 切，不切坏 UTF-8
```

三层解析：`args.maxTokens` → `GITNEXUS_MCP_DEFAULT_MAX_TOKENS` 环境变量 → 无限制。只有 3 个"可能返回海量结果"的工具受预算约束。

**细节到位**：`utf8Prefix` 按 code point 累加字节数，保证截断后仍是合法 UTF-8。

### 2.2 只读模式

```typescript
export const MCP_READ_ONLY_TOOLS = new Set([
  'list_repos','query','context','detect_changes','check','impact',
  'explain','pdg_query','route_map','tool_map','shape_check','api_impact','trace',
]);
export function resolveMcpReadOnlyMode(env): boolean
```

`GITNEXUS_MCP_READ_ONLY=1` 时只暴露只读工具（`rename`、`group_sync` 这类会写的被隐藏）。**这是给 Agent 的能力衰减** —— 与 scope 的 `CapabilitySet`「子能力只能是父能力的子集，不能递归提权」是同一思想在 MCP 层的表达。

### 2.3 陈旧性与不完整性建模 ⭐

这是 GitNexus 最深的一处设计。`staleness.ts` + `index-freshness.ts` + `incompleteReasons`：

```
incompleteReasons: [
  "incremental-in-progress",        // 上次增量没跑完
  "embedding-checkpoint-pending",   // 部分节点的 embedding 因端点抖动失败，已记账待补
  "graph-write-collapsed",          // 写回的边数远少于产出的边数
]
```

三种状态语义**完全不同**，且被区分对待：

- 前两种：「这次跑完了它承诺的事，留了活给下次」→ 退出码 0
- `graph-write-collapsed`：「大部分边没了，但每个查询都会给出缺边的答案而不是报错」→ **退出码非零**

GUARDRAILS.md 对它的解释是整份文档最锋利的一句：

> Nothing throws: the DB holds rows and the metadata is valid, so every query answers with missing edges rather than an error — **a confident empty answer, which is worse than a failure because it looks like a result.**

对应的 embedding 局部失败处理也一样精细：

> 长 analyze 遇到不稳定的 embedding 端点时，容忍有界的子批失败而不是整轮中止：**删掉受影响节点的 embedding 行（所以它们要么零行、要么全集，绝不半集）**，并把这些节点记进 `embeddingCheckpoint`。`stats.embeddings` 保持诚实的非零计数。

**"要么零行要么全集，绝不半集"** —— 这是一个明确的原子性不变量，写在了运维文档里。

### 2.4 与 scope 的对照

| 能力 | GitNexus | scope |
|---|---|---|
| 输出预算/截断 | ✅ 按工具 + 三层解析 + UTF-8 安全 | ✅ `tools`「输出超限截断而非报错，带 truncated 标记」 |
| 只读/能力衰减 | ✅ 环境变量控制工具集 | ✅ `CapabilitySet` 从父衰减；`tools` 手动注册无全局 registry |
| 部分失败的原子性 | ✅ 零行或全集，checkpoint 记账 | ✅ `IndexRequest` 分批规则；`etl` `ErrSourceTooLarge` 不产出部分文档 |
| **不完整性的分级建模** | ✅ 三种 reason + 差异化退出码 | ⚠️ **无对应物** |
| **陈旧性显式暴露** | ✅ `staleness.ts` 比对 `lastCommit` vs `HEAD` | ⚠️ 无对应物（scope 的 `skills` 是懒读 per-call，绕开了这个问题） |

**缺口**：scope 的 `vectorstores` / `rag` 目前没有"索引与源不同步"的概念。一个 `Searcher` 返回的 Document 可能来自三天前的文档版本，调用方无从得知。GitNexus 证明这在真实使用中是首要故障模式。

不过要公允：scope 的 `vectorstore` 是**通用**存储契约，"源"是什么由调用方定义，框架无从判断陈旧。GitNexus 能做是因为它的源固定是 git repo。**这个缺口的正确落点是 `etl` 的 metadata（reader 记录源版本/哈希）+ 调用方策略，不是 `core/vectorstore` 的契约。** 记账，按 `DESIGN_PHILOSOPHY §2.5` 的判据 —— "是每个消费者都需要它，还是只有这一个消费方需要"——，倾向不下沉。

---

## 3. 维度三：管线架构

### 3.1 静态 DAG，无插件

> `runner.ts` — **static phase graph, no plugins, compile-time type safety.**

执行合同：

1. **校验** —— Kahn 拓扑排序。拒绝：重名、缺依赖、成环（DFS 追出具体环路径 `A -> B -> C -> A`，并报出被传递阻塞的下游数量）
2. **执行** —— 拓扑序顺序执行。每个 phase 拿到：
   - `ctx: PipelineContext` —— 共享可变 `KnowledgeGraph`
   - `deps: ReadonlyMap<string, PhaseResult>` —— **只含声明过的依赖**（runner 主动过滤结果 map，**防止隐式耦合**）
3. **错误** —— 用 phase 名包裹，发终态 error 事件，吞掉 progress handler 的错误以保住原始 cause
4. **所有权** —— `BindingAccumulator` 在 `parse` 创建，由 `crossFile` 在 `finally` 里销毁。**"No other phase should take ownership."**

### 3.2 与 scope 的对照 —— 三处强同构

**① "runner 主动过滤 deps map 防止隐式耦合"**

这是把"依赖必须显式声明"从约定变成了**机制强制**：你没声明 `parse`，你就拿不到 `parse` 的输出，代码根本编译不过（`getPhaseOutput<T>(deps, 'name')`）。

scope 的同型机制是 architecture test：

> 全图 architecture test 扫描所有非测试生产 `.go` 文件（包括受 build tag 约束的文件），锁定 package 集合、允许的内部直连边和关键外部依赖归属。

两者都是**把架构约束变成机器可验证的东西**，只是一个在运行期数据流上做，一个在编译期 import 图上做。

**② "无插件"**

GitNexus 明确拒绝插件化管线。scope 的对应立场（`DESIGN_PHILOSOPHY §2.3` + `core/CLAUDE.md`）：❌ 新增第二套 Advisor/Hook/Interceptor/Plugin 扩展链。

**③ 显式所有权与生命周期**

`BindingAccumulator` 由谁创建、谁销毁、在哪个 `finally` 里 —— 写进了架构文档。scope 的对应红线：

> **并发 / 事务 / 安全约束**：goroutine 所有权与生命周期、锁持有顺序、channel 关闭方、ctx 取消语义、信任边界 —— 违反不报编译错、只在生产炸。

GUARDRAILS 里那条 native worker 的规则是这条原则最好的实例：

> **Never `terminate()` a worker that may be inside a native call** —— 在 N-API 调用中杀 worker 线程会 abort 整个进程（`Napi::Error` → `std::terminate` → SIGABRT）。**This bites hardest on the path you cannot test locally, because the abort only reproduces once the native module actually loads.**

### 3.3 一处分歧：共享可变累加器

> **Single graph accumulator** — all phases mutate the same `KnowledgeGraph` in `ctx`; the graph is the primary output.

19 个阶段共同改一张可变的图。这在 scope 里对应的是 **Blackboard 模式**，而 scope 的 agent 架构明确拒绝了它：

> Framework 不增加通用 Worker、Task、Result、Team、Supervisor 或**共享 Blackboard** 类型。
> attempt history、Score、Feedback、阈值范围和最终 report 都是 consumer-owned typed schema，不进入 Workflow/Kernel，**也不借共享 Blackboard 或 runtime type 查询传播**。

**但这两个拒绝不冲突**：GitNexus 的 phase 是**单进程、顺序执行、无恢复语义**的批处理管线，共享累加器是合理的（等价于一个 fold）。scope 拒绝 Blackboard 是在**并发、可恢复、有预算划拨的 Process 树**语境下 —— 那里共享可变状态会摧毁快照一致性和父子隔离。

**判据是"有没有恢复与并发语义"**，不是"共享可变状态本身好不好"。scope 的 `etl` 管线（顺序、无恢复）用同样的形态是完全正确的。

---

## 4. 工程治理：`GUARDRAILS.md` 的 "Signs" 格式 ⭐

GitNexus 的治理文档有一个 scope 没有的体裁：

```
### <失败模式的名字>
- **Trigger:**     什么现象说明你撞上了它
- **Do:**          具体做什么
- **Why:**         根因是什么

> Append new Signs when the same mistake repeats.
```

实测收录的 Signs：`Stale graph after edits`、`Index seems corrupt`、`Embeddings vanished after analyze`、`Analyze finishes but embeddings are incomplete`、`Analyze reports INCOMPLETE with a collapsed graph write`。

**每一条都是一次真实事故的结晶**，且带着 issue 编号（`#2432`、`#2623`、`#2790`、`#2841`、`#1939`）。

### 与 scope 的对照

scope 的治理文档体系是**三层 + 所有权分离**：

| 文档 | 回答 |
|---|---|
| `CLAUDE.md` | 能不能这么写（红线 / 反向不变量） |
| `DESIGN_PHILOSOPHY.md` | 该不该这么设计（为什么） |
| `REFACTORING.md` | 重构时改什么、怎么改 |
| `agent/doc/DECISIONS.md` | ADR，追加不改写 |
| `agent/doc/API_BASELINE.md` | 公开面基线，机器守卫 |

scope 的 `agent/CLAUDE.md` 甚至明写文档所有权规则：

> The documents have distinct owners. Do not copy progress into architecture, copy architecture into the execution log, copy architecture into the capability ledger, or rewrite accepted decisions without a superseding ADR.

**scope 的体系在"设计意图"这一侧远比 GitNexus 完备。** 但 GitNexus 有一个 scope 缺的东西：**运行期失败模式的可检索索引**。

scope 的反向不变量（❌ 列表）记的是"别这么设计"，`docs/` 下的三份审计报告记的是"曾经发现过什么"。**没有一个地方记"当你看到 X 现象时，根因通常是 Y"。**

这个缺口在 scope 的语境下真实存在：85 个 module、Engine 的 prepare/finalize 边界、TreeSnapshot 的 quiescent cut、Effect 的 replay contract —— 这些地方的故障现象和根因之间的距离很远，且大概率会重复踩。

**建议**：`docs/` 下增加一份 `SIGNS.md`（或并入 `REFACTORING.md`），用 Trigger → Do → Why 三段式记录反复出现的运行期失败模式。判据与 GitNexus 一致：**同一个错误重复出现时才追加**，不预写。这符合 YAGNI（不为不存在的失败预写条目），也符合「已发生过多次的扩展是预见、不是推测」。

---

## 5. 跨仓库：Contract Registry

值得单独一提的能力：`src/core/group/`。

多个 repo 组成 group，`group_sync` 构建 **Contract Registry**（`contracts.json`）+ bridge graph。跨仓库的 `impact` 和 `trace` 通过 `ContractLink`（HTTP consumer → provider，按 `Contract.symbolUid` 连接）跨越边界。

设计上的诚实之处：

> The crossing is **clamped to one boundary** (`MAX_SUPPORTED_CROSS_DEPTH`); deeper `crossDepth` is reported via `notes[]`.
> Two stores meet only at the `symbolUid` grain — the per-repo PDG/call graph and the group bridge — so **this is the documented join**; full cross-program (SDG-like) data flow across the boundary **remains deferred**.

**把能力边界写进文档，把未实现的部分明确标为 deferred 并给出 plan 链接。** 这与 scope 的 ADR + 「不 commit placeholder / 已知债务 / 部分语义」是同一态度。

对 scope 的启发有限（scope 的 `a2a` 已经是跨 Agent 的协议边界），但它对**"跨边界分析必须显式声明 join 的粒度和深度上限"**这条工程纪律的表述值得记住。

---

## 6. 结论

### 同构

| 取舍 | GitNexus | scope |
|---|---|---|
| 静态拓扑、构造期校验、无插件 | ✅ 19-phase DAG + Kahn | ✅ Workflow 封闭 Stage 词汇；package DAG gate |
| 依赖必须显式声明，机制强制 | ✅ runner 过滤 deps map | ✅ architecture test 锁 import 边 |
| 显式所有权与生命周期写进文档 | ✅ BindingAccumulator / worker terminate | ✅ goroutine/锁/ctx 所有权红线 |
| 部分失败不伪装成完整结果 | ✅ 零行或全集 | ✅ 分支失败则检索失败 |
| 输出超限截断 + 标记，不报错 | ✅ | ✅ |
| 能力衰减（只读模式 / CapabilitySet） | ✅ | ✅ |
| 一个后端、多个前端（MCP/HTTP/CLI） | ✅ | ✅ `tool.Tool` 一套身份，MCP/A2A 薄适配 |
| 未实现部分显式标 deferred | ✅ | ✅ ADR |

### 分歧（语境不同，非对错）

1. **共享可变累加器（Blackboard）** —— 在无恢复的顺序批处理里正确；scope 在有恢复/并发/预算的 Process 树里拒绝它。判据是语境不是形态。
2. **单一领域 schema（代码）** —— GitNexus 因此能做陈旧性检测、图查询、PDG。scope 的 `vectorstore`/`document` 是通用契约，付不起这个特化。

### 真实缺口（scope 应认真对待）

| # | 能力 | 说明 | 建议 |
|---|---|---|---|
| 1 | **"我不知道" ≠ "没有" 的统一原则** | 空结果必须能与"未解析/不可知"区分。scope 已有四处分身，缺统一判词 | **建议收录进 `DESIGN_PHILOSOPHY.md`**。这是所有喂 LLM 的能力（tool 结果 / retrieval / 探测）的共同不变量，成本为零，收益是把四条散落条款收成一条可推理的原则 |
| 2 | **`SIGNS.md`：运行期失败模式索引（Trigger → Do → Why）** | scope 有"别这么设计"和"曾发现什么"，缺"看到 X 现象根因通常是 Y" | **建议新增**。只在同一错误重复出现时追加，不预写。Engine 的 prepare/finalize、TreeSnapshot quiescent cut、Effect replay contract 是最可能需要它的地方 |
| 3 | **不完整性的分级建模 + 差异化退出码** | `incremental-in-progress`（留活给下次，OK）vs `graph-write-collapsed`（答案是错的，必须失败） | scope 的 `vectorstore`/`etl` 目前只有成功/失败两态。**中优先级**，需要真实故障驱动 |
| 4 | **索引陈旧性检测** | 索引与源不同步时主动暴露 | **不下沉到 `core/vectorstore`**（只有部分消费方需要，违反 §2.5）。正确落点是 `etl` reader 在 metadata 里记源版本/哈希，陈旧判定归调用方 |
| 5 | **图谱检索作为 `rag.Retriever` 实现** | 结构化检索能无损塞进 scope 现有窄腰 | 这不是缺口，是**窄腰选对了的证明**。无需动作 |

### 判词

> **GitNexus 不与 scope 竞争，它验证 scope。**
> 它是一个完全独立演化的、非 Go 的、单一领域的系统，却在管线拓扑、依赖显式化、所有权纪律、部分失败原子性、能力衰减这五处与 scope 得出了同样的答案 —— 这类跨语言跨领域的收敛，比任何同语言框架的相似都更有说服力。
> 它领先 scope 的地方只有一处，但很重要：**它把"喂给 LLM 的数据不能说谎"当成了第一原则，并为此建立了不完整性的分级模型和失败模式索引。**
> `A missing route is a coverage limit; an invented one is a lie.` —— 这句话应该进 scope 的设计哲学。
