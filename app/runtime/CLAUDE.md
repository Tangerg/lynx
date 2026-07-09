# CLAUDE.md — lyra module

> **Lyra Runtime** — Go agent runtime backend. 实现 Lyra Runtime Protocol
> （JSON-RPC 2.0, MCP-inspired）给 Wails / Web frontend 用（前端是同仓独立模块 [`../desktop`](../desktop)）。协议规范在 `../desktop/docs/protocol/API.md` / `../desktop/docs/protocol/TRANSPORT.md`。
>
> 项目级约定（设计原则 / 重构策略 / Go idiom / 共用反向不变量 / 沟通约定）见 `../CLAUDE.md`。本文件只放 lyra 模块特有内容。
>
> **目录命名（2026-06-14 重构落地）**：`engine→kernel` / `service→domain` / `rpc→internal/delivery` / `engine/chat→kernel/turn`，目录名 = Clean Arch 环名（架构基准见 [`doc/GREENFIELD_ARCHITECTURE.md`](doc/GREENFIELD_ARCHITECTURE.md)）。

---

## 一句话定位

**协议层薄、业务层厚、传输层可换。** `internal/delivery/` 是 wire 形态契约（JSON-RPC 2.0，Lyra Runtime Protocol 自有事件/Item 模型），`internal/kernel` 是真正的"跑 chat turn + 工具循环"，`internal/domain/*` 把业务能力按 domain 切片。Transport（HTTP+SSE / inprocess）只是 envelope I/O，对业务零感知。

## 技术栈

- Go 1.26.4
- **CLI**: `spf13/cobra`（每个子命令是 `App` 的方法 —— `ServeCmd()` / `ChatCmd()` / `AgentsCmd()` / …）
- **协议 envelope**: `modelcontextprotocol/go-sdk/jsonrpc`（wire 类型借用，不自己重新定义）
- **HTTP transport**: stdlib `net/http` + `go-chi/chi` v5 路由（`r.Use` 中间件链 + `POST /v2/rpc/{method}`）+ `go-chi/cors`（CORS 中间件，替掉手写 cors）。**streamable HTTP**：流式方法（`runs.start/resume/subscribe`、`background.subscribe`）的 POST 响应体本身就是 `text/event-stream` 事件流（首帧=JSON-RPC 响应，无 SSE id；其后 `notifications.run.event` 帧带 SSE id=eventId）；**无独立 `/v2/rpc/stream` 端点、无 `X-Conn-Id` 连接路由**。事件源是 server 侧的 per-run hub（`internal/delivery/server/hub.go`），transport 只是把 hub 订阅抽成 SSE
- **SSE 写端**: `github.com/Tangerg/sse`（WHATWG §9.2 合规，auto-flush）—— 自家库
- **MCP 客户端**: `modelcontextprotocol/go-sdk/mcp`
- **事件/Item 编码**: `turn.Event`（业务层）→ `internal/delivery/server/translator.go` 翻译成 wire `StreamEvent`/`Item`（Lyra Runtime Protocol 自有模型，**非 AG-UI**——该名词已弃用）
- **持久化**: 单一 SQLite 后端（`modernc.org/sqlite` 纯 Go 无 CGO，`$LYRA_HOME/lyra.db`）；唯一例外是用户可编辑的 LYRA.md memory 文件（详见 §持久化后端）
- **LLM provider**: 多 provider × 多 model。`internal/domain/provider` 是运行态可变注册表（每 provider 的 key+baseURL，file/sqlite 持久化）；`config.BuildClient(ClientSpec)` 按 provider id 建 client（anthropic/openai/moonshot/deepseek，后两者走 OpenAI 兼容端点，全部支持 baseURL 覆盖）。**per-run model**：`runs.start{provider, model}`(显式配对,缺一即 `invalid_params`、都缺用默认 —— provider **不从 model 推断**)→ `clientResolver` 取该 provider 的注册表凭证建/缓存 client → 经 agent `core.ChatClientProvider` 扩展点让该 turn 用它。model 元数据/定价/能力全来自公开的 `models/catalog`（models.list 直读，无需 key）。`config.yaml` 的 `provider`/`apiKey`/`baseURL` 是默认 provider 的种子
- **测试**: stdlib `testing` + `httptest`

## 架构分层（Clean Arch 依赖向内：delivery → adapter/kernel → domain；infra 是最外侧 driven adapter，见 doc/GREENFIELD_ARCHITECTURE.md）

1. **`internal/delivery/` —— 协议契约（delivery / 接口适配器，原 `rpc/`）**
   - `internal/delivery/protocol/` 接口（`Runtime` + 12 个子接口）+ wire 数据类型 + v2 协议错误码 / Item / RunEvent 形状
   - `internal/delivery/server/` in-process `Runtime` 实现
   - `internal/delivery/dispatch/` JSON-RPC 方法路由
   - `internal/delivery/transport/` envelope I/O：`http/`（HTTP + SSE）/ `inprocess/`

