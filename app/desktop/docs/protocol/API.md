# Lyra Runtime Protocol（定稿 `2026-06-07`）

> **状态：正式契约（canonical, frozen baseline）。** 本文是 Lyra 客户端 ↔ Lyra Runtime 的唯一 wire 契约真相源。
> 物理传输见同目录 [`TRANSPORT.md`](./TRANSPORT.md)。本文与 `TRANSPORT.md` 自包含、互为配套，不依赖任何其它文档。
>
> 类型命名以后端 Go interface（`lyra/internal/delivery/protocol`）为机械 SSOT，经 codegen 派生 TS / 其它语言类型与
> JSON Schema / OpenRPC 制品（§14）；本文与生成产物**同名零映射层**。
>
> `protocolVersion`: **`2026-06-07`**。

---

## 目录

- §0 模型与概念
- §1 Wire 格式（JSON-RPC 2.0）
- §2 命名规范（**一个判别字段 `type`**）
- §3 Lifecycle（无状态 discovery / 取消 / §3.1 端到端走查 / §3.2 Minimal Profile）
- §4 数据类型目录（schemas）
- §5 流式（事件信封 / Item·Run·State 事件 / 不变量）
- §6 Human-in-the-Loop（R 模型）
- §7 方法参考（逐方法：输入 / 输出 / 错误 / 示例）
- §8 错误（投递通道 + 落点决策表 + 错误码表）
- §9 Capabilities 与请求能力（开放 features map）
- §10 历史 / 重连 / 恢复
- §11 三个扩展缝：Item / state / custom（选择指南）
- §12 版本规则
- §13 v2 明确不做
- §14 机器可读制品 / 漂移闸
- §15 安全不变量汇总
- 附录 A 设计不变量摘要

---

## 0. 模型与概念

Lyra Runtime 是一个**本地、领域中立的 agent runtime**。客户端可以是 web UI、桌面外壳（Wails/Tauri/Electron）、TUI 或
另一个本地进程。协议是 **JSON-RPC 2.0**，跑在多种 transport（InProcess / IPC / HTTP）上，语义在所有 transport 上一致。

**"领域中立"是核心设计立场**：协议核心只懂 Session / Run / Item / 通用工具调用这套**通用原语**；"某个工具长什么样、
该怎么富渲染"是**领域知识**，不焊进 wire（见 §4.4）。换个领域（客服 / 数据分析 / 运营）协议核心一字不改。

### 0.1 三级资源模型

```text
Session            会话；绑定一个工作目录 cwd          id: ses_…
  └─ Run           一次 agent 执行，从用户输入到一个 outcome    id: run_…
       └─ Item     run 内一个 durable 工作单元                  id: item_…
```

- **Session**：一次对话，绑定 `cwd`。
- **Run**：一次 agent 执行；以一个 `RunOutcome` 收尾（completed / error / maxSteps / maxBudget / canceled / interrupt）。
- **Item**：run 内一个 durable 工作单元 —— `userMessage` / `agentMessage` / `reasoning` / `plan` / `question` / `toolCall`。

**`Item` 是历史与流式的唯一原语**：流式推 Item 生命周期事件（`item.started → item.delta* → item.completed`），
历史 `items.list` 返回 completed Item。**没有独立的 `Message` 资源类型**；"消息"就是 `userMessage`/`agentMessage`
两种 Item。

### 0.2 工程目录模型（cwd）

- `cwd` 是**项目身份 + 文件系统工具根**。`Session.cwd` 建会话时设定，可经 relocate（`sessions.update`）更新。
- runtime **不持有连接级的 active project**；workspace 读方法显式收 `cwd?`，缺省 = runtime serve 目录（`ServerInfo.cwd`）。
- `Project` 是按 `Session.cwd` 分组的**派生视图**：无不透明 id、无 active 标记。
- `projectRoot` 只是**配置发现根**（向上找到最近含 `.git` 的祖先，找不到回落 cwd），**不取代** cwd 作为身份/工具根。

### 0.3 HITL 采用 R 模型（分段续跑）

agent 需要人介入（审批 / 提问 / client 侧工具结果）时，**当前流式段（segment）结束**，以 `RunOutcome.type="interrupt"`
收尾、资源全释放；**Run 本身（稳定 `runId`）不结束**——它挂起为一个持久 `OpenInterrupt`。客户端用 `runs.resume{runId}`
在**同一个 Run 上开新的一段**来应答。无"活跃挂起 run"、无被占用的 goroutine。详见 §6。

> **Run vs Segment（贯穿全协议的身份模型）**：一个 **Run**（`runId`，`run_…`）是一次逻辑运行的**稳定身份**——`runs.start`
> 铸一次、跨所有 interrupt/resume 周期与进程重启**永不重铸**。一个 **Segment**（`segmentId`，`seg_…`）是该 Run 的一个连续
> 流式段：`runs.start` 开第一段，每次 `runs.resume` 开新的一段。事件流、重连/回放都**按 Segment**划界；Run 的生命周期跨其
> 全部 Segment。终态信号 `segment.finished` 标记的是**当前 Segment 的结束**——`outcome.type=interrupt` 表示停车待续（同 Run 会有
> 下一段），其余 outcome 表示 Run 真正终结。

### 0.4 收敛点

- **一个下行事件方法**：`notifications.run.event` 承载 run/item/state 全部事件（HTTP 上随对应流式调用的响应流投递，见 TRANSPORT §6.4）。
- **一个终态信号**：`segment.finished{ outcome }`，判别式 outcome。
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

每个 Request 的 `params` 若为对象，可带 `_meta`：

```ts
interface RequestMeta {
  protocolVersion?: string;
  clientInfo?: { name: string; version: string };
  clientCapabilities?: ClientCapabilities;
}
```

`_meta` 是请求自描述元数据，不属于业务参数；runtime 在 dispatch 边界剥离后再解码具体 method params。

规则：

- envelope `id` 一律 **string**；Response 的 `id` 匹配 Request。
- Response 必须恰好含 `result` 与 `error` 之一。
- **不支持** JSON-RPC batch。
- **不支持** server→client request（HITL 用"通知 + 后续 client request"，§6）。
- `params._meta` 承载协议版本 / clientInfo / clientCapabilities 这类**请求自描述元数据**；dispatch 在业务参数解码前剥离。
  传输元数据（trace / 幂等键 / 门禁 token / 流游标）仍走带外通道。判定见 [`TRANSPORT.md`](./TRANSPORT.md) §2。

---

## 2. 命名规范

### 2.1 判别字段：**一律 `type`（无例外）**

**所有判别联合（discriminated union）的判别字段名一律是 `type`**。`kind` **不在 wire 上出现**——无论作判别字段还是
枚举属性。这是一条无例外的硬规则：写 reducer / 序列化 / codegen 时永远只看 `type`，消除"这个看 type、那个看 kind"
的认知税与拼错判别字段的无声 bug。

> 与 `type` 区分的是**状态/分类枚举属性**（不判别对象形状，只标一个有限态）：这类字段按其语义命名（`status` /
> `decision` / `scheme` / `safetyClass` 等），不叫 `kind` 也不滥用 `type`。判据：它决定"对象是哪个变体"→ 用 `type`；
> 它只是"对象的一个有限态字段"→ 用语义名。

### 2.2 字段与枚举

- 字段名、枚举值一律 **camelCase**。
- 缩写**白名单**（写死，其余全词）：`id` / `url` / `mime` / `cwd` / `api`（如 `apiKey` / `apiKeyMasked`）。不在表内的缩写一律展开成全词。
- 单位用显式后缀，**后缀集也写死**（新单位先在此登记，杜绝随意缩写）：
  - `Usd`（货币）：`maxBudgetUsd` / `inputUsdPerMillionTokens`
  - `Ms`（毫秒）：`durationMs`
  - `Seconds`：`timeoutSeconds`
  - `Bytes`：`sizeBytes` / `maxBytes`
  - `At`（ISO-8601 时刻串）：`createdAt` / `expiresAt` / `finishedAt`

### 2.3 开放 vs 闭合枚举（原则）

- **面向插件 / 未来扩展的分类** → **开放 `string` + §2.6 命名空间**（first-party 裸符号，第三方 `plugin:` 前缀）。
  例：`safetyClass`、error `type`、`custom` 事件 `name`、共享 state 顶层 key。
- **纯内部有限态** → **闭合枚举**。例：`RunStatus`（`running|finished`）、`ItemStatus`、`SessionStatus`。
- 拿不准时问：这个分类**会被插件 / 新领域扩展**吗？会 → 开放；只在 runtime 内部取有限值 → 闭合。

### 2.4 资源 id（server 生成 + 类型前缀）

业务资源 id **一律 server 生成**：

| 资源            | 前缀    | 说明                                           |
| --------------- | ------- | ---------------------------------------------- |
| Session         | `ses_`  |                                                |
| Run             | `run_`  | 幂等见 §7；client 不传                         |
| Item            | `item_` | HITL 关联键（§6）                              |
| Segment         | `seg_`  | Run 的一个流式段（§0.3）；server→client，client 不传 |
| Background task | `tsk_`  |                                                |
| Event           | `evt_`  | **单个 Segment 流内单调**，流续传/去重锚       |

- 客户端只生成两种非业务 id：JSON-RPC envelope `id`、幂等键。
- **id 自带类型**：前缀（`ses_`/`run_`/`item_` …）即资源的**自描述类型标签**——一个 id 的归属无需额外字段就能识别（不另设
  `object`/`resourceType` 字段，前缀已足）。
- **id 不透明、不承载顺序**：除前缀外，id 内部结构**不是契约**——client **不得**按 id 字典序推断创建顺序或时间。排序一律
  锚定**显式字段**：会话 / 历史按 `createdAt`（ISO-8601），run 内事件按 `eventId`（下条）。分页 `cursor` 同样不透明（§4.11）。
  > 实现**可选**用时间可排序 id（ULID/KSUID 式，保留类型前缀）以获得 restart-stable 的事件序与 cursor-by-id 便利，
  > 但这是**实现优化、非 wire 保证**；client 永远按上面的显式字段排序，不依赖 id 形状。
- **`eventId` 作用域**：在单个 **Segment 流**内单调——这是**唯一**被契约保证"单调有序"的 id。一条段流复用根 Run 该段 +
  所有子孙 Run 的事件。`runs.resume` 在同一 Run 上开**新的一段**（同 runId + 新 segmentId，eventId 从头）；subagent Run =
  事件并入父段流、共享父序列。client 按 `segmentId` 划定去重/续传作用域（§5）。

### 2.5 事件名 / 方法名

- 事件名小写 `domain.action`：`segment.started` / `segment.finished` / `item.started` / `item.delta` / `item.completed` /
  `state.snapshot` / `state.delta` / `custom`。运行时自有行为**必须**用一等事件/Item 类型；`custom` 只留第三方扩展。
- 方法名 `<domain>.<verb>`，HTTP URL 保留点（不斜杠化）。例：`runs.start` / `items.list` / `workspace.getDiff` /
  `workspace.mcp.listServers`。

### 2.6 第三方扩展命名空间（防撞名）

**唯一**的可枚举标识命名约定，统一适用于所有"first-party 与第三方共用一个 keyspace"的扩展缝：

- **first-party**（runtime 自身 / 内置）用**裸符号**（`session_not_found` / `progress`）。
- **第三方插件**产出的同类标识一律加前缀 `plugin:<pluginName>/<symbol>`（如 `plugin:acme/progress`），
  避免与 first-party 及彼此撞名。

适用面（**全部沿用此一条**，不各自再定）：

| 缝                           | 标识     | 见       |
| ---------------------------- | -------- | -------- |
| `custom` 事件                | `name`   | §5 / §9  |
| 共享 `state`                 | 顶层 key | §5 / §11 |
| error                        | `type`   | §8.4     |
| 开放枚举（如 `safetyClass`） | 值       | §2.3     |

client 路由：裸符号按 first-party 集匹配；`plugin:` 前缀按 `<pluginName>` 分发。

---

## 3. Lifecycle

```text
connect → operate → runtime.shutdown → disconnect
```

runtime 是无状态 JSON-RPC 服务：业务方法**不要求**先调用 discovery。client 可在启动时调用 `runtime.discover`
或 HTTP `GET /v2/info` 读取 serverInfo / capabilities，但这只是信息查询，不改变连接状态。

协议版本、clientInfo、clientCapabilities 随每个 request 的 `params._meta` 发送。server 用这份 request-scoped
metadata 判断本次 run / subscription 能否产出某些事件或 HITL interrupt；不把 client 能力写成 runtime 进程全局状态。

取消有两个不同对象：

| 对象                    | 信号                                                                         | 效果                                   |
| ----------------------- | ---------------------------------------------------------------------------- | -------------------------------------- |
| 在飞的 JSON-RPC request | `notifications.canceled`（client→server 通知，带被取消请求的 envelope `id`） | 取消一个慢请求                         |
| agent run               | `runs.cancel`（request，带 `runId`）                                         | 硬终止在跑的 run（`outcome:canceled`） |

网络断开**不**取消 run。

### 3.1 一次 run 的端到端走查（先建立心智，再看 §4 类型）

```text
runs.start ──▶ segment.started ──▶ (item.started → item.delta* → item.completed)*  ──▶ segment.finished{outcome}
                                  └ assistant message / reasoning / toolCall 逐个流式落地        │
                                                                                                ├─ completed / error / … → 结束
                                                                                                └─ interrupt → 待人介入（见下）
```

1. **起 run**：客户端 `runs.start{ sessionId, input }`，**立即**返 `{ runId }`，同一条流随即推 `RunEvent`（§5）。
2. **流式产出**：先 `segment.started`，然后每个 Item 走 `item.started`（壳）→ `item.delta*`（文本 / 工具入参 / 输出增量，§5.1）→
   `item.completed`（权威终态）。assistant 的 message / reasoning / toolCall（§4.3 / §4.4）就这样逐个落地。
3. **需要人介入**（HITL，§0.3 / §6）：当前**段**以 `segment.finished{ outcome: interrupt }` 收尾、资源释放；**Run 不结束**，
   待解项作 `OpenInterrupt` 持久化（跨重启可发现）。
4. **续段**：客户端 `runs.resume{ runId, responses }` 在**同一 Run 上开新的一段**（同 `runId`、新 `segmentId`），又一段
   `segment.started → items → …`。**所以一个"对话回合"= 一个 Run 的若干 Segment**，始终一个 `runId`。
5. **收尾**：某一段以 `segment.finished{ outcome: completed | error | maxSteps | maxBudget | canceled }` 终结整个 Run。
6. **历史 vs 实时**：重开会话用 `items.list` 按持久 seq 重建（§10）；实时流内按 `eventId` 排序（§5）。两套各自权威，靠 item id 关联。

### 3.2 Minimal Profile（最小可用客户端）

只想做"发消息 → 流式显示回复"的客户端**最少**只需实现下面这一小撮，其余全是**分层可选能力**（各由 §9 capability
门控，关掉就不会产出对应事件 / interrupt）：

| 必需面                                                                       | 用途                                                 |
| ---------------------------------------------------------------------------- | ---------------------------------------------------- |
| `runtime.discover`（可选）                                                   | 读取 serverInfo / capabilities，用于 UI feature gate |
| `sessions.create`                                                            | 开一个会话                                           |
| `runs.start` + 消费 `notifications.run.event` 里的 `item.*` + `segment.finished` | 发消息、流式显示、知道何时结束                       |
| `items.list`                                                                 | 重开会话补历史                                       |

