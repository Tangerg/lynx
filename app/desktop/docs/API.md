# Lyra Runtime Protocol

> Lyra Runtime 和它的"表现层"（Web / 套壳 Web / TUI / ...）之间的
> 通讯协议。Runtime 是无状态纯计算单元（AG-UI 兼容的 agent 执行
> 引擎 + tool 调用 + LLM provider 调用），**不感知用户、不做鉴权、
> 不管账号**，只暴露本文档定义的方法。表现层通过这些方法驱动
> Runtime。
>
> Wire 格式参考 MCP（Model Context Protocol）—— JSON-RPC 2.0
> envelope + 多种 transport 共享同一份语义。

---

## 0. 架构模型

### 0.1 Runtime 一句话定义

**Lyra Runtime = "实现本协议的一个进程（或同进程对象）"。**

- 不知道 OS 用户是谁
- 不持有任何用户 / 账号 / 订阅记录
- 不主动连任何 facade / 上层服务
- 只暴露本文档定义的 method
- 内部职责：会话存储 / AG-UI 事件流 / LLM provider 调用 /
  tool 执行 / HITL 审批管理

### 0.2 表现层一句话定义

**表现层 = "实现本协议 client 一侧 + 渲染 / 输入的程序"。**

- 知道一个 `runtime` 句柄（in-process 时是 Go 对象，跨进程时
  是 transport endpoint）
- 把用户输入翻译成 method call
- 把 Runtime 流回来的 AG-UI 事件渲染成 UI

### 0.3 设计原则

| 原则 | 来源 / 理由 |
| --- | --- |
| **JSON-RPC 2.0 envelope** | MCP 验证过的多 transport 共享方案。Request / Response / Notification 三种 message，transport 不污染 envelope。 |
| **元数据走带外通道** | Session id / trace id / idempotency key / 流恢复 cursor 全部走 `context.Context` 或 transport header，**永不进 message body**。 |
| **取消必须显式信号** | 断 transport ≠ 取消 run。重连可续传。 |
| **能力协商必经 `initialize`** | 没握手前 Runtime 不接业务方法。版本不匹配硬断开。 |
| **AG-UI 事件作为流式语言** | 16 个标准事件 + Lyra CUSTOM 事件，封装在 JSON-RPC notification 的 `params.event` 里。 |
| **不做用户鉴权** | Runtime 协议层零 user/auth/subscription 概念。需要时由外层 facade 实现（附录 B）。 |

---

## 1. Wire 格式 — JSON-RPC 2.0

### 1.1 消息类型

```ts
type Message = Request | Response | Notification;

interface Request {
  jsonrpc: "2.0";
  id: string | number;          // 同会话内单调，绝不重用，绝不 null
  method: string;               // "runs.start" / "sessions.list" / ...
  params?: unknown;             // method-specific
}

interface Response {
  jsonrpc: "2.0";
  id: string | number;          // 匹配 Request.id
  result?: unknown;             // result 和 error 互斥
  error?: { code: number; message: string; data?: unknown };
}

interface Notification {
  jsonrpc: "2.0";
  method: string;               // "notifications/run/event" / ...
  params?: unknown;
  // 无 id
}
```

**不做的事**：
- ❌ JSON-RPC batch（MCP 2025-06-18 也删了 —— pipeline 用并发连接解决）
- ❌ stdio transport（Lyra 没这需求）
- ❌ Server → Client request（双向 RPC 让 transport 复杂化；HITL 用
  "Notification + 后续 Request" 配对实现，见 §4.3）

### 1.2 元数据走带外通道

JSON-RPC envelope **永远只装 `{jsonrpc, id, method, params}` 或
result / error / notification 字段**。其他元信息按 transport 各
取一种通道：

| 元数据 | InProcess | Wails IPC | HTTP |
| --- | --- | --- | --- |
| Session ID | Go `context.Context` | message metadata 字段 | `Lyra-Session-Id` header |
| Trace ID | context | metadata | `Lyra-Trace-Id` header |
| Idempotency key | context | metadata | `Idempotency-Key` header |
| Protocol version | 编译期保证 | metadata | `Lyra-Protocol-Version` header |
| Last-Event-Id（流恢复） | n/a | metadata | `Last-Event-Id` header |
| 本地进程门禁 token（仅 Web 前端）| n/a | n/a | `Authorization: Bearer <token>` |

