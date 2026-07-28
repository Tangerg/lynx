# Lyra Runtime vNext —— 实施计划与进度台账

> **这份文档的职责**：把 [`codex_runtime_protocol_vnext_final.md`](codex_runtime_protocol_vnext_final.md)（冻结契约）翻译成可执行、可复查、跨 session 不跑偏的实施台账。
>
> 契约回答「vNext 要长什么样」；**本文回答「怎么做到、做到哪了、为什么这样切」**。
>
> - 契约是唯一目标基线。本文与契约冲突时，**改本文**。
> - 本文与代码冲突时，**改代码**（契约 §0.8：当前代码与旧文档的差异都是待修正项，不构成弱化目标的理由）。
> - 审计事实标注日期。带行号的引用是**当时**的证据，行号会漂移，语义不会。

**审计基线日期**：2026-07-28
**审计时 `main` 尾 commit**：`50942c0e9`
**目标 `protocolVersion`**：`2026-07-27`（当前 `2026-07-19`）
**目标 `SessionArtifactVersion`**：`7`（当前 `6`）

---

## 执行法则（不编号 —— 凌驾于下文所有安排）

> **每个 slice 开工前过一遍，收工前再过一遍。** 与下文任何 slice 安排冲突时，**改 slice**。

### 法则一 —— 治本，不补丁

根仓 `CLAUDE.md` 第二法则在本工程的落法：改在**根因所在的那一层**，不在症状点绕。

**判据**：「根因消除了吗，还是只是这个现象不出现了？」

本工程里它最常伪装成：

- 在 presenter 补一个 domain 没给的值 —— 现象消失了，「谁该给这个值」没解决（§4.2 泄露 1）。
- 加一条 fallback / coerce / 默认值去吸收上游的空状态 —— 空状态还在，只是不可见了。
- 契约要求删的字段先留着「以后再删」—— `protocolVersion` 一次性切换，留 = 第二次 breaking bump（§2.1）。

### 法则二 —— 抽象要合理：不过度，也不不足

两个方向都是缺陷，**"不足"更常被当成"简单"而放过**。

| | 症状 | 判据 |
|---|---|---|
| **不足** | 一个职责散在 N 处、每处各写一遍（查询语义散在 presenter；同一事实两个作者） | 「改这条规则要动几个文件？」—— > 1 且它们没有独立演化的理由 → 抽象不足 |
| **过度** | 为单一实现抽接口、为一个概念新造一层、为不存在的需求留 hook | 「现在有几个真实实现 / 真实消费者？删掉这层谁会疼？」—— 1 个 / 没人疼 → 过度 |

**本工程的具体标定**（防止两个方向各自跑偏）：

- ✅ application query port（B′4）**是必要抽象**：delivery 与 store 之间是真实替换点，且过滤 / 排序 / 分页三件事今天确实散着。
- ✅ Contract Registry（B）**是必要抽象**：它消除的是「同一份 method/union 事实抄在 N 处」；14 类产物是那**一份**事实的投影，不是 14 个新概念。
- ❌ 不为 vNext 的每个新 wire 概念在 domain 造一个对应类型 —— wire 概念只活在 delivery，domain 只有它自己的领域概念（§4.3 已逐行裁决归属）。
- ❌ 不为「以后可能有第二个 transport / 第二个 store」留接口 —— YAGNI，真要时再定义。

### 法则三 —— 抽象不得泄露

`delivery` 只做 wire。完整法则见 **§4.1**，三个现役泄露见 **§4.2**，vNext 新概念的归属裁决表见 **§4.3**。

**判据**：「删掉这段 delivery 代码，是否有业务事实随之消失？」

---

## 0. 状态总览

| 批次 | 名称 | 状态 | 说明 |
|---|---|---|---|
| A | 协议文档事实对齐 | `TODO` | 零代码变化 |
| A′ | 现役泄露治本（**前置**） | `IN PROGRESS` | 5 slice（A′5 实施中发现），落 main |
| B | Contract Registry（全量） | `TODO` | 4 slice，可与 B′ 并行 |
| B′ | 权威读面 + store cutover | `TODO` | 5 slice，落 main，**建议起点** |
| C | vNext 原子切换 | `TODO` | 16 stacked commit，一条 cutover 分支 |
| D | 切换后数值调优 | `TODO` | 依赖 C |

**状态取值**：`TODO` / `IN PROGRESS` / `DONE` / `DEFERRED` / `N/A（附理由）`

---

## 1. 目标

### 1.1 一句话

把 app/runtime 与 app/desktop 从当前协议一次性切到 vNext（`2026-07-27`），并在此过程中把 `delivery` 环收回成纯 wire 层。

### 1.2 vNext 的窄腰（契约 §16）

```text
typed commands
  → Session / Run / Segment / Item
  → authoritative projection + scoped live stream
  → replay when possible, cold recovery always possible
```

三类流量各有唯一职责：

| 流量 | 机制 | 正确性来源 |
|---|---|---|
| 当前 Run 树细节 | root-segment-scoped tree stream | Item/Run/Interrupt/state projection |
| 非当前 Run 失效 | 单条 filtered runtime stream | 对应 domain query |
| 历史恢复 | query | SQLite durable projection |

### 1.3 本次工程的两个并列目标

1. **API 重塑**：方法面、wire shape、durable read model、Registry 与生成物、Artifact v7 —— 见 §3。
2. **delivery 环收回纯 wire**：消除现役泄露 + 预埋守卫，防 vNext 的新概念在 presenter 里长出业务逻辑 —— 见 §4。

二者不是"顺带"关系：§4 的三个现役泄露正落在 §3 要重建的读面上，**先修再建**。

---

## 2. 已确定的范围决策

