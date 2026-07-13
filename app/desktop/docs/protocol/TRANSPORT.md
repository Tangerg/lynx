# Lyra Runtime Transport（定稿 `2026-06-07`）

> **状态：正式契约（canonical）。** 本文定义同目录 [`API.md`](./API.md)（Lyra Runtime Protocol）如何在具体 transport
> 上承载。两文件自包含、互为配套，不依赖任何其它文档。`protocolVersion`: **`2026-06-07`**。

## 0. 目的

API 定义 JSON-RPC 方法、资源、事件语义；transport 定义 message 如何在 client 与 runtime 之间搬运。

**transport 层不得解读业务 params**（如 `cwd` / `sessionId` / `runId`）。

## 1. Transport 矩阵

| 客户端形态      | runtime 位置      | transport     |
| --------------- | ----------------- | ------------- |
| Go TUI          | 同进程            | InProcess     |
| 桌面 web 外壳   | 宿主进程          | IPC           |
| 浏览器 UI       | 本地 runtime 进程 | HTTP loopback |
| 未来远程 facade | facade 之后       | HTTP          |

所有 transport 暴露相同协议语义：request/response JSON-RPC；server→client 通知；显式取消；尽量用 event id
做流重连。

## 2. 元数据划分 —— 业务与请求自描述进 params，纯传输才走带外

**原则**：JSON-RPC `params` 承载**业务 / 语义载荷**（这次操作"是关于什么"的数据），以及 `params._meta`
里的请求自描述信息（协议版本、clientInfo、clientCapabilities）。**只有纯传输 / 观测 / 可靠性元数据**才走带外通道
（HTTP header、`context.Context`、IPC metadata）。

**判定一个字段放哪**：问"它是这次操作语义的一部分，还是只是承载这次操作的传输上下文？"

- **是语义的一部分 → params**（如 `cwd` 决定 session 属于哪个项目、`sessionId` / `runId` 指明操作对象）。
- **只是传输上下文 → 带外**（trace、幂等键、门禁 token、流游标）。

走带外的（**非业务**）：

| 元数据           | Header / 字段                                                                 |
| ---------------- | ----------------------------------------------------------------------------- |
| Trace 上下文     | `traceparent`（+ `tracestate` / `baggage`）—— W3C TraceContext，OTel 标准注入 |
| 幂等键           | `X-Idempotency-Key`                                                           |
| 响应 method 标签 | `X-Method`                                                                    |
| 响应 server 标签 | `X-Server`                                                                    |
| 本地门禁 token   | `Authorization: Bearer <token>`                                               |
| 流重放游标       | `Last-Event-Id`（仅 `runs.subscribe` / `background.subscribe` 续流，§9.2）    |

规则与易错点：

- `cwd` 是**会话身份**（业务）→ **进 params**，**永不**走带外 directory header（它像目录、其实是身份）。
- `sessionId` / `runId` = 业务 → 进 params。
- 幂等键像业务、其实是**重试可靠性控制**（非业务）→ 走 header（思想取自 Stripe `Idempotency-Key`）。
- 协议版本 / clientInfo / clientCapabilities = 请求自描述 → `params._meta`。
- trace / 门禁 token / `Last-Event-Id` = 传输上下文 → 带外。
- **不再有连接 id**：streamable HTTP 下事件属于"开它的那条 POST 流"，无需带外路由键（§6 / §8）。
- **JSON-RPC envelope `id` 必须是 string**（硬约束）。虽然 JSON-RPC 2.0 本身也允许 number id，本协议**只接受字符串 id**
  —— number id 会被拒为 `invalid_request`（`detail: "id must be a JSON string"`）。本文所有示例用字符串即遵此约定。
  （`null` id 仅用于无法解析 request 时的错误响应，按 JSON-RPC 2.0。）

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

> `receive()` 的入站消息**从哪来**是各 transport 自己的事，抽象层不规定：InProcess/IPC 走宿主的 push
> channel / callback；HTTP **streamable** 则来自各 POST 的响应（`application/json` 单条，或
> `text/event-stream` 多帧，§6.4）汇入同一条可迭代流。**没有"常开的 server→client 通道"这一前提** —— 响应
> 与通知都依附于某次调用。

