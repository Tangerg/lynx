# Lyra — 架构设计（CS 架构 / v2）

> **基线**：lynx HEAD `c68784d`，2026-05-21。Lyra 是基于 lynx-agent framework 构建的**通用 agent 运行时**，采用 **client-server 架构**，多 transport 可选，前端独立仓库。

---

## 1. 产品定档

**Lyra is a general-purpose agent runtime — product-grade, transport-agnostic, deployable as either a local process or a remote service.**

中文一句话：**用 lynx framework 搭起来的通用 agent 服务运行时，可本地起，也可服务端起；前端任意（TUI / Web / Desktop）通过 HTTP / gRPC / IPC 接入**。

### 1.1 Lyra 是什么

```
┌───────────────────────────────────────────────────────────────┐
│                    Clients (separate repos)                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐    │
│  │   TUI    │  │   Web    │  │ Desktop  │  │ MCP client │    │
│  └─────┬────┘  └─────┬────┘  └─────┬────┘  └─────┬──────┘    │
│        │             │             │              │            │
│        │ IPC (stdio  │ HTTP+SSE    │ gRPC stream  │ MCP        │
│        │  JSON-RPC)  │             │              │ (stdio/HTTP)│
└────────┼─────────────┼─────────────┼──────────────┼────────────┘
         │             │             │              │
         ▼             ▼             ▼              ▼
┌───────────────────────────────────────────────────────────────┐
│                  Lyra Server (this module)                     │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │ Transport Layer (4 adapters)                             │  │
│  │  http/     grpc/     ipc/        mcp/                    │  │
│  └────────────────────────┬────────────────────────────────┘  │
│                           │ all map to one interface           │
│  ┌────────────────────────▼────────────────────────────────┐  │
│  │ Core Service Interface (transport-agnostic Go API)       │  │
│  │  SessionService / ChatService / ToolService /            │  │
│  │  ApprovalService / MemoryService / TraceService          │  │
│  └────────────────────────┬────────────────────────────────┘  │
│                           │                                    │
│  ┌────────────────────────▼────────────────────────────────┐  │
│  │ Engine (产品级 agent loop, 内置 tool/sandbox/compact 等) │  │
│  └────────────────────────┬────────────────────────────────┘  │
└───────────────────────────┼────────────────────────────────────┘
                            │ depends on
                            ▼
┌───────────────────────────────────────────────────────────────┐
│                  lynx-agent (framework)                         │
│  Platform / Action / Goal / Planner / Workflow / HITL          │
│  Extension / Snapshot / Session / Invocation history           │
└────────────────────────────┬──────────────────────────────────┘
                             ▼
┌───────────────────────────────────────────────────────────────┐
│                  lynx-core (foundation)                         │
│  Chat / Tool / Vector / RAG / MCP / OTel                        │
└───────────────────────────────────────────────────────────────┘
```

### 1.2 Lyra 是什么

| 维度 | 说明 |
|---|---|
| 形态 | Long-running server，多 transport 接入 |
| 部署 | 本地（`lyra serve`，stdio/unix-socket）或远端（HTTP/gRPC over TCP） |
| 客户端 | **不在本 repo**：TUI / Web / Desktop 由独立 repo 实现，通过 IDL 消费 |
| 协议 | HTTP+SSE / gRPC bidirectional / IPC stdio JSON-RPC / MCP 兼容 |
| Engine | 内置一套 opinionated agent loop（system prompt / tool 集 / sandbox / compact / memory / approval / planner） |
| 模型 | 不绑定 — 走 lynx-core 的 `chat.Client`，44+ provider 可用 |
| 沙箱 | macOS Seatbelt / Linux bwrap / Windows Sandbox 三后端 |

### 1.3 Lyra 不是什么

- ❌ 不是 framework — framework 已经是 lynx-agent
- ❌ 不是 CLI 工具 — `lyra` 二进制是 server 入口，不是单次调用
- ❌ 不是 Go SDK — 客户端不 `import lyra`，走 IDL 通信
- ❌ 不是只 local 跑 — 同一份 binary 在本地 / 容器 / k8s 都能跑
- ❌ 不做 client 实现 — TUI / Web 在别的 repo

