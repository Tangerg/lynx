# Lyra Runtime Protocol

> 表现层（Web / 套壳 Web / TUI / …）与 **Lyra Runtime** 之间的通讯协议。
> Runtime 是无状态纯计算单元（AG-UI 兼容的 agent 执行引擎 + tool 调用
> + LLM provider 调用），**不感知用户、不做鉴权、不管账号**，只暴露
> 本文档定义的方法。
>
> Wire 格式参考 MCP —— **JSON-RPC 2.0 envelope**，多种 transport 共享
> 同一份语义。物理传输见 `TRANSPORT.md`。**本文档是前后端共享的单一契约
> 真相源**（CLOSED contract）：只描述协议，不记录实现进度；动任何方法 /
> shape 前后端先对齐。落地进度各自仓库自管。

---

## 0. 架构模型

**Runtime** = 实现本协议的一个进程（或同进程对象）。不知道 OS 用户是
谁、不持有任何账号 / 订阅记录、不主动连任何上层服务，只暴露本文档的
method。内部职责：会话存储 / AG-UI 事件流 / LLM provider 调用 / tool
执行 / HITL 审批。

**表现层** = 实现本协议 client 一侧 + 渲染/输入的程序。持有一个
`runtime` 句柄（in-process 是 Go 对象，跨进程是 transport endpoint），
把用户输入翻译成 method call，把 Runtime 流回的 AG-UI 事件渲染成 UI。

### 设计原则

| 原则 | 理由 |
| --- | --- |
| **JSON-RPC 2.0 envelope** | MCP 验证过的多 transport 共享方案。三种 message：Request / Response / Notification。 |
| **元数据走带外通道** | connection id / trace id / idempotency key / 流恢复 cursor 全部走 `context.Context` 或 transport header/URL，**永不进 message body**（§1.2）。 |
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
  id: number;            // 单调递增整数，同连接内不重用，绝不 null
  method: string;        // "runs.start" / "sessions.list" / …
  params?: unknown;
}