| # | 决策 | 日期 | 内容 |
|---|---|---|---|
| D1 | **Registry 全量治本级** | 2026-07-28 | 契约 §11 全量：14 类产物（含 **TypeScript wire types + typed client stubs**）+ 18 项 CI drift gate。**推翻**记忆 `project_lyra_no_protocol_ts_codegen`（"codegen 拒"）—— 当初反对理由是"Go flat-struct 不映 TS union"，§11.2 的显式 `UnionSpec` metadata 正是该理由的解：不靠 reflection 猜 discriminator，而是显式登记。 |
| D2 | **子 Run / subagents 本轮不做** | 2026-07-28 | **shape 全量落地，行为用 capability 关掉** —— 见 §2.1。 |
| D3 | **`SystemInvariantSpec` 归 application** | 2026-07-28 | 见 §2.2。 |
| D4 | **A′ 前置于 B / B′** | 2026-07-28 | 不在被泄露污染的读面上重建。 |
| D5 | **`transcript.Problem.Scope` 留在 domain** | 2026-07-28 | 它在做真实领域校验；只是停止上 wire。见 §5.1。 |
| D6 | **`transcript.Problem.Retryable` 三处全删** | 2026-07-28 | 冗余中间量。见 §5.2。 |

### 2.1 D2 的精确边界

「子 Run 先不做」**不等于**「从契约里删掉子 Run」。契约冻结、`protocolVersion` 一次性切换，所以先删字段以后再加 = **第二次 breaking bump**，违反第一法则。

> **shape 全量落地（闭合联合完整、validator 双 variant 齐全），只把「产生行为」用 capability 关掉。**

这不是为兼容留字段 —— 契约 §8.1/§8.3 的 capability 机制本来就是为此设计的。

| 项 | 本轮 shape | 本轮行为 |
|---|---|---|
| `parentRunId` / `rootRunId` / `spawnedByItemId` | 入 schema，root/child 双 variant validator 齐 | 永不产生 child 行 |
| `SegmentOutcome{type:"suspended"}` | 入闭合联合 + reducer 分支 | 永不发布 |
| `CancelRunResponse{type:"child"}` | 入闭合联合 + validator | cancel 仍 root-only，只返 `root` variant |
| `RunProtocolProfile` | durable、不可变、root-only projection | `requiredFeatures` 恒为 `[]` |
| `features.subagents` | 入 Registry，`{stable, clientOptIn:true, requiredByRunProtocol:true}` | 广告 `enabled:false` |
| `includeDescendants:true` / `items.list` child scope / `runs.get(child)` | 入请求 shape | 一律 `capability_not_negotiated`（**不静默降级**，契约 §8.1 硬要求） |
| `child_run_canceled` | 入 Error Registry | 永不发出 |

**本轮因此省掉的真正难题**：tree interrupt barrier、quiescence 收敛、child-before-parent postorder 发布、child cancel 的父 tool-call 结构化错误注入、`Waiting→Running` 的第二触发源。

**连带结论：本轮不需要动 `agent` 模块。**（原先标为风险的执行层依赖被 D2 消掉。）

**支撑事实（2026-07-28 核实）**：`Item.RunID` 是必填字段（`delivery/protocol/items.go`），task 工具跑出的子 agent process 其 Item 仍归属 root Run，所以 `Interrupt.runId` = root id，契约的 `interrupt_source_matches_item` 平凡成立。子 agent 执行行为**保持今天原样**，不需要禁用 task 工具。

### 2.2 D3 的推导（含一次自我纠正）

**被否决的方案**（曾提出，属补丁式）：`SystemInvariantSpec` 条目放 `delivery`（"纯清单"）、fixture 放 application。

否决理由有两条，第二条是硬错误：

1. 它让**生成器的需要**去决定分层 —— 便利驱动。
2. 类型若定义在 delivery，application 注册就要 import delivery = **向外依赖**，直接违反 `TestDependencyRule`。

**治本答案**：

```text
SystemInvariantSpec 类型 + 注册  → application（与它描述的事务边界同处）
生成器                          → 环外 build-time 工具（cmd/ 或 internal/contract/gen）
                                  同时读 delivery 的 method/union spec
                                  与 application 的 invariant spec，合成 manifest
delivery                        → 永不见到它
```

**关键洞察**：分层规则约束的是**运行时 import 图**；代码生成器不在那张图里。

---

## 3. API 重塑清单（核实过的现状 → 目标）

### 3.1 方法面 delta

当前 65 个方法。核心面差异：

| 动作 | 方法 |
|---|---|
| **新增 4** | `runtime.subscribe`、`runs.get`、`interrupts.list`、`todos.get` |
| **删除 2** | `runs.listOpenInterrupts`、`workspace.subscribe` |
| **已齐 ✅** | 九个 `sessions.*` 全部存在（契约 §0.3 担心的缺口不存在） |

旁路域按契约保留自己的协议根，但**全部进同一个 Registry**：
`mcp.configs.* / mcp.servers.* / mcp.tools.*`、`skills.*`、`memory.*`、`agentMemory.*`、`agentDocs.*`、`goals.*`、`schedules.*`、`workspace.*`、`providers.*`、`models.*`、`codebase.*`、`approval.*`、`hooks.*`、`recipes.*`、`tools.*`、`usage.*`、`feedback.*`

**不提供 deprecated alias。**

### 3.2 wire shape delta