---

## 2. CS 架构的核心约束

### 2.1 Transport-agnostic Service 接口

所有业务逻辑写在 `internal/service/` 下的 Go interface 里，**完全不感知 transport**。Transport adapter 只做 marshal/unmarshal + 协议路由。

```go
// 伪代码 — 实际接口见 internal/service/
type ChatService interface {
    StartTurn(ctx context.Context, req StartTurnRequest) (TurnHandle, error)
    StreamEvents(ctx context.Context, handle TurnHandle) (<-chan Event, error)
    InjectSteering(ctx context.Context, handle TurnHandle, msg string) error
    CancelTurn(ctx context.Context, handle TurnHandle) error
}

// HTTP adapter
func (h *HTTPHandler) PostTurn(w http.ResponseWriter, r *http.Request) {
    var req StartTurnRequest
    json.NewDecoder(r.Body).Decode(&req)
    handle, _ := h.chat.StartTurn(r.Context(), req)
    events, _ := h.chat.StreamEvents(r.Context(), handle)
    sseStream(w, events)
}

// gRPC adapter — 同一接口，proto 映射
// IPC adapter — 同一接口，JSON-RPC over stdio
// MCP adapter — 同一接口，MCP tool call 映射
```

### 2.2 Transport 矩阵

| Transport | 用途 | 双向流 | 序列化 | 状态 |
|---|---|---|---|---|
| **HTTP + SSE** | Web 客户端 / 远端部署 | 单向 SSE（客户端→服务端用普通 POST）| JSON | 必做（M2） |
| **gRPC** | Desktop / 高吞吐场景 | 双向 stream | Protobuf | 必做（M5） |
| **IPC (stdio JSON-RPC)** | TUI / 本地 spawn | 双向 | JSON | 必做（M3） |
| **MCP** | 让其他 agent 把 Lyra 当 tool | 由 MCP 协议定 | JSON | 选做（M9） |

**默认开几个**：`lyra serve` 默认开 IPC（stdio，最快），HTTP / gRPC 按 flag 开启。

### 2.3 IDL 与版本管理

```
lyra/proto/                       # IDL 源（前端 repo 通过 git submodule / npm 消费）
├── lyra/v1/
│   ├── session.proto             # SessionService
│   ├── chat.proto                # ChatService
│   ├── tool.proto                # ToolService
│   ├── approval.proto            # ApprovalService
│   ├── memory.proto              # MemoryService
│   └── trace.proto               # TraceService
├── openapi.yaml                  # 从 proto 自动生成的 HTTP+SSE 描述
└── README.md                     # 协议版本规则
```

- **gRPC** 直接用 proto
- **HTTP+SSE** 走 gRPC-Gateway 风格，proto 注解里写 HTTP 映射
- **IPC** JSON-RPC 用 proto-JSON 编码（同一 schema）
- **MCP** 把每个 service method 映射成 MCP tool

**版本规则**：semver。`v1` 内部 backward-compatible；break 升 `v2` 并保留 `v1` adapter。

### 2.4 客户端无状态原则

服务端持有所有状态：
- Session 历史 / 树
- Memory（LYRA.md / 用户 prefs）
- Approval cache
- 当前 turn 的 cancel handle

客户端只持有：
- 连接配置（地址 / token）
- 当前显示的 session id
- UI 临时状态

**客户端断开重连不丢上下文**。

### 2.5 多客户端同 session 的约束

MVP **不支持**多个 client 同时连同一 session（互斥锁）。第二个 client 连 → 返回 `SESSION_IN_USE` 错误。后续 v0.2+ 再考虑 collaborative editing 语义。

---

## 3. 模块拆分

