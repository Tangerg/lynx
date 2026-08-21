# Lyra Runtime Transport（定稿 `2026-08-17`）

> **状态：正式契约（canonical）。** 本文定义同目录 [`API.md`](./API.md)（Lyra Runtime Protocol）如何在具体 transport
> 上承载，并且是 **binding 层的唯一作者**：端点、POST 契约、HTTP status、SSE 帧、续流、门禁 token、sidecar、CORS、
> 背压 —— 这些在别处都没有第二份定义。`protocolVersion`: **`2026-08-17`**。

## 0. 目的

API 定义 JSON-RPC 方法、资源、事件语义；transport 定义 message 如何在 client 与 runtime 之间搬运。

**transport 层不得解读业务 params**（如 `workspace` / `sessionId` / `runId`）。

## 1. Transport 矩阵

| 客户端形态             | runtime 位置      | transport               | 状态                   |
| ---------------------- | ----------------- | ----------------------- | ---------------------- |
| 桌面 / Web / CLI / TUI | 本地 runtime 进程 | HTTP loopback           | 已实现                 |
| Go host / CLI          | 同一进程          | public embedded binding | 可用                   |
| 未来远程 facade        | facade 之后       | HTTP                    | 同上，无需新 transport |

网络 binding 是 streamable HTTP，暴露 request/response JSON-RPC、server→client 通知、显式取消和基于 event id 的流重连。P19 新增公共类型化 embedded binding；它不是 Transport 的第三种 envelope 实现，不搬运 JSON-RPC message，而是直接调用 HTTP 同源的 operation pipeline。此前的 `internal` in-process channel 原型继续保持删除。桌面外壳当前走 loopback HTTP；这是当前打包实现，不是逻辑 Runtime identity 或永久产品拓扑。Desktop、CLI 等 Go host 若改用 embedded 或将来出现真实 socket binding，必须继续复用同一 operation pipeline。

## 2. 元数据划分 —— 业务与请求自描述进 params，纯传输才走带外

**原则**：JSON-RPC `params` 承载**业务 / 语义载荷**（这次操作"是关于什么"的数据），以及 `params._meta`
里的请求自描述信息（协议版本、clientInfo、clientCapabilities）。**只有纯传输 / 观测 / 可靠性元数据**才走带外通道
（HTTP header、`context.Context`）。

**判定一个字段放哪**：问"它是这次操作语义的一部分，还是只是承载这次操作的传输上下文？"

- **是语义的一部分 → params**（如 `WorkspaceRef` 决定资源根、`sessionId` / `runId` 指明操作对象）。
- **只是传输上下文 → 带外**（trace、门禁 token、流游标）。

走带外的（**非业务**）：

| 元数据           | Header / 字段                                                                 |
| ---------------- | ----------------------------------------------------------------------------- |
| Trace 上下文     | `traceparent`（+ `tracestate` / `baggage`）—— W3C TraceContext，OTel 标准注入 |
| 响应 method 标签 | `X-Method`                                                                    |
| 响应 server 标签 | `X-Server`                                                                    |
| 本地门禁 token   | `Authorization: Bearer <token>`                                               |
| 流重放游标       | `Last-Event-Id`（仅 `runs.subscribe` 续流，§9.2）                             |
| 幂等键           | `Idempotency-Key`（有副作用的调用，§6.2 / §10）                               |
| 幂等 store fence | `Idempotency-Namespace`（discovery 发布的 opaque store identity，§6.2 / §10） |

规则与易错点：

- `workspace` 是**资源身份**（业务）→ **进 params**，**永不**走带外 directory header。
- `sessionId` / `runId` = 业务 → 进 params。
- 协议版本 / clientInfo / clientCapabilities = 请求自描述 → `params._meta`。
- trace / 门禁 token / `Last-Event-Id` / idempotency identity = 传输上下文 → 带外。
- **不再有连接 id**：streamable HTTP 下事件属于"开它的那条 POST 流"，无需带外路由键（§6 / §8）。
- **JSON-RPC envelope `id` 必须是 string**（硬约束）。虽然 JSON-RPC 2.0 本身也允许 number id，本协议**只接受字符串 id**
  —— number / boolean / array / object / explicit `null` id 都在 SDK decode 前被拒为 `400 invalid_request`；通知必须省略 `id`。
  这样不会让 SDK 把 `null` 折叠成通知，或把小数、超大整数截断/饱和成另一个请求 id。本文所有示例用字符串即遵此约定。
  Lyra 的 streamable-HTTP binding 对无法解码的 request 返回 transport problem，不再生成带 `null` id 的 JSON-RPC error envelope。