## 4. InProcess Transport

InProcess 用于与 runtime 链接在同一二进制里的 Go 客户端。

推荐实现：

- 直接暴露类型化 runtime interface；
- 跳过 JSON 序列化；
- 保持相同的方法名、params、result 类型、流式语义；
- 通知走 Go channel 或 callback。

InProcess 没有 HTTP header，元数据走 `context.Context`。

| 元数据       | InProcess 来源         |
| ------------ | ---------------------- |
| Trace 上下文 | context value          |
| 协议版本     | 编译期或 context value |
| 幂等键       | context value          |

## 5. IPC Transport

IPC 用于桌面外壳（Wails / Tauri / Electron + 宿主 runtime）。

要求：

- request/response 带 JSON-RPC envelope；
- 通知从宿主推到 webview；
- 元数据走 IPC message metadata（若有），否则走 JSON-RPC body 之外、宿主自有的小 wrapper；
- IPC 必须保证每连接的 message 顺序。

IPC 适配器应把宿主框架细节藏在与 HTTP 相同的 `Transport` 形态背后。

## 6. HTTP Transport

HTTP 用于浏览器与未来 facade 部署。采用 **streamable HTTP**：**流式方法的 POST 响应体本身就是这次操作的
事件流**，没有独立的"开流"连接（思想取自 OpenAI / Anthropic / MCP Streamable HTTP）。所有 server→client 消息
都依附于某个客户端调用，故无需常开的带外通道。

### 6.1 端点

| 端点                    | 用途                                                                                              |
| ----------------------- | ------------------------------------------------------------------------------------------------- |
| `POST /v2/rpc/{method}` | 所有 JSON-RPC 调用。响应是 `application/json`（非流式）或 `text/event-stream`（流式方法，§6.4）。 |
| `GET /v2/info`          | sidecar runtime 信息。                                                                            |
| `GET /v2/health`        | sidecar 健康检查。                                                                                |

> **没有独立的通知流端点**：每个流式调用的事件走它自己那条 POST 响应流（§6.4）。若将来真出现"带外 /
> 服务端主动推送"需求（多客户端同步、server→client request 等 —— 目前 API.md §13 明确不做），可**增量**加回
> 一条可选 `GET /v2/rpc/stream` 专收带外消息，不影响现有契约。

> **路径里的 `/v2/` 与 `protocolVersion`（日期串）是两个层级**：`/v2/` 是 wire major epoch（只有大破坏
> 才换路径前缀）；日期 `protocolVersion` 是该 epoch 内随 request metadata 发送的版本。两者不重复。

`{method}` 保留点。例如：

```text
POST /v2/rpc/runs.start
POST /v2/rpc/workspace.getDiff
POST /v2/rpc/workspace.mcp.listServers
```

无 method 后缀的 `POST /v2/rpc` 非法。

### 6.2 POST 契约

请求（流式与非流式同一形态；客户端声明它两种响应都能收）：

```http
POST /v2/rpc/runs.start
Content-Type: application/json
Accept: application/json, text/event-stream
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
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
    "_meta": {
      "protocolVersion": "2026-06-07",
      "clientInfo": { "name": "lyra-desktop", "version": "0.1.0" },
      "clientCapabilities": {
        "events": [
          "segment.started",
          "segment.finished",
          "item.started",
          "item.completed"
        ],
        "features": {},
        "interruptTypes": ["approval", "question"]
      }
    },
    "sessionId": "ses_...",
    "input": [{ "type": "text", "text": "hello" }]
  }
}
```

URL 里的 method 与 body 里的 method 必须一致；不一致是**自相矛盾的畸形请求**（不是资源状态冲突），返
`400 Bad Request`，与其它畸形请求同类。

