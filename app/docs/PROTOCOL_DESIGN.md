# Lyra Runtime Protocol 设计指南 —— 四方对照 · 取舍基准 · 演进路线

> **本篇是什么**：`API.md` / `AUX_API.md` / `TRANSPORT.md` 是 wire 契约的**真相源**（有什么、什么形状）；本篇是**为什么这样定、接下来怎么调**。它合并两轮独立分析——一轮以四方协议的一手源码对照为主，一轮以状态机与契约体系为主——并把其中每条关于本仓库的事实主张**逐条到代码核实**后收敛成一份。
>
> **对比对象（只谈协议/API 设计，不谈引擎能力）**：**Codex app-server**（OpenAI，双向 JSON-RPC over stdio/WS/unix）· **opencode v2**（sst，Effect HttpApi + OpenAPI 3.1 + 事件源）· **Claude Code / Agent SDK**（Anthropic，NDJSON over stdio + in-band control 协议）。
>
> **方法**：一手核对。opencode 读其 `packages/protocol` + `packages/schema` 源码（v2 重写后的形态，与其在线文档能看到的部分不同）；Codex 读 app-server 规范的全量方法与通知表；Claude Code 读 headless + Agent SDK 文档，并以其 bridge 实现的 `control_request` 交叉验证；我方读 `docs/protocol/*` + `internal/delivery/{protocol,dispatch,server}` + `internal/application/runs` + `internal/domain/execution` + 前端 `src/rpc`。
>
> **结论分三级，正文逐条标注**：
> - **【已核实】** 已在本仓库代码中确认的事实（附证据位置，符号级、不写行号）。
> - **【设计判断】** 基于第一性与项目法则的取舍建议，未经确认不落地。
> - **【已裁决】** 已在 §19.2 落定（结论 / 理由 / 影响 / 被替代方案），可以照它动手。
> - **【待定】** 有两种合理解，需要决策而不是继续讨论（§19.3 剩三项，均为实现期数值）。
>
> **裁决立场**：对照 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md)（薄核 / 窄腰 / 一个扩展机制）与 [`../../CLAUDE.md`](../../CLAUDE.md)（第一法则不留债、第二法则治本、YAGNI）判"该不该学"，而非见特性就抄。引擎能力对比见 [`RUNTIME_COMPARISON.md`](RUNTIME_COMPARISON.md)，GUI 形态见 [`DESKTOP_COMPARISON.md`](DESKTOP_COMPARISON.md)。
>
> **与 [`codex_runtime_api_design_guide.md`](codex_runtime_api_design_guide.md) 的分工**：那份是**独立复核稿**（保留独立判断、负责挑战本篇的结论）；本篇是**收敛稿**，额外承载 §19 的决策簿。两份都用同一套三级标注，互为对照——**决策簿只存在于本篇一处**，避免"哪份是真相"再出现第三次。
>
> **已吸收的反驳（本篇被改正的地方）**：cursor 结论（§7.3，"实现是安全的一方"是错的）· 前端错误码结论（§15 表第 11 行，我查错了文件）· 跨 Segment 计量（§5.3）· 控制面协议根（§16 P2，撤回了本篇原来的推荐）· opencode 可靠性是两层不是 volatile 一层（§2）· Codex 强类型 item 的代价界定（§2）· per-request 协商的绝对化表述降调（§3.5）· canonical sample 不能由 Registry 生成（§12.1）· 错误码清理属代码批不属文档批（§16 P0-B）。
>
> 本篇与 canonical 协议冲突时，**以 canonical 协议与代码为准**；本篇用于指导下一轮经确认的调整。

---

## 0. TL;DR

**守 7 条**（比三家更有原则，动它即回归）：领域中立工具信封 · durable 事件是投影而非存储 · HITL R 模型 · 一个判别字段 `type` · 无状态 per-request 能力协商 · **可推导的事实不上 wire** · 核心/旁路分离 + 三扩展缝。

**吸收 4 条**：① 可执行契约注册表 → 机器可读制品（method 名现在写四遍）；② 精确状态 + 并发前置条件（`waiting` 一等、`expectedSegmentId`、subscribe 绑 segment）；③ 把 `durable` 拆成 **authoritative / replayable / ephemeral** 三级；④ 高层 SDK 流句柄（wire 保持窄，复杂度收在客户端库）。

**修 9 类**（§15 逐条附证据）：wire 把 parked run 报成 `finished` · `RunOutcome` 混入 `interrupt` · steer 无执行实例前置 · subscribe 只绑 runId 且三种情况同一个错 · replay cursor 无归属校验（重启后会静默跳过 replay）· **跨 Segment 计量无闭合语义（interrupt 边界不带计量）** · MCP 工具 identity 文档与代码不同 · **前端存在第二份手写错误码表且已错值** · 一批文档与类型漂移。

**七项已裁决**（§19.2，breaking 已授权）：计量走 Run 累计快照 · duration 只留 `activeDurationMs` · cursor 自带 epoch 且 subscribe 绑 segment · `interrupts.list` 取代 `runs.listOpenInterrupts` · 删 `ProblemData.channel` 与 `retryable` · Registry 用显式 descriptor + 字段 reflection · `runtime.subscribe` 完整替换 `workspace.subscribe`。

**拒 8 条**：REST 影子面 · 常开全局 SSE 总线 · 强制连接握手 · server→client request · per-tool Item 联合膨胀 · per-event 版本化与 V1/V2 并存 · 业务错误映 HTTP status · 把进程内 SDK 消息联合当 wire。

---

## 1. 目标：三个角色看到同一个世界

```text
用户：我在这个对话里发起了一次任务；它在跑 / 在等我 / 继续了 / 完成了。
Agent：同一个 Run 跨若干 Segment 推进，产出一条权威 Item 时间线。
客户端：发 typed command，fold typed event；断线就 replay，replay 不行就冷恢复。
```

**最小客户端只需理解**：建/选 Session → 起 Run → 消费 Item 与 Run 事件 → 遇 Interrupt 应答 → `items.list` 补历史。

**最小客户端不得被要求理解**：连接身份 · 服务端 goroutine · executor process · checkpoint 存储格式 · 并行存在的多套"消息/步骤/任务/后台作业"模型 · 同一业务操作的两套入口。

---

## 2. 四方形态对照

| 维度 | Codex app-server | opencode v2 | Claude Code | **lyra（目标）** |
|---|---|---|---|---|
| 对外形态 | 双向 JSON-RPC（stdio/WS/unix） | REST + OpenAPI 3.1（标 Experimental） | Agent SDK / CLI，进程级 AsyncGenerator | JSON-RPC + Streamable HTTP，同协议支持 in-process |
| 核心资源 | Thread → Turn → Item（**强类型变体**） | Session → Message → Part（**事件源投影**） | 无服务端资源（JSONL + `session_id`） | **Session → Run → Segment → Item** |
| 初始化 | 每连接 `initialize` → `initialized` | 无握手（调 list 探能力） | SDK 起子进程 | `runtime.discover` 可选，请求自描述 |
| 流式 | 同一双向连接的通知 | 单条全局 `/api/event` SSE | `Query extends AsyncGenerator` | 每个流式调用走自己的 POST 响应流 |
| 事件可靠性 | 通知 + 持久 thread/item 查询 | **两层**：全局 `/api/event` volatile（断线即丢）+ **per-session durable 事件流** `GET /api/session/:id/event?after=<seq>` 与 `/history` | transcript + 当前进程流 | **三级：authoritative / replayable / ephemeral** |
| HITL | **server→client request** + `serverRequest/resolved` | **三套并存**：permission / question / form | 进程内 `canUseTool` 回调 | durable Interrupt + `runs.resume`，不绑连接 |
| steer | `turn/steer` + `expectedTurnId` | prompt 支持 steer delivery | streaming input / control 方法 | `runs.steer` + `expectedSegmentId` |
| schema | 运行版本生成 TS + JSON Schema | Schema 即 SSOT → OpenAPI → 生成 SDK | SDK 自带 TS/Python 类型 | Go 契约注册表 → OpenRPC / JSON Schema / TS |
| 错误 | RPC error + `codexErrorInfo` + `httpStatusCode` | tagged error class + `httpApiStatus` | result subtype + `system/api_retry` | RPC / Run / Tool 三落点，统一 Problem，不映 HTTP status |
| 多客户端 | 请求绑连接，需处理订阅归属 | HTTP 天然松耦合，但状态靠轮询 | SDK handle 通常单宿主 | 任意客户端可查询与恢复 |