| 契约要求 | 现状（2026-07-28） | 性质 |
|---|---|---|
| `RunStatus = running\|waiting\|finished` | 只有 `running\|finished`；`presenter_run.go` 把 `Interrupted` 映射为 `finished` + `outcome:{type:"interrupt"}` | ✅ **domain 的 `RunState` 已有 `Interrupted`，五态机完整**。只是 wire 折叠了它 —— 加枚举值 + 停止折叠，**不是状态机重设计** |
| `RunOutcome` 5 终态 / `SegmentOutcome` = +`interrupt`+`suspended` | `RunOutcome` 混装 `interrupt` variant 和 `Result *RunResult` | 拆分 + 新增 `suspended` |
| 删除 `RunResult`（含 `durationMs`） | `RunResult{Usage,Steps,Error,DurationMs}` 存在 | 删，`metrics` 取代 |
| `RunSummary` base + `RunRef extends` | 只有 `RunRef`，**缺 4 字段**：`activeSegmentId`、`limits`、`protocolProfile`、`metrics` | 分层 + 新增；**必须由同一嵌入定义生成 Go/TS/schema，禁止手抄两套同名字段** |
| `Interrupt.runId` 必填 | `{itemId, type, payload}` —— 无 runId | 新增必填字段 |
| `PendingInterruptSet.rootRunId` | `OpenInterrupt{runId,…}` | 改名 + 语义收紧为 root |
| `Interrupt` payload 三 typed variant | `InterruptPayload` 是扁平 struct | 拆 union |
| `ListItemsRequest{scope: ItemListScope, order?}` | `{sessionId, PageQuery}` 扁平 | 换请求 shape |
| `ListItemsResponse.runs: RunSummary[]` | `[]RunRef` | 换轻量类型 |
| `ProblemData` 删 `channel`/`retryable`，加 `activeRun`/`requiredCapabilities` | `Channel` + `Retryable` 都在 | 删 2 加 2 |
| `ServerCapabilities` | `events`（→`runEvents`）；**缺** `runtimeTopics`、`stateSnapshots`；`FeatureCapability` 缺 `clientOptIn`/`requiredByRunProtocol`；`limits` 缺 `runReplay`/`runtimeSubscription` | 改名 + 4 处新增 |
| `RuntimeEvent` 九值闭合 + `resync` | `WorkspaceEvent` 只 5 值：`files.changed`、`skills.changed`、`mcp.serverChanged`、`schedules.fired`、`resync` | **新增 5 topic**（`sessions/runs/state/goals/interrupts.changed`）+ 2 处改名并**删 payload 真相**（`mcp.serverChanged.status/toolCount/error`） |
| Artifact v7 | v6 | bump + shape 同步（删 `ArtifactRunResult`/`durationMs`/`retryable`，加 `states: ArtifactState[]`） |
| `protocolVersion = minSupported = 2026-07-27` | 均为 `2026-07-19` | 一次切换 |

### 3.3 durable read model delta

```sql
-- 现状（2026-07-28）
history_runs (run_id, session_id, updated_at, payload, message_mark)   ← 契约要求删表
runs         (run_id, session_id, state, provider, model, outcome,
              started_at, updated_at)                                  ← 仅 8 列
interrupts   (run_id PRIMARY KEY, session_id, …)                       ← 一 run 一行
todos        (session_id PRIMARY KEY, items, updated_at)               ← 缺 revision
```

**`runs` 需补约 12 列**：`spawned_by_item_id / parent_run_id / root_run_id / active_segment_id / max_steps / max_budget_usd / protocol_profile / steps / usage / active_duration_ms / finished_at / message_mark`

**一个已经对的地方**：`idx_runs_session_active ON runs(session_id) WHERE state != 'terminal'` 就是「一 Session 最多一个非终态 root」的 admission gate，已存在且正确。⚠️ **本轮 child 行不产生，索引保持原样**；等子 Run 那轮再收窄成 root-only。

**`interrupts` 改造**：从「一 run 一行」改为 **root-owned aggregate（按 `root_run_id`）+ 每个 Interrupt 持久化直接来源 `run_id`**，支持按 `(createdAt ASC, rootRunId ASC)` 的 keyset 分页且**一个 set 不跨页**（契约 §3.1：cursor 不依赖 JSON 数组下标）。

**store cutover 纪律（契约 §13 Batch B′ / §14.7）**：不 migration、不回填、不双读双写、不 compatibility view。丢本地 dev 库重建 + schema epoch 守卫（遇旧 epoch **确定性拒绝启动并提示重建**，不后台静默删用户文件）。
> 与 `feedback_dev_drop_no_hedge`（drifted schema → rm + 重建）一致。

### 3.4 Registry 与生成物（D1 全量）

**现状核实**：`go:generate` 0 处、OpenRPC 0、JSON Schema 0、TS codegen 0。已有的只是 `delivery/protocol/wire_golden_test.go`（golden-sample 漂移 gate）。

契约 §11.3 的 14 类产物全部要有：
`OpenRPC` · `JSON Schema bundle` · `TypeScript wire types` · `authoritative/terminal runtime validators` · `method constants + typed client stubs` · `protocol manifest` · `error registry` · `capability gate policy` · `run event policy` · `state snapshot recovery policy` · `state scope/writer/lifecycle policy` · `runtime topic capability list` · `system invariant manifest` · `human-readable API reference`

**Canonical samples 保持人工编写与审阅**（§11.3）：Registry 只生成 sample 索引与缺口检查；**禁止生成 fixture 再用同源 schema 证明自己正确**。现有 `wire_golden_test.go` 的样本是起点，不是被替换物。

### 3.5 前端（app/desktop）delta

- **55 个文件**命中受影响面（`workspace.subscribe` / `listOpenInterrupts` / `durationMs` / `retryable` / `RunResult` / `runs.list` / `items.list` / `segment.finished`）
- **删除 business numeric code 镜像**：`frontend/src/rpc/types.ts` 有 `RPC_PROVIDER_ERROR = -32001`、`RPC_SESSION_NOT_FOUND = -32002`、`RPC_RUN_NOT_FOUND`、`RPC_ITEM_NOT_FOUND` … 契约 §9.2/§14.6 要求全删，**只留五个标准 JSON-RPC 常量**。后端 15 个 business code 常量对外只能由 Registry 生成
- ✅ **单流约束已结构性满足**：`plugins/builtin/workspace/events/index.ts` 自述 "the app's ONE workspace.subscribe consumer"，`rpc/transports/http.ts` 自述 "the one NON-run stream"。契约「每个 client process 至多一条 runtime stream」不需要架构改造 —— **C15 因此从架构改造降级为改名 + topic 扩容 + reducer 扩展**
- 新增 exhaustive reducer：`suspended` outcome、`state.changed`、九值 topic、`revision` 单调合并、`items.list` 双向 order
- goal 4 秒轮询删除（改 `goals.changed`）

