# Lyra 传输层

> 描述 `API.md` 定义的 **Lyra Runtime Protocol**（JSON-RPC 2.0）
> 在表现层 ↔ Runtime 之间**物理上**怎么传输。协议在所有传输上
> 完全一致；**传输层**就是负责把 JSON-RPC message（或者在 Go-to-Go
> 情况下，直接把 struct 指针）从一边搬到另一边的东西。
>
> 读者：要做新表现层、要把 Runtime 嵌入到新外壳里、或者要为特定
> 部署写适配器（HTTP → in-process、Wails IPC → in-process 等）的人。

---

## 1. 部署矩阵

Lyra Runtime 永远是**单一形态**（本地或同进程；远程模式留给未来
facade，详见 `API.md` 附录 B）。表现层有 3 种主要形态。每种挑
**满足本组合进程 / 网络边界的最便宜的传输**：

| 表现层 | Runtime 在哪 | 推荐 transport | 备注 |
| --- | --- | --- | --- |
| **TUI（Go，如 Bubble Tea）** | 同一 Go 二进制 | **InProcess** | 直接持有 Runtime 对象，函数调用，零序列化 |
| **包装客户端（Wails / Tauri / Electron）** | 宿主进程 | **Wails IPC** | WebView ↔ 宿主进程的 IPC bridge |
| **Web（浏览器）** | 同机独立进程（守护） | **HTTP loopback** | 浏览器没法嵌后端，必走 HTTP；本地 token 文件做进程门禁 |

未来 facade 模式（**本轮不实现**）：任意表现层走 **HTTP** 连云端
facade，facade 内部再转发到 Runtime。详见 `API.md` 附录 B。

三种 transport 共享**同一个 Go interface**，handler 代码
（`Router.Handle(msg)`）跟 transport 选哪个无关。

---

## 2. 唯一抽象 —— `Transport`

两端都对接同一个 Go interface。`Transport` 就是 JSON-RPC message
的双向管道——**剩下的一切（HTTP framing、SSE 解析、Wails IPC
bridge）都藏在接口背后**。

```go
// pkg/transport/transport.go
package transport

import (
    "context"
    "encoding/json"
)

// Message is one JSON-RPC 2.0 envelope — Request, Response, or
// Notification. Per spec, only the corresponding fields are populated:
// Request:      jsonrpc + id + method + params?
// Response:     jsonrpc + id + (result XOR error)
// Notification: jsonrpc + method + params?  (no id)
type Message struct {
    JSONRPC string          `json:"jsonrpc"`           // always "2.0"
    ID      json.RawMessage `json:"id,omitempty"`      // string|number, absent on Notification
    Method  string          `json:"method,omitempty"`  // Request / Notification
    Params  json.RawMessage `json:"params,omitempty"`  // method-specific
    Result  json.RawMessage `json:"result,omitempty"`  // Response success
    Error   *RPCError       `json:"error,omitempty"`   // Response failure
}

type RPCError struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}

// Transport is a bidirectional message pipe. One interface, three
// implementations in scope (InProcess, Wails IPC, HTTP).
type Transport interface {
    // Send queues an outbound message (Request / Response / Notification).
    // Returns when the message is handed to the underlying transport,
    // not when the peer has processed it.
    Send(ctx context.Context, msg *Message) error

    // Recv returns a channel of incoming messages. The channel is
    // closed when the transport disconnects. Consumers must drain it.
    Recv() <-chan *Message

    // Close terminates the transport. Any pending Send fails with
    // context.Canceled; Recv channel is closed.
    Close() error
}
```

注意：`Transport` 是**纯 message pipe**，不区分 Request / Response
配对——配对（按 `id` match response）是上一层 `RPCClient` 的事。
Streaming 也是 `Recv()` 持续吐 Notification 而已，不需要专门
"Stream" 字段。

前端永远见不到 `*http.Request` 或 `net.Conn`。后端的 router 签名
是 `func(ctx context.Context, msg *Message) *Message`——同一份
代码跑在所有传输背后。

---

## 3. 服务契约 —— `pkg/coreapi`

`Transport` 搬运的是不透明 JSON-RPC envelope。**类型化的 Go
interface**——两端真正同意的那份契约——位于其上一层 `pkg/coreapi`：

