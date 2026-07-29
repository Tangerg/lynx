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
| A | 协议文档事实对齐 | `DONE` | wire 零变化；「零代码」判断错了，见 §6 A1 范围修正 |
| A′ | 现役泄露治本（**前置**） | `DONE` | 5 slice 全 `DONE`（A′5 为实施中发现追加）；三个现役泄露消除，5 个守卫落地 |
| B | Contract Registry（全量） | `TODO` | 4 slice，可与 B′ 并行 |
| B′ | 权威读面 + store cutover | `DONE` | B′1 + B′2/B′3（合并）落 main；B′4 逐项核实为已完成 / 余项 `DEFERRED → C`（无 producer），B′5 `DEFERRED → C7` |
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

当前 **83** 个可派发方法（`dispatch/method_table.go` 实测；计划原写「65」为审计误差，2026-07-29 更正）+ 2 个
server→client notification。核心面差异：

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
TestDeliveryDoesNotAuthorDomainText        // ✅ DONE (4571e91e9 + 98399fab6)
TestDeliveryDoesNotImplementQuerySemantics // ✅ DONE (dfcadad95)
TestDeliveryReadsRunsFromDurableProjection // ✅ DONE (03943f663)
TestSystemInvariantsStayInApplication      // ✅ DONE (B2) — 禁 delivery 出现 SystemInvariantSpec
TestDeliveryDoesNotComputeMetrics          // C 预埋
TestRunWireShapesShareOneDefinition        // C 预埋
```

**A′1 的守卫比原计划强，理由值得记下**：原计划写的是「禁 default 文案字面量」，但泄露的实际形态是**文案藏在一个看起来像 mapper 的 helper 里**（`presentProblemDetail`），禁字面量赋值挡不住它。落地形态改为：*在 `presenter_run.go` / `artifact_encode.go` 里，字符串字面量只能是给程序员的诊断*（豁免 import path / `panic` / `errors.New` / `fmt.Errorf` / 空串）。已验证：把文案塞进新 helper 里也会被咬到。
> 教训：守卫要挡**形态**，不是挡**符号名**。换个函数名就绕过的守卫等于没有。

**A′4 的守卫同样按形态写**：除了禁回 `pageByCursor` 与两个页宽常量，更关键的是**禁 delivery 调用 `keyset.Decode/Encode/Limit/PageOf`** —— delivery 可以命名"这个 cursor 被拒了"（映射成 `invalid_params`），但不能运行任何分页机制。已验证：在 handler 里加一行 `keyset.Limit` 立刻被咬。

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
| A1 | 修 `desktop/docs/protocol/{API,AUX_API,TRANSPORT}.md` 中已核实漂移：steer stale 描述、MCP method table、MCP tool identity、`background.subscribe` 残留、错误续流 URL、遗漏字段 | `DONE` | 见下 |

**影响面**：wire 一字未变；**代码有变**（见下方 ⚠️ 范围修正）。
**DoD**：文档与当前代码逐项一致。
**价值**：给 Batch B 的 Registry 一个可信输入基线。

#### ⚠️ A1 范围修正（2026-07-29 实施中）：不是"零代码"

计划写「零代码、零 wire」。**wire 确实零变化，但"零代码"错了** —— 同一份漂移在代码里也有副本，只改文档等于把
「文档与代码一致」做成「文档与另一半代码一致」。逐条核实后，六项点名漂移的实际形态是：

| 计划点名项 | 实际形态 |
|---|---|
| steer stale 描述 | §13「v2 明确不做」仍列着 *"经 `runs.send` 的 mid-run steering（留 v2.x）"*，而它早已作为 `runs.steer` 落地。**同一句谎言在代码里也有一份**：`InjectSteering` 的 godoc 写着 *"This is next-turn semantics — not true mid-stream injection"*，而它下面十行的 `steerSource` 就是逐轮 drain 的 mid-run 注入 |
| MCP method table | §7.5 表缺 6 个方法（`mcp.servers.authorize` + `mcp.configs.*` 五个），§4.10 缺 `McpServerConfig` / `ConfigureMCPServerRequest` / `McpTestResult` 三个类型 —— **`mcp.configs.*` 的 6 个 wire 字段在三份文档里 0 次出现** |
| MCP tool identity | §4.4 写 `MCP 用 "<server>.<tool>"`。真相是**两个身份**：`McpTool.name` 是远端原名，`ToolInvocation.name` 是 `sanitize("<server>_<tool>")` 截断 64 —— **有损**（`("a_b","c")` 与 `("a","b_c")` 都塌成 `a_b_c`，domain 有测试钉着）。审批 `remember` 的 key 是后者、`disabledTools` 用前者，文档一处都没说 |
| `background.subscribe` 残留 | TRANSPORT 4 处（header 表 / 流式方法举例 / §6.4 要点 / §7 SSE `id:` 规则），指向一个 §7.7 已宣布删除的方法 |
| 错误续流 URL | §9.2 写 `POST /v2/rpc/runs.subscribe`，与同文件 §6.1「URL 不再重复 method」自相矛盾（router 只有 `POST /v2/rpc`）。**连带**：`400` 的含义仍写着「URL method 与 body method 不一致」—— 那个分支不存在了；`500` 写「dispatch **前**的适配器失败」，实际三个 500 全在 dispatch **后** |
| 遗漏字段 | 用 json tag 全集扫三份文档，258 个字段里 9 个 0 次出现：6 个属上面的 `mcp.configs.*`，另 3 个是真遗漏 —— `RunProgress.contextTokens`、`TodoSnapshot.blockedReason` / `.nextAction` |

**顺带查出的、计划没有的：**

1. **`excludedEvents` 文档声称的行为不存在。** AUX_API 写「按事件 `type` 抑制（如 `["mcp.serverChanged"]`）」，但
   `streamFilter` 只挂在 run 事件上，workspace 事件根本不过滤。且即使在 run 流上，它**只对 ephemeral 生效** —— 这不是
   缺陷而是**保证**：durable 帧可被 opt-out 就等于客户端能把自己 opt-out 出 §5.2 的收敛性。已按「事实 + 它为什么是对的」
   写进两份文档与 Go 注释，而不是把文档那句话当目标去实现。
2. **`bash→shell` 改名留下 6 处死键**（`project_shell_tool_rename` 那轮的残留）：`run_in_background` 与 `subagent`
   **都不是工具名** —— 前者是 `shell` 的一个**参数**，后者从来没存在过（真名 `task`）。它们出现在后端
   `toolPresentations` ×2、`approval.Query.subject()` 的 `case` ×1、前端 icon 表 / preview 表 / `TOOL_CATEGORY` ×3，
   外加 4 个测试**为不存在的工具锁定行为**（含一条 approval subject 提取的断言）。全部删除并把测试改写成真实形态。
3. **§4.4.2 展示约定表的 arguments 列全错**：`file_path` 被写成 `path`（read/edit/write）、`pattern` 被写成 `query`
   （grep）、`web_search` 被写成 `webSearch`、`task` 被写成 `subagent`。**result 列反而是对的** —— 因为
   `normalizeToolResult` 真的把结果归一化成了约定形状。另有 `outputTruncated` **无任何产生点**（后端 0 处），已从表里删。
4. **`ProblemData.retryable` / `McpServer.status:"disconnected"` 是两个没有作者的 wire 成员**（前者 A′2 删掉了 domain
   侧来源，后者 domain 状态机只有 4 态）。二者都**只**在文档里标注「声明着但无产生点」+ 禁止从 `type` 反推 —— 删 wire
   成员属破坏性改动，按契约排在 C10 / C11，不在本 slice 抢跑。
5. **A′5 的 4 个 inline-status problem type 从未进文档**（`mcp_authorization_required` / `mcp_dial_failed` /
   `provider_not_configured` / `provider_test_failed`）。它们既不带数字码、也不走 §8.1 三条通道 —— 骑在查询自己的结果
   里，且**刻意无 `detail`**（文案归客户端 locale 表）。已作为「第四类」写入 §8.4。

**为什么这些代码改动仍属 A1、而非另开 slice**：它们没有一处改变行为或 wire —— 只删死键、修谎注释、改写为不存在的
工具写的断言。把它们留到"以后"，等于让 Batch B 的 Registry 从一份仍有 6 个幽灵工具名的代码里取事实。

---

### Batch A′ —— 现役泄露治本（前置，D4）

| slice | scope | 状态 | commit |
|---|---|---|---|
| A′1 | 泄露 1 治本：Problem/Outcome detail **单一作者**。为 `RunLost / DeniedByUser / ToolFailed / Internal` 四个 kind 找回产生点并在那里写全；删 delivery 的 6 处默认文案（4 个 problem kind + 2 个 outcome kind）。领域无话可说的 → wire 省略字段 | `DONE` | `2a75bc6ae`（前端）+ `4571e91e9`（后端） |
| A′2 | D6：`Retryable` 三处删除（`domain/execution/transcript` + `delivery/dispatch` spec 表 + wire/Artifact 映射），按 kind 直接决定 `retryAfterSeconds` | `DONE` | `0b7888f8c`（后端）+ `b3f3fb6b3`（前端） |
| A′3 | 泄露 2 治本：`ListRuns` 改读 durable projection，删 live registry 读取与硬编码 status。**三态留给 C1** —— 本 slice 只做「单一真相源」 | `DONE` | `03943f663` |
| A′4 | 泄露 3 治本：过滤 / 排序 / cursor 编解码下沉；delivery 只传 opaque token。**按 A′4-全 + 真 keyset 执行（见下方 ⚠️）** | `DONE` | `2130ba78d`（keyset 原语）+ `f87d7f9ca`（items.list）+ `dfcadad95`（余下四面 + 守卫） |
| A′5 | 同一泄露的相邻面（A′1 实施中发现）：`mcp_projection.go` / `providers.go` 由 domain enum 编造 5 句英文 detail。**enum→enum 映射留 delivery（本职），enum→人话移入 locale catalog**。⚠️ 与 A′1 不同，**有前端半部** —— 这两个面板走 `errorDetail()`，其 fallback 是裸 symbol | `DONE` | `98399fab6`（后端）+ `5d3b2f2a7`（前端） |

**A′5 顺带治掉的根**：`errorDetail()` 的 `?? type` fallback 就是"裸符号能露出来"的总根 —— 它让这个 reader **永远无法回答"runtime 什么也没说"**，于是把「该由 UI 供词」的信号提前填满了。已改为只返 detail；要文案的走 `describeProblem` / `rpcErrorText` 单一入口。**这条 fallback 原先零测试覆盖**，现已钉住。

**A′5 另外两个发现**：
- `mcp_invalid_connection_state`：delivery 为一个不可达分支**发明的 wire 类型**，把"投影没跟上 domain 枚举"这个缺陷当成用户可读的判决发出去。已改为 panic（与 `presenter_run.go` 同一处境同一处理），该 symbol 消失。
- `mcp_auth_failed`：前端 8 语言有文案、`MAPPED_TYPES` 有登记，但**后端无任何产生点**（`AuthorizeMCPServer` 只映 `invalid_params` 或落 `internal_error`）。已删。
- ⚠️ **未擅自改动**：`mcp_authorization_required` 的 ProblemData 目前**无消费者**（两个面板都只在 `status === "failed"` 时读 `errorDetail`，而它配的是 `needsAuth`）。删它是 wire 变更 → 留给 C12 一并裁决，本轮只移走文案。

**影响面**：`delivery/server`（presenter + runs_query + items）、`adapter/agentexec/turn`、`domain/execution/transcript`、`application` 新 query port。**wire 不变**（A′2 的 `retryable` 是 wire 字段 —— 它在 C10 才从 wire 消失，A′2 只断掉 domain 侧来源，wire 上恒为 false／省略）。

---

#### ⚠️ A′4 范围修正（2026-07-28 实施中发现，**已按 A′4-全 + 真 keyset 完成**）

计划原文说 A′4 含「`items.list` 全量物化」的治本，并把 durable keyset 推给 B′4。**两处都判断错了**，实施时纠正：

**事实 1 —— `pageByCursor` 的签名本身就要求全量物化。**

```go
func pageByCursor[T any](elems []T, key func(T) string, cursor string, limit, maxLimit int) ([]T, string, error)
```

它拿全量 slice 线性扫描找 cursor 锚点。真 keyset 分页必须在 SQL 里 `WHERE (sort_key) > (anchor) LIMIT n` —— 那需要 durable 排序键：
- `runs.list`：**今天就可以**（A′3 已落 `ORDER BY started_at, run_id`）。
- `items.list`：**`history_items.seq` 是 AUTOINCREMENT 主键、且 `idx_history_items_session(session_id, seq)` 早已存在** —— B′4 打算"新增"的 durable sequence 本来就在。**不依赖 B′。**
- `interrupts.list`：`(created_at, run_id)` 可用，`run_id` 是主键。**不依赖 B′**（root-owned aggregate 是 C6 的语义收紧，与分页机制无关）。
- `sessions.list` / `schedules.list`：`(favorite, updated_at, id)` / `(created_at, id)` 都有索引。**不依赖 B′。**

**事实 2 —— `pageByCursor` 有 5 个调用点，跨 4 个域，不是计划设想的 2 个。**

| 调用点 | 域 | delivery 现在做了什么 |
|---|---|---|
| `runs_query.go` ListRuns | runs | ~~过滤+排序~~（A′3 已下沉）+ 分页 |
| `runs_query.go` ListOpenInterrupts | interrupts | 分页 |
| `items.go` ListItems | items | 分页（+ 全量物化） |
| `sessions.go` | sessions | 分页 |
| `schedules.go` | schedules | 分页 |

后两个的过滤/排序**已经在 application**，delivery 只剩分页。

**由此得到的两种 A′4：**

| | 范围 | 代价 | 结果 |
|---|---|---|---|
| **A′4-窄** | 只动 §4.2 点名的三个（runs / interrupts / items）的分页归属 | 小 | `pageByCursor` 仍留在 delivery 供 sessions / schedules 用 → **守卫 `TestDeliveryDoesNotImplementQuerySemantics` 落不了地** |
| **A′4-全** | 分页机制移入 `component/`（域中立原语），**5 个调用点各自的 application coordinator 做分页**，delivery 一件不留 | 3 个 application 包 + 1 个新 component 包 + 5 个 handler + 新 sentinel（`queries` 不能返 `protocol.ErrInvalidParams`，需自己的错误由 delivery 映射） | 泄露 3 的**所有权**彻底清掉，守卫可落地；B′4 之后只是把内存分页换成 SQL keyset，**port 签名不变** |

**已执行：A′4-全 + 真 keyset。** 内存分页那个中间态被完全跳过 —— 它会被 B′4 推翻，正是要防的返工。落地形态：

```text
component/keyset/          cursor 编解码 + limit 收敛 + 过取一行的 Page[T]
                           token = formatVersion + method + filters 指纹 + sort tuple
                           跨 method / 跨 filters / 损坏 → ErrInvalidCursor（不静默重置）
