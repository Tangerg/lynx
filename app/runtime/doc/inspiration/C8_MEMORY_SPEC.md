# C8 · 跨会话记忆 —— 落地 Spec（最大并集，参考 Hermes 实现）

> **状态**：设计已钉死（用户确认：全面对齐顶尖 / 混合 FTS5+向量 / 审阅队列 HITL / 最大并集注入）。本文是执行锚——4 批结构性重写，各批一个绿灯独立 commit。分析来源见 [`HERMES.md`](HERMES.md)（Focus 1）、[`README.md`](README.md)（T4）。
>
> **一句话**：lyra 今天**已在跑** Hermes 式"有界+自动 curate+整块注入"的 agent 记忆（不是纸面、不是休眠），但缺顶尖记忆的三件套——**检索式召回、用户可见/可控(HITL)、user 全局域**——再加 Hermes 几处高含金量不变量（冻结快照、load 重扫、provenance、session-log 搜索）。C8 = 把这套从"能用"抬到"顶尖"，**不重造 miner**。

---

## 0. 现状（已建成 70% 且 LIVE，务必先认清，勿重造）

turn 末 `Extractor.MaybeExtract`（`internal/adapter/maintenance/extraction.go`）恒跑：
```
读 post-compaction history → LLM askForFacts → append 日 ledger(append-only, 无界)
  → watermark 到点(curationDue: minPending/maxPending/maxAge) → LLM askForCuration 整块 re-curate(2048 tok 硬顶)
  → CAS PublishCuratedMemory → 每轮 composePrompt 整块注入 "## Agent-curated project memory"
```
- 存储：`agent_memory_ledger` + `agent_memory_curated` 两表（`sqlite/db.go:349-368`），`AgentMemoryStore`（`sqlite/agent_memory.go`：AppendLedger/PendingLedger/CuratedMemory/PublishCuratedMemory 四个 op）。
- 接线：`persistence.go:87` 恒构造 → `runtime_config.go:30,32` 同喂 Engine(`CuratedMemory`)+Extractor(`AgentMemoryStore`) → `maintenance_wiring.go:42` gate 恒真。**LIVE。**
- 注入：`agentexec/prompt.go:94-104` `composePrompt` 每轮读 curated blob 整块拼进 system prompt。
- **可见面：零**。`internal/delivery/` 无任何 `AgentMemory`/`curated` 引用；`memory.*` RPC(list/get/update) 只服务人写的 LYRA.md（`knowledge.Store`）。用户看不见/改不了/删不掉 agent 写的记忆。
- scope：**project-only**（`filepath.Clean(cwd)` 键）。
- 人写记忆另有一套：LYRA.md cascade（user/project，文件、`knowledge_store.go`、agent 永不写）—— **不动**。

## 1. 缺口（vs 顶尖：Hermes + ChatGPT/Cursor + Claude Code）

| # | 缺口 | 顶尖怎么做 | 落到哪批 |
|---|---|---|---|
| G1 | 检索式召回 | 无界语料 + top-k 相关性；注入有界 | B2 |
| G2 | 个体化 + HITL 可控 | 一条条可寻址 item，用户看/删/纠/固定 | B1+B3 |
| G3 | user 全局域 | 跨项目"这个人怎么工作" | B4 |
| G4 | **frozen-at-session-start 快照** | 写立即落盘、下次 session 才进 prompt → prefix-cache 稳 | B1 |
| G5 | **load 时重扫→`[BLOCKED]`占位** | 记忆是持久注入面，进 prompt 前重扫中和投毒 | B1 |
| G6 | **session-log 搜索工具** | FTS5 查过往会话 transcript（非 curated 记忆） | B2 |
| G7 | provenance 一等 | origin(auto/user)、session、day 可溯源 | B1 |

## 2. 目标架构（最大并集，三层 + 硬化）

- **L1 · 常驻核心**（Hermes Tier-1）：有界、agent-owned、**item 化**、**session-start 冻结注入**(G4：写落盘即时、进 prompt 延到下 session；mid-session 快照不变)、**load 时重扫→占位**(G5)、整块带（有界故本层不检索）。承载"永远该在场"的核心（用户偏好、build/test 命令、关键决策）。
- **L1.5 · 每轮轻量检索**（ChatGPT/Cursor，最大并集才有）：以当前 user 消息为 query，从无界语料取 top-k 高相关 item，注入 `## Relevant memories`。**注意与 G4 的张力**：L1.5 每轮变，只作**补充层**、与 L1 分开标注；prefix-cache 影响限于该小块（可只在首轮/compaction 边界刷新以缓解——实现时定）。
- **L2 · 按需搜索工具**（Hermes Tier-2 泛化）：**混合 FTS5+向量**后端，over(a)记忆 item +(b)过往会话 transcript，暴露 `memory_search` / `session_search` 两工具。无界、查询前零 token、不碰 prefix cache。
- **捕获**：cadence 自动挖掘（保留/升级现有 Extractor）→ **提议**（不直写）→ **审阅队列 HITL** → 批准才进 L1/语料。**provenance 一等**(G7)。
- **可见面**：`memory.agent.*` RPC（list/get/approve/reject/delete/pin/edit）+ 桌面 Memory 视图（复用 B4 Skill Library 先例）+ pending 队列 UI。
- **scope**：project + **user 全局**(G3)。
- **consolidate 岔路（③）已消解**：因走审阅队列（agent 提议、非直写），Hermes 的"当轮强制 consolidate + 3 次熔断"不再需要；lyra 现有后台有界 re-curate 留作 ledger→item 折叠器。