```
lyra/                                  # Go module
├── cmd/
│   └── lyra/                          # 唯一 binary entry
│       └── main.go                    # cobra root，subcommand: serve / version / config
├── proto/                             # IDL（前端 repo 消费）
│   ├── lyra/v1/*.proto
│   └── openapi.yaml
├── internal/
│   ├── service/                       # Transport-agnostic 业务接口 + 实现
│   │   ├── session/                   # 会话管理
│   │   ├── chat/                      # turn 编排 + event stream
│   │   ├── tool/                      # tool 注册 + dispatch
│   │   ├── approval/                  # 权限决策
│   │   ├── memory/                    # LYRA.md / 用户记忆
│   │   └── trace/                     # OTel 收集与查询
│   ├── transport/                     # Transport adapter
│   │   ├── http/                      # gin / echo / chi（待定）
│   │   ├── grpc/                      # google.golang.org/grpc
│   │   ├── ipc/                       # stdio JSON-RPC
│   │   └── mcp/                       # 走 lynx-mcp server
│   ├── engine/                        # 内部 agent loop
│   │   ├── agent.go                   # 构造 *core.Agent（lynx）
│   │   ├── prompt/                    # 内置 system prompt（多 mode）
│   │   ├── tools/                     # Lyra 自带的 tool 实现
│   │   │   ├── bash/                  # Bash + 沙箱
│   │   │   ├── fs/                    # Read/Write/Edit
│   │   │   ├── grep/                  # ripgrep wrapper
│   │   │   ├── glob/                  # glob 匹配
│   │   │   ├── webfetch/              # HTTP fetch
│   │   │   └── task/                  # 子任务委派（lynx AsChatTool）
│   │   ├── sandbox/                   # 三平台沙箱
│   │   ├── compaction/                # token-ratio auto-compact
│   │   └── stream/                    # Event channel 转 stream
│   ├── storage/                       # 持久化层
│   │   ├── session_store.go           # JSONL 实现 lynx core.SessionStore
│   │   ├── process_store.go           # lynx core.ProcessStore 实现
│   │   ├── memory_store.go            # LYRA.md 文件级联
│   │   └── approval_cache.go          # bbolt / sqlite
│   ├── config/                        # ~/.lyra/config.toml
│   └── auth/                          # 远端模式的 API key / OAuth
└── doc/
    ├── ARCHITECTURE.md                # 本文档
    ├── ROADMAP.md                     # 路线图
    ├── PROTOCOL.md                    # 协议设计细节
    └── CLIENT.md                      # 客户端接入指南
```

### 3.1 跟 lynx 的依赖关系

```
┌─────────────────────────────────────────────┐
│  Lyra (server runtime)                       │
│  - 不引入新的 framework 抽象                  │
│  - 在 lynx-agent 上加 transport + 产品级集成  │
└──────────────────┬───────────────────────────┘
                   ↓ depends on
┌─────────────────────────────────────────────┐
│  lynx-agent (framework)                      │
│  Platform / Action / Goal / Planner / ...    │
└──────────────────┬───────────────────────────┘
                   ↓ depends on
┌─────────────────────────────────────────────┐
│  lynx-core (foundation)                      │
└─────────────────────────────────────────────┘
```

**强约束**：Lyra 不向 lynx 反向贡献抽象（除非沉淀过 3+ 用例）。

### 3.2 跟前端 repo 的依赖关系

```
lyra (this repo)                    frontend (separate repo)
├── proto/lyra/v1/*.proto    ←─────── git submodule 或 buf module
├── proto/openapi.yaml       ←─────── codegen TypeScript client
└── server impl                       └── TUI / Web / Desktop
```

- 前端 repo **只依赖 `lyra/proto/`**，不依赖 server 实现
- 协议变更通过 PR review → 版本 bump → 前端升级

---

## 4. 关键架构决策

### 4.1 Service 接口分层（六大 service）

| Service | 职责 | 主要方法 |
|---|---|---|
| **SessionService** | 会话生命周期 | List / Create / Get / Fork / Delete / Resume |
| **ChatService** | 一轮交互 | StartTurn / StreamEvents / InjectSteering / Cancel |
| **ToolService** | 工具元信息 + 手动调用 | ListTools / GetToolSchema / InvokeTool |
| **ApprovalService** | 权限决策 | RequestApproval / Decide / ListPending |
| **MemoryService** | 长期记忆 | GetLYRAMd / UpdateLYRAMd / ListMemories |
| **TraceService** | 观测 | ListTraces / GetTrace / StreamLiveSpans |

