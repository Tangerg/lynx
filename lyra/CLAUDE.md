# CLAUDE.md — project context for Claude Code (backend)

> **Lyra Runtime** — Go agent runtime. Implements the Lyra Runtime Protocol
> (JSON-RPC 2.0, MCP-inspired) consumed by the Wails / Web frontend at
> `/Users/tangerg/Desktop/lyra/`. 协议规范在前端仓 `docs/API.md` / `docs/TRANSPORT.md`，我们这边是它的服务端实现。

---

## 一句话定位

**协议层薄、业务层厚、传输层可换。** `rpc/` 是 wire 形态契约（JSON-RPC 2.0 / AG-UI events），`internal/engine` 是真正的"跑 chat turn + 工具循环"，`internal/service/*` 把业务能力按 domain 切片。Transport（HTTP+SSE / inprocess）只是 envelope I/O，对业务零感知。

## 技术栈

- **语言**: Go 1.26.3（go.mod / go.work 同步）
- **CLI**: `spf13/cobra`（每个子命令是 `App` 的方法 — `ServeCmd()` / `ChatCmd()` / `AgentsCmd()` / …）
- **协议 envelope**: `modelcontextprotocol/go-sdk/jsonrpc`（wire 类型借用，不自己重新定义 JSON-RPC 2.0）
- **HTTP transport**: stdlib `net/http` + Go 1.22 `ServeMux` 的 `POST /v1/rpc/{method...}` pattern。**不引 chi / gin / echo / fiber**（4 个 endpoint + 3 个 middleware，stdlib 完全够；fiber/echo 会把 SSE flush 搞砸）
- **SSE 写端**: `github.com/Tangerg/sse`（WHATWG §9.2 合规，auto-flush，多行 data 拆分）—— 这是自家库，发现问题反向推动迭代
- **MCP 客户端**: `modelcontextprotocol/go-sdk/mcp`（dial 外部 MCP server，工具合并进 engine 工具表）
- **AG-UI 事件**: `core/model/chat` + 自家 `internal/agui` 编码层
- **持久化**: 文件（默认，JSON / JSONL 平铺）或 SQLite（`modernc.org/sqlite` 纯 Go 无 CGO，`LYRA_STORAGE=sqlite` 启用）
- **LLM provider**: lynx adapters（`models/anthropic` / `models/openai`），`config.BuildChatClient` 按 `LYRA_PROVIDER` 分发
- **测试**: `go test`（无外部测试框架，stdlib `testing` + `httptest` 足够）

## 三大支柱

1. **`rpc/` —— 协议契约**
   - `rpc/protocol/` 接口（`Runtime` + 10 个子接口：`Lifecycle` / `Sessions` / `Messages` / `Runs` / …）+ wire 数据类型
   - `rpc/server/` 是 in-process 的 `Runtime` 实现，把协议方法映射到 `internal/service/*`
   - `rpc/dispatch/` JSON-RPC 方法路由（method 名 → Runtime 方法的查表派发）
   - `rpc/transport/` envelope I/O：`http/`（HTTP + SSE）/ `inprocess/`（同进程 chan）

2. **`internal/engine/` —— Chat 循环 + 工具编排**
   - 一个 facade，组合 `compactor` / `extractor` / `planner` / MCP sessions 三个子组件
   - 唯一暴露给 `chat.Service` 的契约是 `chat.Engine` 窄接口（5 方法）—— **chat 不直接依赖 `*engine.Engine`**
   - 系统提示拼装在 `engine/prompt.go`：base persona + `memory.Service` 两 scope + AGENTS.md 级联

3. **`internal/service/*` —— Domain 切片**
   - 每个 domain 一个目录：`session` / `memory` / `chat` / `approval` / `tool` / `agentdoc`
   - 每个目录都是 `service.go`（interface）+ `inmemory.go` 或 `engine.go`（具体实现）+ tests
   - 持久化实现放 `internal/storage/`（不混进 service 包）

## 持久化后端