**响应按方法分两种形态**（content negotiation —— client 按响应 `Content-Type` 分支；**哪些方法流式由
`ServerCapabilities.streamingMethods` 机器可读声明**，见 API.md §9，client 不硬编码方法名）：

- **非流式方法** → `200 application/json`，body 是单条 JSON-RPC 响应；业务错误同样返 `200 application/json`
  - JSON-RPC `error`：

  ```http
  HTTP/1.1 200 OK
  Content-Type: application/json
  X-Method: sessions.get
  X-Server: lyra-runtime
  ```

  ```json
  { "jsonrpc": "2.0", "id": "1", "result": { "id": "ses_..." } }
  ```

- **流式方法**（如 `runs.start` / `runs.resume` / `runs.subscribe` / `background.subscribe`，以
  `streamingMethods` 为准）→ `200 text/event-stream`，响应体是这次操作的事件流（§6.4）。

> 流式方法**开流前**的失败（params 非法 / `session_not_found` 等同步错误）仍返 `application/json` +
> JSON-RPC `error`，**不开流**；开流后的执行期错误走流内的 `segment.finished{outcome:error}` 帧。这正是
> API.md §8.1 的三条投递通道在 HTTP 上的落点（rpc 通道走同步 error，run/tool 通道走流内帧）。

### 6.3 HTTP status

HTTP status 只描述传输层失败。

| status | 含义                                                                                                                          |
| ------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `200`  | JSON-RPC 响应已接受（`application/json` 单条，body 里仍可能含业务 error）**或** 流式响应已开启（`text/event-stream`，§6.4）。 |
| `204`  | client 通知已接受、同步 dispatch 完毕；无 body。                                                                              |
| `400`  | HTTP 请求畸形、JSON 非法、或 URL method 与 body method 不一致。                                                               |
| `401`  | 本地门禁 token 缺失或错误。响应**必带** `WWW-Authenticate: Bearer`（RFC 9110 §15.5.2）。                                      |
| `404`  | 未知 transport 端点。                                                                                                         |
| `405`  | HTTP 方法错误。响应**必带** `Allow`（列出该端点支持的方法，RFC 9110 §15.5.6）。                                               |
| `413`  | HTTP body 超传输上限。                                                                                                        |
| `415`  | content-type 不支持。                                                                                                         |
| `500`  | JSON-RPC dispatch 前的适配器失败。                                                                                            |

> 状态码只描述传输层（RFC 9110）。**通知**同步处理完且无 body 用 `204`（非 `202` —— 后者语义是"已收下、
> 处理未决"）；自相矛盾的请求（method 不一致）归 `400`，**不**用 `409`（409 专表与资源当前状态的冲突）。

**不要**把 `session_not_found` / `path_outside_root` 等业务错误映射成 HTTP status（业务错误走 JSON-RPC
`error`，见 API.md §8）。

### 6.4 流式方法响应（Streamable HTTP）

流式方法的 POST 响应体是一条 SSE 流，**承载这一次操作的完整 JSON-RPC 消息序列**：

1. **首帧 = 本次调用的 JSON-RPC 响应**（带请求的 envelope `id`），如 `runs.start` 的 `{ "id":"1",
"result":{ "runId":"run_..." } }` —— 客户端据此拿到 `runId`，无需单独的同步响应。此帧是一次性 ack，
   **不带 SSE `id:`**（它不属于可重放的 run 事件序列，§9.1）；
2. 随后是 `notifications.run.event` 帧（run / item / state 事件，API.md §5），每帧 SSE `id:` =
   `RunEvent.eventId`；
3. **root `segment.finished` 后服务端关闭这条流**。

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
X-Method: runs.start
X-Server: lyra-runtime
```

```text
data: {"jsonrpc":"2.0","id":"1","result":{"runId":"run_01","userItemId":"item_00"}}

id: evt_0001
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_01","eventId":"evt_0001","timestamp":"2026-06-07T10:00:00Z","event":{"type":"segment.started","run":{"id":"run_01","sessionId":"ses_01"}}}}