每个 service 都是 Go interface（`internal/service/<name>/service.go`），实现在同一目录的 `impl.go`。

### 4.2 Event 流模型

一个 turn 产生的 event 类型（参考 pi-mono 但精简）：

```
TurnStart           # turn 开始
MessageDelta        # 模型流式输出文本片段
ToolCallRequest     # 模型要调 tool
ToolCallApproval    # 等待用户批准（如果需要）
ToolCallStart       # 开始执行 tool
ToolCallOutput      # tool 流式 stdout/stderr
ToolCallEnd         # tool 完成（含结果）
ReasoningDelta      # extended thinking 流式
PlanGenerated       # planner 产出 plan（plan mode 下）
TurnEnd             # turn 结束（含总结：token / cost / duration）
Error               # turn 内任何错误
```

每个 event 有 `turn_id` / `session_id` / `seq` / `timestamp`。客户端按 `seq` 排序，断线重连可从 `last_seq` 恢复。

### 4.3 部署形态

| 模式 | 启动方式 | Transport | 用例 |
|---|---|---|---|
| **Embedded** | `lyra serve --stdio` | IPC only | TUI spawn 子进程 |
| **Local daemon** | `lyra serve` | IPC + HTTP（localhost）| 桌面 app 后台运行 |
| **Network service** | `lyra serve --listen :8080 --grpc :9090` | HTTP + gRPC | 团队共享 / 云部署 |

**第一次跑 TUI 客户端时**：如果检测不到本地 lyra-server，自动 spawn 一个（stdio 模式），跟随 TUI 退出而退出。

### 4.4 Auth

| 部署模式 | Auth |
|---|---|
| Embedded (stdio) | 无需 auth（父子进程信任）|
| Local daemon (Unix socket) | 文件权限（owner-only）|
| Local daemon (TCP localhost) | API key（启动时生成 `~/.lyra/api-key`）|
| Network service | API key 或 OAuth（按 config）|

### 4.5 Planner / Tool / Sandbox

承接 v1 设计：
- 默认 reactive，长任务切 goap，`/plan` 走 HTN
- 7 个内置 tool（Bash / Read / Write / Edit / Grep / Glob / WebFetch）
- macOS Seatbelt / Linux bwrap / Windows Sandbox

### 4.6 Permission（三档）

```
模式         Bash         FS-Write     Network       MCP-tool
─────        ────         ────────     ───────       ─────────
safe         ASK          ASK          DENY          ASK
balanced     ASK*         ALLOW        ASK           ASK*
yolo         ALLOW        ALLOW        ALLOW         ALLOW

* = 带 LLM classifier
```

Approval 走 **ApprovalService**：服务端发起 `ToolCallApproval` event → 客户端 UI 询问 → 客户端调 `ApprovalService.Decide` → 服务端继续。

### 4.7 上下文压缩 + Memory

承接 v1：token > 60% auto-compact，写回 `<cwd>/LYRA.md` + `~/.lyra/LYRA.md`，下次 session 自动加载。

### 4.8 Session 树

- JSONL 持久化（每个 session 一个文件）
- 树结构（message 带 `parent_id`）
- `SessionService.Fork(id, at_message_id)` 创建分支

---

## 5. "不做"清单

| 不做 | 理由 |
|---|---|
| ❌ 在本 repo 写 TUI / Web 客户端 | 独立 repo，避免耦合 |
| ❌ 自建协议格式 | proto + JSON-RPC 是事实标准 |
| ❌ 多 client 协同同一 session | 复杂度爆炸，v0.2+ 再说 |
| ❌ 用户管理 / 多租户 | 单用户 server，企业需求走二开 |
| ❌ 计费 / quota | YAGNI |
| ❌ 自建 LLM client / vector store / MCP | lynx 已有 |
| ❌ Plugin marketplace | YAGNI |
| ❌ 自己做 LSP | 走 MCP，让用户接 lsp-mcp-server |
| ❌ 多语言 i18n | MVP 英文 |