**本地进程门禁** ≠ 用户鉴权。它只是 Web 前端连 loopback HTTP
时阻止同机其他进程乱调，token 写在 `~/.lyra/local-token`（chmod 600）
启动时随机生成，没有过期 / refresh / 用户绑定概念。

---

## 2. Lifecycle

### 2.1 三段式

```
[Connect] → initialize → operate → shutdown → [Disconnect]
```

握手没完成前 Runtime 拒绝任何业务方法。

### 2.2 `runtime.initialize`

```ts
// → Runtime
interface InitializeParams {
  protocolVersion: string;      // 日期串："2026-05-28"
  clientInfo: { name: string; version: string };
  capabilities: ClientCapabilities;
}

// ← Result
interface InitializeResult {
  protocolVersion: string;      // 必须 client 支持的版本
  serverInfo: { name: string; version: string };
  capabilities: ServerCapabilities;
}
```

**版本协商**（抄 MCP 规则）：
- Client 报想用的版本
- Server 支持 → 回同一版本；不支持 → 回 server 支持的最新版本
- Client 拿到不能 fall back 到的版本 → 必须断开

### 2.3 `runtime.shutdown` (Notification)

```ts
// → Runtime  (no response expected)
interface ShutdownParams {
  reason?: string;
}
```

Runtime 收到后停接新 Request、把进行中的 run 用
`notifications/cancelled` 终止、关 transport。

### 2.4 取消必须显式

```ts
// → Runtime  (Notification)
"notifications/cancelled" { requestId: string|number; reason?: string }
```

Runtime 必须区分"网络抖断"和"主动取消"。仅 transport 断开时按
"client 短暂离线"处理，run 继续跑、缓冲事件等续传；显式收到
cancelled notification 才真停。

---

## 3. Streaming

### 3.1 流式方法的请求 / 响应模型

某些 method（主要是 `runs.start` / `workspace.terminal.subscribe` /
`background.subscribe`）返回流而不是单一结果：

1. Client 发 Request（如 `runs.start`）
2. Server **立即回 Response**，包含 `runId` + `streamHandle`，
   **不等流结束**
3. Server 通过 Notification 推 event：
   ```ts
   "notifications/run/event" {
     streamHandle: string;
     eventId: string;          // 单调递增，用作 Last-Event-Id resume key
     event: AgUiEvent;          // 见 §4
   }
   ```
4. 流结束时 server 发 `notifications/run/closed { streamHandle, reason }`
5. Client 想取消用 `notifications/cancelled { requestId: <run request id> }`

### 3.2 每种 transport 上的物理形态

| Transport | 流物理形态 |
| --- | --- |
| InProcess | `<-chan Notification`（Go channel） |
| Wails IPC | `EventsEmit` + 前端 AsyncIterator |
| HTTP | SSE (`text/event-stream`)，每条 SSE event 是一个 Notification 的 JSON |

### 3.3 续传

Client 断线 → 重连 → 重新发 Request（带 `lastEventId` 参数 OR
`Last-Event-Id` header） → Server 回放该 stream 里 `eventId >
lastEventId` 的事件。

- **重放窗口：30s**。超时后 client 要从 `messages.list` 拉历史补全
- 续传只在同一个 `streamHandle` 内有效，不跨 run

---

## 4. AG-UI Events

跟当前 `frontend/src/protocol/agui/` 保持一致。详细 schema 在
`schemas/events.yaml`。事件**总是**作为 `notifications/run/event`
notification 的 `params.event` 字段出现。

### 4.1 AG-UI 标准事件（16 个）

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

### 4.2 Lyra CUSTOM 事件 (`event.type === "CUSTOM"`，按 `event.name` 分发)