**每家的代价，一句话**：

- **Codex** 为强类型 item 付出的代价要**准确界定**：新增一个一等**执行类别**（command execution / file change 这种）要动 wire 类型**并**分裂审批方法（`item/commandExecution/requestApproval`、`item/fileChange/requestApproval`、`item/permissions/requestApproval`）；普通工具（含 MCP）走 `mcpToolCall` 通用路径、**不**动协议。所以这不是"每个新工具都改协议"，而是"每类新执行语义都改三处"。另外为 server→client request 付出一个必须额外发明 `serverRequest/resolved` 去清理的孤儿态。
- **opencode v2** 的 durable 层是真实存在的（per-session 事件流 + `history`），**这正是 per-event 版本化的原因**：事件日志就是存储格式，于是 schema 包里有 `Event.latest()` / `versionedType()` 这套迁移机制、事件帧带 `{aggregateID, seq, version}`，并且 **session / permission / question 的 V1 V2 定义并存 + 专门的 legacy 事件模块**。它的 replay cursor 是**按 aggregate 的显式数字 seq**（`?after=<seq>`），归属靠**资源路径**结构性保证——这一点见 §7.3 的对照。
- **Claude Code** 为极致易用付出"正确性依赖进程存活 + 单客户端"（控制通道 `control_request` 复用同一条 stdio 管道）。

---

## 3. 必须守住（7 条）

1. **领域中立 `ToolInvocation`**：核心只认 `{name, arguments, result}`，富渲染走客户端展示注册表，加工具零协议成本。**判据**：任何"给某类工具加 wire 一等类型"的提案一律拒。
2. **durable 事件是投影，不是存储**：durability 是 `event.type` 的纯函数（不每帧冗余携带），权威落点在 Item + SQLite。**判据**：任何要给单个事件加 `version` 的提案都是事件源化第一步，拒。
3. **HITL R 模型**：interrupt 收尾当前 segment、资源释放、待解项 durable、任意客户端可 resume。**判据**：不引入 server→client request；不让正确性依赖某个客户端在线。
4. **一个判别字段 `type`**（`kind` 禁上 wire）：给人和 codegen 同时省事的硬规则，不是洁癖。
5. **无状态 per-request 能力协商**：能力随 `params._meta` 走，server 不维护 client 连接注册表。**这是最适合我们"同一语义跑多 transport + 多客户端"目标的形态**（握手式协商必须回答"这条连接的能力是谁的能力"），不是断言它对所有系统都唯一自洽——单宿主长连接系统里握手更合适。
6. **可推导的事实不上 wire**（本轮命名的既有原则）：字段只承载**无法从上下文或其它字段推导**的量。已按它做出的五个决定互为印证——per-frame `durable`（可由 `event.type` 推导）· `ProblemData.channel`（可由落点推导）· `retryable`（可由 `type` 推导）· 墙钟 duration（可由 `createdAt`/`finishedAt` 推导）· `object`/`resourceType` 字段（可由 id 前缀推导）。**判据**：新字段先问"客户端能不能自己算出来"——能，就别加；不能，才加。冗余字段的代价不是几个字节，是它**必然与推导源漂移**。
7. **核心/旁路分离 + 三扩展缝**：核心只 sessions/runs/items；新增旁路能力先选它**真实的**协议根（`workspace.*` 只放工作树视图，skills / recipes / agentDocs / mcp / hooks / codebase 各自顶层根）；"额外的东西"先过 Item / state / custom 选择表，并统一走 `plugin:<name>/` 命名空间。

---

## 4. 必须吸收（4 条）

### 4.1 可执行契约注册表（取自 Codex + opencode 的生成体系）

**现状【已核实】**：method 名分别手写在 `delivery/dispatch/method_names.go`、`method_table.go`、前端 `rpc/methods.ts`、以及协议 Markdown 的方法表——**四处**。`API.md §14` 自己把"导出 OpenRPC + JSON Schema"写成要求，至今是空的；golden-sample 双侧闸已生效（覆盖 §4 数据目录 + §5 流的每个联合变体），但样本只能覆盖有样本的形状。

取的是"**一个注册点、多份生成物**"的思想，不取其框架——不换 SSOT 语言方向，Go 仍是源。见 §12。

### 4.2 精确状态与并发前置条件（取自 Codex）

`thread/status` 与某个 turn 的完成原因是两件事；`turn/steer.expectedTurnId` 让延迟命令确定性失败而不是注入错实例。我们对应的是 Run 的 `waiting` 一等化 + `expectedSegmentId` + subscribe 绑 segment。见 §5、§6。

### 4.3 三级可靠性术语

当前一个词 `durable` 同时表示"进了 SQLite 可冷恢复"和"还在 hub 回放窗口内可 replay"，`state.snapshot` 那句"取决于 key"就是这个混淆的产物。拆成：

- **Authoritative**：进程重启后仍可从持久读模型恢复的事实。
- **Replayable**：当前流的 journal 在窗口内可从 cursor 重放（**内存 journal ≠ 已持久化**）。
- **Ephemeral**：只改善实时体验，可丢，且必须有 authoritative 落点。

见 §7.2。**成本极低（术语 + 一张表 + retention 进 limits），概念收益大。**

### 4.4 高层 SDK 流句柄（取自 Claude Code 的 `Query`）

wire 窄不等于让每个 UI 手拼 JSON-RPC。生成的客户端应把 request id / `_meta` / Idempotency-Key / SSE 解析 / 去重 / 重连 / 冷恢复信号 / typed error / reducer 收在库里。**但控制方法不逐一上升为核心 wire**——SDK 是便利层，wire 只保留稳定领域命令。见 §13。

---

## 5. 目标语义：Run / Segment / Item / Interrupt

### 5.1 状态机（`waiting` 一等）

```text
[*] --runs.start--> Running
Running --segment 以 interrupt 收尾--> Waiting
Waiting --runs.resume--> Running（同 runId，新 segmentId）
Running --completed|error|maxSteps|maxBudget|canceled--> Finished
Waiting --runs.cancel--> Finished
```

不变量：

- `Running`：有一个活跃 Segment；
- `Waiting`：无活跃 Segment，有 durable open Interrupt；
- `Finished`：不可 resume，必有 terminal outcome，且**只有 Finished 有 `finishedAt`**；
- **Interrupt 不是 RunOutcome**（它是 Run 的等待态、是 Segment 的收尾原因）；
- 一个 Session 至多一个非 terminal Run；
- cancel 对 Running 与 Waiting 都有效。