5 个 store                 各自 ORDER BY 上加 keyset 谓词 + LIMIT
3 个 application 包        各自持有 filters + order + cursor + 页宽上限
delivery                   只传 opaque token；把拒绝映射成 invalid_params
```

**顺带治掉的两处**：
- `sessions.list` 原先把**每个** session 都做完 fs + live-run 富化，再由 delivery 切出 100 条 —— 现在只富化本页。
- 两个 port 收窄：无分页的 interrupt 读、整体 transcript 读**都没有消费者了**，直接删（而非留成比实际驱动更宽的 seam）。

**B′4 因此大幅缩小**：durable keyset 已在，剩下的只是 `runs` 补列后把 `AdmittedRun` 读面变宽（C4 的全历史 / status filter 才需要）。
**C13 只剩 replay cursor**（`processEpoch` / `headEventId`）—— query cursor 已是契约要求的形态。

> ✅ **A′2 的 wire 问题已在实施时查清（2026-07-28）**，结论比预设的更干净：
>
> `retryable` 的 json tag 是 `omitempty`，**bool 的 `false` 会被省略** —— 所以 wire 上 `retryable` 从来只有"true 或缺席"两态。前端唯一的消费点 `RunErrorBanner` 写的是 `error?.retryable !== false`，**恒为真**：它注释里声明的"永久性错误（凭证错 / 参数错）不给 Retry"从上线起就没生效过。
>
> 因此：**wire 半部无可观察行为变化，不必推迟**；A′2 做了 domain + dispatch 表 + 三处映射删除。前端的 retry 门禁改为按 symbolic type 判断（`invalid_api_key` / `invalid_params` / `provider_rejected`），并从 `RunError` 删掉该字段 —— 旧写法因此**在类型上不可表达**，比加测试更强。
>
> ⚠️ **`ProblemData.Retryable` / `ArtifactProblem.Retryable` 两个 wire 字段仍声明着**（无 writer），按契约在 **C10 / C14** 随 `protocolVersion` / Artifact v7 一起删 —— 删 wire 字段属破坏性公开 API 改动，归 C 的原子切换。两处都留了注释**禁止**从 `Type` 反推它（那正是泄露 1 的形态）。
>
> 📌 **这是 `omitempty`-on-bool 的同一类 bug**（agent 模块那次是 `omitempty`-on-struct）：**字段的"缺席"与"负值"不可区分时，任何 `!== false` / `!= zero` 的门禁都是死的。** 扫其余 wire bool 时按这条查。

**DoD**：
- 三个泄露消失
- 新增 4 个 arch 守卫（`DoesNotAuthorDomainText` / `DoesNotImplementQuerySemantics` / `ReadsRunsFromDurableProjection` / `SystemInvariantsStayInApplication`）
- 全绿（build + vet + test + `-race` + golangci-lint + 22 模块 build）

---

### Batch B —— Contract Registry（D1 全量）

| slice | scope | 状态 | commit |
|---|---|---|---|
| B1 | Registry 骨架 + 方法注册：`MethodMeta{Name,Kind,Idempotency,Errors,CapabilityRules,Stability}`；`Unary[P,R]` / `Stream[P,A,E]` 泛型工厂生成 decode/invoke/encode closure；**dispatcher 直接消费 Registry，删掉第二份 method table**（`dispatch/method_names.go`）；登记全部 83 方法 + 2 notification；`CapabilityRule.When` 支持条件门控（`sessionExport` 无条件；`checkpoints` 仅当 `restoreType ∈ {files,both}`） | `DONE` | 见下 |
| B2 | Union 与约束 metadata：`UnionSpec` / `ObjectConstraintSpec` / `FieldCondition` / `PresenceRule` / `StateKeySpec`；登记契约 §11.2 点名的 13 类高风险 union（先按当前 shape）。`SystemInvariantSpec` 按 **D3** 注册在 application | `DONE` | 见下 |
| B3 | 生成器与 14 类产物（含 TS wire types + typed client stubs）。生成器置于**环外** build-time 工具。`streamingMethods` 转生成 | `DONE` | 见下（**14/14**） |
| B4 | CI drift gate 18 项。依赖 C 才有意义的 3 项（#16/#17/#18）先建骨架标 pending | `IN PROGRESS` | 见下 |

#### ✅ B4 完成（2026-07-29）：18/18（gate 15/16/17/18 随 C16 落）

| gate | 内容 | 状态 |
|---|---|---|
| 1 | `go generate` 后 worktree 无 diff | ✅ `c83d041f3`（`TestGeneratedContractHasNoDrift` + 防空转的 Substantive） |
| 2 | Registry method 集合 == dispatcher 集合 | ✅ **构造上成立**（B1 后 dispatcher 直接路由 Registry，无第二张表）+ `TestContractIsTheOnlyMethodTable` |
| 3 | capability rules 三方等价 | ✅ **三方全落**：dispatcher↔discovery 构造上等价（enforcement 读 advertisement，`ba6a301db`）；SDK 侧 `rpc/preflight.ts` 读**生成的** `WIRE_CAPABILITY_POLICY`（同一份 metas 投影，所以规则也构造上等价）。剩下唯一能分歧的是**条件语义**（`present` 对空数组算不算命中），所以 `preflight.test.ts` 把 dispatch 那 7 条 gate case 原样问一遍客户端 matcher —— 产物比对永远发现不了这类分歧 |
| 5 | 所有 closed union 有 discriminator + 完整 variant | ✅ `7e7a9ec12`（注册期反射校验，含"字段无变体认领"） |
| 7 | DTO validator 无 store/dispatcher/executor 依赖 | ✅ `fff51823d`（禁 internal import + 禁 Validate 带参数） |
| 12 | protocol manifest / canonical 文档 / 代码 三方版本一致 | ✅ `TestProtocolVersionAgreesEverywhere` —— C16 只改一处常量，这条点名了每份还写着旧版本的文档。**C16 扩到四方**：canonical 样本（`rpc/samples/*.json`）也是**已发布的版本声明**（客户端会照抄），而 shape gate 看不见它——schema 管字段类型，管不了"哪个日期是当前的"。扫的是键名（`protocolVersion` / `protocol.current` / `minSupported`），所以后加的样本自动被扫；**不扫测试 fixture**（拒绝测试必须能点名一个本 build 不服务的版本） |
| 13 | business error type/code 单一源 | ✅ `c83d041f3`（error registry 由 sentinel↔code 生成） |
| 4 | OpenRPC / JSON Schema 可解析 | ✅ `TestGeneratedSchemasResolve` + `TestOpenRPCDescribesEveryMethod` —— 自带 `$ref` 解析器（无网络、无 vendored validator），并禁"定义了但无人引用"的孤儿 shape |
| 9 | TS types / validators / client stubs 可编译 | ✅ 三类全生成且前端 `npm run check` 全绿（typecheck + oxlint + prettier + 175 test file + knip + 8 个结构脚本 + bundle 预算）。**knip 是这条的真门禁**：它拒绝"生成了但没人读"的导出，所以每一类产物都必须当场接上消费者——`WIRE_STREAM_METHODS` / `WireEvent` 因此被删（无 reader，要时再生成），`WIRE_CAPABILITY_POLICY` 与 preflight 同批落 |
| 6 | 三方约束等价 | ✅ **三方全落**：`TestValueConstraintsAgreeAcrossArtifacts` 回读三份产物 —— 一份声明喂三个独立 emitter（Go 发 `required(...)`、schema 发 `minLength`、TS 发 `minLength(1)`；嵌套路径在后两者里走第四条 allOf 代码路径），构造**不**保证一致，只有回读才保证。TS 侧还按 shape 切块回读（不是全文 grep），所以"规则落在别的 shape 上"也会被点名 |
| 10 | canonical samples 三方 | ✅ **真三方**：binding 收成一份（`protocol/canonical_samples.go`，非测试代码，生成器投影进 manifest 与 `wire.samples.generated.ts`），84 个 fixture 各过三关 —— Go round-trip / TS 编译出的 checks / **ajv 读已发布的 `contract/schema.json`**。第三关不可省：前两关都从同一棵 schema 树派生，编译器里的 bug 会让它们互相认同；只有一个没参与生产的实现能回答"第三方拿到的那份文档接受运行时真发的帧吗"。三关各带反例（缺 required / variant 串字段），否则 84 个合法样本对"什么都接受"的 schema 也全过 |
| 14 | state key fixture | ⚠️ **能做的半边已落**：`TestEveryStateKeyHasAShapeFixture`（arch，证据索引）+ `server:TestStateSnapshotCarriesItsDeclaredTodosPayload` 走 presenter→wire→**按声明类型读回再重编码**比对（多一个字段会在回来的路上消失，少一个会冒出来）+ 前端 `validateStatePayload` 校验 canonical snapshot 的每个 state key。recovery 是已注册方法由注册期校验、scope/writer/feature/stability 已声明并投影。**revision/lifecycle policy 与 `state.changed` fixture 依赖 C**（今天的 wire 只有无 revision 的 `state.snapshot`，没有 policy 可钉） |
| — | **新增守卫（非 18 项之列，但同类）** | ✅ `TestEveryWireStructIsPublished`：protocol 的每个 exported struct 要么在 bundle 里、要么带理由列入 `notOnTheWire`（"两者都是"也报错）—— shape 漏发是**静默**的，这条让它出声 |
| 8 | invariant integration fixture | ✅ `TestEverySystemInvariantHasAnIntegrationFixture` —— 13 个 (invariant, boundary) 对逐一有跨 projection fixture，**双向链接**（索引点名 fixture，fixture 的 godoc 写明它守哪条 invariant）。补了 3 条新 fixture，纠正了审计里 1 处误判 |
| 11 | list query fixture | ✅ 两半都落：结构半 `TestPageCursorsBindToTheirOwnMethod`（每个 `*PageMethod` 常量必须是**已注册方法名**且**互不相同** —— 两个读共用命名空间会互相接受对方的 anchor，seek 落到错的排序上），行为半 `TestEverySeekPagedReadHasQueryFixtures`（5 个读 × 3 条腿的证据索引，双向链接）。补了 9 格 fixture，`interrupts.list` 已改名 |
| 15 | Artifact v7 round-trip | ✅ **C16**：`TestArtifactV7RoundTripsEveryFieldItCarries` —— 不是一串挑出来的断言，而是**走 shape 检查 fixture 的完备性**（每个字段都被填过，填不到的进 `unreachableArtifactFields` 带理由；一个字段变得可填就必须离开该表）+ **整份文档 export→import→export 逐字节相等**。版本号自身钉在字面量 `7`（比常量只证明"一行读另一行"多一件事）。**咬出一个真缺陷**：export 给每个 run 都盖自己的 profile，而归档的 lineage 规则禁止 child 带 profile —— 一旦出现 child run，runtime 会写出一份**它自己读不回来**的归档。两端各加一道 fail-closed（export 拒写、import 拒读并点名 `features.subagents`），gate 15 的不变量因此是"这个 runtime 写出来的，它一定读得回来" |
| 16 | compatibility diff 判本轮整体 breaking | ✅ **C16**：`internal/arch/compatibility_test.go` 的 differ（读 baseline 与当前的 manifest+schema 两份，**不读 openrpc** —— 它是二者的再表述）。分类规则只有一条判据：**"照 baseline 写的客户端会不会做错事"**。断言不是"有 breaking"（一个改名字段就满足了），而是**方法面 / 闭合 topic 集 / 错误注册表 / shape / union 五类各要出现** —— 对某一类瞎的 differ 会一直对那一类瞎。实测 64 breaking / 93 compatible |
| 17 | 新增闭合 RuntimeTopic 判 breaking + 要求同步 bump | ✅ **C16**：topic 是**闭合**集合（客户端 exhaustive fold），故**加成员与删成员同样 breaking**；规则按一般形式写（**任何** breaking 都要求 bump），否则下一版删个方法就免费了。**带三种反例**：contract 与自己 diff 必须零变化（否则"永远说 breaking"的函数也能过 16/17）、16 个合成 pair 逐条钉分类（新增可选字段/新方法/新错误码 = 兼容；字段变必填/改类型/删字段/闭合枚举加值/新 capability 要求/改码号/抬 minSupported = breaking） |
| 18 | state fixture（reducer 不倒退 / ownership 不越界 / segment fence / runtime invalidation） | ✅ **C16**：四条 claim 的证据索引 + **双向链接**（claim 点名 fixture，fixture godoc 点名 claim）。四条里**三条此前没有专属 fixture**，cold read 在**任何层**都没有 —— 与"`todos.get` 前端零调用方"是同一个盲区；补了 `TestTodoStateIsOwnedByItsSession`（两 session 各自的 revision 空间 + **把更早的值重新写入必须拿更大的 revision**）、`TestTodosQueryAnswersWithTheStreamsOwnSnapshot`（cold read 必须与事件同形同 revision，否则按 revision 折叠的客户端会把 recovery 的答案当旧值丢掉）、`TestSegmentFencesItsFinalStateBeforeFinishing`（终态前一帧是终值，**没改过就不发** —— 一份 revision 0 的空快照读作"清单被清空了"）。顺手发现 harness 的 queries 协调器**没接 todo 端口**，所以 `todos.get` 在后端也是零 fixture |

#### ✅ B3 完成（2026-07-29）：14/14 产物

**已落地**（`cmd/contractgen` → `app/runtime/contract/manifest.json`，38KB，`go:generate` 挂在
`internal/delivery/dispatch/contract_methods.go`）：

| # | 产物 | 来源 |
|---|---|---|
| 1 | protocol manifest | `Registry.Metas()` |
| 2 | error registry | `protocol.Err*` sentinel ↔ `Code*` + run-channel / inline-status 两类无码符号 |
| 3 | capability gate policy | `MethodMeta.CapabilityRules`（含 `When` 条件） |
| 4 | run event policy | **调 `StreamEvent.IsDurable()`**，不抄 §5.2 表 —— 与 hub replay buffer / SSE id gate 同一函数 |
| 5 | state snapshot recovery policy | `StateKeySpec.RecoveryMethod` |
| 6 | state scope/writer/lifecycle policy | `StateKeySpec.Scope/Writer` |
| 7 | runtime topic capability list | `WorkspaceEvent` union + per-topic feature |
| 8 | system invariant manifest | `application/contract.SystemInvariants()` |
| 9 | **human-readable API reference** | manifest 的 markdown 投影（`contract/API_REFERENCE.md`）—— 14 类里唯一不需要 schema walker 的剩余项 |

外加：union / objectConstraint 两节（B2 的 spec 投影）、`streamingMethods`（Registry 派生）。

**生成器落点定案**：`app/runtime/cmd/contractgen/`。它同时读 `delivery`（method/shape spec）与 `application/contract`
（invariant spec）—— 这个组合没有任何 runtime 组件可以有。D3 的洞察是「分层规则约束运行时 import 图，生成器不在那张图里」。

**drift gate 已落**（§11.4 gate 1）：`internal/arch/contract_drift_test.go` 重跑生成器并比 worktree；配一条
`TestGeneratedContractIsSubstantive` 防止空 manifest 让 gate 空转通过。

**schema walker 已落地**（`cmd/contractgen/schema.go`）—— 第 10、11 类产物随之落地：

| # | 产物 | 落点 |
|---|---|---|
| 10 | **JSON Schema bundle** | `contract/schema.json`，232 个定义、171 个内部 `$ref` 全解析 |
| 11 | **OpenRPC** | `contract/openrpc.json`，83 方法；shape **不复制**，全部 `schema.json#/$defs/X` 外部引用 |

**walker 的分工原则**：反射看得见的（字段名、`omitempty` 决定的可选性、嵌套、泛型实例化）走反射；反射看不见的
（discriminator、variant 字段表、presence rule、enum 值集）走已注册的 spec。**两半都不猜对方的活**。

三条实施决定：

1. **不发 `additionalProperties: false`。** Go decoder 忽略未知字段，所以闭合 schema 会拒掉 runtime 接受的帧 ——
   *比代码严*和*比代码松*一样是说谎，且严的那种直接打断所有前向兼容客户端。variant 排他靠 `properties: {x: false}`
   （boolean schema）表达，不靠关闭对象。
2. **enum 值集必须声明**（新 `protocol/wire_enums.go`，51 个 wire enum）。反射能看到字段类型是 `RunStatus`，永远看不到
   `RunStatus` 只有两个值 —— const block 不可运行时枚举。表**引用常量、不复写字面量**（值改名只有一处），并由
   `TestWireEnumsAreComplete` 用 AST 读回常量比对。**这不违反 §11.2 的「AST 只可读取 godoc」**：读 AST 的是**测试**，
   产物管道读的是声明；测试只证明"声明就是全部真相"。没有这张表，每个 enum 在 schema/TS 里都是裸 `string` ——
   一份允许 runtime 会拒的帧的已发布契约。
3. **`encoding/json` 语义收成一处**（新 `protocol/wire_fields.go`：`WireFields` / `WireFieldNames` / `LookupWireField` /
   `HasWirePath` / `Deref`）。dispatch 的注册期校验、schema walker、待做的 TS emitter 与 validator generator 需要同一个
   答案；三四份私有 copy 就是三四次对"wire 到底长什么样"产生分歧的机会。`contract_shape.go` 的四个私有 helper 已删。

**当场咬到一个真错**：`external()`（把 walk 出来的引用重指向 bundle）原先只改顶层 `Ref`，于是 `[]ContentBlock` 这类
**嵌套在数组里的引用**留在了 openrpc.json 里指向自己（3 处 dangling）。改成深拷贝重写 —— 拷贝而非就地改，是为了
两份文档相互独立：渲染一份永远不可能在另一份里留下本地引用。gate 4 的解析器正是为抓这类错写的。

**第 12 类 TypeScript wire types 已落地**（`app/desktop/frontend/src/rpc/wire.generated.ts`，1290 行）—— 手写的
`src/rpc/shapes.ts`（1304 行）**已删**，`memory: project_lyra_no_protocol_ts_codegen 的"拒 codegen"结论由本批推翻**：
当年的理由是「Go flat-struct 不映 TS union」，而 B2 的 union spec 正好补上了 reflection 缺的那一半，现在生成的是**真正的
discriminated union**（比手写的更严）。

**TS 由 schema 树派生，不做第二次 Go 类型 walk** —— 两次独立 walk 就是同一形状的两个作者，而 gate 6 要的正是"schema 与
TS 等价"。派生使等价**结构上成立**：不存在"schema 里禁、TS 里放"的路径。

**Go 泛型还原成 TS 泛型**：reflect 只看得见实例化（`Page[Session]`），发 19 份 `PageOfX` interface 会打断前端所有泛型
helper，所以从一个实例化里把类型参数替换出来生成 `Page<T>`，**其余每个实例化必须复现同一份 body，否则生成失败**。

**一条规则被这次落地推翻（重要）**：walker 原先按 Go 类型机械地把"required 且可为 nil"的字段发成 `T[] | null`
（nil slice marshal 成 null 是事实）。生成后前端报出 34 处 null 检查 —— 这说明**规则错了**：`required` 本身就是契约
（"字段在，且是这个类型的值"）；把 null 发出去等于把一个 runtime 缺陷**永久转嫁**成每个客户端的防御代码。改为：
required 字段不加 null，**违反由 validator 抓**（那才是坏帧该现形的地方）。同时把 `Page` 的构造收成
`NewPage` / `NewPageWithCursor` 两个都归一化 nil→`[]` 的构造器（7 处字面量构造中真有可能发 `null`，且与其他 list 方法
发 `[]` 不一致 —— 客户端得按方法分别处理"空"）。

**生成 TS 当场咬出 3 个真错**：
1. **`DiffRow` 是 union，却从未登记。** godoc 一直写着 `hunk → Text` / `context → LeftLine,RightLine,Code`，前端也一直
   按 union 建模，但 wire 上没有任何声明 —— published shape 允许一行同时带 hunk 的 text 和两个行号。已登记（union 数
   12 → 13），`leftLine`/`rightLine` 在各自 variant 里标 required（`omitempty` 是因为一个 flat struct 服务四个 tag，而
   unified diff 行号从 1 起，那个 0 值省略永不发生）。
2. **`WatchSpec.Path` 的 tag 与 canonical 文档相反。** AUX_API §3 写 `path?` 且明文「当前未用」（后端不递归监视，理由在
   §3），Go 却是 `json:"path"`（必发）。改成 `omitempty` 对齐文档 —— 这是 A1 那轮漏掉的一处 doc↔code 漂移。
3. **前端 `ResumeRunResponse` / `TodoItem` / `ScheduleInput` / `SubscribeWorkspaceRequest` / `WorkspaceQuery` 五个类型名
   与后端不一致**，且 10 个 re-export（`ItemBase` / `ApprovalPayload` / `ServerFeatures` / `JsonSchema` …）**零消费者**
   —— 前者改名对齐，后者删。`isDurableEvent` / `DURABLE_EVENT_TYPES` 同样零消费者，删；`STREAM_EVENT_TYPES` 换成生成的
   `WIRE_ENUMS.StreamEventType`（union type 在运行时不存在，所以 enum 的值集也生成一份数据面）。

**前端 branded id 的归属定案**：wire 类型用裸 `string`，brand 留在 `ids.ts` 并**在解析点**贴（`asRunId(...)`）——
这本来就是 `ids.ts` 自己写的设计（"At the wire boundary plain strings arrive from JSON.parse"），手写 shapes.ts 把 brand
放进 wire 接口反而与它矛盾。`RunId = string & {…}` 可赋给 `string`，所以只有 wire→app 方向需要贴，共 3 处 adapter。

**工具配置**：`knip.json` 的 ignore 从 `src/rpc/shapes.ts` 换成 `wire.generated.ts`；`.prettierignore` 加同一文件
（生成物的格式属于生成器）。

**最后两类的落地记录（勿重新推导）**：

**(a) authoritative/terminal runtime validators —— 两半都已落地**

上面这条路线已按定案实施：
- `FieldConstraintSpec` 进 shape registry（`ConstraintNonEmpty` / `ConstraintPositive` 两种；注册期校验字段存在**且类型
  匹配** —— 对非 string 声明 NonEmpty 直接 panic）；声明集在 `contract_values_wire.go`（41 个请求形状）。
- **Go validator 生成进 `protocol/request_constraints.generated.go`**（41 个 `Validate()`），手写那 30 个已删；
  `Validator` 接口 / `ConstraintError` / `required` / `positive` 三个 helper 留手写，`decode[In]` 一行未改。
  生成器多一个 `-validators` flag（`go generate` 的 cwd 是 directive 所在目录，不是 module root —— 踩过一次）。
- **schema 从同一声明发 `minLength: 1` / `minimum: 1`**（37 处）。嵌套约束（`artifact.session.id`）**不能**去改共享
  定义（那条规则属于这个 request、不属于每个携带 `ArtifactSession` 的类型），所以走 request 自己的 `allOf` 分支。
- **enum 成员检查是派生的，不是声明的**：`WireEnum` 已经声明了值集，所以生成器对每个 enum 字段自动发
  `oneOf(field, value, values, optional)`。这比手写那版**更严** —— 原先只有 `MemoryScope` 一处有检查，
  `RestoreType` 之类的坏 tag 会一路流到 use case；现在一律 `invalid_params`。手写的 `validMemoryScope` 随之删除。
- drift gate 与 gate 7 都已扩到生成物（gate 7 现在两个文件都查：手写 helper 与生成的 validator 都不许 import
  `/internal/`、`Validate()` 都不许带参数）。

**TS 半边（2026-07-29 落地）**：`wire.validate.generated.ts`（1987 行，241 个 shape）由 `cmd/contractgen/typescript_validator.go`
**从 schema 树编译**，与 TS types 同一原则 —— gate 6 因此是结构上成立，不是靠"记得同步"。

- **两侧 scope 不同不是偷懒，是各自拥有的帧不同**：runtime **收**请求，Go 的类型化 decode 已经定了结构，所以生成的 Go
  validator 只补类型说不出的那部分（值约束、enum 成员）；client **收** result / event，而 TS 类型在运行时**已被擦除**，
  所以这一份必须扛全树（type keyword / required / enum / minLength·minimum / union 排他 / presence rule）。
- **一个 keyword 一个原语**（`wireCheck.ts` 手写，与 Go 侧手写 `required` / `positive` / `oneOf` 同一分工）：
  `object` / `fields` / `array` / `record` / `text` / `integer` / `numeric` / `flag` / `minLength` / `minimum` /
  `enumOf` / `literal` / `absent` / `anything` / `ref` / `oneOf` / `allOf` / `ifThen`。**咬到的第一个真错就出自"图省事把
  两个 keyword 融进一个原语"**：最初写成 `text(1)`，于是 schema 里那 5 处**没有 type keyword 的裸 `minLength`**
  （嵌套约束走的 allOf 分支）无处可放，被静默编译成 `anything()` —— 约束整个丢了。改成一 keyword 一原语后，一条规则
  只有一种拼写，gate 6 才有东西可回读。`enum` 是唯一保留的融合（值集蕴含它旁边的 `type: string`，且**从不**单独出现）。
- **`fields` 与 `object` 的区别是刻意的**：union branch 与 presence rule 在 schema 里**没有** type keyword，按 JSON Schema
  `properties` 只对 object 生效 —— 所以它们编译成不断言类型的 `fields`，由外层定义断言一次。否则一个非 object 的帧会被
  13 个 variant 各报一次"类型不对"。
- **不加 schema 没说的规则**：`format` / `contentEncoding` 按默认词汇表当**注解**处理（时间戳格式由 Go decoder 拒）。
  加一条 RFC-3339 断言会让 validator **比已发布 schema 严**，那是与"不发 `additionalProperties: false`"对称的同一种说谎。
- **`oneOf` 的判定精确、报告是启发式**：0 个 variant 命中就附上"最接近的那个"的违规（`ContentBlock.mime must not be
  present here`），而不是只说一句"没有 variant 匹配"；命中 >1 个则是 union 排他性自己有毛病，直说。
- **当场咬出第二个真错**：`method.sessions.rollback.resp.json` 这份**手写** canonical sample 里，dropped run 带
  `status:"finished"` + `finishedAt` 却**没有 outcome** —— 而 `presentRun` 只要 `state != Running` 就一定填 outcome，
  `presentRunStatus` 又把每个非 Running 状态映射成 `finished`，所以这是一帧 runtime **产生不出来**的数据。
  Go round-trip 抓不到（`*RunOutcome` + `omitempty` 解得干净）、`tsc` 也抓不到（类型里 `outcome?`）—— **只有 presence
  rule 抓得到**。已补上 outcome。这正是 §11.3 要 validator 而不只要 types 的理由。
- 验证：13 条 per-keyword 行为测试（`wire.validate.test.ts`，正反各一半 —— sample 全是合法帧，只有非法帧才能证明规则真
  在生效），外加落地时一次性跑过**全部 78 个 canonical sample**（77 通过 + 上面那 1 个真错）。
- gate 1 顺手收窄成"环外产物按目录逐文件比对"，所以下一个落在 `src/rpc` 或 `protocol` 的生成物**不需要再改 gate**。

**(b) TypeScript method constants + typed client stubs（2026-07-29 落地）**

契约 §12 明文「wire 保持窄，SDK 提供高层句柄」，手写的 `src/rpc/methods.ts` **就是那个 SDK 层** —— 它的签名刻意比
wire 友好（`setEnabled(name, enabled)` 而不是 `setEnabled(params: SetMCPEnabledRequest)`），整份生成掉换不来任何契约收益。
所以生成的是 `wire.methods.generated.ts`：`WireMethodName` / `WireShapes` / `WireParams<M>` / `WireResult<M>` / `WireFeature`
/ `WIRE_CAPABILITY_POLICY`，SDK 在上面组合。

- **"一方法一函数"最终落成一个 `WireCall` 类型 seam，不是 83 个 wrapper**。契约要的效果是"每个调用点的 method 名与
  两个 shape 都被契约检查"，而 83 个只做转发的 closure 既做不到 `mutations.call` 那条路（幂等重放走另一个 helper），
  又多一层。`methods.ts` 里两行 `const call: WireCall` / `const mutate` 就把 83 个调用点全接上了：
  **46 个手写 result 类型和 83 个 method 名字面量从此是被检查的，不是被复述的**。method 名写错现在是编译错，
  以前是运行时 `method_not_found`。
- **Go 侧不生成方法名常量**（结论不变）：`server` 与 `dispatch` 同环、Registry 本身就是共读源。
- **8 个手写的 response 形状被删**：SDK 里 `{ mode: ApprovalMode }` / `{ hits: CodebaseHit[] }` /
  `{ runId; segmentId }` … 都是已发布 shape 的孪生体（`ApprovalModeResult` / `CodebaseSearchResult` /
  `StartRunResponse`）。换成正名后 `runs.subscribe` 的结果不再是"刚好够用的窄类型"，branded id 在
  `agentSessionRecovery` 的解析点贴（与 `runtimeRunsGateway` 同一规则）。
- **feature key 先收成一个作者**：原先有三份 —— dispatch 的 11 个私有常量、`server.go` 广告 map 里的 19 个字面量、
  前端 `RuntimeCapability` 的 19 个名字。features map 是**开放**的（§9 规定未出现的 key 读作 off），所以任何一处拼错
  都**静默**表现为"这个 build 不支持"。现在归 `protocol/features.go`（常量 + 可枚举表 + AST 完整性测试），
  并双向设闸：广告必须覆盖词表（`TestCapabilitiesAdvertiseThePublishedVocabulary`）、规则只能引用已发布 key
  （`TestCapabilityRulesNameAPublishedFeature`）、前端的联合类型删掉改用 owner 的名字。
- **咬出第三个真错**：`goals.get` 无 goal 时返 `null` —— API.md §7.14 写了、`GetGoal` 就是 `return nil, nil`、
  前端也一直写 `Goal | null`，而**每一份产物都只说 `Goal`**。可空性推导不出来（几乎每个 handler 都返指针，
  只有这一个 nil 可达），所以由方法自己声明 `ResultNullable`（注册期校验必须是指针）。
- **SDK preflight 落在 `rpc/preflight.ts`**（gate 3 第三条腿）：读生成的 policy，条件语义镜像 `capability.go`
  （dotted path / `present` 把空数组空串当缺席 / `equals` 比字符串）。**只在服务器已经说过"不行"时才本地拒**——
  没有 negotiated 快照（discovery 之前）一律放行，让 runtime 保持权威；反过来会把服务器支持的功能凭猜测拿掉。
  拒绝复用 `RpcError` + `capability_not_negotiated`（而不是新错误类），因为所有既有 `isErrorType` 分支必须照旧命中。
  snapshot 的唯一家还是 runtime 插件的 store，经 `SingletonPort.peek()` 取（port 未装 = 还没协商 = null，
  而不是抛 wiring 错——让 preflight try/catch 一个 wiring 错就会掩盖真的 wiring 错）。
- notification 名已生成（`NOTIFICATIONS_RUN_EVENT` / `NOTIFICATIONS_WORKSPACE_EVENT`），frontend 已改读。
- **`WIRE_STREAM_METHODS` / `WireEvent` 生成后又删掉**：knip 拒绝"导出了没人读"，而它们当下确实无 reader
  （transport 判流看 Content-Type，SDK 直接写 `RunEvent`）。要用时再生成 —— 这条比留着更符合 YAGNI。

**gate 10 的前置（写 TS validator 时看清楚了，比原先记的更小也更准）**：sample **文件**本来就只有一个家
（`app/desktop/frontend/src/rpc/samples/`，Go 的 golden test 跨模块读它）。真正抄了两遍的是 **file↔shape 映射** ——
Go 的 `wireSamples`（`file` → `func() any`）与 TS 的 `wire<T>(sample)` 各写一份，78 条各自维护。§11.3 要求 Go / TS
validator / JSON Schema **分别验证同一批 fixture**，"同一批"要有指代就得让这份映射只有一份。
落地形态：一份**人工编写**的索引（sample 文件名 → 已发布 shape 名），三方各按自己的方式读它 —— Go 按名字取
`reflect.Type`、TS 调 `validateWire(name, …)`、schema 侧解析 `schema.json#/$defs/<name>`。索引本身人工写才守住
§11.3「禁止生成 fixture 再用同源 schema 自证」；缺口检查（哪个 shape 还没有 sample）可以生成。

**接手须知（避免重新推导）**：
- `MethodMeta` 现带 `Params` / `Result` / `Event` 三个 `reflect.Type`（工厂填，`Result` 对 ack-only 方法为 nil、
  `Event` 只有 stream 有，`validate()` 已钉住这两条）。TS emitter 与 client stub 直接读它。
- `schemaSet` 的 defs 是 `map[string]*schema`（encoder 自带 key 排序 → diff 稳定）。刻意**没有**保留"首次到达顺序"
  —— TS emitter 要稳定顺序就对 map key 排序，别为它先留字段。
- **request 的值约束（非空串 / 正整数）目前只在手写 `request_constraints.go` 里，schema 侧没有对应的
  `minLength` / `minimum`** —— 这是 gate 6（三方约束等价）真正的缺口，也是 validator 那一类产物必须先解决的根：
  约束要**声明一次**，Go/TS/schema 三方都从声明生成。**不要**先在 walker 里猜"required string ⇒ minLength 1"
  （有合法的可空必填串），也不要保留手写 Go + 另写一份声明（两个源）。
- **canonical samples 必须保持人工编写**（§11.3）—— 生成器只做 sample 索引与缺口检查，禁止生成 fixture 再用同源
  schema 自证。现有 `delivery/protocol/wire_golden_test.go` 的样本是起点。
- ✅ `server.capabilitiesFor` 的 `StreamingMethods` 已收口 —— 直接读 `dispatch.Contract().StreamMethods()`。
  **结论修正**：Go 侧不需要"生成一份共读常量"。`server` 与 `dispatch` 同属 delivery 环、无 import cycle、arch 无禁令，
  所以 Registry **本身**就是 Go 的共读源；再生成一份 `const MethodRunsStart = "runs.start"` 而 dispatch 自己不消费，
  才是新的第二个源。第 5 类产物（method constants + typed client stubs）因此**只对 TypeScript 有意义**（TS 读不到
  Registry）。gate 2 的"discovery 与 dispatcher 相等"由此**构造上成立**。

#### ⚠️ gate 8 证据审计（2026-07-29，实现前先做完的功课）

`SystemInvariantSpec` 的 `Boundaries` 是复数且注释写明「多个 = 有多种破法，每种都必须守住」，所以 gate 8 的诚实形态是
**每个 (invariant, boundary) 对都要有一个跨 projection fixture**，不是"每个 invariant 有一个测试"。6 个 invariant 展开
成 13 对，逐对查过现有测试后：

| invariant | boundary | 证据 |
|---|---|---|
| session_has_at_most_one_open_run | runs.admission | ✅ `sqlite:TestRunAdmitEnforcesOneActivePerSession` |
| | runsegment.opening | ❌ **缺口** —— 需要：已 park 的 session 再来一次 `CommitOpening{Admit}` 必须失败且**什么都不写** |
| terminal_run_carries_its_result | runsegment.event | ⚠️ `runsegment:TestCommitEventRecordsGoalTurnWithTerminalRun` 只断言 run 落到 `terminal`，没断言 result 随之落盘；牙齿在 `sqlite:TestTerminalizeRejectsUnknownOutcome`，但那条是单表的，不算跨 projection |
| | runs.recovery | ✅ `sqlite:TestReconcileOrphansRepairsWholeDurableLifecycle` |
| | sessions.import | ⚠️ `Snapshot.Validate()` 确实拒 "state outcome mismatch" / "missing finished time"（`TestValidateSnapshotRejectsInconsistentPortableState`），但那是纯函数单测；**缺**一条走 import 写集、证明这类 artifact 进不来的 fixture |
| parked_run_has_exactly_one_open_interrupt_set | runsegment.event | ✅ `runsegment:TestCommitEventParkProducesBootResumableTriplet`（park → boot reconcile 不动它 → 再 admit 得 `ErrSessionBusy`） |
| | runs.recovery | ✅ `sqlite:TestReconcileOrphans{SweepsInterruptedWithoutRecord,RejectsPartialParkWithoutMutatingIt}` |
| dropped_run_leaves_nothing_behind | sessions.rollback | ✅ `bootstrap:TestApplyRollbackDropsRunsAndFreesAdmission` |
| | sessions.delete | ✅ `bootstrap:TestApplyDeleteRemovesRunRows` |
| imported_session_keeps_its_identity | sessions.import | ✅ `bootstrap:TestApplyRestoreClearsSessionOwnedProjections`（按给定 id restore，并清掉 session 私有投影） |
| goal_never_outlives_its_session | goals.lifecycle | ✅ `sqlite:TestGoalStore_CompareAndSwap` + `TestGoalStore_ClearThenRecreateRejectsStaleLease` |
| | sessions.delete | ✅ `bootstrap:TestApplyDeleteClearsSessionGoal` + `sqlite:TestGoalStoreCascadesWithSessionDeletion` |
| | sessions.rollback | ❌ **缺口** —— rollback **不删父 session**，所以它的 goal 合法地活着；这条在 rollback 边界的含义是「被丢掉的 subtask 子 session 的 goal 必须跟着走」，现有 `TestApplyRollbackDeletesSubtaskSetAtomically` 只证原子性，没证 goal |

**机制定案（勿重新推导）**：`internal/arch` 一张 (invariant, boundary) → fixture 的证据索引 + 四向校验：声明的每对都在索引里、
索引里没有多余对、被点名的 test 确实存在（AST）、且**该 test 的 godoc 里写了 invariant key**（双向可查，读 fixture 的人
也知道它是承重的）。**不采用**「fixture 里调 `Covers(t, key, boundary)` 运行时登记」：Go 的 test binary 按包分，跨包的
全局登记表根本汇不到一起，而要靠 AST 反查 `contract.BoundaryX` 这个标识符对应哪个字面量，又得再手写一张 name→value 表。

**实现结果（同日落地）**：先把功课落进文档再动手是对的 —— 上表里**有一处审计误判**：`goal_never_outlives_its_session`
@ `sessions.rollback` 其实早有证据，`TestApplyRollbackDropsRunsAndFreesAdmission` 本来就断言 goal 被清掉（我先读了
`applyRollback` 的 `withGoalMutation` 就以为 rollback 只 quiesce 不清）。真缺口是 3 条，都已补：

- `runsegment:TestCommitOpeningRefusesASecondOpenRun` —— 已 park 的 session 再 `CommitOpening{Admit}` 得 `ErrSessionBusy`，
  且**已经投影的 item 一条都不许留**（否则 session 的历史里会提到一个不存在的 run，而后续没有任何写会补上它）。
- `runsegment:TestCommitEventPersistsTheTerminalRunsResult` —— 通过 `ListRuns` **回读**终态 run 的 outcome / result /
  detail / finishedAt。原先最接近的那条只断言 `runs.state = 'terminal'`，那只能证明状态列翻了，证不了"解释它为何结束的事实
  也落盘了"。写这条时顺手撞到 store 的既有牙齿：`OutcomeError` 的 run 必须带 `Result.Error`，否则 terminalize 直接拒。
- `server:TestSessionImportRejectsATerminalRunWithoutItsResult` —— 走 **use case**（不是 `Snapshot.Validate()` 纯函数）
  拒掉"自称终态却什么都没带"的 artifact，并断言 session 没被留下半个。

gate 的两条腿都验过会响：改错 fixture 名 → "named as the fixture ... and does not exist"；漏写 godoc → 一次点出 10 条。

#### ⚠️ gate 11 证据审计（2026-07-29）

契约要求 list query fixture 覆盖三条腿：**固定排序**、**scope/filter cursor binding**、**下一页方向**。全仓 seek 分页
只有一个实现（`component/keyset`：`Encode` / `Decode` / `Limit` / `PageOf`，cursor 里绑 method 名 + filter 指纹），
消费它的分页读共 5 个：

| 分页读 | cursor 身份常量 | 固定排序 | filter binding | 下一页方向 |
|---|---|---|---|---|
| `items.list` | `itemPageMethod` | ✅ `queries:TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor` | ✅ `queries:TestListItemPageRefusesAForeignCursor` | ✅ 同左 |
| `runs.list` | `runPageMethod` | ✅ `sqlite:TestListRunningOrdersByAdmission` | ❌ | ✅ `sqlite:TestListRunningSeeksPastItsAnchor` |
| open interrupts | `interruptPageMethod` | ❌ | ❌ | ❌ |
| `sessions.list` | `viewPageMethod` | ❌ | ❌ | ❌ |
| `schedules.list` | `listPageMethod` | ❌ | ❌ | ❌ |

组件层三条腿都齐（`keyset:TestCursorFromAnotherQueryIsRejected` / `TestFilterBoundariesAreNotInterchangeable` /
`TestPageOfSignalsMoreOnlyWhenTheOverFetchProvesIt` / `TestLimitClampsToTheReadsCeiling`），所以缺的不是机制，是
**每个读各自接对了没有** —— 而这正是 gate 11 存在的理由：`keyset` 再对，某个读传错自己的 method 名或漏传 filter，
它的 cursor 就会接受别处铸的 anchor，组件测试一个都看不见。

**当场咬到一个命名分歧（待用户定）**：open interrupts 的 cursor 身份是 `interruptPageMethod = "interrupts.list"`，
而已发布的方法叫 `runs.listOpenInterrupts` —— **没有任何方法叫 `interrupts.list`**。功能上不算 bug（这是内部命名空间，
不上 wire），但它是"同一件事的第二套词汇"：cursor 的 method 绑定本该就是已发布的方法名，否则 gate 11 想核"每个读绑的是
自己那个方法"时，得先知道有一处例外。**倾向**：改成 `runs.listOpenInterrupts`（破坏性只限于"旧 cursor 失效"，
dev 阶段无迁移负担），并让 gate 顺手核对每个身份常量都是**注册过的方法名** —— 那条校验现在就能加，和 `StateKeySpec`
的 `RecoveryMethod` 是同一招。

**实现结果（同日落地，按上面的顺序做完）**：

1. **cursor 命名统一** —— `interruptPageMethod` 改成 `runs.listOpenInterrupts`。这**不是**破坏性公开 API 改动：常量是内部的，
   cursor 对客户端是不透明 token，代价只有"已铸 cursor 失效"，而 cursor 本来就是 per-query 短命的。
2. **9 格 fixture 补齐**，每格都带反例。四个分页 fake（runs / interrupts / sessions / schedules）改成**按 store 的方式
   seek**：`(0, "")` 是"没有 anchor"= 第一页（与 sqlite 的 `if afterStartedAt > 0 || afterRunID != ""` 一致），其余严格大于
   上一页最后一行。原先的 fake 直接忽略 anchor —— 那种 fake 下"下一页方向"根本无从断言，读怎么写测试都过。
   关键反例：**把 A 读的 cursor 喂给 B 读必须被拒** —— run 页与 interrupt 页 scope 相同、都按时间戳排序，**只有 query
   命名空间能把它们分开**，所以两个方向都试了。
3. **静态校验**：`*PageMethod` 常量必须是已注册方法名 **且互不相同**。后半条是审计时没想到的 —— 两个读共用命名空间，
   彼此的 anchor 会被对方接受，然后 seek 落到错的排序上，而组件测试与产物比对都看不见。

`keyset` 那 4 条组件测试仍是组件层的证明；gate 11 证明的是**每个读接对了**。

#### ⚠️ B2 实施记录（2026-07-29）：spec **现在**就用反射自校验，不等生成器

B3 的生成器还没有，但一份说谎的 spec 现在就该测出来。所以 `UnionSpec` / `ObjectConstraintSpec` / `StateKeySpec` 在
注册期按 Go 类型校验（dotted path 走 embedded 内联），init 时 panic：

- 变体声明了结构体没有的字段 → 拒；
- **结构体有字段却没有任何变体认领 → 拒**（这条是真会发生的漂移：加个字段忘了登记，生成的 schema 就会让它在每个 tag 下合法）；
- discriminator 不是 `type` → 拒（API.md §2.1 无例外）；tag 重复 → 拒；
- state key 的 `RecoveryMethod` 必须是**已注册方法** → 否则等于承诺一个客户端调不出来的调用。

**当场咬到一个**：`Interrupt` 的变体字段在 `payload` 里（`payload.tool`），覆盖记账没把 `payload` 本身算进去 —— 校验器
自己第一次运行就报了这个，说明它在干活。

**13 类 union 只登记了今天存在的 12 个**（含 §11.2 未点名但同样闭合的 `ContentBlock` / `QuestionField` /
`ArtifactContentBlock`）。`SegmentOutcome` / `ItemListScope` / `CapabilityRequirement` / `CancelRunResponse`
**类型还不存在**（C1 / C5 / C6 才产生）—— 为一个谁都发不出来的类型登记 shape，校验不了任何东西，是纯占位。
`TestEveryClosedWireUnionIsRegistered` 用显式清单钉住"今天该有哪 12 个"，C 里加类型时这条测试会先红。

**D3 落地形态**：`internal/application/contract` 包 —— `SystemInvariantSpec` + 6 条已声明不变量 + 8 个具名事务边界。
它**不执行**任何校验：契约 §11.2 明文禁止让 `Validate()` 拿到 repository dependency，所以这里只做"给不变量稳定命名 +
声明谁负责维护"，验证归 B4 gate 8 的 integration fixture。新增守卫
**`TestSystemInvariantsStayInApplication`**（§4.4 第 4 条）：禁 `delivery` 出现 `SystemInvariantSpec` / `TransactionBoundary`，
并顺带断言声明集本身可执行（无重名、每条都有 Why 与责任边界）。arch 守卫 41 → 42。

**§4.4 六个守卫现状**：4 个 DONE（`DoesNotAuthorDomainText` / `DoesNotImplementQuerySemantics` /
`ReadsRunsFromDurableProjection` / `SystemInvariantsStayInApplication`），剩 2 个（`DoesNotComputeMetrics` /
`RunWireShapesShareOneDefinition`）按原计划是 **C 预埋** —— 它们要禁的符号在 C1/C2 才产生。

#### ⚠️ B1 实施记录（2026-07-29）：Registry 落在 `delivery/dispatch`，不在 `protocol`

契约 §11.1 的示意把 `Registry` 画在 wire 类型旁边。**实施时判断放 `dispatch`**，理由是抽象归属而非便利：

- Registry 的内容与它驱动的路由不可分 —— `Method` 持有 decode/invoke/encode 闭包，那是 dispatch 的本职。把 metadata
  放 `protocol`、Registry 放 `dispatch`，等于把**一个概念劈成两个包**，且「唯一注册点」会变成"metadata 一处、绑定另一处"。
- 计划 §2.2（D3）本就写「生成器…**读 delivery 的 method/union spec**」，未指定 `protocol`。
- `TestProtocolStaysWireOnly` 不受影响（dispatch 可 import protocol，反向不需要）。

**注册点用方法表达式**（`func(*Dispatcher, ctx, P) (R, error)`）而非绑定方法值：`contract` 因此是 package-level 值，
**不需要 Runtime 实例就能构建**。这正是 B3 生成器要的 —— 读全量 metadata + reflect 类型，无需 stub 一个 83 方法的 Runtime。

**消掉的重复：4 张按方法名索引的表 → 1 处注册。**

| 删掉的 | 规模 |
|---|---|
| `method_names.go` 方法名常量 | 85 |
| `method_table.go` handler 映射 | 83 |
| `idempotency.go` 的 `replayProtectedMethods` | 32 |
| `handlers_*.go` 里 `if in.X == ""` 必填校验 | 44（前一 commit 已搬到请求形状） |
| `server/*.go` 里 `if !s.features.X` 能力检查 | 20 |

**能力门禁读 discovery 自己的输出**（`d.api.Discover(ctx)`），不另开一条 feature 通道。契约 §11.4 gate 3 要求
「dispatcher / discovery / SDK preflight 三方等价」—— 让 enforcement 读 advertisement 使二者**构造上等价**，而不是靠一条
"记得同步"的测试。代价是每次被门控的调用多构造一个 19 项 map；被门控的方法都是低频面板调用，未测量前不优化（Pike 2/3）。

**实施中查出并修掉的 3 个真错（都不是"移动代码"）：**

1. **`codebase` 把"能力未装配"和"未配 embedding role"混成一个答案。** 协调器 `c.index == nil` 时返回
   `ErrNoEmbeddingModel`，而 API.md §7.10 明文区分：未装配 → `capability_not_negotiated`，装配但未配置 → `invalid_params`
   （后者是用户可修的）。新增 `codebase.ErrUnavailable` 分开；`codebase.status` 原先对未装配的 runtime 报
   `state:"none"`，读起来像"装配好了但索引是空的"，会诱导客户端给一个不存在的能力显示"开始构建"按钮 —— 这条分支
   在 server 的 feature 检查后面本来就不可达，是死行为。
2. **`memory` 的 `ErrMemoryUnavailable` 没有 wire 映射。** 之前被 server 的 feature 检查挡住看不见；检查一移走就以裸
   sentinel 露出去。补进 `wireWorkspaceError`（与 agentMemory / schedules / skills 同一形态）。
3. **`goals.Driver` 未装配时是 nil，公开方法不自守 → 直接调用 panic。** 之前靠 delivery 的 feature 检查兜住，也就是说
   「不 panic」这件事依赖上层记得检查。改为 driver 自己回答 `ErrUnavailable`（与 `agentmemory.Coordinator` 已有的
   `Available()` 同一形态），delivery 不再为它守卫。

**跟着搬家的 3 个测试**：`TestAgentMemoryHandlersDisabled` / codebase 的 query 必填断言 / memory 的 disabled 断言 ——
它们断言的规则已经不属于 `server` 了。前者移入 `dispatch` 的 gate 测试（覆盖无条件 + 三种条件形态），后两者由请求形状
的 `Validate()` 与新 sentinel 覆盖。**没有降低覆盖**：gate 测试比原先的 20 个 per-method 检查更严（含"空 watches 不算 watch"
与 rollback 三值）。

**一处按契约修正了行为**：`restoreType: files|both` 在 `features.checkpoints=false` 时，从 `checkpoint_unavailable`
改为 `capability_not_negotiated`。AUX_API §4.1 本就把二者分开（能力关 vs 该 run 无快照），旧代码把两个事实压成一个错误码。
wire **shape** 未变。

**未做（留 B3）**：`server.capabilitiesFor` 的 `StreamingMethods` 仍是手写 4 项。Registry 已有 `StreamMethods()` 作为唯一
真相，但 `server` 是 Runtime 实现、不该 import dispatcher；正确的收口是 B3 生成一份供二者读的常量。本轮先用
`TestStreamMethodsAreTheStreamingContract` 钉住 Registry 侧，B4 gate 2 再校验两侧相等。

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
| B′1 | bump internal store schema epoch；遇旧 epoch **确定性拒绝启动并提示重建**，不后台静默删用户文件；不 `ALTER TABLE`、不读旧行、不伪造零值、不留 compatibility view / dual-read / dual-write | `DONE` | `ca24bb3b4` |
| B′2+B′3 | **合并**：`runs` 成为完整 Run 行（补列 + 一个 owner）、`history_runs` 删表、全部消费方改读同一行 | `DONE` | 见下 |
| B′4 | application query ports（下表逐项核实） | `DONE` / 余项 `DEFERRED → C` | A′3 / A′4 / B′2 |
| B′5 | todos session-scoped projection | `DEFERRED → C7` | — |

**Batch B′ DoD**：fresh schema 无 `history_runs`；旧 schema fixture 被明确拒绝；代码库无旧 Run payload decoder、无零值回填、无兼容读路径；全绿；**未对外声称支持 vNext**。

#### ⚠️ B′1 范围修正 —— epoch bump 不在 B′1（2026-07-28 实施中）

B′1 原文含「bump epoch」。**bump 与「拒绝旧 epoch」是两件事**：拒绝是**策略**（silent DROP 掉用户全部表 → 命名文件、拒绝启动、由用户决定删不删），bump 是**形态变更的后果**。在 B′1 里 bump 一次形态没变的 epoch，只会白白让所有本地库被拒一次。所以 B′1 只落策略，epoch 34 随真正改形态的 B′2 一起落。
另有一处**豁免**：完全没有表的空文件不算 mismatch（否则每次首启都失败）—— 这是「没有任何东西会丢」的唯一情形。

#### ⚠️ B′2 与 B′3 合并（2026-07-28 实施中，**已 DONE**）

计划把「补列」和「删旧表」分成两个 slice。**在「不双读双写」纪律下这两步无法分开**：
- 只补列不切读写 = 一批没有 writer 的死列；
- 只删表不搬走 owner = 编译不过。
中间态只能靠 dual-write 撑住，而那正是本批禁止的东西。于是合并成一个 commit，claim 是一句话：**一个 Run 一行，一张表一个 owner。**

**§3.3 的 12 列只补了今天有 writer 的那些。** `parent_run_id / root_run_id / active_segment_id / max_steps / max_budget_usd / protocol_profile` 全部无 writer（D2 子 Run、vNext 配置面才产生），现在加就是空列 —— 按 `feedback_yagni_speculative_headroom` 留到 C 里它们的 writer 落地时再加。实际补的是：`spawned_by_item_id / detail / steps / active_duration_ns / usage / problem / message_mark / finished_at`。
**一处刻意偏离计划命名**：`active_duration_ms` → **`active_duration_ns`**。本表其余时间列全是 UnixNano，用 ms 会在 store 层丢精度去迁就一个正在被删的 wire 字段（§3.2 删 `durationMs`）。

**实施中发现的、计划没有的东西：**

1. **`Run.Interrupts` 是第三份真相**，且 `run_recovery_validation.go` 有一个 `reflect.DeepEqual(run.Interrupts, pending.Interrupts)` —— 一段专门用来检查两份拷贝是否一致的代码，正是病征本身。新表**不存** interrupts：`interrupts` 表是唯一 owner，读时 join 补上；parked 行 join 不到就是坏 park（报错，不是「等待空集合」）。同理删掉的还有 `validateParkedTranscript` 里 model/createdAt 的互校 —— 它们校的是同一行。
2. **rollback 会 terminalize 一个它随即删掉的 run**。合表后这是不可表达的（`state=terminal` + 无 result 违反行不变量），且本来就多余：**删行本身就是释放 admission slot**（`DeleteForSession` 的注释早就这么说）。`RollbackPlan.Terminate` / `.RunID` 因此成为死字段，一并删。
3. **`ApplyRestore` 从来不写 admission 行** —— 导入一个 session 后 `runs` 表对它一无所知。旧 schema 下不可见（导入的 run 全是终态），合表后会直接丢掉整段历史。新增 `RunStore.Restore`（只收终态行，非终态一律拒 —— 那会把 admission slot 交给一个不存在的 executor）。
4. **`Admit` 把「run id 撞车」报成了 `ErrSessionBusy`** —— 两个约束意思相反（PK 说 id 被占，partial index 说 session 被占）。SQLite 的扩展码本就区分（1555 vs 2067），`isUniqueViolation` 只匹配后者，所以撞车过去是漏成裸错误。现在分别映射到 `ErrIdentityConflict` / `ErrSessionBusy` —— 后者会让调用方去等一个根本不存在的 run 结束。
5. **`Admit` 允许零 CreatedAt** → `started_at` 落成一个 1754 年的荒谬值，而它现在是所有 Run 读面的排序键。改为 fail-fast。
6. **非终态 Run 的 durable 投影整个是多余的**。SegmentStarted 的 Run 除 `SpawnedByItemID`（今天恒空）外没有一个字段是 admission 不知道的；park 的 Run 也一样（state 归 Suspend、interrupts 归 interrupts 表）。于是 `PutRun` 整个方法删掉，**Run 行只由生命周期转换写**：`Admit` / `Suspend` / `Resume` / `Terminalize(run)` / `RecoverLost(run)` / `Restore(run)` / `Delete`。终态状态与解释它的 result **在同一条 UPDATE 里**落地 —— 行不可能声称终态却没有 result，也不可能一边 running 一边带着 result。
   连带：opening 的 durable 投影**就是** admission/resume 本身，所以「opening 必须有 durable projection」这条检查（会让 approval-only 的 resume 假失败）撤掉；「一个 batch 至多一次生命周期事件」的不变量从「有没有 Run commit」改成直接查事件本身（原来靠 commit 间接查，投影删掉后会失守）。
7. **`transcript.Run.Validate()`** 新增 —— 终态事实与 state 的等价关系（终态 ⟺ 有 outcome/result/finishedAt/messageMark）此前只写在 `Snapshot.validateRuns` 里；store 编解码要同一条规则，复述一遍就是两个作者。现在 domain 一处定义，store 写入前校验、`Snapshot` 委托它。这也是「行不需要 `has_result` 标志位」的依据：state 就是答案。
8. `UnknownMessageMark = -1` 命名了原先散在 5 处的字面量（含 schema 默认值注释）。

**未加 arch 守卫**：本 slice 的规则由 API 形态而非命名强制 —— 没有 `PutRun` 可调，加回来是显眼的 review 事件；不为一条 SQL 字符串写守卫。

#### ⚠️ B′4 逐项核实（2026-07-29）：已完成 + 余项 **DEFERRED → C（附因）**

| B′4 原文项 | 判定 |
|---|---|
| Run query 读 durable projection | ✅ **DONE**（A′3 `runs.list` 改读 admission 记录；B′2 `ListRuns` 读同一行）+ 守卫 `TestDeliveryReadsRunsFromDurableProjection` |
| Item query 按 Session、durable `sequence` 唯一排序键 | ✅ **DONE**（A′4 真 keyset over `history_items.seq`） |
| 过滤 + 排序 + cursor 编解码三件事全在 port，delivery 一件不留 | ✅ **DONE** + 守卫 `TestDeliveryDoesNotImplementQuerySemantics` |
| `interrupts.list` keyset、**一个 set 不跨页** | ✅ **DONE**：A′4 落 `(created_at, run_id)` keyset；「不拆散 set」已是结构事实 —— 一行持有整个 set，分页单位就是行 |
| Item query **按 Run subtree** + `run_id/sequence` 与 lineage 索引 | ⏸ **DEFERRED → C**：`items.list` 今天没有 `runId` 入参，subtree 也需要子 Run（D2 本轮不做）。**没有消费方的索引与读面 = 死码。** 唯一今天会用到 `run_id` 的查询是 `DeleteRun`，n = 单 session 的 item 数，未测量前不优化（Pike 规则 2/3） |
| `interrupts` 改按 `root_run_id` root-owned aggregate、每个 Interrupt 持久化来源 `run_id` | ⏸ **DEFERRED → C**：**没有 producer** —— 来源 `run_id` 与 root 的区别只在子 Run 存在时才产生（D2）。今天一 Run 一行、来源恒等于自己 |
| `waiting` 停止折叠 | ⏸ **DEFERRED → C**：今天 `SessionState` 就是 running / waiting / idle 三值，与「执行中 / 停在 HITL / 空闲」**一对一**，没有信息被折掉。vNext 的 state 扩容是 wire enum 变更 |

#### ⚠️ B′5 判定（2026-07-29）：**DEFERRED → C7**，逐条附因

todos 今天只以**流内 ephemeral `state.snapshot`** 形态上 wire：没有 `todos.get`（计划本就把它留到 C7），没有任何 durable 读面。于是 B′5 的四项全部没有今天的消费方：

- **补 `revision` + 单调分配** —— 唯一用途是 vNext `state.changed` 的单调合并（C15 reducer）。现在加 = 一列没有读者的列，与 §3.3 那 6 列同一个判断。
- **root-only write policy** —— 今天一个 session 同时只有一个 Run（子 Run 未做），每个 writer 本来就是 root，**这条规则今天没有可违反的路径**。
- **按 history boundary 的 lifecycle substrate** —— 今天 rollback 直接清空 todo 投影。在 todos 仍是 per-run 临时工作清单（无 durable 读面）的前提下，清空是自洽的；改成按边界恢复必须和 `todos.get` 一起设计，否则是给一个看不见的东西做版本管理。
- **只读 query/presenter** —— 即 `todos.get`，计划已排 C7。

**结论：Batch B′ 的 DoD 已达成**（fresh schema 无 `history_runs`、旧 epoch 明确拒绝、无旧 payload decoder / 零值回填 / 兼容读路径、全绿、未对外声称 vNext）。B′4/B′5 的余项不是「跳过」，是**它们的 writer 与 consumer 都在 C**，在 C 里和各自的 wire 面一起落地才有验证手段。

---

### Batch C —— vNext 原子切换

> **一个 release、一条 cutover 分支。** 分支内按依赖顺序做小而可审查、每个全绿的栈式提交；**只有整体合并时才切版本**，所以 main 上不存在半套 vNext。分支可整体 revert。
>
> **依赖**：B 与 B′ 都完成。

| slice | scope | 状态 | commit |
|---|---|---|---|
| C1 | `RunStatus` 三态；`RunOutcome`/`SegmentOutcome` 拆分；删 `RunResult`+`durationMs`；`RunMetrics`/`RunLimits`（+ Artifact run 同步拆，契约 §10 要求同时改）| `DONE` | `5e3217241` |
| C2 | `RunSummary` base + `RunRef` 嵌入（同一定义生成三方，**TS 不做语法继承**）+ `activeSegmentId` durable + 约束随嵌入继承 | `DONE` | `688dfc17f` |
| C3 | `RunProtocolProfile` durable 不可变 + `features.subagents` gate（广告 `false`）+ Minimal Profile + capability preflight | `DONE` | `6b7d76d2c` + `baa7a304d` |
| C4 | `runs.get` 新方法 + `runs.list` 全历史/status/`includeDescendants` + cursor 绑定规范化 filters | `DONE` | `843c9437c` |
| C5 | `ItemListScope` 闭合联合替换扁平请求 + `items.list.order` 双向 + 页级 `RunSummary[]` | `DONE` | `73519e301` |
| C6 | `Interrupt.runId` 必填 + payload 三 variant + `OpenInterrupt`→`PendingInterruptSet{rootRunId}` + `interrupts.list` 取代 `runs.listOpenInterrupts`（aggregate 分页，set 不跨页） | `DONE` | `b5a2c7773` + `07e703eaf` |
| C7 | `todos.get` + typed `state.snapshot` + `revision` reducer + **Segment final snapshot fence**（§5.6 第 1 点） | `DONE` | `a92d36d8e` |
| C8 | `session_has_active_run` typed conflict（带 `activeRun`）+ 无隐式 cancel 的 start 语义 | `DONE` | `966bef2b7` |
| C9 | `runs.resume.input` 原子 user Item + 条件必填 `userItemId`（iff 关系登记为 method contract fixture） | `DONE` | `d829cef7c` |
| C10 | `ProblemData` 删 `channel`/`retryable`、加 `activeRun`/`requiredCapabilities` + `RecoveryAction` 闭合枚举 + Error Registry | `DONE` | `7fd71658c` |
| C11 | `ServerCapabilities` 新 shape（`runEvents`/`runtimeTopics`/`stateSnapshots`/`FeatureCapability` 两新字段/`limits` 两新块） | `DONE` | `a9bd0f1fa` |
| C12 | 九值闭合 `RuntimeTopic` + `runtime.subscribe` 取代 `workspace.subscribe`；删 `mcp.serverChanged` payload 真相、`schedules.fired` | `DONE` | `a9bd0f1fa` |
| C13 | cursor/replay 定稿（`processEpoch`、`headEventId` opaque、retention 走 discover）+ `runs.subscribe/steer` 认领 segment | `DONE` | `361668817` |
| C14 | `sessions.rollback/export/import` capability 规则（已在）+ Artifact v7（`states`/child edges/root-only profile/`DroppedRun`→RunSummary） | `DONE` | `86c79a21f` |
| C14b | **session-scoped state 的另外两半生命周期**：`fork` 复制 fork boundary 的值、`history/both` rollback **恢复到目标 boundary**（原先是清空）。per-run-boundary 快照（`todo_boundaries` + FK cascade，terminal CAS 内一处 stamp，epoch 39→40）—— 见下「C14b 为什么单独一片」 | `DONE` | `463910d21` |
| C15 | 前端 fold + **它依赖的后端生产者**：九 topic 生产者（原先只有四个）+ hub 不再丢帧（合并成 resync）；前端 exhaustive reducer、删 business numeric code 镜像、订满九 topic、删 goal 4 秒轮询、state 冷读（`todos.get`）、run stream 断线按 `Last-Event-Id` 重接 | `DONE` | `cad9f7069` / `57d2cf9ed` / `c58906cbf` |
| C16 | 切版本：`protocolVersion = minSupported = "2026-07-27"`、`SessionArtifactVersion = 7`；旧协议 / 旧 Artifact 确定性拒绝；三份 canonical 文档按 vNext 改写（删掉被生成物重述的目录）+ gate 15/16/17/18 | `DONE` | 见下 |

#### ⚠️ Batch C 接手须知（2026-07-29，B 收口时量过的，别重新踩）

**"分支可整体 revert" 指合 main 是一次原子切换，不是必须一坐做完 16 个。** 在 cutover 分支上逐个 stacked commit、
每个自己全绿（Go 五关 + 前端 `npm run check`），才是计划描述的做法。任何一个 slice 没绿就**不要 commit** ——
main 不动，损失只是当次未提交的工作。

**C1 的真实爆炸半径（量过）**：Go 侧 24 个文件提到 `RunResult`、51 个提到 interrupt 相关；前端 26 个文件提到
`RunOutcome`/`RunResult`/`durationMs`（含 fold reducer 的 6 个测试文件、`agentStore`、observability sink/stores、
`sdk/types/{agentView,events}`）。它**不只是 wire 类型改名**：

- `transcript.Run.Result` 是 **domain** 类型，`RunMetrics`/`RunLimits` 的正交化要穿到 domain；
- sqlite `runs` 表的 `steps` / `active_duration_ns` / `usage` / `problem` 四列就是今天的 `RunResult` 投影 ——
  按 B′ 纪律**丢库重建 + schema epoch 守卫**，不写 migration；
- `status:"waiting"` 是**新的第三态**。今天 `presentRunStatus` 把每个非 `Running` 状态都映射成 `finished`，
  所以 parked run 现在是"finished + interrupt outcome"。C1 之后 parked run 是 `waiting`、**没有 outcome**，
  而 `interrupt` 从 `RunOutcome` 移到 `SegmentOutcome` —— 这同时推翻 B2 给 `RunRef` 登记的 presence rule
  （`status == finished ⇒ outcome + finishedAt`），必须**同一个 commit 改掉**，否则注册期校验或 gate 6 会红。

**B 建的闸会在 C 里咬人，这是设计如此 —— 每条都有明确的同步动作：**

| 闸 | 什么时候咬 | 同一 commit 里要做什么 |
|---|---|---|
| `TestPageCursorsBindToTheirOwnMethod` | **C6** 用 `interrupts.list` 取代 `runs.listOpenInterrupts` | 改 `interruptPageMethod` 常量（现在正是 `runs.listOpenInterrupts`）—— 闸要求它必须是**已注册方法名**且唯一 |
| 三方 canonical sample gate | **每个**改 wire shape 的 slice | 改 `protocol/canonical_samples.go` 的 binding + 手写 sample JSON；ajv 读的是**已发布 schema**，所以样本必须真能过 |
| `TestEveryWireStructIsPublished` | 新增 exported wire struct（C1 的 `RunMetrics`/`RunLimits`、C2 的 `RunSummary` …） | 要么被某方法可达，要么登记 carried shape / 列入 `notOnTheWire` 并写理由 |
| `TestEveryClosedWireUnionIsRegistered` | C1 拆 `SegmentOutcome`、C6 拆 payload 三 variant | 登记新 `UnionSpec`；**结构体有字段没被任何 variant 认领 → 拒**（这条真会咬） |
| gate 8 证据索引 | C1 改 `terminal_run_carries_its_result` 的形状 | fixture 的 godoc 写着 invariant key，改测试时别把那行删掉；索引在 `internal/arch/invariant_coverage_test.go` |
| `TestEveryStateKeyHasAShapeFixture` | **C7** 加 `todos.get` + typed `state.snapshot` | `StateKeySpec.RecoveryMethod` 从 `runs.subscribe` 改成 `todos.get`（`contract_shapes_wire.go` 的注释已预告这一行会变），payload fixture 同步 |
| `TestProtocolVersionAgreesEverywhere` | **C16** | 只改一处常量，这条会点名每份还写旧版本的 canonical 文档 |
| gate 15/16/17/18 | C 的产出**就是**它们的输入 | 16/17 的 compatibility diff 需要一个 **baseline 产物快照**才有意义 —— ✅ 基线已在 C 开工前取：`internal/arch/testdata/baseline/{manifest,schema}.json`（切换前 `contract/` 的逐字节副本，`protocol.current = 2026-07-19`）。**只存这两份**：它们分别是方法/能力/错误面与 shape 面的权威投影，`openrpc.json` 是二者的再表述，diff 读它就多了一套说法。基线不可事后重建，所以先取；两条 gate 本身在 C16 落（版本翻转前"breaking 但没 bump"必然红，那是分支中段的真实状态，不是缺陷） |

**C2 的"同一嵌入定义生成三方" —— ✅ 已裁决（2026-07-29，做 C2 时）：TS 不做语法继承。**

Go 嵌入 `RunSummary` 到 `RunRef`，`protocol.WireFields` 按 `encoding/json` 语义把 embedded 字段内联，schema/TS
拿到扁平 `RunRef`。**不让 TS emitter 产生 `extends`**，三条理由：

1. 契约要求的是**一个定义**（"禁止手工复制两套同名字段"），不是一种语法；嵌入已经满足。
2. TS 的结构化类型已经给了客户端 `extends` 想给的可替换性 —— 扁平 `RunRef` 本来就能传给要 `RunSummary` 的地方。
3. 让 emitter 按"字段集是某已发布 def 的超集"推断继承，就是**从形状反推意图**：两个无关 shape 恰好成超集关系时
   会静默获得一条继承边，那是published contract 里的一句假话 —— 换了衣服的"第二真相源"。而**声明**一条继承关系，
   又是把 Go 嵌入已经说过的事再说一遍。

**`RunSummary` 的 root/child lineage 约束怎么落 —— ✅ 已裁决**：登记为**三条 `OperatorPresent` 规则**（三个 child
edge 任一存在则另两个必需 = all-or-none），这是 schema 能表达的部分。契约 §4.2 括号里的"两个 RunId 均不等于 `id`"
**不是 frame constraint**：JSON Schema 2020-12 无 `$data`，**表达不了字段间不等式**，而 §11.2 要求这些约束生成
"JSON Schema `if/then` 与等价 `Validate()`" —— 没有 schema 表达就谈不上等价。它的正确归属是 **child 创建事务的
identity 不变量**（同 `goal_never_outlives_its_session` 那一类）；D2 下本轮没有 child 创建事务，现在登记就是给一个
不存在的边界写 spec。**做子 Run 时把它登记进 `SystemInvariantSpec`，别塞进 `PresenceRule`** —— 把不等式融进
presence 原语正是"一 keyword 一原语"禁的那种融合。

#### C3 接手须知（2026-07-29，做完 C2 后核过现状）

**C3 与 C10/C11 真实纠缠** —— 不是能纯按 slice 边界切的。核过的现状与结论：

| C3 要的东西 | 现状（`delivery/protocol/capabilities.go`） | 归谁 |
|---|---|---|
| `FeatureCapability` 加 `clientOptIn`/`requiredByRunProtocol` | 现在只有 `enabled`+`stability` | **C3 必须做**：`requiredFeatures` 就是从 `requiredByRunProtocol` 的已协商 key 算出来的，没有它算不出 profile |
| `ServerCapabilities` 其余新字段（`runEvents`/`runtimeTopics`/`stateSnapshots`/两个 limits 块） | 现在是 `events`/`streamingMethods`/`features`/`limits` | **C11**。C3 只动 `features` 的值类型，别顺手把 `events`→`runEvents` 一起改 |
| `ClientCapabilities` 删 `events`、`excludedEvents`→`excludedEphemeralEvents` | 现有 `Events`（必填）+ `ExcludedEvents` | **C3 做**（§8.1 明确删 `events`，且它是 profile 协商的输入）。注意前端 `request.meta.json` 样本与 `samples.test.ts` 里"只广告已发布 stream event"那条断言会一起动 |
| `ProblemData.requiredCapabilities` | 无 | **C10**。C3 先返 `capability_not_negotiated`（错误码已在），payload 字段到 C10 统一改 |

**`RunProtocolProfile` 的落点（已核）**：
- 它是 **root Run 创建时冻结的 durable 值**：`runs` 表一列 JSON（整读整写、从不跨行查，同 `accounting` 的判断），domain 上 `transcript.Run.ProtocolProfile`，由 `RunDraft` 带进来。
- `interruptTypes` 今天已有等价物在流转：`StartCommand.InterruptKinds` → `TurnControl.Resume(..., interruptKinds)`。**它现在是 per-request、没落库** —— C3 做的正是"冻结成 Run 的属性"，别新造平行通道。
- `requiredFeatures` 本轮恒为 `[]`（`subagents` 广告 `enabled:false`，client 协商不上）。**空数组是合法且明确的 Minimal Profile，不是"没设"** —— 字段非 optional，必须序列化成 `[]`。
- 不可变性检查的正确位置：`runs.resume` 不接受新的 feature/interrupt 声明（§4.2「profile 由 root 拥有且全生命周期不可变」）。今天 `ResumeCommand.InterruptKinds` **恰恰每次 resume 重传** —— 这是 C3 要治的真实偏差，不是新加校验。

**注册期校验**：`requiredByRunProtocol:true` 蕴含 `clientOptIn:true`（§8.2），违反要让 Registry **构建失败**（panic），不是运行时报错 —— §14 验收项明写这一条。

**C3 已完成（2026-07-29）**，落两个 commit：

- `6b7d76d2c` **前置重构（无 wire 变更）**：interrupt kind 统一成 `execution.InterruptKind`。**这是 C3 逼出来的**：profile 是 admission 冻结的、`RunDraft` 在 `execution`，而 kind 原本有两份（application string + transcript uint8，各跟一个 Interrupt 形状）；不统一就要在"冻结的 profile → executor"路上写一张两个同值集枚举之间的映射表。snapshot codec 的显式 wire 常量保留（durable 编码不跟 Go 改名走）。
- `baa7a304d` **C3 本体**：negotiate-once + 冻结 + 覆盖检查。

**实现落点（与「接手须知」的差异，按实际代码）**：

| 面 | 落点 |
|---|---|
| 协商 | `delivery/server/capability_negotiation.go` 一个函数，两种读法：start 冻结它、resume/subscribe 拿它做覆盖检查 —— 同一个词汇表，不可能各说各话 |
| 覆盖检查 | **application**（`runs.ErrProfileNotCovered`）—— resume 的 profile 只在 `pending` 里（`Consume` 是 `DELETE…RETURNING`，JOIN 不可能），delivery 不该为一道闸新开 run 读面；`SubscribeLive` 因此改成返回 error（`ok bool` → `ErrRunNotFound`/`ErrProfileNotCovered`），两个"进入已存在 Run"的入口共用一条规则 |
| 不可变 | `runs.protocol_profile` 列**只有 admission INSERT 与 Restore 写**，Suspend/Resume/finish 都不提及该列 —— 不是加校验，是让"能改它的语句不存在" |
| gap 类型 | `execution.RunProtocolProfile.Uncovered(caller)` 返回**同形状的 profile**（缺口本身就是一对 features+kinds），不新造词汇 |
| 不变量 | `run_protocol_profile_is_immutable` 已登记（boundary = `runs.admission`，fixture = sqlite `TestRunProtocolProfileIsImmutable`）；reducer 侧另有单测证明续段报的是 park 冻结值 |
| `toolResult` interrupt | 映射不到 domain kind ≠ 非法值：它是 `features.clientTools` 缺失，按 capability_not_negotiated 报；真正的未知枚举值才 invalid_params |

**顺带治的两处**（同一协商的另一半，不是顺手扩 scope）：`ClientCapabilities.events` 删除后 stream filter 只剩 ephemeral opt-out（"按声明裁流"给出的是没人看得出被裁过的短流）；`excludedEphemeralEvents` 命名 durable 事件现在报 `invalid_params`（在 `_meta` 解码处），不再静默无视。

**C3 明确 deferred（附因，非跳过）**：`interrupts.list` 的同一条覆盖规则（§8.1「一个 set 不许部分返回」）→ **C6**。那个 slice 本就把方法换成 root-owned aggregate，且它的拒绝要带 C10 的 `requiredCapabilities`；今天桌面端两种 kind 全声明，缺口不可达。

**建议顺序**：接下来做 **C4/C5**（都只依赖已完成的 C1/C2，互不依赖）。C3 唯一挡住的是 C4 里
`includeDescendants:true` 要返 `capability_not_negotiated` —— 那条在 C4 用现有 feature gate 直接拒即可（`features.subagents` 广告 false 已足够），profile 不参与该判断。

#### C4–C12 已完成（2026-07-29，同一会话连做）

**每个 slice 自己全绿后 commit + push**（Go 五关 + 前端 `npm run check`）。store epoch 37 → **39**（C6 改 interrupts 主键列名 → 38，C7 加 todos.revision → 39；两次都是丢库重建）。

**每 slice 咬出的真缺陷 / 真裁决**（只记不写在代码里就会重踩的）：

| slice | 真缺陷 / 关键裁决 |
|---|---|
| C4 | 三个位置有两种拼写、零个作者 → 新增 **`execution.RunStatus`**（domain），`RunState.Status()` 是唯一投影（store 列 + wire status 都从它派生），且**穷尽 + panic**（猜 "running" 会让 client 挂流、session 永占 admission 槽）。root-only 用 `spawned_by_item_id = ''` **谓词**而不是"反正没 child"。cursor 绑**规范化后**的 statuses。`includeDescendants:true` 用注册表的 **CapabilityRule**（registry doc 明写 handler 复查 = 第二作者）——handler 完全不读该字段 |
| C5 | 三个真问题：scope 靠猜（拿着 runId 问不了"那个 run 干了什么"）、错 id 返空页（"这 session 是空的" ≠ "没这个 session"）、读尾巴要翻完全部历史。store 拆**两个方法**而非一个可空 subject（"恰好一个非空"是没人检查的契约）。anchor 0 在**两个方向**都是"无锚"（sequence 从 1 起，精确）。page 的 `runs` 只含**本页 item 引用的** run |
| C6 | `Pending.RunID`（集合归属）与新增 `Interrupt.RunID`（谁提的）同名 → 先拆名（`RootRunID`）。**C3 deferred 的覆盖规则在此落地**：不覆盖就整集拒绝（裁剪后客户端会"答完"却让 run 永远等在它以为已解决的 interrupt 上）。`run_not_root` = **-32022**（-32007/-32010/-32012/-32015 是退役洞，不复用） |
| C7 | 三个"客户端做不到"：拿不到（recovery method 曾是 `runs.subscribe` = "想知道清单请挂一个 run"）、分不清先后（整体替换，内容说不出新旧）、校验不了（开放 map = 没人检查过任何 key 的值）。**Segment final snapshot fence** 落在 reducer 的 batch（park 与 terminal 是一个边界的两个理由），重复发是**代价也是要点**：收到 finish 的人必然收到了终值 |
| C8 | `session_busy` 与"工作树 mutation 在飞"同词 → 客户端只知道"有东西挡着"，给不出 steer/resume/cancel 三选一。**绝不隐式 cancel**。冲突可从两处到达（进程内 / durable unique index），走**同一个查找**。顺带删掉 start 读 open-interrupt 列表判 busy 的**重复真相源** |
| C9 | "批准并补充指示"曾是 resume + steer 两调用，中间有竞态窗口（模型可能已走完这一轮）。user Item 与 continuation **同一 opening write-set**；`userItemId` 的 iff 跨两个 shape，**没有 schema keyword 能表达 → fixture 持有** |
| C10 | `channel` 复述"落在哪"（第二答案会与第一个不一致）；`retryable` 没有 writer 且是被否决的 transient/permanent 分类。改为 **Error Registry 声明每 type 的 RecoveryAction**（闭合 7 值）。`capability_not_negotiated` 现在**必带非空 requiredCapabilities**（四个拒绝点都喂它，两个 profile 覆盖检查走同一个 translator：run 的未覆盖 profile 与"调用方要声明什么"是同一张表的两面） |
| C11+C12 | 一个 commit（topic 词汇表是 C11 `runtimeTopics` 的前置）。`workspace.subscribe` 装着 session/MCP/schedule 变更 —— 用一个域的名字装另外几个域只能靠"说谎"。**signal 只是失效通知**：`mcp.serverChanged` 曾带 status/toolCount/error（掉一帧就与 mcp.servers 分叉，且客户端察觉不到）。`topics` 必填无通配（没说能折叠什么就不能发给它）。**两处诚实省略**：`limits.runReplay` 缺席（journal 目前保留整段历史，没有上限可发布；发一个 runtime 不执行的数字 = discovery 说谎 → C13 落窗口+拒绝+该字段）；桌面端只订 5 个 topic（runs/interrupts/goals/state 今天从 run stream 折叠，订了也只会换来"刷新空气" → C15 给它们消费方后再订） |

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
- **B 与 B′ 无相互依赖，可并行。** B′ 每个 slice 可独立落 main，风险最低，且先清掉「第二真相源」会让 C 简单不少 —— **已作为起点执行完毕**。
- **C 必须等 B 与 B′ 都完成。**

---

## 8. 记忆修正 —— ✅ 已做完（2026-07-29）

| 记忆 | 结果 |
|---|---|
| `project_lyra_no_protocol_ts_codegen` | ✅ 已改写成 OVERTURNED，写清推翻理由（`UnionSpec` 显式 metadata 解掉 flat-struct 不映 union），并补了 2026-07-29 的四个生成模块现状 |
| `project_agent_refinement_closed_perf_dropped` | ✅ 已补边界：关闭只覆盖 agent 模块 CPU/延迟，**不覆盖** Batch D 的**有界性**问题（不封顶的 buffer 不让 turn 变慢，它让进程 OOM —— 论证救不了） |
| **新增** `feedback_delivery_never_leaks_business` | ✅ 三种形态都写了**判据**（文案要改措辞该改哪层 / 反推=把业务规则抄进 wire / 这段回答"问了什么"还是"怎么算"），并写明 arch 守卫只挡符号与 import 方向、新形态会全绿 |
| **新增** `project_vnext_contract_registry_shipped` | ✅ 跨 session 接手的入口：进度 + 产物形状原则 + 证据索引机制 + 「先审后做」的理由 |

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


---

## C13 / C14 已完成（2026-07-29，同一会话续做）

| slice | 咬出的真缺陷 |
|---|---|
| **C13** | 订阅只说「哪个 run」，runtime 就把它接到**恰好在跑的那个 segment** —— 一直在折叠旧 segment 的客户端会静默续进另一次执行，**自己的重连变成察觉不到的状态污染**。`runs.steer` 是同一个洞的另一面（parked→resume 之间发出的指示会灌进用户没见过的 continuation）。cursor 是裸序号，**上一个进程/上一个 segment 的 cursor 会解析成本流里一个真实位置**。「无 cursor」曾=重放整段（与 items.list 双重折叠）。retention 无上限（2048 events / 16 MiB 是**真需要**的：几个事件就能各带一份数 MB tool result，只数条数不bound内存）。**顺带三个**：`runs.start` 幂等重试返回的是 subscribe 的 ack 而非缓存的那个（同 key 两种结果，`userItemId` 静默消失）；generator 自己那张 problem-type→code 表是 dispatcher 的逐字副本；file-watch 的 `capability_not_negotiated` 报的是**方法名**（且 C12 改名后是个不存在的方法）|
| **C14** | 导出的 session 回来时**没有任务清单** —— 对话回来了、transcript 回来了，人最会注意到丢失的那份工作计划没有落点。且 restore 写集**先删 todos 行再重建 → revision 归 1**，客户端手上是 7 就会把导入值当旧的丢掉（治法：Replace 而非 delete+insert）。child edges 与 root-only `protocolProfile` 无处表达；`DroppedRun.run` 用 RunRef —— 对**同一事务刚删掉的记录**断言 activeSegment / 累计 metrics / profile |

**C13 也修掉了 C12 留下的红**（已对着 C12 自己那个 commit 复核）：两个 `SubscribeRuntime` 不传 topics、一个 hub 测试发的是编造的 event type 被新的 per-subscription filter 正确丢弃 → **挂十分钟而不是失败**；另有两个 transport 测试还在调 `workspace.subscribe`。

### C14b 为什么单独一片（不是漏做）

契约 §3.2 最后一条要求 session-scoped state 服从**完整** Session 生命周期。四半里两半已在：`delete` 删除（早已）、`export/import` 保留（C14）。剩下两半**做不到**，原因是**数据不存在**，不是没写代码：

- `fork` 要「fork boundary 时的语义值」、`history/both` rollback 要「恢复到目标 history boundary」；
- todos 投影是**单行最新值**，没有任何历史 —— 没有「run X 那一刻的值」可读。今天 rollback 只能**清空**（"恢复到空"，仅当边界处本来没有清单时才恰好对），fork **完全不带**。
- 治本形态：**per-run-boundary 快照**（run terminal 时把「本 session 此刻的 todos」记进该 run 的投影，一列 JSON 即可）→ rollback 读「存活的最后一个 run 的快照」、fork 读「fork 边界那个 run 的快照」。代价：store schema 加列（epoch 39→40）+ run terminal 写集 + 两处读 + 测试。
- 归为独立 slice 是因为它与 C14 的**根因不同**（C14 是「archive 没有落点」，C14b 是「runtime 没有历史」），且它有自己的 schema 爆炸半径。**必须在 C16 之前落**，否则 vNext 版本号声明的 §3.2 有一条不成立。

---

## C14b / C15 已完成（2026-07-29，同一会话续做）

| slice | 咬出的真缺陷 |
|---|---|
| **C14b** | 上一节预告的那半：**数据不存在**。`todo_boundaries`（`run_id` PK + FK cascade 到 `runs`）在**终态 CAS 那一条语句里**盖章 —— 那是 Run 唯一能到 terminal 的地方，所以"没有无边界的终态 Run"是构造保证，不靠每个调用方记得；`Restore` 刻意不盖（导入的 Run 在别的 runtime 结束，拿导入方的 live 清单当它的边界是编造）。**缺行 ≠ 空清单**：导入的 Run 从没被捕获过，两个读者一律"别动 live 值"，与未知 messageMark 不动日志同一条规矩。**顺带一个真 bug**：旧 rollback **DELETE 掉 todos 行**，revision 归 1，客户端手上是 7 就把回退后的清单当旧的丢掉 —— 与 C14 在 import 上修的是同一个根因（改 Replace） |
| **C15** | discovery 广告九个 topic，**只有四个有生产者**。第二个窗口对"run 开始了 / 有人在等回答 / goal 在烧预算 / 清单被改了"一无所知，而且**无法察觉**自己在漏（topic 在，流是静的）—— 与"discovery 发布 runtime 不执行的 limit"同一个病灶。goal banner 用四秒轮询遮住了自己那一份。**hub 还在丢帧**：它递增 sequence 然后丢掉帧，客户端只能靠"看见空号"知道漏了 —— 安静的流上永远看不见；现在未投递的失效合并成一条点名 topic 的 `resync`，**号只发给真进队列的帧**。前端：九 topic 全订 + exhaustive reducer（default 分支只在每个成员都处理时才编译得过，闭合联合写在 workspace 层而非从 wire 导入，**新增 signal 在 subscribe 边界报类型错**——这条当场抓出一个还在发 `mcp.serverChanged` 的测试）；session-scoped state 补上冷读（`todos.get` 此前**零调用方**，而它正是 capability 广告的 recovery method）；**run stream 断线不再冻结 transcript** —— 按最后折叠的事件重接，cursor 只由折叠推进（重接 ack 的 head 在请求位置**之前面**，采用它会静默跳过刚请求的重放），replay 窗口过期则冷读 items.list + tail 重接 |

### C16 已完成（2026-07-29）—— 咬出的真缺陷

| 面 | 咬出来的 |
|---|---|
| **一件事两种拼写** | 前端把 `protocolVersion` **自己拼了一遍**（`main/config.ts` 一个字面量，注释还写着"与后端常量一致"——没有任何东西保证）。翻版本会让它发出一个 runtime 当场拒绝的握手，而**类型系统看不见**。治本：生成器把 `PROTOCOL_VERSION` 投影进 `wire.generated.ts`，前端读它，config 里那份删掉 —— gate 1（generate 后无 diff）+ knip（生成了必须有人读）替代了那条注释。同类：`http` transport 测试的 `testProtocolVersion` 字面量恰好等于常量，翻版本会让一批**与传输无关**的测试变红 |
| **广告了没有实现的能力** | 三份 canonical 文档在删掉字段/方法/错误码目录之后，暴露出代码里 **305 处 `API.md §x` 引用**里有一批指向**根本不存在的小节**（`API.md §1.1`、`AUX_API §2.2/§2.3/§3.1/§4.2/§4.3/§5.1`——AUX_API 的方法根本没编号）。改写时把这些锚点**建出来**，引用因此第一次真的能落地 |
| **一个 shape 覆盖三个契约** | `TRANSPORT.md §5` 整节描述一个**不存在的 IPC transport**（"桌面外壳走宿主 IPC"）。桌面壳走的是 loopback HTTP，IPC 从来没实现、也不该实现。删掉会打断 §6.x 的引用编号，所以该节**改写成一条裁决记录**（为什么没有 IPC），编号不动、内容不再撒谎 |
| **文档自身的病灶** | `smoke.test.ts` 手写了一份 discover 响应，里面 `capabilities.events` / `limits.maxConcurrentRuns` 早已退役 —— TS 的 excess-property 检查在这个位置**不触发**（实测确认），所以它可以一直看起来权威。治本：改用 canonical 样本（同一份被 schema gate 校验的文档），测试从此不是那份 payload 的第二作者。`runtime/index.test.ts` 的 `as unknown as ServerCapabilities` 是同一个洞的另一种打开方式，换成 typed fixture |

**canonical 文档的最终形态**（结论已在下一节记录，此处只记落地事实）：`API.md` 2074 → 1039 行（删掉 §4 字段目录与 §7 逐方法输入/输出/错误表，改为"生成物是字段级真相"+ 每域的语义与不变量）；`AUX_API.md` 补齐被引用的小节编号并按 vNext 改写（`workspace.subscribe`→`runtime.subscribe`、`DroppedRun.run: RunSummary`、新增 §4.3 export/import v7）；`TRANSPORT.md` 逐条改准（SSE 例子里的 `outcome.result` → `outcome`+`metrics`、subscribe 必带 `segmentId`、cursor 的两种拒绝、失效流的 coalesce-into-resync）。

### C16 为什么是一个不可拆的 commit

三条 gate 把它锁成一体，拆开必有一步是红的：

- **gate 12**（`TestProtocolVersionAgreesEverywhere`）会点名 canonical 文档里任何本 build 不服务的版本号 —— 所以"翻常量"和"改文档"必须同一个 commit；
- **gate 16/17**（compatibility diff 判 breaking 且要求同步 bump）在 bump 之前必然红；
- **gate 15**（Artifact v7 round-trip）要的就是 `SessionArtifactVersion = 7`。

**canonical 文档怎么处理（已按契约 §0 定案，不要再讨论"删掉换指针"）**：契约 §0 明写"完成切换后：本文定义的目标 shape **必须进入 canonical 协议与生成物**""本文不是第二份字段目录，**字段级真相最终只存在于 Registry 生成的 schema**"。合起来只有一种读法 ——

- `desktop/docs/protocol/{API,AUX_API,TRANSPORT}.md` **仍是 canonical**，按 vNext 改写；**不删**（另有硬理由：Go/TS 注释里有 **305 处** `API.md §x` / `AUX_API §x` 引用，删文件等于制造 305 个悬空引用，而重编号到契约的 § 号是逐条易错的静默误导）；
- 改写时**删掉被生成物重述的字段/方法/错误目录**，改为引 `contract/{openrpc,schema,manifest}.json` 与 `contract/API_REFERENCE.md` —— 一个事实一个作者；文档留自己独家的：语义、不变量、wire 例子；
- `TRANSPORT.md` 是 transport binding 的**唯一**作者（契约本身没有 transport 章）：端点 / POST 契约 / HTTP status / streamable 帧 / SSE / event id 与续流 / 门禁 token / sidecar / CORS / 背压 —— 它不是任何东西的第二份拼写，按 vNext 改准即可。

C16 的完整清单：① 两个常量 ② `go generate` 重出产物 ③ 旧协议 / 旧 Artifact 的确定性拒绝各带测试 ④ fixture 扫（Go `testProtocolVersion`、前端 `main/config.ts` + samples + 测试里的硬编码日期）⑤ 三份 canonical 文档改写 ⑥ gate 15/16/17/18（16/17 需要一个 compatibility differ，读 `internal/arch/testdata/baseline/{manifest,schema}.json` 与当前产物比）。