interface Response {
  jsonrpc: "2.0";
  id: number;            // 匹配 Request.id
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

`id` 锁死为整数（JSON-RPC 2.0 允许 `string | number`，本协议收紧到
`number`）：客户端用单调递增 integer，服务端不接 string id —— 少一个
union 分支、少一组解析路径。

**协议明确不做**：

- ❌ JSON-RPC batch —— pipelining 用并发连接解决（MCP 2025-06-18 也删了）
- ❌ stdio transport —— Lyra 无此需求
- ❌ Server → Client request —— 双向 RPC 让每种 transport 都得实现反向
  通道。HITL 用「Notification + 后续 Request」配对实现（§4.3）
- ❌ `id` 用字符串

### 1.2 元数据走带外通道

JSON-RPC envelope **只装** `{jsonrpc, id, method, params}` / result / error /
notification 字段。其余元信息按 transport 各取一种带外通道：

| 元数据 | InProcess | Wails IPC | HTTP |
| --- | --- | --- | --- |
| **Connection ID**¹ | Go `context.Context` | message metadata | `Lyra-Connection-Id` header（POST）/ `?conn=` query（SSE，见 §3.2） |
| Trace ID | context | metadata | `Lyra-Trace-Id` header |
| Idempotency key | context | metadata | `Idempotency-Key` header |
| Protocol version | 编译期保证 | metadata | `Lyra-Protocol-Version` header |
| Last-Event-Id（流恢复） | n/a | metadata | `Last-Event-Id` header（SSE 自动）|
| 本地进程门禁 token² | n/a | n/a | `Authorization: Bearer <token>` |

¹ **Connection ID**（旧文档叫 "Session ID"，已改名消歧）标识一个**传输
连接**，不是聊天 `Session`。它把客户端发出的 Request 与该客户端的
notification 流（§3.2）关联起来。一个连接上可以同时操作多个聊天
`Session`。

² **本地进程门禁** ≠ 用户鉴权。仅当 Web 表现层连 loopback HTTP 时，阻止
同机其他进程乱调。token 写在 `~/.lyra/local-token`（chmod 600），启动时
随机生成，无过期 / refresh / 用户绑定。

---

## 2. Lifecycle

```
[Connect] → runtime.initialize → operate → runtime.shutdown → [Disconnect]
```

握手完成前 Runtime 拒绝任何业务方法。

### 2.1 `runtime.initialize`

```ts
// → Runtime
interface InitializeParams {
  protocolVersion: string;                 // 日期串 "2026-05-28"
  clientInfo: { name: string; version: string };
  capabilities: ClientCapabilities;        // §6.1
}

// ← Result
interface InitializeResult {
  protocolVersion: string;
  serverInfo: { name: string; version: string };
  capabilities: ServerCapabilities;        // §6.1
}
```

版本协商（同 MCP）：client 报想用的版本 → server 支持则回同版本，否则
回自己支持的最新版本 → client 若无法 fall back 必须断开。`initialize`
是**唯一**因版本不匹配硬断开的方法。

### 2.2 `runtime.shutdown`（Notification）

```ts
// → Runtime（无响应）
interface ShutdownParams { reason?: string }
```

Runtime 收到后停接新 Request、用 `notifications/run/closed`（status:
`"cancelled"`）终止进行中的 run、关 transport。

### 2.3 取消必须显式

两个**语义不同**的取消，不要混：

| 取消对象 | 机制 | 说明 |
| --- | --- | --- |
| 在飞的 JSON-RPC **Request**（如慢 `sessions.list`） | `notifications/cancelled { requestId, reason? }`（client→server） | 取消单个还没返回的 Request |
| long-running **run** | `runs.cancel { runId, reason? }`（正经 Request，§5.2） | 停止一个 run，流以 `run/closed` status `"cancelled"` 收尾 |

Runtime 必须区分「网络抖断」与「主动取消」：仅 transport 断开时按
client 短暂离线处理，run 继续跑、缓冲事件等续传；**只有**显式信号才真停。

---

## 3. Streaming

### 3.1 请求 / 响应模型

流式方法（`runs.start` / `workspace.terminal.subscribe` /
`background.subscribe`）返回流而非单一结果：

1. Client 发 Request（如 `runs.start`）。
2. Server **立即回 Response**，含**资源唯一 id**（`runs.start` 返 `runId`，
   `background.subscribe` 返 `taskId`），不等流结束。
3. Server 通过 Notification 推事件，每条带资源 id + 单调递增 `eventId`：
   ```ts
   interface RunEventNotification {
     runId: string;            // 关联回 runs.start
     eventId: string;          // 单调递增，Last-Event-Id 续传 key
     ts: string;               // 服务端权威时间戳 ISO-8601
     parentToolUseId?: string; // 子 agent 归属：该事件属于哪个 tool-use 派生的子 agent
     event: AgUiEvent;         // §4
   }
   ```
   `parentToolUseId` 缺省 = 主 agent 的事件；存在 = 某次 tool-use 派生的子
   agent（同步 task / 后台 subagent）产生的事件，客户端据此归到对应轨道
   （对标 Agent SDK 的 `parent_tool_use_id`）。
4. 流结束时 server 发 `notifications/run/closed { runId, result }`，`result`
   是结构化终态（§6.3 `RunResult`）：停止原因 + 用量 + 成本 + 轮数。
   **终态 + 计量从这里一次读全，不靠解析末事件、不靠扫 `lyra.telemetry`
   流**（与 `background/update` 对齐）。
5. 客户端用资源 id（runId / taskId）过滤属于自己的事件。

**为什么不用第二个 streamHandle id**：每个流式方法已有自己的唯一资源
id，再加一个是冗余 —— 流过滤直接按资源 id。

### 3.2 连接关联（哪条流收哪个 run 的事件）

notification 流是**按连接**投递的：run R 的事件发给「启动 R 的那条
连接」。各 transport 的关联机制：

| Transport | 关联机制 |
| --- | --- |
| InProcess | 隐式 —— 客户端直接持有 `<-chan`，无歧义 |
| Wails IPC | 隐式 —— 单 WebView ↔ 宿主连接 |
| HTTP | 客户端生成 **connection id**；`runs.start` 等 POST 用 `Lyra-Connection-Id` header 带上；SSE 订阅用 `GET /v1/rpc/stream?conn=<id>` 的 **query 参数**带上（浏览器 `EventSource` 无法设自定义 header，故走 query —— 仍是 transport URL 层，不进 envelope）。Server 把该连接启动的 run 的 notification 路由到匹配 `conn` 的 SSE 流。 |

### 3.3 续传

Client 断线 → 重连 → 重发 Request（或重开 SSE）带 `Last-Event-Id` →
Server 回放该资源 id 内 `eventId > lastEventId` 的事件。

- **重放窗口绑 run 生命周期**：run 未结束，其事件一直可 replay；run 结束
  后保留 30s。
- 续传只在**同一资源 id**（runId / taskId）内有效，不跨资源。
- **崩溃恢复（丢了 runId）**：客户端重启后用 `runs.list { sessionId }`（§5.2）
  发现会话里**仍在飞 / 等待审批**的 run（含其 `runId` + 状态），再按 runId
  重开流续传。这是 durable HITL（plan-mode 暂停跨重启恢复）的发现入口 ——
  Runtime 侧已能把 `Waiting` 的 run 快照持久化并恢复，缺的就是这个发现 + 重连
  协议。
- 超窗口后客户端从 `messages.list` 拉历史补全已落库的消息；进行中
  run 的非消息态（in-flight tool / reasoning / state）恢复见 §11 未决。

### 3.4 各 transport 物理形态

| Transport | 流物理形态 |
| --- | --- |
| InProcess | `<-chan Notification`（Go channel） |
| Wails IPC | `EventsEmit` + 前端 AsyncIterator |
| HTTP | SSE（`text/event-stream`），每条 SSE event 的 `data:` 是一条 Notification 的 JSON |

---

## 4. AG-UI Events

事件**总是**作为 `notifications/run/event` 的 `params.event` 出现。schema
与 `frontend/src/protocol/agui/` 一致；非 Go 客户端从 `schemas/events.yaml`
生成。

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

| `name` | Payload | 用途 |
| --- | --- | --- |
| `lyra.plan` | `{ items: PlanItem[] }` | 替换 `state.plan` |
| `lyra.plan-block` | `{ messageId }` | 挂 plan content block |
| `lyra.code-proposal` | `{ parentMessageId, lang, file, text }` | Diff 提案 block |
| `lyra.search-results` | `{ parentMessageId, results }` | 搜索结果 block |
| `lyra.approval` | `{ requestId, parentMessageId, text, command, args?, reason, risk?, expiresAt?, … }` | HITL 审批请求 |
| `lyra.approval-result` | `{ requestId, decision }` | HITL 决策回执 |
| `lyra.question` | `{ requestId, parentMessageId, questions: Question[] }` | 澄清式提问（§4.4） |
| `lyra.question-result` | `{ requestId }` | 提问已应答回执 |
| `lyra.telemetry` | 自由形态 | 性能 / 调试信号 |

预留（kimi-code / agent-chat-ui 启发）：`lyra.interrupt` / `lyra.resume` /
`lyra.checkpoint` / `lyra.meta` / `lyra.subagent.*` / `lyra.background.*` /
`lyra.compaction.*`。

所有 CUSTOM 事件 payload 必须有 Zod schema
（`frontend/src/protocol/agui/schemas.ts`）+ Go mirror
（`schemas/events.yaml` 生成）。

### 4.3 HITL 审批 —— Notification + 后续 Request

1. Server 在 `notifications/run/event` 里推 `lyra.approval`（含 `requestId`，
   可选 `args`（待执行的工具入参）、可选 `expiresAt`）。
2. 前端渲染审批 block，用户点「允许」/「拒绝」（可选：改参后再允许）。
3. Client 发 Request `runs.approval.submit { requestId, decision, editedArgs?, reason? }`。
4. Server 校验后继续 run，在流里推 `lyra.approval-result`。

`decision` 仅 `"approve" | "deny"`（两值闭集）。**协议层适配的两点表达力**
（对标 Agent SDK `canUseTool`）：
- `editedArgs?` —— **改参后批准**：客户端可改写工具入参再放行（如把命令限域
  到 sandbox）。缺省 = 按 `lyra.approval.args` 原样执行。
- `reason`（`deny` 时）—— **回灌给 agent**：拒绝理由进 agent 上下文，它可据此
  换方案（不只是给人看）。

「记住选择」/「永久允许」等**策略语义不进协议** —— 由前端 UI 选项表达，wire
层只看 `approve` / `deny`（+ 可选 `editedArgs`）。审批超过 `expiresAt`（若给）
未回，server 按默认策略（deny/abort）收尾并推 `lyra.approval-result`。

**为什么不用 server → client request**：那会逼每种 transport 实现真双向
RPC（HTTP 上得开反向 SSE）。复杂度跟收益不成正比。

### 4.4 澄清式提问 —— 同 HITL 的 Notification + 后续 Request

agent 遇到多解任务（"用哪个数据库 / 要不要 X"）时**主动抛带选项的问题**，而非
塞进自由文本让客户端无法结构化渲染。流程与审批完全平行：

1. Server 在 `notifications/run/event` 里推 `lyra.question { requestId, questions }`。
2. 前端把每题渲染成单选/多选卡片（kernel 已有 `AskUserQuestion` 工具可复用）。
3. Client 发 Request `runs.question.answer { requestId, answers }`。
4. Server 把答案灌回 agent 上下文继续 run，并推 `lyra.question-result`。

```ts
interface Question {
  question: string;                              // 题干
  header: string;                                // 短标签（≤ 12 字符）
  options: { label: string; description: string; preview?: string }[]; // 2-4 项
  multiSelect: boolean;
}
// answers: 题干 → 选中 label（multiSelect 为 label[]；自由文本直接用文本）
type Answers = Record<string, string | string[]>;
```

`expiresAt` / 默认应答语义同审批。**与审批共享同一条"Notification + Request"
通道形态** —— 不引入 server→client request。

---

## 5. Methods

### 5.1 命名约定

- `<domain>.<verb>` / `<domain>.<resource>.<verb>`，camelCase verb
- 复数 noun（`sessions` / `messages`），单数 verb
- 流式方法名以 `.start` / `.subscribe` 结尾
- **点在 method 名里有语义、永远保留**：HTTP 上 `runs.start` →
  `/v1/rpc/runs.start`，**严禁斜杠化**（`/v1/rpc/runs/start` 会跟 REST
  风格混淆、引诱 REST shadow 蔓延）

### 5.2 方法表

**Returns 约定**：`T` 单对象/标量；`T[]` 裸数组（非分页 —— 集合天然
有界）；`Page<T>` 游标分页（§6.4）；`Stream<R, E>` 立即返 `R` + 事件经
notification 流出；`void` 无返回（typed 上 `null` / 空 object，不返哨兵）。

| Method | Returns | 关键参数 |
| --- | --- | --- |
| **Runtime** | | |
| `runtime.initialize` | `InitializeResult` | `{ protocolVersion, clientInfo, capabilities }` |
| `runtime.shutdown`（notify） | `void` | `{ reason? }` |
| `runtime.ping` | `void` | — （HTTP 上推荐用 sidecar `/v1/health`，§9） |
| **Runs** | | |
| `runs.start` | `Stream<{runId}, AgUiEvent>` | `StartRunParams`（§6.3，含可选 `maxTurns?` / `maxBudgetUsd?`）；事件经 `notifications/run/event`，终态经 `notifications/run/closed.result` |
| `runs.list` | `RunSummary[]` | `{ sessionId }` —— 会话内仍在飞 / 等待审批的 run（崩溃恢复 + durable HITL 发现入口，§3.3） |
| `runs.cancel` | `void` | `{ runId, reason? }` —— 停止 run（与 `notifications/cancelled` 不同，§2.3） |
| `runs.approval.submit` | `void` | `{ requestId, decision: "approve"\|"deny", editedArgs?, reason? }`（§4.3） |
| `runs.question.answer` | `void` | `{ requestId, answers }`（§4.4） |
| **Sessions** | | |
| `sessions.list` | `Page<Session>` | `PageQuery` |
| `sessions.get` | `Session` | `{ id }` |
| `sessions.create` | `Session` | `{ title?, model?, metadata? }` |
| `sessions.update` | `Session` | `{ id, title?, pinned?, archived?, metadata? }` |
| `sessions.delete` | `void` | `{ id }` |
| `sessions.fork` | `Session` | `{ parentId, atMessageId }` —— `parentId` 是被 fork 的源 |
| `sessions.export` | `{ url }` | `{ id, format: "md"\|"json" }` |
| **Messages** | | |
| `messages.list` | `Page<Message>` | `{ sessionId, …PageQuery }` |
| `messages.edit` | `MessageEditResult` | `{ sessionId, messageId, content }` —— 字段名 `content`；新 run 推 `lyra.checkpoint` |
| **Workspace** | | |
| `workspace.filesChanged` | `FileChange[]` | — |
| `workspace.diff` | `DiffRow[]` | `{ path }` —— 结构化 row（§6.5） |
| `workspace.fileHead` | `FileLine[]` | `{ path }` —— 结构化 line（§6.5） |
| `workspace.grep` | `GrepResult` | `{ query }` |
| `workspace.terminal.subscribe` | `Stream<{runId}, TermLine>` | `{ runId }`；输出经 `notifications/terminal/output` |
| `workspace.projects` | `Project[]` | — |
| `workspace.mcp.list` | `MCPServer[]` | — |
| `workspace.mcp.reconnect` | `void` | `{ name }` —— MCP 原生 name 作唯一标识 |
| `workspace.skills` | `Skill[]` | — |
| **Providers / Models / Tools** | | |
| `providers.list` | `Provider[]` | — |
| `providers.test` | `ProviderTestResult` | `{ id }` |
| `models.list` | `Model[]` | `{ provider? }` |
| `tools.list` | `ToolSpec[]` | — |
| **Attachments** | | |
| `attachments.createUploadUrl` | `CreateUploadUrlResult` | `{ filename, mime, size }`；走 §5.4 binary 通道 |
| `attachments.delete` | `void` | `{ id }` |
| **Background** | | |
| `background.list` | `BackgroundTask[]` | — |
| `background.stop` | `void` | `{ taskId }` |
| `background.subscribe` | `Stream<{taskId}, BackgroundUpdate>` | `{ taskId }`；输出经 `notifications/background/update` |
| **Feedback** | | |
| `feedback.submit` | `void` | `{ kind, refId, value? }` |

**反向不变量**：

- ❌ 非分页 list 不套 `{items}` wrapper（`tools.list` 等返裸数组）。
  `T[]` → `Page<T>` 本就是 breaking change，无「零破坏升级」路径，不预投资
- ❌ 内部存储类型不泄漏到 wire（`Session.metadata` 是
  `Record<string, unknown>` 而非 string-only，即使后端内部存窄类型）
- ❌ `void` 服务端返 `null` 或空 object，不返 `true` / `1` 哨兵

### 5.3 服务端发出的 Notification

每条 `params` 带对应资源 id 做流过滤（§3.1–3.2）。

| Notification | 何时 | params 关键字段 |
| --- | --- | --- |
| `notifications/run/event` | run 期间每个 AG-UI 事件 | `{ runId, eventId, ts, parentToolUseId?, event }` |
| `notifications/run/closed` | run 流结束 | `{ runId, result: RunResult }`（§6.3 —— stopReason + usage + cost + turns） |
| `notifications/background/update` | background 状态变化 | `{ taskId, eventId, status, progress?, outputDelta? }` |
| `notifications/terminal/output` | `terminal.subscribe` 后的 pty 输出 | `{ runId, eventId, line }` |

> `notifications/cancelled` 是**客户端→服务端**发的（§2.3 取消在飞
> Request），不在此表。HTTP 上服务端对它的确认是 `204 No Content`，不是
> 一条 notification。

### 5.4 Attachments 二进制上传

Multipart binary 不进 JSON-RPC envelope（协议里**唯一**例外）：

1. Client 发 `attachments.createUploadUrl { filename, mime, size }` →
   `{ uploadUrl, attachmentId, expiresAt }`。
2. 走 transport 各自的二进制通道：HTTP `PUT <uploadUrl>` 字节流 / Wails
   native binding 传 `Uint8Array` / InProcess 直接传 `[]byte`。
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

未声明的 feature 默认 `false`。Server **必须不发** client 没声明能渲染的
事件（但 client 必须忽略未知字段以保前向兼容，§8.1）。

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
}