- Envelope 是闭合形状：request/notification 只允许 `jsonrpc`、`id`、`method`、`params`；response 只允许
  `jsonrpc`、`id` 与 `result XOR error`。禁止 unknown member、`method` 与 `result/error` 混合、以及 `result+error` 双载荷。
  `POST /v2/rpc` 是 client→server binding，只接受 request/notification；客户端发送 response envelope 返回 `400 invalid_request`。

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

> `receive()` 的入站消息来自 HTTP **streamable** POST 响应（`application/json` 单条，或
> `text/event-stream` 多帧，§6.4）汇入同一条可迭代流。**没有"常开的 server→client 通道"这一前提** —— 响应
> 与通知都依附于某次调用。

## 4. Go 同进程接入边界

公共 `protocol` 是 HTTP 与 embedded 共用的唯一语义合同；公共 `embedded.Open` 返回 concrete Runtime。embedded 直接调用 binding-neutral operation pipeline，不启动 listener、不要求本地 token、不做 JSON-RPC/SSE 编解码。

`RequestMeta` 仍表达协议版本与客户端能力。写调用通过 `CommandOptions.IdempotencyKey` 提交稳定幂等身份；run 续流通过 `RunSubscriptionOptions.AfterEventID` 表达同一 replay cursor 语义。两者是语义准确的 Go option，不复用 HTTP header 名。请求 context 只约束当前调用/订阅，已接受 Run 由 Runtime lifecycle 继续拥有。

embedded 不公开 protocol method-group interface、Router、Host、Store、Engine、Application concrete、context private key、JSON-RPC numeric code 或 transport problem。消费方在自身边界定义所需窄接口。

HTTP executable 与 embedded 共用同一 instance builder、setup lease、ownership-aware recovery、workers 和 reverse-order shutdown。同一 canonical data directory 可以由少量 Runtime 进程共同打开；客户端仍只绑定一个 Runtime。冲突的同 Session writer/Goal drive 返回 `session_busy`，active Run 对 physical working tree 使用 shared lease，rollback/restore 使用 exclusive lease。另一个 SQLite connection 的提交通过 `runtime.subscribe` 既有 `resync` 语义促使客户端重读；不新增跨进程事件总线、TTL lease 或兼容宿主路径。

## 5. 当前不新增 Desktop IPC binding

当前 production Desktop 以 loopback HTTP 连接本地 Runtime 进程。本阶段不再增加 Wails / Tauri / Electron bridge 或 socket transport：

- 现有 HTTP binding 已拥有 JSON-RPC/SSE envelope、顺序、背压、流式、取消和重连；
- public embedded binding 已让同进程 Go host 直接复用同一 operation pipeline，无需复制协议与业务路径；
- sidecar 探针、CORS 和门禁 token 是 HTTP 专属 mechanism，不得泄露进 binding-neutral operation（§11 / §12 / §13）。

这是当前实现范围，不是“Desktop 永远只能是独立进程客户端”的产品裁决。未来桌面分发若因真实产品需求改用 embedded、socket 或宿主内存通道，新 binding 必须复用现有 operation/Application owner，并以该宿主真实需要的背压、取消和 lifecycle 反例证明自己；不能复制可靠性规则或建立平行 read model。

当前不为这些可能性预留 factory、registry、占位接口或 transport 矩阵。`Transport` 抽象（§3）只描述已经存在的网络消息管道；新的 binding 获得明确授权后再按其实际语义设计。

## 6. HTTP Transport

HTTP 用于浏览器与未来 facade 部署。采用 **streamable HTTP**：**流式方法的 POST 响应体本身就是这次操作的
事件流**，没有独立的"开流"连接（思想取自 OpenAI / Anthropic / MCP Streamable HTTP）。所有 server→client 消息
都依附于某个客户端调用，故无需常开的带外通道。

### 6.1 端点

