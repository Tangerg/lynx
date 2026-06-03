# `lyra/` 命名 review

扫完 lyra 全部模块 (`internal/engine/`, `internal/runtime/`,
`internal/service/*/`, `internal/transport/*/`, `internal/agui/`,
`internal/storage/`, `internal/config/`, `cmd/lyra/`)。

**整体非常干净**。Lyra 是写得最晚的模块，受益于之前 agent / core 的
经验，命名套路一致、零 Java leftover。下面只列少数边界讨论。

---

## 1. 单字包 + 同名类型（轻度 stutter，行业先例可接受）⚪

```go
engine.Engine        // engine.go:74
runtime.Runtime      // runtime.go:93
session.Session      // session.go:21
agui.Translator      // translator.go:56  ←  clean ✓
chat.Service         // service.go:58     ←  clean ✓
```

**评级**：
- `engine.Engine` / `runtime.Runtime` / `session.Session` — 包名 =
  类型核心概念，类似 `time.Time` / `bytes.Buffer` / `errors.New`，
  Go stdlib 公认可接受的"轻度 stutter"。**保留**。
- 改名收益小于成本（影响整个 lyra 代码 ripple）。

---

## 2. `chat.BaseEvent` — 直接 public 字段 + 零 getter ✅

`lyra/internal/service/chat/service.go:136`

```go
type BaseEvent struct {
    SessionID string
    TurnID    string
    Seq       uint64
    Timestamp time.Time
}
```

**评价**：完全符合"数据载体直接 public"规则。字段名清楚、零 getter、
无 JSON tag 重命名魔法 ✓

对比 agent 的 `event.BaseEvent.At/PID + Timestamp()/ProcessID()`
三重命名 — Lyra 这里 **clean implementation**。

---

## 3. `approval.GetMode` / `SetMode` — 状态切换的 Get/Set ⚪

`lyra/internal/service/approval/service.go:58`

```go
type Service interface {
    SetMode(ctx context.Context, mode Mode) error
    GetMode(ctx context.Context) (Mode, error)
    ...
}
```

**问题**：`Get` 前缀违反 Go effective-go。

**反驳**：这不是普通的访问器 — 它是**ctx-aware 的可能 fail 的查询**
（接口方法签名 `(ctx) (Mode, error)`）。Go effective-go 的"无 Get
前缀"规则针对的是**简单字段访问**（`obj.Cup()` 不是 `obj.GetCup()`），
不针对 ctx + error 的远程/可能失败查询。

类似先例：
- `os.LookupEnv` 不叫 `os.GetEnv`，但 `os.Getenv`（简单访问）就叫 `Get`。
- `crypto/x509.SystemCertPool` 没 Get 前缀，因为是简单访问；
  `time.LoadLocation` 也没。

**建议（边界）**：
- 保留 `SetMode` + `GetMode`（已经成对，符合 mutator + accessor 习惯）
- 或者改 `Mode()` + `SetMode()` —  更 Go 但与"对称"语感冲突

判定：**保留**。这种 ctx+error 的"远程查询"用 Get 前缀的 lynx 还有几处
（`MemoryService.Get`），命名一致比单纯遵循 effective-go 重要。

---

## 4. `engine.RunChatRequest` — Request 后缀 ✅

`engine.go:256`

```go
type RunChatRequest struct {
    SessionID string
    Message   string
    Observer  ToolObserver
}
```

**评价**：清晰说明这是 `RunChat` 方法的入参。Go 习惯命名。

---

## 5. `engine.ChatInput` / `ChatOutput` ✅

`agent.go:15,22`

```go
type ChatInput  struct { Message string }
type ChatOutput struct { Reply string; Usage TokenUsage }
```

**评价**：与 agent 的 `core.IOBinding` 配合 — Action 的 typed 输入
输出。命名清楚，public 字段直接暴露 ✓

---

## 6. `engine.MCPTransport` / `MCPServer` ✅

