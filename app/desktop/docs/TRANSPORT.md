# Lyra 传输层

> `API.md` 定义 **Lyra Runtime Protocol**（JSON-RPC 2.0）的 wire 语义。
> 本文档描述该协议在表现层 ↔ Runtime 之间**物理上**怎么传输。协议在所有
> 传输上完全一致；**传输层**只负责把 JSON-RPC message（Go-to-Go 时直接把
> struct 指针）从一边搬到另一边。
>
> 读者：要做新表现层、把 Runtime 嵌入新外壳、或为特定部署写适配器的人。
> 错误码 / HTTP status / observability / sidecar 的细节都在 `API.md`，本文
> 不重复，只引用。

---

## 1. 部署矩阵

Runtime 永远是**单一形态**（本地或同进程；远程留给未来 facade，见
`API.md` 附录 B）。表现层 3 种形态，各挑能跨过自己进程/网络边界的最便宜
传输：

| 表现层 | Runtime 在哪 | 传输 | 备注 |
| --- | --- | --- | --- |
| **TUI（Go，如 Bubble Tea）** | 同一 Go 二进制 | **InProcess** | 直接持有 Runtime 对象，函数调用，零序列化 |
| **套壳客户端（Wails / Tauri / Electron）** | 宿主进程 | **Wails IPC** | WebView ↔ 宿主进程 IPC bridge |
| **Web（浏览器）** | 同机独立进程 | **HTTP loopback** | 浏览器无法嵌后端，必走 HTTP；本地 token 做进程门禁 |

三种传输共享**同一个 Go interface**，handler 代码跟传输选哪个无关。

---

## 2. 唯一抽象 —— `Transport`

两端对接同一个 interface。`Transport` 就是 JSON-RPC message 的双向管道
—— HTTP framing、SSE 解析、Wails IPC bridge 全藏在接口背后。

```go
// pkg/transport/transport.go
package transport

import (
    "context"
    "encoding/json"
)

// Message 是一个 JSON-RPC 2.0 envelope —— Request / Response / Notification。
// 按 spec 只填对应字段：
//   Request:      jsonrpc + id + method + params?
//   Response:     jsonrpc + id + (result XOR error)
//   Notification: jsonrpc + method + params?  (no id)
type Message struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      json.RawMessage `json:"id,omitempty"`
    Method  string          `json:"method,omitempty"`
    Params  json.RawMessage `json:"params,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}

// Transport 是双向 message pipe。三种实现在范围内：InProcess / Wails / HTTP。
type Transport interface {
    // Send 把出站 message 交给底层传输即返回（不等对端处理完）。
    Send(ctx context.Context, msg *Message) error
    // Recv 返回入站 message 的 channel；传输断开时 close，消费方须 drain。
    Recv() <-chan *Message
    // Close 终止传输；pending Send 以 context.Canceled 失败，Recv channel close。
    Close() error
}
```

`Transport` 是**纯 message pipe**：不区分 Request/Response 配对（按 `id`
match 是上层 `RPCClient` 的事），不需要专门的 "Stream" 字段（streaming
就是 `Recv()` 持续吐 Notification）。前端永远见不到 `*http.Request` /
`net.Conn`；后端 router 签名是 `func(ctx, *Message) *Message`，同一份代码
跑在所有传输背后。

---

## 3. 服务契约 —— 后端 `rpc/protocol`

> **实现已 ship**（lynx 仓 `lyra/`），非"待建"。下文的 `pkg/coreapi` /
> `pkg/coreimpl` / `pkg/rpcadapter` / `pkg/transport` 是早期抽象示意名；
> 实际已落地并改名：`rpc/protocol`（interface，`CoreAPI`→`Runtime`）/
> `rpc/server`（impl）/ `rpc/dispatch`（adapter）/ `rpc/transport`。见
> lyra `CLAUDE.md` 与本文 §8 的实际布局。

`Transport` 搬运不透明 envelope；**类型化的 Go interface**（两端真正同意
的契约）在其上一层 `rpc/protocol`：

```go
// pkg/coreapi/coreapi.go
package coreapi

import "context"