| `name` | Payload | 用途 |
| --- | --- | --- |
| `lyra.plan` | `{ items: PlanItem[] }` | 替换 `state.plan` |
| `lyra.plan-block` | `{ messageId }` | 挂 plan content block |
| `lyra.code-proposal` | `{ parentMessageId, lang, file, text }` | Diff 提案 block |
| `lyra.search-results` | `{ parentMessageId, results }` | 搜索结果 block |
| `lyra.approval` | `{ requestId, parentMessageId, text, command, reason, risk?, ... }` | HITL 审批请求 |
| `lyra.approval-result` | `{ requestId, decision }` | HITL 决策回执 |
| `lyra.telemetry` | 自由形态 | 性能 / 调试信号 |

预留事件（kimi-code / agent-chat-ui 启发）：`lyra.interrupt` /
`lyra.resume` / `lyra.checkpoint` / `lyra.meta` / `lyra.subagent.*` /
`lyra.background.*` / `lyra.compaction.*`。

所有 CUSTOM 事件 payload 必须有 Zod schema（`frontend/src/protocol/agui/schemas.ts`）
+ Go mirror（`schemas/events.yaml` 生成）。

### 4.3 HITL 审批 —— Notification + 后续 Request 模式

1. Server 推 `notifications/run/event` 携带 `lyra.approval` payload
2. 前端渲染审批 block，用户点"允许"/"拒绝"
3. Client 发 Request `runs.approval.submit { requestId, decision, reason? }`
4. Server 校验后继续 run，并在流里推 `lyra.approval-result`

**为什么不用 server → client request？** 那会逼每种 transport 实现
真双向 RPC（HTTP 上得开第二条 SSE 反向流），实现复杂度跟收益不成
正比。MCP 走了那条路、坑很多；我们这条更朴素也够用。

---

## 5. Methods

### 5.1 命名约定

- `<domain>.<verb>` / `<domain>.<resource>.<verb>` —— camelCase verb
- 复数 noun（`sessions` / `messages` / `attachments`），单数 verb
- 流式方法名以 `.start` / `.subscribe` 结尾

### 5.2 完整方法表

| Method | C/N | Streaming | 用途 |
| --- | --- | --- | --- |
| `runtime.initialize` | C→R | no | 握手 + 版本协商 + 能力协商 |
| `runtime.shutdown` | C→R notify | no | 礼貌关闭 |
| `runtime.ping` | C→R | no | Liveness 探针 |
| **Runs** | | | |
| `runs.start` | C→R | yes | 启动一次 run，立即返 `{runId, streamHandle}`；事件经 `notifications/run/event` 流出 |
| `runs.cancel` | C→R notify | no | 取消 run（等价于 `notifications/cancelled` 配对 run request id） |
| `runs.approval.submit` | C→R | no | HITL 决策（见 §4.3） |
| **Sessions** | | | |
| `sessions.list` | C→R | no | 列 session（游标分页） |
| `sessions.get` | C→R | no | 读一条 |
| `sessions.create` | C→R | no | 新建 `{ title?, model?, metadata? }` |
| `sessions.update` | C→R | no | 重命名 / pin / metadata patch |
| `sessions.delete` | C→R | no | 删除 |
| `sessions.fork` | C→R | no | 在 checkpoint 分叉 |
| `sessions.export` | C→R | no | 导出 md/json |
| **Messages** | | | |
| `messages.list` | C→R | no | 游标分页历史 |
| `messages.edit` | C→R | no | edit-and-rerun；返 `{ runId, checkpoint }` 并在流里推 `lyra.checkpoint` |
| **Workspace** | | | |
| `workspace.filesChanged` | C→R | no | Diff 概览 |
| `workspace.diff` | C→R | no | 单文件 diff |
| `workspace.fileHead` | C→R | no | 文件预览 |
| `workspace.grep` | C→R | no | 代码搜索 |
| `workspace.terminal.subscribe` | C→R | yes | Tool pty 输出流 |
| `workspace.projects` | C→R | no | 项目列表 |
| `workspace.mcp.list` | C→R | no | MCP 服务状态 |
| `workspace.mcp.reconnect` | C→R | no | 重连 MCP |
| `workspace.skills` | C→R | no | 可用 skill（kimi-code 风格） |
| **Providers / Models / Tools** | | | |
| `providers.list` | C→R | no | LLM provider 注册表 |
| `providers.test` | C→R | no | 校验凭证 |
| `models.list` | C→R | no | per-provider model 列表 |
| `tools.list` | C→R | no | Tool 注册表 + JSON-Schema 参数 |
| **Attachments** | | | |
| `attachments.createUploadUrl` | C→R | no | 返 `{ uploadUrl, attachmentId }`；用 transport 各自的 binary 通道上传 |
| `attachments.delete` | C→R | no | 删除 |
| **Background** | | | |
| `background.list` | C→R | no | 活跃任务 |
| `background.stop` | C→R | no | 停止 |
| `background.subscribe` | C→R | yes | Tail 输出 |
| **Feedback** | | | |
| `feedback.submit` | C→R | no | RLHF —— 替代 `lyra.meta` 事件的 REST 路径 |