`LYRA_STORAGE=file|sqlite`（默认 `file`）：

| 后端 | session | memory | message |
|---|---|---|---|
| `file` | `internal/storage/FileSessionService`（一个 sessions.json 原子重写） | `internal/storage/FileMemoryService`（`<cwd>/LYRA.md` + `~/.lyra/LYRA.md`，用户可直接编辑） | `internal/storage/FileMessageStore`（per-conversation JSONL append-only） |
| `sqlite` | `internal/storage/sqlite.NewSessionService(db)` | `internal/storage/sqlite.NewMemoryService(db)` | **仍是 file**（JSONL append-only 暂时没设计成 SQL schema） |

SQLite 模式的代价：**LYRA.md 不能直接编辑**，所有 memory.Update 都写 DB。换 backend 是 `cmd/lyra/app.go:buildSessionAndMemory(kind)` 一处 switch。

## 强约定（违反 = 回归）

- **协议形态写死 JSON-RPC 2.0**：所有 message envelope 走 MCP SDK 的 `jsonrpc` 类型，**不重新定义 envelope**。HTTP transport 上 method 名照搬协议表（`POST /v1/rpc/runtime.initialize` 这种，点保留不斜杠化）
- **业务 error → JSON-RPC `error.code`**（-32001..-32011 是我们的扩展段），**不映射 HTTP status**。HTTP status 仅反映 transport 层（200 / 204 / 400 / 401 / 404 / 409 / 500 / 503）
- **Sidecar 端点只 `/v1/info` + `/v1/health` 两个**，flat JSON 不走 envelope，no-auth。**永远不加业务 read shadow**（`GET /v1/sessions/{id}` 这种）
- **Transport 元数据走 header，不进 envelope**：trace id 用 `X-Lyra-Trace-Id`、本地 token 用 `Authorization: Bearer`、SSE 续连用 `Last-Event-Id`
- **依赖接口，不依赖具体类型**：跨包消费 service 必须经过 interface（`chat.Engine` / `approval.Gate` / `tool.ToolSource` / `agentdoc.*` 这些都是窄接口）。如果一个新模块要拿到 `*Platform` / `*Engine` 整体，先停下来想能不能拆成只用的几个方法
- **ISP 切碎接口**：典型例子 `approval.Console`（client side: ListPending / Decide / SetMode / GetMode）vs `approval.Gate`（producer side: Register / GetMode），`Service = Console + Gate`。消费者只 import 自己用的那侧
- **`errors.New` 优先于 `fmt.Errorf("constant")`**。`fmt.Errorf` 只在真要格式化时用，包装其他错误必须 `%w` 才能 `errors.Is/As`
- **没有 Java 味**：禁 `impl.go` 文件 / `Impl` / `Service` / `Manager` / `Helper` / `Handler` 这种空白后缀 / `GetX/SetX` getter / `NewBuilder().With().Build()` 链。文件名描述内容（`inmemory.go` / `engine.go` / `sqlite/session.go`），struct 名描述本质
- **现代 Go**：`atomic.Int32` / `atomic.Pointer[T]` / `sync.Map` 优先于自家 atomic wrapper。`slices.*` / `maps.*` helper 替代手写 loop
- **YAGNI / KISS / DRY / SOLID**：不为未来不存在的需求做抽象。看到 "M5 wires this" / "stub for later" / 注释比代码多两倍 —— 删
- **目前开发阶段，公开 API 可以调整**：不写 legacy 兼容代码、不写 migration、schema / exported type / 函数签名变了直接换；注释里不提"Legacy …"。**但任何破坏性公开 API 改动必须先咨询用户**（不只是大重构 —— 改一个 exported 函数签名 / 删一个 exported 类型 / 改 struct field 也算），列清楚 scope + 影响面 + 备选方案，等用户确认再动手。这条规则适用于本仓库所有 sub-module（agent / core / models / vectorstores / tools / ...）
- **加文档？先问** —— `lyra/CLAUDE.md` / `lyra/doc/ARCHITECTURE.md` / `lyra/doc/ROADMAP.md` 已经存在；其他默认不写