interface Message {
  id: string;
  sessionId: string;
  role: "user" | "assistant" | "system" | "tool" | "developer";
  content?: string;
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
  // 服务端可选附加字段；客户端忽略未知字段（前向兼容）
}

// Discriminated union —— 新 kind 扩 union，禁止 { kind, data:{…} } 嵌套
type ContextItem =
  | { kind: "file"; path: string }
  | { kind: "url"; url: string }
  | { kind: "selection"; path: string; range: [number, number] }
  | { kind: "image"; attachmentId: string };

type ApprovalDecision = "approve" | "deny";
interface ApprovalSubmission { requestId: string; decision: ApprovalDecision; reason? : string }

interface MessageEditResult {
  runId: string;                 // 编辑触发的新 run
  checkpoint: string;            // opaque checkpoint id（前端只回传，不解析）
}
```

### 6.3 Run 启动参数

```ts
interface StartRunParams {
  sessionId: string;
  runId?: string;                // 建议客户端自带 UUIDv7（幂等，§11）；缺省 server 生成
  messages: Message[];           // history + 新一轮
  state?: Record<string, unknown>;
  tools?: ToolSpec[];
  context?: ContextItem[];
  model?: string;
  mode?: "agent" | "chat" | "plan";
  attachments?: string[];        // attachmentId from createUploadUrl
  maxTurns?: number;             // 工具循环轮数上限；触顶 → run/closed.result.stopReason="max_turns"
  maxBudgetUsd?: number;         // 成本上限（含子 agent subtree）；触顶 → stopReason="max_budget"
}