**最小 client 声明**：`ClientCapabilities.events` 只列它认得的事件（如 `["segment.started","segment.finished","item.started","item.completed","item.delta"]`）、
`interruptTypes: []`（不处理 HITL）。server **必须**尊重：不产出 client 未声明的事件类型、不产出 client 未声明 type
的 open interrupt（§6.2、§9）。于是"只实现一个子集"是协议合法状态——HITL / subagents / state /
文件监听等全部可后续加，不回头改最小路径。

---

## 4. 数据类型目录（schemas）

> 本节是所有 wire 类型的权威定义。§7 方法参考引用这里的类型，不重复定义。

### 4.1 Session / Project

```ts
type SessionStatus = "running" | "waiting" | "idle";

interface Session {
  id: string; // ses_…
  title: string;
  status: SessionStatus;
  model: string; // 默认模型 id
  cwd: string; // 绝对路径，server 已解符号链接
  projectRoot?: string; // 派生：最近 .git 祖先，无则 = cwd
  cwdMissing?: boolean; // cwd 在磁盘失联 → 降级纯聊天 + 可 relocate
  createdAt: string; // ISO-8601
  updatedAt: string;
  usage?: Usage; // 本 session 累计
  metadata: Record<string, unknown>;
}

interface Project {
  // 派生视图：distinct Session.cwd；无 id、无 active
  cwd: string; // 唯一身份
  name: string; // basename(cwd)
  projectRoot?: string;
  branch?: string; // git 分支，best-effort 装饰
  sessionCount: number;
  lastActiveAt?: string;
  cwdMissing?: boolean;
}
```

### 4.2 Run

```ts
interface RunRef {
  id: string; // run_…（稳定逻辑 Run 身份，跨 resume 永不重铸；§0.3）
  sessionId: string;
  spawnedByItemId?: string; // child-of：本 Run 是子 agent，由该 toolCall Item 派生
  status?: "running" | "finished";
  outcome?: RunOutcome; // status=finished 时给
  model?: string; // 本 Run 用的 model（Model.id）；空=运行时默认
  provider?: string; // 本 Run 用的 provider（Provider.id），与 model 配对；空=默认。
  // 盖在 Run 上使其自描述：usage.summary 按 provider 归因免去 model→provider 反推
  createdAt?: string;
  finishedAt?: string;
}

type RunOutcome =
  | { type: "completed"; result: RunResult }
  | { type: "error"; result: RunResult } // result.error: ProblemData（含 detail）
  | { type: "maxSteps"; result: RunResult; detail?: string } // 单个 Run 内 agent 步数上限（按 step 计，非 turn）
  | { type: "maxBudget"; result: RunResult; detail?: string } // 成本上限（含子 agent 子树）；detail 如 "花了 $4.20 / 上限 $4.00"
  | { type: "canceled"; result: RunResult; detail?: string } // runs.cancel 的 reason 回流到此
  | { type: "interrupt"; interrupts: Interrupt[] }; // ★可恢复；Run 已结束、资源释放

interface RunResult {
  usage?: Usage; // 总成本读 usage.costUsd（不另设 RunResult.costUsd，避免两处总成本不一致）
  steps?: number;
  error?: ProblemData; // outcome.type=error 时给
  durationMs?: number; // run 墙钟耗时（跨 interrupt/resume）；任一终态可显示 "took 12.4s"
}
```

> `spawnedByItemId`（子 agent）是 RunRef 上**唯一的 run 树边**：延续（resume）不再是独立 Run，而是**同一个 `runId` 的新
> Segment**（§0.3），故没有 `parentRunId`。
> `model` 落在 RunRef 上：`runs.subscribe` 重连或 `items.list.runs` 历史还原**没见过原始 `runs.start` 请求**时，
> 仍能据此标注"这条 Run 用的哪个模型"。
> **没有 `mode` 字段**：agent/chat/plan 这套 per-run 模式已移除——run 永远是带工具的 agent 循环，"计划"是
> 一个**全局审批姿态**（`ApprovalMode = "plan"`，见 §C.3），不是 run 的属性。
> **非 error 终态的 `detail?`**（S6）：让客户端能区分"被用户取消" vs "被超时取消"、能给 maxBudget 显示花费/上限；
> `runs.cancel` 的 `reason` 经此回流到 outcome。error 的人读说明仍在 `result.error.detail`（§4.6），不另开第二处。

### 4.3 Item

```ts
type ItemStatus = "running" | "completed" | "incomplete"; // running=进行中；incomplete=被中断/取消而未完成

interface ItemBase {
  id: string; // item_…
  runId: string; // 所属 Run（子 agent 的 item.runId = 子 Run）
  status: ItemStatus;
  createdAt: string;
}

type Item =
  | (ItemBase & { type: "userMessage"; content: ContentBlock[] })
  | (ItemBase & { type: "agentMessage"; content: ContentBlock[] })
  | (ItemBase & { type: "reasoning"; text: string; redacted?: boolean })
  | (ItemBase & { type: "plan"; steps: PlanStep[] })
  | (ItemBase & { type: "question"; question: Question })
  | (ItemBase & {
      type: "toolCall";
      tool: ToolInvocation;
      safetyClass?: string;
      error?: ProblemData;
    });
```

- `question` 是一等 Item：一个提问可能成为 durable open interrupt，之后由 `runs.resume` 应答。
- `toolCall.error`（+ `status:"incomplete"`）是**工具级失败的统一结构化落点**。工具失败**通常不终止整个 run** ——
  agent 可据此换方案、继续（§8 通道 b）。
- **`compaction`** —— additive 第 7 变体，标"此处压缩了 N 条更早消息"。turn 边界的**自动压缩已落地**：产出该 Item（`item.started` + `item.completed`，带 `droppedMessages` = 压缩前后净减条数），fold 成时间线分隔条。显式 `sessions.compact` RPC 仍是提案（613 B10，见附录 C.4）。

```ts
type ContentBlock =
  | { type: "text"; text: string }
  | { type: "image"; mime: string; data: string }; // 图片内联：mime=媒体类型，data=base64（无 data: 前缀）

interface PlanStep {
  id: string;
  title: string;
  status: "pending" | "running" | "completed" | "failed"; // "进行中"统一用 running（§2.3）
}

interface Question {
  prompt: string;
  fields: QuestionField[];
}
interface QuestionFieldBase {
  name: string; // answers 按此 key 索引
  label: string;
  header?: string; // ≤12 字短标签（UI chip）
  required?: boolean;
}
type QuestionField =
  | (QuestionFieldBase & { type: "text" })
  | (QuestionFieldBase & {
      type: "choice";
      options: QuestionOption[];
      multiple?: boolean;
    });
interface QuestionOption {
  label: string;
  description?: string; // 选项释义
  preview?: string; // 侧边预览（方案对比）
}
```

### 4.4 ToolInvocation（领域中立的通用信封）

**核心只有一个工具形状**（不是联合）。`name` 是身份，`arguments` 是已解析 JSON 对象，`result` 是 best-effort JSON
输出。"某个工具该怎么富渲染"是**领域知识**，不进 wire——由客户端按 `name` 命中的**展示注册表**叠加（§4.4.2）。

```ts
interface ToolInvocation {
  name: string; // 工具身份（稳定）；MCP 用 "<server>.<tool>"
  arguments: Record<string, unknown>; // 已解析 JSON 对象（永不回传 JSON 字符串）
  result?: unknown; // best-effort JSON 输出；item.started 壳上无，item.completed 上权威落定
}
```

设计后果（一次性消除一整串旧形状缺陷）：

- **无 `kind`↔`name` 双重身份**：只有 `name` 一个身份字段，不存在"强变体没 name / 通用变体有 name"的分裂。
- **审批 / resume 关联键恒可得**：`ApprovalPayload.tool.name` + `arguments` 永远在 wire 上，server 无需把后端内部
  resume 绑定数据（旧设计的 `_resume`）塞进 payload —— 该泄漏从根上不存在。
- **新工具零协议成本**：加一个工具 = 在展示注册表登记一行（§4.4.2），**wire 不动、codegen 不动、契约不 bump**。
  这是协议级 OCP：核心对"新工具"开放、对"改 wire"封闭。
- **跨工具统一处理可行**：所有 toolCall 都有 `name` → "按 name 分组的调用日志 / 审计视图"等统一处理天然成立。

#### 4.4.1 入参 / 输出的硬约束

- **`arguments` 永远是 JSON 对象，绝不回传 JSON 字符串。** 流式部分入参走 `item.delta{ type:"toolArguments",
argumentsTextDelta }`（JSON 文本增量，§5.1）累积；server 在 `item.completed`（及审批 payload，§4.8）处 `unmarshal`
  成对象再发——消除"双重转义"。
- **`result` 是 best-effort JSON**：首选对象（可按约定取键），也允许任意 JSON 值（数组 / 标量 / 字符串）。硬约束：
  **绝不双重编码**（`{x:1}` 不是 `"{\"x\":1}"`）；纯文本用 JSON string 值即可（`result:"ok"`），能结构化就尽量给对象。
- **`result` 在 `item.completed` 上权威落定**（durable，§5.2）：流式期间的命令 stdout 经 `item.delta{ type:"toolOutput",
text }`（`durable=false`，可丢）预览；客户端**不可**把流式累积当终态来源——那会在历史回放 / 重连 / 后端整发（无 delta）
  时丢内容。丢掉每个 `toolOutput` / `toolArguments` delta，客户端仍必从 completed 的 `tool.result` / `tool.arguments`
  得到正确终态（权威落点校验表见 §5.2）。
- **工具级失败不进 `result`**：错误一律走 `toolCall.error`（ProblemData）+ `status:"incomplete"`（§4.3 / §8）。
- 子 agent 的子 Run 经 `子 RunRef.spawnedByItemId == 本 toolCall.id` 关联（也可在 `result.childRunId` 给便利值）。

#### 4.4.2 展示约定（非规范，客户端展示注册表用）

下表是**客户端展示层**按 `name` 富渲染时识别的 `arguments` / `result` 约定 —— **不是 wire 强制**。未知 `name` 一律
JSON 树兜底渲染（`arguments` 必为对象 → 树；`result` 是 JSON → 美化展开，非 JSON → 原始文本）。富 result 形状复用
§4.5 的可复用结构（`DiffRow` / `FileEdit` / `SearchHit` / `WebSearchResult`）。

| name（约定）             | arguments（取值键） | result（取值键）                         | 富渲染卡片                                                             |
| ------------------------ | ------------------- | ---------------------------------------- | ---------------------------------------------------------------------- |
| `shell`                  | `command`           | `{ exitCode, output, outputTruncated? }` | 命令卡片（output = 合并 stdout+stderr 全文；截断置 `outputTruncated`） |
| `edit` / `write`         | `path`, …           | `{ changes: FileEdit[] }`                | diff 卡片                                                              |
| `grep` / `glob`          | `query` / `pattern` | `{ hits: SearchHit[] }`                  | 本地搜索卡片                                                           |
| `webSearch`              | `query`             | `{ results: WebSearchResult[] }`         | 网络结果卡片                                                           |
| `read`                   | `path`, `range?`    | `{ content }`                            |                                                                        |
| `<server>.<tool>`（MCP） | 工具自定义          | 工具自定义                               | JSON 树（默认）                                                        |
| `subagent`               | `prompt` / `task`   | `{ summary, childRunId? }`               |                                                                        |

> 新工具只需在客户端展示注册表登记一行、约定其 `result` 形状即可富渲染；不约定也能 JSON 兜底渲染、开箱即用。
> **协议核心永不感知这张表。**

### 4.5 可复用结构（Diff / Search / 文件）

> 这些结构供 §7.5 `workspace.*` 方法**直接返回**，并被 §4.4.2 的工具 `result` 约定**复用**。它们是
> 领域便利结构，不是核心原语。

```ts
type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

// `workspace.getDiff` 结果：sum-type，按请求 `format` 二选一——rows→`files`、raw→`patch`。
// truncated=超 `limit` 在**文件边界**截断（整文件 rows 要么全在要么不在，无半截 diff；no silent caps）。
interface Diff {
  files?: FileDiff[];
  patch?: string;
  truncated?: boolean;
}
interface FileDiff {
  path: string;
  status: FileStatus;
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: true;
  rows: DiffRow[];
} // binary：rows=[]、added/removed 省略

// 过去式工作树状态词汇，三个文件类型共用（§2.1 分类属性用语义名 status，非 kind/type）。untracked 仅 VCS 有。
type FileStatus = "added" | "modified" | "deleted" | "renamed" | "untracked";
interface WorkspaceFileChange {
  path: string;
  status: FileStatus;
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: true;
} // VCS 工作区扫描态（listFileChanges）；added/removed binary 时省略（不伪造 0），previousPath 仅 renamed
interface FileEdit {
  path: string;
  status: Exclude<FileStatus, "untracked">;
  diff?: DiffRow[];
} // 一次编辑的应用结果（带 diff；无 untracked）

interface SearchHit {
  path: string;
  lineNumber?: number;
  snippet?: string;
} // 本地：grep=path+line+snippet；glob=仅 path
interface WebSearchResult {
  title?: string;
  url: string;
  snippet?: string;
  faviconUrl?: string;
} // 网络检索结果

interface FileHead {
  path: string;
  lines: FileLine[];
}
interface FileLine {
  lineNumber: number;
  text: string;
}
interface GrepResult {
  matches: GrepMatch[];
  total: number;
} // total ≥ matches.length（可能被 limit 截断）
interface GrepMatch {
  path: string;
  lineNumber: number;
  text: string;
}
```

> **三个文件类型共用 `FileStatus` 词汇，但意图不同、故各自独立**：`WorkspaceFileChange`（`workspace.listFileChanges` 的 VCS
> 工作区扫描态，含 `untracked`）/ `FileDiff`（`workspace.getDiff{format:rows}` 的逐文件结构化 diff，带 `rows`）/ `FileEdit`
> （agent 一次编辑的应用结果，工具 `result` 约定 §4.4.2，无 `untracked`）。共用过去式 `status`（描述"变更后的态"），消除旧版
> `added` vs `add` 的无谓分叉。`added`/`removed`（±行）binary 时省略（不伪造 0），`previousPath` 仅 `renamed` 给。
> **`Diff` 是 sum-type**（`files` ⊕ `patch`，按 `format`），不是松对象同时带两者。
> **本地搜索命中（`SearchHit`：文件 + 行）与网络检索结果（`WebSearchResult`：url + 标题）是两个互斥类型**，不合并成一个松形状
> （避免"一个结果同时带 path 和 url"这种非法可表达态）。
> `FileLine.text` / `DiffRow.code` / `GrepMatch.text` 是**纯文本**（不含 server 端 HTML）；高亮由客户端做。

### 4.6 Usage / Error

```ts
interface ModelUsage {
  // —— 包含式总量（provider 直接报的数；各含其子项）——
  inputTokens?: number; // 总输入（含 cacheRead 部分）
  outputTokens?: number; // 总输出（含 reasoning 部分）
  // —— 互不重叠的子项（每项独立标注，客户端不必相减，杜绝下溢）——
  cacheReadTokens?: number; // inputTokens 里命中缓存的子集
  cacheWriteTokens?: number; // 写入缓存的部分
  reasoningTokens?: number; // outputTokens 里隐藏推理的子集
  costUsd?: number; // 顶层 Usage=总成本；byModel 条目=该 model 成本。模型不在定价表则省略，不臆造 0
}
interface Usage extends ModelUsage {
  byModel?: Record<string, ModelUsage>; // 子 agent 子树可跨多模型；per-model 拆分（不递归）。条目与顶层同形（含 cache）
}

interface ProblemData {
  // 用于 RPCError.data、RunResult.error、toolCall.error
  type: string; // 稳定符号名（判错用，§8.3）；插件用命名空间（§8.4）
  channel?: "rpc" | "run" | "tool"; // 自描述：本错误从哪条通道来（§8.1）——免客户端反推
  detail?: string; // 本次发生的人读说明（per-occurrence）
  docUrl?: string; // 可选：指向该 type 的文档（降对接门槛；缺省时按 §8.2 符号名查表即可）
  retryable?: boolean;
  retryAfterSeconds?: number; // 可重试时的最早重试时机（如 provider 限流回传的退避）
  errors?: FieldError[]; // 字段级错误（invalid_params / 表单校验，按 field 寻址）
  [key: string]: unknown; // 仍可附加 type-specific 扩展成员
}
interface FieldError {
  field: string;
  detail: string;
} // field = 出错字段名（params 里的 key）
```