## 重构策略（参考前端 CLAUDE.md，Go 化）

**重构是节奏，不是可选项 —— 分两档**：

- **小型重构（每 3-5 轮 feature）**：聚焦最近改动的那几个文件。扫一遍：
  - 单文件超 300 行没？
  - 局部 3+ 重复 pattern？
  - 最近加的注释里有 what-说明可删？
  - 最近 rename 漂移没（两个名字指同一个东西）？
  - 最近新增的 exported API 破坏既有抽象没？
  - **产出**：抽 1-2 个 helper / 删几条死注释 / 精修 1-2 个名字 / 改 1 个文件的字段分区 —— **净变化 < 100 LOC、touch < 5 文件**。
  - **不需要咨询用户**（除非碰到破坏性公开 API），直接做完跑 `go build && go vet && go test ./...` 全绿 commit

- **大型重构（每 15-20 轮 feature）**：跨整个模块扫，参考本 session 做过的 lyra A-E / agent A-D 范例。
  - 跑 `go vet ./...` + `staticcheck ./...`（如装了）找未引用的 exports / dead branches
  - 找 > 500 行的文件考虑拆 SRP
  - 找跨包的 3+ 重复（不只是局部）—— typical 例子：MCP enum 双定义、`fmt.Errorf("constant")` 散落
  - 找 god struct（field > 8 个 + method > 10 个）考虑组合化
  - 找具体类型跨包暴露 —— 是不是应该收窄成 interface（参考 `chat.Engine` / `approval.Gate` / `autonomy.platform`）
  - 考虑是否要拆 / 合 package
  - **产出**：multi-batch 重构计划（A/B/C/D/E），用户确认后逐批 commit；每批之间跑 `go build && go vet && go test ./...` 全绿

- **共同做法**（这次 lyra / agent 重构 session 验证过）：
  - 上来**先深度审计**（grep / Explore agent / 读文件），不要直接动手
  - 把发现分类（Java-isms / coupling / cohesion / SOLID / DRY / 命名 / 现代 Go），按 impact 排序
  - 给 3-5 项候选 batch + 每项的"动 vs 不动"权衡
  - **等用户确认再动**，每批一个 commit、可独立 revert
  - 重构跑完**承认 audit 过度 call 的项**（我们这次 lyra C + E、agent C + E 都最终 skip 了，因为深入看发现 audit 误判）—— 这是正常 false positive，不是失败
  - 每批 commit message 写清"why"，把 audit 的发现 + skip 理由都记下来

- **目的**：小型重构防局部熵增（每个文件不至于失控），大型重构防架构熵增（整体不至于尾大不掉）

- **触发信号**（任何一项命中就该考虑）：
  - 单文件 > 500 行（god file）
  - 同 struct field > 8 + method > 10（god object）
  - 一个 type 的方法被多个包消费、但消费方各自只用 2-3 个（→ 该收窄成 interface）
  - `addXxx / removeXxx` 或 `getXxx / setXxx` 模式 > 3 处未抽象
  - 命名漂移（两个名字指同一个东西，或一个名字指两个东西）
  - 最近 commit 里有反复改同一段代码（说明抽象方向错了）
  - 加新 feature 需要改多个文件的同一类样板代码（说明缺一层抽象）
  - `// TODO: M5 wires this` / `// stub for later` 这种推测性占位（YAGNI 信号）

**重构清单**（Fowler《重构》的 Go 实践版，每轮扫的不止拆分还包含）：