interface StartRunResult { runId: string }   // notifications/run/event 用此 id 关联

// run 终态 —— notifications/run/closed.result。一次读全停止原因 + 计量。
interface RunResult {
  stopReason: "completed" | "cancelled" | "error" | "max_turns" | "max_budget";
  usage?: Usage;                 // 累计用量（含子 agent subtree）
  costUsd?: number;              // 累计成本；模型不在定价表时省略（不臆造 0）
  turns?: number;                // 工具循环轮数
  error?: { code: number; message: string }; // stopReason="error" 时
}

interface Usage {
  inputTokens: number;
  outputTokens: number;
  reasoningTokens?: number;
  cacheReadTokens?: number;      // 缓存读/写拆分（Runtime 已细到这一层）
  cacheWriteTokens?: number;
  byModel?: Record<string, { inputTokens: number; outputTokens: number; costUsd?: number }>;
}

// runs.list 返回的轻量摘要 —— 崩溃恢复 / durable HITL 发现（§3.3）
interface RunSummary {
  runId: string;
  status: "running" | "waiting" | "done";   // waiting = 卡在 HITL 审批/提问，可重连恢复
  startedAt: string;
  lastEventId?: string;          // 重开流时作 Last-Event-Id 续传锚点
}
```

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

interface Project { id: string; name: string; branch: string; active?: boolean }

// MCP server —— `name` 是 MCP 协议原生唯一标识（如 "filesystem" / "github"）。
// 不带 id / displayName / icon（UI presentation 不进 wire，客户端按 name 映射图标）。
interface MCPServer { name: string; desc: string; tools: number; status: "active" | "idle" | "error" }

interface Skill { id: string; name: string; description: string }
```

