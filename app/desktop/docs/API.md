# Lyra Runtime Protocol

> 表现层（Web / 套壳 Web / TUI / …）与 **Lyra Runtime** 之间的通讯协议。
> Runtime 是无状态纯计算单元（AG-UI 兼容的 agent 执行引擎 + tool 调用
> + LLM provider 调用），**不感知用户、不做鉴权、不管账号**，只暴露
> 本文档定义的方法。
>
> Wire 格式参考 MCP —— **JSON-RPC 2.0 envelope**，多种 transport 共享
> 同一份语义。物理传输见 `TRANSPORT.md`。**本文档是前后端共享的单一契约
> 真相源**（CLOSED contract）：只描述协议、不记录实现进度；动任何方法 /
> shape 前后端先对齐。**类型命名以后端 Go interface（`lyra/rpc/protocol`）
> 为 SSOT**，规则见 §8.1。

---

## 0. 架构模型

**Runtime** = 实现本协议的一个进程（或同进程对象）。不知道 OS 用户是谁、
不持有账号 / 订阅、不主动连任何上层服务，只暴露本文档的 method。内部职责：
会话存储 / AG-UI 事件流 / LLM provider 调用 / tool 执行 / HITL 审批。

**表现层** = 实现本协议 client 一侧 + 渲染/输入的程序。持有一个 `runtime`
句柄（in-process 是 Go 对象，跨进程是 transport endpoint），把用户输入翻译成
method call，把 Runtime 流回的 AG-UI 事件渲染成 UI。

### 设计原则

| 原则 | 理由 |
| --- | --- |
| **JSON-RPC 2.0 envelope** | MCP 验证过的多 transport 共享方案。三种 message：Request / Response / Notification。 |
| **元数据走带外通道** | connection id / trace id / 流恢复 cursor 走 `context.Context` 或 transport header/URL，**永不进 message body**（§1.2）。（幂等是例外：runId 就在 body，§6.3。） |
| **取消必须显式信号** | 断 transport ≠ 取消 run；重连可续传。只有显式 cancel 才真停（§2.3）。 |
| **能力协商必经 `initialize`** | 握手前 Runtime 不接业务方法；版本不匹配硬断开（§2.1）。 |
| **AG-UI 事件作为流式语言** | 标准事件 + Lyra CUSTOM 事件，封装在 notification 的 `params.event` 里（§4）。 |
| **协议层零鉴权** | 没有 user / account / subscription 概念。需要时由外层（OS 信任、本地进程门禁 token、未来 facade）解决。 |
| **HTTP transport ops 友好** | method 名进 URL、sidecar metadata 端点、强制 method label（§9 / §10）。 |

---

## 1. Wire 格式 —— JSON-RPC 2.0

### 1.1 消息类型

```ts
type Message = Request | Response | Notification;

interface Request {
  jsonrpc: "2.0";
  id: string;            // 连接内唯一字符串（计数器 stringify 或 UUID），不重用，绝不 null
  method: string;        // "runs.start" / "sessions.list" / …
  params?: unknown;
}

interface Response {
  jsonrpc: "2.0";
  id: string;            // 匹配 Request.id
  result?: unknown;      // result 与 error 互斥
  error?: { code: number; message: string; data?: unknown };
}

interface Notification {
  jsonrpc: "2.0";
  method: string;        // "notifications/run/event" / …
  params?: unknown;
  // 无 id
}
```

`id` 是 **string**（JSON-RPC 2.0 原生允许 `string | number`，本协议统一取
string）：客户端连接内生成唯一字符串（计数器 stringify 或 UUID 皆可），服务端
原样回显。**全协议 id 一律 string** —— 业务资源 id（runId / sessionId /
messageId / eventId / requestId / taskId / attachmentId）本就都是 string，
envelope `id` 也随之 string，前端不必维护"envelope=number / 业务=string"两套
心智。（注：envelope id 是连接内 request↔response correlation 用、transient、
不跨边界，**与"雪花/精度"无关**，选 string 纯为类型统一。）

**协议明确不做**：

- ❌ JSON-RPC batch —— pipelining 用并发连接解决（MCP 2025-06-18 也删了）
- ❌ stdio transport —— Lyra 无此需求
- ❌ Server → Client request —— 双向 RPC 让每种 transport 都得实现反向通道。
  HITL 用「Notification + 后续 Request」配对实现（§4.3）

### 1.2 元数据走带外通道

JSON-RPC envelope **只装** `{jsonrpc, id, method, params}` / result / error /
notification 字段。其余元信息按 transport 各取一种带外通道：

| 元数据 | InProcess | Wails IPC | HTTP |
| --- | --- | --- | --- |
| **Connection ID**¹ | Go `context.Context` | message metadata | `Lyra-Connection-Id` header（POST）/ `?conn=` query（SSE，见 §3.2） |
| Trace ID | context | metadata | `Lyra-Trace-Id` header |
| Protocol version | 编译期保证 | metadata | `Lyra-Protocol-Version` header |
| Last-Event-Id（流恢复） | n/a | metadata | `Last-Event-Id` header（SSE 自动） |
| 本地进程门禁 token² | n/a | n/a | `Authorization: Bearer <token>` |

¹ **Connection ID** 标识一个**传输连接**，不是聊天 `Session`。它把客户端发出
的 Request 与该客户端的 notification 流（§3.2）关联起来；一个连接上可同时
操作多个聊天 `Session`。

² **本地进程门禁** ≠ 用户鉴权。仅当 Web 表现层连 loopback HTTP 时阻止同机
其他进程乱调。token 写 `~/.lyra/local-token`（chmod 600），启动随机生成，无
过期 / refresh / 用户绑定。

---

## 2. Lifecycle

```
[Connect] → runtime.initialize → operate → runtime.shutdown → [Disconnect]
```

握手完成前 Runtime 拒绝任何业务方法。

### 2.1 `runtime.initialize`

```ts
// params
interface InitializeRequest {
  protocolVersion: string;                 // 日期串 "2026-05-28"
  clientInfo: { name: string; version: string };
  capabilities: ClientCapabilities;        // §6.1
}
// result
interface InitializeResponse {
  protocolVersion: string;
  serverInfo: { name: string; version: string };
  capabilities: ServerCapabilities;        // §6.1
}
```

版本协商（同 MCP）：client 报想用的版本 → server 支持则回同版本，否则回自己
支持的最新版本 → client 若无法 fall back 必须断开。`initialize` 是**唯一**因
版本不匹配硬断开的方法。

### 2.2 `runtime.shutdown`（Notification）

```ts
interface ShutdownRequest { reason?: string }   // 无 response
```

Runtime 收到后停接新 Request、用 `notifications/run/closed`（stopReason
`"canceled"`）终止进行中的 run、关 transport。

### 2.3 取消必须显式 —— 两个不同的取消，不要混

| 取消对象 | 机制 | 关键字段 |
| --- | --- | --- |
| 在飞的 JSON-RPC **Request**（如慢 `sessions.list`） | `notifications/canceled`（client→server） | `{ id: string, reason? }` —— `id` 是被取消 **Request 的 `id`**（§1.1 的 envelope id） |
| long-running **run** | `runs.cancel`（正经 Request，§5.2） | `{ runId, reason? }` —— 停 run，流以 `run/closed` stopReason `"canceled"` 收尾 |

> **`id` vs `requestId` 不同概念**（都是 string，靠名字区分）：取消在飞 Request
> 用 `id` —— 就是那条 Request 的 **envelope id**（§1.1）；HITL 审批 / 提问用
> `requestId` —— 业务 id（§4.3/§4.4）。取消侧用 `id` 而非 `requestId`，避免与
> 审批的业务 id 混淆。