**为什么这是第一优先级【已核实】**：`domain/execution/run.go` 的注释原话是 *"An interrupt is deliberately NOT an Outcome: parking is the Interrupted state"*，而 wire 的 `RunOutcome` 有 `{type:"interrupt"}` 变体，`delivery/server/presenter_run.go` 把 `execution.Interrupted` 与 `Completed/Failed/Canceled` 一并映射为 `RunStatusFinished`。于是 **wire 同时违反 domain 写明的不变量与 `API.md §0.3` 那句"Run 本身不结束"**：客户端看到 `status:"finished"` 的 run 却仍能 resume。补一刀：**wire 的 `SessionStatus` 已经有 `waiting`**——今天是会话级与 run 级两层状态自己不一致。

### 5.2 目标 wire 分层【设计判断】

```ts
type RunStatus = "running" | "waiting" | "finished";

type RunOutcome =                                    // 只有真正 terminal
  | { type: "completed";  result: RunResult }
  | { type: "error";      result: RunResult }
  | { type: "maxSteps";   result: RunResult; detail?: string }
  | { type: "maxBudget";  result: RunResult; detail?: string }
  | { type: "canceled";   result: RunResult; detail?: string };

type SegmentOutcome = { type: "interrupt"; interrupts: Interrupt[] } | RunOutcome;
```

`segment.finished` 携带 `SegmentOutcome`；`RunRef.outcome` 只携带 `RunOutcome`。这一改同时消灭"已完成但还能继续"的非法心智，和"哪个 outcome 属于哪一层"的模糊。**破坏性**：见 §16 P1。

### 5.3 跨 Segment 计量必须闭环【已核实 + 已裁决】

Run 跨 resume 保持身份，计量就不能在每段悄悄归零。**已核实的缺口**：`delivery/server/presenter_run.go` 在 `Interrupted` 分支返回的是 `RunOutcome{Type: OutcomeInterrupt, Interrupts: …}`——**不带 `Result`**。也就是说停车边界上客户端拿不到任何 authoritative 的 usage / steps 快照；要显示"这次运行已花了多少"，只能另查 `usage.session` 或丢掉这段的部分值。

**已裁决（§19.2 第 1、2 项）**：

- **每次 `segment.finished`（含 `interrupt`）携带 Run 的累计快照**，terminal `RunResult` 用同一口径。**理由是幂等**：累计快照重放/重复 fold 都不会多算；per-segment 增量值一旦被 replay 就会双计——而"想看单段花了多少"可以两次快照相减得到（可推导，见 §3.6）。
- **累计口径含 subagent 子树**（与 `maxBudgetUsd` 已定义为含子树一致）；每个 subagent 的 `RunRef` 另带它自己的用量。**客户端永不需要自己求和，也就不必猜有没有重复计。**
- `maxSteps` / `maxBudgetUsd` 是 **Run 生命周期**限制，跨 resume 按同一棵树继续累计。
- `segment.progress` 可近似 / 节流 / 丢帧，**不得**成为计费与预算判断的唯一来源。
- **duration 只留一个不可推导的量**：`activeDurationMs`（各执行段之和，**不含 Waiting**，累计）。墙钟耗时由 `createdAt` / `finishedAt` **推导**，不再存字段；单段时长归 observability，不进 wire。现有 `durationMs`（"跨 interrupt/resume 的墙钟"）同时被当性能指标和用户总耗时用，**删除**。

### 5.4 所有权

| 对象 | 身份稳定期 | 拥有 | 不应拥有 |
|---|---|---|---|
| Session | 整个对话 | cwd、标题、默认模型、Run 历史 | 当前 executor handle |
| Run | 一次用户意图（跨 resume） | 状态、终态、**累计计量**、subagent 树 | transport connection |
| Segment | 一段连续执行 | stream root、停止原因、**段计量边界** | 长期对话身份 |
| Item | 时间线单元 | 内容、工具调用、权威完成态 | transport delta buffer |
| Interrupt | Run 的 durable 等待 | 待答内容、关联 item、创建时间 | 原客户端连接 |

---

## 6. 核心面与控制命令

### 6.1 最小核心

```text
runtime.discover（可选）
sessions.create / get / list / update / delete / fork
runs.start / resume / steer / cancel / subscribe / list
items.list
interrupts.list          // 待处理"收件箱"心智；命名决策见 §16 P1
```

workspace、mcp、providers、models、memory、schedules、goals 等继续是 capability-gated 辅助域，不进最小 profile。Interrupt 的解决仍只通过 `runs.resume` 原子完成——不设"先 resolve、后 start"的中间态。

### 6.2 各命令的前置条件与保证【设计判断】

| 方法 | 必须保证 |
|---|---|
| `runs.start` | 成功响应 = 用户输入与 admission 已 durable；同 Idempotency-Key + 同请求只创建一次；返回 `runId` + `segmentId` + `userItemId` 供乐观 UI 精确对账；断网不取消 run；admission 错误走同步 RPC error，执行错误走 `segment.finished` |
| `runs.resume` | 只允许 Waiting；每个 response 必须对应仍 open 的 Interrupt；**consume + Waiting→Running + 开新 Segment 原子提交**；重复应答返回 `interrupt_not_open`；返回新 `segmentId` |
| `runs.steer` | 带 `expectedSegmentId`，不匹配即确定性拒绝（不 best-effort 注入）；输入用 `ContentBlock[]` |
| `runs.cancel` | Running 与 Waiting 都可 cancel；已 Finished 返回可分支业务错误；reason 回流 terminal `detail`；**durable 状态提交先于对外确认** |
| `runs.subscribe` | 绑 `{runId, segmentId}`；目标段已结束 / 不可 replay / 进程已重启时返回**可行动**错误，客户端转冷恢复（`items.list` + `interrupts.list` + 状态查询） |

**现状【已核实】**：`SteerRunRequest` 是 `{runId, message string}`——既无执行实例前置（多客户端 / 延迟请求 / resume race 下可注入错段），`message` 还是裸 string（图片等输入被形状挡住）。`SubscribeRunRequest` 是 `{runId}`；且 `delivery/server` 的 SubscribeRun 把**不存在 / 已结束 / 停车中**三种情况统一回 `run_not_found`——重连到一个正等你审批的 run，客户端收到的是"找不到"。

---

## 7. 流与恢复

### 7.1 保留单一通知方法

继续 `notifications.run.event` 承载全部 run/item/state 事件：一个 reducer、一个订阅过滤器、一个 run-tree membership 模型；新事件只扩 `event.type`，不扩方法路由。**对照**：opencode 深点号嵌套事件名，Codex 每类 delta 一个通知方法。

### 7.2 三级可靠性落点表【设计判断】

| 事件 | Authoritative | Replayable | 冷恢复来源 |
|---|---|---|---|
| `item.completed` | ✅ | ✅ | `items.list` |
| `segment.finished` | ✅ | ✅ | run 状态查询 / `items.list` |
| `state.snapshot` | 按 key 声明 | ✅ | **first-party key 必须声明冷恢复来源** |
| open Interrupt | ✅ | ✅ | `interrupts.list` |
| `item.delta{*}` | ❌ | 可选 | `item.completed`（content / text / tool.arguments / tool.result） |
| `segment.progress` | ❌ | 可选 | terminal `RunResult`（usage / steps） |

**硬规则不变**：丢弃所有 ephemeral，客户端仍必然得到正确终态；新增无 authoritative 落点的 ephemeral = 协议违规。**新增一条**：文档不得把 "replayable" 写成"已持久化"——内存 journal 与 SQLite projection 是不同保证。