### 6.6 Provider / Model

```ts
interface Provider { id: string; type: string; baseUrl?: string; hasApiKey: boolean /* 仅 boolean，不暴露 key */ }
interface ProviderTestResult { ok: boolean; detail?: string }
interface Model { id: string; provider: string; contextWindow?: number; description?: string }
```

### 6.7 Background

```ts
type BackgroundStatus = "running" | "stopped" | "succeeded" | "failed";
interface BackgroundTask { taskId: string; label: string; status: BackgroundStatus; startedAt: string; progress?: number /* 0..1 */ }
interface BackgroundUpdate { taskId: string; status: BackgroundStatus; progress?: number; outputDelta?: string }
```

### 6.8 Attachments / Feedback

```ts
interface CreateUploadUrlResult { uploadUrl: string; attachmentId: string; expiresAt: string }

type FeedbackKind = "thumbs-up" | "thumbs-down" | "note" | "bookmark";
interface FeedbackInput { kind: FeedbackKind; refId: string /* message/run id */; value?: string }
```

---

## 7. 错误

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
| -32004 | `approval_required` | run 卡 HITL 且 client 未在 `expiresAt` 前回 |
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

HTTP 是 transport，**业务错误一律走 `error.code`，不映射 HTTP status**。
HTTP status 仅反映 transport 层：

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
method-not-found 的双重信号）；**401 / 500 / 503 不走 envelope**（transport
层故障，请求可能没到 router，无法构造合理 `id`）；sidecar 端点（§9）也
不走 envelope。