2. **`internal/kernel/` —— 微内核（装配 + 驱动 agent loop，原 `engine/`）**
   - facade：装配 system prompt（`kernel/prompt.go`）+ 工具集 + model client，驱动 agent SDK 跑一个 turn
   - turn 边界维护由 `internal/adapter/maintenance` 实现 kernel-owned `Compactor` / `Extractor` port；kernel 只编排接口，不依赖具体实现
   - `internal/kernel/accounting` 承载 token/cost accounting 值对象；engine 只驱动记录和预算判断
   - **`internal/kernel/turn`** —— "跑一个 turn" 用例（状态机 / lifecycle / observer / policy，原 `engine/chat`）。它是编排层（不是 domain service），经包内 `engineDep` 窄接口驱动 `*kernel.Engine`（同层 子包→父包,非反向边）
   - 详见 [`doc/GREENFIELD_ARCHITECTURE.md`](doc/GREENFIELD_ARCHITECTURE.md)：依赖一律向内（`internal/arch/arch_test.go` 机器强制）

3. **`internal/domain/*` —— 限界上下文（领域层，实体 + 领域服务 + consumer-side port，原 `service/`）**
   - 每个 domain 一个目录：`session` / `knowledge`（LYRA.md）/ `transcript`（items+runs）/ `conversation` / `approval` / `tool` / `agentdoc` / `interrupts` / `provider` / `skills` / `todo` / `editguard` / `codebaseindex` / `mcpserver` / `recipes` / `run` / `schedule`
   - 每个目录：`service.go`（interface）+ 实现（按本质命名）+ tests

4. **`internal/adapter/*` —— 能力适配器（实现 kernel/domain port，包装外部能力）**
   - `maintenance`（压缩 / 提取 / 标题）/ `toolset`（工具装配）/ `codeintel` / `workspace` / `hooks` / `codebaseindex` / `persistence` / `observability` / `pricing` / `startup`

5. **`internal/infra/*` —— 技术设施（零领域,不依赖任何上层）**
   - `infra/storage`（sqlite + file LYRA.md）/ `infra/git` / `infra/lsp` / `infra/checkpoint` / `infra/exec` / `infra/mcp` / `infra/a2a`

## 持久化后端

**dev 阶段单一后端：SQLite**（`modernc.org/sqlite` 纯 Go 无 CGO），`$LYRA_HOME/lyra.db` 一个 *sql.DB 跨表持久化 **session / process-snapshot / interrupt / history / provider / conversation message**。没有 `LYRA_STORAGE` 开关、没有 file/in-memory 并行实现（已删）。

**唯一例外仍是文件**（"用户可编辑" 是它存在的意义）：
- **knowledge**：`internal/infra/storage/FileKnowledgeService`（`<会话 cwd>/LYRA.md` + `~/.lyra/LYRA.md`，用户可 `cat`/编辑；实现 `domain/knowledge.Service`，project scope 按每次调用传入的 dir 寻址——一个 service 服务所有项目，空 dir 回退构造时的进程 cwd）。
- （AGENTS.md 是发现，不是 storage。）

持久化 bundle 装配在 `internal/adapter/persistence.Open()`（恒 sqlite + file LYRA.md memory），`cmd/lyra` 只调用它；bundle → `runtime.Config` 的投影在 `internal/adapter/startup.RuntimeConfig()`。runtime **不再有 in-memory 回退** —— session/interrupt/provider 由 composition root 注入;测试 + 回退用 `sqlite.Open(":memory:" 或 temp file)`。

## Lyra-specific 强约定

跨模块共用规则见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的：

- **协议形态写死 JSON-RPC 2.0**：所有 message envelope 走 MCP SDK 的 `jsonrpc` 类型，**不重新定义 envelope**。HTTP transport 上 method 名照搬协议表（`POST /v2/rpc/runtime.initialize`，点保留不斜杠化）
- **业务 error → JSON-RPC `error.code`**（`-32001..-32016` 是扩展段，客户端按 `error.data.type` 的 symbolic name 分支、不按数字码），**不映射 HTTP status**。HTTP status 仅反映 transport 层（TRANSPORT §6.3：200 / 204 通知 / 400 含 method 不一致 / 401 带 `WWW-Authenticate` / 404 / 405 带 `Allow` / 413 / 415 / 500 / 503；**无 409** —— 自相矛盾请求归 400 而非资源冲突）
- **Sidecar 端点只 `/v2/info` + `/v2/health` 两个**，flat JSON 不走 envelope，no-auth。**永远不加业务 read shadow**（如 `GET /v2/sessions/{id}`）
- **Transport 元数据走 header，不进 envelope**：trace 关联走 **W3C `traceparent`**（otel 标准 propagator，无自有 `X-Trace-Id`）、本地 token 用 `Authorization: Bearer`、SSE 续连用 `Last-Event-Id`、协议版本 `X-Protocol-Version`、幂等键 `X-Idempotency-Key`（响应侧另有 `X-Server` / `X-Method` / SSE 的 `X-Accel-Buffering`）。**无 `X-Conn-Id`**（streamable HTTP 已无连接路由）

