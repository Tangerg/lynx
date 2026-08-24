# Lyra Runtime Protocol（定稿 `2026-08-24`）

> **状态：正式契约（canonical）。** 本文是 Lyra 客户端 ↔ Lyra Runtime 的 wire 契约真相源之一。物理传输见同目录
> [`TRANSPORT.md`](./TRANSPORT.md)，旁路能力见 [`AUX_API.md`](./AUX_API.md)。
>
> **本文不是字段目录。** 字段级真相只有一处：Contract Registry 生成的制品（§14）——
> [`app/runtime/contract/schema.json`](../contract/schema.json)（所有 shape 与跨字段约束）、
> [`openrpc.json`](../contract/openrpc.json)（方法的入/出 schema）、
> [`manifest.json`](../contract/manifest.json)（方法 / 能力门禁 / 错误注册表 / union / state 策略 /
> canonical 样本索引）、[`API_REFERENCE.md`](../contract/API_REFERENCE.md)（人读索引：每个方法的
> kind、幂等性、所需 feature、可能错误）。
>
> **本文写的是生成物写不出来的东西**：语义、不变量、"为什么不能是另一种形状"、以及跨方法的走查。一个事实一个作者
> —— 本文一旦重述字段表，它就成了第二份会腐烂的真相。
>
> `protocolVersion`: **`2026-08-24`**（本 build 只服务这一个精确版本，旧版本请求确定性返回
> `invalid_protocol_version`，见 §12）。

---

## 目录

- §0 模型与概念（Workspace/Session/Run/Item · R 模型 · Run vs Segment）
- §1 Wire 格式（JSON-RPC 2.0 / §1.1 信封规则）
- §2 命名规范（**一个判别字段 `type`**）
- §3 Lifecycle（无状态 discovery / 取消 / §3.1 端到端走查 / §3.2 Minimal Profile）
- §4 类型语义（**字段表在 `schema.json`**；本节只写语义与不变量）
- §5 流式（事件信封 / 可靠性不变量 / state 边界 / Run 树）
- §6 Human-in-the-Loop（R 模型）
- §7 方法语义（**方法表在 `API_REFERENCE.md`**；本节只写各域的语义与约束）
- §8 错误（三个落点 + 符号名判别 + ProblemData）
- §9 Capabilities 与请求能力
- §10 历史 / 重连 / 恢复
- §11 两个扩展缝：Item / state（选择指南）
- §12 版本规则
- §13 明确不做
- §14 机器可读制品 / 漂移闸
- §15 安全不变量汇总
- 附录 A 设计不变量摘要 · 附录 B 设计借鉴 · 附录 C 扩展说明

---

## 0. 模型与概念

Lyra Runtime 是一个**本地、领域中立的 agent runtime**。客户端可以是 web UI、桌面外壳、TUI 或另一个本地进程。协议是
**JSON-RPC 2.0**，当前通过 streamable HTTP 承载。协议语义不依赖 HTTP 实现细节。

**"领域中立"是核心设计立场**：协议核心只懂 Session / Run / Item / 通用工具调用这套**通用原语**；"某个工具长什么样、
该怎么富渲染"是**领域知识**，不焊进 wire（见 §4.4）。换个领域（客服 / 数据分析 / 运营）协议核心一字不改。

### 0.1 资源层级模型

```text
Workspace          文件、配置发现与执行的显式资源根              ref: {path}
  └─ Session       会话；绑定一个 WorkspaceInfo                  id: ses_…
       └─ Run      一次 agent 执行，从用户输入到一个 outcome      id: run_…
            └─ Item run 内一个持久化工作单元                      id: item_…
```

- **Workspace**：文件系统资源根。wire 只以 `WorkspaceRef{path}` 寻址，以 `WorkspaceInfo` 回报规范化身份、
  `projectRoot` 与 `availability`。
- **Session**：一次对话，绑定一个 `WorkspaceInfo`。
- **Run**：一次 agent 执行。它有**三个**持久化状态（`RunStatus`）：`running`（在执行）/ `waiting`（停在人身上）/
  `finished`（终结，带 `RunOutcome`）。`waiting` 不是 `finished` 的一种打扮 —— 一个等人回答的 Run 既没有结束、也没有
  在烧钱，把它记成任何一个都会让列表说谎。
- **Item**：run 内一个持久化工作单元 —— `userMessage` / `agentMessage` / `reasoning` / `plan` / `question` /
  `toolCall` / `compaction`。

**`Item` 是历史与流式的唯一原语**：流式推 Item 生命周期事件（`item.started → item.delta* → item.completed`），
历史 `items.list` 返回持久化 Item。**没有独立的 `Message` 资源类型**；"消息"就是 `userMessage`/`agentMessage`
两种 Item。

### 0.2 Workspace 资源模型

- `WorkspaceRef.path` 是**资源身份 + 文件系统工具根**；它是值对象，不是服务端铸造的不透明 id。
- runtime **不持有连接级 active workspace**。每个作用域方法必带 `workspace: WorkspaceRef`；只有
  `workspaces.resolve{ref?}` 与创建 Session 时允许省略，省略即使用 `ServerInfo.defaultWorkspace`。
- `WorkspaceInfo.availability` 显式表达资源根当前是 `available` 还是 `missing`；失联不是另一个布尔字段，也不改变身份。
- `workspaces.list` 返回按 Session 绑定聚合的 `WorkspaceSummary`；无 active 标记，不制造平行的 Project 资源。
- `projectRoot` 只是**配置发现根**（向上找到最近含 `.git` 的祖先，找不到回落 workspace path），不取代
  `WorkspaceRef` 作为身份与工具根。
- SDK 用 `client.workspace(ref)` 把已知身份一次绑定到 `files/diff/changes/skills/recipes/agentDocs/hooks/knowledge/agentMemory`
  等子资源；`await client.workspaces.open(ref?)` 在需要规范化或默认 workspace 时先 resolve 再绑定。绑定对象冻结自己的
  `ref`，调用方后来修改原对象不能偷换已打开资源的目标；默认 resolve 只缓存成功结果，瞬时失败可重试。

### 0.3 HITL 采用 R 模型（分段续跑）

agent 需要人介入（审批 / 提问 / client 侧工具结果）时，**当前流式段（segment）结束**，以
`SegmentOutcome.type="interrupt"` 收尾、资源全释放；**Run 本身（稳定 `runId`）不结束**——它转入 `waiting`，待解项作
持久化 interrupt 集。客户端用 `runs.resume{runId}` 在**同一个 Run 上开新的一段**来应答。无"活跃挂起 run"、
无被占用的 goroutine。详见 §6。

> **Run vs Segment（贯穿全协议的身份模型）**：一个 **Run**（`runId`，`run_…`）是一次逻辑运行的**稳定身份**——
> `runs.start` 铸一次、跨所有 interrupt/resume 周期与进程重启**永不重铸**。一个 **Segment**（`segmentId`，`seg_…`）
> 是该 Run 的一个连续流式段：`runs.start` 开第一段，每次 `runs.resume` 开新的一段。事件流、重连/回放都**按 Segment**
> 划界；Run 的生命周期跨其全部 Segment。
>
> **Segment 与 Run 的终态是两个词汇**：`segment.finished{outcome}` 用 `SegmentOutcome`（多出 `interrupt` 与
> `suspended` 两个变体——它们是"这一段停了、Run 还在"），Run 的终态用 `RunOutcome`（只有真正结束的五个）。两份枚举
> 不合并，是因为合并后每个读 Run 终态的地方都得先排除两个不可能的值。
>
> **`activeSegmentId` 只在 `status == "running"` 时存在**，与状态同事务持久化：一个 Run 的位置和"哪一段在驱动它"
> 不可能各说一套。要 steer 或 subscribe 就得带上它（§7.3）。

### 0.4 收敛点

- **一个下行 run 事件方法**：`notifications.run.event` 承载 run/item/state 全部事件（HTTP 上随对应流式调用的响应流
  投递，见 TRANSPORT §6.4）。
- **一个下行失效方法**：`notifications.runtime.event` 承载工作区 / 会话 / run / interrupt / goal / state 的
  失效信号（§7.8、AUX_API §3）。
- **一个终态信号**：`segment.finished{ outcome, metrics }`，判别式 outcome。
- **一个 HITL 恢复口**：`runs.resume`。
- **一个判别字段**：所有判别联合一律看 `type`（§2.1）。

---

## 1. Wire 格式（JSON-RPC 2.0）

```ts
type Message = Request | Response | Notification;

interface Request {
  jsonrpc: "2.0";
  id: string;
  method: string;
  params?: unknown;
}
interface Response {
  jsonrpc: "2.0";
  id: string;
  result?: unknown;
  error?: RPCError;
}
interface Notification {
  jsonrpc: "2.0";
  method: string;
  params?: unknown;
} // 无 id

interface RPCError {
  code: number;
  message: string;
  data?: ProblemData;
}
```

每个 Request 的 `params` 若为对象，可带 `_meta`（`RequestMeta`：`protocolVersion?` / `clientInfo?` /
`clientCapabilities?`）。`_meta` 是请求自描述元数据，不属于业务参数；runtime 在 dispatch 边界剥离后再解码具体
method params。它先按 `RequestMeta` DTO 严格解码，再走同源生成约束：未知字段直接拒绝；非法枚举、重复集合成员和空
client identity 返回带精确 `errors[].field` 的 `invalid_params`，不会绕过已发布的 metadata 形状。

### 1.1 信封规则

- envelope `id` 一律 **string**；Response 的 `id` 匹配 Request。
- Response 必须恰好含 `result` 与 `error` 之一。
- **不支持** JSON-RPC batch。
- **不支持** server→client request——HITL 用"通知 + 后续 client request"（§6）。客户端收到带 `id` 的入站
  Request 应直接丢弃。
- `params._meta` 承载协议版本 / clientInfo / clientCapabilities 这类**请求自描述元数据**；传输元数据（trace / 门禁
  token / 流游标）走带外通道，判定见 [`TRANSPORT.md`](./TRANSPORT.md) §2。
- method params 按对应请求 DTO **严格解码**：未知字段、`null`、类型不符或多余 JSON 值均返回 `invalid_params`，
  不允许服务端悄悄丢弃客户端意图。**required 字段不接受 `null`**，客户端也不发 `null` 占位。
- OpenRPC 的 by-name `params[]` 从完整 request frame 的属性直接投影；不得重新按字段 Go 类型生成一个更宽的副本。
  因而 `minLength` / `minimum` / `maximum` 等约束在逐参数视图、`x-lyra-requestFrame`、Go 与 TS validator 中完全一致。

---

## 2. 命名规范

### 2.1 判别字段：**一律 `type`（无例外）**

**所有判别联合（discriminated union）的判别字段名一律是 `type`**。`kind` **不在 wire 上出现**——无论作判别字段还是
枚举属性。这是一条无例外的硬规则：写 reducer / 序列化 / codegen 时永远只看 `type`，消除"这个看 type、那个看 kind"
的认知税与拼错判别字段的无声 bug。

> 与 `type` 区分的是**状态/分类枚举属性**（不判别对象形状，只标一个有限态）：这类字段按其语义命名（`status` /
> `decision` / `scheme` / `safetyClass` 等）。判据：它决定"对象是哪个变体"→ 用 `type`；它只是"对象的一个有限态
> 字段"→ 用语义名。

每个闭合 union 的判别值集合、每个变体的必填/选填字段，都在 `manifest.json` 的 `unions` 节里机器可读（注册期反射
校验，见 §14），本文不重述。

