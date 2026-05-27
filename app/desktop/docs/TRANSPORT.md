# Lyra 传输层

> 描述 `API.md` 定义的协议在 Lyra 前端和后端之间**物理上**怎么传输。
> 协议在所有部署形态下保持一致；**传输层**就是负责把字节（或者
> 在 Go-to-Go 情况下，直接把 struct 指针）从一边搬到另一边的东西。
>
> 读者：要做新前端、要把后端嵌入到新外壳里、或者要为特定部署写
> 适配器（HTTP → in-process、socket → in-process 等）的人。

---

## 1. m × n 矩阵

3 种后端部署 × 3 种前端形态 = 9 种可行组合。每个格子挑**满足本组合
进程 / 网络边界的最便宜的传输**。

| ↓ 前端 / 后端 → | Local（loopback） | Server（局域网 / 公网） | Embedded（同一进程） |
| --- | --- | --- | --- |
| **TUI（Go，如 Bubble Tea）** | Socket / HTTP | HTTP | **In-process** |
| **包装客户端（Wails / Tauri / Electron）** | Socket / HTTP | HTTP | Wails IPC / HTTP |
| **Web（浏览器）** | HTTP | HTTP | n/a（浏览器无法嵌后端） |

矩阵释义：

- **Local**：后端作为独立 OS 进程跑在同一台机器上（比如 `lyra-server`
  作为 systemd unit、或者一个 per-user 守护进程）。前端通过 Unix
  socket / Named pipe（最便宜）或 loopback HTTP 连上去。
- **Server**：后端是远程服务。只有 HTTP 能用——其他方式都假设
  共驻一台机器。
- **Embedded**：后端代码和前端链接进同一个二进制。如果两边都是 Go，
  边界就是一次函数调用；如果前端是 WebView（Wails / Tauri / Electron），
  边界就是外壳本身已提供的 IPC bridge。

六种独立传输共享**同一个 Go interface**；handler 代码
（`Router.Dispatch(req)`）在它们之间不变。

---

## 2. 唯一抽象 —— `Transport`

两端都对接同一个 Go interface。前端拿到的 `Client` 包一层 `Transport`；
后端用 `Server` 把 `Transport` 接到 `Router` 上。**剩下的一切——HTTP
framing、SSE 解析、socket 缓冲——都藏在这个接口背后。**

```go
// pkg/transport/transport.go
package transport

import "context"

// Request is the wire payload — a method name plus opaque bytes.
// For Go-to-Go in-process the bytes are skipped (see InProcessTransport).
type Request struct {
    Method  string            // "v1.session.send", "v1.message.list", ...
    Headers map[string]string // auth, idempotency-key, trace context
    Body    []byte            // JSON-encoded per API.md, OR nil for in-process
    Stream  bool              // true → response is an event stream (SSE-equivalent)
}

type Response struct {
    Status  int               // mirrors HTTP semantics even when not on HTTP
    Headers map[string]string
    Body    []byte            // for non-streaming
    Events  <-chan Event      // populated when Request.Stream == true
}

type Event struct {
    ID   string  // monotonic per stream — replay key
    Type string  // "TEXT_MESSAGE_CONTENT", "RUN_FINISHED", ...
    Data []byte  // JSON-encoded event payload
}

// Transport is what every adapter implements. One interface, six
// implementations (in-process, Wails, Unix socket, named pipe, HTTP, WS).
type Transport interface {
    // Call performs one request. Honors ctx cancellation. For streaming
    // requests, Response.Events is non-nil and the caller drains it.
    Call(ctx context.Context, req *Request) (*Response, error)
    // Close releases any underlying resources (sockets, channels, ...).
    Close() error
}
```

前端永远见不到 `*http.Request` 或 `net.Conn`。后端的 handler 签名
是 `func(ctx, *Request) (*Response, error)`——同一份代码跑在所有
传输背后。

---

## 3. 服务契约 —— `pkg/coreapi`

