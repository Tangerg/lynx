# CLAUDE.md — lyra module

> **Lyra Runtime** — Go agent runtime backend. 实现 Lyra Runtime Protocol
> （JSON-RPC 2.0, MCP-inspired）给 Wails / Web frontend 用（前端在 `/Users/tangerg/Desktop/lyra/`）。协议规范在前端仓 `docs/API.md` / `docs/TRANSPORT.md`。
>
> 项目级约定（设计原则 / 重构策略 / Go idiom / 共用反向不变量 / 沟通约定）见 `../CLAUDE.md`。本文件只放 lyra 模块特有内容。

---

## 一句话定位

**协议层薄、业务层厚、传输层可换。** `rpc/` 是 wire 形态契约（JSON-RPC 2.0 / AG-UI events），`internal/engine` 是真正的"跑 chat turn + 工具循环"，`internal/service/*` 把业务能力按 domain 切片。Transport（HTTP+SSE / inprocess）只是 envelope I/O，对业务零感知。

## 技术栈

- Go 1.26.3
- **CLI**: `spf13/cobra`（每个子命令是 `App` 的方法 —— `ServeCmd()` / `ChatCmd()` / `AgentsCmd()` / …）
- **协议 envelope**: `modelcontextprotocol/go-sdk/jsonrpc`（wire 类型借用，不自己重新定义）
- **HTTP transport**: stdlib `net/http` + `go-chi/chi` v5 路由（`r.Use` 中间件链 + `POST /v2/rpc/{method}`）+ `go-chi/cors`（CORS 中间件，替掉手写 cors）。**streamable HTTP**：流式方法（`runs.start/resume/subscribe`、`background.subscribe`）的 POST 响应体本身就是 `text/event-stream` 事件流（首帧=JSON-RPC 响应，无 SSE id；其后 `notifications.run.event` 帧带 SSE id=eventId）；**无独立 `/v2/rpc/stream` 端点、无 `X-Conn-Id` 连接路由**。事件源是 server 侧的 per-run hub（`rpc/server/hub.go`），transport 只是把 hub 订阅抽成 SSE
- **SSE 写端**: `github.com/Tangerg/sse`（WHATWG §9.2 合规，auto-flush）—— 自家库
- **MCP 客户端**: `modelcontextprotocol/go-sdk/mcp`
- **AG-UI 事件**: `core/model/chat` + 自家 `internal/agui` 编码层
- **持久化**: 文件（默认 JSON / JSONL）或 SQLite（`modernc.org/sqlite` 纯 Go 无 CGO，`LYRA_STORAGE=sqlite`）
- **LLM provider**: 多 provider × 多 model。`internal/service/provider` 是运行态可变注册表（每 provider 的 key+baseURL，file/sqlite 持久化）；`config.BuildClient(ClientSpec)` 按 provider id 建 client（anthropic/openai/moonshot/deepseek，后两者走 OpenAI 兼容端点，全部支持 baseURL 覆盖）。**per-run model**：`runs.start{provider, model}`(显式配对,缺一即 `invalid_params`、都缺用默认 —— provider **不从 model 推断**)→ `clientResolver` 取该 provider 的注册表凭证建/缓存 client → 经 agent `core.ChatClientProvider` 扩展点让该 turn 用它。model 元数据/定价/能力全来自公开的 `models/catalog`（models.list 直读，无需 key）。`config.yaml` 的 `provider`/`apiKey`/`baseURL` 是默认 provider 的种子
- **测试**: stdlib `testing` + `httptest`

## 三大支柱

1. **`rpc/` —— 协议契约**
   - `rpc/protocol/` 接口（`Runtime` + 12 个子接口）+ wire 数据类型 + v2 协议错误码 / Item / RunEvent 形状
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

- **协议形态写死 JSON-RPC 2.0**：所有 message envelope 走 MCP SDK 的 `jsonrpc` 类型，**不重新定义 envelope**。HTTP transport 上 method 名照搬协议表（`POST /v2/rpc/runtime.initialize`，点保留不斜杠化）
- **业务 error → JSON-RPC `error.code`**（`-32001..-32016` 是扩展段，客户端按 `error.data.type` 的 symbolic name 分支、不按数字码），**不映射 HTTP status**。HTTP status 仅反映 transport 层（TRANSPORT §6.3：200 / 204 通知 / 400 含 method 不一致 / 401 带 `WWW-Authenticate` / 404 / 405 带 `Allow` / 413 / 415 / 500 / 503；**无 409** —— 自相矛盾请求归 400 而非资源冲突）
- **Sidecar 端点只 `/v2/info` + `/v2/health` 两个**，flat JSON 不走 envelope，no-auth。**永远不加业务 read shadow**（如 `GET /v2/sessions/{id}`）
- **Transport 元数据走 header，不进 envelope**：trace id 用 `X-Trace-Id`（已去品牌前缀）、本地 token 用 `Authorization: Bearer`、SSE 续连用 `Last-Event-Id`、协议版本 `X-Protocol-Version`、连接 id `X-Conn-Id`、幂等键 `X-Idempotency-Key`

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
│   │   ├── approval/               运行态审批 stance（`Mode` + GetMode/SetMode）—— R 模型工具审批查它决定 pass/deny/prompt
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
curl http://127.0.0.1:17171/v2/info | jq                 # sidecar（no-auth）
curl -H "Authorization: Bearer $(cat ~/.lyra/local-token)" \
  -X POST -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":"1","method":"runtime.initialize","params":{}}' \
  http://127.0.0.1:17171/v2/rpc/runtime.initialize
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
- ✅ **agent 委派 spawn 语义修复**（用 lyra 倒逼出的 agent bug）：agent-as-tool（`AsChatTool`/`AsChatToolFromAgent`/`SubagentTools`/`AllAchievableTools`）原经 `SpawnChild` **全继承**父 blackboard → `GoalProducing[T]` 子 agent 被继承状态预满足、planner 不产 action → 子任务**静默不干活**（lyra `task` 命中）。新增 `runtime.SpawnChildFreshProtected`（白板但保留 `BindProtected` 的 ambient/session 项，经 `Spawn()`+`Clear()`），是"全继承（SpawnChild）"与"全空白（SpawnChildFresh）"之间缺失的中间档,作委派默认。四个委派构造器全部改用它;`SpawnChild` 零调用 → 删（doc 锚移到 `SpawnChildFreshProtected`）。async 路径（`SpawnChildAsync`/`AsBackgroundChatTool`）仍全继承，无 lyra 消费方,未动。lyra `task` 子任务现在真跑且经 protected 继承 cwd（`TestEngine_RunChat_SubtaskInheritsCwd` 验证）