---

## 4. delivery 反泄露

### 4.1 法则

> **delivery 只做三件事：wire shape、编解码、把 application/domain 的值一对一映射成 wire 值。**
>
> 它**不得**：补充缺失的业务事实、二次推导任何已在内层决定的事实、持有生命周期状态、实现查询语义（过滤/排序/分页）。

**判据一句话**：「删掉这段 delivery 代码，是否有业务事实随之消失？」—— 是，就是泄露；只有 JSON 形状变了，才是本职。

`delivery/protocol` 更严：`TestProtocolStaysWireOnly` 已禁它 import domain/application。契约 §11.2 的「`Validate()` 禁止查存储/dispatcher/executor」与它完全同向 —— **不用取舍**。

### 4.2 三个现役泄露（证据，2026-07-28）

#### 泄露 1 —— 同一事实两个作者，delivery 补另一半

```text
adapter/agentexec/turn/terminal.go   为 6 个 kind 写 Detail
                                     （AgentStuck / RateLimited / InvalidCredentials /
                                       Timeout / ProviderUnavailable / ProviderRejected）
delivery/server/presenter_run.go     为另外 4 个 kind 补 Detail
                                     （RunLost / DeniedByUser / ToolFailed / Internal）
                                     —— 因为它们不从 terminal.go 那条路来
```

delivery 侧还为 `OutcomeMaxBudget` / `OutcomeMaxSteps` 编了两句默认文案。

**实施时（2026-07-28）又发现两条更硬的证据**：

1. **前端本来就是对的，错的是后端。** `lib/rpcErrors.ts` 的文件头注释记录着**同一个 bug 的前科**：*"They were English string literals here … the app shipped fifteen sentences no translator could see."* 前端已按 type 建好 8 语言文案表，且 `MAPPED_TYPES` 恰好**缺**这四个 kind —— delivery 是在替它补，且只有英文。
2. **两个编码器不一致。** `artifact_encode.go` 直接用 `problem.Detail`（无 fallback），`presenter_run.go` 有 fallback —— **同一次失败，客户端从 live 流看到一句话、从 Artifact 看到空**。这不是"多写了一份文案"，是 wire 上的真相分叉。

**"这个 Run 为什么停了"是领域语义。** domain 没给 detail，delivery 就补一个 —— 不是映射，是填充缺失的业务事实。已有守卫 `TestDeliveryDoesNotDeriveSessionActivity` 的注释写的正是这条边界：*"Delivery maps the resulting enum but cannot duplicate the precedence rule"*。

**vNext 会放大它**：`RunOutcome.detail`（3 variant）、`ProblemData.detail`、`child_run_canceled` detail 都是同一模式。

**治本**：**一个作者。** 找到那 4 个 kind 各自的产生点，在那里一次写全；delivery 的 fallback 随之消失。若某个 kind 领域确实无话可说 → **wire 省略该字段**，而不是让 delivery 补。

#### 泄露 2 —— `RunStatus` 有两个真相源

```go
// delivery/server/presenter_run.go —— 从 durable transcript 推导
case execution.Interrupted, Completed, Failed, Canceled: status = RunStatusFinished

// delivery/server/runs_query.go —— 从 live registry 读，并硬编码
records := s.coordinator.List()
… Status: protocol.RunStatusRunning,
```

契约 §3.1 明文：*"`runs.list` 必须读 durable projection，不能只读 process-local live registry"*。

#### 泄露 3 —— 查询语义（过滤/排序/分页）在 delivery 实现

```go
// delivery/server/runs_query.go —— 业务过滤 + 内存分页
if in.SessionID != "" && r.SessionID != in.SessionID { continue }
page, next, err := pageByCursor(out, func(r protocol.RunRef) string { return r.ID }, …)

// delivery/server/items.go —— 最严重的一处
hItems, hRuns, err := s.queries.ListTranscript(ctx, in.SessionID)  // 加载整个会话时间线
return s.listItemsFromHistory(hItems, hRuns, in)                   // 再在内存里分页
```

vNext 要求 cursor 是 **server-issued opaque keyset token**，内含 `formatVersion + method + normalized scope/filters + last sort tuple` 并做完整性保护、可无状态解码；`interrupts.list` 的 cursor **不依赖 JSON 数组下标**。这不可能活在 presenter 里 —— 它是 query substrate。

`items.list` 那处尤其严重：**长会话每次调用都全量物化**。B′4 的 durable `sequence` keyset 分页不是优化，是修这个。

### 4.3 vNext 新增的泄露诱惑面 —— 先裁决归属再写代码