`Transport` 搬运的是不透明字节。**类型化的 Go interface**——两端
真正同意的那份契约——位于其上一层 `pkg/coreapi`：

```go
// pkg/coreapi/coreapi.go
package coreapi

import "context"

type CoreAPI interface {
    // Sessions
    ListSessions(ctx context.Context, q ListSessionsQuery) (*Page[Session], error)
    CreateSession(ctx context.Context, in CreateSessionIn) (*Session, error)
    // Messages
    ListMessages(ctx context.Context, sessionID string, q ListMessagesQuery) (*Page[Message], error)
    // Runs (the streaming surface)
    StartRun(ctx context.Context, in StartRunIn) (<-chan RunEvent, error)
    CancelRun(ctx context.Context, runID string) error
    // HITL
    SubmitApproval(ctx context.Context, in ApprovalIn) error
    // ... full surface mirrors API.md §4
}
```

这个 interface 是**行为的 single source of truth**。从它衍生出两份
产物：

- **OpenAPI 3.1 + AsyncAPI 2.6** 文件（`schemas/`）——扫描 struct
  tag 生成，给非 Go 客户端（TS、Rust、Python）用。
- **HTTP server 适配器**（`pkg/httpserver`）——把进来的 `*Request`
  路由到 `CoreAPI` 的方法，再把结果重新编码成字节。

所以结构是：

```
              ┌───────────────────┐
   Frontend ──│   CoreAPI client  │── uses Transport ──→ wire
              └───────────────────┘
                                                            ↓
              ┌───────────────────┐
   Backend  ──│  CoreAPI impl     │── exposed by Adapter ─→ wire
              └───────────────────┘
```

适配器形态因传输而异：

| 传输 | 适配器形态 |
| --- | --- |
| InProcess | 前端的 `Client` 直接持有一个 `CoreAPI` 引用。**没有适配器**——`client.ListSessions(...)` 就是 `impl.ListSessions(...)`。 |
| Wails / Socket / HTTP | 适配器在 `*Request` ↔ `CoreAPI` 方法调用之间做编解码。 |

---

## 4. 六种传输实现

### 4.1 InProcessTransport —— Go ↔ Go，同一二进制

**适用**：链接了 `pkg/coreimpl/` 的 Bubble Tea TUI；或者 Wails 应用
里我们选择绕开 WebView bridge、走纯 Go 路径的情况。

```go
// pkg/transport/inprocess/inprocess.go
type InProcessTransport struct {
    api coreapi.CoreAPI // direct reference, no serialization
}

func (t *InProcessTransport) Call(ctx context.Context, req *Request) (*Response, error) {
    // For in-process we DON'T marshal through Request.Body — the client
    // wrapper bypasses bytes entirely and calls api.* directly. This
    // implementation exists only for codepaths that genuinely need the
    // generic Request shape (e.g. shared logging middleware).
    return dispatchInProc(ctx, t.api, req)
}
```

实战中 **Go 客户端直接把 `CoreAPI` 包一层就完事**——in-process 情况
不需要 `Request`/`Response` 往返：

```go
// pkg/coreclient/inproc.go
func NewInProcClient(api coreapi.CoreAPI) coreapi.CoreAPI {
    return api // same interface, returned as-is
}
```

这就是全部节省所在：**没有 JSON、没有 binding、没有 codegen**。
Bubble Tea 的 `tea.Cmd` 返回 `tea.Msg`，我们让 `RunEvent` 实现
`tea.Msg`，从 `StartRun` 直接 stream 到 TUI 的 update loop。

成本：~50 ns / 调用（Go interface dispatch）。流式吞吐只受 channel
buffer 限制。

### 4.2 WailsTransport —— 包装外壳 ↔ 嵌入式 Go 后端

**适用**：Wails / Tauri / Electron，后端跑在宿主进程里、前端在
WebView 里。

