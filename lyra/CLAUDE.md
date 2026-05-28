# CLAUDE.md — lyra module

> **Lyra Runtime** — Go agent runtime backend. 实现 Lyra Runtime Protocol
> （JSON-RPC 2.0, MCP-inspired）给 Wails / Web frontend 用（前端在 `/Users/tangerg/Desktop/lyra/`）。协议规范在前端仓 `docs/API.md` / `docs/TRANSPORT.md`。
>
> **项目级约定（设计原则 / 重构策略 / Go idiom / 共用反向不变量 / 沟通约定）见根级 [`../CLAUDE.md`](../CLAUDE.md)**。本文件只放 lyra 模块特有内容。

---

## 一句话定位

**协议层薄、业务层厚、传输层可换。** `rpc/` 是 wire 形态契约（JSON-RPC 2.0 / AG-UI events），`internal/engine` 是真正的"跑 chat turn + 工具循环"，`internal/service/*` 把业务能力按 domain 切片。Transport（HTTP+SSE / inprocess）只是 envelope I/O，对业务零感知。

## 技术栈

- Go 1.26.3
- **CLI**: `spf13/cobra`（每个子命令是 `App` 的方法 —— `ServeCmd()` / `ChatCmd()` / `AgentsCmd()` / …）
- **协议 envelope**: `modelcontextprotocol/go-sdk/jsonrpc`（wire 类型借用，不自己重新定义）
- **HTTP transport**: stdlib `net/http` + Go 1.22 `ServeMux` 的 `POST /v1/rpc/{method...}` pattern
- **SSE 写端**: `github.com/Tangerg/sse`（WHATWG §9.2 合规，auto-flush）—— 自家库
- **MCP 客户端**: `modelcontextprotocol/go-sdk/mcp`
- **AG-UI 事件**: `core/model/chat` + 自家 `internal/agui` 编码层
- **持久化**: 文件（默认 JSON / JSONL）或 SQLite（`modernc.org/sqlite` 纯 Go 无 CGO，`LYRA_STORAGE=sqlite`）
- **LLM provider**: `models/anthropic` / `models/openai`，按 `LYRA_PROVIDER` 分发
- **测试**: stdlib `testing` + `httptest`

## 三大支柱

1. **`rpc/` —— 协议契约**
   - `rpc/protocol/` 接口（`Runtime` + 10 个子接口）+ wire 数据类型
   - `rpc/server/` in-process `Runtime` 实现
   - `rpc/dispatch/` JSON-RPC 方法路由
   - `rpc/transport/` envelope I/O：`http/`（HTTP + SSE）/ `inprocess/`

2. **`internal/engine/` —— Chat 循环 + 工具编排**
   - facade，组合 `compactor` / `extractor` / `planner` / MCP sessions
   - 给 `chat.Service` 暴露 `chat.Engine` 窄接口（5 方法）—— **chat 不直接依赖 `*engine.Engine`**
   - 系统提示拼装在 `engine/prompt.go`

3. **`internal/service/*` —— Domain 切片**
   - 每个 domain 一个目录：`session` / `memory` / `chat` / `approval` / `tool` / `agentdoc`
   - 每个目录：`service.go`（interface）+ `inmemory.go` 或 `engine.go`（实现）+ tests

## 持久化后端

`LYRA_STORAGE=file|sqlite`（默认 `file`）：

| 后端 | session | memory | message |
|---|---|---|---|
| `file` | `internal/storage/FileSessionService` | `FileMemoryService`（`<cwd>/LYRA.md` + `~/.lyra/LYRA.md`，用户可编辑） | `FileMessageStore`（per-conversation JSONL append-only） |
| `sqlite` | `internal/storage/sqlite.NewSessionService` | `NewMemoryService` | **仍是 file**（JSONL append-only 暂无 SQL schema） |

切换在 `cmd/lyra/app.go:buildSessionAndMemory(kind)` 一处 switch。**SQLite 模式代价**：LYRA.md 不能直接编辑。

## Lyra-specific 强约定

跨模块共用规则见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的：

