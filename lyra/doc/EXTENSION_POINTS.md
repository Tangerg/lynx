# EXTENSION_POINTS.md — Lyra Runtime 的「kernel 不长肉」扩展点底座

> **目标**：把前端那套「Kernel 不长肉，所有能力都是插件」搬到 Lyra Runtime。
> 后端 kernel 也退化成 **纯派发器 + 状态容器 + 扩展底座**，零功能代码；
> 连 agent loop 本身都是插件——正如前端把协议 fold（`core-reducer`）做成插件、
> reducer 只是派发器一样。
>
> 镜像文档：前端 `/Users/tangerg/Desktop/lyra/docs/EXTENSION_POINTS.md`。
> 协议契约：前端仓 `docs/API.md` / `docs/TRANSPORT.md`。本文件是 **设计与迁移蓝图**，
> 不是已落地现状——落地路径见 §14。

---

## 1 · 一句话定位

**Kernel 只提供「插件」这套机制 + 跑 run 的最小骨架；tool / provider / agent loop /
RPC 方法 / 检索 / 中间件全是贡献进来的插件。** 加一个能力 = 加一个插件包 + 在
composition root 列表里加一行；加一类扩展面 = 一个 `DefinePoint` + 一个 `Lookup` 助手。
**kernel 包本身一行不改。**

现状（要反转的）：`internal/engine` 把 chat 循环 / 工具编排 / provider 解析 **硬接**在一起，
`internal/service/*` 按 domain 切片但仍由 engine 显式组合。目标是把 engine 收缩成 kernel 底座，
loop / tools / providers / methods 全部变成 `Contribute` 进来的插件。

---

## 2 · Kernel 不可约的最小内核

只有这 5 样不能是插件——其余全是插件：

| 留在 kernel | 为什么不能是插件 |
|---|---|
| **扩展底座**（`DefinePoint` / `Contribute` / `Lookup`） | 它就是「插件」这套机制本身 |
| **插件 host + 加载器 + 生命周期 / Dispose** | 加载插件的东西不能由插件加载 |
| **transport + JSON-RPC envelope I/O**（`rpc/transport/*`） | 线本身；但 method **路由表**是贡献的（见 `PointRpcMethod`） |
| **event sink**（把 `StreamEvent` 落到 transport 的 per-run hub，`rpc/server/hub.go`） | 插件调 `host.Emit(ev)`，kernel 负责序列化 + 投递 |
| **run / session 容器**（哪些 run 在跑、能否取消、history 记账） | 仅生命周期记账；**语义是插件**（对应前端 store 由 kernel 持有、写 store 的 fold 是插件） |

> 判据：**「跑 run 这件事该怎么跑」kernel 不知道**——它只持有 run 的生命周期，把语义交给
> 贡献进来的 `RunStrategy`。和前端 reducer 不懂协议语义、全交给 `core-reducer` 同构。

---

## 3 · 扩展底座 API（Go 形态）

```go
// ── kernel/ext：底座（纯机制，零功能） ──

// Point 是一个 typed 贡献面的句柄。
type Point[T any] struct{ name string }

func DefinePoint[T any](name string) Point[T] { return Point[T]{name: name} }

// 唯一写路径。Go 方法不能带类型参数，所以 Contribute/Lookup 是收 host 的自由函数
// （而非 host.Contribute(...)）——这是与前端 `host.extensions.contribute` 唯一的形态差异。
func Contribute[T any](h Host, p Point[T], item T) { h.registry().add(p.name, item) }

// 读路径。底座内部存 map[string][]any，Lookup 做断言（= 前端单一 `extensions` 底座）。
func Lookup[T any](h Host, p Point[T]) []T { return assertAll[T](h.registry().get(p.name)) }

// 带 key 的查找（如按 tool name / provider id 定位单条）。
func LookupByKey[T Keyed](h Host, p Point[T], key string) (T, bool) { ... }
```

```go
// ── Host：唯一传给插件的对象。插件之间永不互相 import，一切走 host + Point。 ──
type Host interface {
    Emit(ev protocol.StreamEvent)        // run.* / item.* / custom 都从这出
    Config() Config                      // 只读配置
    Storage() Storage                    // 注入的 KV/SQLite 抽象（插件不自建全局态）
    Secrets() Secrets                    // provider key 等，受能力门禁
    Logger() *slog.Logger
    Tracer() trace.Tracer
    registry() *registry                 // 内部；Contribute/Lookup 用
}
```