### 2.2 字段与枚举

- 字段名、枚举值一律 **camelCase**。
- 缩写**白名单**（写死，其余全词）：`id` / `url` / `mime` / `api`（如 `apiKey` / `apiKeyMasked`）。
- 单位用显式后缀，**后缀集也写死**（新单位先在此登记）：
  - `USD`（货币）：`maxBudgetUsd` / `inputUsdPerMillionTokens`
  - `Millis`（毫秒）：`activeDurationMillis`
  - `Seconds`：`retryAfterSeconds`
  - `Bytes`：`sizeBytes` / `maxBytes`
  - `At`（ISO-8601 时刻串）：`createdAt` / `expiresAt` / `finishedAt`

### 2.3 开放 vs 闭合枚举（原则）

- **面向插件 / 未来扩展的分类** → **开放 `string` + §2.6 命名空间**（first-party 裸符号，第三方 `plugin:` 前缀）。
  例：`safetyClass`、error `type`、`features` map 的 key。
- **客户端要穷举分支的分类** → **闭合枚举**。例：`RunStatus`、`ItemStatus`、`SessionStatus`、`StreamEventType`、
  `RuntimeTopic`、`SegmentOutcomeType`。
- 判据不是"会不会加值"，而是**"加一个值会不会让老客户端做错事"**：客户端对它写 exhaustive switch → 闭合，加值即
  breaking，必须 bump `protocolVersion`（§12）；客户端只是路由/展示未知值 → 开放。

### 2.4 资源 id（server 生成 + 类型前缀）

业务资源 id **一律 server 生成**：`ses_`（Session）/ `run_`（Run）/ `item_`（Item）/ `seg_`（Segment）/
`evt_`（Event）。客户端只生成 JSON-RPC envelope `id`。

- **id 自带类型**：前缀即资源的**自描述类型标签**，不另设 `object`/`resourceType` 字段。
- **id 不透明、不承载顺序**：除前缀外 id 内部结构**不是契约**——client **不得**按 id 字典序推断创建顺序。排序一律
  锚定**显式字段**：会话 / 历史按 `createdAt`，段内事件按 `eventId`。分页 `cursor` 同样不透明（§4.11）。
- **`eventId` 作用域**：在单个 **Segment 流**内单调——这是**唯一**被契约保证"单调有序"的 id。一条段流复用根 Run
  该段 + 所有子孙 Run 的事件。`runs.resume` 在同一 Run 上开**新的一段**（同 runId + 新 segmentId，eventId 从头）。
  client 按 `segmentId` 划定去重/续传作用域（§5、§10）。
- **`eventId` 是进程内的，`cursor` 才是可回传的续流锚**：`runs.subscribe` 的续流用 `Last-Event-Id`
  （TRANSPORT §6.4），runtime 用它定位重放起点；跨进程重启后旧位置不再存在，runtime 返回 `replay_unavailable`
  而不是假装能接上（§10.1）。

### 2.5 事件名 / 方法名

- 事件名小写 `domain.action`：`segment.started` / `segment.progress` / `segment.finished` / `item.started` /
  `item.delta` / `item.completed` / `plan.updated`；失效信号同形：`files.changed` / `runs.changed` /
  `plan.changed` / …（§7.8）。运行时行为必须使用一等事件或 Item 类型。
- 方法名 `<domain>.<verb>`，HTTP URL 保留点（不斜杠化）。例：`runs.start` / `items.list` / `mcp.servers.list`。
- **一件事一种拼写**：信号名与它的资源同名（`runs.changed` 对 `runs.*`、`plan.changed` 对 `plan.get`），不出现
  `mcp.serverChanged` 这类"动词挪进名词"的第二种拼法。

### 2.6 第三方扩展命名空间（防撞名）

**唯一**的可枚举标识命名约定，统一适用于所有"first-party 与第三方共用一个 keyspace"的扩展缝：

- **first-party**（runtime 自身 / 内置）用**裸符号**（`session_not_found` / `progress`）。
- **第三方插件**产出的同类标识一律加前缀 `plugin:<pluginName>/<symbol>`（如 `plugin:acme/progress`）。

适用面：error `type`（§8.4）、开放枚举值（§2.3）、`features` map 的 key（§9）。
client 路由：裸符号按 first-party 集匹配；`plugin:` 前缀按 `<pluginName>` 分发。

---

## 3. Lifecycle

```text
discover（可选）→ operate → host/transport disconnect
```

runtime 是无状态 JSON-RPC 服务：业务方法**不要求**先调用 discovery。client 可在启动时调用 `runtime.discover`
读取完整 serverInfo / capabilities；HTTP client 也可用 `GET /v2/info` 读取最小 binding 与进程身份，但这些都只是
信息查询，不改变连接状态。`ServerInfo.instanceId` 每次 Runtime 进程启动都重新生成，不是持久 identity、replay scope
或 idempotency namespace。

协议版本、clientInfo、clientCapabilities 随每个 request 的 `params._meta` 发送。server 用这份 request-scoped
metadata 判断本次 run / subscription 能否产出某些事件或 HITL interrupt；不把 client 能力写成 runtime 进程全局状态。

取消有两个不同对象：

| 对象                    | 信号                                 | 效果                                                 |
| ----------------------- | ------------------------------------ | ---------------------------------------------------- |
| 在飞的 JSON-RPC request | transport context / HTTP abort       | 取消一个慢请求                                       |
| agent run               | `runs.cancel`（request，带 `runId`） | 硬终止 Running / Waiting run，并返回已提交的终态快照 |

网络断开**不**取消 run。

### 3.1 一次 run 的端到端走查

```text
runs.start ──▶ segment.started ──▶ (item.started → item.delta* → item.completed)*  ──▶ segment.finished{outcome, metrics}
                                  └ assistant message / reasoning / toolCall 逐个流式落地            │
                                                                                                    ├─ completed / error / … → Run finished
                                                                                                    └─ interrupt → Run waiting（见下）
```

1. **起 run**：客户端 `runs.start{ sessionId, input }`，**立即**返 `{ runId, segmentId, userItemId }`，同一条流随即推
   `RunEvent`（§5）。会话已有非终态 root run 时**不隐式取消它**，而是返回 `session_has_active_run`（§7.3）。
2. **流式产出**：先 `segment.started{run}`，然后每个 Item 走 `item.started`（壳）→ `item.delta*`（文本 / 工具入参 /
   输出增量，§5.1）→ `item.completed`（权威终态）。
3. **需要人介入**（HITL，§0.3 / §6）：当前**段**以 `segment.finished{ outcome:{type:"interrupt", interrupts} }`
   收尾、资源释放；**Run 转 `waiting`**，待解项持久化（跨重启可经 `interrupts.list` 发现）。
4. **续段**：客户端 `runs.resume{ runId, responses, input? }` 在**同一 Run 上开新的一段**（同 `runId`、新
   `segmentId`），又一段 `segment.started → items → …`。**所以一个"对话回合"= 一个 Run 的若干 Segment**。
5. **收尾**：某一段以 `segment.finished{ outcome: completed | error | maxSteps | maxBudget | canceled }` 终结整个
   Run，Run 转 `finished` 并带同名 `RunOutcome`。
6. **历史 vs 实时**：挂载/重开会话用 `sessions.snapshot` 重建完整 material view，独立历史浏览用
   `items.list`（§10）；实时流内按 `eventId` 排序（§5）。两套各自权威，靠 item id 关联。

### 3.2 Minimal Profile（最小可用客户端）

只想做"发消息 → 流式显示回复"的客户端**最少**只需 `sessions.create` + `runs.start` + 消费 `item.*` /
`segment.finished` + `items.list`（`runtime.discover` 可选），其余全是**分层可选能力**（各由 §9 capability 门控）。

**Minimal Profile 的含义是"少声明"，不是"少收"**：client 在 `ClientCapabilities.interruptTypes` 里声明它能应答哪些
interrupt，声明为空即"不处理 HITL"，server 必须不产出它解不了的 interrupt（§6.2）。**没有"客户端枚举所有可理解事件"
这一说** —— run 的核心生命周期事件是 runtime 必发的，一个连 `segment.finished` 都不折的客户端不是子集实现、是坏
实现；客户端能抑制的只有 ephemeral 预览（`excludedEphemeralEvents`，§9）。

一个 Run 在创建时把它**实际**依赖的能力冻结成持久化的 `RunProtocolProfile`（`requiredFeatures` +
`interruptTypes`），并在 `segment.started.run` 上报同一份。后来的 `runs.resume` / `runs.subscribe` 若声明不了该
profile，会被**拒绝**而不是降级投递——降级等于给同一个 Run 开第二条更安静的事件流（§9）。

---

## 4. 类型语义（schemas）

> **字段表在 [`schema.json`](../contract/schema.json)**（含跨字段 presence 规则）与
> [`API_REFERENCE.md`](../contract/API_REFERENCE.md)。本节只写生成物表达不了的语义、不变量与
> "为什么不是另一种形状"。小节编号与生成物里的类型名一一对得上。

### 4.1 Session / Workspace

`Session` 是会话聚合：身份（`id` / `title` / `createdAt`）、绑定（`workspace: WorkspaceInfo`）、默认
`provider` + `model` 精确选择、
派生 `status`（`running|waiting|idle`）与乐观并发的 `revision`。

- **`revision` 是条件写的唯一凭证**：`sessions.update` 必带 `expectedRevision`，过期返回 `revision_conflict`
  （客户端重拉聚合再合并）。没有"最后写赢"的路径。
- **`status` 是派生的**：它由该会话的 run 推出来（有 running run → `running`；有 waiting run → `waiting`；否则
  `idle`），不是一个可以单独被写坏的字段。
- **`workspace.availability="missing"`**：资源根在磁盘失联 → 降级纯聊天 + 可 relocate；不是错误态，是客户端要显式渲染的事实。
- `WorkspaceSummary` 是按 `workspace.ref` 分组的派生读模型（`workspaces.list`），不是另一个可写聚合。

### 4.2 Run（`RunSummary` / `RunRef` 分层）

同一个 Run 有**两个投影**，因为两个读者要的不是一回事：

| 类型         | 谁给                                                             | 载什么                                                                 |
| ------------ | ---------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `RunSummary` | `items.list.runs` / 归档，且嵌入每个 `RunRef`                    | 身份 + 血缘 + 状态 + 终态：列一行、连一棵树够用                        |
| `RunRef`     | `runs.get/list` / `segment.started.run` / `runs.cancel` 成功结果 | Summary + `metrics` + `limits` + `activeSegmentId` + `protocolProfile` |

- **`status` 与 `outcome` 正交**：`outcome` 只在 `finished` 时存在，`activeSegmentId` 只在 `running` 时存在，
  `finishedAt` 只在 `finished` 时存在。这三条是 schema 里的 presence 规则，不是约定。
- **`metrics` 与 `outcome` 分家**：`RunMetrics{usage, steps, activeDurationMillis}` 是"花了多少"，`RunOutcome` 是
  "为什么停"。旧形状把两者塞进一个 `result`，于是"取消掉的 run 花了多少钱"没有位置放。`metrics` 单调不减，
  `activeDuration` **不含等人的时间**（一个停在审批上过夜的 Run 不因此变贵）。
- **`limits` 是一份冻结的 Run-tree policy**：`maxTotalTokens`、`maxSteps`、`maxBudgetUsd` 都跨 child 与 resume
  累计；其中 `maxTotalTokens` 统计 prompt + completion，而 `params.maxTokens` 只约束一次模型输出。两者不同名，
  避免 SDK 和 Agent 把累计预算当成 generation 参数。
