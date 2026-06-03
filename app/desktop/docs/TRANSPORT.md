# Lyra Runtime Transport（正式定稿 v2）

> **状态：正式契约（canonical）。** 本文定义 [`API.md`](./API.md)（Lyra Runtime Protocol v2）如何在具体
> transport 上承载。`protocolVersion`: **`2026-06-03`**。

## 0. 目的

API 定义 JSON-RPC 方法、资源、事件语义；transport 定义 message 如何在 client 与 runtime 之间搬运。

**transport 层不得解读业务 params**（如 `cwd` / `sessionId` / `runId`），除非显式用于流路由。

## 1. Transport 矩阵

| 客户端形态 | runtime 位置 | transport |
| --- | --- | --- |
| Go TUI | 同进程 | InProcess |
| 桌面 web 外壳 | 宿主进程 | IPC |
| 浏览器 UI | 本地 runtime 进程 | HTTP loopback |
| 未来远程 facade | facade 之后 | HTTP |

所有 transport 暴露相同协议语义：request/response JSON-RPC；server→client 通知；显式取消；尽量用 event id
做流重连。

## 2. 元数据划分 —— 业务进 params，非业务才走带外

**原则**：JSON-RPC `params` 承载**业务 / 语义载荷**（这次操作"是关于什么"的数据）；**只有非业务**的
传输 / 连接 / 观测 / 可靠性元数据才走带外通道（HTTP header、`context.Context`、IPC metadata）。

**判定一个字段放哪**：问"它是这次操作语义的一部分，还是只是承载这次操作的传输上下文？"

- **是语义的一部分 → params**（如 `cwd` 决定 session 属于哪个项目、`sessionId` / `runId` 指明操作对象）。
- **只是传输上下文 → 带外**（连接 id、trace、协议版本、幂等键、门禁 token、流游标）。

走带外的（**非业务**）：

| 元数据 | Header / 字段 |
| --- | --- |
| 连接 id | `X-Conn-Id` |
| Trace id | `X-Trace-Id` |
| 协议版本 | `X-Protocol-Version` |
| 幂等键 | `X-Idempotency-Key` |
| 响应 method 标签 | `X-Method` |
| 响应 server 标签 | `X-Server` |
| 本地门禁 token | `Authorization: Bearer <token>` |
| SSE 重放游标 | `Last-Event-Id` |

规则与两个易错点：

- `cwd` 是**会话身份**（业务）→ **进 params**，**永不**走带外 directory header（它像目录、其实是身份）。
- `sessionId` / `runId` = 业务 → 进 params。
- 幂等键像业务、其实是**重试可靠性控制**（非业务）→ 走 header（对标 Stripe `Idempotency-Key`）。
- 连接 id / trace / 协议版本 / 门禁 token / `Last-Event-Id` = 非业务 → 带外。
- 连接 id 标识一个**传输连接**，不是 session、不是 project。

## 3. 抽象形态

transport 是一条双向 message 管道。

```ts
interface Transport {
  send(message: Message): Promise<void>;
  receive(): AsyncIterable<Message>;
  close(): Promise<void>;
}
```

transport **不**配对 request/response id —— 那是上层 RPC client 的事。

## 4. InProcess Transport

InProcess 用于与 runtime 链接在同一二进制里的 Go 客户端。

推荐实现：

- 直接暴露类型化 runtime interface；
- 跳过 JSON 序列化；
- 保持相同的方法名、params、result 类型、流式语义；
- 通知走 Go channel 或 callback。

InProcess 没有 HTTP header，元数据走 `context.Context`。

| 元数据 | InProcess 来源 |
| --- | --- |
| 连接 id | context value |
| Trace id | context value |
| 协议版本 | 编译期或 context value |
| 幂等键 | context value |

## 5. IPC Transport

IPC 用于桌面外壳（Wails / Tauri / Electron + 宿主 runtime）。

要求：

- request/response 带 JSON-RPC envelope；
- 通知从宿主推到 webview；
- 元数据走 IPC message metadata（若有），否则走 JSON-RPC body 之外、宿主自有的小 wrapper；
- IPC 必须保证每连接的 message 顺序。