---

## 8. Schema SSOT + 版本

```
pkg/coreapi/*.go                      # Go interface + struct tags —— SSOT
   ├─ go-jsonrpc  → schemas/methods.yaml  → jsonrpc-ts  → frontend/src/lib/runtime-types.ts
   └─ go-asyncapi → schemas/events.yaml   → asyncapi-ts → frontend/src/lib/events.ts
```

Go ↔ Go（in-process / Wails）两端直接 import `pkg/coreapi`，**不需要
codegen**。TS / Rust / Python client 从 schema 生成。

### 8.1 版本规则

- `protocolVersion` 是日期串 `"2026-MM-DD"`。
- 加 method / 加可选字段 / 加事件 → 同版本号（client 声明
  `unknownMethodTolerance: true` 时）。客户端**必须忽略未知字段**。
- 改语义 / 删字段 → 新日期版本 + 协商。
- `initialize` 是唯一拒绝版本不匹配硬断开的方法。

---

## 9. Sidecar Endpoints（仅 HTTP transport）

> 存在的唯一理由：oncall 一条 `curl` 看清 Runtime 是否活着、版本对不
> 对。套 JSON-RPC envelope 就破功了。

仅 HTTP 暴露；read-only metadata only（永不暴露 sessions/messages 等业务
数据）；不走 envelope（扁平 JSON）；无鉴权（连本地门禁 token 也不要）；
不参与 lifecycle（不需先 `initialize`）。