- **血缘三字段全有或全无**：`spawnedByItemId` / `parentRunId` / `rootRunId` 要么都在（child），要么都不在（root）。
  半连的树导不出来。`features.subagents` 关闭时不产生 child run，这三个字段恒空——**shape 在、行为关**，这是
  刻意的（见 §13）。
- **没有 `mode` 字段**：agent/chat/plan 这套 per-run 模式已移除——run 永远是带工具的 agent 循环，"计划"是一个
  **全局审批姿态**（`ApprovalMode = "plan"`，见 §C.2）。
- **非 error 终态的 `detail?`**：让客户端区分"被用户取消" vs "被超时取消"、给 maxBudget 显示花费/上限；
  `runs.cancel` 的 `reason` 经此回流到 outcome。error 的人读说明在 `outcome.error.detail`（§4.6）。

### 4.3 Item

六个变体：`userMessage` / `agentMessage` / `reasoning` / `question` / `toolCall` / `compaction`，判别
字段 `type`，`status ∈ {running, completed, incomplete}`。

- **只有 ToolCall 拥有持久 lifecycle**：它从 `running` 进入 `completed` 或 `incomplete`。user message、Question、
  compaction，以及落入 Transcript 的完整 agent message/reasoning 都以 `completed` 形成。流式 agent message/reasoning
  的 `item.started` 只是客户端渲染锚点，不是可以持久化或恢复的 running Domain Item。
- **ToolCall 的 `startedAt` / `finishedAt` 是可见 Item lifecycle**；终态可选的 `durationMillis` 是 Runtime 在 Tool
  executor 边界测得的精确执行时长，不包含审批或其他执行前等待，因此可以小于 `finishedAt - startedAt`。恢复无法证明
  精确执行区间时字段缺省，客户端不得用 lifecycle 差值伪造。
- ToolCall 可选 `approvalDecision ∈ {approve, deny}` 只表示**这一次调用实际接受过的人类决定**。自动放行的调用缺省；
  客户端不得从当前 ApprovalMode、remembered rule、safetyClass、工具成功或 `denied_by_user` 反推该字段。决定在
  `runs.resume` 的 exact answer-claim 事务内落到原 ToolCall，并随续跑终态、冷启动、Runtime 重启和 Session artifact 保留。
- `question` 是一等 complete Item：它记录“问题已经提出”；是否仍待回答只由同一个 park write-set 中的
  `PendingInterruptSet` 表达，之后由 `runs.resume` 应答。它不是另一套 Form
  子系统：`fields` 是有序、非空、**全部必答**的闭合联合，仅有 `text` 与 `choice`。因此没有与实际行为冲突的
  `required:false`，也没有 number/date/conditional/external 等配置表单词汇。
- Question 不携带可从顺序推导的字段 id，也不重复一份顶层 prompt；每个 field 自带 `prompt`。`choice` 至少有两个
  唯一 label，可选 `multiple` 与 `allowCustom`。普通 `ask_user` 的 choice 允许一个自定义值，让推荐选项不封死用户表达；
  plan/安全决策保持闭合选项。answer 与 field 始终按同一顺序对应。
- `toolCall.error`（+ `status:"incomplete"`）是**工具级失败的统一结构化落点**。工具失败**通常不终止整个 run** ——
  agent 可据此换方案、继续（§8 落点 c）。
- **`compaction`** 标"此处压缩了 N 条更早消息"（`droppedMessages` = 压缩前后净减条数），fold 成时间线分隔条。摘要
  文本已折进重写后的对话历史，故 Item 只表达边界与删减计数。
- **一个 Item 属于一个 Run**（`runId`），不属于 Segment：Item 是历史，段是投递单位。

### 4.4 ToolInvocation（领域中立的通用信封）

**核心只有一个工具形状**（不是联合）。`name` 是身份，`arguments` 是已解析 JSON 对象，`result` 是 best-effort JSON
输出。"某个工具该怎么富渲染"是**领域知识**，不进 wire——由客户端按 `name` 命中的**展示注册表**叠加（§4.4.2）。

**两个 MCP 工具身份（必读，二者不可互换）**：

| 出现处                                            | 值                                                                                          |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `MCPTool.name`（`mcp.tools.list`，§4.10）         | MCP server **原样播报**的远端工具名（可含 `.` / 任意字符）                                  |
| `ToolInvocation.name`（toolCall Item / 审批载荷） | **模型可见名** `sanitize("<server>_<tool>")`：非 `[A-Za-z0-9_-]` 一律换 `_`，超 64 字符截断 |

模型可见名是**有损**的：不同 `(server, tool)` 对可能塌成同一个字符串（`("a_b","c")` 与 `("a","b_c")` 都得
`a_b_c`）。因此：

- 要把一次 toolCall 关联回 `mcp.tools.list` 的条目时**不能**反解 `ToolInvocation.name`；需要精确身份就按
  `(server, tool)` 对匹配。
- `MCPServer.disabledTools` / `autoApproveTools` 用**远端原名**（在其 server 条目内寻址，不受塌名影响）。
- 审批记忆（`remember`，AUX_API §6）的 key 是**模型可见名** —— 塌名的两个 MCP 工具会共享一条规则。这是当前形态的
  已知后果，非疏漏。

设计后果（一次性消除一整串旧形状缺陷）：

- **无 `kind`↔`name` 双重身份**：只有 `name` 一个身份字段。
- **审批 / resume 关联键恒可得**：`payload.tool.name` + `arguments` 永远在 wire 上，server 无需把内部 resume 绑定
  数据塞进 payload —— 该泄漏从根上不存在。
- **新工具零协议成本**：加一个工具 = 在展示注册表登记一行（§4.4.2），**wire 不动、codegen 不动、契约不 bump**。
- **跨工具统一处理可行**：所有 toolCall 都有 `name` → "按 name 分组的调用日志 / 审计视图"天然成立。

#### 4.4.1 入参 / 输出的硬约束

- **`arguments` 永远是 JSON 对象，绝不回传 JSON 字符串。** 流式部分入参走
  `item.delta{ type:"toolArguments", argumentsTextDelta }`（JSON 文本增量，§5.1）累积；server 在 `item.completed`
  （及审批 payload，§4.8）处 unmarshal 成对象再发——消除"双重转义"。
- **`result` 是 best-effort JSON**：首选对象，也允许任意 JSON 值。硬约束：**绝不双重编码**（`{x:1}` 不是
  `"{\"x\":1}"`）。
- **`result` 在 `item.completed` 上权威落定并持久化**（§5.2）：流式期间的 stdout 经
  `item.delta{ type:"toolOutput" }`（ephemeral，可丢）预览；客户端**不可**把流式累积当终态来源。
- **超大工具结果会被 offload**：item 上只留有界预览，完整正文单独持久化（归档里是
  `SessionArtifact.toolResults`，AUX_API §4.3），模型侧经一次显式读取取回。客户端读到的 `result` 就是预览，
  不必也不该拼回全文。
- **工具级失败不进 `result`**：错误一律走 `toolCall.error`（ProblemData）+ `status:"incomplete"`（§4.3 / §8）。

#### 4.4.2 展示契约

Runtime 对少数内建工具的终态结果做稳定投影，供客户端按 `ToolInvocation.name` 富渲染。精确 JSON Schema 由
[`manifest.json`](../contract/manifest.json) 的 `toolResultPresentations` 发布并引用 `schema.json`；它与真正执行投影的
Toolset descriptor 同源生成，不维护手写字段表。当前投影覆盖 `shell`、`glob`、`grep`、`apply_patch` 和
`web_search`。未知工具及未声明展示契约的内建工具一律保留 canonical result，以 JSON 树兜底。

展示契约只约束 Runtime 已投影后的 `result`，不复制模型工具的 input schema；工具入参由 `tools.list` 返回的
`ToolSpec.inputSchema` 拥有。新增或修改投影必须先改变 Toolset descriptor 并重新生成 `contract/`，客户端不得自行猜测
私有执行结果。

### 4.5 可复用结构（Diff / Search / 文件）

供 §7.5 `workspace.*` 直接返回。它们是工作区领域的便利结构，不是 Toolset 展示契约或协议核心原语。

> **两个文件类型共用 `FileStatus` 词汇，但意图不同、故各自独立**：`WorkspaceFileChange`（VCS 工作区扫描态，含
> `untracked`）/ `FileDiff`（逐文件结构化 diff，带 `rows`）。共用过去式 `status`（描述“变更后的态”）。`added`/`removed`（±行）binary 时省略（不伪造 0），
> `previousPath` 仅 `renamed` 给。
> **`Diff` 是 sum-type**（`files` ⊕ `patch`，按 `format`），不是松对象同时带两者。
> `FileLine.text` / `DiffRow.code` / `GrepMatch.text` 是**纯文本**（不含 server 端 HTML）；高亮由客户端做。
> `GrepMatch.path` 与 `FileEntry.path` 一样是 workspace-root-relative、slash-separated 的资源身份；搜索结果按
> path、line、text 稳定排序，底层 grep 的绝对宿主路径不得穿过 Adapter 端口。

### 4.6 Usage / ProblemData

`Usage` 是**非重叠细分**（`inputTokens` / `outputTokens` / `cacheReadTokens` / `cacheWriteTokens` /
`reasoningTokens` + `costUsd` + `byModel`）：把同一批 token 计两次的读数没法做占用条。

`ProblemData` 是错误的**唯一判别联合**，三个落点共用（§8）。`type` 是唯一机器判别键；每个 first-party type
只开放它实际拥有的成员，其他成员必须拒绝，而不是把 `detail?` / `retryAfterSeconds?` / `errors?` /
`activeRun?` / `requiredCapabilities?` 当作任意组合的可选字段袋。完整且可执行的 variant 表由 Contract Registry
生成在 `contract/API_REFERENCE.md`；第三方只经 §2.6 的命名空间分支进入。

- **没有 `retryable`**：那个布尔量除了"这个 type"之外不携带任何信息，且 `omitempty` 让 `false` 与"缺席"不可区分，
  任何 `retryable !== false` 形式的门禁恒为真。客户端的重试门禁一律**按 `type` 判**。
- **没有 `channel`**：一个 problem 属于哪条通道，由**它落在哪儿**决定（RPC error / `outcome.error` /
  `toolCall.error`），字段只是把同一事实抄第二遍。
- **结构化成员由 problem 类型自己带**：`session_has_active_run` 带 `activeRun`、`capability_not_negotiated` 带
  非空且按 `{type,name}` 唯一的 `requiredCapabilities`。客户端因此不必从 `detail` 里 substring-match 出一个
  runId，也不会一次只发现一个能力缺口。

### 4.7 工具规格

`ToolSpec` 是 `tools.list` 返回的**直接诊断**工具目录描述，不是 `runs.start` 的客户端覆盖入口，也不是 Agent 的
完整工具集。一个 Run 的工具集仍由 runtime 按 Session、审批策略、Skills 与 MCP 配置统一装配；直接调用只开放无需
process、session、审批或模型循环的只读诊断能力。

### 4.8 HITL 类型

`Interrupt` 两个一等变体（`approval` / `question`），每个都带 `itemId` + `runId` + 自包含 `payload`。

- **`runId` 是"谁提出的"，不是"谁在等"**：一棵 run 树里，interrupt 集挂在 root 上，但每条 interrupt 记的是提出它的
  那个 Run。