Runtime 必须区分「网络抖断」与「主动取消」：仅 transport 断开时按 client 短暂
离线处理（run 继续跑、缓冲事件等续传）；**只有**显式信号才真停。

---

## 3. Streaming

### 3.1 请求 / 响应模型

流式方法（`runs.start` / `workspace.terminal.subscribe` / `background.subscribe`）
返回流而非单一结果：

1. Client 发 Request（如 `runs.start`）。
2. Server **立即回 Response**，含**资源唯一 id**（`runs.start` 返 `runId`，
   `background.subscribe` 返 `taskId`），不等流结束。
3. Server 通过 Notification 推事件，每条带资源 id + 单调递增 `eventId`：
   ```ts
   interface RunEvent {              // notifications/run/event 的 params
     runId: string;                  // 关联回 runs.start
     eventId: string;                // 单调递增，Last-Event-Id 续传 key
     ts: string;                     // 服务端权威时间戳 ISO-8601（每条必带）
     parentToolUseId?: string;       // 子 agent 归属（见下）
     event: AgUiEvent;               // §4
   }
   ```
   `parentToolUseId` 缺省 = 主 agent 的事件；存在 = 某次 tool-use 派生的子
   agent（同步 task / 后台 subagent）产生的事件，客户端据此归到对应轨道
   （对标 Agent SDK 的 `parent_tool_use_id`）。
4. 流结束时 server 发 `notifications/run/closed { runId, result: RunResult }`
   （§6.3）：停止原因 + 用量 + 成本 + 轮数。**终态 + 计量从这里一次读全，
   不靠解析末事件、不靠扫 `lyra.telemetry` 流**（与 `background/update` 对齐）。
5. 客户端用资源 id（runId / taskId）过滤属于自己的事件。

**为什么不用第二个 streamHandle id**：每个流式方法已有自己的唯一资源 id，再加
一个是冗余 —— 流过滤直接按资源 id。

### 3.2 连接关联（哪条流收哪个 run 的事件）

notification 流**按连接**投递。默认路由：run R 的事件发给「启动 R 的那条连接」。
**重连 / 崩溃恢复**（新连接要收一个它没启动的旧 run）走显式 `runs.subscribe
{ runId }`（§5.2）把该 run 重新绑到当前连接 —— 解决 §3.3 崩溃恢复与本节默认路由
的张力：默认路由管"谁起的发给谁"，`runs.subscribe` 管"换了连接后重新认领"。

| Transport | 关联机制 |
| --- | --- |
| InProcess | 隐式 —— 客户端直接持有 `<-chan`，无歧义 |
| Wails IPC | 隐式 —— 单 WebView ↔ 宿主连接 |
| HTTP | 客户端生成 **connection id**；`runs.start` 等 POST 用 `Lyra-Connection-Id` header 带上；SSE 订阅用 `GET /v1/rpc/stream?conn=<id>` query 带上（`EventSource` 无法设自定义 header，故走 query —— 仍是 transport URL 层，不进 envelope）。Server 把该连接启动 + 该连接 `runs.subscribe` 过的 run 的 notification 路由到匹配 `conn` 的 SSE 流。 |

**首次连接的回放起点（消除 SSE 建立 vs `runs.start` 的竞态）**：run 的事件从
**run 起点**起一直可缓冲/重放（§3.3）。一条流式订阅（首次 SSE、或 `runs.subscribe`）
**不带 `Last-Event-Id` 时，server 从该 run 的第一个事件（`eventId` 起点）重放**，
不是"只发此刻之后"。因此 client **不必** 卡"先建 SSE 再 `runs.start`"的顺序 ——
无论谁先落地，`RUN_STARTED` 等头部事件都不会丢。

### 3.3 续传 + 崩溃恢复

Client 断线 → 重连 → 重发 Request（或重开 SSE）带 `Last-Event-Id` → Server
回放该资源 id 内 `eventId > lastEventId` 的事件。

- **重放窗口绑 run 生命周期**：run 未结束其事件一直可 replay；run 结束后保留 30s。
- 续传只在**同一资源 id**（runId / taskId）内有效，不跨资源。
- **崩溃恢复（丢了 runId，新连接）**：客户端重启后 ① `runs.list { sessionId }`
  （§5.2）发现仍在跑 / 等待输入的 run（`RunSummary`：runId + status +
  lastEventId）→ ② 对每个要恢复的 runId 调 `runs.subscribe { runId }`（§5.2）把它
  绑到**新连接**（默认路由只会发给原启动连接，§3.2，故必须显式认领）→ ③ 带上
  `RunSummary.lastEventId` 作 `Last-Event-Id` 续传（或不带，从起点重放）。这是
  durable HITL（plan-mode 暂停跨重启恢复）的完整闭环。`runs.list` **只返活跃 /
  等待态**，不返已完成的 run（历史走 `messages.list`）。
- 超窗口后客户端从 `messages.list` 拉历史补全已落库消息；进行中 run 的非消息
  态（in-flight tool / reasoning / state）恢复见 §11。

### 3.4 各 transport 物理形态

| Transport | 流物理形态 |
| --- | --- |
| InProcess | `<-chan Notification`（Go channel） |
| Wails IPC | `EventsEmit` + 前端 AsyncIterator |
| HTTP | SSE（`text/event-stream`），每条 SSE event 的 `data:` 是一条 Notification 的 JSON |

---

## 4. AG-UI Events

事件**总是**作为 `notifications/run/event` 的 `params.event` 出现。schema 与
`frontend/src/protocol/agui/` 一致；非 Go 客户端从 `schemas/events.yaml` 生成。

### 4.1 AG-UI 标准事件

| 分组 | 事件 |
| --- | --- |
| 生命周期 | `RUN_STARTED` / `RUN_FINISHED` / `RUN_ERROR` |
| Step | `STEP_STARTED` / `STEP_FINISHED` |
| 文本 | `TEXT_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` |
| 工具调用 | `TOOL_CALL_START` / `_ARGS` / `_END` / `_CHUNK` / `_RESULT` |
| 推理 | `REASONING_MESSAGE_START` / `_CONTENT` / `_END` / `_CHUNK` + `THINKING_TEXT_MESSAGE_*` |
| 共享状态 | `STATE_SNAPSHOT` / `STATE_DELTA`（RFC 6902 JSON Patch） |
| 历史 | `MESSAGES_SNAPSHOT` |
| Per-message 活动 | `ACTIVITY_SNAPSHOT` / `ACTIVITY_DELTA` |
| 扩展 | `CUSTOM` / `RAW` |

### 4.2 Lyra CUSTOM 事件（`event.type === "CUSTOM"`，按 `event.name` 分发）

| `name` | Payload 类型 | 用途 |
| --- | --- | --- |
| `lyra.plan` | `{ items: PlanItem[] }` | 替换 `state.plan` |
| `lyra.plan-block` | `{ messageId }` | 挂 plan content block |
| `lyra.code-proposal` | `{ parentMessageId, lang, file, text }` | Diff 提案 block |
| `lyra.search-results` | `{ parentMessageId, results }` | 搜索结果 block |
| `lyra.approval` | `ApprovalRequest`（§6.9） | HITL 审批请求（server→client） |
| `lyra.approval-result` | `ApprovalResult`（§6.9） | HITL 决策回执 |
| `lyra.question` | `QuestionRequest`（§6.9） | 澄清式提问（server→client，§4.4） |
| `lyra.question-result` | `QuestionResult`（§6.9） | 提问已应答回执 |
| `lyra.telemetry` | 自由形态 | 性能 / 调试信号 |

预留（kimi-code / agent-chat-ui 启发）：`lyra.interrupt` / `lyra.resume` /
`lyra.checkpoint` / `lyra.meta` / `lyra.subagent.*` / `lyra.background.*` /
`lyra.compaction.*`。