### 7.3 cursor 契约【已核实 + 设计判断】

**核实结果**：event cursor 由 `application/runs` 的 Coordinator 持有**进程级**单调计数器（定宽零填充，保证字典序=数值序），`evt_` 框架在 delivery 施加。而 `API.md §2.4` 写的是"`runs.resume` … eventId 从头"（每段重置）——**文档描述的不是实现**。

**但"实现是安全的那一方"这个结论是错的（本篇上一版的结论，已由独立复核推翻，此处以代码为准）**。进程级计数器只消除了"同进程内不同 Segment 复用同一数值"这一种冲突，三个漏洞仍在：

- `application/runs` 的 `Journal.Subscribe` 只做 `ev.Cursor() > fromCursor` 的**字符串比较，不校验 cursor 是否属于本流**；
- **计数器随进程重启归零**（这是有意的：eventId 不是全局时钟），于是重启前的旧 cursor 可能**大于**新段已有 cursor → replay 被**静默跳过**，而不是报错；
- `runs.subscribe` 只按 runId 选活跃 entry，**无法证明** `Last-Event-Id` 与它返回的 `segmentId` 同源。

**所以正确结论是**：实现比"每段裸重置"少一种冲突，但目标契约仍未落地——**不是把文档改成"进程级全局递增"就完事**，必须同时补归属校验与两个显式错误。

**目标契约**：

- eventId 对客户端**完全 opaque**：只做相等去重，不做大小比较；
- cursor 绑 `{runId, segmentId}`，`Last-Event-Id` 只能与同一对一起使用；
- server 检测 cursor 不属于目标流 → `replay_cursor_invalid`；无法 replay → `replay_unavailable`，客户端转冷恢复；
- replay 保留窗口（时长 / 事件数 / 仅当前进程）由 capability `limits` 明示；
- 服务端**可以**把 segment/epoch/position 编进 opaque id，但那不是客户端契约。

**已裁决（§19.2 第 3 项）**：

- **寻址用 params 的 `{runId, segmentId}`**，cursor 仍走带外 `Last-Event-Id`（符合既有元数据划分）；
- **opaque cursor 内部编码 `version + epoch + segment + sequence`**——这是让带外 cursor 可被校验的关键：server 无需查任何状态就能判定它是否属于本流、是否来自上一个进程 epoch；
- 归属不符 → `replay_cursor_invalid`；窗口已淘汰 / epoch 已换 → `replay_unavailable`（两者分开报，客户端一个重订阅、一个转冷恢复）；
- replay 只保证**当前进程**、有限窗口，retention 在 capability `limits` 里明示。

**对照**：opencode v2 把流身份放进**地址**（`GET /api/session/:id/event?after=<seq>`），归属由资源路径结构性保证——最干净，但要求"一条流一个 URL"。我们选"params 带 segment + cursor 自带 epoch"，代价是编码约定，收益是不破坏元数据划分规则。**无论哪条路，没有 segmentId 参数的 subscribe 都永远无法校验 cursor**——这让 P1 里"subscribe 绑 segment"从"更清晰"变成"否则做不到"。

### 7.4 慢消费者

authoritative / terminal 事件不因背压丢失；ephemeral 可合并或丢弃；慢消费者不得阻塞 agent 执行；丢 delta 后不得发出无法解释的残缺 completed item；流断开不取消 run；SDK 在重连失败时切冷恢复而不是无限 retry。

---

## 8. Human-in-the-Loop

**基线不变**（R 模型）：`durable Interrupt 提交 → segment.finished{interrupt} → Run.status=waiting → 任意客户端 interrupts.list → runs.resume → 同 Run 新 Segment`。它不依赖原连接、不依赖原客户端、可跨进程重启、桌面关掉仍可恢复，且 executor 能释放资源。

三条增强：

1. **保温但不依赖保温**【设计判断，低优先】：server 内部可给 parked executor 有限 grace period，期内 resume 复用热进程、期外从 durable snapshot 恢复，**两条路径 wire 完全一致**；正确性永不依赖进程存活。
2. **Interrupt 必须自包含**：runId / sessionId / itemId / type / 人读 prompt / 工具名 + 已解析入参或完整 question fields / safetyClass / rememberable / createdAt。客户端不应为了画一个审批框，去 join 一个可能还没持久化的 running item。
3. **多客户端失效通知**【设计判断】：别的客户端 resume/cancel 或自动策略解决了某个 Interrupt 时，推一条 `interrupt.resolved` 失效通知（走 §16 P2 的控制面事件），**只触发 refetch、不承载唯一事实**——吸收 Codex `serverRequest/resolved` 的 UI 清理价值，不继承其连接耦合。

**审批记忆分层保持**：一次批准 / 一次性 `editedArgs` / 长期规则 `remember{session|project|global}` × `(tool, subject)` 是三个不同概念，别合并。

---

## 9. Item 与 Tool

### 9.1 中立信封的硬约束

`arguments` 永远是 JSON 对象、绝不双重编码；部分入参只走文本 delta；`result` 在 completed item 上权威；工具失败走 `toolCall.error` + `status:"incomplete"`（**不伪装成 result，也不上升为 run 失败**）；未知工具用 JSON 树兜底渲染。

### 9.2 工具 identity 只能有一个权威【已核实 —— 最贵的一处漂移】

`mcp/tools.go` 组装的 wire 名是 `sourceName + "_" + tool.Name` 再 sanitize（非 `[A-Za-z0-9_-]` 一律替 `_`，截断 64）；`API.md` 有两处写 MCP 工具是 `"<server>.<tool>"`（点号）。**这个字符串是审批规则匹配、展示注册表命中、日志聚合共用的 identity 值**——不是漏个字段，是客户端拿去比较的东西写错了。

**判断**：以**代码为准**改文档。下划线形态不是将就某个 SDK，而是工具名要能安全地送进各家 LLM API 的 tool schema（点号并非普遍安全）。同时把一条已知边界写进文档而不是留给下一个人踩：sanitize + 64 截断使碰撞可能，当前是**装配期 fail-closed 报错**（`duplicate tool name after prefixing`），代价是"整套 MCP 工具装不起来"而非"那一个工具不可用"。

**不加 `displayName`**：只有确有展示需求时才加，现在没有。

### 9.3 闭核、开放工具

Item 生命周期语义是**闭合核心**；tool name / arguments / result、custom 事件名、feature key、state 顶层 key 是**开放扩展面**。新工具**不得**要求新 Item type；只有真正拥有独立生命周期、状态与跨工具语义的概念，才有资格成为新 Item type。

### 9.4 flat union 的非法状态【形状事实 + 设计判断】

`StreamEvent` / `Item` / `RunOutcome` / `ItemDelta` 用"flat struct + optional 字段"表达联合，因此可表达 `type:"item.completed"` 却没有 item、content delta 却带 plan steps 之类的非法组合。**务实顺序**（不为类型漂亮让业务层塞满 marshal glue）：

1. 先有生成的 schema + **encode/decode 边界的统一 `Validate()`**；
2. 再评估是否把高风险联合改成 per-variant struct + sealed interface；
3. 入站已由严格解码 + golden 样本兜住，出站的唯一生产者是我们自己的 presenter——所以这是**加固**，不是止漏。

---

## 10. Capability 与版本

### 10.1 不引入强制 initialize

`runtime.discover` 是无副作用查询、可缓存；`_meta` 由 SDK 自动注入；HTTP / IPC / in-process 不因连接形态得到不同业务语义；server 不维护 client 连接注册表。