- **payload 自包含**：`OpenInterrupt` 的语义是"什么在等我处理"——它本就该是个自足的待办快照。question 的
  `payload.question` 与 complete Question Item 在同一个 park write-set 内持久化，因此既不需要对 `items.list` 二次 join，
  也不会在重启后丢失渲染事实。两处各存一份不算虚假 DRY：前者是尚待回答的待办快照，后者是已经提出问题的历史事实；
  是否仍在等待只属于 Pending interrupt，不属于 Transcript Item 的第二套 lifecycle。
- **分页单位是"一个 waiting Run 的完整 interrupt 集"**（`PendingInterruptSet`）：半个集合无法应答，按条分页会
  发明出不可用的页（§7.3）；`interrupts` 必填且非空，空集合不是一个可恢复的 Waiting 状态。

### 4.9 Provider / Model

provider 凭证只回 `apiKeyMasked`，永不可逆推。per-run 的 provider + model **显式配对**（缺一即错；都缺时读取
既有 Session 的 durable pair，只有 fresh Session admission 才安装 Runtime 默认），provider **不从 model 名推断**。
`models.list` 的 `contextWindow` 配 live `RunProgress.contextTokens` 或 durable
`RunRef.contextTokens` 做占用条。

Session 同样持久化完整 provider + model pair，并作为省略 per-run 选择时的唯一默认 owner。显式 Run 选择成功进入
opening write-set 后原子替换该 pair；创建、schedule、fork 与 artifact round-trip 都保留完整身份。两个 provider
发布同名 model 时不得按 model id 取首项或回落全局默认。

### 4.10 Workspace 周边 / 可选域类型

MCP server / skills / recipes / hooks / schedules / knowledge / goals / agentMemory 等可选域的类型都在
`schema.json` 里，语义见 [`AUX_API.md`](./AUX_API.md)。每个域由 §9 的一个 feature 门控。

MCP 只发布一个 `MCPServer` 资源，不再把可编辑配置与连接状态拆成两个集合让客户端 join：

- `connection` 是闭合的**安全读联合**：`stdio{command,args?,envMasked?,dir?}` 或
  `streamableHttp{url,authorizationMasked?,headersMasked?}`；另一种 transport 的字段不可出现，secret 原文永不通过读
  API 返回。
- `status` 是闭合联合：`disabled` / `disconnected` / `connecting` /
  `connected{toolCount}` / `failed{error}` / `needsAuth{error}`。`disabled` 是持久化开关关闭，`disconnected` 是已启用但
  当前 live projection 尚无连接；二者不再靠字段缺席或客户端猜测区分。
- authorization / headers / environment 都是 write-only，分别使用 `MCPAuthorizationChange` / `MCPHeadersChange` /
  `MCPEnvironmentChange`；三者共享精确的 `set` / `clear` 词汇，但保留独立领域类型，避免把一种 secret 误传到另一种
  位置。省略仅在相同 secret scope 内表示保留：HTTP 以 URL origin 为 scope；stdio 以完整进程目标
  `(command,args,dir)` 为 scope。scope 变化且已有 secret 时必须显式 `set` 或 `clear`，runtime 绝不把凭证静默带到新
  origin / 进程，也不替调用方猜测是否删除。显式切换 transport 会原子丢弃旧 transport 专属 secret；它们不可能进入新
  transport 的联合。`set.value` 必须非空；删除只用 `clear`，不让空 map 成为第二种删除写法。
- 交互式 OAuth 不是一个“已开始”的 ACK，而是一等 `MCPAuthorizationAttempt` 资源：
  `mcp.authorizationAttempts.create{server}` 返回 `pending`，客户端用
  `mcp.authorizationAttempts.get{attemptId}` 观察 `succeeded` / `failed{error}` / `canceled`。只有 live server 已实际
  connected 才是 `succeeded`；该操作只适用于 `streamableHttp`，其他 transport 在创建前拒绝；新的
  configure/delete/reconnect/authorization 会取消同一 server 的旧 attempt。
  `failed.error.type=mcp_authorization_failed`，底层 OAuth/provider 文本只进 telemetry。终态保留窗口由
  `capabilities.limits.mcpAuthorizationAttempts.retentionSeconds` 公布；pending 不按该窗口清理。

### 4.11 list 信封与 cursor 分页

- **所有 list 方法一律返回 `Page<T>`**（`data` + 可选 `nextCursor`），客户端一个读法。
- list 分为两种诚实能力：**有界集合**不接受 `cursor/limit`，一次返回完整 `Page<T>` 且不产生 `nextCursor`；
  **cursor 集合**同时接受 `cursor/limit`，`nextCursor` 的存在性就是“还有更多”。不得让一个已经全量物化、也不会续页的
  有界集合暴露空转的 cursor 参数。
- **server 不得静默截断**：有界集合超过自身明确的安全边界必须失败并要求调用方缩小查询；cursor 集合只要还有数据就
  必须返回 `nextCursor`（"no silent caps"）。统一信封不等于虚构统一的分页能力。
- **cursor 不透明**：client 不解析、不据其推断顺序，只回传上次拿到的 `nextCursor`。
- **cursor 绑定完整查询**：method、scope、filter、排序方向等所有影响成员与顺序的规范化输入都属于 cursor 身份；跨读或
  跨查询复用会被拒绝。cursor 锚定上一页末尾的**排序键值**而非行本身，所以锚点行随后被删除也能继续。
- **分页类别不是方法名约定**：Registry 从 result 的完整 `data/nextCursor` shape 与 params 是否同时含有
  `cursor/limit` 推导能力。`Page<T>` 不带这对参数就是 `pagination:"none"`；同时具备才是
  `pagination:"cursor"`；只出现半套字段时启动即失败。manifest、OpenRPC 的 `x-lyra-pagination` 与 SDK 都消费这一个
  事实，不维护第二张方法表。
- **SDK 只为真实 cursor 调用提供自动续页**：cursor list 的调用可用 `for await` / `.autoPagingEach()` /
  `.autoPagingToArray()`，`.pages()` 在需要 `items.list.runs` 等 page 级附加数据时逐页交付；有界 list 直接
  `await` 一次取得 `Page<T>`。server 重复 cursor 是协议错误，SDK 抛 `PaginationError`，不得把残缺集合静默当成完整
  集合。

---

## 5. 流式

所有 run 事件走**一个**通知方法 `notifications.run.event`，params 为 `RunEvent`
（`runId` / `segmentId` / `eventId` / `timestamp` / `event: StreamEvent`）。七个 `StreamEvent` 变体与各自必填字段见
`manifest.json` 的 `unions`；下面是它们的语义。

| event.type         | 语义                                                             |
| ------------------ | ---------------------------------------------------------------- |
| `segment.started`  | 这一段开始，带 `run: RunRef`（含 `activeSegmentId` 与 profile）  |
| `segment.progress` | 瞬时读数（step / activity / usage / contextTokens）              |
| `item.started`     | 一个 Item 的壳落地                                               |
| `item.delta`       | 该 Item 的增量预览（五种，§5.1）                                 |
| `item.completed`   | 该 Item 的**权威终态**                                           |
| `plan.updated`     | Session Plan 的**整份**当前值（§5.3）                            |
| `segment.finished` | 这一段结束，带 `outcome: SegmentOutcome` + `metrics: RunMetrics` |

> **`contextTokens` 不是 `usage.inputTokens`**：前者是**此刻**窗口占了多少（压缩后会回落），后者是跨轮**累计**只增。
> 正值在每次 progress commit 时同时推进到 root Run 的 `RunRef.contextTokens`，因此终态、重连、Runtime restart 与
> Artifact round-trip 都能恢复最近一次权威 prompt footprint；压缩后的较小正值允许覆盖旧值。`0` 表示本次没有新读数，
> 不得擦除已有 footprint。客户端按 Session 中最新的正值 root Run 读取，不能把累计 usage 当作窗口占用。
>
> **`segment.finished` 不带 `run`**：一段结束时 Run 的完整状态该从 `runs.get` 读，把 RunRef 再抄一份到终态帧上，
> 就等于让终态成为 Run 状态的第二个作者。

### 5.1 ItemDelta

五种：`content`（agentMessage 文本）/ `reasoning` / `toolArguments` / `toolOutput` / `plan`。

> **`toolArguments` 是文本增量**：流式工具入参逐片到达、中途不构成合法 JSON，客户端累积 `argumentsTextDelta` 做
> 预览；**已解析结构化字段只在 completed `toolCall` 的 `tool.arguments` 落定**（§4.4）。
>
> **`toolArguments` / `toolOutput` 都是预览通道**：二者皆 ephemeral，权威终值在 completed item 上
> （`toolArguments`→`tool.arguments`，`toolOutput`→`tool.result.output`）。见 §5.2。

### 5.2 Authoritative / Replayable 不变量（协议级保证）

> **丢弃每一个 non-authoritative 事件，客户端仍必然得到正确终态。**

Authoritative、replayable 与 persisted 是三个不同概念：

- authoritative：客户端能否把事件当作可折叠事实；
- replayable：当前 process / root Segment 的有界窗口是否保留它；
- persisted：事实是否已经进入可跨进程恢复的 Run / Item / Plan 持久化投影。

前两项由 `event.type` 决定，wire 上没有 sender-controlled reliability flag。这张表由 Registry 生成到
`manifest.json.runEventPolicy`，SSE id 与 replay journal 只读取 replayable：

| event.type                  | authoritative | replayable | 冷恢复 / 权威落点                           |
| --------------------------- | ------------- | ---------- | ------------------------------------------- |
| `segment.started`           | ✅            | ✅         | RunRef                                      |
| `segment.finished`          | ✅            | ✅         | RunRef / Interrupt query                    |
| `item.started`              | ✅            | ✅         | `items.list`                                |
| `item.completed`            | ✅            | ✅         | `items.list`                                |
| `plan.updated`              | ✅            | ✅         | `plan.get`                                  |
| `segment.progress`          | ⬜            | ⬜         | `segment.finished.metrics` / RunRef.metrics |
| `item.delta{content}`       | ⬜            | ⬜         | `agentMessage.content`（completed）         |
| `item.delta{reasoning}`     | ⬜            | ⬜         | `reasoning.text`（completed）               |
| `item.delta{toolArguments}` | ⬜            | ⬜         | `tool.arguments`（completed，§4.4）         |
| `item.delta{toolOutput}`    | ⬜            | ⬜         | `tool.result.output`（completed，§4.4.2）   |
| `item.delta{plan}`          | ⬜            | ⬜         | `plan.steps`（completed）                   |

**硬规则**：每个 preview **必须**在 authoritative projection 上有命名终值。第三方扩展若需要新事实，
使用已有的领域中立 Item，或定义带 read model 的 capability-gated 领域资源。

推论：客户端可排除高频 delta（§9 `excludedEphemeralEvents`）而仍保持正确；不流式的 runtime
可不发任何 delta，completed item 一样必发权威终值。

### 5.3 Plan 必发边界

Plan 只以整份 `plan.updated` 传播（**无增量事件**），由 root Run 写入，作用域固定为 Session；它不是通用 state
registry 的一个变体。`plan.updated` 与 `plan.get` 使用同一个 `Plan` shape，并遵守：

- **每次改变时发**（快照即当前完整视图，带单调 `revision`）；
- **`segment.finished` 之前必发**该段改过的 Plan —— 这条是**位置保证**：收到终态的人就已经收到了终值。晚接入或
  从更后的 cursor 重放的订阅者，否则会走到终态却从没见过 Plan。**这一段没改过 Plan 就不发**：一份 revision 0 的
  空 Plan 在按 revision 折叠的客户端那里读作"清单被清空了"，不是"没变"。