```go
// ── Plugin：贡献的单位 ──
type Plugin struct {
    Name    string
    Version string
    // Setup 在加载期（启动单线程）调用，在这里 Contribute(...)。
    // 返回 Disposable 交给 host 统一回收——插件不自己管 dispose。
    Setup   func(h Host) (Disposable, error)
}

type Disposable interface{ Dispose() error }
```

**规矩（机械）**：
- 插件**只在 `Setup` 里 `Contribute`**；运行期不再写 registry（见 §11 冻结）。
- 插件**永不 import 另一个插件包**去直接用——永远走 `Lookup`（= 前端「Plugin 一定走 registry」）。
- 副作用 / goroutine / 连接由 `Setup` 返回的 `Disposable` 收，**不手动 dispose**。

---

## 4 · 内置扩展点清单

kernel 预定义这一把 Point（`kernel/points.go`，对应前端 `kernelPoints.ts`）：

```go
var (
    // 「跑一次 run 怎么跑」——核心插件 core-agent-loop 贡献它（可换 ReAct / plan-exec / single-shot）
    PointRunStrategy  = DefinePoint[RunStrategy]("run.strategy")

    // 能力面
    PointTool         = DefinePoint[Tool]("tool")              // 每个工具 / 工具族
    PointProvider     = DefinePoint[Provider]("provider")      // 每个 LLM 适配（models/*）
    PointRunMiddleware = DefinePoint[Middleware]("run.middleware") // pre-llm / pre-tool / post-tool 钩子
    PointInterruptPolicy = DefinePoint[InterruptPolicy]("interrupt.policy") // HITL 审批策略
    PointRetriever    = DefinePoint[Retriever]("retriever")    // RAG
    PointMemory       = DefinePoint[MemoryBackend]("memory")
    PointVectorStore  = DefinePoint[VectorStore]("vectorstore")
    PointDocReader    = DefinePoint[DocReader]("docreader")

    // 协议面
    PointRpcMethod    = DefinePoint[Method]("rpc.method")      // runs.start / items.list / ... 每个方法都是贡献的 handler
    PointCapability   = DefinePoint[Capability]("capability")  // /v2/info 的 feature flag，谁加载谁点亮
)
```

加一类扩展面 = 这里加一个 `DefinePoint` + 写一个 `Lookup` 助手，**不碰底座、不碰 host**。

---

## 5 · 核心洞察：agent loop 本身是插件

`runs.start` 这个 **method 也是插件**（贡献到 `PointRpcMethod`）。它被调用时：

```
kernel transport 收到 POST /v2/rpc/runs.start
  → kernel 在 PointRpcMethod 查到 "runs.start" 的 handler（插件）
  → 该 handler 建一个 run（kernel 的 run 容器记账）
  → 从 PointRunStrategy 取出策略，把 RunContext 交给它驱动
  → strategy 用 RunContext 调 provider / tools，并 host.Emit(...) 出 run.* / item.*
```

```go
// kernel 定义接口；core-agent-loop 插件实现并贡献。
type RunStrategy interface {
    Run(ctx context.Context, rc RunContext) error
}

// RunContext 由 kernel 注入——语义插件用它做事，但不碰 kernel 内部。
type RunContext interface {
    SessionID() protocol.SessionID
    Input() []protocol.ContentBlock
    Tools() []Tool                       // 已 Lookup + middleware 包好
    Provider() Provider                  // 该 run 解析出的 provider/model
    Emit(ev protocol.StreamEvent)        // run.started / item.* / run.finished 从这出
    Interrupt(req InterruptRequest) (InterruptResponse, error) // HITL：park 当前 run
}
```

**kernel 自己不知道「一次 run 该怎么跑」**——ReAct、plan-execute、single-shot 都是不同的
`RunStrategy` 插件。这正是「core-reducer 是插件、reducer 只派发」在后端的对偶。

迁移上：**现有 `internal/engine` 的 chat 循环原样包成 `core-agent-loop` 插件**（先不改逻辑，
只搬位置），即得到第一个 `RunStrategy` 贡献。