| 端点                   | 用途                                                             |
| ---------------------- | ---------------------------------------------------------------- |
| `POST /v2/rpc`         | 所有 JSON-RPC 调用；method 只来自 envelope。响应为 JSON 或 SSE。 |
| `GET /v2/info`         | 公开的协议、服务与端点信息。                                     |
| `GET /v2/health/live`  | 只检查进程是否存活。                                             |
| `GET /v2/health/ready` | 并发执行依赖探针；不就绪时返回 503 和逐项检查结果。              |

> **没有独立的通知流端点**：每个流式调用的事件走它自己那条 POST 响应流（§6.4）。若将来真出现"带外 /
> 服务端主动推送"需求（多客户端同步、server→client request 等 —— 目前 API.md §13 明确不做），可**增量**加回
> 一条可选 `GET /v2/rpc/stream` 专收带外消息，不影响现有契约。

> **路径里的 `/v2/` 与 `protocolVersion`（日期串）是两个层级**：`/v2/` 是 wire major epoch（只有大破坏
> 才换路径前缀）；日期 `protocolVersion` 是该 epoch 内随 request metadata 发送的版本。两者不重复。

URL 不再重复 method，避免 URL 与 body 出现两个真相来源。所有调用统一发送到 `POST /v2/rpc`。

### 6.2 POST 契约

请求（流式与非流式同一形态；客户端声明它两种响应都能收）：

```http
POST /v2/rpc
Content-Type: application/json
Accept: application/json, text/event-stream
Idempotency-Key: 80bcab0c-77f5-4778-9934-fd8621683188
Idempotency-Namespace: idp_0123456789abcdef0123456789abcdef
traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01
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
      "protocolVersion": "2026-08-17",
      "clientInfo": { "name": "lyra-desktop", "version": "0.1.0" },
      "clientCapabilities": {
        "features": {},
        "interruptTypes": ["approval", "question"]
      }
    },
    "sessionId": "ses_...",
    "input": [{ "type": "text", "text": "hello" }]
  }
}
```

`Idempotency-Key` 与 `Idempotency-Namespace` 是传输元数据，不进入 params。携带 namespace 时，服务端在 claim key 与业务
admission 之前先与当前 durable store identity 比较；不匹配返回 `idempotency_store_mismatch`，不得触发 handler。随后服务端
原子 claim key，再执行并持久化第一份响应；同 key + 同请求在完成后
重放，首个执行未完成时返回 `idempotency_in_progress`，同 key + 不同请求返回 `idempotency_conflict`。因此并发请求不能越过
缓存重复落业务写入。流式 run 的重放会订阅既有 run；run 已结束时返回缓存成功响应并立即结束流，由客户端按正常断流恢复
路径重拉持久化状态。业务响应已生成但缓存 `Complete` 暂时失败时，服务端保留该响应并返回 `idempotency_in_progress`；后续
同 key 重试先补写/重放它，绝不重跑业务 handler。已完成结果保留 24 小时；过期后 key 可重新表示一次新操作，服务端会在后续
claim 时清理过期结果。没有 outcome 的 reservation 不按时间自动释放：进程崩溃后，时间流逝无法证明此前业务写入没有 commit，
自动释放会让同 key 第二次运行 handler。调用方若明确选择新的逻辑操作，必须使用新 key。

`runtime.discover.capabilities.limits.idempotency.namespace` 是 durable replay store 的 opaque 身份，不是路径、凭证或 transport
binding。需要跨客户端进程保存未决 key 时，客户端只用该 namespace 绑定 durable store；同一逻辑 Runtime 改用 HTTP、socket、
同进程 binding 或不同 endpoint 不得退休 exact key。数据目录被删除/重建时 namespace 会变化，旧 key 必须 fail closed，不得投递到
新 store。客户端只持久化 method/params 的不可逆匹配指纹、key、namespace 与 retention 时间，不能把
prompt、credential、文件内容或完整 params 写进 retry journal。取得确定 JSON-RPC 结果后立即删除记录；transport/protocol 结算不确定
时才保留。同进程的两个同形 fresh intent 仍必须获得不同 key，不能因 journal lookup 被合并。

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

- **流式方法**（`runs.start` / `runs.resume` / `runs.subscribe` / `runtime.subscribe`，以
  `streamingMethods` 为准）→ `200 text/event-stream`，响应体是这次操作的事件流（§6.4）。