### 10.2 简化事件能力声明【设计判断】

现在同时有 `events` allowlist 与 `excludedEvents`，客户端要处理"省略 / 空数组 / 未知事件"三种含义。目标：

```text
核心生命周期事件           → 版本保证（支持该 protocolVersion 的客户端必须接受，未知必须忽略）
高频预览事件               → subscription preference（excludedEvents 只允许关 ephemeral）
可选领域事件               → feature capability
需要客户端行动的 Interrupt → client capability（interruptTypes 继续显式 allowlist，防永久等待）
clientTools                → 继续显式协商（它要求客户端干活）
```

比"让客户端枚举所有它能渲染的事件"更符合人体工程学，且不削弱防挂死保证。

### 10.3 experimental 按 feature 精确 opt-in

吸收 Codex 的 experimental gate 思想，但**不要**一个全局 `experimentalApi:true` 解锁全部。规则：experimental method/field 必须归属一个 discoverable feature key；client 精确 opt-in；未协商即拒绝；晋升 stable 后进入版本契约；**不用散落的 `xFoo` 字段制造永久实验垃圾**。

### 10.4 版本规则

`/v2/` epoch（wire major）+ 日期 `protocolVersion`（epoch 内请求版本），两层不重复。

- **additive（同版本）**：加可选响应字段、加 method（须 discovery 可见）、加事件类型、加 feature key、加 custom/state key。
- **breaking（新日期版本）**：加客户端必须发送的请求字段、改字段含义、改闭合 enum/union、删除或改名。
- experimental feature 内的变化只对 opt-in 客户端负责。
- **dev 阶段 bump 后直接更新，不留 legacy shim、不写 migration。**
- 生成的 manifest 须带：current version、min supported version、feature stability、method stability（deprecated 只在真有外部兼容期时使用）。

---

## 11. 错误模型

### 11.1 三个物理落点

| 阶段 | 落点 |
|---|---|
| admission / params / capability | JSON-RPC error（带数字码，仅此通道） |
| Run 执行整体失败 | terminal outcome 的 `result.error` |
| 单个工具失败 | `toolCall` item 的 `error` + `status:"incomplete"` |

不把所有错误塞进 RPC error，也不把 admission 错误伪装成一个已经开始的 Run。**HTTP status 只表示 transport**：业务错误一律 HTTP 200 + JSON-RPC error；malformed HTTP / 门禁 / content-type / body size 才用 status；开流后的业务失败在 SSE 内收尾；sidecar health/info 不进 JSON-RPC。

### 11.2 `ProblemData` 字段【已裁决（§19.2 第 5 项）】

**保留**：`type`（稳定符号，唯一判别键）、`detail`、`docUrl`、`errors[].field`、`retryAfterSeconds`（backoff hint）。

**删除两个**，依据 §3.6"可推导的事实不上 wire"：

- **`channel`**：物理落点已经决定了它（RPC 响应 / `segment.finished` / `toolCall.error`）。**前置顾虑已核实排除**：`errors.go` 里没有任何两个不同含义共用一个 symbolic type；唯一跨通道复用的 `provider_error` 在两个通道里**是同一件事**（provider 请求失败的兜底），所以删掉 `channel` 不产生歧义。文档里那句"同名不同物靠 channel 区分"是过时表述，随字段一起删。
- **`retryable`**：它想说"暂时性"，客户端却会读成"这次 mutation 可以安全自动重试"——后者只由 method + Idempotency-Key 决定。而暂时性本身是 `type` 的属性（`rate_limited` / `timeout` / `provider_unavailable` 暂时，`invalid_api_key` / `provider_rejected` 不暂时），**在错误码表里逐 type 写明即可**。**不改名 `transient`**——改名只是把一个可推导字段换个说法。

### 11.3 错误必须可行动

至少要能区分、且各自对应一个明确客户端动作：stale segment · revision conflict · interrupt no longer open · replay cursor invalid · replay unavailable · **run waiting** · **run finished** · capability not negotiated · idempotency in progress / conflict。下一步是 refetch、重订阅、冷恢复还是提示用户，必须由 `type` 决定，**永不 substring-match `detail`**。

---

## 12. 机器契约与生成体系

### 12.1 目标：Go 契约注册表

```text
Go DTO / union metadata  +  Method Registry
  （method name · params · result · stream · errors · mutation/idempotency · capability/stability）
        ├─ dispatcher 装配或校验
        ├─ OpenRPC
        ├─ JSON Schema bundle
        ├─ TypeScript wire types
        ├─ canonical sample 的**索引与缺口检查**（不是 fixture 本身）
        └─ 人读 API reference
```

**method 名不能在 dispatcher、前端、Markdown、测试里手写四遍。**

**样本必须保持独立【重要陷阱】**：canonical sample **不能**由 Registry 生成后再拿去验证 Registry——那是同源自证，闸会永远绿。Registry 只负责生成**样本索引 + 缺口检查**（哪个变体没有样本），真实 wire fixture 仍是**人工审阅**的，并由 Go / TS / schema **三方各自独立**验证。这也是 golden sample 与 schema 互为补充、彼此不可替代的原因。

**实现方式已裁决（§19.2 第 6 项）**：**显式 typed Go descriptor + 显式 union metadata，字段 schema 用 reflection**。理由——纯 reflection 拿不到方法表、错误集、stability、幂等性这些无法从类型推导的事实；AST 注解则不可编译检查、review 时看不出效果。显式 descriptor 是**可编译、可 review、可被 dispatcher 与生成器共同消费**的那一份，reflection 只干"结构体字段 → schema"这件机械活。

### 12.2 为什么不引入 TypeSpec / CUE 作新 SSOT

Schema-first DSL 会带来新语言与工具链、生成 Go 类型的可读性与 `time.Time`/泛型/union 适配问题、delivery 业务代码与生成类型之间的新映射层、以及同时维护两套的认知成本。Go 已是 runtime 主实现 → **先让 Go 契约可生成**。只有当 union/schema 生成被证明无法可靠完成时，再评估——不预先加一层。

### 12.3 前端信任边界【设计判断，注意范围】

生成**预编译** validator，而不是在高频路径解释通用 JSON Schema：

- 普通 RPC response：完整验证；
- authoritative / terminal 事件：完整变体验证；
- **高频 delta：只验 envelope + `type` + 该变体必需字段**（别扩成全量校验）；
- 未知字段允许；单个坏事件记录并丢弃，不杀整条流；
- **terminal authoritative 事件验证失败必须触发冷恢复，不能静默当作完成。**

### 12.4 CI drift gate

`go generate` 后 worktree 无 diff · Registry 与 dispatcher 方法集完全一致 · OpenRPC / JSON Schema 可解析 · TS 产物可编译 · Go 与 TS 对 canonical samples 双向验证 · 与上一 protocolVersion 做 compatibility diff · 文档 protocolVersion 与生成 manifest 一致 · error 符号名与数字码表一致。**golden samples 是必要补充，不能替代 schema 与生成客户端。**

---

## 13. SDK 人体工程学

理想形态：

```ts
const stream = await client.runs.start({ sessionId, input: [{ type: "text", text: "Fix the failing tests" }] });
for await (const event of stream.events) state = foldRunEvent(state, event);
```

**SDK 负责**：request id · `_meta` · Idempotency-Key · SSE 解析 · ack 与 notification 分离 · 去重 · 重连/replay · replay 不可用时给冷恢复信号 · typed error · **abort 只取消订阅、不误取消 Run** · reducer · 未知可选事件的前向兼容。