id: evt_0002
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_01","eventId":"evt_0002","event":{"type":"item.delta","itemId":"item_01","delta":{"type":"content","text":"Hello"}}}}

id: evt_0009
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_01","eventId":"evt_0009","event":{"type":"segment.finished","outcome":{"type":"completed","result":{}}}}}
```

> `RunEvent` 信封**不带 `durable` 字段**：first-party 事件的 durable 性由 `event.type` 推导（API.md §5.2 推导表），
> 唯 `custom` 事件在 `event` 内自带 `durable?`。SSE 重放（§9）据这张推导表决定哪些事件要带 `id:` / 可重放。

要点：

- **一次操作 = 一条流 = 一个 HTTP 交换**；`curl -N` 即可看全程，日志里一请求对应一操作。
- 该流承载**整棵 run 树**：子孙 subagent run 的事件并入此流（每帧带自己的 `runId`，客户端用
  `RunRef.spawnedByItemId` 还原树，API.md §5.4 / §10.3）。`background.subscribe` 的流承载该 task 的
  `notifications.background.update` 帧。
- 网络断开**不取消** run（API.md §3）；run 在服务端继续，客户端按 §9 续流。

### 6.5 并发与连接预算（HTTP/1.1）

一个**活跃流式 run 占一条 HTTP 连接**（整个 run 期间不释放）。浏览器 / WebView 对同 origin 在 **HTTP/1.1 上
约 6 条并发连接**，且对明文 `http://` loopback **只走 HTTP/1.1** —— 浏览器 / WKWebView 仅在 **TLS + ALPN** 下
协商 HTTP/2，**不支持 h2c**（明文 HTTP/2），故 server 端开 h2c **对浏览器客户端无效**。

客户端策略（避免连接耗尽 / head-of-line 阻塞）：

- **只对活跃 run 开流**；后台 run 用 `items.list` 补历史，需要 live 再 `runs.subscribe`；
- 普通 RPC 走短连接（keep-alive 复用），不与流式连接抢占；
- `maxConcurrentRuns` 是 server 并发上限，**不等于**客户端要同时开这么多条流。

真需要"同时 live 跟多个 run"并会顶到上限时，loopback 上启用 **TLS**（`https://127.0.0.1` + 本地证书）让浏览器
ALPN 协商 HTTP/2 多路复用 —— 这才是有效解，**不是 h2c**。

## 7. SSE 帧格式

每个 SSE 帧的 `data:` 是一条 JSON-RPC message（流式响应体 §6.4 用此格式）。

```text
id: evt_00000000042
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{...}}
```

规则：

- 只有 `notifications.run.event`（以及 `background.subscribe` 流里的 `notifications.background.update`）等
  **可重放的事件帧**带 SSE `id:`，且 = 其 `eventId`；JSON-RPC 响应帧（首帧）**不带 `id:`**（一次性 ack，非可重放事件，§9.1）。
- `event:` / `retry:` 不使用；客户端忽略未知字段。
- 心跳用 SSE comment（`: ...` 行）。
- 流是 **POST** 而非 `EventSource` GET，故门禁 token 照常走 `Authorization: Bearer` —— "`EventSource` 不能设
  header / 只能用 cookie / 由此引入 CSRF 面"那套问题**随 streamable HTTP 一并消失**。

## 8. 运行树（一条流承载整棵树）

一个流式调用的响应流承载**整棵 run 树**的事件（根 + 所有子孙 subagent run）：

- 每帧带它所属的 `runId`；
- 子 run 以 `segment.started` 携带 `spawnedByItemId` 开始；
- 客户端用 `runId` + `spawnedByItemId` join 还原树（API.md §5.4 / §10.3）；
- `features.subagents=false` 时不产出任何子 run 事件。

**不再有连接级路由**：事件天然属于"开它的那条 POST 响应流"，无 `X-Conn-Id`、无 run→连接登记表。

**一个 run 可被 ≥1 条流并发订阅（N fan-out）**：server 须支持把同一 run 的事件序列同时 fan-out 给多条 POST
响应流（多 tab、或重连时旧流未拆的短暂重叠），每条流从各自的 `Last-Event-Id` 之后续发（§9.2）。