---

## 6. 风险与权衡

### 6.1 协议演化成本

CS 架构最大代价：协议改一次，所有客户端跟着升。

**应对**：
- proto v1 内部 backward-compat（只能加字段，不能删 / 改语义）
- 重大不兼容升 v2，v1 adapter 至少保留 6 个月
- 前端在独立 repo，pin proto version

### 6.2 IPC vs HTTP 性能差距

stdio JSON-RPC 比 HTTP 快 ~3-5x（无 socket 协商 / 无 HTTP header），但实现复杂度类似。

**应对**：
- IPC 作为本地优化路径
- 协议保持一致（同一份 proto，不同 transport）

### 6.3 服务端启动延迟

如果 lyra-server 启动 >1s，本地 TUI 用户感知差。

**应对**：
- 启动时 lazy init（不预热 LLM）
- 长连接复用（首次 spawn 后保活 idle 10min）
- 测量：M2 起跑 `time lyra serve --version` < 100ms

### 6.4 跟 MCP 的关系

Lyra-server 同时是：
- **MCP client**：消费外部 MCP server（github / filesystem / lsp 等）
- **MCP server**（M9）：把自己暴露成 MCP，让别的 agent 把 Lyra 当 subagent 调

这两个角色都是 transport，不是独立功能。在架构上一视同仁。

---

## 7. 成功标准（v0.1）

1. `go install ./lyra/cmd/lyra` → `lyra serve` 启动 < 100ms
2. 前端（任意 transport）能完成：单轮 chat / 文件操作 / Bash 执行 / 长对话不爆 context / session 恢复
3. 同一份 server binary 既能 stdio 模式跑（嵌入 TUI），也能 HTTP 模式跑（远端部署）
4. proto v1 稳定（v0.1 之后不破坏）
5. 文档：`PROTOCOL.md`（协议详解）+ `CLIENT.md`（接入指南，含示例代码）

---

## 8. 跟参考项目的关系（v2 更新）

| 项目 | 架构 | Lyra 借鉴 | Lyra 不借鉴 |
|---|---|---|---|
| **pi-mono** | CLI 单体（带 RPC mode） | event stream / session 树 / steering 队列 | extension-first（Lyra 是 product） |
| **Claude Code** | CLI 单体 + IDE bridge | tool 集 / auto-compact / LYRA.md memory / 三档 permission | 强耦合 IDE bridge / 多层 compaction 过度设计 |
| **Codex** | **CS：CLI ↔ app-server (Rust core)** | **多 transport / approval cache / sandbox 多后端 / sticky routing** | Rust 重写 |

**Codex 的 CS 设计跟 Lyra 思路最接近** —— `codex-cli` (TS) 通过 app-server protocol 跟 `codex-rs` (Rust core) 通信。我们抄它的 CS 架构思路，但用 Go 实现，且 transport 更多（HTTP / gRPC / IPC / MCP）。

---

## 9. 跟独立前端 repo 的协作

### 9.1 协议同步

```
1. Lyra repo: lyra/proto/lyra/v1/*.proto
2. Frontend repo: 通过其一方式消费
   a) git submodule lyra/proto
   b) buf.build module
   c) 自动生成的 npm 包 @lyra/proto-ts
3. CI: proto 改动触发前端 codegen → 跑前端测试 → 提 PR
```

### 9.2 开发流

- 协议变更 → Lyra 这边先提 proto PR
- 前端 repo 维护者审核协议改动
- 双方同步 merge

### 9.3 兼容性

服务端**永远兼容** v1 proto 直到 v0.2 主版本发布。客户端可以提前用 v2，服务端按 transport header 判断 client 期望版本，路由到对应 adapter。

---

*文档版本：v2（CS 架构）。v1（CLI 单体）见 git 历史。*