```go
// pkg/coreapi/coreapi.go
package coreapi

import "context"

type CoreAPI interface {
    // Lifecycle
    Initialize(ctx context.Context, in InitializeIn) (*InitializeOut, error)
    Shutdown(ctx context.Context, in ShutdownIn) error

    // Sessions
    ListSessions(ctx context.Context, q ListSessionsQuery) (*Page[Session], error)
    GetSession(ctx context.Context, id string) (*Session, error)
    CreateSession(ctx context.Context, in CreateSessionIn) (*Session, error)
    // ...

    // Messages
    ListMessages(ctx context.Context, sessionID string, q ListMessagesQuery) (*Page[Message], error)

    // Runs (streaming surface)
    StartRun(ctx context.Context, in StartRunIn) (*StartRunOut, <-chan AgUiEvent, error)
    CancelRun(ctx context.Context, runID string) error
    SubmitApproval(ctx context.Context, in ApprovalIn) error

    // ... full surface mirrors API.md §5.2 (the JSON-RPC method table)
}
```

这个 interface 是**行为的 single source of truth**。从它衍生出
两份产物：

- **JSON-RPC method 表** + **AsyncAPI 流式事件 schema**（`schemas/`）
  ——扫描 struct tag 生成，给非 Go 客户端（TS / Rust / Python）用
- **JSON-RPC 适配器**（`pkg/rpcadapter`）——把 inbound JSON-RPC
  `Message` 路由到 `CoreAPI` 方法，再把结果重新编码成 outbound
  `Message`

所以结构是：

```
              ┌────────────────────┐    Transport.Send(msg)
   Frontend ──│  RPCClient(typed)  │─────────────────────────► wire
              │  + CoreAPI mirror  │◄──── Transport.Recv() ────
              └────────────────────┘
                                                                ↓
              ┌────────────────────┐    Router.Handle(msg)
   Backend  ──│  CoreAPI impl      │◄────────────────────────── wire
              │  + RPC adapter     │─── Transport.Send(reply) ─►
              └────────────────────┘
```

适配器形态因传输而异：

| 传输 | 适配器形态 |
| --- | --- |
| InProcess | 前端 `Client` 直接持有一个 `CoreAPI` 引用，**完全跳过 JSON-RPC 序列化**——`client.ListSessions(...)` 直译为 `impl.ListSessions(...)`。`Transport` 接口在此场景仍可选保留，只为复用 logging / tracing middleware。 |
| Wails IPC / HTTP | 适配器把 JSON-RPC `Message` 解码为 method name + params struct，调对应 `CoreAPI` 方法，把返回值打回 `Message`。 |

---

## 4. 三种传输实现

### 4.1 InProcessTransport —— Go ↔ Go，同一二进制

**适用**：链接了 `pkg/coreimpl/` 的 Bubble Tea TUI（或任何 Go
表现层）。两端共享 event loop。

```go
// pkg/transport/inprocess/inprocess.go
type InProcessTransport struct {
    api coreapi.CoreAPI // direct reference, no serialization
    in  chan *Message   // notifications routed back to client
}

func (t *InProcessTransport) Send(ctx context.Context, msg *Message) error {
    // For requests, dispatch directly to CoreAPI and stuff the response
    // into t.in. For notifications, route to the in-process router.
    // Skips all JSON marshaling.
    return dispatchInProc(ctx, t.api, msg, t.in)
}

func (t *InProcessTransport) Recv() <-chan *Message { return t.in }
```

实战中 **Go 表现层直接持有 `CoreAPI` 对象就够了**——根本不需要
`Message` 往返：

```go
// pkg/coreclient/inproc.go
func NewInProcClient(api coreapi.CoreAPI) coreapi.CoreAPI {
    return api // same interface, returned as-is
}
```

`Transport` 接口此时只是为了让 logging / tracing middleware 可以
统一注入，**业务路径完全跳过 JSON-RPC 序列化**。Bubble Tea 的
`tea.Cmd` 返回 `tea.Msg`，让 `AgUiEvent` 实现 `tea.Msg` → 从
`StartRun` 的 `<-chan AgUiEvent` 直接 stream 到 TUI 的 update loop。