| 新概念 | 归属 | delivery 只做 | 做错会长成 |
|---|---|---|---|
| `RunProtocolProfile` 协商 | **application**（admission 事务内冻结） | 映射 durable profile | presenter 每次按 request capabilities 重算 —— 契约 §15 明文禁止 |
| `CapabilityRule` 门控 | **Registry（delivery 合法）**，但只能**拒绝** | 拒绝 + 生成 `requiredCapabilities` | delivery 按 capability 改变业务行为（静默降级）而非报错 |
| `RunMetrics` root/child 聚合 | **domain/application** | 映射数字 | presenter 做跨层加减 —— 契约 §4.5 明文禁止 |
| `activeSegmentId` 存在性 | **application**（与状态同事务） | 映射 | presenter 按 status 推断"要不要带这字段" |
| `PendingInterruptSet` aggregate 组装 | **application query** | 映射 set | presenter 遍历 interrupt 自己按 root 分组 |
| `revision` 单调比较 | **projection** | 原样映射整数 | presenter 比较/兜底 revision |
| cursor 编解码 | **application query port** | 传递 opaque token | `pageByCursor` 继续留在 delivery |
| `RunSummary` vs `RunRef` 分层 | **同一嵌入定义生成** | — | 手抄两套同名字段 |
| `SystemInvariantSpec` | **application**（D3） | 永不见到 | delivery 命名业务不变量 + 向外依赖 |
| `RecoveryAction` 闭合枚举 | **Registry（delivery 合法）** | 生成 exhaustive 分支 | —（这是协议 metadata，不是业务逻辑） |

### 4.4 要新增的 arch 守卫（把教训机器化）

现有 12 个 delivery 守卫都是过去泄露的化石。本轮该留下 6 个新的：

```go
TestDeliveryDoesNotAuthorDomainText        // ✅ DONE (4571e91e9)
TestDeliveryDoesNotImplementQuerySemantics // A′4 长出：禁 pageByCursor / 切片过滤 / 排序
TestDeliveryReadsRunsFromDurableProjection // A′3 长出：禁 delivery 调 coordinator.List() 取 Run 事实
TestSystemInvariantsStayInApplication      // D3 长出：禁 delivery 出现 SystemInvariantSpec
TestDeliveryDoesNotComputeMetrics          // C 预埋
TestRunWireShapesShareOneDefinition        // C 预埋
```

**A′1 的守卫比原计划强，理由值得记下**：原计划写的是「禁 default 文案字面量」，但泄露的实际形态是**文案藏在一个看起来像 mapper 的 helper 里**（`presentProblemDetail`），禁字面量赋值挡不住它。落地形态改为：*在 `presenter_run.go` / `artifact_encode.go` 里，字符串字面量只能是给程序员的诊断*（豁免 import path / `panic` / `errors.New` / `fmt.Errorf` / 空串）。已验证：把文案塞进新 helper 里也会被咬到。
> 教训：守卫要挡**形态**，不是挡**符号名**。换个函数名就绕过的守卫等于没有。

契约 §14.6 那句「validator 的依赖图不包含 store/dispatcher/executor」**已有等价守卫**，不用新建。

---

## 5. 已定案的技术判断（附证据，防重新论证）

### 5.1 `transcript.Problem.Scope` → 留在 domain，从 wire 删除

```go
// domain/execution/transcript/model.go —— 它在做真实的领域校验
switch problem.Scope { … return fmt.Errorf("unknown scope %d", …) }
if problem.Scope != scope { return fmt.Errorf("scope %d, want %d", …) }
```

它校验「run 级 problem 不能挂到 tool 级位置」—— **领域不变量，不是 wire 字段**。契约删 `ProblemData.channel` 的理由是「物理落点已经表达 channel」，说的是 wire，没说领域不能有这个不变量。

**⚠️ 不要顺手删** —— 那会误删一个真在用的领域校验。本条**不产生任何工作项**。

### 5.2 `transcript.Problem.Retryable` → domain 也删，是冗余中间量

```go
// adapter/agentexec/turn/terminal.go —— 只有 3 个 kind 置 true
RateLimited / Timeout / ProviderUnavailable  → Retryable = true
// 唯一消费点
if problem.Retryable && retryAfter > 0 { problem.RetryAfterSeconds = … }
```

**置 true 的那 3 个 kind，恰好就是该接受 retryAfter 的那 3 个 kind** —— 布尔量没携带任何 kind 之外的信息。

**治本**：按 kind 直接决定是否附 `retryAfterSeconds`，`domain` / `wire` / `Artifact` 三处 `Retryable` 全删。正是契约 §9.2「不建立通用暂时/永久分类」要的。

**第二个 Retryable 源**：`delivery/dispatch/errors.go` 的 `spec.retryable` 错误规格表 —— 一并删，归 Error Registry。

### 5.3 `items.list` 的 `sessionId` 可安全移除 ✅

```go
// delivery/server/items.go —— sessionId 只是查询键，零前置校验、零权限检查
hItems, hRuns, err := s.queries.ListTranscript(ctx, in.SessionID)
```

vNext 用 `ItemListScope` 闭合联合取代它是干净替换（run scope 内部解析 session）。**无隐藏依赖。**

### 5.4 `streamingMethods` 手维护 → 归 Registry 生成

```go
// delivery/server/server.go
StreamingMethods: []string{"runs.start", "runs.resume", "runs.subscribe", "workspace.subscribe"}
```

B1 之后由 Registry 的 `Stream[]` 注册自动生成（且 `workspace.subscribe` → `runtime.subscribe`）。

### 5.5 Idempotency 基础设施已在 ✅

`component/idempotency` + `sqlite.IdempotencyStore.Claim(ctx, key, fingerprint)` 已存在。vNext §5.1 只要求指纹**纳入 profile 意图、排除 `clientInfo` / `excludedEphemeralEvents`** —— 指纹组合改动，不是新建机制。

### 5.6 三处 shape 之外的执行语义（C 的真正难点，非工作量）

1. **`state_final_before_segment_finished` fence + 双通道幂等**（契约 §14.4）：同一次 state commit 要在四种投递顺序下都靠 `revision` 收敛到同一值（只收 snapshot / 只收 invalidation 后 refetch / snapshot 先 / invalidation 先）。
2. **并行 fence partial order**（契约 §14.4）：并行 child 触发 barrier 且 root 本段改过 todos 时要同时满足两条偏序，而契约**故意不规定** child finished 与 root snapshot 之间的先后 —— 实现不能依赖一个全序。
3. **`runs.cancel(child)` 让 Waiting tree 恢复**（契约 §5.4）：同一 command boundary 里 target child 走 `Waiting→Finished`、surviving suspended Runs 走 `Waiting→Running`；reducer 必须接受 `Waiting→Running` 不只来自 `runs.resume`。