所有 CUSTOM 事件 payload 必须有 Zod schema（`frontend/src/protocol/agui/schemas.ts`）
+ Go mirror（`schemas/events.yaml` 生成）。

> 命名：server→client 的"请求用户输入"事件 payload 用 `XxxRequest`
> （`ApprovalRequest` / `QuestionRequest`），其回执用 `XxxResult`。client→
> server 提交决策的 **method 参数**用 `<Verb><Noun>Request`
> （`SubmitApprovalRequest` / `AnswerQuestionRequest`，§8.1）—— 两者分属事件层
> 与方法层，不冲突。

### 4.3 HITL 审批 —— Notification + 后续 Request

1. Server 在 `notifications/run/event` 里推 `lyra.approval`（`ApprovalRequest`，
   含 `requestId`、待执行工具入参 `args?`、可选 `expiresAt`）。
2. 前端渲染审批 block，用户点「允许」/「拒绝」（可选：改参后再允许）。
3. Client 发 Request `runs.approval.submit`（参数 `SubmitApprovalRequest`）。
4. Server 校验后继续 run，在流里推 `lyra.approval-result`（`ApprovalResult`）。

`decision` 仅 `"approve" | "deny"`（两值闭集）。**协议层适配的两点表达力**
（对标 Agent SDK `canUseTool`）：

- `editedArgs?` —— **改参后批准**：客户端改写工具入参再放行（如把命令限域到
  sandbox）。缺省 = 按 `ApprovalRequest.args` 原样执行。
- `reason`（`deny` 时）—— **回灌给 agent**：拒绝理由进 agent 上下文，它可据此
  换方案（不只给人看）。

「记住选择」/「永久允许」等**策略语义不进协议** —— 由前端 UI 表达，wire 层只看
`approve` / `deny`（+ 可选 `editedArgs`）。

**超时（定死，避免前后端对不齐）**：`ApprovalRequest` / `QuestionRequest` 带
`expiresAt?` + `onTimeout?: "deny" | "abort"`（**默认 `"deny"`**）。到点未回 →
server 按 `onTimeout` 收尾并推 `*-result`：`deny` = 当作拒绝、run 继续（agent 可
换方案，可恢复，故为默认）；`abort` = 整个 run 以 `stopReason="canceled"` 结束。
不给 `expiresAt` = 永不超时（run 一直 waiting，靠客户端应答或 `runs.cancel`）。

**HITL 能力协商（避免 run 挂死，§6.1）**：client 通过 `capabilities.events.custom`
**声明能渲染/应答哪些 HITL 事件**（含 `lyra.approval` / `lyra.question` 即表示
支持对应 HITL）。Server **必须不向不支持的 client 发起对应 HITL** —— 改走默认
策略（如 auto-deny / 不进入需审批模式），**绝不把 run 永久挂在 waiting**。这是
协议级约束，不是实现细节。

**为什么不用 server → client request**：那会逼每种 transport 实现真双向 RPC
（HTTP 上得开反向 SSE），复杂度跟收益不成正比。

### 4.4 澄清式提问 —— 同 HITL 的 Notification + 后续 Request

agent 遇多解任务（"用哪个数据库 / 要不要 X"）时**主动抛带选项的问题**，而非塞进
自由文本让客户端无法结构化渲染。流程与审批完全平行：

1. Server 推 `lyra.question`（`QuestionRequest { requestId, questions }`）。
2. 前端把每题渲染成单选/多选卡片（kernel 已有 `AskUserQuestion` 工具可复用）。
3. Client 发 Request `runs.question.answer`（参数 `AnswerQuestionRequest`）。
4. Server 把答案灌回 agent 上下文继续 run，并推 `lyra.question-result`。

`Question` / `Answers` 形状见 §6.9。`expiresAt` / `onTimeout` / HITL 能力协商
（§6.1）语义同审批（§4.3）。每题可带 `allowFreeText?: boolean` —— 为 `true` 时
client 在固定 `options` 外再给一个自由输入框；此时 `AnswerQuestionRequest.answers`
的 value 可以是 option 的 `label`，也可以是用户的自由文本（不是 "Other" 字样，
是文本本身）。**与审批共享同一条 "Notification + Request" 通道**，不引入
server→client request。

---

## 5. Methods

### 5.1 方法命名约定（method 名本身）

- `<domain>.<verb>` / `<domain>.<resource>.<verb>`，camelCase verb
- 复数 noun（`sessions` / `messages`），单数 verb
- 流式方法名以 `.start` / `.subscribe` 结尾
- **点在 method 名里有语义、永远保留**：HTTP 上 `runs.start` → `/v1/rpc/runs.start`，
  **严禁斜杠化**（`/v1/rpc/runs/start` 会跟 REST 混淆、引诱 REST shadow 蔓延）
- **`workspace.*` 只读查询豁免 verb**：`filesChanged` / `diff` / `grep` / `fileHead`
  / `projects` / `skills` / `agentDocs` 用 **资源名**作 method（"取某资源"的快捷读，
  对标 `git diff` / `git grep` 的命令命名），不强加 `list`/`get` 前缀。带副作用的
  workspace 方法仍用 verb（`selectProject` / `mcp.reconnect`）。

> **参数 / 结果的类型名**遵循 §8.1 的 `<Verb><Noun>Request` / `Response` 规则
> （与后端 Go SSOT 对齐）。

### 5.2 方法表

**Returns 约定**：`T` 单对象/标量；`T[]` 裸数组（非分页 —— 集合天然有界）；
`Page<T>` 游标分页（§6.4）；`Stream<R, E>` 立即返 `R`（即 method 的 Response）+
`E` 是**流出的 notification params 信封类型**（不是裸 AG-UI 事件 —— 如 run 流的
`E = RunEvent`，AG-UI 事件在 `RunEvent.event` 里，§3.1）；`void` 无返回（typed 上
`null` / 空 object，不返哨兵）。"参数类型"列给该 method 的 params/result 类型名
（§8.1 规则）。流式方法的 `R`：主创建方法（`runs.start`）给命名 Response
（`StartRunResponse`）；纯订阅方法（`*.subscribe`）的 `R` 是 trivial id 回显
（`{runId}` / `{taskId}`），按 §8.1 不单独命名。