```go
// pkg/transport/wails/wails.go
type WailsTransport struct {
    runtime wails.Runtime
}

func (t *WailsTransport) Call(ctx context.Context, req *Request) (*Response, error) {
    // Wails IPC: WebView posts the Request as a message, host routes it
    // to a registered handler. For Stream=true we use Wails' EventsEmit
    // to push events back to the WebView, where a thin shim reassembles
    // them into a channel-like iterable for the TS client.
    ...
}
```

前端是一个 TS 薄壳，对外暴露同样的 `Transport` 形状：

```ts
// frontend/src/transport/wails.ts
export const wailsTransport: Transport = {
  async call(req) {
    if (req.stream) {
      const events = makeEventStream(req.method); // wraps Wails EventsOn
      window.go.transport.Stream(req); // fire-and-forget; events flow back
      return { status: 200, headers: {}, events };
    }
    return window.go.transport.Call(req);
  },
  close() {},
};
```

成本：~30 µs / 调用（Wails IPC + structured-clone）。完全避开 HTTP
framing + TCP 栈，又不动 WebView。

### 4.3 SocketTransport —— 跨进程 loopback（Unix socket / Named pipe）

**适用**：语言 X 写的 TUI 跟跑在旁边的 Go 后端通讯。比 HTTP loopback
便宜（没有 TCP、没有 header parsing），但仍然跨进程，所以需要
序列化。

```go
// pkg/transport/socket/socket.go
type SocketTransport struct {
    addr string // "/tmp/lyra.sock" on Unix, `\\.\pipe\lyra` on Windows
    conn net.Conn
}

func (t *SocketTransport) Call(ctx context.Context, req *Request) (*Response, error) {
    // Wire format: length-prefixed JSON. For streaming, each event is a
    // separate length-prefixed frame on the same conn until end-of-stream.
    ...
}
```

线格式选择：**length-prefixed JSON**（varint 长度 + body）走 socket。
任何语言都好实现、没有 framing 歧义、stream 就是 N 个 frame 一个
接一个。和 HTTP body 用同一份序列化——TS / Rust / Python 客户端
复用 schema 生成的类型即可。

成本：~30 µs / 调用。跨平台：macOS / Linux 上 `unix:///tmp/lyra.sock`，
Windows 上 `npipe:///./pipe/lyra`。鉴权靠 socket 文件 mode（Unix）
或 named-pipe ACL（Windows）——loopback only 时不需要 Bearer token。

### 4.4 FetchTransport —— anywhere → anywhere，HTTP

**适用**：浏览器；连远程服务器的 TUI；连托管云的包装客户端；
任何跨机器组合。

```ts
// frontend/src/transport/fetch.ts
export const fetchTransport = (baseURL: string, token: string): Transport => ({
  async call(req) {
    const res = await fetch(`${baseURL}${methodToPath(req.method)}`, {
      method: methodToVerb(req.method),
      headers: { Authorization: `Bearer ${token}`, ...req.headers },
      body: req.body,
    });
    if (req.stream) {
      return { status: res.status, headers: hdrs(res), events: parseSSE(res.body!) };
    }
    return { status: res.status, headers: hdrs(res), body: new Uint8Array(await res.arrayBuffer()) };
  },
  close() {},
});
```

后端侧：`pkg/httpserver/` 是一个薄适配器，把 HTTP verb + 路径映射
到 `CoreAPI` 的方法，再把响应重新编码。

成本：本地 ~200 µs / 调用，WAN ~20–200 ms。浏览器必须用（socket
没得选），远程必须用（这是唯一能干净穿过防火墙 / 代理的协议）。

### 4.5 WebSocketTransport —— 浏览器需要双向时的兜底

**适用**：web 前端需要在 run 进行中往上游推事件（罕见——HITL
approval 可以走普通 `POST /permission`，下行 SSE 已经覆盖了）。

仅作完整性列出；**MVP 不实现**。SSE + REST 已经覆盖所有场景，除了
"server stream 期间真正需要 client 反向 push"——而我们协议里没有
这种需求。

### 4.6 按组合挑选