`mcp.go:18,41`

```go
type MCPTransport int  // 初始词大写正确
type MCPServer    struct { ... }
```

**评价**：初始词全大写符合 Go 习惯（不像 agent 仓库 `Api`），并与
`mcp.HTTPClientOptions` 等保持一致 ✓

---

## 7. `transport/http.Server` 配置 ✅

`server.go:30`

```go
type Server struct { ... }
type Config struct { Runtime *lyraruntime.Runtime; Addr string }
```

**评价**：包名 `http` 是子目录名，类型 `Server` 单字 — 等价于 stdlib
`http.Server` 风格。无口吃 ✓

---

## 8. `transport/ipc.Server` ✅

`server.go:37` / `handlers.go`

**评价**：与 http 包对称设计 ✓

---

## 9. `runtime.MaybeMaintain(ctx, sessionID) (bool, error)` ✅

`runtime.go`

**评价**：动词 + 副词，清楚表达"看情况执行 + 维护"语义 ✓

---

## 10. `chat.Event` 接口 + 7 个事件类型 ✅

`chat/service.go:128`

```go
type Event interface { ... }
type TurnStart       struct { BaseEvent; Model string }
type MessageDelta    struct { BaseEvent; Text string }
type ReasoningDelta  struct { BaseEvent; Text string }
type ToolCallStart   struct { BaseEvent; ToolName string; ... }
type PlanGenerated   struct { BaseEvent; Plan string }
type ToolCallApproval struct { BaseEvent; Request approval.Request }
type ToolCallEnd     struct { BaseEvent; ToolName string; ... }
type TurnEnd         struct { BaseEvent; Reason TurnEndReason; ... }
type ErrorEvent      struct { BaseEvent; Message string }
```

**评价**：每个事件类型都是**纯数据载体 + public 字段**，无 getter，
无 ToString。完全符合"数据载体直接 public"规则 ✓

---

## 11. `session.Repo` / `session.Service` ✅

`session/repo.go:22` / `service.go:36`

**评价**：内存仓 + service 接口分离，命名清楚。`Repo` 是 lynx 此前
session-extract refactor 的产物，namespace 干净 ✓

---

## 12. `runtime.Config` 字段命名 ✅

`runtime/runtime.go:44`

```go
type Config struct {
    ChatClient    *chat.Client
    Workdir       string
    Online        engine.OnlineConfig
    MCPServers    []engine.MCPServer
    Compaction    engine.CompactionConfig
    MemoryStore   chatmem.Store
    MemoryService memsvc.Service
    SessionService sessionsvc.Service
    ApprovalMode  approval.Mode
}
```

**评价**：所有字段命名直白，无缩写、无 hack ✓

---

## 不动 / 已经 OK 的

- 零 `*Impl` / `*Interface` 后缀
- 零 ToString / InfoString
- 零 Manager / Helper / Util / Factory 命名
- 所有数据载体 public 字段直接暴露
- 所有初始词大小写正确 (HTTP / MCP / IPC / SSE / API / JSON 等)
- `engine.Engine` / `runtime.Runtime` / `session.Session` 轻度 stutter
  是设计取舍，保留
- `Service` 接口在每个 service 包内单字，跨包命名一致

---

## 优先级建议

**无任何 P0~P2 调整项**。

Lyra 是写得最晚的模块，命名跟着 agent / core 的经验走，**没有需要改
的命名问题**。

唯一可斟酌的（**P3**）：
1. `approval.GetMode` / `SetMode` — 保留对称命名，或改 `Mode()` +
   `SetMode()` 单边去 Get 前缀。审美讨论。

---

## 体检命令

- `go test ./lyra/...` — 应全绿
- `grep -rnE "^type \w*(Impl|Interface)\b" lyra/` — 应得 0
- `grep -rn "InfoString\|ToString" lyra/` — 应只匹配外部库调用