## 3. 召回机制（混合 FTS5+向量）

- **FTS5 恒开**：modernc.org/sqlite v1.52.0 已编译 `-DSQLITE_ENABLE_FTS5`（全仓首次用）。`CREATE VIRTUAL TABLE … USING fts5(…)` + `MATCH`。关键词层不依赖 embedding、Hermes 证明够用。
- **向量叠加（配了 embedding 才生效）**：复用 `modelclient.EmbeddingResolver.Resolve(ctx, provider, model) (codebaseindex.Embedder, error)` + `Embedder.Embed(ctx, []string) ([][]float32, error)`。cosine/topK 数学（`domain/codebaseindex/scoring.go` 现为 unexported、耦合 Chunk/Hit）**提取成共享 `vectorsearch` helper**（泛化到 `(query []float32, vectors [][]float32) → scored idx`），并把 `sqlite` 里 unexported 的 `encodeVec`/`decodeVec`（float32-LE BLOB）提升复用。
- 混合排序：FTS5 候选 ∪ 向量候选 → 归一化融合（RRF 或加权，实现时定）。无 embedding 时纯 FTS5 优雅降级。

## 4. 批次（各批绿灯独立 commit + push）

**B1 · 域 item 化 + provenance（✅ SHIPPED）**
- 新 `domain/agentmemory` 包（SRP：`knowledge` 只留人写 LYRA.md 的 Scope/Entry/Store；agent 记忆 + ledger 类型迁出）。`Item{ID, Scope(project|user), Content, Origin(auto|user), Pinned, SessionID, Day, Created/Updated}` + `Store` 端口 + `Render`（pinned 先、token 预算）+ `EstimateTokens`。
- 重塑 `AgentMemoryStore`：`agent_memory_curated` 单块表 → `agent_memory_items`（离散 item + digest 身份 + provenance）+ `agent_memory_state`（per-project watermark，CAS）；`agent_memory_ledger` 保留。schema 11→12（弃旧 agent_memory 数据）。`Reconcile` 按 digest 保 id/剪枝、pinned/user-origin 不动。
- miner：curation prompt 改扁平 standalone bullets；`maybeCurate` 读 State → askForCuration → `NormalizeFacts` → `Reconcile`。
- 注入：`composePrompt` 读 `Items(ScopeProject)` → `Render`（预算 4096）→ `## Agent-curated project memory`。`CuratedMemoryReader`→`AgentMemoryReader`（method `Items`），`Config.CuratedMemory`→`AgentMemory`。
- **挪批**：G4 冻结快照 → **B4**（需 per-session 快照缓存 + 生命周期淘汰，独立于 item 化；prefix-cache 收益被 todos 已动态注入部分抵消）；G5 load 重扫占位 → **B3**（需威胁扫描器，随 HITL 安全面一起，避免在 B1 造弱扫描器）。

**B2 · 混合检索后端 + memory_search 工具（✅ SHIPPED）**
- `agentmemory.Searcher`（`search.go`）：**in-Go 关键词打分 + 向量 cosine，RRF 融合**。**决策**：item 层用 in-Go 关键词而非 FTS5——item 语料小（curation 有界），FTS5 会引入 shadow-table + 触发器 + 改 discardSchema 的风险（违 KISS）；FTS5 留给 session_search 那种大语料。cosine/topK 按 DRY-3 在 memory 侧写小份（未动 codebase）。
- item 加 `embedding BLOB` 列（复用同包 `encodeVec/decodeVec`）；store 加 `ItemsForSearch`/`UnembeddedItems`/`SetEmbeddings`。schema 12→13。
- 向量**写时嵌入 + 回填**：extractor reconcile 后 `embedNewItems`（best-effort，复用 `EmbeddingResolver`，`codebaseindex.Embedder`→`agentmemory.Embedder` 桥接；无 embedder 则纯关键词）。也回填 embedding role 后配的旧 item。
- `memory_search` 工具（`adapter/toolset/memorysearch`，照 codebasesearch 先例，两角色恒在——关键词不依赖 embedding）。全链路 wiring：`toolEnvironmentBuilder` 签名 + `BuildConfig.MemorySearch` + resolver 注册 + bootstrap 建 Searcher + 桥 embedder 进 extractor。
- **挪批**：L1.5 每轮检索注入 → **B3**（与 pinning 一起，pinned=always-on L1 / 非 pinned=检索,分区才无重复注入/无 always-on 回归）；**session_search**（会话 transcript FTS5，大语料）→ 单列后续。