## 9. 重放与重连

### 9.1 Event id

`eventId` 由 server 生成，**在一个 Segment 的事件序列内单调有序**（= API.md §2.4 的 "Segment 流"；含该段根 Run +
所有子孙 Run 的事件）。同一段序列**可能分布在多条 HTTP 响应里** —— 原始 `runs.start`/`runs.resume` 流 + 之后对该段的
`runs.subscribe` 续流 —— 单调性贯穿整段，故 `Last-Event-Id` 能线性重放。

- `runs.subscribe` 续流**沿用同一段序列**：从 `Last-Event-Id` 之后接着发（重放的 durable 事件保持其原始
  `eventId`，客户端据此去重）；
- `runs.resume` 在同一 Run 上开**新的一段**（同 `runId`、新 `segmentId`、`eventId` 从头，API.md §2.4）。

server 应保留 durable 事件足够久以支撑续流，但**正确性不得依赖 ephemeral delta 的重放**。

### 9.2 续流流程（per-run）

某条 run 流断开时（run 在服务端继续）：

1. 客户端对该 run 调 `POST /v2/rpc/runs.subscribe { runId }`，带 `Last-Event-Id: <最后见到的 eventId>`；
2. server 在新响应流里**重放该 id 之后的 durable 事件**，再接上 live；
3. 客户端按 `eventId` 与 `itemId` 去重。
4. **兜底**（server 暂未就绪 `Last-Event-Id` 重放时）：不带 `Last-Event-Id` 整段重订阅 + `items.list` 补
   durable 缺口 —— 靠 `item.completed` / `state.snapshot` 兜终态正确（durable/ephemeral 不变量，API.md §5.2）。

> 重连的是**具体某个 run**（`runs.subscribe` 即续流口），粒度准，且不需要维护连接身份。

### 9.3 Delta 重放

server **不需要**重放任何 ephemeral 事件（按 API.md §5.2 推导表为 `durable=false` 的：`item.delta` /
`state.delta` / `segment.progress` / `durable:false` 的 `custom`）。

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

HTTP `runs.start` 上，重试的幂等键不新建 run；这次 POST 的 `text/event-stream` 响应接上既有 run（按
`Last-Event-Id` 重放 durable 缺口 + live，§9.2），首帧仍返回既有 `runId`。

## 11. 本地门禁 token

loopback HTTP 必须防止任意本地网页 / 本地进程访问 runtime。

推荐：

- 运行时初始化时生成随机 token；
- 存到 owner-only 权限的用户私有文件；
- `/v2/rpc/*` 要求 `Authorization: Bearer <token>`；token 缺失/错误返 `401` + `WWW-Authenticate: Bearer`（RFC 9110 §15.5.2）；
- `/v2/health` 免 token；
- `/v2/info` 仅在不含 secret 时免 token。

token 是本地进程门禁，**不是用户鉴权**（协议层零 user 概念，见 API.md §15）。

## 12. Sidecar 端点（HTTP 专属）

sidecar 端点不走 JSON-RPC（扁平 JSON、无需 discovery、无鉴权）。

**它们只在 HTTP transport 存在**，因为只有 HTTP 才有这种运维场景：`curl` / oncall 探活、k8s
liveness/readiness、反代 upstream 健康检查 —— 这些要的是"不套 envelope、无鉴权"的端点。
InProcess / IPC 没有这种场景（无网络探针、客户端直接持有 runtime 对象），**同样的需求由协议内方法覆盖**：

| 需求                                            | HTTP                     | InProcess / IPC                                 |
| ----------------------------------------------- | ------------------------ | ----------------------------------------------- |
| 运行信息（serverInfo / version / capabilities） | sidecar `GET /v2/info`   | `runtime.discover` 响应（本就携带同样内容）     |
| 存活探测                                        | sidecar `GET /v2/health` | `runtime.ping`（API.md §7.1：仅 InProcess/IPC） |