### 4.7 上下文 / 工具规格

```ts
type ContextItem =
  | { type: "file"; path: string } // 相对 Session.cwd
  | { type: "selection"; path: string; range: [number, number] } // 1-based 闭区间
  | { type: "url"; url: string }; // Runtime 主动 fetch
// 图片不是 context item —— 直接作为 input 里的 image ContentBlock（mime + base64 data）内联传入

interface ToolSpec {
  name: string;
  description?: string;
  parameters?: Record<string, unknown>; // JSON Schema
  safetyClass?: string; // 开放枚举（§2.3）："safe" | "write" | "exec" | "network" …
}
interface GenerationParams {
  temperature?: number;
  maxTokens?: number;
  topP?: number;
  stop?: string[];
}
```

**`ContextItem` 安全不变量（定死，不许蒸发）**：

- `file.path` / `selection.path` **相对 `Session.cwd`**；逃出 cwd（`../` 越界）必须拒绝并返 `path_outside_root`。
- `selection.range` = `[startLine, endLine]`，1-based 闭区间。
- `url`：Runtime 会主动 fetch，**有 SSRF 面** —— 必须拦 loopback / 私网 / 元数据地址（`169.254.*` 等），除非宿主策略显式放行。

### 4.8 HITL 类型

```ts
// 三类 interrupt 一致"payload 足以渲染"，无一需要二次请求（S1：question 也自包含）。
// approval / toolResult 复用 ToolInvocation（§4.4）——客户端读 payload.tool 即可（name + arguments 恒在）。
type Interrupt =
  | { type: "approval"; itemId: string; payload: ApprovalPayload }
  | { type: "question"; itemId: string; payload: { question: Question } } // 自包含；无需 join items.list
  | { type: "toolResult"; itemId: string; payload: ToolResultPayload };

interface ApprovalPayload {
  tool: ToolInvocation; // 待批工具（此时 result 尚无；name+arguments 在）
  risk?: "low" | "medium" | "high"; // 由门禁按工具安全类派生（write→medium / exec→high）；客户端无需 join tools.list
  reason?: string; // 为何需要批准（一行）
}
interface ToolResultPayload {
  tool: ToolInvocation; // 要客户端执行的工具（client-side tools）；结果经 runs.resume 回传
}
interface OpenInterrupt {
  runId: string; // 待 resume 的 Run（稳定 runId；其当前段以 outcome.type=interrupt 收尾）
  sessionId: string;
  interrupts: Interrupt[];
  createdAt: string;
}
```

> **为什么 question 自包含**：`OpenInterrupt` 的语义是"什么在等我处理"——它本就该是个**自足的待办快照**。让 question
> 也带 `payload.question`，三类一致、去掉对 `items.list` 的二次 join，也根除"进程重启后 `running` 态的 question 还没进
> durable 历史、join 落空、待答问题渲染不出来"的坑。interrupt 上的 question 与（completed 后的）question Item 上各存一份
> 不算虚假 DRY：前者是待办快照、后者是历史，生命周期不同。

### 4.9 Provider / Model

```ts
interface Provider {
  id: string; // 既是身份也是类型标识（如 "groq" / "openai-compatible"）
  baseUrl?: string;
  apiKeyMasked: string; // "" = 未配置；如 "sk-…fc78"；永不可逆推（短 key 全打码）
  keySource?: "stored" | "env"; // key 来源：stored=providers.configure 设置（可编辑/持久化）；
  // env=从该 provider 的环境变量读取（只读，UI 显示「from env」）。
  // 未配置时省略（apiKeyMasked 同为 ""）。stored > env。
  requiresBaseUrl?: boolean; // 无内建 endpoint：通用兼容 passthrough + Azure。配置时必须收集 baseUrl，
  // 且因无 catalog，model 走自由输入（models.list 返回空）
}
interface Model {
  id: string;
  provider: string; // Provider.id
  displayName?: string;
  contextWindow?: number;
  maxInputTokens?: number;
  maxOutputTokens?: number;
  knowledgeCutoff?: string; // 训练知识截止（YYYY-MM-DD），未知时省略
  deprecated?: boolean; // provider 已下线该模型；client 隐藏或标记
  capabilities?: ModelCapabilities;
  pricing?: ModelPricing;
}
// 媒体类型（输入/输出模态），与 core chat.Modality 同值。开放枚举。
type Modality = "text" | "image" | "audio" | "video" | "pdf";
interface ModelCapabilities {
  reasoning?: boolean; // 是否支持扩展思考
  reasoningLevels?: string[]; // 离散思考档位（升序，如 ["low","medium","high"]）；预算式/不支持时为空
  reasoningDefaultLevel?: string; // 不指定档位时的默认；无档位时为空
  multimodal?: boolean; // 便捷位：是否接受图片输入（完整集见 inputModalities）
  inputModalities?: Modality[]; // 接受的全部媒体类型（text 在前，再到 image/pdf/audio/…）
  outputModalities?: Modality[]; // 产出的媒体类型（chat 模型为 text）
  toolUse?: boolean; // 工具/函数调用
  structuredOutput?: boolean; // 原生 structured-output / JSON-schema
}
interface ModelPricing {
  // 主费率档；provider 不单列缓存价时缓存字段为 0；阈值分档的长上下文价仅给 base 档
  inputUsdPerMillionTokens?: number;
  outputUsdPerMillionTokens?: number;
  cacheReadUsdPerMillionTokens?: number;
  cacheWriteUsdPerMillionTokens?: number;
}
```

### 4.10 Workspace 周边 / 可选域类型

```ts
interface Skill {
  name: string;
  description?: string;
  source?: string;
}
interface AgentDoc {
  path: string;
  title?: string;
  scope: "cwd" | "projectRoot" | "home";
}
// status: 5 态（AUX_API §5.1）。toolCount 内联，省去 listServers⨝listTools。
// error 仅 status:"failed" 时给出（dial 失败原因，type:"mcp_dial_failed"）。
// authStatus 省略 = 未跟踪（v1 不产 needsAuth：dial 暂不暴露可区分鉴权错）。
interface McpServer {
  name: string;
  status: "connecting" | "connected" | "disconnected" | "failed" | "needsAuth";
  toolCount?: number;
  authStatus?: "none" | "bearerToken" | "oauth" | "notLoggedIn";
  error?: ProblemData;
  description?: string;
}
interface McpTool {
  server: string;
  name: string;
  description?: string;
  inputSchema?: Record<string, unknown>;
}

interface MemoryEntry {
  scope: "cwd" | "projectRoot" | "home";
  path: string;
  content: string;
  updatedAt?: string;
}
```

### 4.11 分页（所有 list 统一）

```ts
interface Page<T> {
  data: T[];
  nextCursor?: string;
} // cursor 不透明
```

- **所有 list 方法一律返回 `Page<T>`**（§7 标注），客户端一个读法（`resp.data` + 可选 `resp.nextCursor`）。
- **`nextCursor` 的存在性即 "还有更多"信号**（等价于 Stripe 的 `has_more`）：缺省 = 已到末页；存在 = 还有更多，client 带
  `cursor` 续拉。**server 不得静默截断**——超过单页上限必须回填 `nextCursor`（"no silent caps"）。本地有界 list
  （如 `tools.list`）`nextCursor` 恒空，但形状一致、无破坏性扩展路径。
- **cursor 不透明**：client 不解析其内容、不据其推断顺序。server 可用任意编码（"上一页最后一项的 id"、编码偏移等），
  client 只回传上次拿到的 `nextCursor`。

---

## 5. 流式

所有 run 事件走**一个**通知方法 `notifications.run.event`，params 为 `RunEvent`：

```ts
interface RunEvent {
  runId: string; // 稳定逻辑 Run（§0.3）
  segmentId: string; // seg_…；事件所属的流式段——client 按它 key 流树 + 重连回放去重
  eventId: string; // evt_…；单 Segment 流内单调（§2.4）
  timestamp: string; // ISO-8601
  event: StreamEvent;
}

type StreamEvent =
  | { type: "segment.started"; run: RunRef }
  | { type: "segment.progress"; progress: RunProgress } // mid-run 进度预览（ephemeral）
  | { type: "segment.finished"; outcome: RunOutcome } // 标记当前 Segment 结束；outcome=interrupt 即停车待续（§0.3）
  | { type: "item.started"; item: Item } // item 壳（status=running）
  | { type: "item.delta"; itemId: string; delta: ItemDelta }
  | { type: "item.completed"; item: Item } // 权威终态（durable）
  | { type: "state.snapshot"; state: Record<string, unknown> } // 第三方顶层 key 遵 §2.6
  | { type: "state.delta"; patch: JsonPatch }
  | { type: "custom"; name: string; durable?: boolean; payload: unknown }; // 第三方 name 遵 §2.6

interface RunProgress {
  step?: number; // 已走的 agent 步数
  maxSteps?: number; // 上限（run 起时设了才有）
  usage?: Usage; // 至此累计用量（成本读 usage.costUsd）
  activity?: string; // 人读的当前动作（"calling tool: shell"）
}

type JsonPatch = Array<{
  op: "add" | "remove" | "replace" | "move" | "copy" | "test";
  path: string;
  value?: unknown;
  from?: string;
}>;
```

### 5.1 ItemDelta

```ts
type ItemDelta =
  | { type: "content"; index?: number; text: string } // agentMessage 文本增量
  | { type: "reasoning"; text: string } // 推理文本增量
  | { type: "toolArguments"; argumentsTextDelta: string } // 工具入参的部分 JSON 文本增量
  | { type: "toolOutput"; text: string } // 工具输出（如命令 stdout）增量
  | { type: "plan"; steps: PlanStep[] }; // 计划当前全量（非高频字符流）
```

> **`toolArguments` 是文本增量**：流式工具入参是逐片到达的不完整 JSON 文本，无法在中途构成合法对象；客户端累积
> `argumentsTextDelta`、用 `untruncate-json` 一类修复成可解析片段做预览；**已解析结构化字段只在 completed `toolCall`
> item 的 `tool.arguments` 落定**（§4.4）。
>
> **`toolArguments` / `toolOutput` 都是预览通道**：二者皆 ephemeral，其权威终值都在 completed item 上
> （`toolArguments`→`tool.arguments`，`toolOutput`→`tool.result.output`）。见 §5.2。

### 5.2 Durable / Ephemeral 不变量（协议级保证）

> **丢弃每一个 ephemeral 事件，客户端仍必然得到正确终态。**

**`durable` 不再每帧冗余携带**（S4）。对所有 first-party 事件，"是否 durable"是 `event.type` 的**纯函数**，由下表推导；
冗余的 per-frame bool 会引入"`item.completed` 却 `durable:false`"这种自相矛盾的可表达非法态，故移除。唯一**不能**从
type 推导的是 `custom`——它由产出方在帧上自带 `durable?`（缺省 `false`）。

**事件 durable 推导表（权威）**：

| event.type                  | durable                       | 权威落点（该类 ephemeral 的终值在哪）                                                |
| --------------------------- | ----------------------------- | ------------------------------------------------------------------------------------ |
| `segment.started`               | ✅ true                       | ——                                                                                   |
| `segment.finished`              | ✅ true                       | ——                                                                                   |
| `item.started`              | ✅ true                       | ——                                                                                   |
| `item.completed`            | ✅ true                       | ——                                                                                   |
| `state.snapshot`            | ✅ true                       | ——                                                                                   |
| `segment.progress`              | ⬜ false                      | `segment.finished.result`（`usage`〔含 `costUsd`〕/ `steps`）                            |
| `item.delta{content}`       | ⬜ false                      | `agentMessage.content`（completed）                                                  |
| `item.delta{reasoning}`     | ⬜ false                      | `reasoning.text`（completed）                                                        |
| `item.delta{toolArguments}` | ⬜ false                      | `tool.arguments`（completed，§4.4）                                                  |
| `item.delta{toolOutput}`    | ⬜ false                      | `tool.result.output`（completed——仅累积 result 的 output 键，非整个 result，§4.4.2） |
| `item.delta{plan}`          | ⬜ false                      | `plan.steps`（completed）                                                            |
| `state.delta`               | ⬜ false                      | `state.snapshot`（§5.3 必发边界）                                                    |
| `custom`                    | 帧上 `durable?`（默认 false） | 由产出方保证（durable custom 须有自己的可恢复落点）                                  |

**硬规则**：每个 ephemeral 事件**必须**在某个 durable 落点上有命名终值。新增一个无落点的 ephemeral 事件 / delta 类型 =
**协议违规**（它会在历史回放 / 重连 / opt-out / 后端整发时丢内容，§10）。

推论：客户端可 opt out 高频 delta（§9 `optOutNotificationMethods`）而仍保持正确；不流式的 runtime 可不发任何 delta，
completed item 一样必发权威终值。

### 5.3 state.snapshot 必发边界

当 run 期间创建或改变了共享状态时，server **必须**在以下边界发 `state.snapshot`：

- `segment.finished` 之前；
- 重连后，若客户端可能漏过 `state.delta`；
- 在尊重某个 `state.delta` opt-out 之后。

无共享状态时 `state.snapshot` 可省略。

**first-party 共享 key**：`todos` —— 模型的任务清单（`todo_write` 工具全量替换后投影），值是 `TodoSnapshot[]`（`{ id, text, status: "pending"|"in_progress"|"completed" }`，id 为位置序、随整表替换而非持久身份）。客户端读 `shared.todos` 渲染任务面板，无需 join 工具结果（`todo_write` 结果本身仅面向模型）。第三方 key 遵 §2.6 命名空间。

### 5.4 Run 树

一条 root run 的**一段流**包含**整棵 run 树**的事件（根 + 所有子孙 subagent run）：

- 每条事件带它所属的 `runId`（稳定）+ `segmentId`（所属段）；
- 子 run 以 `segment.started` 携带 `spawnedByItemId` 开始，有**自己的** `runId` 与 `segmentId`；
- 客户端用 `runId` + `spawnedByItemId` join 还原树，用 `segmentId` key 流；
- 对 root run `runs.subscribe` = 订阅其当前活跃段的整棵树；
- `features.subagents=false` 时不产出任何子 run 事件。

---

## 6. Human-in-the-Loop（R 模型）

流程：

1. agent 需要人介入 → 产出对应 Item（`toolCall` 待批 / `question` 待答 / client 工具调用）。
2. 当前 **Segment 结束**：`segment.finished{ outcome:{ type:"interrupt", interrupts:[…] } }`，资源全释放。**Run 不结束**——
   稳定 `runId` 挂起待续。每个 `Interrupt.itemId` 指向那个待解 Item。