**B3 · 捕获 + HITL 审阅队列 + RPC（✅ SHIPPED，后端+RPC，前端 defer）**
- item 加 `Status`（active/pending/rejected 墓碑）+ schema 13→14。**捕获改 pending 默认**：`reconcileItems` 折叠成 **pending 提议**（不再 auto-active），且**尊重墓碑**（rejected digest 挡重提）+ **active 粘性**（approve 后不被下轮 curation 删）。注入/检索/嵌入（`Items`/`ItemsForSearch`/`UnembeddedItems`）全过滤 `status='active'`——未审不入 prompt（用户选的 HITL 语义）。
- store 管理方法（concrete，经 consumer-side 窄接口用）：`List`/`Get`/`SetStatus`/`SetPinned`/`UpdateContent`(清陈旧 embedding)/`Delete`/`Add`。domain 加 `Management` 端口 + `ErrNotFound`。
- `agentMemory.*` RPC（list/review/update/delete/add），全链路照 goal.go（method_names/method_table/handlers_agentmemory → server/agentmemory.go → protocol/agentmemory.go → runtime.go；disabled→capability_not_negotiated）。server 依赖 domain `agentmemory.Management`（不碰 infra）。pinning 经 update.pinned；Render 已 pinned 优先，**L1 即生效**。
- 桌面 Memory 视图 **defer**（用户指定"只到 API 暴露"）。
- **挪批**：G5 load 重扫占位 → **B4**（威胁扫描器随安全收尾）；**L1.5 每轮检索注入 → B4**（pinned=always-on L1 / 非 pinned 检索）。

**B4 · user 全局域 + 冻结快照 + 可观测 + 测试 + 安全收尾**
- scope 扩 user 全局（跨项目）。
- **G4（从 B1 挪入）冻结快照**：per-session 注入快照（写落盘即时、下 session 才进 prompt，prefix-cache 稳），带生命周期淘汰。
- OTel：extract/curate/recall/search span + 计数（照 `application/goals/observe.go` 先例）。
- 单测 + race + 12 模块 build 全绿。

## 5. 复用地图（勿重造）

| 要用的 | 在哪 |
|---|---|
| embedding 栈（整体复用） | `modelclient.EmbeddingResolver` + `EmbeddingRoleStore` + `modelrole.Role` + `infra/llm/BuildEmbeddingModel` |
| cosine/topK 数学（提取共享） | `domain/codebaseindex/scoring.go`（unexported，泛化） |
| float32 BLOB 编解码（提升） | `sqlite/codebaseindex.go` `encodeVec/decodeVec`（unexported） |
| 冻结/注入端口先例 | `agentexec/config.go:18` `CuratedMemoryReader` + `prompt.go:94-104` |
| 确定性注入先例（`<system-reminder>`） | `maintenance/compaction_livestate.go:40` `liveStateReminder` + `compaction.go:180-191` splice |
| store 模板 | `sqlite/goal.go`（conn/RunInTx/scanRow/`_ = (*T)(nil)`） |
| store 全链路接线 | persistence.go → runtime_config_types.go → runtime_config.go → toolenv.go/assemble.go → serve.go |
| RPC 全链路 | goal.go 那条（§B3） |
| 工具一包 | `adapter/toolset/codebasesearch`、`goaltool` 先例 |
| HITL 管理面前端 | B4 Skill Library 视图（`app/desktop`） |
| 可观测先例 | `application/goals/observe.go` |
| schemaVersion | 现 **11** → bump **12**（无迁移，mismatch 弃表重建） |

## 6. 刻意不吸（Hermes，防重新论证）

- **两文件切分**（MEMORY.md + USER.md）—— 对单一记忆域是双机制债；user 画像作 user-scope item。
- **8 个外部 memory provider / Honcho 云** —— 多厂商蔓延 + 云，off-strategy。
- **自由写默认**（write_approval=false）—— 违 HITL 不变量；lyra 走审阅队列强制门。
- **weekly LLM umbrella-consolidation** —— aux 成本，Hermes 自己默认 off，单用户 YAGNI。
- **semver version 字段** —— theater，无历史。
- **external-drift `.bak` guard** —— Hermes 因文件后端才需；lyra agent 记忆是 SQLite（CAS/watermark 已够）。

## 7. 反向不变量校验

- 无 retry/Transient 分类（召回失败优雅降级，不重试层）。
- 无隐藏后台 dream daemon（curate 是 post-turn maintenance，非独立周期 LLM）。
- 单 SQLite 唯一真源（记忆 item + FTS5 + 向量全在 lyra.db；人写 LYRA.md 是唯一文件例外，不动）。
- 破坏性 schema/公开 API 改动已获用户确认（本 spec 即 scope+影响+备选的落定）。