| Method | Returns | 参数类型 / 关键字段 |
| --- | --- | --- |
| **Runtime** | | |
| `runtime.initialize` | `InitializeResponse` | `InitializeRequest` |
| `runtime.shutdown`（notify） | `void` | `ShutdownRequest` |
| `runtime.ping` | `void` | — （HTTP 上推荐用 sidecar `/v1/health`，§9） |
| **Runs** | | |
| `runs.start` | `Stream<StartRunResponse, RunEvent>` | `StartRunRequest`（§6.3，含 `maxTurns?` / `maxBudgetUsd?`）；事件经 `notifications/run/event`（params=`RunEvent`），终态经 `run/closed.result` |
| `runs.list` | `RunSummary[]` | `{ sessionId }` —— 仅活跃 / 等待态的 run（崩溃恢复 + durable HITL 发现，§3.3） |
| `runs.subscribe` | `Stream<{runId}, RunEvent>` | `{ runId }` —— 把一个**已存在的** run（非本连接启动的）的事件流重新绑到当前连接（崩溃恢复认领，§3.2/§3.3）。带 `Last-Event-Id` 续传，否则从起点重放 |
| `runs.cancel` | `void` | `CancelRunRequest { runId, reason? }`（与 `notifications/canceled` 不同，§2.3） |
| `runs.approval.submit` | `void` | `SubmitApprovalRequest { requestId, decision, editedArgs?, reason? }`（§4.3 / §6.9） |
| `runs.question.answer` | `void` | `AnswerQuestionRequest { requestId, answers }`（§4.4 / §6.9） |
| **Sessions** | | |
| `sessions.list` | `Page<Session>` | `PageQuery` |
| `sessions.get` | `Session` | `{ id }` |
| `sessions.create` | `Session` | `CreateSessionRequest { title?, model?, metadata? }` |
| `sessions.update` | `Session` | `UpdateSessionRequest { id, title?, pinned?, archived?, metadata? }` |
| `sessions.delete` | `void` | `{ id }` |
| `sessions.fork` | `Session` | `ForkSessionRequest { parentId, atMessageId }` —— `parentId` 是被 fork 的源 |
| `sessions.export` | `ExportSessionResponse { url, expiresAt }` | `ExportSessionRequest { id, format: "md"\|"json" }`（url 走 transport 文件通道，§6.8） |
| **Messages** | | |
| `messages.list` | `Page<Message>` | `ListMessagesRequest { sessionId, …PageQuery }` |
| `messages.edit` | `EditMessageResponse { runId, checkpoint }` | `EditMessageRequest { sessionId, messageId, content }` —— 新 run 推 `lyra.checkpoint`。**`capabilities.features.checkpoints=false` 时此 method 返 `-32009 capability_not_negotiated`**（不返空 checkpoint） |
| **Workspace**（下列读类方法都作用于 **active project**，见表后注） | | |
| `workspace.filesChanged` | `FileChange[]` | — |
| `workspace.diff` | `DiffRow[]` | `{ path }` —— 结构化 row（§6.5） |
| `workspace.fileHead` | `FileLine[]` | `{ path }` —— 结构化 line（§6.5） |
| `workspace.grep` | `GrepResult` | `{ query }` |
| `workspace.terminal.subscribe` | `Stream<{runId}, TerminalOutput>` | `{ runId }`；输出经 `notifications/terminal/output`（params=`TerminalOutput`，§6.5） |
| `workspace.projects` | `Project[]` | — （`Project.active` 标当前 active） |
| `workspace.selectProject` | `void` | `{ id }` —— 切换 active project（影响后续所有 `workspace.*` 读方法） |
| `workspace.mcp.list` | `MCPServer[]` | — （`MCPServer.toolCount` 给数量；详情用下方 mcp.tools） |
| `workspace.mcp.tools` | `ToolSpec[]` | `{ name }` —— 展开某 MCP server 的具体工具（list 页只给数量，详情按需拉） |
| `workspace.mcp.reconnect` | `void` | `{ name }` —— MCP 原生 name 作唯一标识 |
| `workspace.agentDocs` | `AgentDoc[]` | — —— 级联发现的 AGENTS.md **正文**（sidecar `/v1/info` 只给 path+size，业务读取走这里） |
| `workspace.skills` | `Skill[]` | — |
| **Providers / Models / Tools** | | |
| `providers.list` | `Provider[]` | — |
| `providers.test` | `ProviderTestResult` | `{ id }` |
| `providers.configure` | `Provider` | `ConfigureProviderRequest { id, apiKey?, baseUrl? }` —— 配置 provider 凭据/端点（返更新后的 `Provider`，`hasApiKey` 反映结果）。配置 ≠ 鉴权，是 Runtime 的 provider 管理 |
| `models.list` | `Model[]` | `{ provider? }` |
| `tools.list` | `ToolSpec[]` | — |
| `tools.invoke` | `InvokeToolResponse { output }` | `InvokeToolRequest { name, arguments }` —— 不经 LLM 直接调一个工具（诊断 / 工作流） |
| **Memory**（LYRA.md 长期记忆） | | |
| `memory.list` | `MemoryEntry[]` | — —— 所有 scope（天然有界） |
| `memory.get` | `GetMemoryResponse { scope, content }` | `GetMemoryRequest { scope }` |
| `memory.update` | `void` | `UpdateMemoryRequest { scope, content }` —— `features.memory=false`（如 SQLite 模式）时返 `-32009` |
| **Attachments** | | |
| `attachments.createUploadUrl` | `CreateUploadURLResponse` | `CreateUploadURLRequest { filename, mime, size }`；走 §5.4 binary 通道 |
| `attachments.delete` | `void` | `{ id }` |
| **Background** | | |
| `background.list` | `BackgroundTask[]` | — |
| `background.stop` | `void` | `{ taskId }` |
| `background.subscribe` | `Stream<{taskId}, BackgroundUpdate>` | `{ taskId }`；输出经 `notifications/background/update` |
| **Feedback** | | |
| `feedback.submit` | `void` | `FeedbackRequest { kind, refId, value? }` |

> **Project 维度**：`workspace.diff/grep/fileHead/filesChanged` 不带 `projectId`，
> 一律作用于 **active project**（`workspace.projects` 里 `active:true` 的那个）。
> 切换用 `workspace.selectProject { id }`。多 project 下这避免了每个方法都塞
> `projectId` 的噪音，代价是 active 是连接级隐式状态（切换后旧读结果可能 stale，
> client 切 project 后应 refetch）。

**反向不变量**：

- ❌ 非分页 list 不套 `{items}` wrapper（`tools.list` 等返裸数组）。`T[]` →
  `Page<T>` 本就是 breaking change，无「零破坏升级」路径，不预投资
- ❌ 内部存储类型不泄漏到 wire（`Session.metadata` 是 `Record<string, unknown>`
  而非 string-only，即使后端内部存窄类型）
- ❌ `void` 服务端返 `null` 或空 object，不返 `true` / `1` 哨兵

### 5.3 服务端发出的 Notification

每条 `params` 带对应资源 id 做流过滤（§3.1–3.2）。

| Notification | 何时 | params |
| --- | --- | --- |
| `notifications/run/event` | run 期间每个 AG-UI 事件 | `RunEvent { runId, eventId, ts, parentToolUseId?, event }` |
| `notifications/run/closed` | run 流结束 | `{ runId, result: RunResult }`（§6.3） |
| `notifications/background/update` | background 状态变化 | `BackgroundUpdate { taskId, eventId, status, progress?, outputDelta? }` |
| `notifications/terminal/output` | `terminal.subscribe` 后的 pty 输出 | `TerminalOutput { runId, eventId, line }`（§6.5） |

> `notifications/canceled` 是**客户端→服务端**发的（§2.3 取消在飞 Request），
> 不在此表。HTTP 上服务端对它的确认是 `204 No Content`，不是一条 notification。

### 5.4 Attachments 二进制上传

Multipart binary 不进 JSON-RPC envelope（协议里**唯一**例外）：

1. Client 发 `attachments.createUploadUrl`（`CreateUploadURLRequest`）→
   `CreateUploadURLResponse { uploadUrl, attachmentId, expiresAt }`。
2. 走 transport 各自的二进制通道：HTTP `PUT <uploadUrl>` 字节流 / Wails native
   binding 传 `Uint8Array` / InProcess 直接传 `[]byte`。
3. 后续用 `attachmentId` 在其他 method 里引用。

---

## 6. Shapes

### 6.1 Capabilities

```ts
interface ServerCapabilities {
  events: { standard: string[]; custom: string[] };   // 发出的事件
  features: {
    multimodal: boolean; reasoning: boolean; checkpoints: boolean;
    interrupts: boolean; background: boolean; subagents: boolean;
    skills: boolean; mcp: boolean; sessionExport: boolean;
    attachments: { enabled: boolean; maxSizeBytes?: number; mimeTypes?: string[] };
  };
  providers: string[];
  limits: { maxMessagesPerSession?: number; maxConcurrentRuns?: number };
}

interface ClientCapabilities {
  events: { standard: string[]; custom: string[] };   // 渲染得了的事件
  features: { multimodal?: boolean; markdown?: boolean; /* … */ };
}
```

> ServerCapabilities.features 另有 `memory?: boolean` —— 为 `false` 时 LYRA.md
> 不可写（如 SQLite 模式），`memory.update` 返 `-32009`（§5.2）。

