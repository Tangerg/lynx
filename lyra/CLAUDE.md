# CLAUDE.md — lyra module

> **Lyra Runtime** — Go agent runtime backend. 实现 Lyra Runtime Protocol
> （JSON-RPC 2.0, MCP-inspired）给 Wails / Web frontend 用（前端在 `/Users/tangerg/Desktop/lyra/`）。协议规范在前端仓 `docs/API.md` / `docs/TRANSPORT.md`。
>
> 项目级约定（设计原则 / 重构策略 / Go idiom / 共用反向不变量 / 沟通约定）见 `../CLAUDE.md`。本文件只放 lyra 模块特有内容。
>
> **目录命名（2026-06-14 重构落地）**：`engine→kernel` / `service→domain` / `rpc→internal/delivery` / `engine/chat→kernel/turn`，目录名 = Clean Arch 环名（见 [`doc/GREENFIELD_ARCHITECTURE.md`](doc/GREENFIELD_ARCHITECTURE.md)）。下方「已经做过的大重构」changelog 保留各条目**当时**的路径名（历史记录，勿据以定位当前代码）。

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

## 架构分层（Clean Arch 依赖向内：delivery → kernel → domain → infra，见 doc/GREENFIELD_ARCHITECTURE.md）

1. **`internal/delivery/` —— 协议契约（delivery / 接口适配器，原 `rpc/`）**
   - `internal/delivery/protocol/` 接口（`Runtime` + 12 个子接口）+ wire 数据类型 + v2 协议错误码 / Item / RunEvent 形状
   - `internal/delivery/server/` in-process `Runtime` 实现
   - `internal/delivery/dispatch/` JSON-RPC 方法路由
   - `internal/delivery/transport/` envelope I/O：`http/`（HTTP + SSE）/ `inprocess/`

2. **`internal/kernel/` —— 微内核（装配 + 驱动 agent loop，原 `engine/`）**
   - facade：装配 system prompt（`kernel/prompt.go`）+ 工具集 + model client，驱动 agent SDK 跑一个 turn
   - 领域算法（压缩/提取/规划）已下沉到 `internal/domain/maintenance`，kernel 经 `*maintenance.{Compactor,Extractor,Planner}` 编排
   - **`internal/kernel/turn`** —— "跑一个 turn" 用例（状态机 / lifecycle / observer / policy，原 `engine/chat`）。它是编排层（不是 domain service），经包内 `engineDep` 窄接口驱动 `*kernel.Engine`（同层 子包→父包,非反向边）
   - 详见 [`doc/GREENFIELD_ARCHITECTURE.md`](doc/GREENFIELD_ARCHITECTURE.md)：分层为 `delivery → 微内核(kernel + kernel/turn) → 领域(domain/*) → infra(infra/*)`，依赖一律向内（`internal/arch/arch_test.go` 机器强制）

3. **`internal/domain/*` —— 限界上下文（领域层,只向下依赖 infra，原 `service/`）**
   - 每个 domain 一个目录：`session` / `knowledge`（LYRA.md）/ `transcript`（items+runs）/ `conversation` / `approval` / `tool` / `agentdoc` / `codeintel`（包 infra/lsp）/ `workspace`（包 infra/git+checkpoint）/ `maintenance`（压缩·提取·规划）/ `interrupts` / `provider` / `skills` / `todo` / `editguard`
   - 每个目录：`service.go`（interface）+ 实现（按本质命名）+ tests

4. **`internal/infra/*` —— 技术设施（零领域,不依赖任何上层）**
   - `infra/storage`（sqlite + file LYRA.md）/ `infra/git` / `infra/lsp` / `infra/checkpoint` / `infra/exec` / `infra/mcp` / `infra/a2a`

## 持久化后端

**dev 阶段单一后端：SQLite**（`modernc.org/sqlite` 纯 Go 无 CGO），`$LYRA_HOME/lyra.db` 一个 *sql.DB 跨表持久化 **session / process-snapshot / interrupt / history / provider / message（chat-memory）**。没有 `LYRA_STORAGE` 开关、没有 file/in-memory 并行实现（已删）。

**唯一例外仍是文件**（"用户可编辑" 是它存在的意义）：
- **knowledge**：`internal/infra/storage/FileKnowledgeService`（`<会话 cwd>/LYRA.md` + `~/.lyra/LYRA.md`，用户可 `cat`/编辑；实现 `domain/knowledge.Service`，project scope 按每次调用传入的 dir 寻址——一个 service 服务所有项目，空 dir 回退构造时的进程 cwd）。
- （AGENTS.md 是发现，不是 storage。）