- **(a) 死代码清理** —— 跑 `go vet` + `staticcheck`（如装了）+ 全文 grep 一遍 exported 符号的调用方。零调用方 = 删，**不留"将来可能用"**（本次 `ServiceProvider`、`Platform()` getter 都是这条命中删的）
- **(b) 卫语句替代嵌套 if** —— `if !ok { return }` 比层层缩进的 if/else 链可读 10 倍
- **(c) 查表法替代条件链** —— 3+ `switch case` 或嵌套 `if/else if` 通常应该是 `map[K]V` 查表（除非用 generic 类型分发，如 `collectExtensions[T]`）
- **(d) 精准命名** —— `idCounter` → `nextCompositeKeyId`；`x` / `tmp` / `data` / `result` / `obj` 不算名字；文件名 `impl.go` 是 Java 味（→ `inmemory.go` / `engine.go` / `sqlite.go`）
- **(e) 注释清理** —— 大段解释 what 的删（代码自身说明）；过期的迁移注释删（"Legacy …" 这种）；误导性的"为什么这样写"（实际不再这样了）删。留下来的只解释 _why_ 而不是 _what_。`// M5 wires this` 这种推测性占位删
- **(f) 现代 Go 扫描** —— `sync/atomic` 用 `atomic.Int32` 不用手写 wrapper / `sync.Map` 替代 `mutex + map`（适合 write-rare）/ `slices.*` `maps.*` helper 替代手写 loop / `iter.Seq2` 替代 channel-based 流 / `errors.New("...")` 替代 `fmt.Errorf("constant")` / `%w` 包装错误
- **(g) 接口收窄扫描** —— 跨包传递的具体类型（`*Engine` / `*Platform` 等），看消费方实际只用几个方法 —— 能收窄成 interface 就收窄，附带加 compile-time tripwire 测试（参考 `autonomy/platform_iface_test.go`）
- **(h) 性能扫描** —— 热路径上 `sync.RWMutex` vs `sync.Map` 是否合适？循环里有 N² `slices.Index` / `Contains`？大 struct 是 copy 还是 pointer 传递？SSE / stream 路径有没有 buffering 把 flush 搞砸？

## 强反向不变量（已知错的方向）

- ❌ **Stdio transport**（CLI 给 LLM 用那种）：协议层有意不实现，见前端 docs/API.md §1.1。Web 走 HTTP loopback、TUI 走 inprocess，没人需要 stdio
- ❌ **后端做用户鉴权 / 账号 / 订阅 / 多租户**：Runtime 协议层零 user 概念，鉴权由更外层（OS 信任、本地进程门禁 token、未来 facade）解决
- ❌ **给 LLM provider 加 OAuth / token refresh**：用户填 API key、Lyra 存进程内存或 keychain，provider 401 让 UI 提示重填。OAuth 是 Claude Code 复杂度
- ❌ **业务方法的 RESTy read-only shadow**：业务调用一律走 `POST /v1/rpc/{method}`。诱惑出现就驳回（详见前端 docs/API.md §9.3）
- ❌ **HTTP transport 换 chi / gin / echo / fiber**：4 个 endpoint + 3 个 middleware 用不上路由框架；fiber/echo 把 SSE 的 buffer/flush 搞砸过。stdlib `ServeMux` 1.22+ `{method...}` pattern 已足
- ❌ **SSE 写自己的 frame 编码**：用 `github.com/Tangerg/sse`（auto-flush + spec compliance）。手写 `fmt.Fprintf(w, "data: %s\n\n", body)` 在 body 含 `\n` 时会破坏帧
- ❌ **加 retry layer**：SDK 内部已有 retry 就够，不在 `pkg/retry` 引入 Transient/NonTransient 分类
- ❌ **structured output 自己开 converter 链**：`chat.JSONParser[T]` / `ListParser` / `MapParser` 已覆盖 spring-ai converter family，Reasoning 是 first-class
- ❌ **DefaultOptions 返回 `*Options` 指针**：必须返值（intentional immutability），别提改指针
- ❌ **手写 `fmt.Errorf("xxx is nil")`**：换 `errors.New`。包装 err 时一律 `%w`，没有 `%v` for errors
- ❌ **新增模块直接 import `*engine.Engine` / `*runtime.Platform` 整体**：先在自己包里定义窄接口（`Engine` / `Platform`），具体类型隐式满足。这条规则我们已经踩过 3 次坑（chat / tool / autonomy 都做过）
- ❌ **接口里塞所有 method**：subscriber 只用 3 个，producer 用另外 2 个 —— 拆 ISP（approval.Console / Gate 是范例）
- ❌ **复制公共类型**（典型：MCP enum config 一份 engine 一份）：留一份，import 一下
- ❌ **trace stub interface**（M7 待实现那种 placeholder）：真要做时再定义；当前删掉
- ❌ **`/v1/rpc` 不带 method**（裸路径）：v4 协议 greenfield 决议，单一形态 `POST /v1/rpc/{method}`，裸路径 404
- ❌ **协议 envelope 装 transport 元数据**（session id / auth token / trace id / idempotency key）：走 Go `context.Context` 或 HTTP header，永不进 message body