| 组合 | 推荐传输 | 理由 |
| --- | --- | --- |
| Bubble Tea TUI + 嵌入式 Go 运行时 | **InProcess** | 同一二进制、两端都是 Go——直接函数调用比其他方案快 600×。 |
| Wails 客户端 + 嵌入式 Go 运行时 | **Wails IPC** | Wails 已经替你付过这笔成本；不用占额外端口；macOS 上不弹防火墙提示。 |
| Wails 客户端 + 本地服务 | **Socket** | 跨进程但同机；socket 比 loopback HTTP 快 6×，还省 Bearer token 管线。 |
| Wails 客户端 + 远程服务 | **HTTP** | 唯一能跨网络的。 |
| 浏览器 + 任意后端 | **HTTP** | 浏览器唯一选项。 |
| Node / Rust / Python TUI + 嵌入式 | n/a | 这些语言没法 in-process 嵌我们的 Go 运行时。改成 Socket 连一个旁边起的 `lyra-server` 子进程。 |
| Node / Rust / Python TUI + 本地 | **Socket** | 同 Wails-local 的理由。 |
| 任意客户端 + 远程 / 托管 | **HTTP** | 跨网络。 |

口诀：**能不序列化就不序列化**。In-process 免费，IPC 便宜，socket
还行，HTTP 是天花板。

---

## 5. 流式 —— 所有传输统一语义

每个传输都暴露同一个原语：`Response.Events`，一个 Go channel
（或它的 TS 等价——async iterator），产出 `Event`，带单调递增的
`id`、`type`、`data`。这意味着 **resume / replay 只在协议层实现
一次，不是每个传输各搞一份**。

| 传输 | 底层机制 |
| --- | --- |
| InProcess | `RunEvent` 的 Go channel |
| Wails | `EventsEmit` + JS 侧重组成 `AsyncIterator<Event>` |
| Socket | conn 上一系列 length-prefixed frame，直到 end-of-stream 哨兵帧 |
| HTTP | SSE（`text/event-stream`），用 `id:`、`event:`、`data:` |

客户端的 resume 逻辑——"重连、发送 `Last-Event-ID: <last>`、
后端从第 N+1 个事件开始 replay"——在每个传输上都用同一份代码，
因为它们都暴露同一个 `Event.ID` 序列。

对于 in-process，"resume" 退化（channel 是进程内的、随进程一起死），
客户端只要检查它的 `<-chan Event` 是不是被 close 了、然后重启。
同一份代码路径，不同的物理。

---

## 6. 鉴权 & 边界

| 传输 | 鉴权模型 |
| --- | --- |
| InProcess | 无——同进程互信。能力校验（caller 是不是有权？）发生在 `CoreAPI` impl 内部，不在传输边界。 |
| Wails | 线上无（Wails IPC 是 window-scoped）；`CoreAPI` impl 仍然做能力校验。 |
| Socket | Unix 用文件 mode / Windows 用 ACL。能连上 socket 的用户必须是文件 owner。没有 Bearer token。 |
| HTTP local | **必须 Bearer token。** 嵌入式后端安装时生成 token，写到 `~/.config/lyra/token`（chmod 600）。没有它，任何同机进程都能调我们。 |
| HTTP remote | OAuth（托管云）或静态 API key（自建）。 |

`Request.Headers["authorization"]` 这个字段是给 HTTP 类传输用的；
in-process 和 Wails 留空。`CoreAPI` impl **永远不信任传输层**——
能力校验（这个用户是不是拥有这个 session？）一律在 impl 内部
做，不管调用从哪条管子进来。

---

## 7. Schema 流 —— 一份契约，多个发射器