type CoreAPI interface {
    Initialize(ctx context.Context, in InitializeIn) (*InitializeOut, error)
    Shutdown(ctx context.Context, in ShutdownIn) error

    ListSessions(ctx context.Context, q ListSessionsQuery) (*Page[Session], error)
    GetSession(ctx context.Context, id string) (*Session, error)
    CreateSession(ctx context.Context, in CreateSessionIn) (*Session, error)
    // …

    ListMessages(ctx context.Context, sessionID string, q ListMessagesQuery) (*Page[Message], error)

    // 流式 surface
    StartRun(ctx context.Context, in StartRunIn) (*StartRunOut, <-chan AgUiEvent, error)
    CancelRun(ctx context.Context, runID string) error
    SubmitApproval(ctx context.Context, in ApprovalIn) error

    // …完整 surface 镜像 API.md §5.2 方法表
}
```

这个 interface 是**行为的 SSOT**。从它衍生两份产物：JSON-RPC method 表
+ AsyncAPI 事件 schema（`schemas/`，给非 Go 客户端 codegen）；以及
JSON-RPC 适配器（`pkg/rpcadapter`，把 inbound `Message` 路由到 `CoreAPI`
方法、把结果编码回 `Message`）。

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

适配器形态因传输而异：InProcess 时前端 `Client` 直接持有 `CoreAPI` 引用，
**完全跳过 JSON-RPC 序列化**；Wails / HTTP 时适配器解码 `Message` → method
+ params struct，调 `CoreAPI`，把返回值打回 `Message`。

---

## 4. 三种传输实现

### 4.1 InProcessTransport —— Go ↔ Go，同一二进制

**适用**：链接 `pkg/coreimpl/` 的 Bubble Tea TUI（或任何 Go 表现层）。

实战中 Go 表现层**直接持有 `CoreAPI` 对象就够了** —— 根本不需要 `Message`
往返：

```go
// pkg/coreclient/inproc.go
func NewInProcClient(api coreapi.CoreAPI) coreapi.CoreAPI {
    return api // 同一 interface，原样返回
}
```

`Transport` 接口此时只为统一注入 logging / tracing middleware，业务路径完
全跳过序列化。`StartRun` 的 `<-chan AgUiEvent` 直接 stream 到 TUI 的 update
loop（让 `AgUiEvent` 实现 `tea.Msg`）。

成本：~50 ns/调用（Go interface dispatch）。流式吞吐只受 channel buffer 限制。

### 4.2 WailsTransport —— 套壳外壳 ↔ 嵌入式 Go Runtime

**适用**：Wails / Tauri / Electron，Runtime 在宿主进程、前端在 WebView。

前端是 TS 薄壳，对外暴露同样的 `Transport` 形状：Request/Notification 经
Wails IPC POST 给宿主的 JSON-RPC 适配器；流式 notification 经
`EventsEmit` 推回，JS 侧重组成 `Recv()` 的 AsyncIterator。连接关联隐式
（单 WebView ↔ 宿主连接，见 API.md §3.2）。

成本：~30 µs/调用（Wails IPC + structured-clone）。避开 HTTP framing + TCP，
又不动 WebView。

### 4.3 HTTPTransport —— 浏览器 + 未来 facade

**适用**：浏览器表现层（必走网络），以及未来经云端 facade 连远程 Runtime。

线协议（wire 细节全在 API.md，此处只列端点）：

| Endpoint | 用途 |
| --- | --- |
| `POST /v1/rpc/{method}` | **唯一 JSON-RPC entry**。method 名在 URL（点保留、禁止斜杠化），body 里仍含 `method` 做 cross-check（不匹配 `409`）。不接受无后缀 `POST /v1/rpc`（返 `404`）。详见 API.md §10.1。 |
| `GET /v1/rpc/stream?conn=<id>` | SSE 流，server → client notification。`conn` query 把这条流关联到客户端连接（详见**下方连接关联**）。每个 SSE event 的 `data:` 是一条 JSON-RPC Notification。重连带 `Last-Event-Id` 续传。 |
| `GET /v1/info` / `GET /v1/health` | **Sidecar**。扁平 JSON、不走 envelope、无鉴权。详见 API.md §9。 |

HTTP status 仅反映 transport 层，业务 error 走 `error.code`（不映射 status）
—— 完整映射表见 API.md §7.3。Observability 强制要求（`X-Lyra-Method` 等
header + 结构化日志 + Prometheus）见 API.md §10。

**连接关联**（API.md §3.2 的 HTTP 落地）：浏览器 `EventSource` API **无法
设自定义 header**，所以连接身份不能像 POST 那样走 `Lyra-Connection-Id`
header，而走 SSE URL 的 `?conn=<id>` query —— 仍在 transport URL 层，不进
JSON-RPC envelope。服务端把该 `conn` 启动的 run 的 notification 路由到匹配
的 SSE 流。

```ts
// frontend/src/transport/http.ts（要点）
export const httpTransport = (baseUrl: string, opts: {
  connId: string;          // 客户端生成的连接 id（UUIDv7）
  localToken?: string;     // 本地进程门禁 token，仅 loopback 场景
}): Transport => {
  // 下行：SSE 用 ?conn= 关联（EventSource 不能设 header）
  const sse = new EventSource(`${baseUrl}/v1/rpc/stream?conn=${opts.connId}`, {
    withCredentials: false,
  });
  sse.onmessage = (e) => { /* JSON.parse(e.data) → Recv channel */ };

  return {
    async send(msg) {
      if (!msg.method) throw new Error("HTTP transport 只发 Request / Notification");
      const headers: Record<string, string> = {
        "Content-Type": "application/json",
        "Lyra-Connection-Id": opts.connId,       // 上行：header 带连接 id
      };
      if (opts.localToken) headers["Authorization"] = `Bearer ${opts.localToken}`;
      // 单一形态：POST /v1/rpc/{method}（无后缀返 404，greenfield 无 fallback）
      await fetch(`${baseUrl}/v1/rpc/${msg.method}`, {
        method: "POST", headers, body: JSON.stringify(msg),
      });
    },
    recv() { /* 由 sse 喂的 AsyncIterator */ },
    close() { sse.close(); },
  };
};
```

`localToken` 仅本地进程门禁场景用（Web 前端连同机 Runtime），读自
`~/.lyra/local-token`。未来 facade 场景由 facade 层处理鉴权再转发，Runtime
看到的 HTTP 仍是同一个 `/v1/rpc/{method}` 入口。

成本：本地 ~200 µs/调用，WAN ~20–200 ms。

### 4.4 按场景挑选

| 场景 | 传输 | 理由 |
| --- | --- | --- |
| Bubble Tea TUI + Go Runtime（同二进制） | **InProcess** | 同进程两端都是 Go，函数调用比 IPC 快 ~600× |
| Wails / Tauri / Electron + Go Runtime | **Wails IPC** | 已付过这笔成本，不占额外端口，macOS 不弹防火墙 |
| 浏览器 | **HTTP** | 浏览器唯一选项。本地走 loopback + 门禁 token；云端走 facade |

口诀：**能不序列化就不序列化**。In-process 免费，IPC 便宜，HTTP 是天花板。

非 Go TUI / Unix socket / WebSocket 本轮**不实现** —— 本地 Lyra 不需要跨
语言进程间通讯，远程访问等 facade 时再加。

---

## 5. 流式 —— 所有传输统一语义

每个传输都通过 `Recv()` 吐出**同一种 Notification 流**（主要是
`notifications/run/event`，带 `eventId` + AG-UI `event`）。这意味着
**resume / replay 在协议层只实现一次**，不是每个传输各搞一份。

| 传输 | 底层机制 |
| --- | --- |
| InProcess | Notification 的 Go channel |
| Wails IPC | `EventsEmit` + JS 侧重组 AsyncIterator |
| HTTP | SSE，每个 event 的 `data:` 是一条 Notification |

续传逻辑（重连、带 `Last-Event-Id`、Runtime 从该资源 id 内 `eventId >
lastEventId` 处 replay）在每个传输上用同一份代码，因为它们都暴露同一个
`eventId` 序列（详见 API.md §3.3）。对 in-process，"resume" 退化（channel
随进程一起死），客户端检查 `<-chan` 是否被 close 然后重启 —— 同一份代码
路径，不同物理。

---

## 6. 鉴权 & 边界

| 传输 | 鉴权模型 |
| --- | --- |
| InProcess | 无 —— 同进程互信，运行环境的信任 = OS 用户的信任 |
| Wails IPC | 线上无（IPC 是 window-scoped）；同进程内等价信任 |
| HTTP（本地 loopback） | **本地进程门禁 token**（非用户鉴权）。启动随机生成、写 `~/.lyra/local-token`（chmod 600）。仅阻止同机其他进程乱调 |
| HTTP sidecar | **永远无鉴权** —— curl-friendly read-only metadata（API.md §9） |
| HTTP（未来 facade） | 由 facade 层处理用户鉴权 + 转发；Runtime 看到的请求仍是同样形态 |

**核心不变量**：`CoreAPI` impl **永远不做用户鉴权 / 授权** —— 协议层根本
没有 user / account / owner 概念。需要时由外层（OS、本地 token 门禁、未来
facade）解决。

---

## 7. Schema 流 —— 一份契约，多个发射器

```
  pkg/coreapi/*.go  (Go interface + structs)
        ├── go-jsonrpc  → schemas/methods.yaml  → jsonrpc-ts  → frontend/src/lib/runtime-types.ts
        └── go-asyncapi → schemas/events.yaml   → asyncapi-ts → frontend/src/lib/events.ts
```

Go ↔ Go（in-process / Wails）两端直接 import `pkg/coreapi`，**不需要
codegen**。其他场景 schema 才是契约，TS / Rust / Python 客户端从它生成。

---

## 8. 实际文件布局（已 ship）

后端在 lynx 仓 `lyra/`（本仓 `internal/agui/` 是 dev mock）：

```
lyra/                     # lynx 仓的 Runtime 后端
├── rpc/
│   ├── protocol/         # Go interface（Runtime + 11 domain 子接口）—— 行为 SSOT
│   ├── server/           # in-process Runtime impl（wire internal/* → protocol）
│   ├── dispatch/         # JSON-RPC Message ↔ Runtime 方法 dispatch + 路由
│   └── transport/
│       ├── http/         # HTTP + SSE（CORS / auth / tracing）
│       └── inprocess/    # Go chan transport（TUI）
├── internal/
│   ├── engine/           # chat loop（agent / turn / usage / compaction）
│   ├── service/          # session / chat / approval(Console·Gate) / tool / memory
│   └── storage/          # file-backed + sqlite
└── cmd/lyra/             # CLI 入口

frontend/src/rpc/         # TS 客户端镜像：client / methods / transports/{http,inprocess} / stream
frontend/src/rpc/transports/   # transport 实现（http.ts / memory.ts）
```

> `schemas/` codegen 产物（methods.yaml / events.yaml）+ 多语言 client 生成
> 是计划态；当前前端类型手写、以 `rpc/protocol` 的 Go interface 为准对齐。

---

## 9. 这套切分为什么干净

- **加传输不动 handler**：新形态（如未来 facade 内部走 gRPC）只需实现
  `Transport` + 注册一个调 `CoreAPI` 的适配器。
- **加前端不分裂后端**：新外壳（如 Tauri 重写）挑合适传输、实现 TS 侧
  `Transport`，其余复用。
- **Go-to-Go 不是特例**：用同一个 `Transport` 形状，只是实现是空壳（客户端
  直接持有 `CoreAPI` 值）。
- **HTTP 不享特权**：`pkg/transport/http/` 只是三个传输之一。`CoreAPI` 里
  没有 HTTP-isms（无 `*http.Request` 参数、无 `http.ResponseWriter` 返回）。
  测试用 in-process，生产挑部署合适的。

---

## 10. 未决问题

- **gRPC？** 能干净装进 `Transport`，但会多一份 proto schema 作 SSOT。等真
  有非 MVP 场景再做（最可能是未来 facade ↔ Runtime pool 内部互联）。
- **HTTP 流式用 WebSocket 还是 SSE？** 今天 SSE。只有当 HITL 需要 run 进行
  中 client → server 推送才换 WS（当前不需要 —— approval 走普通
  `runs.approval.submit` Request）。
- **运行时热切换传输？** 可行（`Transport` 就是个接口），但还没 UI 入口。

---

## 11. 速查表

```
同一 Go 二进制、两端都是 Go            →  InProcessTransport       (无 codegen, ~50ns)
Wails / Tauri / Electron + Go Runtime  →  WailsTransport           (~30µs)
浏览器、本地 Runtime                   →  HTTPTransport + 本地 token 门禁 (~200µs)
浏览器、未来云端 facade                →  HTTPTransport + facade 处理鉴权
```

一个 `Transport` 接口、三种实现。挑能跨过你的边界的最便宜那个盒子，剩下
的协议层零代码改动。