## Go idiom 纪律（写代码 / review 时必看）

近期重构沉淀的硬规则：

- **错误构造**：`fmt.Errorf("constant string")` 是浪费 → `errors.New("...")`。错误包装一律 `%w`，没有 `%v`
- **接口在消费方定义**：`chat.Engine` 定义在 `internal/service/chat/engine.go`，`*engine.Engine` 隐式满足。被消费的具体类型**不主动暴露接口**给消费者 import —— 消费者自己写
- **测试用 stub 接口而不是真实 platform**：`internal/service/chat/engine_test.go` 的 `stubEngine` 就是范例；`internal/service/autonomy/platform_iface_test.go` 是 compile-time tripwire（如果 `platform` interface 偷偷长大，stub 就编译失败）
- **`impl.go` 是 Java 味**：实现文件按本质命名 —— `inmemory.go`（单进程内存实现）/ `engine.go`（engine-backed）/ `sqlite/session.go`（特定 backend）
- **`atomic.Int32` 直接用**，别再包一层 `atomicXxx` wrapper（`Store(int32(v))` / `Load()` 就够）
- **结构体字段超过 6 个**：用注释 `// --- xxx ---` 分区（per-process state / platform-wired hooks / per-action state 这种）。读起来一目了然
- **跨包用 generic 类型分发**：`runtime.collectExtensions[T any]([]Extension)` 是这个 codebase 的核心 pattern。需要类型路由的场景优先用 generic，不用 type switch
- **dead code 立刻删**：发现 `Platform()` 这种零调用方的 exported getter —— 删。哪天真需要时再加，那时已经知道签名该长什么样
- **`sync.Map` 适合 write-once / read-many**：`FileMessageStore` 的 per-conversation lock 是典型场景。不适合 write-heavy
- **不要测一个具体类型有没有实现接口**：编译期断言 `var _ Service = (*inMemory)(nil)` 比运行时检查好

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
go test ./<pkg>/... -v   # 单包 verbose

# 启动 server（dev）
ANTHROPIC_API_KEY=xxx ./lyra serve                       # 默认 127.0.0.1:17171（匹配 FE AGUI_BASE）
LYRA_STORAGE=sqlite ANTHROPIC_API_KEY=xxx ./lyra serve   # 切 SQLite

# 看本会话能读到哪些 AGENTS.md
./lyra agents             # 列表
./lyra agents --show      # 带内容

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
- ✅ `fmt.Errorf("constant")` 全部改 `errors.New`（58 处）
- ✅ autonomy 解耦 `*runtime.Platform` —— 改用包内 `platform` 窄接口 + compile-time tripwire

## 沟通约定

- **中文回复**（用户偏好）
- 代码 / 注释保持英文
- 大重构前先给批次方案 + 权衡，等用户确认再动；每批一个 commit，可独立 revert
- **公开 API 改动前先咨询用户**（exported 函数 / 类型 / 字段签名变化，跨包消费者会受影响）—— dev 阶段允许改，但不允许"擅自"改。列出 scope + 影响面 + 备选方案，等"动"再动
- 改动后跑 `go build && go vet && go test ./...` 全绿才 commit
- commit message 写清"why"而不仅是"what"
- commit trailer 用 `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`