未声明的 feature 默认 `false`。Server **必须不发** client 没声明能渲染的事件
（但 client 必须忽略未知字段以保前向兼容，§8.2）。

**HITL 能力即 `events.custom` 的成员**：client 在 `events.custom` 含 `lyra.approval`
/ `lyra.question` 表示支持对应 HITL；不含则 server 不得对它发起该 HITL、改走默认
策略（§4.3 防挂死规则）。不另设 `features.hitl` 位 —— HITL 本质是"能不能渲染并
回传这个 CUSTOM 事件"，`events.custom` 已精确表达。

### 6.2 核心对话形状

```ts
type SessionStatus = "running" | "waiting" | "idle";

interface Session {
  id: string;
  title: string;
  status: SessionStatus;
  model: string;
  createdAt: string;             // ISO-8601
  updatedAt: string;
  lastMessageAt?: string;
  metadata: Record<string, unknown>;  // 任意 JSON，不约束 string-only
  pinned?: boolean;
  archived?: boolean;
  usage?: Usage;                 // 本 session 累计用量/成本（§6.3）；list 可省、get 可带
}

interface Message {
  id: string;
  sessionId: string;
  role: "user" | "assistant" | "system" | "tool" | "developer";
  content?: string;
  attachments?: string[];        // attachmentId[]，回放多模态历史消息（multimodal）
  toolCalls?: ToolCall[];
  toolCallId?: string;
  createdAt: string;
  metadata?: Record<string, unknown>;
}

interface ToolCall { id: string; name: string; arguments: string /* JSON-encoded */ }

// JSON Schema draft 2020-12。明确类型让 codegen 客户端能做参数校验。
type JsonSchema = Record<string, unknown>;

interface ToolSpec {
  name: string;
  description?: string;
  parameters: JsonSchema;
  origin: "server" | "client" | "mcp";
  safetyClass?: string;          // 危险度标记（前端标红/需审批用），对齐后端 Tool.SafetyClass
  // 服务端可选附加字段；客户端忽略未知字段（前向兼容）
}

// Discriminated union —— 新 kind 扩 union，禁止 { kind, data:{…} } 嵌套
type ContextItem =
  | { kind: "file"; path: string }                          // 相对 project root（见下）
  | { kind: "url"; url: string }                            // Runtime 会 fetch（见下，有 SSRF 面）
  | { kind: "selection"; path: string; range: [number, number] } // [起始行, 结束行]，1-based，含端
  | { kind: "image"; attachmentId: string };
```

**ContextItem 语义（安全相关，定死在协议层）**：

- `file.path` / `selection.path` —— **相对 project root**（非 cwd）。越界路径
  （`../` 逃逸出 root）Runtime 必须拒绝。
- `url` —— **Runtime 会主动 fetch 该 URL** 作为上下文。这有 **SSRF 面**：本地
  loopback 部署下尤其要拦内网 / 元数据地址（`169.254.*` / `localhost` 私网段
  等）。Runtime 必须做 egress 白/黑名单，**不能盲目 fetch 任意 url**。
- `selection.range` —— `[startLine, endLine]`，**1-based、闭区间**（行号，不是
  字符偏移）。

### 6.3 Run 启动 / 终态 / 用量

```ts
interface StartRunRequest {
  sessionId: string;
  runId?: string;                // 建议客户端自带 UUIDv7；缺省 server 生成。**幂等键**：
                                 // 重提同一 runId（网络重试）→ server 不新起 run，
                                 // 而是把现有 run 的流当 runs.subscribe 重新接上（§3.2/§5.2）。
                                 // 这是协议唯一的幂等机制（无独立 Idempotency-Key header）。
  messages: Message[];           // history + 新一轮
  state?: Record<string, unknown>;
  tools?: ToolSpec[];
  context?: ContextItem[];
  model?: string;
  mode?: "agent" | "chat" | "plan";
  attachments?: string[];        // attachmentId（来自 createUploadUrl）
  maxTurns?: number;             // 工具循环轮数上限；触顶 → stopReason="max_turns"
  maxBudgetUsd?: number;         // 成本上限（含子 agent subtree）；触顶 → stopReason="max_budget"
  params?: GenerationParams;     // 生成调参（高级设置）
}

// LLM 生成参数（对齐后端 chat.Options）。收进子对象而非平铺顶层。
interface GenerationParams {
  temperature?: number;
  maxTokens?: number;            // chat.Options.MaxTokens
  maxOutputTokens?: number;      // chat.Options.MaxOutputTokens
  topP?: number;
  stop?: string[];
}

interface StartRunResponse { runId: string }   // notifications/run/event 用此 id 关联

// run 终态 —— notifications/run/closed.result。一次读全停止原因 + 计量。
interface RunResult {
  stopReason: "completed" | "canceled" | "error" | "max_turns" | "max_budget";
  usage?: Usage;                 // 累计用量（含子 agent subtree）
  costUsd?: number;              // 累计成本；模型不在定价表时省略（不臆造 0）
  turns?: number;                // 工具循环轮数
  error?: { code: number; message: string; data?: ProblemData }; // stopReason="error"；复用 §7 错误形状
}

interface Usage {
  inputTokens: number;
  outputTokens: number;
  reasoningTokens?: number;
  cacheReadTokens?: number;      // 缓存读/写拆分（Runtime 已细到这一层）
  cacheWriteTokens?: number;
  byModel?: Record<string, { inputTokens: number; outputTokens: number; costUsd?: number }>;
}

// runs.list 返回项 —— 崩溃恢复 / durable HITL 发现（§3.3）。只列活跃 / 等待态。
interface RunSummary {
  runId: string;
  status: "running" | "waiting"; // waiting = 卡在 HITL 审批/提问/plan-mode，可重连恢复
  startedAt: string;
  lastEventId?: string;          // 重开流时作 Last-Event-Id 续传锚点
}
```

> `RunSummary.status`（仅 `running | waiting`，runs.list 不返终态）与
> `SessionStatus`（`…| idle`，会话静息态）**有意不同**，别"对齐"成一个 enum。

### 6.4 分页

```ts
interface PageQuery { limit?: number /* default 20, max 100 */; cursor?: string }
interface Page<T> { items: T[]; nextCursor?: string; hasMore: boolean }
```

**用** `Page<T>`：`sessions.list` / `messages.list`（集合可能很大）。
**不用**：`tools.list` / `providers.list` / `models.list` / `workspace.*` /
`background.list`（天然有界，返裸数组）。

### 6.5 Workspace 数据形状

```ts
interface FileChange { path: string; change: "add" | "mod" | "del"; added: number; removed: number }

// 结构化 diff row —— 服务端解析一次，前端不再 regex
type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "ctx";  l: number; r: number; code: string }
  | { type: "add";  r: number; code: string }
  | { type: "del";  l: number; code: string };

interface FileLine { ln: string /* 行号或 "···" */; code: string /* 服务端 highlight 的 HTML */; muted?: boolean }
interface GrepMatch { path: string; match: string /* 预渲染 snippet */ }
interface GrepResult { matches: GrepMatch[]; total: number /* 可能 > matches.length；将来 additive 加 nextCursor? */ }

type TermLineKind = "prompt" | "cmd" | "out" | "err" | "warn" | "mute" | "ok";
interface TermLine { kind: TermLineKind; text: string }
// terminal.subscribe 流出的 notification params 信封（带 eventId 续传锚点）
interface TerminalOutput { runId: string; eventId: string; line: TermLine }

interface Project { id: string; name: string; branch: string; active?: boolean }

// MCP server —— `name` 是 MCP 协议原生唯一标识（如 "filesystem" / "github"）。
// 不带 id / displayName / icon（UI presentation 不进 wire，客户端按 name 映射图标）。
// `toolCount` 只给数量（list 页用）；具体工具走 workspace.mcp.tools（§5.2）。
interface MCPServer { name: string; desc: string; toolCount: number; status: "active" | "idle" | "error" }

interface Skill { id: string; name: string; description: string }

interface AgentDoc { path: string; content: string }   // workspace.agentDocs（AGENTS.md 正文）
```