3. 待解项作 `OpenInterrupt` **持久化**（跨重启可经 `runs.listOpenInterrupts` 发现）。
4. 客户端调 `runs.resume{ runId, responses:[…] }` 在**同一 Run 上开新的一段**应答（返回同 `runId` + 新 `segmentId`）。
5. 续段像普通流一样，直到下一个 outcome。

**关联键 = `itemId`**（无单独 requestId）。**拒绝 ≠ 取消**：拒绝是 `runs.resume` 带 `decision:"deny"`，run 继续
（agent 据理由换方案）；取消是 `runs.cancel`，硬终止整个 run。

### 6.1 InterruptResponse

```ts
interface InterruptResponse {
  itemId: string; // 对应 Interrupt.itemId
  response: ApprovalResponse | AnswerResponse | ToolResultResponse;
}
interface ApprovalResponse {
  type: "approval";
  decision: "approve" | "deny";
  remember?: { scope: "session" | "project" | "global" }; // 记住这个决策（approve 或 deny），匹配的后续调用免提示（AUX_API §6）
  editedArgs?: Record<string, unknown>; // 批准前一次性改写工具入参（不进 remember）
  reason?: string;
}
interface AnswerResponse {
  type: "answer";
  answers: Record<string, string[]>;
} // key = QuestionField.name；单选=单元素数组（S8）
interface ToolResultResponse {
  type: "toolResult";
  result?: unknown;
  error?: ProblemData;
} // client 工具结果，与 tool.result 同形（best-effort JSON）；失败给 error
```

> `answers` 值一律 `string[]`（单选也是单元素数组）——消费端形状统一、不用每次判 `string | string[]`（S8）。

> **`remember`（审批 scope，AUX_API §6）**：持久化成一条**细粒度规则**（`ApprovalRule`，§C.3）。规则按 `(scope, tool, subject)`
> 命中：`subject` 是后端按工具从被批准调用里提取的子主题（shell 的 command / 文件工具的 file_path），所以记的是
> 「`npm run *` 在本 project」而非笼统「整个 shell」。`decision:"deny" + remember` 合法 = 记住拒绝。`editedArgs` 仍是
> **一次性**的（不折进规则）。三个 `scope` **全部持久**（SQLite）：`session` 键到会话、`project` 键到会话 cwd、`global`
> 处处生效；最具体的命中胜出（session > project > global，再 exact > glob > 任意），同特异度冲突取 deny。

### 6.2 防挂死（协议级硬约束）

client 在 `ClientCapabilities.interruptTypes` 声明能处理的 `Interrupt.type`。server **必须不产出 client 未声明 type
的 open interrupt** —— 否则会留下一个**永远 `runs.resume` 不了的持久 open interrupt**（比挂起 run 更糟）。不支持时
server 走非阻塞默认策略（auto-deny / 不进该模式）。`toolResult` interrupt 额外受 `features.clientTools` 门控。

---

## 7. 方法参考

> 约定：**Stream<R>** = 返回 `R`（含 `runId`）+ 随后流式 `notifications.run.event` 的 `RunEvent`。这是传输无关
> 语义；HTTP 上 `R` 作首帧、与后续 `RunEvent` 同走该 POST 的 `text/event-stream` 响应（streamable HTTP，见
> TRANSPORT §6.4），InProcess/IPC 则返回值 + 通知回调。**流式方法集机器可读于 `ServerCapabilities.streamingMethods`**
> （§9）——客户端据此预知该走哪种响应分支，不硬编码方法名。
> 每个方法的「错误」列的是**可能返回的 `ProblemData.type`**（按 name 判，§8）；通用错误（`invalid_params` /
> `internal_error` / `capability_not_negotiated`）不逐一重列。

### 7.1 runtime.*

#### `runtime.discover`

读取 runtime 信息。它是无状态查询，不是生命周期切换；业务方法不要求先调用它。

- 入参：无。
- 返回 `DiscoverResponse`：

  | 字段              | 类型                           | 说明                                        |
  | ----------------- | ------------------------------ | ------------------------------------------- |
  | `protocolVersion` | string                         | server 实现的协议版本                       |
  | `serverInfo`      | `{ name; version; cwd; home }` | serve 进程目录上下文（冷启动默认 cwd 兜底） |
  | `capabilities`    | `ServerCapabilities`（§9）     |                                             |

#### `runtime.shutdown`（通知，无响应）

- 入参 `{ reason?: string }`。runtime 停接新工作、按宿主策略结束/取消进行中的 run、关闭 transport。

#### `runtime.ping`

- 入参：无；返回：无。**仅 InProcess / IPC**；HTTP 探活走 sidecar `GET /v2/health`（见 TRANSPORT）。

#### `notifications.canceled`（client→server 通知）

- 入参 `{ id: string; reason?: string }` —— `id` 是被取消的在飞 Request 的 envelope id。取消一个慢请求，不取消 run。

---

### 7.2 sessions.*

#### `sessions.list`

- 入参 `{ cursor?: string; limit?: number }`；返回 `Page<Session>`。

#### `sessions.get`

- 入参 `{ sessionId: string }`；返回 `Session`；错误 `session_not_found`。

#### `sessions.create`

- 入参 `CreateSessionRequest`：

  | 字段       | 类型   | 必填 | 说明                                                    |
  | ---------- | ------ | ---- | ------------------------------------------------------- |
  | `cwd`      | string | 否   | 缺省 = `ServerInfo.cwd`（冷启动零摩擦，不弹强制选择框） |
  | `title`    | string | 否   |                                                         |
  | `model`    | string | 否   | 默认模型                                                |
  | `metadata` | object | 否   |                                                         |

- 返回 `Session`（`cwd` 为 server 规范化后的绝对路径）。错误 `cwd_unavailable`。

#### `sessions.update`

- 入参 `UpdateSessionRequest`：

  | 字段        | 类型   | 必填 | 说明                                               |
  | ----------- | ------ | ---- | -------------------------------------------------- |
  | `sessionId` | string | 是   |                                                    |
  | `title`     | string | 否   |                                                    |
  | `cwd`       | string | 否   | **改 cwd = relocate**；受 `features.relocate` 门控 |
  | `model`     | string | 否   |                                                    |
  | `metadata`  | object | 否   | 全替换                                             |
  | `favorite`  | bool   | 否   | 置顶/取消置顶；favorited 会话在列表里排在前面      |

- 返回 `Session`。错误 `session_not_found` / `capability_not_negotiated`（relocate 关闭时改 cwd）/ `cwd_unavailable`。

#### `sessions.delete`

- 入参 `{ sessionId: string }`；返回无。

#### `sessions.fork`

- 入参 `{ sessionId: string; fromRunId?: string; title?: string }`；返回新 `Session`（继承源 cwd）。
- 省略 `fromRunId` = 整段 fork（复制全部历史）；给定 = **含 `fromRunId` 在内**截断复制到该 run 边界（AUX_API §4.2）。
- **快照语义、随时可调**：只复制**已完结**的 run（in-flight run 不进副本），故 fork 删不动任何东西、无 `session_busy`。
- 错误 `session_not_found` / `run_not_found`。

#### `sessions.rollback` **（turn 粒度回退，AUX_API §4.1）**

- 入参 `{ sessionId: string; toRunId?: string; restoreType?: "history" | "files" | "both" }`；返回 `{ session: Session; droppedRuns: DroppedRun[] }`。
- **`toRunId` = inclusive-keep**：保留的**最后一个 root run**（含其停车-续跑的全部段），其后**全部丢弃**。省略 `toRunId` = 丢弃全部、回到空会话（覆盖「编辑第一条消息重跑」）。
- `toRunId` **必须是 root run**（子 agent run → `invalid_params`）；未知 → `run_not_found`。
- **就地销毁**：截断聊天历史、删被丢 run 的 Item/记录、清其悬挂 open interrupt、并**递归 purge 被丢 run 派生的 subagent 子会话整棵子树**。
- **运行中拒绝**：session 有 run 在跑 → `session_busy`（避免与在 append 的历史竞争）。
- **`restoreType`（默认 `history`，AUX_API §4.1，门控 `features.checkpoints`）**：
  - `history` → 只回退聊天历史（不动文件，老行为）。
  - `files` → 只把工作区**文件**还原到 `toRunId` 的影子-git 快照（历史不动）。
  - `both` → 二者，**原子**：files 先行,失败 → 整体失败、history 不动、返 `checkpoint_unavailable`，**绝不静默降级**。
  - `files`/`both` **必须带 `toRunId`**（否则 `invalid_params`）；该 run 无快照 → `checkpoint_unavailable`。还原前自动快照当前态(可 unrevert)。
  - `history`（默认）下不动文件：UI 用 `workspace.getDiff` 自查未还原改动。
- `DroppedRun.userInput` 是被丢 run 的开场 userMessage `content`（与 `StartRunRequest.input` 同型，composer 零转换预填）；子 agent run 无开场轮 → 省略。
- 错误 `session_not_found` / `run_not_found` / `invalid_params` / `session_busy`。

```ts
interface DroppedRun {
  run: RunRef;
  userInput?: ContentBlock[];
}
```

#### `sessions.export`

- 入参 `{ sessionId: string; format?: "md" | "json" }`（缺省 `json`）；返回 `{ format; artifact?: SessionArtifact; markdown?: string }`。
- **内联返回**（后端是本地 loopback 运行时，无长会话巨型 payload 顾虑，故不走 out-of-band 文件通道）：
  - `format:"json"` → `artifact`：可 round-trip 的会话包，喂给 `sessions.import` 原样恢复。
  - `format:"md"` → `markdown`：人读转写文本（**不可**再导入）。
- 受 `features.sessionExport` 门控。

```ts
interface SessionArtifact {
  version: number; // artifact schema 版本（当前 2）；import 不识别即 invalid_params
  session: Session; // 会话元数据（wire 形态）
  messages: unknown[]; // chat 消息 blob（模型上下文）
  runs: {
    runId: string;
    updatedAt: string;
    messageMark: number;
    run: RunRef;
  }[]; // messageMark：rollback/fork 边界水位（-1=未知）
  items: { runId: string; itemId: string; createdAt: string; item: Item }[];
}
```

#### `sessions.import`

- 入参 `{ artifact: SessionArtifact }`；返回 `{ session: Session }`。
- **restore 语义**：在 artifact **原 id** 下重建会话（已存在则覆盖），并替换其历史后重灌 messages + runs + items。
  幂等：重复导入同一 artifact 不产生副本（按 id UPSERT）。受 `features.sessionExport` 门控。
- 错误 `invalid_params`（缺 `artifact.session.id` / 版本不识别 / 消息 blob 损坏）。

---

### 7.3 runs.*

#### `runs.start` — Stream

起一个新 Run 并打开事件流。

- 入参 `StartRunRequest`：

  | 字段           | 类型               | 必填 | 说明                                                                         |
  | -------------- | ------------------ | ---- | ---------------------------------------------------------------------------- |
  | `sessionId`    | string             | 是   | cwd 从 session 解析；本请求**不带 cwd**、**不带 runId**                      |
  | `input`        | `ContentBlock[]`   | 是   | user 消息正文；图片作为 image block 内联（mime + base64 data）               |
  | `context`      | `ContextItem[]`    | 否   | 附带上下文；`file.path` 相对 session.cwd（§4.7 安全规则）                    |
  | `tools`        | `ToolSpec[]`       | 否   | 覆盖该 run 的默认工具集                                                      |
  | `state`        | object             | 否   | 初始共享状态                                                                 |
  | `provider`     | string             | 否¹  | 选用的 provider（`Provider.id`，来自 `providers.list`）。与 `model` **配对** |
  | `model`        | string             | 否¹  | 选用的 model（`Model.id`，来自 `models.list`）。与 `provider` **配对**       |
  | `maxSteps`     | number             | 否   | 触顶 → `outcome.maxSteps`                                                    |
  | `maxBudgetUsd` | number             | 否   | 含子 agent 子树；触顶 → `outcome.maxBudget`                                  |
  | `params`       | `GenerationParams` | 否   |                                                                              |

  ¹ **`provider` + `model` 配对、显式**：要么**都给**，要么**都不给**（用服务端默认）。**只给其一 → `invalid_params`**。
  provider **不从 model 反推**（同名 model 可能跨多 provider；自定义 model 可能不在 catalog）。所选 provider 未配置 key
  （`apiKeyMasked == ""`）时，run 以 `segment.finished{outcome:error}`（"set its API key first"）收尾。

  > **没有 `mode` 字段**：agent/chat/plan 这套 per-run 模式已移除。run 永远是带工具的 agent 循环。
  > "计划"通过**全局审批姿态**实现：先 `approval.setMode{"plan"}`（§C.3，只读门禁——写/exec/network 工具一律拒），
  > agent 调研完调 `exit_plan_mode` 工具把计划作为一个 `question` interrupt 抛出（§6）；用户批了，姿态自动翻回
  > `balanced` 执行，拒了留在 `plan`。"chat"（无工具单轮）没有替代——run 总是带工具的。

- 返回 `{ runId: string; segmentId: string; userItemId?: string }`；随后流式 `RunEvent`（§5），以 `segment.finished{outcome}`
  收尾（**该段**的结束）。
  - `userItemId`：本 run 开场 `userMessage` Item 的 id —— 与流上 `item.*` 及 `items.list` 里同一个 id。客户端用它把
    乐观气泡按精确 id 对账（不按内容文本启发式匹配）。`runs.resume` 无开场 user 回合，故为空。
- 幂等：**尚未实现**（`X-Idempotency-Key` 头当前未被读取；`idempotency_conflict`/`-32015` 为预留码、暂不产生）。
  目前重复键不会重连既有 run —— 原 run 还在跑则新请求得 `session_busy`（下条），已结束则会另起一个新 run。
  设计意图是「重复键重连既有 run」，待实现；当前真正防重的是下面的「一会话一 run」守卫。
- **一会话一 run（`session_busy`）**：该 session 已有 run 在跑（**正在 pump 或 parked 在 HITL interrupt 上**）时拒绝——
  两个 run 同时写一条会话的消息日志会被 turn 末压缩（整段重写）吞掉彼此的消息。继续既有 run 用 `runs.resume`、
  插话用 `runs.steer`,而非再开一个 `runs.start`。（注:开 run 的「检查—注册」非单一临界区,多客户端同瞬并发 start 仍有
  极窄 TOCTOU；单客户端被 `starting` 闩串行化,不触发。）
- 错误（通道 a，§8.1）：`session_not_found` / `cwd_unavailable` / `invalid_params`（含 `provider`/`model` 只给其一）/ `session_busy`。
  执行期错误（通道 b）：`segment.finished{outcome:error}` / 工具级 `toolCall.error`。

- **示例**：

  请求

  ```json
  {
    "jsonrpc": "2.0",
    "id": "1",
    "method": "runs.start",
    "params": {
      "sessionId": "ses_01",
      "input": [{ "type": "text", "text": "列出当前目录" }]
    }
  }
  ```

  响应（HTTP 上是流的首帧）

  ```json
  {
    "jsonrpc": "2.0",
    "id": "1",
    "result": { "runId": "run_01", "segmentId": "seg_01", "userItemId": "item_00" }
  }
  ```

  随后事件（节选）

  ```json
  { "jsonrpc":"2.0", "method":"notifications.run.event",
    "params": { "runId":"run_01", "segmentId":"seg_01", "eventId":"evt_0001", "timestamp":"2026-06-07T10:00:00Z",
                "event": { "type":"segment.started", "run": { "id":"run_01", "sessionId":"ses_01" } } } }
  { "jsonrpc":"2.0", "method":"notifications.run.event",
    "params": { "runId":"run_01", "segmentId":"seg_01", "eventId":"evt_0008", "timestamp":"2026-06-07T10:00:02Z",
                "event": { "type":"segment.finished",
                           "outcome": { "type":"completed", "result": { "usage": { "inputTokens": 1200, "outputTokens": 80 }, "steps": 2 } } } } }
  ```