---

## 6 · RPC method 作为插件 + 能力派生

每个 method 是一条贡献：

```go
type Method struct {
    Name     string                                 // "runs.start" / "items.list" / "providers.configure"
    Stream   bool                                   // streamable HTTP？（响应体即 event-stream）
    Handle   func(ctx context.Context, rc MethodContext) error
}
```

`/v2/info` 的 `capabilities.features` **不在 kernel 硬编码**，而是聚合 `PointCapability` 的贡献：

```go
// MCP 插件加载 → 贡献 Capability{Feature:"mcp", Enabled:true}
// memory 插件加载 → 贡献 Capability{Feature:"memory", Enabled:true}
// kernel 的 info method 只是把所有 PointCapability 贡献 reduce 成 features map。
```

谁加载谁点亮——和前端「features 由贡献派生」同构。删掉一个插件，对应能力自动从握手消失，
**无需改 info 代码**。

---

## 7 · 信任分级与第三方插件

镜像前端 `capabilities.ts`（能力分级）+ `pluginOrigin.ts`（sideload 默认 deny）：

| 档 | 机制 | 信任 | 加一个 |
|---|---|---|---|
| **Tier 1 · 内置** | compile-time Go 包，在 composition root 注册 | 第一方，全权限 | 加包 + 列表一行（= 前端 `plugins/builtin/`） |
| **Tier 2 · 第三方** | **out-of-process 沙箱** | **默认 deny + 能力分级** | 装一个进程，运行期发现 |

Tier 2 的承载：
- **工具走 MCP**（`mcp/` 已是完整实现，server/tool/provider/transport/sampling/reverse）——已经是一等公民。
- 更广的扩展面用 **Lyra-native 插件协议**（同 JSON-RPC envelope 哲学）over stdio/socket，
  或 **WASM（wazero）** 做进程内沙箱。
- **绝不用 Go `plugin`（.so）**：版本锁死、无 Windows、极脆。

能力门禁：第三方插件声明所需能力（`fs.read` / `net` / `secret:<id>` / `exec`），host 在
`Setup` 注入受限的 `Storage`/`Secrets`/`net` 句柄；未授权能力即缺失，**不是运行期抛错**。

---

## 8 · 多前端协调：后端插件是「纯语义 Model」

Lyra Runtime 要喂 **GUI（Wails/Web）+ TUI + headless** 多前端。所以：

> **后端插件只产语义、带类型的负载，永不内嵌「某个前端」，也不分支判断连上来的是谁。**

把 MVC 拆到进程边界上：

```
后端插件 = Model（纯语义）            协议 = 传输 Model 的线
每个前端 = View（GUI / TUI / …）      各自把同一份 Model 映射到原生呈现 + 通用兜底
```

- 插件产语义：typed tool result、`custom` 事件 `{...}`、命令描述符、配置 schema。
- 一个「功能」的 Pack = `{ 后端 Model 组件 }` + `{ 可选的 per-frontend view-adapter }`：

| 前端 | view-adapter 怎么来 |
|---|---|
| GUI（可动态加载） | runtime serve JS bundle → 前端 sideload（前端 `plugins/host/sideload.ts` 已从 runtime base URL 拉 manifest） |
| TUI（静态 Go 二进制） | **编译期注册**到 TUI 自己的 renderer 表（= GUI 的 builtin 那一档） |
| headless | 不要 adapter，直接吃语义负载 |

- **双向能力协商**：客户端上报 kind + 支持的 surface；runtime 据此**只投递匹配的 view-adapter**
  并决定降级——**但这结果不给插件拿去分支**（插件永远只发一份语义）。
- **硬规则**：每个语义类型必须有「通用兜底渲染」，否则多前端必有白屏。

详见前端镜像文档同名章节。

---

## 9 · 并发与冻结

- run 是并发的（`maxConcurrentRuns`）。底座契约：**启动时单线程把所有 `Setup` 跑完、注册完毕，
  随后 freeze registry**；run 期间只对 registry 做只读 `Lookup` → **无锁**。
- 这条等价于前端「插件 load 完即定型」。运行期严禁 `Contribute`（freeze 后 panic / 拒绝）。

---

## 10 · 与前端不对称处（老实说）