### 6.6 Provider / Model

```ts
interface Provider { id: string; type: string; baseUrl?: string; hasApiKey: boolean /* 仅 boolean，不暴露 key */ }
interface ProviderTestResult { ok: boolean; detail?: string }
// providers.configure 参数；返更新后的 Provider
interface ConfigureProviderRequest { id: string; apiKey?: string; baseUrl?: string }

interface Model {
  id: string;
  provider: string;
  contextWindow?: number;
  description?: string;
  // 来自 model catalog（纯投影，全可选）：
  maxOutputTokens?: number;
  pricing?: { inputPerMTokens: number; outputPerMTokens: number; cacheReadPerMTokens?: number; cacheWritePerMTokens?: number };
  capabilities?: { tools?: boolean; vision?: boolean; reasoning?: boolean };
}
```

### 6.7 Background

```ts
type BackgroundStatus = "running" | "stopped" | "succeeded" | "failed";
interface BackgroundTask { taskId: string; label: string; status: BackgroundStatus; startedAt: string; progress?: number /* 0..1 */ }
// background.subscribe 流出的 notification params 信封（带 eventId 续传锚点）
interface BackgroundUpdate { taskId: string; eventId: string; status: BackgroundStatus; progress?: number; outputDelta?: string }
```

### 6.8 Attachments / Feedback

```ts
interface CreateUploadURLRequest { filename: string; mime: string; size: number }
interface CreateUploadURLResponse { uploadUrl: string; attachmentId: string; expiresAt: string }

// sessions.export —— 导出文件不进 JSON-RPC envelope，走 transport 的文件通道
// （同 attachments §5.4 的对偶下行）。HTTP：`url` 是同 origin 的短期下载路径，
// 带本地门禁 token（若启用）、`expiresAt` 后失效；InProcess/Wails：`url` 是
// file:// 或 native binding 句柄。
interface ExportSessionResponse { url: string; expiresAt: string }

type FeedbackKind = "thumbs-up" | "thumbs-down" | "note" | "bookmark";
interface FeedbackRequest { kind: FeedbackKind; refId: string /* message/run id */; value?: string }
```

### 6.9 HITL approval / 澄清提问

```ts
type ApprovalDecision = "approve" | "deny";

type OnTimeout = "deny" | "abort"; // 默认 "deny"（可恢复），§4.3

// server→client（lyra.approval 事件 payload）
interface ApprovalRequest {
  requestId: string;             // 业务 id（区别于取消用的 envelope `id`，§2.3）
  parentMessageId: string;
  text: string;
  command?: string;
  args?: Record<string, unknown>; // 待执行工具入参（editedArgs 的基准）
  reason?: string;
  risk?: string;
  expiresAt?: string;            // ISO-8601；缺省 = 永不超时
  onTimeout?: OnTimeout;         // 到 expiresAt 未回的收尾策略，默认 "deny"
}
// client→server（runs.approval.submit 参数）
interface SubmitApprovalRequest {
  requestId: string;
  decision: ApprovalDecision;
  editedArgs?: Record<string, unknown>; // approve-with-modified-args（§4.3）
  reason?: string;                       // deny 时回灌给 agent
}
// server→client（lyra.approval-result 事件 payload）
interface ApprovalResult { requestId: string; decision: ApprovalDecision }

interface Question {
  id: string;                    // 稳定 key（answers 按此索引，不用题干）
  question: string;
  header: string;                // 短标签（≤ 12 字符）
  options: { label: string; description: string; preview?: string }[]; // 2-4 项
  multiSelect: boolean;
  allowFreeText?: boolean;       // true = options 外另给自由输入框（§4.4）
}
// server→client（lyra.question 事件 payload）
interface QuestionRequest {
  requestId: string;
  parentMessageId: string;
  questions: Question[];
  expiresAt?: string;            // 同 ApprovalRequest
  onTimeout?: OnTimeout;         // 同 ApprovalRequest，默认 "deny"
}
// client→server（runs.question.answer 参数）；answers: question.id → 选中 label
interface AnswerQuestionRequest { requestId: string; answers: Record<string, string | string[]> }
// server→client（lyra.question-result 事件 payload）
interface QuestionResult { requestId: string }
```

### 6.10 id 唯一性作用域

影响 client 去重 / 索引 / 续传，定死：

| id | 唯一性 | 生成方 |
| --- | --- | --- |
| `sessionId` / `runId` / `messageId` / `attachmentId` / `taskId` | **全局唯一**（跨 session/连接） | server（`runId` 可 client 自带，§6.3） |
| `eventId` | **单 runId 内**单调递增（不跨 run） | server |
| `requestId`（HITL approval/question） | **单 run 内**唯一 | server |
| envelope `id`（§1.1） | **单连接内**唯一 | client |

`feedback.submit { refId }`（§6.8）只给 `refId` 不给 sessionId —— 正因 messageId /
runId 全局唯一，refId 自包含可定位。

### 6.11 会话与 run 的并发

- 一个**连接**可操作多个 `Session`（§1.2）。
- 一个 **`Session` 同一时刻只有一个活跃 run**（串行）—— `SessionStatus`（§6.2）
  是单值即反映这点；客户端不应对同 session 并发 `runs.start`（server 可拒
  `-32011` 或排队，实现自选，但**协议层语义是串行**）。
- `capabilities.limits.maxConcurrentRuns`（§6.1）是 **runtime 全局**（跨 session）
  的并发 run 上限，不是单 session 内的。

### 6.12 Memory / Tool 直接调用

```ts
type MemoryScope = "project" | "user";   // project=<cwd>/LYRA.md / user=~/.lyra/LYRA.md
interface MemoryEntry { scope: MemoryScope; content: string; capturedAt: string /* ISO-8601 */ }
interface GetMemoryRequest    { scope: MemoryScope }
interface GetMemoryResponse   { scope: MemoryScope; content: string }
interface UpdateMemoryRequest { scope: MemoryScope; content: string }

// tools.invoke —— 不经 LLM 直接调一个工具
interface InvokeToolRequest  { name: string; arguments: string /* JSON-encoded，同 ToolCall.arguments */ }
interface InvokeToolResponse { output: string }
```

---

## 7. 错误

### 7.0 两条错误投递通道（流式场景必读）

`runs.start` **立即返成功**（`{runId}`），所以 **run 执行期**的失败不可能再走那条
已返回的 Response。错误分两类、走两条路：

| 通道 | 何时 | 怎么投递 |
| --- | --- | --- |
| (a) **method 调用失败** | 调用本身就错：`session_not_found` / `invalid_params` / `capability_not_negotiated` / 未握手 / `run_not_found` 等 | 该 method 的 **JSON-RPC error response**（同步返回，含 §7.2 的 code） |
| (b) **run 执行期失败** | run 已起、执行中出错：`provider_error` / `provider_rate_limited` / `tool_failed` | **`RUN_ERROR` 事件** + **`run/closed` 的 `RunResult{ stopReason:"error", error }`**（§6.3），`error.code` 复用同一套 §7.2 code |

同一个 `-32xxx` code 表两条通道都可能用（如 `tool_failed`：同步工具调用失败走 (a)，
run 内工具失败走 (b)）。**`approval_required` 不再作为 error code** —— 它是 run 的
状态，已由"run 停在 `waiting` + `onTimeout`"（§4.3）表达，故从下表移除。

### 7.1 error.code 范围