装配在 `cmd/lyra/app.go:buildStores()`（恒 sqlite + file LYRA.md memory）。runtime **不再有 in-memory 回退** —— session/interrupt/provider 由 composition root 注入;测试 + 回退用 `sqlite.Open(":memory:" 或 temp file)`。

## Lyra-specific 强约定

跨模块共用规则见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的：

- **协议形态写死 JSON-RPC 2.0**：所有 message envelope 走 MCP SDK 的 `jsonrpc` 类型，**不重新定义 envelope**。HTTP transport 上 method 名照搬协议表（`POST /v2/rpc/runtime.initialize`，点保留不斜杠化）
- **业务 error → JSON-RPC `error.code`**（`-32001..-32016` 是扩展段，客户端按 `error.data.type` 的 symbolic name 分支、不按数字码），**不映射 HTTP status**。HTTP status 仅反映 transport 层（TRANSPORT §6.3：200 / 204 通知 / 400 含 method 不一致 / 401 带 `WWW-Authenticate` / 404 / 405 带 `Allow` / 413 / 415 / 500 / 503；**无 409** —— 自相矛盾请求归 400 而非资源冲突）
- **Sidecar 端点只 `/v2/info` + `/v2/health` 两个**，flat JSON 不走 envelope，no-auth。**永远不加业务 read shadow**（如 `GET /v2/sessions/{id}`）
- **Transport 元数据走 header，不进 envelope**：trace 关联走 **W3C `traceparent`**（otel 标准 propagator，无自有 `X-Trace-Id`）、本地 token 用 `Authorization: Bearer`、SSE 续连用 `Last-Event-Id`、协议版本 `X-Protocol-Version`、幂等键 `X-Idempotency-Key`（响应侧另有 `X-Server` / `X-Method` / SSE 的 `X-Accel-Buffering`）。**无 `X-Conn-Id`**（streamable HTTP 已无连接路由）

## Lyra-specific 强反向不变量

跨模块共用反向不变量见 [`../CLAUDE.md`](../CLAUDE.md)。下面是 lyra 协议层独有的：

- ❌ **Stdio transport**（CLI 给 LLM 用那种）：协议层有意不实现（前端 docs/API.md §1.1）。Web 走 HTTP loopback、TUI 走 inprocess
- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**：Runtime 协议层零 user 概念，鉴权由更外层（OS 信任、本地进程门禁 token、未来 facade）解决
- ❌ **业务方法的 RESTy read-only shadow**：业务调用一律 `POST /v2/rpc/{method}`。详见前端 docs/API.md §9.3
- ❌ **HTTP transport 换 gin / echo / fiber**：它们用自家 ctx / ResponseWriter，把 SSE 的 buffer/flush 搞砸过。**chi 是例外、已采用**：它就是标准 `net/http` handler（SSE flush 与 stdlib 一致），且 `go-chi/cors` 直接替掉了手写 CORS（见 §技术栈 + §已做过的大重构）。所以"换 router"≠"换 chi"
- ❌ **SSE 写自己的 frame 编码**：用 `github.com/Tangerg/sse`（auto-flush + spec compliance）。手写 `fmt.Fprintf(w, "data: %s\n\n", body)` 在 body 含 `\n` 时会破坏帧
- ❌ **`/v2/rpc` 不带 method**（裸路径）：v2 协议 greenfield 决议，单一形态 `POST /v2/rpc/{method}`，裸路径 404
- ❌ **协议 envelope 装 transport 元数据**（session id / auth token / trace id / idempotency key）：走 Go `context.Context` 或 HTTP header，永不进 message body
- ❌ **退回"常开的 server→client 通知通道"**（独立 `GET /v2/rpc/stream` + `X-Conn-Id` 连接路由 + 全局/广播 fan-out）：已被 streamable HTTP 取代 —— 每个流式调用的事件走它**自己那条 POST 响应流**，事件源是 per-run hub（`rpc/server/hub.go`），无连接身份簿记。重连是 per-run（`runs.subscribe{runId}` + `Last-Event-Id`），不是"重连那条共享流"。真要 server 主动推送（多客户端同步等，API.md §12 当前不做），按前端 docs/TRANSPORT §6.1 的退路**增量**加一条可选 GET 流，别把旧模型整套搬回来