IPC 适配器应把宿主框架细节藏在与 HTTP 相同的 `Transport` 形态背后。

## 6. HTTP Transport

HTTP 用于浏览器与未来 facade 部署。

### 6.1 端点

| 端点 | 用途 |
| --- | --- |
| `POST /v2/rpc/{method}` | JSON-RPC request 或 client 通知。 |
| `GET /v2/rpc/stream?conn=<connectionId>` | SSE 通知流。 |
| `GET /v2/info` | sidecar runtime 信息。 |
| `GET /v2/health` | sidecar 健康检查。 |

> **路径里的 `/v2/` 与 `protocolVersion`（日期串）是两个层级**：`/v2/` 是 wire major epoch（只有大破坏
> 才换路径前缀）；日期 `protocolVersion` 是该 epoch 内经 `initialize` 协商的版本。两者不重复。

`{method}` 保留点。例如：

```text
POST /v2/rpc/runs.start
POST /v2/rpc/workspace.getDiff
POST /v2/rpc/workspace.mcp.listServers
```

无 method 后缀的 `POST /v2/rpc` 非法。

### 6.2 POST 契约

请求：

```http
POST /v2/rpc/runs.start
Content-Type: application/json
X-Conn-Id: conn_...
X-Trace-Id: trace_...
X-Protocol-Version: 2026-06-03
X-Idempotency-Key: 018f...
Authorization: Bearer <local-token>
```