> ⚠️ 2 和 3 因 **D2** 落在本轮范围外，但 shape/reducer 分支仍要落地。**1 在本轮范围内**（todos 是 session-scoped、root-only，不依赖子 Run）。

---

## 6. 实施步骤（逐 slice）

> 每个 slice 一个可独立 revert 的 commit。批次之间全绿。
> `DoD` = Definition of Done。落 commit 后在本文回填 `状态` 与 `commit`。

### Batch A —— 协议文档事实对齐

| slice | scope | 状态 | commit |
|---|---|---|---|
| A1 | 修 `desktop/docs/protocol/{API,AUX_API,TRANSPORT}.md` 中已核实漂移：steer stale 描述、MCP method table、MCP tool identity、`background.subscribe` 残留、错误续流 URL、遗漏字段 | `TODO` | — |

**影响面**：零代码、零 wire。
**DoD**：文档与当前代码逐项一致。
**价值**：给 Batch B 的 Registry 一个可信输入基线。

---

### Batch A′ —— 现役泄露治本（前置，D4）

| slice | scope | 状态 | commit |
|---|---|---|---|
| A′1 | 泄露 1 治本：Problem/Outcome detail **单一作者**。为 `RunLost / DeniedByUser / ToolFailed / Internal` 四个 kind 找回产生点并在那里写全；删 delivery 的 6 处默认文案（4 个 problem kind + 2 个 outcome kind）。领域无话可说的 → wire 省略字段 | `DONE` | `2a75bc6ae`（前端）+ `4571e91e9`（后端） |
| A′2 | D6：`Retryable` 三处删除（`domain/execution/transcript` + `delivery/dispatch` spec 表 + wire/Artifact 映射），按 kind 直接决定 `retryAfterSeconds` | `TODO` | — |
| A′3 | 泄露 2 治本：`ListRuns` 改读 durable projection，删 live registry 读取与硬编码 status。**三态留给 C1** —— 本 slice 只做「单一真相源」 | `TODO` | — |
| A′4 | 泄露 3 治本：过滤 / 排序 / keyset cursor 编解码下沉为 application query port（含 `items.list` 全量物化）；delivery 只传 opaque token | `TODO` | — |
| A′5 | 同一泄露的相邻面（A′1 实施中发现）：`mcp_projection.go` / `providers.go` 由 domain enum 编造 5 句英文 detail。**enum→enum 映射留 delivery（本职），enum→人话移入 locale catalog**。⚠️ 与 A′1 不同，**有前端半部** —— 这两个面板走 `errorDetail()`，其 fallback 是裸 symbol | `DONE` | `98399fab6`（后端）+ `5d3b2f2a7`（前端） |

**A′5 顺带治掉的根**：`errorDetail()` 的 `?? type` fallback 就是"裸符号能露出来"的总根 —— 它让这个 reader **永远无法回答"runtime 什么也没说"**，于是把「该由 UI 供词」的信号提前填满了。已改为只返 detail；要文案的走 `describeProblem` / `rpcErrorText` 单一入口。**这条 fallback 原先零测试覆盖**，现已钉住。

**A′5 另外两个发现**：
- `mcp_invalid_connection_state`：delivery 为一个不可达分支**发明的 wire 类型**，把"投影没跟上 domain 枚举"这个缺陷当成用户可读的判决发出去。已改为 panic（与 `presenter_run.go` 同一处境同一处理），该 symbol 消失。
- `mcp_auth_failed`：前端 8 语言有文案、`MAPPED_TYPES` 有登记，但**后端无任何产生点**（`AuthorizeMCPServer` 只映 `invalid_params` 或落 `internal_error`）。已删。
- ⚠️ **未擅自改动**：`mcp_authorization_required` 的 ProblemData 目前**无消费者**（两个面板都只在 `status === "failed"` 时读 `errorDetail`，而它配的是 `needsAuth`）。删它是 wire 变更 → 留给 C12 一并裁决，本轮只移走文案。

**影响面**：`delivery/server`（presenter + runs_query + items）、`adapter/agentexec/turn`、`domain/execution/transcript`、`application` 新 query port。**wire 不变**（A′2 的 `retryable` 是 wire 字段 —— 它在 C10 才从 wire 消失，A′2 只断掉 domain 侧来源，wire 上恒为 false／省略）。

> ⚠️ A′2 的 wire 影响需在实施时确认：若 `retryable` 在 A′ 后恒缺省，前端现有分支会失效。**若这构成可观察行为变化，则 A′2 的 wire 半部推迟到 C10**，A′ 只做 domain + dispatch 表。**实施前必须先查前端是否消费 `retryable`。**

**DoD**：
- 三个泄露消失
- 新增 4 个 arch 守卫（`DoesNotAuthorDomainText` / `DoesNotImplementQuerySemantics` / `ReadsRunsFromDurableProjection` / `SystemInvariantsStayInApplication`）
- 全绿（build + vet + test + `-race` + golangci-lint + 22 模块 build）

---

### Batch B —— Contract Registry（D1 全量）