#### `runs.resume` — Stream

应答 open interrupt，在同一 Run 上开新的一段。

- 入参 `ResumeRunRequest`：

  | 字段        | 类型                          | 必填 | 说明                                                                                                   |
  | ----------- | ----------------------------- | ---- | ------------------------------------------------------------------------------------------------------ |
  | `runId`     | string                        | 是   | 待续的 Run（稳定 `runId`；其当前段以 outcome.type=interrupt 收尾）                                     |
  | `responses` | `InterruptResponse[]`（§6.1） | 是   | 按 itemId 寻址。**必须恰好覆盖该 Run 的全部 open interrupt**——缺漏或含未知/已 resolve 的 itemId → 报错 |

- 返回 `{ runId: string; segmentId: string }`（**同** `runId` + 新一段的 `segmentId`）+ 流式 `RunEvent`。
- 错误：`run_not_found` / `interrupt_not_open`（含未知 / 已 resolve 的 itemId）/ `invalid_params`（`responses` 未覆盖全部 open interrupt）。

- **示例**（批准一个 toolCall interrupt）：
  ```json
  {
    "jsonrpc": "2.0",
    "id": "7",
    "method": "runs.resume",
    "params": {
      "runId": "run_01",
      "responses": [
        {
          "itemId": "item_09",
          "response": { "type": "approval", "decision": "approve" }
        }
      ]
    }
  }
  ```

#### `runs.subscribe` — Stream

为一个已存在的 run（root）打开一条新的事件流（重连/崩溃恢复）。

- 入参 `{ runId: string }`（稳定 Run 身份，重连到其**当前活跃段**）；返回 `{ runId; segmentId }` + 流式 `RunEvent`（订阅整棵 run 树，§5.4）。带 `Last-Event-Id` 从断点续传 durable 事件（HTTP 见 TRANSPORT §9.2）。
- 错误：`run_not_found`。

#### `runs.cancel`

硬终止一个在跑的 run。

- 入参 `{ runId: string; reason?: string }`；返回无。run 以 `outcome:canceled` 收尾（`reason` 回流到 `outcome.detail`，§4.2）。
  错误：`run_not_found`（不存在）/ `run_already_finished`（已收尾——run 二态 running|finished，interrupt outcome 亦属 finished）。

#### `runs.steer`

把一条用户消息**注入正在跑的 run**，模型在**下一个工具轮**读到它（mid-run steering，§6）—— 区别于 `runs.resume`（应答 interrupt）和 `runs.start`（开新回合）。引擎在每个延续轮的请求里把该消息追加在最新工具结果之后（memory 中间件随即持久化进历史），所以它落在正确位置、且后续轮 + 下一回合都可见。

- 入参 `{ runId: string; message: string }`；返回无。
- 仅对**正在跑**（actively pumping）的 run 有效：驻留中（等 interrupt，应走 `runs.resume`）或已结束的 run 报 `run_not_found`。
- 时机是 best-effort：若 run 已无后续工具轮（正出最终答复），该消息落到下一回合（与既有 next-turn steering 回退一致），不丢。

#### `runs.list`

- 入参 `{ sessionId?: string; cursor?: string; limit?: number }`；返回 `Page<RunRef>` —— **仅在跑的 run**。已结束/被中断的 run 经 `listOpenInterrupts` 或 item 历史发现。

#### `runs.listOpenInterrupts`

- 入参 `{ sessionId?: string; cursor?: string; limit?: number }`；返回 `Page<OpenInterrupt>`（§4.8）。durable HITL 发现：重启后据此 `runs.resume`。

---

### 7.4 items.*

#### `items.list`

会话历史 = completed Item 序列 + 还原 run 树所需的 RunRef。

- 入参 `{ sessionId: string; cursor?: string; limit?: number }`。
- 返回 `ListItemsResponse = Page<Item> & { runs: RunRef[] }`（复用 §4.11 `Page<T>`，读 `resp.data`）：

  | 字段         | 类型       | 说明                                                                                                  |
  | ------------ | ---------- | ----------------------------------------------------------------------------------------------------- |
  | `data`       | `Item[]`   | 本页 item（`Page<Item>.data`）                                                                        |
  | `nextCursor` | string?    | 分页游标；存在即还有更多页（**不静默截断**，§4.11）                                                   |
  | `runs`       | `RunRef[]` | 这些 item 所属 Run（含已完成/在跑），带 `spawnedByItemId`，供 join 还原 Run 树（§10.3） |

- 错误：`session_not_found`。

> **已移除 `items.edit`**（AUX_API §7）：turn 粒度下「编辑某条重跑」= `sessions.rollback{toRunId}` + `runs.start`，
> item 精确编辑无独立存在理由。`features.checkpoints` 的语义已改写为 v2 影子 git + `restoreType`（见 §9）。

---

### 7.5 workspace.*

所有读方法显式收 `cwd?`（缺省 = serve 目录）。入参基类 `WorkspaceQuery { cwd?: string }`。**返回列表的方法统一 `Page<T>`**。

> 提案中的 workspace 面（`workspace.code.*` / `mcp.authenticate`，613 批次）见**附录 C**——
> 形状已定、后端方法表待注册，落地后并入本表。（`workspace.listFiles` / `workspace.readFile` 已落地——见附录 C.2 状态注。）

| 方法                           | 入参                                                                                    | 返回                                                               | 说明                                                                                                                                            |
| ------------------------------ | --------------------------------------------------------------------------------------- | ------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `workspace.listFileChanges`    | `WorkspaceQuery & { cursor?; limit? }`                                                  | `Page<WorkspaceFileChange>`                                        | 工作区改动（±行、rename、binary、untracked）；见 AUX_API §2.2                                                                                   |
| `workspace.getDiff`            | `WorkspaceQuery & { path?; mode?: "worktree"\|"base"; format?: "rows"\|"raw"; limit? }` | `Diff`                                                             | sum-type `{ files \| patch, truncated? }`；worktree(含 untracked)\|base(merge-base);超 `limit` 在**文件边界**截断置 `truncated`;见 AUX_API §2.3 |
| `workspace.getFileHead`        | `WorkspaceQuery & { path: string; lines?: number }`                                     | `FileHead`                                                         | 文件头若干行                                                                                                                                    |
| `workspace.grep`               | `WorkspaceQuery & { query: string; path?: string; limit?: number }`                     | `GrepResult`                                                       | `{ matches, total }`（total 反映截断）                                                                                                          |
| `workspace.listProjects`       | `{ cursor?; limit? }`                                                                   | `Page<Project>`                                                    | distinct-cwd 派生视图                                                                                                                           |
| `workspace.listSkills`         | `WorkspaceQuery & { cursor?; limit? }`                                                  | `Page<Skill>`                                                      |                                                                                                                                                 |
| `workspace.recipes.list`       | `WorkspaceQuery`                                                                        | `Page<Recipe>`                                                     | 该 cwd 发现的提示 recipe（全局 + 项目，项目同名胜）；客户端展开 body 后作为一个 turn 发送；见下                                                 |
| `workspace.listAgentDocs`      | `WorkspaceQuery & { cursor?; limit? }`                                                  | `Page<AgentDoc>`                                                   | 从 cwd 向上发现的 AGENTS.md                                                                                                                     |
| `workspace.mcp.listServers`    | `{ cursor?; limit? }`                                                                   | `Page<McpServer>`                                                  | MCP 全局，不收 cwd；含 boot 失败的 server（status:"failed" + error）                                                                            |
| `workspace.mcp.listTools`      | `{ server?: string; cursor?; limit? }`                                                  | `Page<McpTool>`                                                    |                                                                                                                                                 |
| `workspace.mcp.reconnect`      | `{ server: string }`                                                                    | 无（异步）                                                         | 结果走推送，见下                                                                                                                                |
| `workspace.hooks.list`         | `WorkspaceQuery`                                                                        | `HooksListResult`                                                  | 该 cwd 发现的生命周期 hook（全局 + 项目）+ 项目信任态；见下                                                                                     |
| `workspace.hooks.setTrust`     | `{ projectRoot: string; trusted: boolean }`                                             | 无                                                                 | 信任 / 撤销某项目的 hook（下一个 turn 生效）；见下                                                                                              |
| `workspace.subscribe` — Stream | `{ watches?: WatchSpec[] }`                                                             | Stream（`notifications.workspace.event`，params `WorkspaceEvent`） | 流式方法（在 `streamingMethods`）；`watches` 受 `features.fileWatch`                                                                            |

错误：读方法可返 `cwd_unavailable` / `path_outside_root`。git 方法(`listFileChanges` / `getDiff`)按**三态分离**:
无 git 二进制 → `features.git=false`(客户端不调);有 git、cwd 非仓 → `vcs_unavailable`;是仓、无改动 → 空结果。
`getDiff` 的 `mode:"base"` 解析不出基线分支 → `invalid_params`(不是 `vcs_unavailable`)。详见 AUX_API §2。

#### Workspace 事件流（`workspace.subscribe`，AUX_API §3 / §5）

`workspace.subscribe` 打开一条非-run 工作区事件流（生命周期 = 订阅；client 断开即结束）。全局事件（`mcp.serverChanged` /
`skills.changed`）发给每个订阅；带 `watches` 时,**监视该 cwd 的 git 状态**,任何变化发一个去抖 `resync`。

```ts
interface WatchSpec {
  watchId: string;
  cwd?: string;
  path?: string;
} // path 当前未用;cwd 默认 serve 目录
interface WorkspaceEvent {
  type:
    | "files.changed"
    | "skills.changed"
    | "mcp.serverChanged"
    | "schedules.fired"
    | "resync";
  paths?: string[];
  cwd?: string; // files.changed：变更文件(相对 cwd)+ 所属 cwd
  server?: string;
  status?: string;
  toolCount?: number;
  error?: ProblemData; // mcp.serverChanged
  scheduleId?: string; // schedules.fired：刚触发的 schedule(新 run 落在新会话,客户端刷新会话列表)
}
```

**watch 模型(重要,与早期"递归文件监听"不同)** —— 后端**不递归监视工作树**(macOS kqueue 每文件一个 fd,大树会耗尽
fd;且我们用不了 fd-廉价的 FSEvents)。改为两路覆盖,跨平台(inotify/kqueue/Win32 同样廉价):

- **`resync`** —— 带 `watches` 时,监视该 cwd 的 `.git` 信号集(HEAD/index/refs/heads/ORIG_HEAD/MERGE_HEAD)。git 状态一变
  (commit/暂存/checkout/branch/merge,任何进程所为)发去抖 `resync` → client 重拉 `workspace.getDiff`/`listFileChanges`。
  非 git 仓的 cwd → 该 watch 静默无效(getDiff 本身也会 `vcs_unavailable`)。
- **`files.changed{cwd, paths}`** —— **agent 自己的文件编辑**(write/edit 工具)由运行时从 run 流**精确推送**:工具一完成就发
  变更文件路径(相对 `cwd`)。无需 watch、无竞态。`shell` 的文件改动不发(参数无法判定;若是 git 操作则走上面的 `.git` 监视)。
  纯外部进程编辑(非 git、非 agent)不实时,降级到下次 git 操作 / 手动刷新(同 Claude Code 取舍)。
- 客户端用 `cwd` 区分 `files.changed` 属于哪个项目;`resync` 无 paths,语义是"该 cwd 重拉"。
- **`workspace.mcp.reconnect`** 无同步返回 —— 结果经 `mcp.serverChanged` 投递,**保证顺序 `connecting → (connected | failed)`**
  （client 按钮 loading 绑 `connecting`,终态解除）。重连成功热刷新工具集,模型即时可见新工具。`status` 省略 = 条目已不存在。

> `getDiff` / `getFileHead` / `grep` 返回的是**单一聚合结果**（非集合列表），故保留专用 shape、不套 `Page<T>`；但仍守
> "no silent caps"——截断都**自描述**：`grep` 由 `GrepResult.total ≥ matches.length`、`getDiff` 由 `Diff.truncated`。

#### 提示 recipes（`workspace.recipes.list`）

recipe 是**用户触发的参数化提示模板** —— skills（面向模型、渐进披露）的只读姊妹，但面向人。来自两层目录，**与 skills 同款级联**（两个 flat 目录，项目同名胜，非 hooks/AGENTS.md 的逐层级联）：全局 `~/.lyra/recipes/*.md` + 项目 `<cwd>/.lyra/recipes/*.md`。每个 `*.md` 文件 = 一个 recipe，文件名（去 `.md`）即 recipe 名与调用它的 slash 命令（`review.md` → `/review`）。

文件 = 可选 YAML frontmatter（`description` / `argumentHint`，两者皆可省）+ Markdown body（提示模板）。**展开在客户端做**：body 里的 `$ARGUMENTS`（全部尾随文本）/ `$1..$9`（按空白分词的位置参数）由客户端用用户输入替换后，作为普通 prompt 发一个 turn —— 运行时只负责发现，故 `body` 随列表下发（recipe 体积小）。

**无信任门（与 hooks 的关键区别）**：recipe 是注入对话的**惰性文本**，自身不执行任何东西；clone 来的仓库其 recipe 只是预填一个用户主动选择且看得见的 prompt，由此产生的工具调用仍走 approval 门。故无 `setTrust`、无 sqlite 状态。

```ts
interface Recipe {
  name: string; // 文件名去 .md —— 即 slash 命令（review → /review）
  description?: string; // frontmatter：slash 菜单 / 命令面板里显示
  argumentHint?: string; // frontmatter：slash 自动补全的占位提示，如 "[focus area]"
  body: string; // 提示模板（$ARGUMENTS / $1..$9 占位符）
  scope: "project" | "global";
  source: string; // 来源 .md 文件的绝对路径
}
```

错误：同其它读方法，cwd 解析不到 → `cwd_unavailable`。

#### 生命周期 hooks 管理（`workspace.hooks.*`）

运行时在固定的 turn 生命周期点跑用户自写的 hook（外部命令，事件 JSON 走 stdin，按 exit code 决策；或声明式 `inject` 零进程注入上下文）。hook 来自两层 `hooks.json`：**全局 `~/.lyra/hooks.json`（永远信任）** 与**项目 `<root>/.lyra/hooks.json`**。**信任模型（安全要点）**：clone 来的仓库其项目 hook **默认不跑**（防供应链 RCE），须经 `workspace.hooks.setTrust` 显式信任后、下一个 turn 起生效。

`workspace.hooks.list` 列出该 cwd 发现的**全部** hook（含未信任的项目 hook，便于审阅 `command` 后再决定信任），`active` 标注它当前是否真的会跑（全局恒 `true`；项目仅在 `projectTrusted` 时 `true`）。

```ts
interface HookInfo {
  event:
    | "PreToolUse"
    | "PostToolUse"
    | "UserPromptSubmit"
    | "SessionStart"
    | "PreCompact"
    | "Stop"
    | "Notification";
  matcher?: string; // tool-name glob（仅 PreToolUse/PostToolUse 用），缺省 = 全匹配
  command?: string; // 要跑的 shell 命令（展示出来供用户审阅项目 hook）
  inject?: string; // 声明式无-exec 上下文注入（与 command 二选一）
  scope: "global" | "project";
  source: string; // 来源 hooks.json 的绝对路径
  active: boolean; // 当前是否会跑：全局恒真；项目仅在已信任时真
}
interface HooksListResult {
  projectRoot?: string; // 信任键（cwd 最近的 .git 祖先）
  projectTrusted: boolean; // 该项目的 project-scope hook 是否已启用
  hooks: HookInfo[];
}
```