```
  pkg/coreapi/*.go  (Go interface + structs)
            │
            ├── go-openapi codegen ──→  schemas/openapi.yaml
            │                                  │
            │                                  ├── openapi-typescript ──→ frontend/src/lib/api-types.ts
            │                                  └── oapi-codegen      ──→ pkg/httpserver/generated.go
            │
            └── go-asyncapi codegen ──→  schemas/asyncapi.yaml
                                               │
                                               └── asyncapi-codegen-ts ──→ frontend/src/lib/events.ts
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
│   ├── sessions.go
│   ├── messages.go
│   └── runs.go
├── coreimpl/                # implementation (calls LLM, runs tools, stores state)
│   └── ...
├── transport/
│   ├── transport.go         # the Transport interface
│   ├── inprocess/
│   ├── wails/
│   ├── socket/
│   └── http/
├── httpserver/              # HTTP adapter — *Request <-> CoreAPI calls
│   └── ...
└── coreclient/              # client-side wrappers
    ├── inproc.go            # returns CoreAPI as-is
    ├── wails.go             # wraps WailsTransport
    ├── socket.go            # wraps SocketTransport
    └── http.go              # wraps FetchTransport

cmd/
├── lyra-tui/                # Bubble Tea TUI — imports coreapi + coreimpl directly
├── lyra-server/             # standalone server — imports coreimpl + httpserver
└── lyra-desktop/            # Wails shell — imports coreimpl + transport/wails

frontend/src/transport/
├── transport.ts             # TS mirror of the Go interface
├── fetch.ts                 # HTTP transport
├── wails.ts                 # Wails IPC transport
└── socket.ts                # Unix socket / named pipe (via Node IPC bridge for Electron)

schemas/                     # generated, in-repo
├── openapi.yaml
└── asyncapi.yaml
```

---

## 9. 这套切分为什么干净

- **加传输不动 handler 代码。** 新形态——比如托管云内部的 gRPC
  跨服务调用——实现 `Transport`、注册一个调 `CoreAPI` 的适配器，
  完事。
- **加前端不分裂后端。** 新形态——比如把桌面客户端用 Tauri 重写
  一遍——挑跟它外壳合的传输（Tauri IPC ≈ Wails IPC；没嵌入式
  后端就回落 HTTP）、实现 TS 侧 `Transport`、其他都复用。
- **Go-to-Go 不是特例。** 它用同一个 `Transport` 形状，只是实现
  是个空壳：客户端直接持有 `CoreAPI` 值。Bubble Tea、内部 CLI、
  以后 Go-native 插件全走这条路径。
- **HTTP 不享有特权。** `pkg/httpserver/` 只是四个适配器之一。
  协议里没有把 HTTP-isms 烧进 `CoreAPI`（没有 `Request *http.Request`
  参数、没有 `http.ResponseWriter` 返回值）。测试用 in-process；
  生产挑部署适合的。

---

## 10. 未决问题

- **gRPC？** 能干净装进 `Transport`，但会在 OpenAPI/AsyncAPI 之外
  多一份 proto schema 作为 SSOT。等真有非 MVP 客户问起再做。
- **HTTP 流式用 WebSocket 还是 SSE？** 今天用 SSE。只有当 HITL
  需要 run 进行中 client → server 推送的时候才换 WS（当前不需要——
  `/permission` 是普通 POST）。
- **非 Go 语言的 TUI——嵌入还是 spawn？** Node / Python / Rust
  没有 CGO 没法 in-process 嵌我们的 Go 运行时。推荐做法：把
  `lyra-server` 作为子进程 spawn 起来、用 Unix socket 连。
  省下语言互操作的痛苦，代价是多一个进程。
- **运行时热切换传输？** 比如嵌入式客户端在用户打开"连接到远程
  workspace"对话框时从 in-process 回落到 HTTP。可行——`Transport`
  就是个接口——但还没 UI 入口。

---

## 11. 速查表

```
同一 Go 二进制、两端都是 Go         →  InProcessTransport (无 codegen, ~50ns)
Wails / Tauri / Electron + Go core    →  WailsTransport       (无 Bearer, ~30µs)
跨进程、同机                          →  SocketTransport      (无 Bearer, ~30µs)
跨机器、任意语言                      →  FetchTransport (HTTP)(Bearer, ~200µs+)
浏览器、任意后端                      →  FetchTransport (HTTP)(唯一选项)
```

一个 `Transport` 接口、六种实现、九个部署格子。挑能跨过你的
边界的最便宜那个盒子。