> 流式方法**开流前**的失败（params 非法 / `session_not_found` / `session_has_active_run` / `stale_segment` /
> `replay_cursor_invalid` / `replay_unavailable` 等同步错误）仍返 `application/json` + JSON-RPC `error`，
> **不开流**；开流后的执行期错误走流内的 `segment.finished{outcome:{type:"error"}}` 帧。这正是 API.md §8.1 三个
> 落点在 HTTP 上的位置（RPC 落点走同步 error，run/item 落点走流内帧）。**一个开不了的流不会先返 200 再在流里
> 道歉** —— 那会让客户端把一次被拒绝的订阅当成一条空流等下去。

### 6.3 HTTP status

HTTP status 只描述传输层失败。

| status | 含义                                                                                                                                                                                |
| ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `200`  | JSON-RPC 响应已接受（`application/json` 单条，body 里仍可能含业务 error）**或** 流式响应已开启（`text/event-stream`，§6.4）。                                                       |
| `204`  | client 通知已接受、同步 dispatch 完毕；无 body。                                                                                                                                    |
| `400`  | HTTP body 读不出，或 JSON / envelope shape（含 duplicate/unknown member、request/response 混合、空 method、非字符串显式 id、client response）无法解码成 Lyra request/notification。 |
| `401`  | 本地门禁 token 缺失或错误。响应**必带** `WWW-Authenticate: Bearer`（RFC 9110 §15.5.2）。                                                                                            |
| `404`  | 未知 transport 端点。                                                                                                                                                               |
| `405`  | HTTP 方法错误。响应**必带** `Allow`（列出该端点支持的方法，RFC 9110 §15.5.6）。                                                                                                     |
| `413`  | HTTP body 超传输上限。                                                                                                                                                              |
| `415`  | content-type 不支持（仅在客户端**发了** content-type 且非 `application/json` 时判定；省略即放行）。                                                                                 |
| `500`  | envelope 之外的适配器失败：响应编码失败、流式响应不可用、handler panic。                                                                                                            |

> 状态码只描述传输层（RFC 9110）。**通知**同步处理完且无 body 用 `204`（非 `202` —— 后者语义是"已收下、
> 处理未决"）。**URL 从不携带 method**（§6.1），故不存在"URL 与 body 的 method 不一致"这种 `400`；envelope 一旦
> 解码成功，method / params 的一切问题都是 `200` + JSON-RPC error。Lyra 的 string-only id 是 envelope decode 约束，
> 因而非字符串显式 id 在 dispatch 前返回 `400`；`409` 不用于传输层。

除 `404` / `405` 等由 HTTP 路由器直接产生的标准响应外，transport 失败使用
`application/problem+json`，字段为 `{ type, title, status, detail, requestId? }`；`type` 是稳定的
`urn:lyra:transport:*` 判别键，`requestId` 与响应头 `Request-Id` 相同。客户端不得把 transport problem
误当作 JSON-RPC `error`。例如 JSON 无法解码为 JSON-RPC message 时返回 `400` +
`urn:lyra:transport:invalid_request`，而已经解码成功后的 method / params 错误仍返回 `200` JSON-RPC error。

当前的闭合 `type` 集：`invalid_request`（400）、`unauthorized`（401）、`request_too_large`（413）、
`unsupported_media_type`（415）、`response_encoding_failed` / `streaming_unsupported` / `internal_error`（500）。

**不要**把 `session_not_found` / `path_outside_root` 等业务错误映射成 HTTP status（业务错误走 JSON-RPC
`error`，见 API.md §8）。

### 6.4 流式方法响应（Streamable HTTP）

流式方法的 POST 响应体是一条 SSE 流，**承载这一次操作的完整 JSON-RPC 消息序列**：

1. **首帧 = 本次调用的 JSON-RPC 响应**（带请求的 envelope `id`），如 `runs.start` 的
   `{ "id":"1", "result":{ "runId":"run_…", "segmentId":"seg_…" } }` —— 客户端据此拿到这次要跟的 run 与 segment，
   无需单独的同步响应。此帧是一次性 ack，**不带 SSE `id:`**（它不属于可重放的 run 事件序列，§9.1）。
   `runs.subscribe` 的 ack 另带 `headEventId?`：**只许原样保存、作为后续 cursor**，不许比较或解释（API.md §7.3）；
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
data: {"jsonrpc":"2.0","id":"1","result":{"runId":"run_01","segmentId":"seg_01","userItemId":"item_00"}}