- **revision 只增不减**：把一个更早的值重新发布（回退、导入归档）也是**一次新的写入**，拿到更大的 revision。
  否则客户端会把回退后的清单当旧值丢掉。
- **Plan 不带 runId**：它一个 Session 一份值，用 Run 去窄化 refetch 会按一个它并不索引的键去查。

**cold read 是一等公民**：`plan.get` 返回与事件**同形同 revision**的 Plan。重载、回退、replay 窗口过期之后，
客户端靠它把面板接回来；Plan 不是第三方扩展缝，也没有动态 key、scope、writer 或 recovery-method metadata。

### 5.4 Run 树

一条 root run 的**一段流**包含**整棵 run 树**的事件（根 + 所有子孙 subagent run）：

- 每条事件带它所属的 `runId`（稳定）+ `segmentId`（所属段）；
- 子 run 以 `segment.started` 携带 `spawnedByItemId` 开始，有**自己的** `runId` 与 `segmentId`；
- 客户端用 `runId` + `spawnedByItemId` join 还原树，用 `segmentId` key 流；
- 对 root run `runs.subscribe` = 订阅其当前活跃段的整棵树；**只有 root 段的 `segment.finished` 结束这条流**；
- 当前 build 广告 `features.subagents=true`，但它是 request-level opt-in：只有创建
  root Run 的请求显式声明该能力，冻结 profile 才允许产出 child Run；Minimal Profile
  仍保持 root-only。

---

## 6. Human-in-the-Loop（R 模型）

流程：

1. agent 需要人介入 → 产出对应 Item（`toolCall` 待批 / `question` 待答 / client 工具调用）。
2. 当前 **Segment 结束**：`segment.finished{ outcome:{ type:"interrupt", interrupts:[…] } }`，资源全释放。
   **Run 不结束**——它转入 `waiting`。每个 `Interrupt.itemId` 指向那个待解 Item。
3. 待解项**持久化**（跨重启可经 `interrupts.list` 发现），一个 waiting Run 恰好持有一个 open interrupt 集。
4. 客户端调 `runs.resume{ runId, responses, input? }` 在**同一 Run 上开新的一段**应答（返回同 `runId` + 新
   `segmentId`）。
5. 续段像普通流一样，直到下一个 outcome。

**关联键 = `itemId`**（无单独 requestId）。**拒绝 ≠ 取消**：拒绝是 `runs.resume` 带 `decision:"deny"`，run 继续
（agent 据理由换方案）；取消是 `runs.cancel`，硬终止整个 run。成功结果是闭合联合：
root 返回 `{type:"root", run}`；child 返回 `{type:"child", run, rootRun}`。其中被寻址的
`run` 已是 `finished/canceled`，child 分支的 `rootRun` 与它来自同一 command boundary，
客户端不需要再猜 root 是 Running、Waiting 还是 Finished。显式寻址 child 需要本次
请求协商 `features.subagents`；缺失时返回 `capability_not_negotiated`，不能静默退化成
root cancel。root cancel 始终允许，作为所有客户端都能使用的 emergency stop。

**resume 从"那个待答的调用"处续跑**，不重跑整段：runtime 记住 pending 调用的位置，把答复接上去，不会为了续跑再问一次
模型。

### 6.1 InterruptResponse

`{ itemId, response: InterruptResponseValue }`，两种 response 与 interrupt 两型对应。

> question response 的 `answers` 是 `string[][]`，与 `Question.fields` **按下标一一对应**；每个 field 的值始终
> 是 `string[]`（text/单选是一元素，多选可多元素）。它删除了 `q0` 之类可推导 id 和动态 map key，同时避免
> `string | string[]` 双形状。数量必须精确覆盖全部 fields；choice 去重并验证闭合选项，`allowCustom:true` 时至多
> 接受一个不在 options 中的自定义值。失败返回 `invalid_params.errors[]`，field 精确指向
> `responses[i].response.answers[j]`，客户端无需解析 detail 文案。
>
> **`remember`（审批 scope，AUX_API §6）**：持久化成一条**细粒度规则**（`ApprovalRule`，§C.2）。规则按
> `(scope, tool, subject)` 命中：`subject` 是后端按工具从被批准调用里提取的子主题（`shell.command` 或
> `read.path`；多路径的 `apply_patch` 使用空 subject），所以记的是「`npm run *` 在本 project」而非笼统「整个 shell」。`decision:"deny" + remember` 合法 =
> 记住拒绝。`editedArgs` 仍是**一次性**的（不折进规则）。三个 `scope` 全部持久：`session` / `project` / `global`；
> 最具体的命中胜出（session > project > global，再 exact > glob > 任意），同特异度冲突取 deny。

### 6.2 防挂死（协议级硬约束）

client 在 `ClientCapabilities.interruptTypes` 声明能处理的 `Interrupt.type`。server **必须不产出 client 未声明 type
的 open interrupt** —— 否则会留下一个**永远 `runs.resume` 不了的持久 open interrupt**（比挂起 run 更糟）。不支持时
server 走非阻塞默认策略（auto-deny / 不进该模式）。

Run 创建时把这份声明冻进 `RunProtocolProfile.interruptTypes`（§3.2），**Run 整个生命周期不变**：应答一个 interrupt
不该悄悄改变下一段可以停在什么上面。

---

## 7. 方法语义

> **方法表在 [`API_REFERENCE.md`](../contract/API_REFERENCE.md)**（每个方法的 kind、幂等类别、
> 分页类别、所需 feature、可能返回的 problem type），入/出 schema 在
> [`openrpc.json`](../contract/openrpc.json)。本节只写各域的语义、约束与“为什么这样切”。
>
> **Stream** = 返回一个 ack（含 `runId` / `segmentId`）+ 随后流式 `notifications.run.event`。这是传输无关语义；
> HTTP 上 ack 作首帧、与后续事件同走该 POST 的响应流（streamable HTTP，TRANSPORT §6.4）。**流式方法集机器可读于
> `ServerCapabilities.streamingMethods`** —— 客户端据此预知响应分支，不硬编码方法名。
>
> **幂等**：写方法登记幂等类别（`replayResponse` / `replayRunStream`），客户端带 `Idempotency-Key`
> （TRANSPORT §2）重试即安全；同 key 不同 params 返回 `idempotency_conflict`，首个执行未完成返回
> `idempotency_in_progress`。discovery 的 `limits.idempotency` 同时发布 retention 与 durable store 的 opaque `namespace`；跨
> client process 恢复 key 必须以 `Idempotency-Namespace` 绑定该 namespace，不能只凭 endpoint 推断 store 身份；服务端在业务
> admission 前拒绝不匹配的 store identity 并返回 `idempotency_store_mismatch`。**幂等重放不返回一个"新的" ack**：重试
> `runs.start` 拿回的是同一个 run 的 ack。

### 7.1 runtime.\*

- `runtime.discover` —— 读 `{protocol, serverInfo, capabilities}`。它是无状态查询，不是生命周期切换。
  **discovery 只说 runtime 真做得到的事**：每个 topic 都有生产者，每个 feature 都有真实门控，每个
  数字（replay 窗口 / 订阅上限）都被强制执行。发布一个没人实现的能力比不发布更糟。
- `runtime.subscribe` —— 工作区/会话/run 的**失效流**（§7.8、AUX_API §3）。

进程 shutdown 属于宿主生命周期；请求取消由 transport context 传播；HTTP 存活/就绪检查用 sidecar。协议不暴露
无效果的镜像方法。

### 7.2 sessions.\*

`list` / `get` / `snapshot` / `create` / `update` / `delete` / `fork` / `rollback` / `export` / `import`。

- **`create` 的 `workspace` 缺省 = `ServerInfo.defaultWorkspace`**（冷启动零摩擦），返回完整 `WorkspaceInfo`。
- **`snapshot` 是挂载恢复的原子 material read**：Runtime 在一个存储事务内读取完整 Items、Runs、open
  Interrupt sets 与当前 Plan，客户端不能再把四次独立查询的不同提交点拼成一个从未存在过的视图。
  `includeDescendants:true` 与 `runs.list` 一样要求 `features.subagents`；未提供 Plan 能力的 composition 省略
  `plan`。它不是通用 `expand[]`，也不替代下面各资源的分页/筛选接口。
- **`update` 是条件写**：必带 `expectedRevision`（§4.1）。改 `workspace` 需 `features.relocate`。
- **`fork` 在一个 run 边界切开**：`fromRunId` 之前（含）的历史进新会话，之后的不进。Plan 也按同一个边界
  走 —— fork 出来的会话拿到的是**那一刻**的任务清单，不是现在的。
- **`rollback` 是回退到某个 run 之前**（AUX_API §4.1）：删掉之后的 run、把消息日志截回该 run 的水位、按
  `restoreType` 可选还原文件（`features.checkpoints`），并把**边界那一刻的 Plan 作为一次新写入重新发布**
  （§5.3）。返回 `droppedRuns: DroppedRun[]`（每条带 `run: RunSummary` + 触发它的 `userInput`），所以客户端能
  告诉人"回退丢了哪些回合"。session 有 run 在飞时拒绝（`session_busy`），不去和正在 append 的历史赛跑。
- **`export` / `import` 是同一份 `SessionArtifact`（v23）的两端**（AUX_API §4.3）：终态 run + 完整 Item 历史 +
  chat 消息 + offload 的工具正文 + 显式 `plan` 语义值（不带 revision / updatedAt —— 那是源 runtime 的排序
  凭证，带过去会让导入值声称一个目标 runtime 从未发出的位置）。import 是**替换语义**（同 id 覆盖），版本不认识就
  确定性拒绝、**不迁移**，只接受当前 v23 shape。

### 7.3 runs.\*

`start` / `resume` / `subscribe` / `cancel` / `steer` / `get` / `list`，外加 `interrupts.list`。

- **`start` 不隐式取消**：会话已有非终态 root run 时返回 `session_has_active_run`，problem 里带
  `activeRun: {runId, status}`。哪个 run 该继续只有人能决定，隐式取消会为了服务一个"本可以是 steer"的请求而丢掉
  工作。
- **`start` 的 ack 必带 `userItemId`**：成功 start 已经原子创建持久化 user Item，其 id 是 ack 的必备事实；客户端
  以 exact ItemID 对账，绝不按消息内容猜测。`resume` 使用独立的 `ResumeRunResponse`，仅当 request 带 `input` 时
  返回 `userItemId`，不带 input 时禁止出现。
- **`resume` 可带 `input`**：应答 interrupt 的同时追加一句话。**这是一次原子写**：答复与那条 user Item 一起提交，
  不存在"答复进去了、消息丢了"的中间态。
- **`subscribe` 必须认领一个 segment**：`{runId, segmentId}`。段号对不上返回 `stale_segment`（客户端重读 run 再从
  新的 `activeSegmentId` 决定，runtime 不替它改目标）；run 不在执行返回 `run_waiting` / `run_finished`（各自告诉
  客户端答案在别处：等待集 / 历史）；child run 返回 `run_not_root`（跟着 `rootRunId` 走，而不是去找另一个 id）。