## Lyra-specific 强反向不变量

跨模块共用反向不变量见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的：

- ❌ **Stdio transport**（CLI 给 LLM 用那种）：协议层有意不实现（`../desktop/docs/protocol/API.md` §1.1）。Web 走 HTTP loopback、TUI 走 inprocess
- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**：Runtime 协议层零 user 概念，鉴权由更外层（OS 信任、本地进程门禁 token、未来 facade）解决
- ❌ **业务方法的 RESTy read-only shadow**：业务调用一律 `POST /v2/rpc/{method}`。详见 `../desktop/docs/protocol/API.md` §9.3
- ❌ **HTTP transport 换 gin / echo / fiber**：它们用自家 ctx / ResponseWriter，把 SSE 的 buffer/flush 搞砸过。**chi 是例外、已采用**：它就是标准 `net/http` handler（SSE flush 与 stdlib 一致），且 `go-chi/cors` 直接替掉了手写 CORS（见 §技术栈 + §已做过的大重构）。所以"换 router"≠"换 chi"
- ❌ **SSE 写自己的 frame 编码**：用 `github.com/Tangerg/sse`（auto-flush + spec compliance）。手写 `fmt.Fprintf(w, "data: %s\n\n", body)` 在 body 含 `\n` 时会破坏帧
- ❌ **`/v2/rpc` 不带 method**（裸路径）：v2 协议 greenfield 决议，单一形态 `POST /v2/rpc/{method}`，裸路径 404
- ❌ **协议 envelope 装 transport 元数据**（session id / auth token / trace id / idempotency key）：走 Go `context.Context` 或 HTTP header，永不进 message body
- ❌ **退回"常开的 server→client 通知通道"**（独立 `GET /v2/rpc/stream` + `X-Conn-Id` 连接路由 + 全局/广播 fan-out）：已被 streamable HTTP 取代 —— 每个流式调用的事件走它**自己那条 POST 响应流**，事件源是 per-run hub（`internal/delivery/server/hub.go`），无连接身份簿记。重连是 per-run（`runs.subscribe{runId}` + `Last-Event-Id`），不是"重连那条共享流"。真要 server 主动推送（多客户端同步等，API.md §12 当前不做），按 `../desktop/docs/protocol/TRANSPORT.md` §6.1 的退路**增量**加一条可选 GET 流，别把旧模型整套搬回来

## 关键目录