错误：`workspace.hooks.list` 同其它读方法可返 `cwd_unavailable`；`setTrust` 的 `projectRoot` 为空 → `invalid_params`。

---

### 7.6 providers.* / models.* / tools.*

> **多 provider × 多 model**。装配流程：`providers.list` → 用户填 key（`providers.configure`）→ `providers.test`（验）
> → `models.list`（解锁该 provider 的 model）→ `runs.start{ provider, model }`（选用，§7.3）。
>
> 命名：引用 provider / model 的参数一律用裸名 **`provider`** / **`model`**（有意义的 slug，无 `Id` 后缀），与
> `Model.{provider, id}` 字段一致。对象自身的身份字段仍是 `Provider.id` / `Model.id`。

#### `providers.list`

- 入参 `{ cursor?; limit? }`；返回 `Page<Provider>`（§4.9）—— **后端支持的全部 provider**（有 adapter 的），无论是否已配置。
  每条按注册表标注：`apiKeyMasked == ""` 即**未启用**（启用 ⇔ 已配 key），并带已配的 `baseUrl`。
- **env 回退（stored > env）**：未存 key 的 provider 若环境里有它的 key 环境变量（`ANTHROPIC_API_KEY` / `OPENAI_API_KEY` / …），即视为启用，`keySource: "env"`（只读，UI 显示「from env」）；显式 `providers.configure` 的 key 永远优先（`keySource: "stored"`）。需 `baseUrl` 的 provider（azure / 两个通用 passthrough）不参与 env 回退——光有 key 到不了端点。env 只读一次（进程内静态），不入库。
- **当前 21 个**：19 个具名 vendor（anthropic / openai / google / deepseek / moonshot / alibaba / azureopenai / fireworks / groq / huggingface / minimax / mistral / ollama / openrouter / perplexity / together / xai / xiaomi / zhipu）+ 2 个通用兼容 passthrough（`openai-compatible` / `anthropic-compatible`）。具名 vendor 的 model 走 `models.list` 浏览；`requiresBaseUrl` 的几个（两个通用 passthrough + `azureopenai`）无 catalog，需用户填 `baseUrl` + 自由输入 model。IAM-only 的 amazonbedrock / vertexai 不在列（不符合「填 API key」模型）。

#### `providers.configure`

- 入参 `{ provider: string; baseUrl?: string; apiKey?: string }`；upsert 进**运行态注册表**（持久化），返回 masked `Provider`。`provider` 必须是支持的 provider，否则 `invalid_params`。`baseUrl` 可覆盖默认端点（代理 / 网关 / 自建 OpenAI 兼容端点）；`requiresBaseUrl` 的 provider 必填 `baseUrl`。

#### `providers.test`

- 入参 `{ provider: string }`；返回 `{ ok: boolean; error?: ProblemData }`。**真实探活**：用该 provider 默认 model 发一次极小（`maxTokens=1`）请求验证 key + 端点。失败回 `{ ok:false, error }`（**不**报 RPC 错），UI 可内联显示原因（如 401）。

#### `models.list`

- 入参 `{ provider?: string; cursor?; limit? }`；返回 `Page<Model>`（§4.9）。直读后端内置 model catalog —— **不需要 key、不受启用门控**（解决"没填 key 就拿不到 model 列表"的死结）。`provider` 省略时返回空页（model 按 provider 组织）。

#### `models.getUtilityRole` / `models.setUtilityRole`

- **utility model role** —— 后端跑 turn 边界维护工作（压缩 / 提取 / 起标题）所用的那个（通常更便宜的）model，区别于 headline 主 model。
- `getUtilityRole`：无入参；返回 `UtilityRole = { provider?: string; model?: string }`。空 `model` ⇔ 未设置 → 那些工作跑在主 model 上。
- `setUtilityRole`：入参 `UtilityRole`（空 `model` 清除回主 model）；返回存下的 `UtilityRole`。后端**解析该 client 来校验**——未配置的 provider / 未知 model 在此报 RPC 错（UI 内联显示），不会留到下次压缩才静默退化。持久化（跨重启保留）。

#### `models.getEmbeddingRole` / `models.setEmbeddingRole`

- **embedding model role** —— `@codebase` 语义索引嵌入代码用的 (provider, model)。区别于 chat / utility model;**provider 必须是已配置且支持 embedding 的**(OpenAI / Azure OpenAI / Google / Mistral / Ollama / Zhipu / Alibaba;Anthropic 无 embedding API,可用本地 Ollama 零成本兜底)。
- `getEmbeddingRole`：无入参;返回 `EmbeddingRole = { provider?: string; model?: string }`。空 `model` ⇔ 未设置 → `@codebase` 功能关闭(不挂 `codebase_search` 工具)。
- `setEmbeddingRole`：入参 `EmbeddingRole`(空 `model` 清除);返回存下的 `EmbeddingRole`。后端**构建该 embedding client 来校验**——provider 不支持 embedding / 未配置 key / model 建不出 → `invalid_params`(UI 内联显示)。**换 model 会让各项目已存的向量失效**(下次用到时按新 model 重嵌)。持久化(跨重启保留)。

#### `tools.list`

- 入参 `{ cursor?; limit? }`；返回 `Page<ToolSpec>`。

#### `tools.invoke`

不经 LLM 直接调一个工具（诊断 / client 驱动）。

- 入参 `{ name: string; arguments: Record<string, unknown>; cwd?: string }`；返回 `unknown`（工具原始输出，best-effort JSON）。
- 错误：`tool_denied` / `path_outside_root`。

#### `usage.session` / `usage.summary`

只读花费报表,从持久化的 run 历史(`history_runs`)**sum-on-read** 聚合 —— 无 denormalize 计数器要维护,rollback/fork 丢弃的 run 自然反映(没了就不计)。每个 finished run 已带终态计量(`RunResult.usage`,**subtree 聚合**:子 agent 花费已计入父 root run)。

- **`usage.session`** 入参 `{ sessionId: string }`;返回 `Usage`(§4.6)——该会话所有 finished run 的累计 token + 成本(含 `byModel`)。
- **`usage.summary`** 入参 `{ sinceDays?: number }`(省略/0=全时段);返回 `UsageSummary`:
  ```ts
  interface UsageSummary {
    total: ModelUsage; // 总计(成本 via costUsd,未定价则省略)
    byProvider?: UsageBucket[]; // 按花费降序
    byModel?: UsageBucket[]; // 按花费降序
    byDay?: UsageBucket[]; // 按日期升序(YYYY-MM-DD)
    sessions?: number;
    runs?: number; // 计入的用户会话数 / finished run 数
  }
  interface UsageBucket extends ModelUsage {
    key: string;
    runs?: number;
  }
  ```
- **不重复计数**:`summary` 只遍历用户可见会话(子 agent 子会话被 `sessions.list` 排除),父 root run 的 subtree 聚合已覆盖子 agent 花费。
- **归因粒度 = run**:byProvider/byDay 按整 run 折叠(与 total 对账);byModel 优先用 run 自带的 `byModel` 拆分(能捕捉一个 run 触及的 utility / 子 agent model),否则落在该 run 的 headline model。

---

### 7.7 可选域（capability-gated）

关闭时返 `capability_not_negotiated`。门控位见 §9。

| 方法              | 入参                                                                      | 返回                | 门控     |
| ----------------- | ------------------------------------------------------------------------- | ------------------- | -------- |
| `memory.list`     | `WorkspaceQuery & { cursor?; limit? }`                                    | `Page<MemoryEntry>` | `memory` |
| `memory.get`      | `{ scope: "cwd"\|"projectRoot"\|"home"; cwd?: string }`                   | `MemoryEntry`       | `memory` |
| `memory.update`   | `{ scope; cwd?; content: string }`                                        | 无                  | `memory` |
| `feedback.create` | `{ sessionId?; runId?; itemId?; rating?: "positive"\|"negative"; text? }` | 无                  | ——       |

> **图片输入不走上传域**：图片随 `runs.start.input` 的 image ContentBlock 内联（mime + base64 data）传入——对照八家 agent（opencode / Claude Code / codex …）一致做法，无独立 attachment 上传/引用子系统。`unsupported_mime` 仍用于校验 image block 的 mime（非图片类型或不可解析）。

> **已移除 `background.*`**（list/subscribe/cancel + `notifications.background.update` + `BackgroundTask`，AUX_API §7）：
> 八家对照 agent 无一有 client 可见的任务注册表；后台子任务 = 子 agent 的 turn，挂在 run 树上随其流式（§5.4）。

### 7.8 服务端发出的 Notification 汇总

| Notification                    | params           | 何时                                                                                                                |
| ------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------- |
| `notifications.run.event`       | `RunEvent`       | run 期间每个事件（run/item/state/custom）                                                                           |
| `notifications.workspace.event` | `WorkspaceEvent` | 非-run 工作区变化（files/skills/mcp.serverChanged/**schedules.fired**/resync），经 `workspace.subscribe` 流（§7.5） |

> `notifications.canceled` 是 **client→server**（§7.1），不在此表。

---

### 7.9 schedules.*

定时（cron）触发的 **headless run**：到点时运行时用存好的 prompt 在指定 cwd 起一个新会话跑完、落库,**无需客户端在场**。调度器是 runtime server 进程内的常驻 worker（每分钟扫一次到点的 schedule),server 进程不在时不触发(下次起来对错过的只补跑一次、然后跳到下一个未来时点,不回放每个错过的槽)。一个 schedule 存**最终 prompt 文本**(客户端可"从 recipe 填充",但调度器与 recipes 解耦——删/改名 recipe 不会破坏 schedule)。

每次触发产生一条 `schedules.fired{scheduleId}` 工作区事件(§7.5 流),客户端据此刷新会话列表(新 run 落在一个新会话里)。

| method             | params                                                           | result                      | 备注                                                                                                               |
| ------------------ | ---------------------------------------------------------------- | --------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `schedules.list`   | ——                                                               | `{ schedules: Schedule[] }` | 全部 schedule,最新创建在前                                                                                         |
| `schedules.create` | `{ title?; prompt; cwd?; provider?; model?; cron }`              | `Schedule`                  | 新建即 enabled;`prompt`+`cron` 必填,`provider`/`model` 成对(都填选模型、都不填用默认);从 cron 算出首个 `nextRunAt` |
| `schedules.update` | `{ id; title?; prompt; cwd?; provider?; model?; cron; enabled }` | `Schedule`                  | 全替换可编辑字段并按(新)cron 重算 `nextRunAt`;disable 即清空 `nextRunAt`(worker 跳过)                              |
| `schedules.delete` | `{ id }`                                                         | 无                          | 幂等                                                                                                               |
| `schedules.runNow` | `{ id }`                                                         | 无                          | 立即额外触发一次,记录触发但不移动下次到点时间                                                                      |

```ts
interface Schedule {
  id: string;
  title: string;
  prompt: string; // 作为 run 输入发送的最终文本
  cwd?: string; // headless run 的工具锚定目录;空 = serve 目录
  provider?: string; // 与 model 成对;空 = 运行时默认
  model?: string;
  cron: string; // 5 段标准 cron："min hour dom month dow"（如 "0 9 * * 1-5"）
  enabled: boolean;
  lastRunAt?: string; // 从未触发则省略
  nextRunAt?: string; // disabled 则省略
  createdAt: string;
}
```

错误：`prompt` / `cron` 为空或 cron 不可解析、`provider`/`model` 未成对 → `invalid_params`;未知 `id`(update / runNow)→ `invalid_params`(detail 说明)。

---

### 7.10 codebase.*

**@codebase 语义索引** —— 按语义(而非字面文本)检索项目代码。后端把代码分块嵌入成向量,存 sqlite(float32 BLOB),查询时 Go 内暴力 cosine top-k(单仓库几千 chunk,微秒级,零外部依赖)。首次用到时懒构建、按文件 hash 增量刷新。**需先配置 embedding role**(`models.setEmbeddingRole`);未配则功能关闭。agent 走 `codebase_search` 工具,client 走这里(`@codebase` mention / 状态面 / 手动重建)。

| method             | params                    | result                    | 备注                                                                                                  |
| ------------------ | ------------------------- | ------------------------- | ----------------------------------------------------------------------------------------------------- |
| `codebase.search`  | `{ cwd?; query; limit? }` | `{ hits: CodebaseHit[] }` | 语义检索;首次/陈旧时先(增量)构建索引。无 embedding model → `invalid_params`(detail 提示去配置)        |
| `codebase.status`  | `{ cwd? }`                | `CodebaseStatus`          | 该 cwd 索引状态(状态面轮询用)                                                                         |
| `codebase.reindex` | `{ cwd? }`                | 无                        | **后台**全量重建(立即返回);状态面轮询 `codebase.status` 看进度。无 embedding model → `invalid_params` |

```ts
interface CodebaseHit {
  path: string; // 相对 cwd
  startLine: number; // 1-based
  endLine: number;
  snippet: string; // 命中代码片段
  score: number; // cosine 相似度 [0,1]
}
interface CodebaseStatus {
  state: "none" | "indexing" | "ready" | "error";
  modelId?: string; // 向量所用 "provider:model"(换 model 即失效重建)
  fileCount: number;
  chunkCount: number;
  indexedAt?: string; // RFC3339
  truncated?: boolean; // 命中索引上限(部分索引)
  error?: string; // 末次构建失败原因
}
```

embedding-capable provider 由 `providers.list` 的 `Provider.embeddingCapable` / `defaultEmbeddingModel` 标注(`@codebase` 的 embedding-role picker 据此筛选)。

---

## 8. 错误

### 8.1 投递通道（流式场景必读）+ 落点决策表

错误可能出现在**三个落点**。新对接者常只预期"错误在响应里"，故先给决策表（再看码表）：

| #   | 通道 `channel` | 何时                                                                                                   | 怎么投递                                                              | 终止 run？                     |
| --- | -------------- | ------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------- | ------------------------------ |
| a   | `"rpc"`        | 调用本身就错：`session_not_found` / `invalid_params` / `cwd_unavailable` / `capability_not_negotiated` | 该 method 的**同步 JSON-RPC error response**（带 `error.code`，§8.2） | 调用未起 run                   |
| b   | `"run"`        | run 已起、执行中整体失败                                                                               | 流内 **`segment.finished{ outcome:{ type:"error", result.error } }`**     | 是                             |
| c   | `"tool"`       | 单个工具失败                                                                                           | 对应 `toolCall` item 的 **`error` + `status:"incomplete"`**           | **否**（agent 多半换方案继续） |

三处都用**同一个 `ProblemData` 形状**；`ProblemData.channel`（§4.6）**自描述**它属于哪条通道，客户端无需靠"它从哪来"
反推语义。**反直觉但关键**：工具失败（c）通常不终止 run。实现方**不要**期望 run 执行期错误（b/c）能在 `runs.start`
的同步 response（a）里拿到。

### 8.2 错误码表（仅通道 a 带数字码）

业务错误用 JSON-RPC `error.code`，**不映射 HTTP status**（HTTP status 仅反映传输层，见 TRANSPORT §6.3）。

> **码值是本基线分配；client / server 一律按 `error.data.type`（符号名）判错，不按数字码。** 数字码仅作粗分类。