| 范围 | 含义 |
| --- | --- |
| -32700 ~ -32603 | JSON-RPC 标准（parse / invalid request / method not found / invalid params / internal） |
| -32000 ~ -32099 | Lyra 业务错误（§7.2） |

### 7.2 业务错误码

| code | message | 何时 |
| --- | --- | --- |
| -32001 | `provider_error` | LLM provider 非 2xx 或超时 |
| -32002 | `provider_rate_limited` | provider 返 429 |
| -32003 | `tool_failed` | tool 执行 throw |
| -32005 | `session_not_found` | sessionId 不存在 |
| -32006 | `message_not_found` | messageId 不存在 |
| -32007 | `run_not_found` | runId 不存在或已结束 |
| -32008 | `attachment_too_large` | 超 `maxSizeBytes` |
| -32009 | `capability_not_negotiated` | 调了 initialize 时 client 没声明的能力 |
| -32010 | `invalid_protocol_version` | 版本协商失败 |
| -32011 | `protocol_violation` | 不符合协议的消息（batch / 未握手就调业务 / URL method ≠ body method） |

`error.data` 推荐 RFC 7807 ProblemDetails 形状：

```ts
interface ProblemData { type?: string; detail?: string; retryAfterMs?: number; errors?: Array<{ path: string; message: string }> }
```

### 7.3 HTTP status 映射

HTTP 是 transport，**业务错误一律走 `error.code`，不映射 HTTP status**。HTTP
status 仅反映 transport 层：

| status | 何时 | Body |
| --- | --- | --- |
| **200** | JSON-RPC Response 正常返回（含业务 error） | JSON-RPC envelope |
| **204** | Notification 已接收 | 空 |
| **400** | body 非合法 JSON / 非 JSON-RPC envelope / `jsonrpc` ≠ `"2.0"` | error envelope（`-32700` / `-32600`） |
| **404** | URL path 不存在（如 `/v1/rpc/runs.unknownMethod`） | error envelope（`-32601`） |
| **409** | URL method ≠ body method | error envelope（`-32011`） |
| **401** | 本地进程门禁 token 缺失/错 | **扁平 JSON** `{ "error": "missing_local_token" }`（非 envelope） |
| **500** | Runtime panic / 未捕获错误 | **扁平 JSON** `{ "error": "internal", "traceId": "…" }` |
| **503** | `/v1/health` unhealthy | **扁平 JSON**（§9.2） |

核心原则：JSON-RPC error envelope 与 4xx **可共存**（404 + envelope 是
method-not-found 的双重信号）；**401 / 500 / 503 不走 envelope**（transport 层
故障，请求可能没到 router，无法构造合理 `id`）；sidecar 端点（§9）也不走 envelope。

---

## 8. Schema SSOT + 命名约定 + 版本

真 Runtime 后端在 **lynx 仓 `lyra/`**（本仓 `internal/agui/` 只是 dev mock）。
行为 SSOT 是后端的 Go interface（已 ship）：

```
lyra/rpc/protocol/*.go                # Go interface + struct tags —— 行为 SSOT
   ├─ (codegen) → schemas/methods.yaml  → TS  → frontend/src/rpc/ 类型
   └─ (codegen) → schemas/events.yaml   → TS  → frontend/src/protocol/agui/ 类型
```

Go ↔ Go（in-process / Wails）两端直接 import `rpc/protocol`，**不需要 codegen**。
TS / Rust / Python client 从 schema 生成（codegen 管线待接，当前前端类型手写并
以此 Go interface 为准对齐）。

### 8.1 类型命名约定（前后端必须一致）

类型名以后端 Go interface 为 SSOT，**doc 与生成的 TS 同名、零映射层**：

- **method 的参数 / 结果类型** = `<Verb><Noun>Request` / `<Verb><Noun>Response`，
  verb·noun 取自 method 名：
  - `runs.start` → `StartRunRequest` / `StartRunResponse`
  - `messages.edit` → `EditMessageRequest` / `EditMessageResponse`
  - `sessions.create` → `CreateSessionRequest`；`sessions.fork` → `ForkSessionRequest`
  - `runs.approval.submit` → `SubmitApprovalRequest`；`runs.question.answer` → `AnswerQuestionRequest`
  - `attachments.createUploadUrl` → `CreateUploadURLRequest` / `CreateUploadURLResponse`
  - 结果为 `void` 的 method 不定义 `*Response`；参数极简（如 `{ id }`）可不单独命名。
- **域实体**用裸 noun：`Session` / `Message` / `Run*` / `ToolCall` / `ToolSpec` /
  `Provider` / `Model` / `MCPServer` / `BackgroundTask` / `RunResult` / `RunSummary` / `Usage` …
- **CUSTOM 事件 payload**（server→client）：请求用户输入用 `XxxRequest`
  （`ApprovalRequest` / `QuestionRequest`），回执用 `XxxResult`
  （`ApprovalResult` / `QuestionResult`）。
- **缩写大小写跟 Go**：`URL`（非 `Url`）、`ID` 在 Go 内部，wire JSON 字段仍是
  camelCase（`uploadUrl` / `attachmentId`）。

> 旧草案的 `*Params` / `*Result` / `*Submission` 命名已废，统一到上表。前端
> 手写类型（`frontend/src/rpc/shapes.ts` 等）按此对齐（codegen 接上后自动同名）。

### 8.2 版本规则

- `protocolVersion` 是日期串 `"2026-MM-DD"`。
- **前向兼容是硬约定，无需 capability 开关**：客户端**必须忽略未知字段**、对
  未知 method 容忍（不崩）。因此加 method / 加可选字段 / 加事件 → **同版本号**。
- 改语义 / 删字段 / 改字段类型 → 新日期版本 + 协商。
- `initialize` 是唯一拒绝版本不匹配硬断开的方法。

---

## 9. Sidecar Endpoints（仅 HTTP transport）

> 存在的唯一理由：oncall 一条 `curl` 看清 Runtime 是否活着、版本对不对。套
> JSON-RPC envelope 就破功了。

仅 HTTP 暴露；read-only metadata only（永不暴露 sessions/messages 等业务数据）；
不走 envelope（扁平 JSON）；无鉴权（连本地门禁 token 也不要）；不参与 lifecycle
（不需先 `initialize`）。

#### `GET /v1/info`

`runtime.initialize` 结果的扁平子集（不含 client-specific 握手信息）：

```json
{
  "serverInfo": { "name": "lyra-core", "version": "0.8.1" },
  "protocolVersion": "2026-05-28",
  "capabilities": { "events": { "standard": [], "custom": [] }, "features": {}, "providers": [] }
}
```

用途：oncall 确认 server 活着 + 版本；k8s `startupProbe`；部署 smoke test。

#### `GET /v1/health`

```json
{ "status": "ok", "checks": { "storage": "ok", "providers": "ok" } }
```

`200`：`status === "ok"`；`503`：`degraded | unhealthy`。用途：k8s
liveness/readiness、nginx upstream health。

**反向不变量**：❌ 不加 `/v1/sessions/{id}` 这类业务 read shadow（业务统一走
JSON-RPC）；❌ sidecar 不暴露 PII / session / message；❌ 不复用 envelope。

---

## 10. Observability（HTTP transport 强制）

### 10.1 单一路由形态

**所有 JSON-RPC message 走唯一路径** `POST /v1/rpc/{method}`。不接受无后缀的
`POST /v1/rpc`（服务端对无后缀请求返 `404`）。

- **Notification 同样适用**：`POST /v1/rpc/notifications/canceled`，响应 `204`。
  URL 只是 observability 标签，跟有没有 `id` 正交。
- method 名照搬方法表字符串（`runs.start` → `/v1/rpc/runs.start`，点保留，禁止
  斜杠化，§5.1）。