id: evt_0001
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_01","segmentId":"seg_01","eventId":"evt_0001","timestamp":"2026-08-02T10:00:00Z","event":{"type":"segment.started","run":{"id":"run_01","sessionId":"ses_01","status":"running","activeSegmentId":"seg_01","metrics":{"steps":0,"activeDurationMillis":0},"protocolProfile":{"requiredFeatures":[],"interruptTypes":["approval"]}}}}}

id: evt_0002
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_01","segmentId":"seg_01","eventId":"evt_0002","timestamp":"2026-08-02T10:00:01Z","event":{"type":"item.delta","itemId":"item_01","delta":{"type":"content","text":"Hello"}}}}

id: evt_0009
data: {"jsonrpc":"2.0","method":"notifications.run.event","params":{"runId":"run_01","segmentId":"seg_01","eventId":"evt_0009","timestamp":"2026-08-02T10:00:09Z","event":{"type":"segment.finished","outcome":{"type":"completed"},"metrics":{"steps":3,"activeDurationMillis":1500,"usage":{"inputTokens":120,"outputTokens":40,"costUsd":0.01}}}}}
```

> `RunEvent` 信封和 `StreamEvent` 都**不带 reliability flag**。authoritative /
> replayable 由 `event.type` 决定；`custom` 固定 non-authoritative、non-replayable。
> SSE 重放（§9）只按 API.md §5.2 的 replayable 分类决定 `id:`。

要点：

- **一次操作 = 一条流 = 一个 HTTP 交换**；`curl -N` 即可看全程，日志里一请求对应一操作。
- 该流承载**整棵 run 树**：子孙 subagent run 的事件并入此流（每帧带自己的 `runId`，客户端用
  `spawnedByItemId` 还原树，API.md §5.4 / §10.3）。`runtime.subscribe` 的流形状相同、但承载
  `notifications.runtime.event` 帧且**不可重放**（无 SSE `id:`，AUX_API §3.1）。
- **root `segment.finished` 才结束这条流**：子孙 run 的终态只是树里的一个节点结束了。
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

- 只有**可重放的事件帧**带 SSE `id:`，且 = 其 `eventId` —— replayability 由
  `event.type` 推导（API.md §5.2）。JSON-RPC 响应帧（首帧）**不带 `id:`**（一次性 ack，非可重放
  事件，§9.1）。`notifications.runtime.event` 同样不带（该流按 AUX_API §3.1 明确不补发，靠重订时的全量失效收敛）。
- `event:` / `retry:` 不使用；客户端忽略未知字段。
- 心跳用 SSE comment（`: ...` 行）。
- 流是 **POST** 而非 `EventSource` GET，故门禁 token 照常走 `Authorization: Bearer` —— "`EventSource` 不能设
  header / 只能用 cookie / 由此引入 CSRF 面"那套问题**随 streamable HTTP 一并消失**。

## 8. 运行树（一条流承载整棵树）

一个流式调用的响应流承载**整棵 run 树**的事件（根 + 所有子孙 subagent run）：

- 每帧带它所属的 `runId`；
- 子 run 以 `segment.started` 携带 `spawnedByItemId` 开始；
- 客户端用 `runId` + `spawnedByItemId` join 还原树（API.md §5.4 / §10.3）；
- 只有 root Run 的 frozen profile 含 `subagents` 时才产出 child Run 事件；当前 build
  支持该 feature，但每个创建/跟随树的请求仍须显式 opt in。

**不再有连接级路由**：事件天然属于"开它的那条 POST 响应流"，无 `X-Conn-Id`、无 run→连接登记表。

**一个 run 可被 ≥1 条流并发订阅（N fan-out）**：server 须支持把同一 run 的事件序列同时 fan-out 给多条 POST
响应流（多 tab、或重连时旧流未拆的短暂重叠），每条流从各自的 `Last-Event-Id` 之后续发（§9.2）。

## 9. 重放与重连

### 9.1 Event id

`eventId` 由 server 生成，**在一个 Segment 的事件序列内单调有序**（= API.md §2.4 的 "Segment 流"；含该段根 Run +
所有子孙 Run 的事件）。同一段序列**可能分布在多条 HTTP 响应里** —— 原始 `runs.start`/`runs.resume` 流 + 之后对该段的
`runs.subscribe` 续流 —— 单调性贯穿整段，故 `Last-Event-Id` 能线性重放。

- `runs.subscribe` 续流**沿用同一段序列**：从 `Last-Event-Id` 之后接着发（重放的 replayable 事件保持其原始
  `eventId`，客户端据此去重）；
- `runs.resume` 在同一 Run 上开**新的一段**（同 `runId`、新 `segmentId`、`eventId` 从头，API.md §2.4）。

**`Last-Event-Id` 是一个被解释的 cursor，不是一个数**：它编码了流的进程 epoch 与 scope（哪个 run 的哪一段），
所以

- 别的进程或别的段的 cursor 被**拒绝**（`replay_cursor_invalid`）而不是被将就解释 —— 后者会把重放落到另一条流上；
- 曾经合法、但已被保留窗口淘汰的位置返回 `replay_unavailable`。事件没了，但它们产出的 Item 已持久化：客户端
  冷读 `items.list` + 各 state key 的 recovery 方法，再**不带 cursor** 重接（= 只订将来）。

保留窗口的 scope 与容量在 `capabilities.limits.runReplay` 里公布**并被强制执行**（API.md §9）。server 只保留
replayable 事件以支撑续流；**正确性不得依赖 non-authoritative preview 的重放**。

### 9.2 续流流程（per-run）

某条 run 流断开时（run 在服务端继续）：

1. 客户端对该 run 调 `POST /v2/rpc`（envelope `method: "runs.subscribe"`、
   `params: { runId, segmentId }` —— URL 从不携带 method，§6.1），带
   `Last-Event-Id: <最后一个成功折叠的 eventId>`；
2. server 在新响应流里**重放该 id 之后的 replayable 事件**，再接上 live；
3. 客户端按 `eventId` 与 `itemId` 去重；
4. 若客户端本地状态仍不完整（或拿到 `replay_unavailable`），调用 `items.list` 重建持久化历史 + 各 state key 的
   recovery 方法补状态；`item.completed` / `state.snapshot` 保证终态正确（API.md §5.2）。

> 重连的是**具体某个 run 的某一段**，粒度准，且不需要维护连接身份。
>
> **cursor 只由"折叠成功"推进**：不要拿 ack 回来的 `headEventId` 覆盖自己的位置 —— 一次重放式重接的 head 在你
> 请求的位置**之前面**，采用它会静默跳过刚请求的那段重放（API.md §10.1）。只有在客户端一个事件都没折叠过、也没有
> 自己的 cursor 时，才用 ack 的 head 作起点。
> **段没结束就断流，是连接掉了，不是 run 结束了**：服务端那边它还在跑，续流是把这件事变成毫秒级空隙的唯一手段。

### 9.3 Delta 重放

server **不重放 non-replayable 事件：`item.delta`、`segment.progress`、`custom`**。

server **必须**通过 authoritative 的 `item.completed` / `state.snapshot` 与对应持久化读让最终态可得
（API.md §5.2 不变量）。

## 10. 创建请求重试

有副作用的调用应携带稳定的 `Idempotency-Key`，直到取得确定结果后才生成新 key。跨进程恢复还必须把 discovery 发布的
idempotency namespace 作为 `Idempotency-Namespace` 随每次 attempt 发送，并处于 retention window 内；客户端发送前校验与
服务端 admission fence 缺一不可。只读调用可按其业务语义重试。

## 11. 本地门禁 token

loopback HTTP 必须防止任意本地网页 / 本地进程访问 runtime。

约束：

- 数据目录没有 token 时生成随机值，并以 owner-only 权限原子发布到用户私有文件；
- token 由该 durable path 拥有，Runtime `instanceId` 换代或 SIGKILL 重启时复用同一值；凭据轮换是显式删除/替换该文件后的新初始化，不从进程 generation 推导；
- `/v2/rpc` 要求 `Authorization: Bearer <token>`；token 缺失/错误返 `401` + `WWW-Authenticate: Bearer`（RFC 9110 §15.5.2）；
- `/v2/health/live`、`/v2/health/ready` 免 token；
- `/v2/info` 仅在不含 secret 时免 token。

token 是本地 transport 门禁，**不是用户鉴权**（协议层零 user 概念，见 API.md §15）。

## 12. Sidecar 端点（HTTP 专属）

sidecar 端点不走 JSON-RPC（扁平 JSON、无需 discovery、无鉴权）。

**它们只在 HTTP transport 存在**，因为只有 HTTP 才有这种运维场景：`curl` / oncall 探活、k8s
liveness/readiness、反代 upstream 健康检查 —— 这些要的是"不套 envelope、无鉴权"的端点。

| 需求                           | HTTP                   |
| ------------------------------ | ---------------------- |
| 最小 binding / 协议 / 进程身份 | sidecar `GET /v2/info` |
| 存活探测                       | `GET /v2/health/live`  |
| 就绪探测                       | `GET /v2/health/ready` |

`/v2/info` 只公开接入所需信息，不泄露 workspace path、home、能力快照或内部依赖。

> **规律：非 JSON-RPC 的 HTTP 旁路只承载 HTTP 运维场景。**（图片输入不属于此类——它内联在 `runs.start.input` 的 image ContentBlock 里走常规 JSON-RPC，
> 无独立二进制上传通道。）

### 12.1 `GET /v2/health/live` 与 `GET /v2/health/ready`

```json
{ "instanceId": "runtime_019c765b-2f8f-7e36-a2b4-31cb11f48d10", "status": "ok" }
```

live 只返回 200；ready 在依赖异常时返回 503，并携带 `checks`。两者的 `instanceId` 必须与同一进程的
`/v2/info.server.instanceId` 和 `runtime.discover.serverInfo.instanceId` 完全一致。client 只有在一次 inspection 的
四份 identity 全部一致时才能提交 ready；进程交错期间拼出的 cohort 必须整体丢弃。

### 12.2 `GET /v2/info`

```json
{
  "protocolVersion": "2026-08-17",
  "server": {
    "instanceId": "runtime_019c765b-2f8f-7e36-a2b4-31cb11f48d10",
    "name": "lyra-runtime",
    "version": "0.0.0"
  },
  "transport": "http",
  "endpoints": {
    "rpc": "/v2/rpc",
    "info": "/v2/info",
    "liveness": "/v2/health/live",
    "readiness": "/v2/health/ready"
  }
}
```

`instanceId` 是不透明、每次进程启动都变化的 incarnation identity；不得持久化为业务 identity，也不得替代
discovery 中跨重启稳定的 idempotency namespace。

## 13. CORS

loopback HTTP 应限制 origin。

推荐默认：

- 放行内置客户端 origin；
- 放行显式配置的开发 origin；
- 启用本地门禁 token 时拒绝通配 origin；
- 允许 header：`Content-Type`、`Authorization`、`Idempotency-Key`、`Idempotency-Namespace`、`Last-Event-Id`、`traceparent`、`tracestate`、`baggage`；
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

server 可在高负载下**合并 non-authoritative preview**，只要 authoritative 收敛不变量仍成立（API.md §5.2）。

server **不得静默丢弃 replayable 事件**；订阅者积压超过窗口时必须终止该流，让客户端显式重连或冷恢复。

失效流（`runtime.subscribe`）的背压规则不同，且更严：来不及投递的失效**合并成一条点名 topic 的 `resync`**，
而不是丢帧后指望客户端从空号里发现（AUX_API §3.1）。序号只发给真正进入队列的帧 —— 为一个被合并掉的信号消耗
号，等于让客户端以为丢了东西。

## 16. Observability

每个 transport 适配器应记录：method、envelope id、trace 上下文、协议版本、时长、JSON-RPC error code
（若有）、传输 status。流式响应另记 `runId` 与该流发出的事件数。请求的 trace 关联走入站 `traceparent`
（W3C TraceContext）；HTTP 响应在可用时应带 `X-Method` / `X-Server`。

> **反向不变量**：workspace path 不进高 cardinality metric label（路径无界会爆 Prometheus，需要时只在结构化
> 日志记 hash / basename）；PII（消息 / prompt 内容）不进 access log / metric。

## 17. 安全边界

| 层               | 负责                                                                                                     |
| ---------------- | -------------------------------------------------------------------------------------------------------- |
| transport 层     | 本地门禁 token 校验、origin 检查、body 大小限制、content-type 检查、流响应生命周期                       |
| API / runtime 层 | workspace 下的 path containment、URL fetch egress 策略、工具审批策略、能力声明匹配、provider secret 处理 |

## 18. v2 不支持

- WebSocket transport。
- stdio transport。
- JSON-RPC batch。
- server→client JSON-RPC request。
- 客户端自选的业务资源 id。
- 连接级 active project 状态。

---

> 正式契约。配套同目录 [`API.md`](./API.md)。