**SDK 不负责**：替用户批准工具 · 无幂等保证时自动重试 mutation · 推断 provider/model · 从文本猜 error type · 从 delta 重建唯一历史 · 把 transport 连接身份暴露给业务 UI。

---

## 14. 验收判准：人体工程学与 agent 心智模型

### 14.1 对人（客户端作者）

1. **一个读法**：所有 list 是 `Page<T>`；所有联合看 `type`；所有 id 自带类型前缀。
2. **最小路径要短**：`sessions.create` + `runs.start` + 消费 `item.*`/`segment.finished` + `items.list` 就能用。
3. **渲染不需要 join**：interrupt 自包含、`items.list` 顺带返回 `runs`、`RunRef` 自带 model/provider。
4. **错误可分支**：按 `type` 分支，每个错误对应一个明确动作。
5. **丢帧不致命**：丢掉所有 ephemeral 仍得正确终态。
6. **无隐式全局状态**：没有连接级 active project、没有必须先握手、没有调用仪式顺序。

### 14.2 对 agent（模型与工具侧）

1. **Item 是 agent 工作的自然单位**（消息/推理/计划/提问/工具调用），不是 UI 单位——所以流式与回放共用一套原语。
2. **工具形状与模型看到的 tool schema 同构**，协议层不做翻译，也就不会翻错。
3. **"等人"是常态不是失败**：interrupt 是可恢复状态，不是 error——否则客户端要把正常流程写进 catch 分支。
4. **预算/步数上限是终态不是异常**：`maxSteps` / `maxBudget` 是 outcome 变体并带 `detail`（花了多少 / 上限多少）。
5. **共享可变态用整份快照**：与模型 `todo_write` 的全量替换语义一致，不要求模型维护增量。
6. **工具失败不终止 run**：agent 据错误换方案继续，这是它自纠错能力的前提。

---

## 15. 已核实的现存债

| # | 主张 | 证据（符号级） | 判定 |
|---|---|---|---|
| 1 | parked run 在 wire 上被报成 `finished`，`RunOutcome` 混入 `interrupt` | `domain/execution/run.go`（注释明写 interrupt 不是 Outcome）· `delivery/protocol/runs.go` · `delivery/server/presenter_run.go`（`Interrupted → RunStatusFinished`） | **成立，第一优先级根因**；且与 wire 已有的 `SessionStatus.waiting` 自相矛盾 |
| 2 | `runs.steer` 无执行实例前置，输入退化为 string | `delivery/protocol` 的 `SteerRunRequest{runId, message}` | 成立 |
| 3 | `runs.subscribe` 只绑 runId；不存在/已结束/停车中三种情况同一个 `run_not_found` | `delivery/protocol` 的 `SubscribeRunRequest{runId}` · `delivery/server` SubscribeRun | 成立 |
| 4 | replay cursor 契约与实现不一致 | `application/runs` Coordinator 的进程级定宽计数器 vs `API.md §2.4`"eventId 从头"；`Journal.Subscribe` 只做 `cursor >` 字符串比较、无归属校验；计数器随进程重启归零 | 成立。**本篇上一版说"实现是安全的一方"是错的**：它只少一种冲突，重启后的旧 cursor 会静默跳过 replay（§7.3） |
| 5 | MCP 工具 identity 文档与代码不同（点号 vs 下划线） | `mcp/tools.go` 组名 + sanitize vs `API.md §4.4` 两处 | 成立，**最贵**（identity 是审批规则/展示注册表/日志共用的匹配值） |
| 6 | `TRANSPORT.md` 残留不存在的 `background.subscribe` | 该文档多处提及；dispatcher 无 `background.*` | 成立 |
| 7 | `TRANSPORT.md` 续流路径写成 `/v2/rpc/runs.subscribe` | 同文档 §6.1 只有 `POST /v2/rpc` 且明写"URL 不再重复 method" | 成立，**同一文档自相矛盾，且错的那句在重连路径上** |
| 8 | 类型目录漏项 | Go 有 `RunProgress.contextTokens`、`TodoSnapshot.blockedReason/nextAction`、`Provider.embeddingCapable/defaultEmbeddingModel`，`API.md` 对应 schema 未列 | 成立 |
| 9 | dispatcher 有文档未描述的方法 | `mcp.configs.{list,configure,remove,setEnabled,test}` + `mcp.servers.authorize` | 成立 |
| 10 | goal 状态靠前端轮询 | `chat/goal/application/goalData.ts` 的 `refetchInterval` | 成立（根因是控制面无推送落点，见 §16 P2） |
| 11 | 前端 `idempotency_conflict` 数字码与后端不一致 | 前端 `rpc/types.ts` 的 `RPC_IDEMPOTENCY_CONFLICT = -32015`，后端 `CodeIdempotencyConflict = -32020`（`-32015` 在后端表里根本没分配）；且这张手写镜像表停在 `-32016`，缺 `-32017…-32021` 全部 | **成立。本篇上一版判它"不成立"是我查错了文件**（查的是 `rpc/errors.ts`、搜的是 wire 符号名而非常量名）。**治本不是改那个数字**：协议要求客户端按 `type` 判错、数字码只作粗分类，而这张表除了在 barrel 里 re-export **没有任何消费者** → 删掉整份手写镜像（要用就从 §12 的注册表生成） |
| 12 | interrupt 边界不带计量 | `presenter_run.go` 的 `Interrupted` 分支返回的 `RunOutcome` 无 `Result` | 成立（§5.3） |
| 13 | `workspace.subscribe` 承载 runtime 级事件 | 其事件集含 `mcp.serverChanged` / `skills.changed` / `schedules.fired` | 成立——**今天就已经违反"`workspace.*` 只表示工作树"这条领域根规则**（§16 P2） |

**结论**：`API.md` 头部那句"canonical, frozen baseline"与"靠 review 保持同步"不能同时成立。"frozen"这个词只应在 §12.4 的 drift gate 全绿之后使用。

---

## 16. 路线图

> **breaking change 已获授权（2026-07-27）**：不再逐批等批准，但纪律不变——每批一个可独立 revert 的 commit、动 wire 就 bump `protocolVersion`、前后端与 golden sample 同批更新、全绿才 commit。**"允许 breaking"不等于"可以不先定语义"**：P1 仍要求 §19 的相关项先有结论（见下）。

### P0-A · 文档治本（纯文档，零 wire / 零 SDK surface 改动）

- MCP 工具 identity 以代码为准改 `API.md` 两处，并写清 sanitize + 截断 + 装配期 fail-closed 的边界；
- 删 `TRANSPORT.md` 的 `background.subscribe` 与 `/v2/rpc/runs.subscribe`（后者与同文档 §6.1 冲突）；
- 补类型目录漏项（`contextTokens` / `blockedReason` / `nextAction` / provider 两个 embedding 字段）；
- 补 6 个未文档化的 `mcp.*` 方法；
- 删 §13 那条与已实现的 `runs.steer` 打架的 stale 条目；
- cursor 契约按**实现 + 已知漏洞**改写（opaque、相等去重、绑 segment；并写明归属校验尚未落地，见 §7.3）；
- `§14` 的语气改成诚实待办，直到 P1 契约落地。

**验收**：方法表与 dispatcher 逐项对齐；文档内不存在互相矛盾的两处描述。

### P0-B · 删除前端 business-code 镜像（**改 TS 公开 surface，不是纯文档**）