- body 里 `method` 字段必须跟 URL 一致，否则 `409 + -32011`。

### 10.2 强制响应 header

| Header | 值 | 用途 |
| --- | --- | --- |
| `X-Lyra-Method` | 实际执行的 method 名 | 反代 access log 不解 body 即可按 method 分桶 |
| `X-Lyra-Trace-Id` | 回显 client 请求侧传入的同名 header，缺失则服务端生成 | trace correlation（一发一回同名，不再叫 Request-Id，避免与 envelope `id` / 业务 `requestId` 撞脸） |
| `X-Lyra-Server` | `<name>/<version>` | client 侧能力降级判断 |

### 10.3 结构化日志强制字段

```json
{ "ts": "…", "method": "runs.start", "id": "req-…", "duration_ms": 42,
  "error_code": null, "bytes_in": 1234, "bytes_out": 567, "trace_id": "…" }
```

`error_code` 填 JSON-RPC code（如 `-32005`），**不填业务 message**（敏感信息不
进日志）。

### 10.4 Prometheus metric

```
lyra_rpc_request_total{method="runs.start", error_code="0"}
lyra_rpc_duration_seconds{method="runs.start"}
lyra_rpc_bytes_{in,out}_total{method="runs.start"}
```

`method` 是有界 cardinality（~42 method + ~4 notification）；`error_code="0"`
表成功（即使有 JSON-RPC error，HTTP 仍是 200）。

### 10.5 Trace correlation

client 传 `X-Lyra-Trace-Id`（请求）→ 服务端缺失则生成（UUIDv7）→ 响应同名
`X-Lyra-Trace-Id` 回显 → 结构化日志 `trace_id` 绑定。未来 facade 接入时透传 W3C
`traceparent`。

**反向不变量**：❌ 业务 error 不映射 HTTP status；❌ method 名不进高 cardinality
维度（session_id / user_id 会爆 Prometheus）；❌ PII（消息 / prompt 内容）不进
access log / metric。

---

## 11. 未决问题

- **续传窗口外的非消息态恢复**：超 30s 后 `messages.list` 只补已落库消息，不含
  in-flight tool / reasoning / state。`runs.list`（§5.2）已解决"发现仍在飞的 run"，
  但重连后当前 run 的 AG-UI 中间态仍只能 `Last-Event-Id` replay；候选补
  `runs.snapshot { runId }` 返当前状态快照（Runtime 有 typed process snapshot 可投影）。
- **`messages.edit` + checkpoint 语义**：返 `{ runId, checkpoint }` 但 checkpoint /
  rewind 模型未定（Runtime capability 暂标 false）。先定 rewind 语义。
- **mid-turn steering**（`lyra.interrupt` / `lyra.resume`）：当前 steering 落到下一
  轮，非当前轮中断注入。需先定事件 + 方法形状。
- **Terminal 是否交互**：`workspace.terminal.subscribe` 当前只读（收 agent 命令
  输出）。若要做**交互式终端**（用户敲命令）则补 `workspace.terminal.input
  { runId, data }`；若只展示则现状够。待产品定位。
- **高频 delta 流控**：`TEXT_MESSAGE_CONTENT` / `TOOL_CALL_ARGS` ~30 条/秒；是否需
  可选 coalescing。当前 SSE 够用，暂不做。
- **`models.list` 缓存**：provider 返的 model list 不便宜，是否 Runtime 缓存 1h。
- **`workspace.terminal.subscribe` 多 subscriber 去重**。
- **AG-UI 是否直接复用 ag-ui-protocol 官方 schema**：倾向只复用名字、用自己的
  envelope，避免被其演进绑架。
- **派生 OpenAPI**：JSON-RPC 工具链生态小；是否生成一份**派生**（非 SSOT）OpenAPI
  给外部工具。
- **跟 MCP 的 drift 管理**：明确不跟 batch / 反向 RPC，持续 drift，是否每个 MCP
  minor 对一次差异表。

---

## 附录 A — 文件位置

> **后端列指向真 Runtime（lynx 仓 `lyra/`，已 ship）**。本仓 `internal/agui/`
> 是 Go **dev mock**（前端 CI / 本地 `wails dev` 用，在本前端仓内），不是协议的
> 权威实现。

| 关注点 | 前端（本仓 lyra） | 后端（lynx 仓 `lyra/`） |
| --- | --- | --- |
| 协议 interface（SSOT） | `frontend/src/rpc/` | `rpc/protocol/` |
| Method dispatch / router | `frontend/src/rpc/methods.ts` | `rpc/dispatch/` |
| Method 实现 | n/a | `rpc/server/` + `internal/engine/` |
| Transport（HTTP/SSE + InProcess） | `frontend/src/rpc/transports/` | `rpc/transport/{http,inprocess}/` |
| 流式 reducer / CUSTOM handler | `frontend/src/plugins/builtin/{core-reducer,agui-handlers}/` | `internal/engine/` + `internal/agui/`（事件 marshal） |
| HITL approval | `frontend/src/lib/agent/useApprovalSubmit.ts` | `internal/service/approval/`（Console/Gate） |
| dev mock（非权威，Go） | 本仓 `internal/agui/` | n/a |

---

## 附录 B — Facade pattern（未来云端，本轮不实现）

云端化预期架构：`表现层 → Facade（订阅/账号/billing/授权）→ Runtime`。

1. Facade 暴露**完整 Runtime 协议** —— 前端透明，本地/云端切换只改 transport
   endpoint，方法表不变。
2. Facade 附加私有端点（`/account` / `/billing` 等），**不在 Runtime 协议里**。
3. Facade 持有上游 provider 凭据 —— 云端模式用户不用配 key。
4. Runtime 永远不感知「上面有没有 facade」—— 同一份代码跑桌面也跑服务器。
5. 推荐：Facade 只发授权 token + Runtime endpoint，前端拿到后**直连 Runtime**
   （类似 STS / SignedUrl），Facade 不碰用户数据。

本协议**不为 facade 留任何特殊字段**。

---

## 附录 C — 与 MCP 的关系

| 点 | MCP | Lyra |
| --- | --- | --- |
| Wire envelope | JSON-RPC 2.0 | JSON-RPC 2.0 |
| 元数据 | HTTP header | transport-specific 带外通道 |
| Lifecycle | initialize / operate / shutdown | 同 |
| 取消 | 显式 `notifications/cancelled`（英式拼写） | 显式 `notifications/canceled`（US locale，唯一拼写差异） |
| 流式 | SSE + `Last-Event-Id` | 同 + InProcess/Wails 各自原生 |
| Server → client RPC | 支持（sampling / elicitation） | **不支持**（HITL 用 Notification + Request 配对） |
| Batch | 2024 支持 / 2025 移除 | **永不支持** |
| Transport | stdio / Streamable HTTP | InProcess / Wails IPC / HTTP |
| SSOT | TypeScript schema | Go `lyra/rpc/protocol` |
| Sidecar | RFC 9728 Protected Resource Metadata | 自有 `/v1/info` + `/v1/health`（更简单、无 OAuth flow） |

MCP 是 **LLM ↔ MCP Server**（让 LLM 调外部工具）；Lyra Runtime Protocol 是
**表现层 ↔ Runtime**（让 UI 驱动 agent）。两者解决不同问题、可共存：Lyra Runtime
内部可作为 MCP client 接入 MCP server，但本协议本身不是 MCP。

**刻意不跟 MCP 一致处**：不跟 RFC 9728（OAuth flow 用，我们无用户鉴权）；不跟
batch（pipelining 用并发连接）；不跟 server→client RPC（HITL 用 Notification +
Request 配对）。代价是跟 MCP 持续 drift —— 接受，因为完全 align 会让复杂度暴涨
且对 Lyra 无等价收益。