body：

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "method": "runs.start",
  "params": {
    "sessionId": "ses_...",
    "input": [{ "type": "text", "text": "hello" }]
  }
}
```

URL 里的 method 与 body 里的 method 必须一致；不一致返 transport 层 `409 Conflict`。

成功的 JSON-RPC 响应：

```http
HTTP/1.1 200 OK
Content-Type: application/json
X-Method: runs.start
X-Server: lyra-runtime
X-Trace-Id: trace_...
```

```json
{
  "jsonrpc": "2.0",
  "id": "1",
  "result": { "runId": "run_..." }
}
```

业务错误同样返 HTTP `200` + JSON-RPC `error`。

### 6.3 HTTP status

HTTP status 只描述传输层失败。

| status | 含义 |
| --- | --- |
| `200` | JSON-RPC 响应已接受（body 里仍可能含业务 error）。 |
| `202` | client 通知已接受；无 JSON-RPC 响应 body。 |
| `400` | HTTP 请求畸形或 JSON 非法。 |
| `401` | 本地门禁 token 缺失或错误。 |
| `404` | 未知 transport 端点。 |
| `405` | HTTP 方法错误。 |
| `409` | URL method 与 body method 不一致。 |
| `413` | HTTP body 超传输上限。 |
| `415` | content-type 不支持。 |
| `500` | JSON-RPC dispatch 前的适配器失败。 |

**不要**把 `session_not_found` / `path_outside_root` 等业务错误映射成 HTTP status（业务错误走 JSON-RPC
`error`，见 API.md §8）。

## 7. SSE 流

HTTP 客户端经 SSE 收 server 通知。

```http
GET /v2/rpc/stream?conn=conn_...
Accept: text/event-stream
Last-Event-Id: evt_...
Authorization: Bearer <local-token>
```

每个 SSE event 的 `data` 是一条 JSON-RPC 通知。

```text
id: evt_00000000042
event: message
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_...","eventId":"evt_00000000042","timestamp":"2026-06-03T10:00:00Z","durable":true,"event":{"type":"run.started","run":{"id":"run_...","sessionId":"ses_..."}}}}
```

规则：

- run 事件的 SSE `id` 必须等于 `RunEvent.eventId`。
- 一条 SSE 流关联一个 `X-Conn-Id` / `conn` 值。
- 浏览器 `EventSource` 不能设自定义 header，故 `conn` 走 query string。
- 心跳可用 SSE comment 发。

> **SSE 的本地门禁与 CSRF 面**：`EventSource` 不能设 header，故 token 不能像 POST 那样走
> `Authorization`。可选做法：(a) 宿主侧 EventSource 包装器注入 token；(b) loopback 用其他获批的本地
> 门禁机制。**慎用 cookie 承载 token** —— cookie 会让同机任意本地网页都能打开这条只读通知流（CSRF
> 面）。注意：所有**变更**走 POST + `Authorization: Bearer`，不受 SSE 门禁方式影响；SSE 只读，风险面是
> "通知流被同机恶意页读取"，仍应按需收紧。

## 8. 流路由

runtime 把通知路由到"启动或订阅了相关流"的那条连接。

`runs.start` / `runs.resume` / `runs.subscribe` 把调用方连接登记为所返回 root run 流的接收方。

对一条 root run 流：

- 通知含 root run 事件；
- 通知含子孙 subagent run 事件；
- 每条通知仍带自己的 `runId`；
- 客户端用 `RunRef.spawnedByItemId` 还原树（见 API.md §5.4 / §10.3）。

## 9. 重放与重连

### 9.1 Event id

`eventId` 由 server 生成，**在单个 root run stream 内单调有序**（不是 per-Run —— 一条 root 流复用了根 +
所有子孙 Run 的事件，单调性必须在流级别，`Last-Event-Id` 才能线性重放）。它是重放与去重的锚。

server 应保留 durable 事件足够久以支撑普通重连，但**正确性不得依赖 ephemeral delta 的重放**。

### 9.2 重连流程

HTTP 重连：

1. 重开 `GET /v2/rpc/stream?conn=<尽量同一 conn>`。
2. 浏览器在可用时自动带 `Last-Event-Id`。
3. 对仍在显示的每个 root run 调 `runs.subscribe { runId }`。
4. 调 `items.list` 补 durable 缺口。
5. 按 `eventId` 与 `itemId` 去重。

若原连接 id 不可用，新连接 id 也有效；客户端必须重新订阅活跃的 root run。

### 9.3 Delta 重放

server **不需要**重放：`item.delta` / `state.delta` / 任何 `durable=false` 事件。

server **必须**通过 durable 的 `item.completed` / `state.snapshot` 以及 `items.list` 让最终态可得
（API.md §5.2 不变量）。

## 10. 幂等

幂等适用于 run 创建，以及未来任何 opt-in 的非幂等 mutating 方法。

client 发：

```http
X-Idempotency-Key: <client 生成的稳定 key>
```

server 行为：

- 首次请求创建资源。
- 同 key 的精确重试 → 返回 / 重新接上既有结果。
- 同 key + 不同 params → JSON-RPC 错误 `idempotency_conflict`。
- 资源 id 仍由 server 生成。

HTTP `runs.start` 上，重试的幂等键返回既有 `runId` 并把当前连接关联到既有 run 流。

## 11. 本地门禁 token

loopback HTTP 必须防止任意本地网页 / 本地进程访问 runtime。

推荐：

- 运行时初始化时生成随机 token；
- 存到 owner-only 权限的用户私有文件；
- `/v2/rpc/*` 要求 `Authorization: Bearer <token>`；
- `/v2/health` 免 token；
- `/v2/info` 仅在不含 secret 时免 token。

token 是本地进程门禁，**不是用户鉴权**（协议层零 user 概念，见 API.md §13）。

## 12. Sidecar 端点（HTTP 专属）

sidecar 端点不走 JSON-RPC（扁平 JSON、握手前可访问、无鉴权）。

**它们只在 HTTP transport 存在**，因为只有 HTTP 才有这种运维场景：`curl` / oncall 探活、k8s
liveness/readiness、反代 upstream 健康检查 —— 这些要的是"握手前、不套 envelope、无鉴权"的端点。
InProcess / IPC 没有这种场景（无网络探针、客户端直接持有 runtime 对象），**同样的需求由协议内方法覆盖**：

| 需求 | HTTP | InProcess / IPC |
| --- | --- | --- |
| 运行信息（serverInfo / version / capabilities） | sidecar `GET /v2/info` | `runtime.initialize` 响应（本就携带同样内容） |
| 存活探测 | sidecar `GET /v2/health` | `runtime.ping`（API.md §7.1：仅 InProcess/IPC） |

即：`/v2/info` 内容 = `runtime.initialize` 响应的扁平子集；`/v2/health` 等价于 `runtime.ping`。
HTTP 额外开 sidecar，纯为 ops 工具能在握手前、不走 envelope 地访问。

> **同理另一个非 JSON-RPC 通道：附件二进制上传。** `attachments.createUploadUrl` 返回的 `uploadUrl`
> 在 HTTP 上是 `PUT` 二进制端点，在 InProcess/IPC 上是 `file://` / native binding / `[]byte` 句柄。
> 规律：**非 JSON-RPC 的旁路通道各 transport 用自己的原生形态实现；场景不适用的 transport 由协议内
> 方法替代。**

### 12.1 `GET /v2/health`

```json
{ "ok": true }
```

仅用于传输层 liveness。`200`=ok；`503`=degraded/unhealthy。

### 12.2 `GET /v2/info`

```json
{
  "protocolVersion": "2026-06-03",
  "serverInfo": {
    "name": "lyra-runtime",
    "version": "0.0.0",
    "cwd": "/path/to/serve/cwd",
    "home": "/Users/example"
  },
  "capabilities": {
    "protocolVersion": "2026-06-03",
    "events": ["run.started", "run.finished", "item.started", "item.delta", "item.completed", "state.snapshot", "state.delta"],
    "features": {
      "reasoning": true,
      "mcp": true,
      "multimodal": false,
      "checkpoints": false,
      "background": false,
      "subagents": false,
      "skills": false,
      "sessionExport": false,
      "memory": false,
      "relocate": true,
      "clientTools": false,
      "attachments": { "enabled": false }
    },
    "providers": [],
    "limits": { "maxConcurrentRuns": 8 }
  }
}
```

`/v2/info` 与 `runtime.initialize` 必须由同一份 server 状态支撑。

## 13. CORS

loopback HTTP 应限制 origin。

推荐默认：

- 放行内置客户端 origin；
- 放行显式配置的开发 origin；
- 启用本地门禁 token 时拒绝通配 origin；
- 允许 header：`Content-Type`、`Authorization`、`X-Conn-Id`、`X-Trace-Id`、`X-Protocol-Version`、`X-Idempotency-Key`；
- expose header：`X-Method`、`X-Server`、`X-Trace-Id`。

## 14. 压缩与 buffering

POST 响应可用普通 HTTP 压缩。

SSE 流应避免反代 buffering、每条通知及时 flush。推荐 header：

```http
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

## 15. 背压

server 可在高负载下**合并 ephemeral delta**，只要 durable/ephemeral 不变量仍成立（API.md §5.2）。

server **不得**在不可经历史 API 恢复的前提下丢弃 durable 事件。

## 16. Observability

每个 transport 适配器应记录：method、envelope id、连接 id、trace id、协议版本、时长、JSON-RPC error code
（若有）、传输 status。HTTP 响应在可用时应带 `X-Method` / `X-Server` / `X-Trace-Id`。

> **反向不变量**：`cwd` / 路径不进高 cardinality metric label（路径无界会爆 Prometheus，需要时只在结构化
> 日志记 hash / basename）；PII（消息 / prompt 内容）不进 access log / metric。

## 17. 安全边界

| 层 | 负责 |
| --- | --- |
| transport 层 | 本地门禁 token 校验、origin 检查、body 大小限制、content-type 检查、流连接归属 |
| API / runtime 层 | `cwd` 下的 path containment、URL fetch egress 策略、工具审批策略、能力协商、provider secret 处理 |

## 18. v2 不支持

- WebSocket transport。
- stdio transport。
- JSON-RPC batch。
- server→client JSON-RPC request。
- 客户端自选的业务资源 id。
- 连接级 active project 状态。

---

> 正式契约。配套 [`API.md`](./API.md)。