- **协议形态写死 JSON-RPC 2.0**：所有 message envelope 走 MCP SDK 的 `jsonrpc` 类型，**不重新定义 envelope**。HTTP transport 上 method 名照搬协议表（`POST /v1/rpc/runtime.initialize`，点保留不斜杠化）
- **业务 error → JSON-RPC `error.code`**（-32001..-32011 是扩展段），**不映射 HTTP status**。HTTP status 仅反映 transport 层（200 / 204 / 400 / 401 / 404 / 409 / 500 / 503）
- **Sidecar 端点只 `/v1/info` + `/v1/health` 两个**，flat JSON 不走 envelope，no-auth。**永远不加业务 read shadow**（如 `GET /v1/sessions/{id}`）
- **Transport 元数据走 header，不进 envelope**：trace id 用 `X-Lyra-Trace-Id`、本地 token 用 `Authorization: Bearer`、SSE 续连用 `Last-Event-Id`

## Lyra-specific 强反向不变量

跨模块共用反向不变量见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的：

- ❌ **Stdio transport**（CLI 给 LLM 用那种）：协议层有意不实现（前端 docs/API.md §1.1）。Web 走 HTTP loopback、TUI 走 inprocess
- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**：Runtime 协议层零 user 概念，鉴权由更外层（OS 信任、本地进程门禁 token、未来 facade）解决
- ❌ **业务方法的 RESTy read-only shadow**：业务调用一律 `POST /v1/rpc/{method}`。详见前端 docs/API.md §9.3
- ❌ **HTTP transport 换 chi / gin / echo / fiber**：4 个 endpoint + 3 个 middleware 用不上路由框架；fiber/echo 把 SSE 的 buffer/flush 搞砸过。stdlib `ServeMux` 1.22+ `{method...}` 已足
- ❌ **SSE 写自己的 frame 编码**：用 `github.com/Tangerg/sse`（auto-flush + spec compliance）。手写 `fmt.Fprintf(w, "data: %s\n\n", body)` 在 body 含 `\n` 时会破坏帧
- ❌ **`/v1/rpc` 不带 method**（裸路径）：v4 协议 greenfield 决议，单一形态 `POST /v1/rpc/{method}`，裸路径 404
- ❌ **协议 envelope 装 transport 元数据**（session id / auth token / trace id / idempotency key）：走 Go `context.Context` 或 HTTP header，永不进 message body

## 关键目录

```
lyra/
├── cmd/lyra/                       Cobra CLI 入口
│   ├── app.go                      App 装配 + ensureRuntime（buildSessionAndMemory 切 backend）
│   ├── serve.go                    `lyra serve` —— HTTP+SSE 起 server
│   ├── agents.go                   `lyra agents` —— 看哪些 AGENTS.md 会被加载
│   ├── repl.go / chat.go / memory.go / session.go / version.go
│   ├── root.go                     cobra root + subcommand 注册
│   └── runner.go                   main() 入口
│
├── rpc/                            协议层
│   ├── protocol/                   接口定义（Runtime + 子接口）+ wire types
│   ├── server/                     in-process Runtime 实现（绑 internal/service/*）
│   ├── dispatch/                   JSON-RPC method 路由
│   └── transport/
│       ├── transport.go            Transport 接口 + Message 类型别名
│       ├── http/                   HTTP+SSE transport（cors.go / auth.go / sidecar.go / stream.go / rpc.go）
│       └── inprocess/              同进程 chan transport（TUI 用）
│
├── internal/                       业务层
│   ├── config/                     LYRA_* env vars + Config struct + BuildChatClient
│   ├── runtime/                    顶层装配（service 注入 server.Server）
│   ├── engine/                     Chat 循环（agent / compactor / extractor / planner / mcp / prompt）
│   ├── service/                    Domain 切片
│   │   ├── session/                interface + inmemory + Repo（共享数据层）
│   │   ├── memory/                 LYRA.md 长期记忆
│   │   ├── chat/                   单 turn 状态机（engine 通过窄接口注入）
│   │   ├── approval/               Console / Gate / Service 三层 ISP
│   │   ├── tool/                   工具注册 + 直接调用
│   │   └── agentdoc/               AGENTS.md 级联发现 + render
│   ├── storage/                    File-backed 实现
│   │   └── sqlite/                 SQLite-backed 实现（modernc 纯 Go）
│   └── agui/                       AG-UI 事件编码
│
└── doc/                            ARCHITECTURE.md / ROADMAP.md / NAMING_REVIEW.md / REVIEW.md
```