| slice | scope | 状态 | commit |
|---|---|---|---|
| B1 | Registry 骨架 + 方法注册：`MethodMeta{Name,Kind,Idempotency,Errors,CapabilityRules,Stability}`；`Unary[P,R]` / `Stream[P,A,E]` 泛型工厂生成 decode/invoke/encode closure；**dispatcher 直接消费 Registry，删掉第二份 method table**（`dispatch/method_names.go`）；登记全部 65 方法；`CapabilityRule.When` 支持条件门控（`sessionExport` 无条件；`checkpoints` 仅当 `restoreType ∈ {files,both}`） | `TODO` | — |
| B2 | Union 与约束 metadata：`UnionSpec` / `ObjectConstraintSpec` / `FieldCondition` / `PresenceRule` / `StateKeySpec`；登记契约 §11.2 点名的 13 类高风险 union（先按当前 shape）。`SystemInvariantSpec` 按 **D3** 注册在 application | `TODO` | — |
| B3 | 生成器与 14 类产物（含 TS wire types + typed client stubs）。生成器置于**环外** build-time 工具。`streamingMethods` 转生成 | `TODO` | — |
| B4 | CI drift gate 18 项。依赖 C 才有意义的 3 项（#16/#17/#18）先建骨架标 pending | `TODO` | — |

**反泄露 DoD（每 slice 都查）**：
- `delivery/protocol` 仍不 import domain/application（`TestProtocolStaysWireOnly` 绿）
- 生成的 `Validate()` 依赖图不含 store/dispatcher/executor（gate #7）
- Registry 的 `Errors []ProblemType` 只声明**闭合集**（该方法可能返回哪些），**不编码触发条件**（何时返回）—— 后者属 application

**Batch B DoD**：`go generate` 后 worktree 无 diff；**当前 wire 一字未变**；18 gate 中不依赖 C 的全部通过。

---

### Batch B′ —— 权威读面 + store cutover（建议起点）

> 不广告 vNext、不暴露新字段/新方法。**每个 commit 全绿并可落 main。**

| slice | scope | 状态 | commit |
|---|---|---|---|
| B′1 | bump internal store schema epoch；遇旧 epoch **确定性拒绝启动并提示重建**，不后台静默删用户文件；不 `ALTER TABLE`、不读旧行、不伪造零值、不留 compatibility view / dual-read / dual-write | `TODO` | — |
| B′2 | `runs` 重建为 typed durable projection（补 §3.3 的 12 列）；在 start / interrupt / resume / terminal / restart-recovery 的**既有事务内**维护（契约 §4.6 事务边界表）。⚠️ `idx_runs_session_active` 保持原样 | `TODO` | — |
| B′3 | 删 `history_runs` 表 + `payload`；`message_mark`、rollback/fork、`usage.session/summary` 全改读 `runs`；**terminal transcript / Artifact adapter 也改读同一 typed projection**，消除「只存在于 finished payload 的 metrics/outcome 第二真相」 | `TODO` | — |
| B′4 | application query ports：Run query（读 durable projection）、Item query（按 Session / Run subtree，durable `sequence` 唯一排序键，为 `run_id/sequence` 与 lineage traversal 建索引）、`interrupts.list` 的 waiting-Run aggregate keyset query（**不拆散 set**、按 `root_run_id`、每个 Interrupt 持久化来源 `run_id`）。**过滤 + 排序 + cursor 编解码三件事全在 port**，delivery 一件不留。`waiting` 停止折叠（一对一 enum 映射，不引入优先级规则） | `TODO` | — |
| B′5 | todos session-scoped projection：补 `revision`、单调分配、root-only write policy、按 history boundary 的 lifecycle substrate、只读 query/presenter。**`todos.get` wire method 留到 C7** | `TODO` | — |

**Batch B′ DoD**：fresh schema 无 `history_runs`；旧 schema fixture 被明确拒绝；代码库无旧 Run payload decoder、无零值回填、无兼容读路径；全绿；**未对外声称支持 vNext**。

---

### Batch C —— vNext 原子切换

> **一个 release、一条 cutover 分支。** 分支内按依赖顺序做小而可审查、每个全绿的栈式提交；**只有整体合并时才切版本**，所以 main 上不存在半套 vNext。分支可整体 revert。
>
> **依赖**：B 与 B′ 都完成。

| slice | scope | 状态 | commit |
|---|---|---|---|
| C1 | `RunStatus` 三态；`RunOutcome`/`SegmentOutcome` 拆分；删 `RunResult`+`durationMs`；`RunMetrics`/`RunLimits` | `TODO` | — |
| C2 | `RunSummary` base + `RunRef extends`（**同一嵌入定义生成 Go/TS/schema**） | `TODO` | — |
| C3 | `RunProtocolProfile` durable 不可变 + `features.subagents` gate（广告 `false`）+ Minimal Profile + capability preflight | `TODO` | — |
| C4 | `runs.get` 新方法 + `runs.list` 全历史/status/`includeDescendants` + cursor 绑定规范化 filters | `TODO` | — |
| C5 | `ItemListScope` 闭合联合替换扁平请求 + `items.list.order` 双向 + 页级 `RunSummary[]` | `TODO` | — |
| C6 | `Interrupt.runId` 必填 + payload 三 variant + `OpenInterrupt`→`PendingInterruptSet{rootRunId}` + `interrupts.list` 取代 `runs.listOpenInterrupts`（aggregate 分页，set 不跨页） | `TODO` | — |
| C7 | `todos.get` + typed `state.snapshot` + `revision` reducer + **Segment final snapshot fence**（§5.6 第 1 点） | `TODO` | — |
| C8 | `session_has_active_run` typed conflict（带 `activeRun`）+ 无隐式 cancel 的 start 语义 | `TODO` | — |
| C9 | `runs.resume.input` 原子 user Item + 条件必填 `userItemId`（iff 关系登记为 method contract fixture） | `TODO` | — |
| C10 | `ProblemData` 删 `channel`/`retryable`、加 `activeRun`/`requiredCapabilities` + `RecoveryAction` 闭合枚举 + Error Registry | `TODO` | — |
| C11 | `ServerCapabilities` 新 shape（`runEvents`/`runtimeTopics`/`stateSnapshots`/`FeatureCapability` 两新字段/`limits` 两新块） | `TODO` | — |
| C12 | 九值闭合 `RuntimeTopic` + `runtime.subscribe` 取代 `workspace.subscribe`；删 `mcp.serverChanged` payload 真相、`schedules.fired` | `TODO` | — |
| C13 | cursor/replay 定稿（`processEpoch`、`headEventId` opaque、retention 走 discover） | `TODO` | — |
| C14 | `sessions.rollback/export/import` 精确 capability 规则 + Artifact v7 | `TODO` | — |
| C15 | 前端：exhaustive reducer、**删 business numeric code 镜像**、`runtime.subscribe` 单流、删 goal 4 秒轮询、高层 run handle + tail-first cold recovery。✅ 单流已结构满足，本 slice 是改名 + topic 扩容 + reducer 扩展 | `TODO` | — |
| C16 | 切版本：`protocolVersion = minSupported = "2026-07-27"`、`SessionArtifactVersion = 7`；旧协议 `invalid_protocol_version`、旧 Artifact 确定性拒绝 | `TODO` | — |