| Code     | type（name）                | 含义                                                                                                                                           |
| -------- | --------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `-32600` | `invalid_request`           | JSON-RPC envelope 非法                                                                                                                         |
| `-32601` | `method_not_found`          | 未知方法                                                                                                                                       |
| `-32602` | `invalid_params`            | params 校验失败                                                                                                                                |
| `-32603` | `internal_error`            | 运行时意外失败                                                                                                                                 |
| `-32001` | `provider_error`            | provider 请求失败（RPC 级兜底；run 级按模式拆 `rate_limited`/`invalid_api_key`/`timeout`/`provider_unavailable`/`provider_rejected`，见 §8.4） |
| `-32002` | `session_not_found`         | session 不存在                                                                                                                                 |
| `-32003` | `run_not_found`             | run 不存在                                                                                                                                     |
| `-32004` | `item_not_found`            | item 不存在                                                                                                                                    |
| `-32005` | `cwd_unavailable`           | 工作目录缺失或不可读                                                                                                                           |
| `-32006` | `capability_not_negotiated` | 方法或能力被关闭                                                                                                                               |
| `-32008` | `run_already_finished`      | 操作不能作用于已结束 run（run 二态 running\|finished；故无单独 "not running" 码）                                                              |
| `-32009` | `checkpoint_unavailable`    | 编辑 / checkpoint 不可用                                                                                                                       |
| `-32011` | `unsupported_mime`          | image block 的 mime 非图片类型或不可解析（`-32010` 已废，原 `attachment_too_large`）                                                           |
| `-32012` | `tool_denied`               | `tools.invoke` 被策略拒绝                                                                                                                      |
| `-32013` | `path_outside_root`         | 路径逃出 cwd 根                                                                                                                                |
| `-32014` | `interrupt_not_open`        | interrupt 缺失或已 resolve                                                                                                                     |
| `-32015` | `idempotency_conflict`      | 幂等键与不同 params 冲突                                                                                                                       |
| `-32016` | `invalid_protocol_version`  | request metadata 的协议版本不受支持                                                                                                            |
| `-32017` | `vcs_unavailable`           | 有 git 二进制但 cwd 非 git 仓（与"干净仓=空结果"区分；无 git 是 `features.git=false`）                                                         |
| `-32018` | `session_busy`              | session 有 run 在跑，拒绝会破坏在 append 历史的操作（`sessions.rollback`）                                                                     |

`error.data` 为 `ProblemData`，**必须含 `type`（= 上表 name）**。

> 业务码 `-32001..-32018` 落在 JSON-RPC 2.0 的 `-32000..-32099`「implementation-defined server-error」保留段；
> `-326xx` / `-32700` 为 spec 预定义码。数字码合规即可，**判别一律走 `type`**。

### 8.3 错误细节（ProblemData，对标 RFC 9457）

`ProblemData`（§4.6）是 **RFC 9457 _Problem Details_ 数据模型的传输无关裁剪** —— 去掉 HTTP 专属的 `status`（与 JSON-RPC
`code` 冗余）和 `instance`；`type` 是稳定符号名、**不要求是可解析 URI**（命名空间见 §8.4）。单个 `type` 即机器判别键。
约定好的扩展成员：

- **`channel`** —— 自描述错误属于 rpc/run/tool 哪条通道（§8.1）。
- **`docUrl`** —— 可选，指向该 `type` 的文档页（对标 Stripe `doc_url`）；缺省时客户端按 §8.2 符号名查表即可。
- **`retryAfterSeconds`** —— 可重试错误（典型 `rate_limited`）回传的最早重试时机；client 退避以此为准。
- **`errors: FieldError[]`** —— 字段级校验错误（典型 `invalid_params`、provider 配置 / `question` 答案表单）；
  `field` = 出错 params key，UI 可逐字段标红。

### 8.4 `type` 命名空间（防撞名）

error `type` 是 §2.6 命名空间的一个实例：first-party 用裸 `snake_case`；**第三方插件**产出的错误
（工具执行失败落在 `toolCall.error`，§4.3）用 `plugin:<pluginName>/<symbol>`（如 `plugin:acme/quota_exceeded`）。

**first-party `type` 分两类**：

- **RPC 级**（通道 a）：§8.2 数字码表里的那些（带 `error.code`）。
- **run 级 / 执行期**（通道 b/c，**无数字码**，仅 `ProblemData.type`）：`tool_failed`（工具执行失败）、`denied_by_user`
  （HITL 用户拒绝该工具，§6）、`agent_stuck`（agent loop 无前进进度被守卫终止 —— run 终态错误，区别于落 `internal_error` 的意外失败），以及 **provider 失败按模式拆出的稳定符号**（落在 `segment.finished` 终态 `result.error`，`channel:"run"`）：
  - `run_lost` —— runtime 重启时发现 executor 已消失且没有可恢复 interrupt；启动恢复会把该 Run 及仍在 running 的 Item 原子收敛到终态。
  - `rate_limited` —— 被限流（429 / overloaded / quota），`retryable:true` + `retryAfterSeconds`。
  - `invalid_api_key` —— 凭证被拒（401 / 403），**不可重试**，UI 引导改 key。
  - `timeout` —— 请求超时 / 连接失败，`retryable:true`。
  - `provider_unavailable` —— provider 临时不可用（5xx），`retryable:true`。
  - `provider_rejected` —— provider 判定请求非法（400），**不可重试**；与 §8.2 RPC 级 `invalid_request`（-32600，坏 envelope）**同名不同物**，靠 `channel` 区分。
  - `provider_error` —— 兜底的未归类 provider 失败（也是 §8.2 RPC 级 `-32001` 的符号）。
    客户端**只按 `type`（+ `retryable`）分支**，绝不 substring-match `detail`。**与 §8.2 的 `tool_denied` 区分**——后者是**策略**拒绝 `tools.invoke`
    （RPC 级，带码 `-32012`），`denied_by_user` 是**用户**在 HITL 里拒绝（item 级，无码）。

---

## 9. Capabilities 与请求能力

```ts
type FeatureFlag = boolean | { enabled: boolean; [key: string]: unknown };

interface ServerCapabilities {
  protocolVersion: string;
  events: string[]; // 发出的事件 type（run.* / item.* / state.* / custom 名；第三方 custom 名遵 §2.6）
  streamingMethods: string[]; // 机器可读的流式方法集（如 ["runs.start","runs.resume","runs.subscribe","workspace.subscribe"]）
  features: Record<string, FeatureFlag>; // 开放 map：未声明 / falsy 即关闭
  providers: string[];
  limits: { maxConcurrentRuns?: number };
}

interface ClientCapabilities {
  events: string[]; // 渲染得了的事件 type
  features: Record<string, unknown>;
  interruptTypes?: ("approval" | "question" | "toolResult")[]; // 能处理的 HITL 类型（防挂死，§6.2）
  optOutNotificationMethods?: string[]; // 本 request/stream 抑制某些高频通知，如 ["item.delta"]
}
```

**`features` 是开放 map（与 `ClientCapabilities.features` 对称）**：runtime advertise 新能力 = 加一个 key，老客户端按
"忽略未知"自动容忍，**不 bump 契约**（能力声明本就是协议最该可加的扩展点）。已知 key（缺省视为关闭）：

| key             | 形态 | 含义                                                                      |
| --------------- | ---- | ------------------------------------------------------------------------- |
| `reasoning`     | bool | 产出 `reasoning` item / delta                                             |
| `mcp`           | bool | MCP 工具 / `workspace.mcp.*`                                              |
| `multimodal`    | bool | 图片输入：`runs.start.input` 的 image ContentBlock（mime + base64，§4.3） |
| `git`           | bool | `workspace.listFileChanges` / `getDiff`（git 二进制在 PATH）              |
| `checkpoints`   | bool | `restoreType`（v2 影子 git 文件快照）                                     |
| `fileWatch`     | bool | `workspace.subscribe` 的 `watches` → `files.changed`（fsnotify 文件监听） |
| `subagents`     | bool | 子 Run / run 树                                                           |
| `skills`        | bool | `workspace.listSkills`                                                    |
| `sessionExport` | bool | `sessions.export`                                                         |
| `memory`        | bool | `memory.*`                                                                |
| `relocate`      | bool | `sessions.update` 改 cwd                                                  |
| `clientTools`   | bool | `toolResult` interrupt                                                    |

> **提案中的 feature（613，缺省关闭直到后端落地，见附录 C）**：`codeIntel`（`workspace.code.*`）/ `todos`（`todos.list`）/
> `compaction` 的**显式 `sessions.compact` RPC**（自动压缩的 `compaction` Item 已落地，无需协商，见附录 C.4）。additive 加 key、老 client 忽略未知 → 不 bump 契约。

规则：

- 缺省 / falsy feature 默认**关闭**。
- server 不得在本 request/stream 发出 `clientCapabilities.events` 集合外的事件类型（已知 payload 上的未知未来字段除外）；client 必须忽略未知字段。
- server **必须不**产出 client 未在 `interruptTypes` 声明的 open interrupt（§6.2）。
- `features.subagents` 关 → 不产出子 Run；`features.clientTools` 关 → 不产出 `toolResult` interrupt。

---

## 10. 历史 / 重连 / 恢复

### 10.1 正常断线重连

1. 对每个仍显示的 root run 调 `runs.subscribe { runId }` 续流（每个 run 一条流，传输细节见 TRANSPORT §9.2）；
2. 支持时带 last-seen event id（`Last-Event-Id`）让 server 重放 durable 缺口；
3. 调 `items.list` 补 durable 缺口；
4. 按 `itemId` 与 `eventId` 去重。**ephemeral delta 不重放**（§5.2 保证正确）。

### 10.2 进程/客户端重启恢复

1. `sessions.list` / `sessions.get`；
2. `items.list` 拉 durable 历史；
3. `runs.list` 拿在跑的 run；
4. `runs.listOpenInterrupts` 拿可恢复的被中断 run；
5. `runs.resume` 应答 open interrupt（payload 自包含，无需额外 join，§4.8）。

### 10.3 还原 Run 树（一个会话跨多个 Run）

`items.list` 同时返回 `runs: RunRef[]`。客户端按 `runId` 把 item 归到 Run，再用 `RunRef.spawnedByItemId` 把子 Run 嵌到父
toolCall Item 下（**子树，`features.subagents` 门控**）。延续（resume）不产生独立 Run —— 一个 Run 的停车-续跑各段共享同一
`runId`，天然归为一条（§0.3），无需串链。

三个 run 视图职责不重叠：`runs.list`（在跑）/ `listOpenInterrupts`（待解）/ `items.list.runs`（历史结构）。

---

## 11. 三个扩展缝：Item / state / custom（选择指南）

运行时想表达"额外的东西"时有三处可放，**何时用哪个有明确边界**——选错就漂移（如把该 durable 的塞进 ephemeral 的 custom）：

| 缝         | 是什么               | 何时用                                                           | durable                | 命名空间           |
| ---------- | -------------------- | ---------------------------------------------------------------- | ---------------------- | ------------------ |
| **Item**   | durable 历史工作单元 | 要进历史、用户回看的产物（消息 / 推理 / 工具调用 / 计划 / 提问） | ✅                     | item.type 一等枚举 |
| **state**  | run 期共享可变视图态 | run 进行中的可变面板（todo board / 进度面板），有**终值快照**    | snapshot ✅ / delta ⬜ | 顶层 key（§2.6）   |
| **custom** | 一次性信号           | **不进历史、不改状态**的瞬时提示（"已连接 MCP"一类）             | 帧上自带（默认 ⬜）    | `name`（§2.6）     |

- 选 **Item**：它是工作回看的一部分吗？是 → Item（享受 `items.list` 历史 + run 树）。
- 选 **state**：它是 run 期一直在变、最后有个稳定终态的视图吗？是 → state（`state.delta` 流式 + `state.snapshot` 落终值，§5.3）。
- 选 **custom**：它既不回看、也不构成状态，只是一次性提示吗？是 → custom（若需可恢复，必须自带 durable 落点，§5.2）。

> `state` 用 RFC 6902 JSON Patch（`state.delta`）做增量、`state.snapshot` 做终值落点 —— 存在的理由就是"有终值的共享可变态"
> 这一类，不要拿它当一次性信号（那是 custom）或历史产物（那是 Item）。

---

## 12. 版本规则

- `protocolVersion` 是日期串（本定稿 `2026-06-07`）。
- **前向兼容是硬约定**：client 必须忽略未知字段、对未知 method/事件容忍。加 method / 加可选字段 / 加事件 /
  加 `features` map key → **同版本号**。
- 改语义 / 删字段 / 改字段类型 / 改判别集合 → 新日期版本；client 通过 request metadata 明确自己使用的版本。
- 不存在连接级硬断开；版本不兼容以 request 级 `invalid_protocol_version` 返回。
- dev 阶段无 legacy 兼容：shape 变了就 bump version、丢旧 store、不写 migration。
- HTTP URL 里的 `/v2/`（wire major epoch）与日期 `protocolVersion`（epoch 内请求版本）是两个层级，不重复
  （见 TRANSPORT §6.1）。

---

## 13. v2 明确不做

- 经 `runs.send` 的 mid-run steering（留 v2.x，additive）。
- server→client JSON-RPC request。
- 远程多用户鉴权（协议层零 user 概念；鉴权由更外层 / 未来 facade 解决）。
- JSON-RPC batch。
- stdio transport。
- 客户端自选的业务资源 id。
- 多根 workspace（`Session.cwd` 单根；多根是未来破坏性改动）。
- **强类型领域工具变体**（`commandExecution`/`fileChange`/… 作为 wire 一等类型）——核心领域中立，富渲染走客户端
  展示注册表（§4.4），新工具不动协议。

---

## 14. 机器可读制品 / 漂移闸

后端 Go `lyra/internal/delivery/protocol` 是**机械 SSOT**。为根除"手写两份 wire 类型 + 人工 review 同步"导致的漂移，本协议要求：

- 从 Go SSOT **导出机器可读制品**：**OpenRPC**（方法表 + 入/出 schema）与 **JSON Schema**（数据类型）。这是非 TS / 非 Go
  客户端的单一对接物，不必读文档手抄。
- **黄金样本契约测试**：一组 canonical JSON wire 样本（每个方法的请求/响应、每类事件帧），前后端 CI 各自往返校验。
  本基线消除的两类历史 bug（`items` vs `data` 字段名漂移、completed 缺权威落点）正是这层测试当场能抓的。
- CI 卡 drift：生成的 TS / schema 与 SSOT 不一致即红。

> **这是迁移的硬前置项，不是"以后再补"**。§4.4 去领域化后，富 `result` 形状（§4.4.2 的 `shell`→`{exitCode,output,outputTruncated}`、
> `grep`→`{hits}` 等约定）**不再被 wire 联合机器保证**——它们是非规范展示约定。唯一能阻止这些约定在前后端间无声漂移的，
> 就是黄金样本 + 从 SSOT 导出的 schema。**故迁移前必须先立起这层闸**，否则正好放任 G1 本想消除的那类约定漂移 bug。

---

## 15. 安全不变量汇总

- **路径 containment**：`ContextItem` / fs 工具路径相对 `cwd`，越界 → `path_outside_root`（§4.7）。
- **URL fetch SSRF**：拦 loopback / 私网 / 元数据地址，除非宿主放行（§4.7）。
- **provider secret**：只回 `apiKeyMasked`，永不可逆推（§4.9）。
- **协议层零鉴权**：无 user / account 概念；本地进程门禁由 transport 层处理（TRANSPORT §11）。
- **cwd 是会话身份不是传输上下文**：走 body 不走带外 directory header（TRANSPORT §2）。
- **防挂死**：server 不产出 client 解不了的 open interrupt（§6.2）。

---

## 附录 A · 设计不变量摘要