## 常用命令

```bash
# 在 lyra/ 目录下跑
go build ./...           # 编译
go vet ./...             # 静态检查
go test ./...            # 全套测试

# 启动 server（dev）
ANTHROPIC_API_KEY=xxx ./lyra serve                       # 默认 127.0.0.1:17171（匹配 FE AGUI_BASE）
LYRA_STORAGE=sqlite ANTHROPIC_API_KEY=xxx ./lyra serve   # 切 SQLite

# 看本会话能读到哪些 AGENTS.md
./lyra agents --show

# 烟测某个 endpoint
curl http://127.0.0.1:17171/v1/info | jq                 # sidecar（no-auth）
curl -H "Authorization: Bearer $(cat ~/.lyra/local-token)" \
  -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"runtime.initialize","params":{}}' \
  http://127.0.0.1:17171/v1/rpc/runtime.initialize
```

## 修改任何东西之前

1. **`rpc/protocol/`**：动了协议契约 —— 前后端都要同步，先在前端仓 `docs/API.md` 对一遍
2. **`rpc/transport/http/`**：动了 transport —— 跑 `server_test.go` + `auth_cors_test.go` + `sidecar_test.go` 三个文件全套
3. **`internal/engine/`**：动了 chat 循环 —— 跑 `compaction_test.go` + chat stub-engine 测试
4. **`internal/service/<name>/`**：动 service interface —— 跑该包测试。如果改 interface 形状，搜下游 consumer
5. **`internal/storage/`**：动持久化 —— file backend 改了跑 `internal/storage/...`，sqlite 改了跑 `internal/storage/sqlite/...`
6. **`internal/service/agentdoc/`**：动 discovery 规则 —— 跑 `agentdoc_test.go`，烟测 `lyra agents` 在多层目录的输出
7. **加一个新 transport**：实现 `rpc/transport.Transport` 接口，新建 `rpc/transport/<name>/`，参考 inprocess（最简）和 http（最全）

## 已经做过的大重构（避免重复讨论）

- ✅ `pkg/` → `rpc/`（前后端通讯层）+ 子包重命名（coreapi → protocol / coreimpl → server / rpcadapter → dispatch）
- ✅ 接口去 Java 味：`xxxAPI` → 干名 / `In/Out` → `Request/Response` / `Impl` → `Server`
- ✅ JSON-RPC envelope 切到 MCP SDK 的 `jsonrpc` 包（不自维护）
- ✅ HTTP transport：local-token gate + CORS + 4xx access log + `/v1/info` ops 字段 + `/v1/health` 探针
- ✅ SSE 写端切到 `github.com/Tangerg/sse`
- ✅ SQLite 持久化（session / memory），`LYRA_STORAGE` env 切换
- ✅ AGENTS.md walk-from-cwd 级联发现（mirror kimi-code 设计）+ `lyra agents` CLI + `/v1/info.agentDocs`
- ✅ 删 speculative `trace` service stub
- ✅ chat 解耦 `*engine.Engine` —— 改用 `chat.Engine` 窄接口
- ✅ approval ISP split：`Console`（client）/ `Gate`（producer）/ `Service`（union）
- ✅ `impl.go` 全清：approval / chat → `inmemory.go`，tool → `engine.go`
- ✅ `atomicMode` wrapper → `atomic.Int32`
- ✅ `ProcessContextConfig` / `ProcessContext` 字段按 concern 分区
- ✅ MCP enum 在 config 和 engine 双份 → 合一份
- ✅ `fmt.Errorf("constant")` 全部改 `errors.New`
- ✅ autonomy 解耦 `*runtime.Platform` —— 改用包内 `platform` 窄接口 + compile-time tripwire