- **保留**标准 JSON-RPC 常量（`-32700 / -32600 / -32601 / -32602 / -32603`）——envelope 坏掉时 `error.data` 可能不存在，数字码是唯一信号，那是 transport 层事实；
- **删除** Lyra business numeric 镜像（`-32001…-32021` 那一段，含错值 `RPC_IDEMPOTENCY_CONFLICT = -32015`）及其 barrel re-export，并把依赖它的 RPC 单测 fixture 改成不依赖这份镜像；
- 理由：production 客户端按 `ProblemData.type` 分支、这段镜像**无业务消费者**、已漏 `-32017…-32021` 且有错值；只改一个数字等于继续保留第二真相源。将来真需要公开数字码，由 §12 的 Registry 生成。

**为什么单独一批**：常量已从 `rpc/index.ts` 导出，删除会改变 TypeScript 公开 API 并动测试——按代码改动走，不混进文档批。

### P1 · 核心生命周期治本（**破坏性，已授权**）

> **语义已冻结（§19.2 七项全部落定），可以动 shape 了。** §19.3 剩下的三项是实现期填的数值与表达形态，不阻塞本批。

- `RunStatus` 加 `waiting`；`RunOutcome` 移出 `interrupt`；`SegmentOutcome` 承载它；
- 每次 `segment.finished`（含 `interrupt`）带 authoritative 计量边界，`RunResult` 可从各段确定性聚合（§5.3）；
- presenter / `runs.list` / `items.list` / SessionStatus 对齐同一状态机；
- `runs.steer` 加 `expectedSegmentId`、输入改 `ContentBlock[]`；
- `runs.subscribe` 绑 `segmentId`；
- 新增可行动错误：`run_waiting` / `run_finished` / `replay_cursor_invalid` / `replay_unavailable`；
- `interrupts.list` 取代 `runs.listOpenInterrupts`（§19.2 第 4 项）；
- 删 `durationMs`、改 `activeDurationMs`；删 `ProblemData.channel` 与 `retryable`（§19.2 第 2、5 项）。

**影响面**：wire + 前端 reducer + golden samples + `protocolVersion` bump。**验收**：domain / application / wire / 前端使用同一状态机；running↔waiting↔finished 全部 transition 有测试；Waiting 可 cancel；Finished 永不可 resume；stale steer 确定性拒绝。

### P1 · 可执行契约（与上一条可并行）

Registry → OpenRPC + JSON Schema + canonical samples 补全 + §12.4 的 drift gate。**先只做这一圈**；TS validator 只覆盖 authoritative/terminal 帧，typed client 与 compatibility diff 等有真实第三方客户端再说（YAGNI）。

### P2 · 控制面收敛（**破坏性，与 P1 同版本；"先测量"已撤销**）

**根因【已核实】**：非当前流的 run / 会话 / goal 状态没有推送落点——goal 模式在前端定时轮询 `goals.get`，schedules 触发的 headless run 只能靠 `schedules.fired` 提示去重拉列表。同时 `workspace.subscribe` 今天就已经承载 `mcp.serverChanged` / `skills.changed` / `schedules.fired`——**五种事件里只有 `files.changed` 和工作树 resync 真属于 workspace**。

**已裁决（§19.2 第 7 项）**：**用一条带非空 `topics` 的 `runtime.subscribe` 完整替换 `workspace.subscribe`**，同版本一次迁移、**不双发**。

**两次撤回，理由要留着**：
- 撤回本篇最初的"塞进 `workspace.subscribe`"——那违反 §3.7 自己写的领域根规则，是拿"已经有一条流"当理由寄生到错误的根上；
- 撤回上一版的"先测量再决定"——**决定性论据不是轮询成本，是连接预算**：`TRANSPORT.md §6.5` 已确认，明文 loopback 上浏览器 / WebView **只走 HTTP/1.1、每 origin 约 6 条并发连接**（不支持 h2c），而**每条活跃流占一条且整段不释放**。所以"常驻长流"是一个**只有 6 格的预算**：留两条常驻（workspace + runtime）就吃掉两格，还要和活跃 run 抢。**一条可过滤的流不是审美选择，是预算算术。**

**形状**：

1. `runtime.subscribe{ topics, watches? }`——per-call 流、符合 Streamable HTTP、不是 server→client request、不维护客户端连接身份；`topics` **必须非空**（不给"什么都订"的懒惰入口，也就不会退化成全局广播）。
2. 事件**只做 invalidation**，权威事实一律由 sessions / runs / goals / interrupts / workspace 查询面提供；`resync` 让客户端在丢帧或重连后统一 refetch。
3. **`files.changed{watchId, paths}` 不违反"只做 invalidation"**：paths 正是"该重取哪些"的作用域，权威内容仍来自 `workspace.readFile`。watch 注册留在 subscribe 参数里（`watches`），是订阅参数、不是事件形状问题。
4. 迁移与 `protocolVersion` + capability + 生成 SDK **同批一次完成**；dev 阶段**不留双发兼容层**（双发 = 两个真相源）。落地后删掉 goal 轮询。

**不回退到常开、无过滤的全局 SSE 总线**——`topics` 非空 + per-call 正是与它的分界。

### P3 · 加固与便利

flat union 的 encode 边界 `Validate()`；三级可靠性术语落进文档与 capability `limits`；`ProblemData` 的 `channel` / `retryable` 决策（§11.2）；事件能力声明简化（§10.2）；parked executor 保温 grace period；生成高层 SDK 句柄（§13）。

---

## 17. 新增协议表面：十问自检

> 任何新方法 / 新事件 / 新字段落地前逐条回答，答不上就先别写。

1. 它是新资源、已有资源的新状态，还是一次动作？会不会造出第二套 Run/Task/Job 心智？
2. 判别字段是 `type` 吗？枚举开放还是闭合（会被插件扩展吗）？
3. 若是事件：authoritative / replayable / ephemeral 哪一类？**ephemeral 的权威落点能指名吗**？冷恢复从哪来？
4. 它属于核心（sessions/runs/items）还是旁路？根选对了吗？
5. 客户端能只靠这一帧渲染吗，还是要额外 join？
6. 最小客户端不实现它会坏吗？（不会 → 必须 capability 门控且缺省关闭）
7. 会不会让 server 需要主动向某个客户端发 request？（会 → 重新设计成 R 模型）
8. 是 query / mutation / stream？需要 Idempotency-Key 吗？需要 `expectedSegmentId` / `expectedRevision` 这类并发前置吗？admission 成功到底意味着什么已经 durable？
9. 命名过关吗：`<domain>.<verb>` · camelCase · 缩写在白名单 · 单位后缀已登记 · 名字对应用户/agent 的真实心智而非内部实现？
10. 有 canonical sample 吗？Registry / OpenRPC / JSON Schema / TS 是否由同一次改动生成？

---

## 18. 明确不做