```
lyra/
├── cmd/lyra/                       Cobra CLI 入口
│   ├── app.go                      App 装配
│   ├── runtime_bootstrap.go        ensureRuntime 启动剧本（调用 adapter/startup + adapter/persistence）
│   ├── serve.go                    `lyra serve` —— HTTP+SSE 起 server
│   ├── agents.go                   `lyra agents` —— 看哪些 AGENTS.md 会被加载
│   ├── repl.go / chat.go / memory.go / session.go / version.go
│   ├── root.go                     cobra root + subcommand 注册
│   └── runner.go                   main() 入口
│
├── internal/                       Clean Arch 同心环（依赖向内,arch_test 强制；见 doc/GREENFIELD_ARCHITECTURE.md）
│   ├── config/                     纯配置加载：LYRA_* env vars + Config struct
│   ├── runtime/                    组合根：装配各环 + nil-default 注入 SPI（绑进 delivery/server.Server）
│   ├── delivery/                   交付 / 接口适配器（原 rpc/）
│   │   ├── protocol/               接口定义（Runtime + 子接口）+ wire types
│   │   ├── server/                 in-process Runtime 实现（绑 internal/domain/*）
│   │   ├── dispatch/               JSON-RPC method 路由
│   │   └── transport/
│   │       ├── transport.go        Transport 接口 + Message 类型别名
│   │       ├── http/               HTTP+SSE transport（cors.go / auth.go / sidecar.go / stream.go / rpc.go）
│   │       └── inprocess/          同进程 chan transport（TUI 用）
│   ├── kernel/                     微内核：装配 + 驱动 agent loop（原 engine/；prompt / port / turn 编排）
│   │   ├── port.go                 核定义的窄 port（model / 工具集 / maintenance / prompt-ctx / conversation）
│   │   ├── turn/                    "跑一个 turn" 用例（状态机 / lifecycle / observer / policy；经 engineDep 窄接口驱动 kernel.Engine）（原 chat/）
│   │   ├── accounting/              token/cost usage 值对象
│   │   ├── toolport/               工具调用端口与 registry 边界
│   │   ├── turnctx/                turn-scoped 上下文值
│   │   └── lifecycle/              run admission / lifecycle registry
│   ├── domain/                     领域层：限界上下文（一域一包，零 adapter/kernel/infra 依赖）（原 service/）
│   │   ├── session/                会话生命周期
│   │   ├── knowledge/              LYRA.md 长期知识
│   │   ├── transcript/             items+runs 时间线
│   │   ├── conversation/           喂 LLM 的消息上下文
│   │   ├── approval/               运行态审批 stance（`Mode`）—— R 模型工具审批查它
│   │   ├── tool/                   工具注册 + 直接调用（自定义 source 窄接口,不 import kernel）
│   │   ├── editguard/              read-before-edit + stale 不变量（纯领域；toolset 的 guard 包装是其 LLM presentation）
│   │   ├── interrupts/ provider/   HITL 中断登记 / provider 注册表
│   │   ├── skills/ todo/           skill 取用 / 任务清单
│   │   └── agentdoc/               AGENTS.md 级联发现 + render
│   ├── adapter/                    能力适配器：实现 kernel/domain port，包装外部能力
│   │   ├── maintenance/            压缩 / 提取 / 标题（kernel maintenance port 实现）
│   │   ├── persistence/            SQLite + LYRA.md 持久化 bundle 装配
│   │   ├── startup/                config/env → runtime/domain 投影 + 启动 seed
│   │   ├── observability/ pricing/ OTel 进程 bootstrap / catalog pricing port
│   │   ├── toolset/                工具装配层（builders + resolver，loop 之外组好注入）
│   │   ├── codeintel/ workspace/   代码智能 / VCS 视图 + checkpoint
│   │   └── hooks/ codebaseindex/   hook 执行适配 / 代码索引适配
│   └── infra/                      技术设施（零领域,不依赖任何上层）
│       ├── storage/                SQLite（modernc 纯 Go）+ file LYRA.md（FileKnowledgeService）
│       │   └── sqlite/
│       └── git/ lsp/ checkpoint/ exec/ mcp/ a2a/   git / LSP client / 影子 git / 进程执行 / MCP / A2A
│
└── doc/                            GREENFIELD_ARCHITECTURE.md（架构基准）/ EXTENSIBILITY.md / ARCHITECTURE_REVIEW.md / GREENFIELD_DESIGN.md / PRIOR_ART.md（业界横向对照）
```

## 常用命令

```bash
# 在 lyra/ 目录下跑
go build ./...           # 编译
go vet ./...             # 静态检查
go test ./...            # 全套测试

# 启动 server（dev）
ANTHROPIC_API_KEY=xxx ./lyra serve                       # 默认 127.0.0.1:17171（匹配前端默认 base），SQLite at $LYRA_HOME/lyra.db

# 看本会话能读到哪些 AGENTS.md
./lyra agents --show

# 烟测某个 endpoint
curl http://127.0.0.1:17171/v2/info | jq                 # sidecar（no-auth）
curl -H "Authorization: Bearer $(cat ~/.lyra/local-token)" \
  -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}' \
  http://127.0.0.1:17171/v2/rpc/runtime.initialize
```

## 修改任何东西之前

1. **`internal/delivery/protocol/`**：动了协议契约 —— 前后端都要同步，先在 `../desktop/docs/protocol/API.md` 对一遍
2. **`internal/delivery/transport/http/`**：动了 transport —— 跑 `server_test.go` + `auth_cors_test.go` + `sidecar_test.go` 三个文件全套
3. **`internal/kernel/`**：动了编排 / turn 循环 —— 跑 `internal/kernel/...`（含 `kernel/turn` 的 stub-engine 测试 + `adapter/maintenance` 压缩测试）
4. **`internal/domain/<name>/`**：动 service interface —— 跑该包测试。如果改 interface 形状，搜下游 consumer
5. **`internal/infra/storage/`**：动持久化 —— sqlite 改了跑 `internal/infra/storage/sqlite/...`；file knowledge / message 改了跑 `internal/infra/storage/...`
6. **`internal/domain/agentdoc/`**：动 discovery 规则 —— 跑 `agentdoc_test.go`，烟测 `lyra agents` 在多层目录的输出
7. **加一个新 transport**：实现 `internal/delivery/transport.Transport` 接口，新建 `internal/delivery/transport/<name>/`，参考 inprocess（最简）和 http（最全）

## 已经做过的大重构

> 历史大重构流水账见 git log。