把本协议区别于"朴素 agent wire"的关键立场集中一处，便于 onboarding 与 review 自查：

1. **领域中立核心**：核心只懂 Session/Run/Item/通用 `tool`。"工具长什么样"是领域知识，走客户端展示注册表，新工具零协议成本（§4.4）。
2. **一个判别字段 `type`**：所有联合看 `type`，`kind` 不在 wire 上出现（§2.1）。
3. **durable/ephemeral 不变量**：丢掉每个 ephemeral 事件仍得正确终态；每个 ephemeral 必有命名 durable 落点（§5.2）。`durable` 由 `event.type` 推导，不每帧冗余携带。
4. **HITL = R 模型**：interrupt 收尾当前**段**、`runs.resume` 在同一 Run 上续段（同 `runId`、新 `segmentId`）；所有 interrupt payload **自包含**（§6 / §4.8）。
5. **元数据带外、业务进 params**：cwd/sessionId/runId 进 params；trace/版本/幂等键/token/游标走带外（TRANSPORT §2）。
6. **能力开放可加**：`features` 开放 map、`events`/`providers`/`streamingMethods` 开放数组；新能力不 bump 契约（§9）。
7. **错误三落点、一个形状**：rpc / run / tool 三通道共用 `ProblemData`，`channel` 自描述（§8）。
8. **分页一个读法**：所有 list 是 `Page<T>`，不静默截断（§4.11）。
9. **id server 生成 + 类型前缀**；`eventId` 在 root run stream 内单调，是续流/去重锚（§2.4）。
10. **Minimal Profile**：最小客户端只需 sessions.create + runs.start + item/run 事件 + items.list；`runtime.discover` 可选，其余分层可选、capability 门控（§3.2）。

---

## 附录 B · 设计借鉴（取思想，不取命名）

本协议名实第一性自决；下列业界设计仅取**思想**作独立印证，**不作命名锚**——名字相同只因独立评估下最优。

**Stripe（API 工程范式）** —— 借其思想：

- **日期串 pin 版本**：`protocolVersion` 用日期（§12），破坏性变更新日期，非递增 major。
- **幂等键作可靠性控制**：`X-Idempotency-Key` 走带外、重试不重复创建（TRANSPORT §10）——非业务、不进 params。
- **列表统一信封 + `has_more` 语义**：所有 list 是 `Page<T>`，`nextCursor` 存在性即 `has_more`（§4.11）。
- **错误是结构化对象**：`ProblemData` 有稳定 `type`（符号判别键）+ 字段级 `errors[].field`（对标 `param`）+ 可选 `docUrl`（§4.6 / §8）。
- **资源自描述**：用**类型化 id 前缀**（§2.4）满足"对象自报类型"思想，不加冗余 `object` 字段。
- **命名拼全、单位入名**：`maxBudgetUsd` / `sizeBytes` / `expiresAt`（§2.2），缩写白名单收口。
- **不采**：`expand[]` 嵌套展开（YAGNI——用显式 join，如 `items.list.runs`）。

**opencode（同类 agent runtime，强印证）** —— 独立印证我们的核心选择：

- **通用 tool envelope `{ name, arguments, result }`、无 per-tool 类型**（§4.4）—— 与其生产实现同向，验证"领域中立核心"可行。
- **全程 `type` 判别**（§2.1）—— 同向。
- **delta + 权威终态**两段式流（§5）、**可排序 id**（§2.4）、**Usage 非重叠细分**（§4.6）—— 各取其思想。
- **不采**：深层点号事件名（`session.next.tool.input.started` 一类，其自身亦认为偏乱）——我们用扁平 `item.delta{type}`；
  per-event `version` 字段——我们按日期 bump 整协议；分裂的 permission/question 双审批——我们统一到 HITL interrupt（§6），
  正好规避"不知该用哪个"的模糊。

**MCP / JSON-RPC** —— envelope 形态（§1）、capability 声明（§9）、streamable HTTP（TRANSPORT §6）取自此脉络。

---

## 附录 C · 提案面（613 批次 B7–B12，前端已预接、后端待落地）

> **状态：形状已定、后端方法表（`internal/delivery/dispatch/method_names.go`）尚未注册。** 这批是**纯 additive**（§12 同版本号）：
> 落地时整体并入 §7/§9，wire 不变、契约不 bump。**当前调用这些方法返 `method_not_found`（-32601）**；前端 `rpc/` 已按本附录
> 形状预接，并把对应 feature 当作未声明（关闭）→ UI 自然降级、不报错。落地 = 后端注册方法 + advertise feature，本附录条目转入正文。
>
> 新增 feature 门控（§9 features map，缺省关闭）：`codeIntel`（B7）/ `todos`（B11）/ `compaction` 的显式 `sessions.compact` RPC（B10 —
> 自动压缩的 `compaction` Item 已落地、不门控）。B8（文件浏览）属基础读、不门控；B9（审批运行时控制）不门控；B12 归 `mcp` 门控。

### C.1 B7 · `workspace.code.*`（门控 `codeIntel`）—— LSP 支撑的只读代码导航

坐标 **0-based、`character` 计 UTF-16 code unit（LSP 约定）**——与 `workspace.readFile`（C.2）的 1-based 闭区间行号（面向编辑器/人）
**不可混用**。该文件类型无 language server → `no_language_server`（非致命，UI 退避）；server 正索引 / 不可用 → **空结果**
（非错误——wire 上"无结果"与"未就绪"不可区分）。

| 方法                              | 入参                                                          | 返回                            |
| --------------------------------- | ------------------------------------------------------------- | ------------------------------- |
| `workspace.code.definition`       | `CodeQuery & CodePosition`                                    | `{ locations: CodeLocation[] }` |
| `workspace.code.references`       | `CodeQuery & CodePosition & { includeDeclaration?: boolean }` | `Page<CodeLocation>`            |
| `workspace.code.hover`            | `CodeQuery & CodePosition`                                    | `Hover`                         |
| `workspace.code.documentSymbols`  | `CodeQuery`                                                   | `{ symbols: DocumentSymbol[] }` |
| `workspace.code.workspaceSymbols` | `{ cwd?; query: string; limit? }`                             | `Page<WorkspaceSymbol>`         |
| `workspace.code.diagnostics`      | `CodeQuery`                                                   | `{ diagnostics: Diagnostic[] }` |

```ts
interface CodeQuery extends WorkspaceQuery {
  path: string;
} // cwd 下工作区路径（jail，§7.5）
interface CodePosition {
  line: number;
  character: number;
} // 均 0-based；character 计 UTF-16 code unit
interface CodeRange {
  start: CodePosition;
  end: CodePosition;
}
interface CodeLocation {
  path: string;
  range: CodeRange;
  external?: boolean;
  preview?: string;
} // 外部依赖(GOROOT/node_modules)给绝对路径 + external:true；preview=该行文本，省一次 readFile
interface Hover {
  contents: string;
  range?: CodeRange;
} // contents=markdown（签名 + doc）
type SymbolKind =
  | "file"
  | "module"
  | "namespace"
  | "package"
  | "class"
  | "method"
  | "property"
  | "field"
  | "constructor"
  | "enum"
  | "interface"
  | "function"
  | "variable"
  | "constant"
  | "string"
  | "number"
  | "struct"
  | "enumMember"
  | "typeParameter"
  | (string & {}); // 开放：镜像 LSP SymbolKind，未知值降级默认图标
interface DocumentSymbol {
  name: string;
  kind: SymbolKind;
  detail?: string;
  range: CodeRange;
  selectionRange: CodeRange;
  children?: DocumentSymbol[];
}
interface WorkspaceSymbol {
  name: string;
  kind: SymbolKind;
  path: string;
  range: CodeRange;
  containerName?: string;
}
interface Diagnostic {
  range: CodeRange;
  severity: "error" | "warning" | "info" | "hint";
  message: string;
  source?: string;
  code?: string;
}
```

> 与 `features.lsp` 区分：`lsp` 门控**模型用的 `lsp_*` 工具**（走普通 toolCall 渲染）；`codeIntel` 门控 **UI 直调的 `workspace.code.*`
> RPC**（@symbol / 代码导航）。两者正交。

### C.2 B8 · `workspace.listFiles` / `workspace.readFile`（基础读，不门控）—— 文件树浏览 + 查看

> **状态**：`workspace.listFiles` **已落地**——gitignore-aware（repo 用 `git ls-files --cached --others
--exclude-standard`，非 repo 走兜底 walk + 排除 `.git`/`node_modules` 等）；`path`/`recursive`/`glob`/
> `includeIgnored`/`limit` 全支持（`cursor` 暂未真正分页——结果在 `limit` 处截断，消费方靠 path/glob 收窄）；
> `FileEntry.sizeBytes`/`modifiedAt` **暂不填充**（消费方——文件树 + @file——不需要，递归列举逐个 stat 会拖慢调用）。
> `workspace.readFile` **也已落地**——读整文件或 `startLine..endLine` 窗口（1-based 闭区间）；`totalLines` 始终是整文件行数；
> 路径同样 jail 到 root，binary 文件报 `fs.ErrBinaryFile`。支撑文件查看器 + 输出里可点的 `file:line` 引用。

| 方法                  | 入参                                                                   | 返回              |
| --------------------- | ---------------------------------------------------------------------- | ----------------- |
| `workspace.listFiles` | `{ cwd?; path?; glob?; recursive?; includeIgnored?; cursor?; limit? }` | `Page<FileEntry>` |
| `workspace.readFile`  | `{ path: string; cwd?; startLine?; endLine?; maxBytes? }`              | `FileContent`     |

- `listFiles`：`path` 起始目录（相对 cwd，缺省根）；`recursive` 缺省 false（单层惰性树）；`glob`（如 `**/*.go`）隐含递归；
  默认守 `.gitignore` + 兜底排除，除非 `includeIgnored`。
- `readFile`：`startLine`/`endLine` **1-based 闭区间**（面向编辑器，**不同于** C.1 的 0-based）；`truncated` 自描述触顶 `maxBytes`。

```ts
interface FileEntry {
  path: string;
  name: string;
  type: "file" | "dir" | "symlink";
  sizeBytes?: number;
  modifiedAt?: string;
} // path 相对 cwd；sizeBytes 仅 file
interface FileContent {
  path: string;
  content: string;
  encoding: "utf-8";
  totalLines: number;
  truncated?: boolean;
  startLine?: number;
  endLine?: number;
} // 文本内容；totalLines 是整文件行数（切片也给，UI 显示 "12–40 / 320"）
```

### C.3 B9 · `approval.*`（不门控）—— 全局审批姿态 + 记忆决策

`ApprovalMode` 是 **每 Runtime 一个的全局策略**（非 per-session），与 `Item.toolCall.safetyClass`（per-tool 风险）正交，二者合决一次调用是否驻留待批。

> **状态**：四个方法**全部已落地**。`approval.getMode` / `approval.setMode` 取代了原 agent/chat/plan 的 per-run
> `mode`（见 §4.2 / §7.1）；`approval.listRules` / `approval.forgetRule` 是持久细粒度规则的读 + 管理面。

| 方法                  | 入参                     | 返回                                                                     | 状态   |
| --------------------- | ------------------------ | ------------------------------------------------------------------------ | ------ |
| `approval.getMode`    | ——                       | `{ mode: ApprovalMode }`                                                 | 已落地 |
| `approval.setMode`    | `{ mode: ApprovalMode }` | `{ mode: ApprovalMode }`                                                 | 已落地 |
| `approval.listRules`  | `{ sessionId }`          | `{ rules: ApprovalRule[] }`（该会话可见：session + 其 project + global） | 已落地 |
| `approval.forgetRule` | `{ id }`                 | 无（按规则 id 删一条；清空 = 逐 id 调用）                                | 已落地 |

```ts
type ApprovalMode = "plan" | "safe" | "balanced" | "yolo"; // plan=只读规划姿态：写/exec/network 一律拒（不提示），agent 只调研+出计划，由 exit_plan_mode 工具呈交计划并翻回执行；safe=所有写/exec 驻留；balanced(默认)=按 safetyClass 高危驻留、低危过；yolo=全过、不驻留（自动化）
interface ApprovalRule {
  // 一条持久"记住这个决策"规则（AUX_API §6）
  id: string; // 稳定 id（domain 内对 scope+key+tool+subject 哈希），forgetRule 用
  scope: "session" | "project" | "global";
  tool: string; // 工具名，如 "shell"
  subject?: string; // 命中该工具的子主题 glob（shell 的 command / 文件工具的 file_path）；省略 = 该工具任意参数
  dir?: string; // project scope 的目录（仅展示；session/global 省略）
  decision: "allow" | "deny";
}
```

> `plan` 是 agent/chat/plan run-mode 移除后"计划"的归宿：它是一个**姿态**而非 run 模式，模型在该姿态下调研完
> 调 `exit_plan_mode`（一个 `question` interrupt，§6）呈交计划；批准 → 姿态自动翻回 `balanced` 执行，拒绝 → 留在 `plan`。
> 规则的**写入**面是 HITL 应答里的 `ApprovalResponse.remember{scope}`（§6.1 / AUX_API §6）——`subject` 由后端按工具从被批准调用的参数中提取；`listRules`/`forgetRule` 是其**读 + 管理**面。最具体的命中规则胜出（session > project > global，再 exact > glob > 任意），同特异度冲突取 deny。

### C.4 B10 · `sessions.compact` + `compaction` Item（部分落地）—— 主动上下文压缩

- **`compaction` Item 变体已落地**（§4.3 Item 联合的 additive 第 7 变体）：turn 边界的**自发压缩**现产出它（`item.started` + `item.completed`，`droppedMessages` = 压缩前后净减条数），fold 成时间线分隔条。`summary` 暂留空（摘要文本已折进重写后的历史）。
- **`sessions.compact` RPC 仍是提案**：`sessions.compact{ sessionId; force? }` → `CompactionResult`，`force:false` 仅在超内部阈值时压（与自发压缩同条件）、运行中拒绝（`session_busy`）、内部调 LLM 可能数秒 —— 后端尚未实现这条显式入口。

```ts
// §4.3 Item 联合 additive 变体：
| (ItemBase & { type: "compaction"; summary?: string; droppedMessages?: number })

interface CompactionResult { session: Session; compacted: boolean; beforeMessages?: number; afterMessages?: number; summaryItemId?: string }  // compacted:false = 未超阈值且未强制，什么都没做
```

### C.5 B11 · `todos.list`（门控 `todos`）—— 模型的工作清单

模型的 `todo_write` 清单（**非**已删的 `background.*` 任务注册表）。实时更新走既有 `state.snapshot{todos}` 通道（§5.3，无新事件类型）；
`todos.list` 是非活跃 run / 重开历史时的冷读。

| 方法         | 入参            | 返回                    |
| ------------ | --------------- | ----------------------- |
| `todos.list` | `{ sessionId }` | `{ todos: TodoItem[] }` |

```ts
interface TodoItem {
  id: string;
  text: string;
  status: "pending" | "in_progress" | "completed";
}
```

### C.6 B12 · `workspace.mcp.authenticate`（门控 `mcp`）—— 向 needsAuth server 递 token

- 入参 `{ server: string; token: string }`；**无同步返回**（异步，同 `reconnect`）。后端拿 token 重连，经 `mcp.serverChanged`
  推 `connecting → (connected | needsAuth | failed)`。**后端只转发、不存** token；OAuth 浏览器流（若有）是用户侧的事，lyra 只转发结果 token。

---

> 正式契约。配套同目录 [`TRANSPORT.md`](./TRANSPORT.md)。