| 不做 | 判据 |
|---|---|
| REST 影子面（业务方法的 read-only GET 影子） | 一个业务操作两套入口 = 两个真相源 |
| 常开全局 SSE 总线 | 需要连接路由 + 订阅管理 + fan-out；与"一次操作一条流"冲突 |
| 强制连接握手 | 适合双向长连接，不适合 stateless HTTP / in-process 语义一致 |
| server→client JSON-RPC request | 把 HITL 绑到某条连接与某个客户端，还要额外发明孤儿态清理 |
| per-tool Item 联合膨胀 | 每类新执行语义都要动 wire 类型 + 分裂审批方法（Codex 的实际代价，§2） |
| per-event 版本化 / V1·V2 并存 / legacy shim | 事件源化的必然后果（opencode v2 的现实代价）；我们按日期 bump 整协议 |
| 深点号嵌套事件名 | 判别应在 `type` 而不在名字层级 |
| 业务错误映 HTTP status | HTTP status 只反映传输层 |
| 把进程内 SDK 消息联合当 wire | 里面是 hook/plugin/auth/local command/调试细节，不是稳定跨语言契约 |
| 依赖 delta 或活进程保证正确性 | 桌面休眠 / 退出 / 断网时全部失效 |
| runtime 层的 user / account / 多租户 | 鉴权由更外层解决 |
| 客户端自选业务资源 id · JSON-RPC batch · stdio transport · 多根 workspace | 已在 `API.md §13` 定案；多根是未来破坏性改动，须先咨询 |

---

## 19. 决策簿（持续演进的唯一账本）

> 后续讨论不从"四家谁更好"重开。**已收敛的当基线，只有新事实才重开**；待裁决的逐项定，定完就地记录"结论 / 理由 / 影响 protocolVersion / 被替代方案"。**这份账本只存在于本篇**（对照稿不复制，避免同一问题在两处漂移）。

### 19.1 已收敛，不建议反复讨论

- Session → Run → Segment → Item 四层，Run 跨 resume 稳定；
- Run 有 `running / waiting / finished`，**Interrupt 不是 RunOutcome**；
- `runs.steer` 需要 `expectedSegmentId`；subscribe 与 cursor 必须绑 Segment；
- authoritative / replayable / ephemeral 三级分开，ephemeral 必有权威落点；
- 通用 `ToolInvocation`，不扩 per-tool wire union；
- Go 契约注册表导出 OpenRPC / JSON Schema / TS wire types；golden sample 是独立补充、不替代 schema；
- `workspace.*` 不承载 runtime / session / goal 控制面；
- server→client request、REST 影子面、无过滤的全局广播总线不进核心协议；
- 业务错误不映 HTTP status；客户端按符号 `type` 判错。

**本轮（2026-07-27）新增收敛**：

- **breaking change 已获授权**——P1/P2 不再逐批等批准，但每批仍要独立 revert 的 commit + `protocolVersion` bump + 前后端同批（§16 抬头）；
- 数字码：**保留标准 JSON-RPC 5 个常量，删除 Lyra business 镜像**；真需要公开时由 Registry 生成（§16 P0-B）；
- **canonical sample 不由 Registry 生成**——那是同源自证；Registry 只出索引与缺口检查（§12.1）；
- 控制面归位是**与版本同批的一次迁移**，dev 阶段不留双发兼容层（§16 P2）；
- 对照描述要分级：opencode 的 volatile 全局流与 per-session durable 流是**两层**，不能混写（§2）；Codex 强类型 item 的代价限定为"每类新**执行类别**动三处"，不是"每个新工具"（§2）。

### 19.2 本轮裁决结果（2026-07-27，breaking 已授权）

> 七项全部落定。每条格式：**结论 / 理由 / 影响 / 被替代方案**。落地批次见 §16。

| # | 结论 | 理由 | 影响 | 被替代方案 |
|---|---|---|---|---|
| 1 | **计量 = Run 累计快照**：每次 `segment.finished`（含 `interrupt`）与 terminal `RunResult` 同口径，含 subagent 子树；subagent `RunRef` 另带自身用量 | **幂等**：累计快照重放不多算，per-segment 增量一被 replay 就双计；单段值可两快照相减推导 | wire + presenter；`protocolVersion` bump | per-segment 增量值 |
| 2 | **duration 只留 `activeDurationMs`**（不含 Waiting，累计）；删 `durationMs`；墙钟由 `createdAt`/`finishedAt` 推导；单段时长归 observability | §3.6：可推导的不上 wire；一个字段一个含义 | wire + 前端读法；bump | 保留 `durationMs` / 同时给 `elapsed` + `active` |
| 3 | **cursor**：params 带 `{runId, segmentId}` 寻址；opaque cursor 内含 `version+epoch+segment+sequence`；仅当前进程有限窗口，retention 进 `limits`；`replay_cursor_invalid` 与 `replay_unavailable` 分开 | 一次解决跨段误用、重启 epoch、窗口淘汰、冷恢复分支四件事；cursor 自带 epoch 才能在带外校验 | `runs.subscribe` 签名 + journal + SDK；bump | 流身份进地址（opencode 形态）——更干净但要求"一条流一个 URL" |
| 4 | **`interrupts.list` 取代 `runs.listOpenInterrupts`**，P1 同批 | durable 待处理项是独立资源，"收件箱"心智；breaking 已授权且同批无额外成本 | 方法名 + 前端；bump | 保留旧名（本篇上一版立场） |
| 5 | **删 `channel` 与 `retryable`**；不改名 `transient`；留 `retryAfterSeconds` | §3.6：落点已表达 channel、`type` 已表达暂时性；已核实无跨通道同名歧义 | `ProblemData` + 错误码表逐 type 标注暂时性；bump | 保留 `channel` 作脱上下文自描述 / `retryable`→`transient` |
| 6 | **Registry = 显式 typed Go descriptor + 显式 union metadata + 字段 reflection**；dispatcher 与生成器消费同一份 | 方法表/错误集/stability/幂等性无法从类型推导，必须显式；AST 注解不可编译检查 | 新增构建产物 + CI gate；无 wire 改动 | 纯 reflection / AST 注解 codegen / schema-first DSL |
| 7 | **`runtime.subscribe{topics, watches?}` 完整替换 `workspace.subscribe`**，同版本一次迁移、不双发；`topics` 必须非空 | **连接预算**：loopback HTTP/1.1 每 origin ~6 条、每条活跃流占一条不释放，常驻两条流是预算算术问题不是审美；且现有流已混了三类非工作树事件 | 事件集 + 前端订阅 + SDK；bump | 继续轮询（先测量）/ 只加不换 / `sessions.subscribe` |

### 19.3 仍开放（实现期定，不阻塞 P1）

- replay retention 的**具体数值**（窗口时长 / 事件数上限）——按实测填进 `limits`；
- `runtime.subscribe` 的 **topics 枚举最终集**（至少覆盖 session 状态 / goal / interrupt 失效 / mcp / skills / schedules / workspace 文件）；
- Registry 的 union metadata 表达形态（能否可靠生成 `oneOf + discriminator`）——以实测裁决，失败则退回"只生成方法表 + 字段 schema"。

---

## 附录 · 参考

**同期独立复核稿**：[`codex_runtime_api_design_guide.md`](codex_runtime_api_design_guide.md)（分工见文首）。

**外部（一手）**：Codex app-server 规范 · opencode v2 `packages/{protocol,schema}` 源码与在线 API 文档（**注意分级**：公开稳定文档 / Experimental 文档 / 仓库 HEAD 不是同一级契约，其公开 `/api/event` 与源码里的 durable session event 不能混写成一个结论）· Claude Code headless / Agent SDK 文档与其 bridge 的 `control_request` 实现。

**本地**：`app/desktop/docs/protocol/{API,AUX_API,TRANSPORT}.md` · `app/runtime/internal/delivery/{protocol,dispatch,server}` · `app/runtime/internal/application/runs` · `app/runtime/internal/domain/execution` · `app/desktop/frontend/src/rpc` · [`../runtime/CLAUDE.md`](../runtime/CLAUDE.md) · [`../runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md`](../runtime/doc/EXECUTION_CENTERED_ARCHITECTURE.md)