#### `GET /v1/info`

`runtime.initialize.result` 的扁平子集（不含 client-specific 握手信息）：

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

**反向不变量**：❌ 不加 `/v1/sessions/{id}` 这类业务 read shadow（业务统
一走 JSON-RPC）；❌ sidecar 不暴露 PII / session / message；❌ 不复用
envelope。

---

## 10. Observability（HTTP transport 强制）

### 10.1 单一路由形态

**所有 JSON-RPC message 走唯一路径** `POST /v1/rpc/{method}`。不接受无
后缀的 `POST /v1/rpc`（服务端对无后缀请求返 `404`）。

- **Notification 同样适用**：`POST /v1/rpc/notifications/cancelled`，响应
  `204`。URL 只是 observability 标签，跟有没有 `id` 正交。
- method 名照搬方法表字符串（`runs.start` → `/v1/rpc/runs.start`，点保留，
  禁止斜杠化，§5.1）。
- body 里 `method` 字段必须跟 URL 一致，否则 `409 + -32011`。

### 10.2 强制响应 header

| Header | 值 | 用途 |
| --- | --- | --- |
| `X-Lyra-Method` | 实际执行的 method 名 | 反代 access log 不解 body 即可按 method 分桶（响应侧回显） |
| `X-Lyra-Request-Id` | 回显 client 的 trace id，缺失则服务端生成 | trace correlation |
| `X-Lyra-Server` | `<name>/<version>` | client 侧能力降级判断 |

### 10.3 结构化日志强制字段

```json
{ "ts": "…", "method": "runs.start", "id": "req-…", "duration_ms": 42,
  "error_code": null, "bytes_in": 1234, "bytes_out": 567, "trace_id": "…" }
```

`error_code` 填 JSON-RPC code（如 `-32005`），**不填业务 message**（敏感
信息不进日志）。

### 10.4 Prometheus metric

```
lyra_rpc_request_total{method="runs.start", error_code="0"}
lyra_rpc_duration_seconds{method="runs.start"}
lyra_rpc_bytes_{in,out}_total{method="runs.start"}
```

`method` 是有界 cardinality（~32 method + ~5 notification）；`error_code="0"`
表成功（即使有 JSON-RPC error，HTTP 仍是 200）。

### 10.5 Trace correlation

client 传 `X-Lyra-Trace-Id` → 服务端缺失则生成（UUIDv7）→ 响应
`X-Lyra-Request-Id` 回显 → 结构化日志 `trace_id` 绑定。未来 facade 接入时
透传 W3C `traceparent`。

**反向不变量**：❌ 业务 error 不映射 HTTP status；❌ method 名不进高
cardinality 维度（session_id / user_id 会爆 Prometheus）；❌ PII（消息 /
prompt 内容）不进 access log / metric。

---

## 11. 未决问题

- **续传窗口外的非消息态恢复**：超 30s 后 `messages.list` 只补已落库消息，
  不含 in-flight tool / reasoning / state。`runs.list`（§5.2）已解决"发现仍在
  飞的 run"，但重连后**当前 run 的 AG-UI 中间态**（已开但未完的 tool/reasoning
  block）仍只能从 `Last-Event-Id` replay；候选补 `runs.snapshot { runId }` 返
  当前状态快照（Runtime 已有 typed process snapshot 可投影）。
- **`messages.edit` + checkpoint 语义**：协议返 `{ runId, checkpoint }` 但
  checkpoint/rewind 模型未定（Runtime capability 暂标 false）。先定 rewind 语义。
- **mid-turn steering**（`lyra.interrupt` / `lyra.resume`）：当前 steering 落到
  下一轮，非当前轮中断注入。需先定事件 + 方法形状。