- **续流用 cursor**：带 `Last-Event-Id` 表示"从这之后重放"。cursor 编码了流的进程 epoch 与 scope，所以别的进程或
  别的段的 cursor 会被**拒绝**（`replay_cursor_invalid`）而不是去解释；曾经合法但已被窗口淘汰的位置返回
  `replay_unavailable`（事件没了、但它们产出的 Item 已持久化：客户端冷读历史再 tail 重接，§10.1）。
  ack 的 `headEventId` **只许原样保存并作为后续 cursor**，不许比较或解释 —— 重接 ack 的 head 在你请求的位置**之前面**，
  采用它会静默跳过刚请求的重放。replay 窗口的 scope 与容量在 `capabilities.limits.runReplay` 里公布并被强制执行。
- **`steer` 是真正的中途插话**：
  `{runId, expectedSegmentId, input: ContentBlock[]}`，在下一轮开始前把结构化输入交给模型；段号对不上同样
  `stale_segment`。它不是"取消再重发"。
- **`cancel` 的 `reason` 回流进 outcome 的 `detail`**（§4.2）。取消一棵树 = 取消 root；取消一个 child = 取消它的
  子树，父调用收到一条注入的结果说明这次委派被取消了。能力检查在解析 durable run identity **之后**发生：
  root cancel 永远是可用的 emergency stop，不会因为请求声明了本 build 不支持的可选能力而被提前挡住；只有实际
  寻址 child 时才要求已协商 `features.subagents`。
- **`get` / `list` 一律读持久化 projection**（不是内存里的活跃表）：`list` 是**全历史**，可按 `statuses` 过滤、
  按 `sessionId` 收窄、`includeDescendants` 展开子树（需 `features.subagents`）。`get` 寻址 root 不需要该能力，
  寻址 child 才需要；不存在的 id 仍先返回 `run_not_found`，不把资源是否存在泄漏成一条静态能力判断。
  **没有"当前在跑"这个专用读**——它就是 `statuses:["running"]`。
- **`interrupts.list` 的一行是"一个 waiting Run 的完整 interrupt 集"**（`PendingInterruptSet`，§4.8），可按
  `sessionId` 或 `rootRunId` 收窄。它取代了旧的 `runs.listOpenInterrupts`：那个名字把"待解集合"说成了 run 的一个
  子列表。

### 7.4 items.\*

`items.list` 是持久化历史的独立、可分页资源读；`sessions.snapshot` 只在挂载恢复用例中内嵌同一完整历史。

- **`scope` 是 typed union**：`{type:"session", sessionId}` 或 `{type:"run", runId, includeDescendants?}`。一个
  松对象同时带两个 id，就得由服务端猜客户端想要哪个 —— 那是服务端在替客户端做决定。session scope 永远返回
  完整历史（包括 child item）；run scope 若实际寻址 child，或显式要求 `includeDescendants:true`，才要求已协商
  `features.subagents`。
- **`order`** 显式（默认按时间正序）；**页级 `runs: RunSummary[]`** 随每页返回该页 item 所属的 run，客户端因此
  能在拿到第一页时就把树连起来，而不必先把所有页拉完。

### 7.5 workspaces.\* / workspace.\*

集合根提供 `workspaces.resolve` / `workspaces.list`；单资源根按层级提供
`workspace.changes.list` / `workspace.diff.get` / `workspace.files.head|search|list|read`。单资源操作全部显式带
`workspace: WorkspaceRef`（§0.2），路径 jail 到该根、越界 `path_outside_root`（§15）。git 相关读需
`features.git`；有 git 二进制但 workspace 非仓库是 `vcs_unavailable`（与"干净仓 = 空结果"区分）。

文件事件与工作区失效走 `runtime.subscribe`（§7.8 / AUX_API §3）。

### 7.6 providers.\* / models.\* / tools.\*

`providers.list` 返回固定受支持集合与当前有效配置；`providers.update` 对已持久化配置做**原子字段变更**：

- 省略 `apiKey` / `baseUrl` 表示保持；
- `{ type:"set", value }` 表示替换，`value` 必须非空；
- `{ type:"clear" }` 表示清空，不允许同时携带 `value`。

因此客户端无需发送不可回读的旧密钥，也不会把“空字符串”猜成保持或清空。清空 stored key 后若进程环境提供该
provider 的 key，读取面自然回落到 `keySource:"env"`；环境值只参与读取投影，绝不由更新路径写入持久层。声明
`requiresBaseUrl:true` 的 provider 在最终状态中必须保有 endpoint，故首次设置 key 时需同时设置 URL，且不能清空
已有 URL。`providers.test` 是只读探测，失败 verdict 走 `ProviderTestResult.error`，不改变配置。

`models.*` 提供模型目录与 utility / embedding 角色；`tools.*` 提供直接诊断工具的 `list` / `invoke`（§4.7）。

### 7.7 可选域（capability-gated）

`skills.*` / `recipes.*` / `agentDocs.*` / `hooks.*` / `mcp.*` / `knowledge.*` / `agentMemory.*` / `approval.*` /
`checkpoints` / `usage.*` / `feedback.*` 等，每个由 §9 的一个 feature 门控，关掉即返回
`capability_not_negotiated`（problem 里带 `requiredCapabilities`，客户端因此知道缺的是哪一个）。语义见
[`AUX_API.md`](./AUX_API.md)。

`knowledge.*` 专指人工维护的 `LYRA.md` 级联：`knowledge.list` 列出 workspace 可见且可寻址的条目，包括尚未创建的空文档；
`knowledge.get` / `knowledge.update` 以闭合 `scope`（`cwd` / `projectRoot` / `home`）读取或条件覆盖一个条目。
级联按 `home → projectRoot → cwd` 从宽到窄排列；当 workspace 本身就是 project root 时，同一物理文件只列一次并归为
`cwd`，但 `knowledge.get/update` 仍可用任一显式 scope 寻址。
每个 `KnowledgeEntry` 都带非空 opaque `revision`；调用方不得解析它。`knowledge.update` 必须回传最近读取的
`expectedRevision`，只在它仍匹配时原子替换，并返回包含新 revision 的权威 `KnowledgeEntry`。不匹配返回
`revision_conflict`，绝不静默覆盖并发编辑；首次创建也使用空文档读取所得 revision。
文档的 filesystem identity 必须解析后仍位于所选 scope 根内；越界 symlink 对 list/get/update 都返回
`path_outside_root`。域内 symlink 保持为 alias，读、revision 比较和原子替换共同作用于同一 physical target；
替换保留目标文件权限。多个 Runtime 进程对同一 physical document 的 CAS 也只有一个 winner，进程在 publish 前退出只会留下
可安全回收的 staging 文件，不能产生 torn content。
它不承载模型提炼事实，也不与 `agentMemory.*` 共享生命周期；后者才是 Agent 自维护、可检索和人工复核的记忆账本。
因此协议不再提供含混的 `memory.*` 别名或 `features.memory`。

`agentMemory.list` / `agentMemory.add` 的 target 是闭合二选一：`scope:"project"` 必带 `workspace: WorkspaceRef`；
`scope:"user"` 禁止 `workspace`。不再有“省略 scope 默认 project”或“user scope 带一个会被忽略的 workspace”这两种
半有效请求。SDK 的 `workspace(ref).agentMemory` 固定绑定 project target；用户级 agent memory 走顶层 `agentMemory`。

`agentDocs.list` 按 `home → projectRoot → cwd` 返回 AGENTS.md 级联。`scope` 是发现时保留的来源层，不从最终路径反推；
`path` 是去除 symlink/平台别名后的 canonical source identity，因此同一物理文件只出现一次。

### 7.8 服务端发出的 Notification 汇总

**只有两个**：

| method                        | params         | 载什么                                                                                                                                                                                         |
| ----------------------------- | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `notifications.run.event`     | `RunEvent`     | 一个 run 段的 segment/item/Plan 事件（§5）                                                                                                                                                     |
| `notifications.runtime.event` | `RuntimeEvent` | 失效信号：`files.changed` / `skills.changed` / `mcp.changed` / `schedules.changed` / `sessions.changed` / `runs.changed` / `plan.changed` / `goals.changed` / `interrupts.changed` / `knowledge.changed` / `hooks.changed` / `models.changed` / `approvals.changed` / `agentMemory.changed` / `resync` |

失效信号的契约（§9 / AUX_API §3）：

- **每条带从 1 开始的正整数 `sequence`**，且 **`sequence` 只发给真正进入队列的帧** —— 号不会为了一个被合并掉的
  信号而跳。
- **不丢帧**：来不及投递的失效**合并**成一条点名 topic 的 `resync`（"这些 topic 你重读一遍"），而不是丢掉后让
  客户端靠"看见空号"去发现 —— 安静的流上永远看不见。`resync.topics` 必填且非空。
- **一个 topic 一个资源**：客户端收到信号后调该资源的读方法重取（`plan.changed` → `plan.get`）。信号本身不带
  业务数据，它只说"再读一次"。
- **narrowing array 出现即有意义**：`files.changed.paths` 必填且非空；其余
  `names/serverIds/scheduleIds/sessionIds/runIds/watchIds` 出现时均非空，所有集合均无重复。空数组不用来表达
  “全部”，省略才表示该 variant 没有进一步收窄。

### 7.9 schedules.\*

cron 形态的 headless run 管理（`features.schedules`）：一条 schedule 存的是完整、自包含的执行 instructions，不是一个 recipe
引用 —— 引用会让"这条定时任务当初要做什么"随 recipe 被改写而改变。触发一次 = 起一个普通 run。

`schedules.update` 是带 `expectedRevision` 的部分更新。工作区有三个无歧义分支：省略 `workspace` 与
`workspaceMode` 保持现有绑定；发送合法 `workspace: WorkspaceRef` 设置显式绑定；发送
`workspaceMode: "default"` 删除显式绑定并回到 `ServerInfo.defaultWorkspace`。后两者互斥，空路径不是清空语义。

### 7.14 goals.\*

自主执行环的目标与预算（`features.goals`）。**goal 是会话拥有的聚合**：run 的终态在**同一个事务**里给它记账
（所以没有"goal 的账和 run 的账不一致"这种中间态），会话被删 / 回退 / 导入时 goal 一并清理，而不是变成一条指向
不存在会话的孤儿记录。

暂停或阻塞的 goal 通过结构化 `reason { code, detail? }` 暴露停止上下文。`code` 是闭合、稳定的机器语义，客户端据此
决定行为并本地化文案；`detail` 只携带安全的领域/模型上下文。runtime 不生成面向用户的英文句子，也不把基础设施错误
写入 goal。active goal 省略 `reason`。

`status:"completing"` 是可观察的收尾窗口：模型已声明目标成功，但 owning drive 仍在提交最终 Run 记账并清除 goal。
它不是可恢复的暂停态；客户端必须保留 goal 占位、禁止 stop/resume/start，等待下一次 `goals.changed` 后回读到 `null`。
这避免把完成声明与最终记账之间的合法状态伪装成读取失败或“没有 goal”。

`goals.update{sessionId,objective}` 只编辑用户写下的 objective：Application 先收束本进程拥有的 active drive，再以 CAS
保存新 objective incarnation；既有 status/reason、model selection、冻结 capabilities、budget、usage 与 createdAt 保持不变。
编辑前处于 active 的 goal 在取消中的 Run 没有独立 block/complete 时继续 active；旧 incarnation 的迟到 Run 不能给新文本记账
或改 lifecycle。`completing` 拒绝编辑，不能越过最终结算 owner。