成本：~50 ns / 调用（Go interface dispatch）。流式吞吐只受 channel
buffer 限制。

### 4.2 WailsTransport —— 包装外壳 ↔ 嵌入式 Go Runtime

**适用**：Wails / Tauri / Electron，Runtime 跑在宿主进程、前端
在 WebView 里。

```go
// pkg/transport/wails/wails.go
type WailsTransport struct {
    runtime wails.Runtime
    in      chan *Message
}

func (t *WailsTransport) Send(ctx context.Context, msg *Message) error {
    // WebView ↔ host process IPC. For Request/Notification we route
    // to the JSON-RPC adapter; for streaming we use Wails EventsEmit
    // to push notifications back through t.in to the TS shim.
    return t.runtime.Call(ctx, marshalMsg(msg))
}
```

前端是一个 TS 薄壳，对外暴露同样的 `Transport` 形状：

```ts
// frontend/src/transport/wails.ts
export const wailsTransport: Transport = {
  async send(msg) {
    // Wails IPC posts the JSON-RPC envelope to the host. Streaming
    // notifications come back via EventsOn and are fed into the
    // Recv() AsyncIterator.
    await window.go.transport.Send(msg);
  },
  recv() { return wailsEventStream; },
  close() {},
};
```

成本：~30 µs / 调用（Wails IPC + structured-clone）。完全避开 HTTP
framing + TCP 栈，又不动 WebView。

### 4.3 HTTPTransport —— 浏览器 + 未来 facade

**适用**：浏览器表现层（无法嵌入 Runtime，必走网络），以及未来
通过云端 facade 连远程 Runtime 的所有场景。

**线协议（详见 API.md §10）**：

| Endpoint | 用途 |
| --- | --- |
| `POST /v1/rpc/{method}` | **唯一 JSON-RPC entry**。Method 名在 URL（如 `/v1/rpc/runs.start`，点保留、禁止斜杠化），body 里仍含 `method` 字段做 cross-check（不匹配返 `409 + -32011`）。Ops 友好：反代 / k8s / 浏览器 DevTools 直接按 method 分桶。**不接受无 method 后缀的 `POST /v1/rpc`**（greenfield 决议）。 |
| `GET /v1/rpc/stream` | SSE 流，server → client notifications。每个 SSE event 的 `data:` 字段是一条 JSON-RPC Notification。带 `Last-Event-Id` header 续传。 |
| `GET /v1/info` | **Sidecar metadata**。扁平 JSON（不走 envelope）。无鉴权。详见 API.md §9。 |
| `GET /v1/health` | **Sidecar liveness**。扁平 JSON，HTTP 200/503。无鉴权。详见 API.md §9。 |

**HTTP status code 映射**（详见 API.md §7.3）：业务错误一律走
JSON-RPC `error.code`，**不映射 HTTP status**。HTTP status 仅反映
transport 层状态（200 / 204 / 400 / 404 / 409 / 401 / 500 / 503）。

```ts
// frontend/src/transport/http.ts
export const httpTransport = (baseUrl: string, localToken?: string): Transport => {
  const incoming = new EventTarget(); // pumps Recv() channel
  const sse = new EventSource(`${baseUrl}/v1/rpc/stream`, { withCredentials: false });
  sse.onmessage = (e) => { /* feed JSON.parse(e.data) into Recv channel */ };

  return {
    async send(msg) {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (localToken) headers["Authorization"] = `Bearer ${localToken}`;
      // Single-form: POST /v1/rpc/{method}. Server rejects requests
      // without method suffix with 404 (greenfield, no compat fallback).
      // Response messages don't carry method, so this only applies to
      // outbound Request / Notification.
      if (!msg.method) {
        throw new Error("HTTP transport only sends Request / Notification messages");
      }
      const url = `${baseUrl}/v1/rpc/${msg.method}`;
      await fetch(url, {
        method: "POST",
        headers,
        body: JSON.stringify(msg),
      });
    },
    recv() { /* return AsyncIterator backed by `incoming` */ },
    close() { sse.close(); },
  };
};
```