1. **Go 方法不能带类型参数** → `Contribute`/`Lookup` 是收 `host` 的自由函数，不能 `host.Contribute(...)`。
2. **Go 无运行时动态链接** → 「kernel 不长肉」= **kernel 包零功能代码、builtin 是独立包在 composition root 注册**；动态第三方一律 MCP/WASM/子进程。
3. **run loop 是 active long-lived driver，不是纯 reducer** → `PointRunStrategy` 存的是会驱动控制流的策略，不是被动 spec。这是与前端「多数贡献是被动 spec」最大的结构差异，但模式仍成立：贡献 strategy、kernel 调它。
4. **持久化** → 插件态走注入的 `Storage`，**不自建全局**；遵守 runtime 无状态 + 零 user（见 §13）。

---

## 11 · FE ↔ BE 对称表

| 前端 | 后端 |
|---|---|
| Kernel = Slot + Zustand store | Kernel = run/session 容器 + transport + event sink |
| `defineExtensionPoint` / `host.extensions.contribute` | `DefinePoint[T]` / `Contribute[T](host, p, item)` |
| `useExtensionPoint` / `lookupExtensionByKey` | `Lookup[T]` / `LookupByKey[T]` |
| `definePlugin({ setup({host}) })` | `Plugin{ Setup(host) }` |
| **core-reducer 插件**（StreamEvent fold） | **core-agent-loop 插件**（RunStrategy） |
| reducer = 纯派发器 | transport dispatcher = 纯派发器（路由 method + 调 strategy） |
| content-block / command / theme 贡献 | tool / provider / retriever 贡献 |
| `events.onStream` 等薄 facade | `host.Emit` + `PointRunMiddleware` 钩子 |
| features 由贡献派生 | `/v2/info` capabilities 由 `PointCapability` 派生 |
| Disposable 由 host 收 | `Disposable` 由 host 收（defer 回收） |
| builtin 编译期 / sideload 运行期 | builtin 编译期 / 第三方 out-of-process（MCP/WASM） |
| capabilities 风险分级 + sideload 默认 deny | 能力门禁 + 沙箱 + 默认 deny |

---

## 12 · 落地路径（渐进，每步独立跑绿）

和前端那次「40 个命名 map 塌进单一 `extensions` 底座」同一打法：

1. **抽底座**：`kernel/ext`（`Point` / `Contribute` / `Lookup` / `Host` / `Plugin` / `Disposable`），纯机制无功能。
2. **包 loop**：现有 `internal/engine` 的 chat 循环原样包成 `core-agent-loop` 插件，贡献 `RunStrategy`；kernel 改成「查 strategy 并调用」。逻辑不动，只搬位置，验证骨架。
3. **抽 tools / providers**：从 engine 硬接里抽成 `PointTool` / `PointProvider` 贡献；`internal/service/tool`、`internal/service/provider` 退为这些贡献的载体。
4. **method 表插件化**：`rpc/dispatch` 的方法路由改成 `PointRpcMethod` 贡献；`/v2/info` 改由 `PointCapability` 派生。
5. **middleware / interrupt / retriever / memory** 逐步搬（`internal/service/*` 各 domain → 各自的 Point 贡献）。

每步跑全套测试绿再 commit；commit message 写清「why」。

---

## 13 · 强反向不变量（别做的方向）

- ❌ **kernel 里出现任何具体 tool / provider / loop 逻辑**——一旦出现就是「长肉」，回退。
- ❌ **插件互相直接 import 使用**——永远走 `Lookup`。
- ❌ **后端插件分支判断前端 kind**（`if frontend == gui`）——Model 不得耦合 View（见 §8）。
- ❌ **插件自建全局态 / 持久化 / user 概念**——状态走注入的 `Storage` + run/session `context`；
  auth / billing / 多租户**永远在未来 facade**，不进插件、不进协议（项目级 §6.2）。
- ❌ **transport 元数据**（session id / auth / trace）进插件 message body——走 `context` / header。
- ❌ **插件逼核心协议 envelope 膨胀**——只走 `custom` / 通用 `tool` / 自定义 content block 这几条开放车道。
- ❌ **用 Go `plugin`（.so）做动态加载**——MCP / WASM / 子进程。
- ❌ **运行期 `Contribute`**——freeze 后只读（见 §9）。