- **高频 delta 流控**：`TEXT_MESSAGE_CONTENT` / `TOOL_CALL_ARGS` 每秒 ~30
  条；是否需要可选 coalescing。当前 SSE 够用，暂不做。
- **`models.list` 缓存**：provider 返的 model list 不便宜，是否 Runtime
  缓存 1h。
- **`workspace.terminal.subscribe` 多 subscriber 去重**。
- **AG-UI 是否直接复用 ag-ui-protocol 官方 schema**：倾向只复用名字、用
  自己的 envelope，避免被其演进绑架。
- **派生 OpenAPI**：JSON-RPC 工具链生态小；是否生成一份**派生**（非 SSOT）
  OpenAPI 给外部工具。
- **跟 MCP 的 drift 管理**：明确不跟 batch / 反向 RPC，持续 drift，是否每
  个 MCP minor 对一次差异表。

---

## 附录 A — 文件位置

| 关注点 | 前端 | 后端 |
| --- | --- | --- |
| 流式 reducer（event → state） | `frontend/src/plugins/builtin/core-reducer/` | `internal/agui/events.go` |
| CUSTOM 事件 handler | `frontend/src/plugins/builtin/agui-handlers/` | `internal/agui/dsl.go` |
| JSON-RPC client | `frontend/src/rpc/` | `pkg/rpcadapter/`（待建） |
| Method 实现 | n/a | `pkg/coreapi/` + `pkg/coreimpl/`（待建） |
| HITL gateway | `frontend/src/domain/gateways/PermissionGateway.ts` + `infra/http/HttpPermissionGateway.ts` | `internal/agui/permissions.go` |
| Base URL / runtime handle | `frontend/src/main/` | `internal/agui/server.go` |
| Sidecar | n/a | `pkg/httpserver/sidecar.go`（待建） |

---

## 附录 B — Facade pattern（未来云端，本轮不实现）

云端化预期架构：`表现层 → Facade（订阅/账号/billing/授权）→ Runtime`。

1. Facade 暴露**完整 Runtime 协议** —— 前端透明，本地/云端切换只改
   transport endpoint，方法表不变。
2. Facade 附加私有端点（`/account` / `/billing` 等），**不在 Runtime 协议
   里**。
3. Facade 持有上游 provider 凭据 —— 云端模式用户不用配 key。
4. Runtime 永远不感知「上面有没有 facade」—— 同一份代码跑桌面也跑服务器。
5. 推荐：Facade 只发授权 token + Runtime endpoint，前端拿到后**直连
   Runtime**（类似 STS / SignedUrl），Facade 不碰用户数据。

本协议**不为 facade 留任何特殊字段**。

---

## 附录 C — 与 MCP 的关系

| 点 | MCP | Lyra |
| --- | --- | --- |
| Wire envelope | JSON-RPC 2.0 | JSON-RPC 2.0 |
| 元数据 | HTTP header | transport-specific 带外通道 |
| Lifecycle | initialize / operate / shutdown | 同 |
| 取消 | 显式 `notifications/cancelled` | 同 |
| 流式 | SSE + `Last-Event-Id` | 同 + InProcess/Wails 各自原生 |
| Server → client RPC | 支持（sampling / elicitation） | **不支持**（HITL 用 Notification + Request 配对） |
| Batch | 2024 支持 / 2025 移除 | **永不支持** |
| Transport | stdio / Streamable HTTP | InProcess / Wails IPC / HTTP |
| SSOT | TypeScript schema | Go `pkg/coreapi` |
| Sidecar | RFC 9728 Protected Resource Metadata | 自有 `/v1/info` + `/v1/health`（更简单、无 OAuth flow） |

MCP 是 **LLM ↔ MCP Server**（让 LLM 调外部工具）；Lyra Runtime Protocol 是
**表现层 ↔ Runtime**（让 UI 驱动 agent）。两者解决不同问题、可共存：Lyra
Runtime 内部可作为 MCP client 接入 MCP server，但本协议本身不是 MCP。

**刻意不跟 MCP 一致处**：不跟 RFC 9728（那是给 OAuth flow 的，我们无用户
鉴权）；不跟 batch（pipelining 用并发连接）；不跟 server→client RPC（HITL
用 Notification + Request 配对）。代价是跟 MCP 持续 drift —— 接受，因为完全
align 会让复杂度暴涨且对 Lyra 无等价收益。