`localToken` 仅在本地进程门禁场景使用（Web 前端连同机 Runtime），
读自 `~/.lyra/local-token`。其他 HTTP 场景（未来 facade）由 facade
层自己处理鉴权 + 把请求转发给 Runtime，**Runtime 看到的 HTTP
依然是同一个 `/v1/rpc[/{method}]` 入口**。

**Observability 强制要求**（详见 API.md §10）：服务端必须在响应
header 暴露 `X-Lyra-Method` / `X-Lyra-Request-Id` / `X-Lyra-Server`；
必须输出结构化日志 + Prometheus metric with bounded-cardinality
`method` label。

成本：本地 ~200 µs / 调用，WAN ~20–200 ms。

### 4.4 按场景挑选

| 场景 | 推荐传输 | 理由 |
| --- | --- | --- |
| Bubble Tea TUI + Go Runtime（同二进制） | **InProcess** | 同一进程、两端都是 Go——直接函数调用比 IPC 快 600×。 |
| Wails / Tauri / Electron 客户端 + Go Runtime | **Wails IPC** | Wails 已替你付过这笔成本；不用占额外端口；macOS 上不弹防火墙提示。 |
| 浏览器（任意宿主） | **HTTP** | 浏览器唯一选项。本地模式走 loopback + 进程门禁 token；未来云端模式走 facade。 |

口诀：**能不序列化就不序列化**。In-process 免费，IPC 便宜，HTTP 是
天花板。

非 Go TUI（Node / Rust / Python 等）/ Unix socket / WebSocket 这些
形态本轮**不实现**——本地 Lyra 不需要跨语言进程间通讯，远程访问
等做 facade 时再加。

---

## 5. 流式 —— 所有传输统一语义

每个传输都通过 `Recv()` 通道吐出**同一种 JSON-RPC Notification 流**
（主要是 `notifications/run/event`，每条 notification 的 `params`
里带 `eventId` + AG-UI `event`）。这意味着 **resume / replay 只在
协议层实现一次，不是每个传输各搞一份**。

| 传输 | 底层机制 |
| --- | --- |
| InProcess | Notification 的 Go channel |
| Wails IPC | `EventsEmit` + JS 侧重组成 AsyncIterator |
| HTTP | SSE（`text/event-stream`），每个 SSE event 的 `data:` 字段是一条 JSON-RPC Notification |

客户端的续传逻辑——"重连、发送 `Last-Event-Id: <last>`、Runtime
从该 stream 内 `eventId > lastEventId` 处 replay"——在每个传输上
都用同一份代码，因为它们都暴露同一个 `eventId` 序列。

对于 in-process，"resume" 退化（channel 是进程内的、随进程一起死），
客户端只要检查它的 `<-chan` 是不是被 close 了、然后重启。**同一
份代码路径，不同的物理。**

---

## 6. 鉴权 & 边界

| 传输 | 鉴权模型 |
| --- | --- |
| InProcess | 无——同进程互信。Runtime 协议层零用户概念，运行环境的信任 = OS 用户的信任。 |
| Wails IPC | 线上无（Wails IPC 是 window-scoped）；同进程内的等价信任。 |
| HTTP（本地 loopback） | **本地进程门禁 token**（不是用户鉴权）。Runtime 启动时随机生成、写到 `~/.lyra/local-token`（chmod 600）。仅阻止同机其他进程乱调，**不验证用户是谁**。 |
| HTTP sidecar（`/v1/info` + `/v1/health`） | **永远无鉴权**——curl-friendly read-only metadata，详见 API.md §9 |
| HTTP（未来 facade） | 由 facade 层处理用户鉴权、转发到内部 Runtime；Runtime 看到的请求**仍是同样形态**，不感知 facade 在做什么。 |

**核心不变量**：`CoreAPI` impl **永远不做用户鉴权 / 授权**——
Runtime 协议层根本没有 user / account / owner 概念。需要这些
能力时，由更外层（OS、本地 token 门禁、未来 facade）解决。

---

## 7. Schema 流 —— 一份契约，多个发射器