`goals.clear{sessionId}` 收束 owned drive 后按最新 version 清除 aggregate；目标已由完成结算先行清除时同样成功，因此陈旧 UI
动作与自动完成可以幂等收敛。它不删除会话或对话历史。update 返回的 Goal 只是 mutation receipt，clear 返回 ack；挂载 Session
的 standing Goal 仍只由 `sessions.snapshot` 整体回读，客户端不得把 mutation response 写成第二 projection owner。

---

## 8. 错误

### 8.1 三个落点 + 决策表

错误可能出现在**三个落点**。新对接者常只预期"错误在响应里"，故先给决策表：

| #   | 落点      | 何时                                                                                                         | 怎么投递                                                        | 终止 run？                     |
| --- | --------- | ------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------- | ------------------------------ |
| a   | RPC error | 调用本身就错：`session_not_found` / `invalid_params` / `workspace_unavailable` / `capability_not_negotiated` | 该 method 的**同步 JSON-RPC error response**（带 `error.code`） | 调用未起 run                   |
| b   | run 终态  | run 已起、执行中整体失败                                                                                     | 流内 `segment.finished{ outcome:{ type:"error", error } }`      | 是                             |
| c   | item      | 单个工具失败                                                                                                 | 对应 `toolCall` item 的 `error` + `status:"incomplete"`         | **否**（agent 多半换方案继续） |

三处都用**同一个 `ProblemData` 形状**。**落点本身就是判别**（§4.6：没有 `channel` 字段）。**反直觉但关键**：工具
失败（c）通常不终止 run；实现方**不要**期望 run 执行期错误（b/c）能在 `runs.start` 的同步 response（a）里拿到。

第四类是**内联状态**：它骑在**某个查询自己的结果**里（`MCPServer.status.error` / `ProviderTestResult.error`），表达
"这个东西当前坏在哪"，而不是"你这次调用失败了"，所以调用**成功**返回。见 §8.4。

### 8.2 错误码与符号名

业务错误用 JSON-RPC `error.code`，**不映射 HTTP status**（HTTP status 仅反映传输层，见 TRANSPORT §6.3）。
`error.data` 是 `ProblemData`，**必须含 `type`**。

**判别一律走 `type`（符号名），不走数字码。** 数字码只是粗分类，且：

- 完整的 `type ↔ code ↔ recoveryAction` 表在 `manifest.json` 的 `errors` 节（由 Go sentinel 与码常量**生成**，
  §14）；本文不重述，重述就是第二份会腐烂的表。
- 业务码落在 JSON-RPC 2.0 的 `-32000..-32099`「implementation-defined server-error」保留段，`-326xx` / `-32700`
  为 spec 预定义码。
- **退役的码不复用**：一个曾经存在的码号不会被赋予新含义，新码只往后接。老客户端因此不会把新错误认成旧错误。
- 三类符号名并存，各有各的落点：**RPC 级**（带数字码，落点 a）、**run/item 级**（无数字码，落点 b/c）、
  **内联状态级**（无数字码、不属三条通道，§8.4）。

### 8.3 错误细节（ProblemData，对标 RFC 9457）

`ProblemData`（§4.6）是 **RFC 9457 _Problem Details_ 数据模型的传输无关裁剪** —— 去掉 HTTP 专属的 `status`
（与 JSON-RPC `code` 冗余）和 `instance`；`type` 是稳定符号名、**不要求是可解析 URI**（命名空间见 §8.4）。
约定好的扩展成员：

- **`docUrl`** —— 可选，指向该 `type` 的文档页；缺省时客户端按符号名查本地文案表。
- **`retryAfterSeconds`** —— 值得等的 type（`rate_limited` / `timeout` / `provider_unavailable`，以及必带该值的
  `idempotency_in_progress`）回传的正整数最早重试时机；**是否允许/必须附带由 `type` 直接决定**，不经任何中间布尔量。
- **`errors: FieldError[]`** —— 字段级校验错误（典型 `invalid_params`、provider 配置 / `question` 答案表单）；
  `field` = 出错 params key，UI 可逐字段标红。
- **按 type 附带的结构化成员** —— `activeRun`（`session_has_active_run`）、`requiredCapabilities`
  （`capability_not_negotiated`）。它们由 problem 自己填，不由投递层从 `detail` 反推。

这些成员不是全局可选字段：某个 variant 没有列出的字段在该 variant 上必须不存在。first-party type 是生成的精确联合；
扩展 type 只接受 `plugin:<pluginName>/<symbol>`，并只开放 `detail` / `docUrl` / `retryAfterSeconds` 三个通用成员。

### 8.4 `type` 命名空间（防撞名）

error `type` 是 §2.6 命名空间的一个实例：first-party 用裸 `snake_case`；**第三方插件**产出的错误（工具执行失败落在
`toolCall.error`）用 `plugin:<pluginName>/<symbol>`。

**run/item 级符号名**（无数字码，落点 b/c）：

- `tool_failed` —— 工具执行失败。
- `tool_canceled` —— 工具所属 Run 的取消终止了一个已经开始执行的工具；它不是工具执行错误，也不是审批拒绝。父 Run 上代表已取消 child Run 的 Delegate Tool 仍使用独立的 `child_run_canceled`。
- `denied_by_user` —— **用户**在 HITL 里拒绝该工具（item 级）。
- `agent_stuck` —— agent loop 无前进进度被守卫终止（run 终态错误，区别于落 `internal_error` 的意外失败）。
- `run_lost` —— runtime 重启时发现 executor 已消失且没有可恢复 interrupt；启动恢复把该 Run 及仍在 running 的 Item
  原子收敛到终态，**metrics 精确等于最后一次 committed 快照**（不猜、不清零）。
- provider 失败按模式拆出的稳定符号：`rate_limited`（值得等，带 `retryAfterSeconds`）/ `invalid_api_key`（等也没用，
  UI 引导改 key）/ `timeout`（值得等）/ `provider_unavailable`（值得等）/ `provider_rejected`（等也没用）/
  `provider_error`（兜底）。
- `internal_error` —— 未归类失败，三个落点通用兜底；完整错误只进 span，不上 wire。

客户端**只按 `type` 分支**，绝不 substring-match `detail`。

**内联状态级**：`mcp_dial_failed` / `mcp_authorization_required`（`MCPServer.error`）、
`mcp_authorization_failed`（`MCPAuthorizationAttempt.error`）、`provider_not_configured` /
`provider_test_failed`（`ProviderTestResult.error`）。**它们没有 `detail`**：一句英文人话对多语言 UI 是负资产，文案归
客户端按 `type` 查本地表。

---

## 9. Capabilities 与请求能力

`ServerCapabilities`（仅由 `runtime.discover` 返回）由五部分组成，每部分都是**runtime 真做得到的事**：

| 字段               | 含义                                                                                                       |
| ------------------ | ---------------------------------------------------------------------------------------------------------- |
| `features`         | 开放 map：能力 key → `{enabled, clientOptIn, requiredByRunProtocol}`                            |
| `limits`           | 强制执行的正数值：idempotency / run replay / MCP authorization-attempt retention / subscription fan-out |
| `runEvents`        | 本 build 会发的 `StreamEventType` 集合                                                                     |
| `runtimeTopics`    | 可订阅的失效 topic 集合（每个都有生产者）                                                                  |
| `streamingMethods` | 走流式响应的方法名集合（§7）                                                                               |

**`features` 是开放 map（与 `ClientCapabilities.features` 对称）**：runtime advertise 新能力 = 加一个 key，
老客户端按"忽略未知"自动容忍。每个 feature 自带两个协商事实：`clientOptIn`（客户端必须显式声明才给）、
`requiredByRunProtocol`（它改变 run 会发什么，因此进 `RunProtocolProfile`）。已知 key 与其含义见
`manifest.json` 的 `features` / `capabilityPolicy`（后者是**方法级门禁**：哪个方法在什么条件下需要哪个 feature，
dispatcher、discovery 与客户端 preflight 读的是同一份）。

规则：

- 缺省 / falsy feature 默认**关闭**；关闭时相关方法返回 `capability_not_negotiated` 并在
  `requiredCapabilities` 里点名。
- `enabled:true` 只代表 server 支持；若该 feature 的 `clientOptIn:true`，本次请求的
  `_meta.clientCapabilities.features.<key>.enabled` 也必须显式为 true。dispatcher 的静态门禁、解析 durable
  identity 后的 state-dependent 门禁，以及 SDK preflight 使用同一判定；任何一处都不能把 discovery 当成替
  client 自动 opt-in。
- 门禁只咬真正要求能力的请求：root-only 查询和 root cancel 不要求 `features.subagents`；请求参数显式展开子树或
  durable identity 确认目标是 child 时才要求。这样既不返回假装完整的降级结果，也不让能力协商妨碍紧急停止。
- server **必须不**产出 client 未在 `interruptTypes` 声明的 open interrupt（§6.2）。
- `features.subagents` 关 → 不产出子 Run。
- **`excludedEphemeralEvents` 是专用两值集合**：
  `"item.delta" | "segment.progress"`。authoritative 事件在类型上不可填入，
  未知值返回 `invalid_params`，不会被静默忽略。
- `excludedEphemeralEvents` **不作用于失效流**（§7.8）：那条流的收敛范围由订阅的 topic 决定。
- **client 必须忽略未知字段**，server 不得在本 request/stream 发出 `runEvents` 之外的事件类型。

---

## 10. 历史 / 重连 / 恢复

### 10.1 正常断线重连

1. 对每个仍在执行的 root run 调 `runs.subscribe{runId, segmentId}` 续流（每个 run 一条流，传输细节见
   TRANSPORT §9.2）；
2. 带上最后一个**折叠成功**的事件 id（`Last-Event-Id`）让 server 重放 replayable 缺口。**cursor 只由折叠推进** ——
   不要用 ack 回来的 `headEventId` 覆盖自己的位置（§7.3）；
3. `replay_unavailable` 时，已挂载 Session 改走 `sessions.snapshot` 一次补齐 material view；只持有单个 Run
   的通用消费者仍可用 `items.list` + `plan.get`，然后 tail 重接（不带 cursor = 只订将来）；
4. 按 `itemId` 与 `eventId` 去重。**non-replayable preview 不重放**（§5.2 保证正确）。

一个 Run **活得比它的流长**：流在没有本段终态的情况下结束，是**连接掉了**，不是 run 结束了——服务端那边它还在跑。
按最后折叠的事件重接，把这件事变成毫秒级的空隙，而不是一个冻到下次重载的时间线。

### 10.2 进程/客户端重启恢复

1. 完成 discovery，并在读取前建立所需 `runtime.subscribe` generation；
2. `sessions.list` / `sessions.get` 对账身份；
3. 对每个已挂载 Session 调一次 `sessions.snapshot`，在同一数据库提交点恢复 Items、Runs、HITL、Plan 与 Goal；消费者必须把响应视为一个 material unit，并且只在该读取仍拥有当前 view generation 时整体提交；
4. 未挂载 Goal 与其他不属于 Session material 的 capability resource 继续调用各自 recovery query；不能把独立资源目录或筛选语义扩进 `sessions.snapshot`；
5. 折叠读取期间到达的失效事件并按需重读；`runs.resume` 应答 interrupt（payload 自包含，无需额外 join，§4.8）。

### 10.3 还原 Run 树

`items.list` 每页带 `runs: RunSummary[]`。客户端按 `runId` 把 item 归到 Run，再用 `spawnedByItemId` /
`parentRunId` 把子 Run 嵌到父 toolCall Item 下（子树需 `features.subagents`）。续段（resume）不产生独立 Run ——
一个 Run 的停车-续跑各段共享同一 `runId`（§0.3），无需串链。