### 5.3 服务端发出的 Notification 清单

| Notification method | 何时 |
| --- | --- |
| `notifications/run/event` | run 期间每个 AG-UI 事件 |
| `notifications/run/closed` | run 流结束 |
| `notifications/background/update` | background 任务状态变化 |
| `notifications/terminal/output` | `workspace.terminal.subscribe` 后的 pty 输出 |
| `notifications/cancelled` | server 承认收到 client 的 cancellation |

### 5.4 特例 —— Attachments 二进制上传

Multipart binary 不进 JSON-RPC envelope：

1. Client 发 `attachments.createUploadUrl { filename, mime, size }`
   → result: `{ uploadUrl, attachmentId, expiresAt }`
2. Client 走 transport 各自的二进制通道：
   - HTTP: `PUT <uploadUrl>` body 为字节流，header 带 mime
   - Wails IPC: 调一个 native binding 传 `Uint8Array`
   - InProcess: 直接调 Go 函数传 `[]byte`
3. 后续用 `attachmentId` 在其他 method 里引用

这是协议里**唯一**不走 JSON-RPC 的口子。

---

## 6. Shapes

### 6.1 Capabilities

```ts
interface ServerCapabilities {
  events: {
    standard: string[];          // AG-UI events emitted
    custom: string[];            // lyra.* events emitted
  };
  features: {
    multimodal: boolean;
    reasoning: boolean;
    checkpoints: boolean;
    interrupts: boolean;
    background: boolean;
    subagents: boolean;
    skills: boolean;
    mcp: boolean;
    sessionExport: boolean;
    attachments: { enabled: boolean; maxSizeBytes?: number; mimeTypes?: string[] };
  };
  providers: string[];           // ["openai", "anthropic", "moonshot", ...]
  limits: {
    maxMessagesPerSession?: number;
    maxConcurrentRuns?: number;
  };
}

interface ClientCapabilities {
  events: { standard: string[]; custom: string[] };  // 表现层渲染得了的事件
  features: { multimodal?: boolean; markdown?: boolean; ... };
}
```

未声明的 feature 默认 `false`。Server **必须不发** client 没声明
能渲染的事件。

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
  metadata: Record<string, unknown>;
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

interface ToolSpec {
  name: string;
  description?: string;
  parameters: JsonSchema;
  origin: "server" | "client" | "mcp";
}

interface ContextItem {
  kind: "file" | "url" | "selection" | "image";
  // ...kind-specific fields
}
```

### 6.3 Run 启动参数

```ts
interface StartRunParams {
  sessionId: string;
  runId?: string;                // server 不填则生成 UUID v7
  messages: Message[];           // history + 新一轮
  state?: Record<string, unknown>;
  tools?: ToolSpec[];
  context?: ContextItem[];
  model?: string;
  mode?: "agent" | "chat" | "plan";
  attachments?: string[];        // attachmentId from createUploadUrl
}

interface StartRunResult {
  runId: string;
  streamHandle: string;
}
```

### 6.4 分页

```ts
interface PageQuery {
  limit?: number;                // default 20, max 100
  cursor?: string;               // 上次 result.nextCursor
}