```
  pkg/coreapi/*.go  (Go interface + structs)
            │
            ├── go-jsonrpc codegen ──→  schemas/methods.yaml   (JSON-RPC method table)
            │                                  │
            │                                  └── jsonrpc-ts ──→ frontend/src/lib/runtime-types.ts
            │
            └── go-asyncapi codegen ──→  schemas/events.yaml   (AG-UI streaming events)
                                               │
                                               └── asyncapi-ts ──→ frontend/src/lib/events.ts
```

对 Go ↔ Go（in-process / Wails）来说，**根本不需要 codegen**——
两边直接 import `pkg/coreapi`。其他场景下，schema 才是契约，
TS / Rust / Python 客户端都从它生成。

---

## 8. 目标文件布局

```
pkg/
├── coreapi/                 # SSOT — Go interface + types + struct tags for codegen
│   ├── coreapi.go
│   ├── lifecycle.go
│   ├── sessions.go
│   ├── messages.go
│   └── runs.go
├── coreimpl/                # implementation (calls LLM, runs tools, stores state)
│   └── ...
├── transport/
│   ├── transport.go         # the Transport interface + Message envelope
│   ├── inprocess/
│   ├── wails/
│   └── http/
├── rpcadapter/              # JSON-RPC Message ↔ CoreAPI method dispatch
│   └── ...
└── coreclient/              # client-side wrappers
    ├── inproc.go            # returns CoreAPI as-is
    ├── wails.go             # wraps WailsTransport
    └── http.go              # wraps HTTPTransport

cmd/
├── lyra-tui/                # Bubble Tea TUI — imports coreapi + coreimpl directly
└── lyra-desktop/            # Wails shell — imports coreimpl + transport/wails

frontend/src/transport/
├── transport.ts             # TS mirror of the Transport interface
├── wails.ts                 # Wails IPC transport
└── http.ts                  # HTTP transport (loopback + future facade)

schemas/                     # generated, in-repo
├── methods.yaml             # JSON-RPC method table
└── events.yaml              # AG-UI streaming events
```

---

## 9. 这套切分为什么干净

- **加传输不动 handler 代码。** 新形态——比如未来 facade 内部
  跨服务调用走 gRPC——实现 `Transport`、注册一个调 `CoreAPI` 的
  适配器，完事
- **加前端不分裂后端。** 新形态——比如把桌面客户端用 Tauri 重写
  一遍——挑跟它外壳合的传输（Tauri IPC ≈ Wails IPC；浏览器版本
  回落 HTTP）、实现 TS 侧 `Transport`、其他都复用
- **Go-to-Go 不是特例。** 它用同一个 `Transport` 形状，只是实现
  是个空壳：客户端直接持有 `CoreAPI` 值。Bubble Tea、内部 CLI、
  以后 Go-native 插件全走这条路径
- **HTTP 不享有特权。** `pkg/rpcadapter/` + `pkg/transport/http/`
  只是三个传输之一。协议里没有把 HTTP-isms 烧进 `CoreAPI`（没有
  `Request *http.Request` 参数、没有 `http.ResponseWriter` 返回
  值）。测试用 in-process；生产挑部署适合的

---

## 10. 未决问题

- **gRPC？** 能干净装进 `Transport`，但会在 JSON-RPC schema 之外
  多一份 proto schema 作为 SSOT。等真有非 MVP 场景再做（最可能
  是未来 facade ↔ Runtime pool 内部互联）
- **HTTP 流式用 WebSocket 还是 SSE？** 今天用 SSE。只有当 HITL
  需要 run 进行中 client → server 推送的时候才换 WS（当前不需要——
  approval 走普通 `runs.approval.submit` Request）
- **运行时热切换传输？** 比如同一 process 启动时挑 InProcess、
  用户切换"远程模式"时回落 HTTP。可行——`Transport` 就是个接口——
  但还没 UI 入口

---

## 11. 速查表

```
同一 Go 二进制、两端都是 Go            →  InProcessTransport (无 codegen, ~50ns)
Wails / Tauri / Electron + Go Runtime  →  WailsTransport      (~30µs)
浏览器、本地 Runtime                   →  HTTPTransport + 本地 token 门禁 (~200µs)
浏览器、未来云端 facade                →  HTTPTransport + facade 处理鉴权
```

一个 `Transport` 接口、三种实现。挑能跨过你的边界的最便宜那个
盒子，剩下的协议层零代码改动。