**每 slice 反泄露 DoD**：新增 wire 概念不得在 presenter 产生业务判断（对照 §4.3 归属表）。

**Batch C DoD**：契约 §14 验收矩阵九节全绿；18 项 CI gate 全通；分支可整体 revert。
**D2 相关的验收项处理**：§14.1 中 child 相关项记为 **capability-gated 不适用**，并在 fixture 里**显式断言「请求即报 `capability_not_negotiated`」**，而非跳过。

---

### Batch D —— 切换后数值调优

| slice | scope | 状态 | commit |
|---|---|---|---|
| D1 | slow-consumer 与 replay memory benchmark | `TODO` | — |
| D2 | 在不改 scope/语义前提下调 discover 返回的 replay 数值上限 | `TODO` | — |
| D3 | event coalescing / subscriber queue / 生成速度优化 | `TODO` | — |
| D4 | 真实桌面负载验证一条 runtime stream + 多条 active Run stream 的连接占用 | `TODO` | — |

**Batch D 不得重新引入 Batch C 已删除的兼容层。**

> ⚠️ **这是本仓库第一次有 benchmark 需求。** 记忆 `project_agent_refinement_closed_perf_dropped` 论证关闭的是 **agent 模块 CPU 延迟维度**，**不覆盖这里** —— replay journal 与 subscriber queue 是**内存与连接**资源，且契约 §6.5 明确把数值上限做成可测量调整项。

---

## 7. 顺序与并行

```text
A ──→ A′ (4 slice, 修现役泄露, 落 main) ──┬─→ B  (Registry, 4 slice) ─┐
                                         └─→ B′ (store+读面, 5 slice) ┴─→ C (16 stacked, 一条分支) ─→ D
                                              ↑ 每 commit 全绿可落 main      ↑ 仅整体合并时切版本
```

- **A′ 前置**（D4）：B′ 要重建的正是被泄露污染的三处读面。先修再建。
- **B 与 B′ 无相互依赖，可并行。** B′ 每个 slice 可独立落 main，风险最低，且先清掉「第二真相源」会让 C 简单不少 —— **建议作为起点**。
- **C 必须等 B 与 B′ 都完成。**

---

## 8. 待做的记忆修正（本轮结束时）

| 记忆 | 修正 |
|---|---|
| `project_lyra_no_protocol_ts_codegen` | 「codegen 拒」被 **D1** 推翻，必须改写；写清推翻理由（`UnionSpec` 显式 metadata 解掉了 flat-struct 不映 union 的问题） |
| `project_agent_refinement_closed_perf_dropped` | 补一句：性能维度关闭只覆盖 agent 模块 CPU 延迟，**不覆盖 Batch D** 的 replay/queue 内存与连接资源 |
| **新增** | delivery 反泄露法则 + 三个现役泄露的**形态**（补业务文案 / 二次真相源 / 自己实现查询语义）—— arch 守卫只挡具体符号、挡不住新形态，所以判据要进记忆 |

---

## 9. 复查清单（跨 session 接手时先过这一遍）

0. **执行法则三条**（文首，不编号）—— 每个 slice 开工前与收工前各过一遍。它凌驾于本节其余各项。
1. **§0 状态总览**与实际 commit 是否一致？不一致 → 以 git 为准，回填本文。
2. **§2 的六个决策**是否仍成立？特别是 D2（子 Run 不做）—— 若改主意，§2.1 表格里的每一行都要重新裁决。
3. **§5 的六条技术判断**不要重新论证。特别是 **§5.1（`Problem.Scope` 不要删）**。
4. **§4.3 归属表**是每次写 delivery 代码前的必查项。
5. **§4.4 的 6 个 arch 守卫**是否已落地？没落地就是本文与代码不一致。
6. 契约冲突时改本文；代码冲突时改代码（见文首）。

---

## 附：参考

- [`codex_runtime_protocol_vnext_final.md`](codex_runtime_protocol_vnext_final.md) —— **冻结契约，唯一目标基线**
- [`PROTOCOL_DESIGN.md`](PROTOCOL_DESIGN.md) —— 决策历史
- [`PROTOCOL_VNEXT_REVIEW.md`](PROTOCOL_VNEXT_REVIEW.md) —— 八轮评审
- [`codex_runtime_api_design_guide.md`](codex_runtime_api_design_guide.md)
- [`../desktop/docs/protocol/`](../desktop/docs/protocol/) —— 切换前的当前 wire 真相（API / AUX_API / TRANSPORT）
- [`../runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md`](../runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md) —— 环形架构基准
- `app/runtime/internal/arch/arch_test.go` —— 反泄露守卫的机器强制点