interface Page<T> {
  items: T[];
  nextCursor?: string;
  hasMore: boolean;
}
```

---

## 7. 错误

### 7.1 JSON-RPC error.code

| 范围 | 含义 |
| --- | --- |
| -32700 ~ -32603 | JSON-RPC 标准错误（parse / invalid request / method not found / invalid params / internal error） |
| -32000 ~ -32099 | Lyra 业务错误（见下表） |

### 7.2 Lyra 业务错误码

| code | message | 何时 |
| --- | --- | --- |
| -32001 | `provider_error` | LLM provider 返非 2xx 或超时 |
| -32002 | `provider_rate_limited` | provider 返 429 |
| -32003 | `tool_failed` | tool 执行 throw |
| -32004 | `approval_required` | run 卡在 HITL 上、且 client 没在合理时间内回 |
| -32005 | `session_not_found` | sessionId 不存在 |
| -32006 | `message_not_found` | messageId 不存在 |
| -32007 | `run_not_found` | runId 不存在或已结束 |
| -32008 | `attachment_too_large` | 超 `maxSizeBytes` |
| -32009 | `capability_not_negotiated` | 调了 initialize 时 client 没声明的能力 |
| -32010 | `invalid_protocol_version` | initialize 版本协商失败 |
| -32011 | `protocol_violation` | 客户端发了不符合协议的消息（如 batch / 未握手就调业务） |

`error.data` 装额外细节，结构推荐 RFC 7807 ProblemDetails 形状：

```ts
interface ProblemData {
  type?: string;
  detail?: string;
  retryAfterMs?: number;
  errors?: Array<{ path: string; message: string }>;
}
```

---

## 8. Schema SSOT

```
pkg/coreapi/*.go              # Go interface + struct tags —— SSOT
       │
       ├── go-jsonrpc codegen ──→  schemas/methods.yaml      (JSON-RPC method table)
       │                                     │
       │                                     └─ openapi-ts ──→ frontend/src/lib/runtime-types.ts
       │
       └── go-asyncapi codegen ──→  schemas/events.yaml      (AG-UI + Lyra CUSTOM events)
                                              │
                                              └─ asyncapi-ts ──→ frontend/src/lib/events.ts
```

对 Go ↔ Go（in-process / Wails）**不需要 codegen**——两端直接
import `pkg/coreapi`。对 TS / Rust / Python client，schema 是契约
+ 自动 codegen。

### 8.1 版本规则

- `protocolVersion` 是日期串 `"2026-MM-DD"`
- 加 method / 加可选字段 / 加事件 → 同版本号即可（client 已声明
  `unknownMethodTolerance: true` 时）
- 改语义 / 删字段 → 新日期版本 + 协商
- `initialize` 是唯一拒绝版本不匹配硬断开的方法

---

## 9. 现状快照（2026-05-28）

| 表面 | 今天 | 距离协议落地 |
| --- | --- | --- |
| Streaming events | 16 AG-UI 标准 + 7 `lyra.*` CUSTOM，fixture DSL 出 | 换真 LLM + 真 tool 执行；加 `Last-Event-Id` 续传；事件包进 JSON-RPC notification |
| REST endpoints | 13 个 fixture | 全部映射成 JSON-RPC method（§5.2） |
| HITL | `POST /permission` 解锁全局 chan | 改 `runs.approval.submit` + 绑 runId |
| 鉴权 | 无 | **协议层永远无**。Web 形态走本地进程门禁 token |
| Schema SSOT | 前端 `lib/queries.ts` 手写 | `pkg/coreapi` 为 SSOT + codegen |
| 版本 | 无 | `runtime.initialize` 强制协商 |

Mock 后端今天监听 `http://127.0.0.1:17171`；新协议落地后该端点
变成 JSON-RPC over HTTP，路径 `POST /v1/rpc`（单 endpoint 收所有
Request）+ `GET /v1/rpc/stream/{streamHandle}`（SSE 收 notification）。

---

## 10. 未决问题

- [ ] **AG-UI events 是不是直接复用 ag-ui-protocol 官方 schema？** 还
      是只复用名字、用自己的 envelope？倾向后者，避免被 ag-ui 协议
      演进绑架。
- [ ] **`workspace.terminal.subscribe` 怎么处理多 subscriber？** 一
      个 runId 的 terminal 有多个表现层订阅时，server 是否要去重？
- [ ] **`models.list` 缓存策略？** Provider 返的 model list 不便宜，
      要不要 Runtime 缓存 1h？
- [ ] **`attachments.createUploadUrl` 在 InProcess transport 怎么映
      射？** 不需要 URL，直接返 `attachmentId` + 一个 Go binding
      函数指针？
- [ ] **Notification 顺序保证？** 同一 streamHandle 内必须保序；
      跨 stream 不保证。值得文档化吗？

---

## 附录 A — 文件位置

| 关注点 | 前端 | 后端 |
| --- | --- | --- |
| 流式 reducer（event → state） | `frontend/src/plugins/builtin/core-reducer/handlers.ts` | `internal/agui/events.go` |
| CUSTOM 事件 handler | `frontend/src/plugins/builtin/agui-handlers/index.ts` | `internal/agui/dsl.go` |
| JSON-RPC client | `frontend/src/lib/rpc.ts`（待建） | n/a |
| Method 实现入口 | n/a | `pkg/coreapi/*.go` + `pkg/coreimpl/*.go`（待建） |
| HITL 审批 gateway | `frontend/src/domain/gateways/PermissionGateway.ts` + `frontend/src/infra/http/HttpPermissionGateway.ts` | `internal/agui/permissions.go` |
| Base URL / runtime handle | `frontend/src/main/config.ts` | `internal/agui/server.go` |
| Fixture 数据 | — | `internal/agui/demos.go` |

---

## 附录 B — Facade pattern（未来云端架构，本轮不实现）

> 本附录**仅作为补充说明**，不在本轮协议实现范围。当前 Lyra
> Runtime 不感知 facade 是否存在。

云端化时的预期架构：

```
表现层 ──► Facade ──► Runtime
          (订阅/账号/    (跟本地一份代码)
           billing/授权)
```

设计要点：

1. **Facade 暴露完整 Runtime 协议**——前端透明，本地 / 云端 切
   换只改 transport endpoint，方法表不变
2. **Facade 附加私有端点**（`/account` / `/billing` / `/subscription`
   等），**这些不在 Runtime 协议里**，由表现层在云端模式下另外调
3. **Facade 持有上游 provider 凭据**——云端模式下用户不用配
   OpenAI key，那是 Facade 的事
4. **Runtime 永远不感知"上面有没有 facade"**——同一份代码跑桌面
   也跑服务器
5. **推荐方案**：Facade 只发授权 token + Runtime endpoint，前端拿
   到后**直连 Runtime**（类似 AWS STS / GCP SignedUrl）。这样
   Facade 不碰用户数据，隐私故事干净；Runtime 仍只看到 "有人在按
   协议调我"

具体落地等做云端时再写。本协议**不为 facade 留任何特殊字段**。

---

## 附录 C — 与 MCP 的关系

Lyra Runtime Protocol 在精神上和 [MCP](https://modelcontextprotocol.io)
对齐：

| 点 | MCP | Lyra |
| --- | --- | --- |
| Wire envelope | JSON-RPC 2.0 | JSON-RPC 2.0 |
| 元数据 | HTTP header | transport-specific 带外通道 |
| Lifecycle | initialize / operate / shutdown | 同 |
| 取消 | 显式 `notifications/cancelled` | 同 |
| 流式 | SSE + `Last-Event-Id` | 同 + InProcess/Wails 各自原生 |
| Server → client RPC | 支持（sampling / elicitation） | **不支持**（HITL 用 Notification + Request 配对） |
| Transport | stdio / Streamable HTTP | InProcess / Wails IPC / HTTP |
| SSOT | TypeScript schema | Go `pkg/coreapi` |
| Batch | 2024 支持 / 2025 移除 | **永不支持** |

**关键差异**：MCP 是 LLM ↔ MCP Server 之间的协议（让 LLM 调外部
工具）；Lyra Runtime Protocol 是 **表现层 ↔ Runtime** 之间的协议
（让 UI 驱动 agent）。两者解决不同的问题、可以共存：Lyra Runtime
内部可以**作为 MCP client** 接入 MCP server，但本协议本身不是 MCP。