三个独立 run 视图职责不重叠：`runs.list`（全历史 + 状态过滤）/ `interrupts.list`（待解集）/
`items.list.runs`（这一页的历史结构）。`sessions.snapshot` 是明确的挂载恢复 join，在一次事务中组合这些事实，
不是第四个可筛选资源目录。

---

## 11. 扩展选择指南

- 要进历史、供用户回看的工作产物使用一等 Item，并通过 `items.list` 与 Run 树恢复。
- 独立可变事实使用一等领域资源：定义命名 event、冷读、写 owner、scope 与 revision 语义。Plan 是当前实例；它不建立
  通用 key registry，也不授权第三方把任意 JSON 塞进 Run 流。新增一等资源会扩充闭合 event/topic union，必须按 §12
  评估并前移 `protocolVersion`。

---

## 12. 版本规则

- `protocolVersion` 是日期串（本定稿 `2026-08-24`）：**本 build 只服务一个精确版本**，协议没有兼容范围。
- 版本不兼容以 request 级 `invalid_protocol_version` 返回（带上本 build 服务的精确版本），**不存在连接级硬断开**。
- **加什么不用 bump**：加 method / 加可选响应字段 / 加 `features` map key / 加开放枚举值 → 同版本号。
- **加什么必须 bump**：新增请求字段（旧 server 严格拒绝）、**给闭合枚举或闭合 union 加成员**（客户端对它写
  exhaustive switch，§2.3）、加一等事件/资源、改语义 / 删字段 / 改字段类型。
- **判据不是"加还是改"，而是"老客户端会不会做错事"**。这条规则由 CI 强制：compatibility differ 拿本次产物与
  上一版基线对比，判定 breaking 就要求同批 bump（§14）。
- `SessionArtifactVersion` 与 `protocolVersion` 各自独立编号（本定稿 artifact = **23**）：一份归档可能被一个更新的
  runtime 读到。不认识的版本确定性拒绝，**dev 阶段不写 migration**。
- HTTP URL 里的 `/v2/`（wire major epoch）与日期 `protocolVersion`（epoch 内请求版本）是两个层级
  （见 TRANSPORT §6.1）。

---

## 13. 明确不做

- server→client JSON-RPC request。
- 远程多用户鉴权（协议层零 user 概念）。
- JSON-RPC batch。
- stdio transport。
- 客户端自选的业务资源 id。
- 单个 Session 绑定多个 workspace（`Session.workspace` 始终单根）。
- **强类型领域工具变体**（`commandExecution`/`fileChange`/… 作为 wire 一等类型）——核心领域中立，富渲染走客户端
  展示注册表（§4.4）。
- **本轮不做子 Run 的行为**：`features.subagents` 恒 false，不产出 child run。血缘字段、`suspended` 段终态、
  `runs.list.includeDescendants` 等**形状全量存在**——它们是为了让打开这个开关时不必再动一次 wire，也让归档里
  带血缘的文档现在就被明确拒绝而不是被悄悄摊平（§4.2）。

---

## 14. 机器可读制品 / 漂移闸

公共 Go `runtime/protocol` + 私有 binding-neutral Operation Registry 是**机械 SSOT**。`go generate ./...` 从它们导出
`runtime/contract/`（manifest / JSON Schema / OpenRPC / 人读索引 / 错误注册表 / 能力门禁 / 事件策略 /
canonical 样本）以及前端消费的 TS 类型、校验器与 client stub。

Registry 自身也属于合同边界：method / retry / condition / constraint / recovery 等闭合 metadata
出现未知值、重复声明或互相矛盾时，进程初始化与生成器必须确定性失败，不能把未知值格式化成某个合法默认值继续发布。
对外枚举视图返回快照，调用方不能通过修改 slice 反向改写事实源。method 的有效错误集 =
registration 显式业务错误 + capability rule 派生的 `capability_not_negotiated`；manifest、OpenRPC、错误注册表与人读
索引只消费这一份派生结果，不各自维护“哪些方法会返回能力错误”的名单。

**本文与这些制品的分工是硬的**：制品是字段级真相，本文是语义与不变量。本文里不出现字段表、方法表、错误码表 ——
一个事实一个作者。

CI 的 drift gate（契约 §11.4，18 项）覆盖：`go generate` 后 worktree 无 diff；Registry 方法集 == dispatcher 集合；
capability 规则在 dispatcher / discovery / SDK preflight 三方等价；schema 与 OpenRPC 可解析且无悬挂 `$ref`；
每个闭合 union 有 discriminator 与完整变体；约束在 Go / schema / TS 三方等价；DTO validator 无 store 依赖；
每条 system invariant 有跨 projection fixture；TS 产物可编译且**都有消费者**；canonical 样本三方通过（含一个不
参与生产的 JSON Schema 验证器）；list query fixture；**protocol manifest / canonical 文档 / 代码 / canonical 样本
版本一致**；错误 type↔code 单一源；Plan 的 live event、cold read、Session material 与 archive shape 一致；
Artifact v23 round-trip；compatibility differ 判定 breaking 并要求同批 bump（§12）。

---

## 15. 安全不变量汇总

- **路径 containment**：fs 工具路径相对 `WorkspaceRef.path`，越界 → `path_outside_root`。
- **provider secret**：只回 `apiKeyMasked`，永不可逆推（§4.9）。
- **协议层零鉴权**：无 user / account 概念；本地进程门禁由 transport 层处理（TRANSPORT §11）。
- **workspace 是业务身份不是传输上下文**：`WorkspaceRef` 走 body，不走带外 directory header（TRANSPORT §2）。
- **防挂死**：server 不产出 client 解不了的 open interrupt（§6.2）。
- **归档不越权**：import 在任何写入之前拒绝它无法完整还原的文档（未提供 Plan 能力、带血缘的 run 树、
  不认识的版本）。

---

## 附录 A · 设计不变量摘要

1. **领域中立核心**：核心只懂 Session/Run/Item/通用 `tool`；新工具零协议成本（§4.4）。
2. **一个判别字段 `type`**：所有联合看 `type`，`kind` 不在 wire 上出现（§2.1）。
3. **authoritative / replayable / persisted 三分**：丢掉每个 non-authoritative 事件仍得正确终态；
   replay journal 与 SSE id 只看 replayable；跨进程恢复只读持久化 projection。wire 上没有 reliability flag（§5.2）。
4. **状态与终态正交**：`RunStatus` 三态，`outcome` / `activeSegmentId` / `finishedAt` 各只在对应状态下存在；
   `metrics` 与 `outcome` 分家（§4.2）。
5. **HITL = R 模型**：interrupt 收尾当前**段**、`runs.resume` 在同一 Run 上续段；所有 interrupt payload
   **自包含**（§6 / §4.8）。
6. **元数据带外、业务进 params**：workspace/sessionId/runId 进 params；trace/版本/token/游标走带外（TRANSPORT §2）。
7. **能力开放可加、但闭合集合加成员要 bump**：`features` 是开放 map；闭合 union / 枚举 / 一等事件加成员是
   breaking（§2.3 / §12）。
8. **discovery 只说做得到的事**：每个 topic 有生产者、每个 feature 有真实门控、每个数字被强制执行（§9）。
9. **错误三落点、一个形状**：`ProblemData` 唯一形状，落点即判别，符号名即判别键（§8）。
10. **集合一个信封，能力不造假**：所有 list 是 `Page<T>`；只有真正可续页的读接受 cursor，且 cursor 绑定完整查询；
    有界集合不静默截断（§4.11）。
11. **id server 生成 + 类型前缀**；`eventId` 在段流内单调，cursor 是可回传的续流锚（§2.4）。
12. **Minimal Profile 是"少声明"不是"少收"**：核心生命周期事件必发，客户端只能抑制 ephemeral 预览（§3.2 / §9）。

---

## 附录 B · 设计借鉴（取思想，不取命名）

本协议名实第一性自决；下列业界设计仅取**思想**作独立印证，**不作命名锚**。

**Stripe（API 工程范式）**：日期串 pin 版本（§12）；列表统一信封 + `has_more` 语义（§4.11）；错误是结构化对象
（稳定 `type` + 字段级 `errors[].field` + 可选 `docUrl`）；资源自描述用类型化 id 前缀（§2.4）；命名拼全、单位入名
（§2.2）。**不采**：`expand[]` 嵌套展开（YAGNI——用显式 join，如 `items.list.runs`）。

**opencode（同类 agent runtime）**：通用 tool envelope `{name, arguments, result}`、无 per-tool 类型（§4.4）；
全程 `type` 判别；delta + 权威终态两段式流；可排序 id；Usage 非重叠细分。**不采**：深层点号事件名；per-event
`version` 字段；分裂的 permission/question 双审批（我们统一到 HITL interrupt）。

**MCP / JSON-RPC**：envelope 形态（§1）、capability 声明（§9）、streamable HTTP（TRANSPORT §6）取自此脉络。

---

## 附录 C · 已落地扩展说明

### C.1 · `workspace.files.list` / `workspace.files.read`（基础读，不门控）

`listFiles` 是 gitignore-aware 的目录读（repo 用 `git ls-files --cached --others --exclude-standard`，非 repo 走
兜底 walk + 排除 `.git`/`node_modules` 等），支持 `path` / `recursive` / `glob` / `includeIgnored` / 分页；每项的
`type` / `sizeBytes` / `modifiedAt` 来自**同一次 lstat**。超过安全扫描边界时返回 `invalid_params` 并要求收窄
path，**不静默截断**。

`readFile` 读整文件或 `startLine..endLine` 窗口（**1-based 闭区间**，面向编辑器）；`totalLines` 始终是整文件行数；
路径同样 jail 到根，binary 文件明确报错。`truncated` 自描述触顶 `maxBytes`。

### C.2 · `approval.*`（不门控）—— 全局审批姿态 + 记忆决策

`ApprovalMode` 是 **每 Runtime 一个的全局策略**（非 per-session），与 `toolCall.safetyClass`（per-tool 风险）正交，
二者合决一次调用是否驻留待批。`approval.getMode` / `setMode` 取代了原 agent/chat/plan 的 per-run `mode`；
`approval.listRules` / `forgetRule` 是持久细粒度规则的读 + 管理面（写入面是 HITL 应答里的 `remember`，§6.1）。

> `plan` 是"计划"的归宿：它是一个**姿态**而非 run 模式。模型在该姿态下调研完调 `exit_plan_mode`（一个 `question`
> interrupt，§6）呈交计划；批准 → 姿态自动翻回 `balanced` 执行，拒绝 → 留在 `plan`。

### C.3 · `compaction` Item —— 自动上下文压缩

runtime 在 turn 边界需要压缩时产出它（`item.started` + `item.completed`，`droppedMessages` = 净减条数），客户端
fold 成时间线分隔条。摘要文本已折进重写后的对话历史。压缩策略属于 runtime 领域服务，不开放会与自动策略竞争的
手动 RPC。

### C.4 · `plan.updated`（门控 `plan`）—— 模型的工作清单

模型 `set_plan` 写入 Session Plan；root Run 是唯一 writer，`plan.get` 是 cold read。整表替换、带单调 `revision`
（§5.3）。`PlanStep.id` 是**位置序**，随整表替换，不是持久身份。

---

> 正式契约。配套同目录 [`TRANSPORT.md`](./TRANSPORT.md) 与 [`AUX_API.md`](./AUX_API.md)；字段级真相在
> [`runtime/contract/`](../contract/)。