## 关键目录

```
lyra/
├── cmd/lyra/                       Cobra CLI 入口
│   ├── app.go                      App 装配 + ensureRuntime（buildStores 恒 sqlite + file memory）
│   ├── serve.go                    `lyra serve` —— HTTP+SSE 起 server
│   ├── agents.go                   `lyra agents` —— 看哪些 AGENTS.md 会被加载
│   ├── repl.go / chat.go / memory.go / session.go / version.go
│   ├── root.go                     cobra root + subcommand 注册
│   └── runner.go                   main() 入口
│
├── internal/                       Clean Arch 同心环（依赖向内,arch_test 强制；见 doc/GREENFIELD_ARCHITECTURE.md）
│   ├── config/                     组合根：LYRA_* env vars + Config struct + BuildChatClient
│   ├── runtime/                    组合根：装配各环 + nil-default 注入 SPI（绑进 delivery/server.Server）
│   ├── delivery/                   交付 / 接口适配器（原 rpc/）
│   │   ├── protocol/               接口定义（Runtime + 子接口）+ wire types
│   │   ├── server/                 in-process Runtime 实现（绑 internal/domain/*）
│   │   ├── dispatch/               JSON-RPC method 路由
│   │   └── transport/
│   │       ├── transport.go        Transport 接口 + Message 类型别名
│   │       ├── http/               HTTP+SSE transport（cors.go / auth.go / sidecar.go / stream.go / rpc.go）
│   │       └── inprocess/          同进程 chan transport（TUI 用）
│   ├── kernel/                     微内核：装配 + 驱动 agent loop（原 engine/；agent / mcp / a2a / prompt / 工具集）
│   │   ├── port.go                 核定义的窄 port（model / 工具集 / maintenance / prompt-ctx / conversation）
│   │   ├── turn/                    "跑一个 turn" 用例（状态机 / lifecycle / observer / policy；经 engineDep 窄接口驱动 kernel.Engine）（原 chat/）
│   │   └── toolset/                工具装配层（builders + resolver，loop 之外组好注入）
│   ├── domain/                     领域层：限界上下文（一域一包,只向下依赖 infra）（原 service/）
│   │   ├── session/                会话生命周期
│   │   ├── knowledge/              LYRA.md 长期知识
│   │   ├── transcript/             items+runs 时间线
│   │   ├── conversation/           喂 LLM 的消息上下文
│   │   ├── maintenance/            压缩 / 提取 / 规划（turn 边界自治操作）
│   │   ├── codeintel/              代码智能（包住 infra/lsp）
│   │   ├── workspace/              VCS 视图 + 文件 checkpoint（包住 infra/git + infra/checkpoint）
│   │   ├── approval/               运行态审批 stance（`Mode`）—— R 模型工具审批查它
│   │   ├── tool/                   工具注册 + 直接调用（自定义 source 窄接口,不 import kernel）
│   │   ├── editguard/              read-before-edit + stale 不变量（纯领域；toolset 的 guard 包装是其 LLM presentation）
│   │   ├── interrupts/ provider/   HITL 中断登记 / provider 注册表
│   │   ├── skills/ todo/           skill 取用 / 任务清单
│   │   └── agentdoc/               AGENTS.md 级联发现 + render
│   └── infra/                      技术设施（零领域,不依赖任何上层）
│       ├── storage/                SQLite（modernc 纯 Go）+ file LYRA.md（FileKnowledgeService）
│       │   └── sqlite/
│       └── git/ lsp/ checkpoint/ exec/ mcp/ a2a/   git / LSP client / 影子 git / 进程执行 / MCP / A2A
│
└── doc/                            GREENFIELD_ARCHITECTURE.md（当前结构基准）/ MICROKERNEL.md / LAYERING.md / ...
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

1. **`internal/delivery/protocol/`**：动了协议契约 —— 前后端都要同步，先在前端仓 `docs/API.md` 对一遍
2. **`internal/delivery/transport/http/`**：动了 transport —— 跑 `server_test.go` + `auth_cors_test.go` + `sidecar_test.go` 三个文件全套
3. **`internal/kernel/`**：动了编排 / turn 循环 —— 跑 `internal/kernel/...`（含 `kernel/turn` 的 stub-engine 测试 + `domain/maintenance` 压缩测试）
4. **`internal/domain/<name>/`**：动 service interface —— 跑该包测试。如果改 interface 形状，搜下游 consumer
5. **`internal/infra/storage/`**：动持久化 —— sqlite 改了跑 `internal/infra/storage/sqlite/...`；file knowledge / message 改了跑 `internal/infra/storage/...`
6. **`internal/domain/agentdoc/`**：动 discovery 规则 —— 跑 `agentdoc_test.go`，烟测 `lyra agents` 在多层目录的输出
7. **加一个新 transport**：实现 `internal/delivery/transport.Transport` 接口，新建 `internal/delivery/transport/<name>/`，参考 inprocess（最简）和 http（最全）

## 已经做过的大重构（避免重复讨论）

- ✅ `pkg/` → `rpc/`（前后端通讯层）+ 子包重命名（coreapi → protocol / coreimpl → server / rpcadapter → dispatch）
- ✅ 接口去 Java 味：`xxxAPI` → 干名 / `In/Out` → `Request/Response` / `Impl` → `Server`
- ✅ JSON-RPC envelope 切到 MCP SDK 的 `jsonrpc` 包（不自维护）
- ✅ HTTP transport：local-token gate + CORS + 4xx access log + `/v2/info` ops 字段 + `/v2/health` 探针
- ✅ HTTP transport 路由从 stdlib `ServeMux` 换 `go-chi/chi` v5 + CORS 换 `go-chi/cors`：删手写 `cors.go`（106→40 行），`r.Use(observability, cors, authGate)` 比内嵌包裹更可读；chi 用标准 handler 故 SSE 不受影响。**预检状态从 204→200**（go-chi/cors 行为，契约对 2xx 不挑）。gin/echo/fiber 仍禁（破坏 SSE）
- ✅ HTTP transport 对齐冻结 v2 契约（TRANSPORT §6.2/§6.3/§8/§12 + API.md §8.3）：超限 body 返 `413`（原静默截断）、非 JSON content-type 返 `415`、method 不一致从 `409`→`400`（自相矛盾请求非资源冲突）、通知 ack 保持 `204`（契约明确弃 202）、401 带 `WWW-Authenticate: Bearer`、405 带 `Allow`（chi 自动）、`/v2/health` body 补 `{"ok":true}`、`ProblemData` 补 `retryAfterSeconds`/`errors[]`（RFC 9457 裁剪）
- ✅ **streamable HTTP 改造**（取代上一条的"按 conn/run 路由"+ 独立 SSE 流，与前端并行落地，TRANSPORT §6.4）：每个流式调用的事件走它自己那条 `POST` 响应流（首帧=JSON-RPC ack 无 SSE id，其后 run 事件帧带 SSE id=eventId）。**A** per-run hub（`rpc/server/hub.go`：durable 重放 buffer + N 订阅 fan-out，只存 durable，§9.3）；**B** `pumpRun` 喂 hub、`StartRun/Resume/SubscribeRun` 返回 hub 订阅、`Last-Event-Id` 走 ctx（`WithLastEventID`）；**C** transport 翻转 —— streaming POST 直接驱 SSE，**删** `GET /v2/rpc/stream` + `clientRegistry`(conn 路由) + transport 侧 buffer + `X-Conn-Id` + 死 `api` 字段；**D** 文档 + CORS 去 `X-Conn-Id`。重连是 per-run（`runs.subscribe`+`Last-Event-Id`）。连接预算：前端只对**活跃 run** 开流（HTTP/1.1 ~6 连接/origin；h2c 对浏览器无效，需多 live 才上 loopback TLS）
- ✅ items.list 权威 Item 存储（`internal/service/history`，file+sqlite）：事件流落库，runId/runs/createdAt/item-id 与 live 一致；replaces 从 chat.Message 反推
- ✅ SSE 写端切到 `github.com/Tangerg/sse`
- ✅ SQLite 持久化（session / memory），`LYRA_STORAGE` env 切换
- ✅ AGENTS.md walk-from-cwd 级联发现（mirror kimi-code 设计）+ `lyra agents` CLI + `/v2/info.agentDocs`
- ✅ 删 speculative `trace` service stub
- ✅ chat 解耦 `*engine.Engine` —— 改用 `chat.Engine` 窄接口
- ✅ `impl.go` 全清：approval / chat → `inmemory.go`，tool → `engine.go`
- ✅ `atomicMode` wrapper → `atomic.Int32`
- ✅ `ProcessContextConfig` / `ProcessContext` 字段按 concern 分区
- ✅ MCP enum 在 config 和 engine 双份 → 合一份
- ✅ `fmt.Errorf("constant")` 全部改 `errors.New`
- ✅ autonomy 解耦 `*runtime.Platform` —— 改用包内 `platform` 窄接口 + compile-time tripwire
- ✅ **v2 协议迁移**（前端 docs/API.md `protocolVersion 2026-06-03`）：`rpc/protocol` 全面重写成 Session→Run→**Item** 模型（Item 是唯一 history+streaming primitive，无 Message 类型）；`/v1/`→`/v2/` 路径；X- header 去品牌前缀（`X-Trace-Id` 等）；单一 `notifications.run.event` 事件流；错误码按 `error.data.type` symbolic 分支
- ✅ **HITL P→R 模型**：工具审批 / plan-review 从阻塞式 gate 改成 **park-on-interrupt + 续连 resume** —— run 以 `run.finished{outcome:interrupt}` 收尾，客户端经 `runs.resume`(parentRunId 链) 应答；engine 经 `hitl.PauseError` seam 中途挂起、`Platform.ResumeProcess` 把 verdict 写回 blackboard（keyed by tool name+args）让 re-run 观察到；中断存储 `interrupts.Store` 做成可插拔接口（默认 in-memory，跨重启重建是 documented residual）；`approval` 从 Console/Gate/Service 三层 collapse 成运行态 `Mode`（GetMode/SetMode）—— 不再阻塞，故三层 ISP 不再需要
- ✅ **重构 R1-R3**：dispatch 42 handler 样板 → `reply.go` 泛型尾部 helper（`decode`/`reply`/`replyDone`/`replyStream`）；chat 删零调用 `ContinuePlan`/`PlanDecision` + 抽 `finishTurn`；`translator.interrupt()` 拆 `approvalInterrupt`/`planInterrupt`（并修了 plan interrupt 在 wire 上误标 `Kind:"approval"` 的 bug）
- ✅ **run 流投递 userMessage Item**：用户输入作为 run 首个 Item（`item.started`+`item.completed`）随流走（前 wire id == items.list id），前端从「纯乐观渲染」升级为「乐观+按 id 去重」；删静默旁写的 `persistUserItem`
- ✅ **多 provider × 多 model**（跨 models/agent/lyra）：①`models/catalog` 公开包暴露 provider→models 枚举（internal/catalog 不动）；②agent 加 `core.ChatClientProvider` 扩展点（`collectExtensions[T]` 范式，process-scope 优先于 platform 默认 client）让一个 Platform 按 turn 换 model；③lyra `internal/service/provider` 运行态注册表（file/sqlite 持久化）+ `clientResolver`（**显式** (provider,model)→建/缓存 client，provider 不推断）+ `chat.StartTurnRequest.{Provider,Model}` 透传 + per-model `CatalogPricing`；④`providers.list/configure/test`（真实探活 max_tokens=1）+ `models.list`（catalog 直读，不门控）+ `runs.start{provider,model}` 配对必传接线；⑤baseURL 作为 provider 配置项（原生 WithBaseURL / 兼容 BaseURL 字段）。退役 `ProviderInfo` 单 provider 假设
- ✅ **per-session 工具工作目录**（跨 tools/lyra，不动 agent）：原本 engine 单一 `Workdir`，fs/bash 永远跑 serve cwd、无视 `Session.cwd`（多 project 在数据模型成立、工具层是假的）。①`tools/bash.LocalExecutor` 加 `Dir` 字段（fs 的 `Root` 类比；空=继承宿主 cwd）；②engine 工具集拆「cwd 无关（online/MCP/task，构造一次）」vs「cwd 相关（fs/bash，按解析重建）」；③`cwdToolResolver` 取代 static resolver —— `Tools(ctx)` 时从 `core.ProcessFrom(ctx).Blackboard()` 读 `cwdBindingKey`（缺则回退默认 workdir），同时服务 coding/subtask 两 role；④`RunChatRequest.Cwd`/`ChatInput.Cwd` 透传，chat action 用 **`BindProtected`** 种 cwd（过 typed-action `ClearBlackboard` + 随 `Blackboard.Spawn` 下传子进程）；⑤`chat.StartTurnRequest.Cwd` + `runs.start` 从 `Session.cwd` 解析透传。**复用** per-run model 的同款 seam（`ProcessFrom`/平台级 `ToolGroupResolver`），一个 engine 服务所有 session。subtask 的 cwd 继承已端到端验证（见下条 agent 修复）
- ✅ **agent 委派 spawn 语义修复**（用 lyra 倒逼出的 agent bug）：agent-as-tool（`AsChatTool`/`AsChatToolFromAgent`/`SubagentTools`/`AllAchievableTools`）原经 `SpawnChild` **全继承**父 blackboard → `GoalProducing[T]` 子 agent 被继承状态预满足、planner 不产 action → 子任务**静默不干活**（lyra `task` 命中）。新增 `runtime.SpawnChildProtectedOnly`（白板但保留 `BindProtected` 的 ambient/session 项，经 `Spawn()`+`Clear()`），是"全继承（SpawnChild）"与"全空白（SpawnChildFresh）"之间缺失的中间档,作委派默认。四个委派构造器全部改用它;`SpawnChild`(全继承)保留为公开 primitive,保住完整继承梯度。async 路径（`SpawnChildAsync`/`AsBackgroundChatTool`）仍全继承，无 lyra 消费方,未动。lyra `task` 子任务现在真跑且经 protected 继承 cwd（`TestEngine_RunChat_SubtaskInheritsCwd` 验证）
- ✅ **storage 统一 SQLite**（dev 减负）：删掉 session/interrupt/provider 的 in-memory + file 实现、history/process 的 file 实现、message 的 JSONL file 实现（新写 `sqlite.MessageStore` 替代）、冗余的 sqlite memory 表 —— 单一 SQLite 后端（`buildStores` 恒 sqlite，session/process/interrupt/history/provider/message 同一个 db）。去掉 `LYRA_STORAGE` 开关 + `config.StorageKind`。runtime 不再有 in-memory 回退（session/interrupt/provider 必须注入,测试用 `sqlite.Open(temp/:memory:)`）。**唯一保留为可编辑文件**:memory(LYRA.md)。顺带落地前端 B1 —— `Session.model`：domain 加 `Model` 字段 + sqlite `model` 列 + `Service.SetModel`;`runs.start{model}` 显式选则写回,`sessionToWire` 对未选过的会话回退 `Runtime.DefaultModel()`,wire 永远带真实 model 名
- ✅ **前端对接 B1 修复**：`Session.model` 恒空 → assistant 气泡名退化 "Assistant"。见上条 storage 统一（model 随 session 持久化 + 默认兜底）
- ✅ **per-session cwd 贯穿 prompt 层**（补上 per-session 工具工作目录的最后一块）：原本 system prompt 的项目 LYRA.md + AGENTS.md 级联锚定 serve 启动目录——工具跟会话走、prompt 不跟。现在 `SystemPrompt(ctx)` 经同一 seam（`turnCwd` 读 blackboard `cwdBindingKey`）取该 turn 的会话 cwd；`memory.Service` 接口的 project scope 改为按调用传 dir 寻址（`Get/Update(ctx, scope, dir)`、空 dir 回退默认）；extractor 经 `MaybeExtract(ctx, sessionID, cwd)` 把事实写进**该会话项目**的 LYRA.md（turnState 记 cwd）；wire 侧 memory.get/update/list 的既有 `cwd` 字段接通。加载顺序恒为**全局先、项目后**（user LYRA.md → project LYRA.md → AGENTS.md 级联，级联内部同样 ~/.lyra + ~/.agents 先、项目 root→leaf 后）
- ✅ **接入 Agent Skills**（`skills` + `tools/skills` 模块 → engine）：skill 工具按渐进披露(list/load/load_resource)暴露 SKILL.md skills。来源**合并**：项目 `<cwd>/.lyra/skills`（cwd 相关,**复用 per-session-cwd seam**——在 `buildSkillTool` 按解析重建,和 fs/bash 同类,coding+subtask 两 role 都给）+ 全局 `<LYRA_HOME>/skills`（`engine.Config.SkillsGlobalDir`,经 runtime.Config 从 `cmd/lyra` 的 `storage.Home()` 注入）,项目覆盖全局。两个目录都不存在时工具**整体缺席**（不是空列表,免 LLM 噪声）。skills 基础模块加了 `Merge(...Source)`（按 source 顺序定优先级）+ `FS.List` 容忍缺失目录。不执行脚本(model 用自己的 bash/fs 跑)