即：`/v2/info` 内容 = `runtime.discover` 响应的扁平子集；`/v2/health` 等价于 `runtime.ping`。

> **规律：非 JSON-RPC 的旁路通道各 transport 用自己的原生形态实现；场景不适用的 transport 由协议内
> 方法替代。**（图片输入不属于此类——它内联在 `runs.start.input` 的 image ContentBlock 里走常规 JSON-RPC，
> 无独立二进制上传通道。）

### 12.1 `GET /v2/health`

```json
{ "ok": true }
```

仅用于传输层 liveness。`200`=ok；`503`=degraded/unhealthy。

### 12.2 `GET /v2/info`

```json
{
  "protocolVersion": "2026-06-07",
  "serverInfo": {
    "name": "lyra-runtime",
    "version": "0.0.0",
    "cwd": "/path/to/serve/cwd",
    "home": "/Users/example"
  },
  "capabilities": {
    "protocolVersion": "2026-06-07",
    "events": [
      "segment.started",
      "segment.progress",
      "segment.finished",
      "item.started",
      "item.delta",
      "item.completed",
      "state.snapshot",
      "state.delta"
    ],
    "streamingMethods": ["runs.start", "runs.resume", "runs.subscribe"],
    "features": {
      "reasoning": true,
      "mcp": true,
      "relocate": true,
      "multimodal": true
    },
    "providers": [],
    "limits": { "maxConcurrentRuns": 8 }
  }
}
```

> `features` 是**开放 map**（API.md §9）：未声明的 key 视为关闭，故 server 只需 advertise 它真正开启的能力，
> 新增能力加一个 key 即可、不 bump 契约。`/v2/info` 与 `runtime.discover` 必须由同一份 server 状态支撑。

## 13. CORS

loopback HTTP 应限制 origin。

推荐默认：

- 放行内置客户端 origin；
- 放行显式配置的开发 origin；
- 启用本地门禁 token 时拒绝通配 origin；
- 允许 header：`Content-Type`、`Authorization`、`Last-Event-Id`、`traceparent`、`tracestate`、`baggage`、`X-Idempotency-Key`；
- expose header：`X-Method`、`X-Server`、`traceparent`。

## 14. 压缩与 buffering

**非流式**（`application/json`）POST 响应可用普通 HTTP 压缩。

**流式**（`text/event-stream`）POST 响应**不可**被压缩中间件缓冲，须避免反代 buffering、每帧及时 flush。
推荐 header：

```http
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

## 15. 背压

server 可在高负载下**合并 ephemeral delta**，只要 durable/ephemeral 不变量仍成立（API.md §5.2）。

server **不得**在不可经历史 API 恢复的前提下丢弃 durable 事件。

## 16. Observability

每个 transport 适配器应记录：method、envelope id、trace 上下文、协议版本、时长、JSON-RPC error code
（若有）、传输 status。流式响应另记 `runId` 与该流发出的事件数。请求的 trace 关联走入站 `traceparent`
（W3C TraceContext）；HTTP 响应在可用时应带 `X-Method` / `X-Server`。

> **反向不变量**：`cwd` / 路径不进高 cardinality metric label（路径无界会爆 Prometheus，需要时只在结构化
> 日志记 hash / basename）；PII（消息 / prompt 内容）不进 access log / metric。

## 17. 安全边界

| 层               | 负责                                                                                                 |
| ---------------- | ---------------------------------------------------------------------------------------------------- |
| transport 层     | 本地门禁 token 校验、origin 检查、body 大小限制、content-type 检查、流响应生命周期                   |
| API / runtime 层 | `cwd` 下的 path containment、URL fetch egress 策略、工具审批策略、能力声明匹配、provider secret 处理 |

## 18. v2 不支持

- WebSocket transport。
- stdio transport。
- JSON-RPC batch。
- server→client JSON-RPC request。
- 客户端自选的业务资源 id。
- 连接级 active project 状态。

---

> 正式契约。配套同目录 [`API.md`](./API.md)。
