# Greenfield 设计 —— 如果从零写 lyra 模块

> **日期**:2026-06-18。**视角**:架构师命题作文 —— "假设从零开始写 lyra,系统架构与文件组织怎么设计?"
> **状态**:**greenfield 重审(非新基准)**。与 [`GREENFIELD_ARCHITECTURE.md`](GREENFIELD_ARCHITECTURE.md)(模块自己的架构基准)的关系:那份是 2026-06-14 落地的"从零应然"基准(已执行);本文是 2026-06-18 的 greenfield 重审,吸收了 [`ARCHITECTURE_REVIEW.md`](ARCHITECTURE_REVIEW.md)(现状体检)的发现 + 跨模块接缝视角。两份互补不取代 —— GREENFIELD_ARCHITECTURE 是基准,本文是重审 + 跨模块。
> **方法**:第一性推导 + 对照 [`../../../DESIGN_PHILOSOPHY.md`](../../../DESIGN_PHILOSOPHY.md)(§2.3/§2.4/§2.5)+ [`../../../REFACTORING.md`](../../../REFACTORING.md)(§5.1/§8)+ [`../CLAUDE.md`](../CLAUDE.md)。与 agent 侧 greenfield 设计配套见 [`../../../agent/docs/GREENFIELD_DESIGN.md`](../../../agent/docs/GREENFIELD_DESIGN.md)。
>
> **结论先行**:
> 1. **lyra 是应用,agent 是库 —— 二者形状不同,接缝必须干净。** lyra 用 Clean Arch 同心环(delivery→kernel→domain→adapter→infra),agent 用库层级(原语→引擎→策略)。给二者套同一套形状是 junior architect 的错误。
> 2. **当前设计 ~90% 收敛于 greenfield。** 真正欠的债只有一处:`delivery/server` 藏了 use-case 编排(`pump.go` + `rollback.go`)。修这一处,lyra 从 B+ → A。
> 3. **接缝处已经做硬**:lyra 在 `kernel` 定义 `agentRuntime` / `processControl` 窄接口消费 agent(字段不直接持有 `*runtime.Platform`);agent `core/` 定义 `ChatClientProvider` seam(lyra 经它提供 per-run client);tool loop 留共享基础设施;持久化 SPI 分工(agent 定义 `ProcessStore`,lyra 实现)。

---

## 0. 怎么读这份文档

- **作者设计视角**:本文是"如果重来"的应然设计。它**不引入新债、不要求改名**(与 ARCHITECTURE_REVIEW 同纪律)。
- **与既有文档的关系**:`GREENFIELD_ARCHITECTURE.md` 是 2026-06-14 落地的架构基准(已执行,代码按其树形组织);`ARCHITECTURE_REVIEW.md` 是 2026-06-18 现状体检;本文是 greenfield 重审 + 跨模块接缝。落地动作只认本文 §6 + ARCHITECTURE_REVIEW P0。
- **跨模块**:lyra 是消费方,接缝由它定义。agent 侧 greenfield 设计见 [`../../../agent/docs/GREENFIELD_DESIGN.md`](../../../agent/docs/GREENFIELD_DESIGN.md)。

---

## 1. 设计总纲:应用与库的天然不对称

### 1.1 lyra(应用)的天然形状

应用有真实的 delivery(HTTP/SSE transport)、真实的 infra(SQLite/git/lsp)、真实的 use-case(跑一个 turn、回滚一个 run)。Clean Arch 同心环**恰恰**是应用的正确形状:

```
lyra/internal/
├── delivery/      交付/协议适配器 — wire ↔ domain 翻译,调 kernel 用例
├── kernel/        微内核 — 定义 port,驱动 agent loop,编排 use-case
├── domain/        限界上下文 — 实体 + 领域规则 + 持久化 port(Store interface)
├── adapter/       ★greenfield 新环 — 领域化适配器(domain + infra 复合,非纯领域)
├── infra/         技术设施 — 实现 domain/adapter 的 port,零领域知识
├── runtime/       组合根 — 装配各环,nil-default SPI 注入
└── config/        纯数据
```

依赖方向:**一律向内**,`arch_test.go` 机器强制。这是 lyra 现状已做到的(等级 B+→A-),greenfield 从第一天就写。

### 1.2 与 agent(库)的形状对比

| | lyra(应用) | agent(库) |
|---|---|---|
| 自然形状 | Clean Arch 同心环 | 原语 + 引擎 + 策略 + SPI |
| 内部依赖 | 消费方窄接口(应用层处处 DIP) | 具体类型(库内部不做 ISP) |
| 有 delivery? | 有(HTTP/SSE) | 无 |
| 有 infra? | 有(SQLite/git/lsp) | 无(实现由库外写) |
| 有 use-case? | 有(跑 turn/回滚 run) | 无(用例由调用方写) |

**给二者套同一套形状是 junior architect 的错误。** greenfield 的核心洞察是拥抱这个不对称:lyra 用同心环 + 消费方窄接口,agent 用库层级 + 内部具体类型。接缝处保持干净(§4)。

### 1.3 接口放置规则(§2.5 的应用)

- **lyra 内部**:消费方定义窄接口(`delivery/server` 的 `RuntimePort`、`kernel/turn` 的 `engineDep`、`kernel` 的 `agentRuntime` / `processControl`)。
- **跨模块边界**:lyra 消费 agent 走 lyra 定义的窄接口,字段不直接持有 `*runtime.Platform`(§4.1)。
- **kernel port**:`kernel/port.go` 定义窄 port(`Compactor`/`Extractor`/`SteeringSink`),由 domain/infra 实现 —— 正确的六边形方向。

---

## 2. 环层与依赖规则

### 2.1 `delivery/server` use-case 泄漏(#1 lyra 债)

**greenfield:pump/rollback 编排搬进 `kernel/turn/`,不建独立 `application/` 环。**

- **为什么是 `kernel/` 而不是新环**:`kernel/` 本身就是微内核 + use-case 编排层。`kernel/turn.Dispatcher` IS the use-case 接口。"跑一个 turn" 和"回滚一个 run"是同层用例。加独立 `application/` 环是 `delivery/server` 的 1:1 影子(YAGNI —— GREENFIELD_ARCHITECTURE §5.3 正确裁决过)。
- **为什么不放 `domain/`**:pump 的 persist+interrupt+snapshot 是多 domain service 的协调,是 use-case 编排,不是领域规则。`domain/` 放单一限界上下文的实体+规则,不放跨域编排。
- **拆法**(不整体搬 pump.go):
  - pump 里的 `translator`(产出 `protocol.StreamEvent`)+ `hub`(推送 per-run hub)留在 `delivery/server` —— 这是真正的协议适配。
  - pump 里的 `persistStreamEvent` + `recordInterrupt` + `emitToolFileChange` + `snapshotCheckpoint` 协调 → 搬到 `kernel/turn/segment.go`,作为一个 `RunSegmentCoordinator`。
  - pump 仍是 delivery 的流式管道(translator → hub.Append + eventId),但调 kernel 层编排器做多服务协调。
  - rollback 编排(Transcript.List → BoundaryAt → restoreCheckpoint → TruncateMessages → DeleteRun/Delete → purgeSubtasks)→ `kernel/turn/rollback.go`,作为一个 use-case。

### 2.2 `kernel` vs `usecase` 命名

**`kernel` — 不变。** 理由(与 ARCHITECTURE_REVIEW §2.3 一致):`kernel` 描述"微内核"的本质(定义 port + 驱动 agent loop),`usecase` 是通用术语不传达这个特定架构模型。`REFACTORING §1` 要求名字符合本质,`kernel` 符合。`kernel.Engine`(避免 `kernel.Kernel` stutter)也是对的。

### 2.3 `domain/workspace` + `domain/codeintel`(import infra)

**greenfield:移到 `adapter/` 环。** 理由:

- 这两个包 import `infra/git`/`infra/lsp`/`infra/checkpoint`,本质是"领域化适配器"而不是纯领域。叫 `domain` 名实不符。
- Clean Architecture 中 domain 层应零外部依赖。`adapter/` 环准确描述它们的本质:对外(delivery)暴露领域语义(`ListChanges`/`Diff`/`Snapshot`),对内包装 infra 实现细节。
- 对消费方(delivery)来说 import 路径从 `domain/workspace` 变成 `adapter/workspace`,语义更诚实。
- 这是设计取向,非紧急重构 —— 纯机械 churn,零行为变化。

### 2.4 `maintenance` 的归属

**greenfield:放在 `adapter/maintenance`。** 这是正确的六边形方向:`maintenance` 是 kernel-owned port 的实现方(driven adapter),import port owner(`kernel`)的 DTO 合理;但它不应挂在 `domain/` 下。归属改成 `adapter/maintenance` 后,domain 不再需要为 `kernel` DTO 留例外。

### 2.5 `kernel/turn` use-case 粒度 + `adapter/toolset/build.go`

- **`kernel/turn` 粒度正确,不变。** `turn.Dispatcher` 接口定义"跑一个 turn"的完整生命周期(start/resume/cancel/steering/events)。exact right level of abstraction —— 消费方(delivery)只依赖这一个接口。greenfield 把 rollback 用例也放 `kernel/turn/` 旁边(同为 kernel 层 use-case 编排)。
- **`adapter/toolset/build.go` 单一装配点保持。** 这是 toolset 的组合根 —— 所有工具能力在一个地方初始化,依赖关系一目了然。拆成多个构造器散落装配逻辑。ARCHITECTURE_REVIEW §2.6 正确裁决。

---

## 3. 文件组织(greenfield 目标树)

```
lyra/
├── cmd/lyra/                           CLI 入口
│   ├── app.go                          App 装配 + ensureRuntime
│   ├── serve.go / agents.go / repl.go / chat.go / memory.go / session.go / version.go
│   ├── root.go                         cobra root + subcommand 注册
│   └── runner.go                       main() 入口
│
├── internal/
│   ├── config/                         纯数据
│   │   └── config.go                   LYRA_* env vars + Config struct + BuildChatClient
│   │
│   ├── runtime/                        组合根
│   │   └── runtime.go                  装配各环 + nil-default SPI 注入(绑进 delivery/server.Server)
│   │
│   ├── delivery/                       交付/协议适配器(薄!)
│   │   ├── protocol/                   冻结 wire 契约
│   │   │   ├── runtime.go              Runtime interface + 12 子接口 union + wire types
│   │   │   ├── events.go / items.go / errors.go / models.go
│   │   ├── server/                     in-process Runtime 实现
│   │   │   ├── server.go              Server struct + 构造 + 协议方法组
│   │   │   ├── translator.go          wire↔domain 翻译(turn.Event → StreamEvent/Item)
│   │   │   ├── hub.go                 per-run event hub(streamable HTTP 基础设施)
│   │   │   ├── pump.go          ★KEPT but THINNER 事件泵:translator → hub.Append + eventId(调 kernel 编排器)
│   │   │   ├── sessions.go / runs.go / items.go / memory.go / ...
│   │   │   │                         每个协议方法组一个文件,纯 decode → call → present
│   │   │   └── runtime.go         RuntimePort 窄接口(消费方定义)
│   │   ├── dispatch/                  JSON-RPC 方法路由(表驱动)
│   │   └── transport/                 envelope I/O
│   │       ├── transport.go           Transport 接口 + Message 类型别名
│   │       ├── http/                  HTTP+SSE transport
│   │       └── inprocess/             同进程 chan transport
│   │
│   ├── kernel/                         微内核(定义 port + 驱动 agent loop + use-case 编排)
│   │   ├── port.go                     核定义的窄 port + DTO(Compactor/Extractor/SteeringSink/Pricing)
│   │   ├── engine.go                   Engine:装配 prompt/tool/agent,驱动 loop(依赖 agentRuntime 窄接口)
│   │   ├── agent_runtime.go            lyra 消费 agent 的窄接口(取代字段直接持 *runtime.Platform)
│   │   ├── config.go                   kernel.Config(SPI 注入点)
│   │   ├── prompt.go / prompt_test.go  system prompt 骨架装配
│   │   ├── agent.go / chatturn.go / chatprocess.go   agent loop 驱动
│   │   ├── hitl.go                     park-on-interrupt + resume 编排
│   │   ├── mcp.go / skills.go          MCP 集成 / skill 取用
│   │   ├── observer.go / usage.go      事件观察 / usage 追踪
│   │   ├── turn/                       "跑一个 turn" 用例
│   │   │   ├── dispatcher.go          turn.Dispatcher interface — use-case 入口
│   │   │   ├── turn.go                turn 状态机 + lifecycle
│   │   │   ├── engine.go              engineDep 窄接口(turn→Engine)
│   │   │   ├── policy.go / observer.go / metrics.go / tracing.go
│   │   │   ├── errors.go / inmemory.go
│   │   │   ├── segment.go       ★NEW  RunSegmentCoordinator:persist+interrupt+snapshot 协调(从 pump 搬来)
│   │   │   └── rollback.go      ★NEW  rollback use-case(从 delivery/server/rollback.go 搬来)
│   ├── domain/                         限界上下文(纯领域 + Store port,零 infra 依赖)
│   │   ├── session/                    会话聚合根(Fork/NewSubtask/EffectiveModel/rollback 不变量)
│   │   ├── transcript/                 items+runs 时间线(BoundaryAt 领域算法 + RunNode)
│   │   ├── conversation/               喂 LLM 的消息上下文(InjectUser/TruncateMessages)
│   │   ├── knowledge/                  LYRA.md 长期知识(Store interface,用户可编辑)
│   │   ├── approval/                   运行态审批 stance(Mode)
│   │   ├── tool/                       工具注册 + 直接调用
│   │   ├── editguard/                  read-before-edit + stale 不变量
│   │   ├── interrupts/                 HITL 中断登记(Store CRUD)
│   │   ├── provider/                   provider 注册表(CRUD)
│   │   ├── todo/                       任务清单(Validate/Render)
│   │   ├── skills/                     skill 取用
│   │   └── agentdoc/                   AGENTS.md 级联发现 + render
│   │
│   ├── adapter/                        能力适配器(kernel/domain ports + infra wrappers)
│   │   ├── maintenance/                kernel maintenance port(压缩/提取/标题)
│   │   ├── toolset/                    工具装配层
│   │   │   ├── build.go                唯一构造点(codeintel/exec/mcp/a2a → 工具表 → resolver → closers)
│   │   │   ├── resolver.go             per-role/per-cwd 多源聚合
│   │   │   ├── editguard.go / pathguard_test.go
│   │   │   ├── shell/ askuser/ skill/ todotool/ turnctx/ exitplan/ lsptools/
│   │   ├── workspace/                  VCS 视图 + 文件 checkpoint(包 infra/git + infra/checkpoint)
│   │   └── codeintel/                  代码智能(包 infra/lsp)
│   │
│   ├── infra/                          技术设施(零领域,不依赖任何上层)
│   │   ├── storage/
│   │   │   └── sqlite/                 SQLite 后端(modernc 纯 Go)
│   │   └── git/ lsp/ checkpoint/ exec/ mcp/ a2a/
│   │
│   └── arch/                     ★KEPT 机器防腐
│       └── arch_test.go          ★KEPT 机器强制依赖方向(扩展 adapter 环规则)
│
└── doc/                                GREENFIELD_ARCHITECTURE.md(架构基准)/ ARCHITECTURE_REVIEW.md / GREENFIELD_DESIGN.md(本文)/ ...
```

### 与当前目录树的差异

| 改动 | 类型 | 理由 |
|---|---|---|
| **`adapter/` 环 — NEW** | 结构 | `workspace`/`codeintel` 移出 `domain/`。名实相符:它们本质是领域化适配器,放 `domain/` 但依赖 infra 是矛盾的 |
| **`kernel/turn/segment.go` — NEW** | 结构 | pump 的多服务协调(persist+interrupt+snapshot)从 delivery 搬到 kernel |
| **`kernel/turn/rollback.go` — NEW** | 结构 | rollback use-case 从 delivery/server/rollback.go 搬到 kernel |
| **`kernel/agent_runtime.go`** | 结构 | lyra 消费 agent 的窄接口(取代字段直接持 `*runtime.Platform`) |
| **`delivery/server/pump.go` — KEPT but THINNER** | 结构 | 保留为流式管道(translator → hub.Append),调 kernel 编排器 |
| **`delivery/server/rollback.go` — KEPT but THINNER** | 结构 | 保留 decode → 调 kernel rollback → present;wire helper 留下 |
| **`arch_test.go` 分环更新** | 结构 | 加入 `adapter/` 环规则 + 允许 delivery→adapter |
| **`domain/workspace`/`domain/codeintel` 移走** | 结构 | 见 adapter 环 |
| 其他全部 | 不变 | adapter/toolset/build.go、delivery/protocol/、domain/*、infra/* 全部正确 |

---

## 4. 跨模块接缝(lyra 如何消费 agent)

这是整个 greenfield 设计最重要的跨模块部分。lyra 是消费方,接缝由它定义。

### 4.1 lyra 是否直接持有 `*runtime.Platform` 字段?

**当前:否。lyra 定义自己的窄接口,构造点仍使用真实 agent runtime,但 Engine / ChatProcess 字段不直接持有完整 Platform。**

这是 `DESIGN_PHILOSOPHY §2.5` + `CLAUDE.md` ISP「库 vs 应用」规则的直接应用:

- lyra 是**应用层**,agent 是**库**。跨模块边界 + 多实现可能(测试 stub、未来 agent 替换)= 消费方窄接口的经典场景。
- `kernel.New` 仍构造真实 `runtime.Platform`,因为 `runtime.PlatformConfig` 和 `AsChatToolFromAgent` 是 agent 库的构造 API;但持久字段只依赖消费方接口。

```go
// lyra/internal/kernel/agent_runtime.go
type agentRuntime interface {
    agentStarter
    processControl
    Deploy(*core.Agent) error
}

type processControl interface {
    KillProcess(string) error
    ResumeProcess(string, any) (core.ResponseImpact, error)
    ContinueProcessAsync(context.Context, string) <-chan error
    RemoveProcess(string) error
    ProcessStore() core.ProcessStore
}
```

- `kernel.Engine` 依赖 `agentRuntime` interface(不是 `*runtime.Platform`)。
- `chatProcess` 只依赖 `processControl`,按取消 / resume / cleanup 的实际调用面拆小。
- `runtime.Platform` 隐式满足这些接口。

### 4.2 ChatClientProvider 接缝

**当前设计正确,不变。** agent 的 `core/` 定义 `ChatClientProvider` extension interface;lyra 的 `kernel/` 实现这个 extension,在 per-run 时按 `runs.start{provider, model}` 参数选 client。装配路径:

```
lyra delivery/serve decode runs.start{provider, model}
  → lyra kernel/turn startTurn
    → lyra kernel clientResolver.BuildClient(provider, model) → 建/缓存 chat.Client
    → 注入 ProcessOptions.Extensions = [ChatClientProvider wrapping that client]
      → agent runtime Platform.RunAgent(opts)
        → agent core ProcessContext.Chat() 通过 ChatClientProvider 拿到 per-run client
```

这条路径 greenfield 不变。`ChatClientProvider` 是 agent 定义的 SPI,lyra 是实现方 —— 正确的 port/adapter 方向。

### 4.3 事件桥

**当前设计正确,不变。** 三层翻译链:

```
agent event.* (lifecycle: ProcessStarted, ActionExecutionEnd, GoalAchieved…)
    ↓ agent EventListener extension
lyra kernel/turn/observer.go 订阅 → 翻译成 lyra turn.Event (业务事件)
    ↓
lyra delivery/server/translator.go 翻译成 protocol.StreamEvent (wire 事件)
    ↓
delivery/server/hub.go → SSE stream → 前端
```

agent 的事件是单向 lossy JSON(`Action`/`WorldState` 等 interface 字段降级成 summary)。lyra 的 turn 事件是业务语义。协议事件是 wire 契约。三层各司其职,不混淆。greenfield 不变。

### 4.4 Tool loop 所有权

**当前边界正确,不变。** Tool loop 属于 `agent/toolloop/`。

- `core/model/chat` 只保留 chat request/response/tool/middleware 原语，不拥有 agent 级循环策略。
- agent 的 `core/ProcessContext.Chat()` 只消费 guardrails 中注入的 middleware，不直接装配 tool loop。
- lyra 的 `kernel/engine.go` 作为应用组合根直接装配 `toolloop.NewMiddleware(cfg)`。

**greenfield 澄清**:agent/toolloop 依赖 core chat 原语并返回 `chat.CallMiddleware` / `chat.StreamMiddleware`；应用组合根把这对 middleware 注入 agent guardrails。依赖方向是 agent → core，runtime → agent/toolloop，没有 core → agent 的反向依赖。

### 4.5 ProcessStore 与 lyra 持久化的关系

**当前设计正确,不变。** agent 定义 SPI,lyra 实现。

```
agent/core/process_store.go:
    ProcessStore interface { Save/Load/Delete/List }

lyra/infra/storage/sqlite/:
    实现 ProcessStore(存 process snapshot 到 SQLite)

lyra/kernel/:
    通过 ProcessStore interface 使用(不依赖 sqlite 具体实现)
```

- agent 的 `ProcessStore` 存的是 `ProcessSnapshot`(含 `TaggedValue` 类型保持的黑板+条件+对象+调用历史)。
- lyra 的 session/transcript/conversation 是独立持久化(不同表),与 ProcessStore 不重复。
- ProcessStore 给 agent 提供"暂停/恢复"能力;lyra 的 session store 给前端提供"会话列表/时间线"。
- 二者正交,greenfield 不变。

---

## 5. 命名裁决

| 现状 | greenfield | 裁决理由 |
|---|---|---|
| `delivery` | **delivery** — 不变 | "交付"比 "adapter"/"api" 更形象。GREENFIELD_ARCHITECTURE §4 裁决已落地 |
| `kernel` | **kernel** — 不变 | 描述"微内核"本质:定义 port + 驱动 loop。`usecase` 是通用术语不传达这个模型。`kernel.Engine` 避免 `kernel.Kernel` stutter,正确 |
| `domain` | **domain** — 不变 | DDD 术语,准确描述限界上下文 |
| `infra` | **infra** — 不变 | 短、明确、Go 社区通用 |
| `adapter` | **adapter** — ★NEW | 准确描述 workspace/codeintel 的本质:领域化适配器 |
| `turn` | **turn** — 不变 | Lyra ubiquitous language(前端 API.md 的 "turn" = 一次交互),比 "chat"/"run" 准 |
| `protocol.Runtime`(11 子接口 union) | **不变** | 唯一全量消费方是 transport dispatch。子接口 ISP 已满足测试需求 |
| `translator`/`hub`/`pump` | **不变** | 名实相符 |
| `ProcessStore`/`SessionStore` | **不变**(不改 Repository) | 存的不是领域对象,Store 比 Repository 准确。`REFACTORING §1` 裁决已确认 |
| `Store`/`Registry`/`Policy`(各 domain interface) | **按本质保留** | 持久化用 Store,运行态目录用 Registry,决策用 Policy。不用 Repository(YAGNI 仪式) |

---

## 6. 落地优先级

### P0 — 立即可做,低风险

| # | 改动 | scope | API break? |
|---|---|---|---|
| **1** | **lyra 加 `adapter/` 环 + 移 `workspace`/`codeintel`** | ~2 包移动 + import 路径重写 | 破坏性(import 路径变更),需咨询用户。纯机械 churn,零行为变化。让 domain/ 真正纯净 |

### P1 — 结构性,需咨询用户

| # | 改动 | scope | 价值 |
|---|---|---|---|
| **2** | **lyra rollback 编排 → `kernel/turn/rollback.go`** | `delivery/server/rollback.go` 解体 → 编排进 kernel + wire helper 留 delivery | **#1 lyra 债**。让 delivery 回归"协议层薄"。破坏性(kernel 新公开 API) |
| **3** | **lyra pump 多服务协调 → `kernel/turn/segment.go`** | `delivery/server/pump.go` 变薄 → 调 kernel 编排器 | **#2 lyra 债**。pump 只剩 translator → hub.Append。破坏性(kernel 新接口) |
### P2 — 等触发再做

| # | 改动 | 触发条件 |
|---|---|---|
| 5 | `kernel` port DTO 移到独立 `kernel/types` | 有第二个 port 实现方需要这些 DTO |
| 6 | `delivery/server` 拆成子包(`server/runs/` 等) | server 超 ~6000 LOC |

### 明确不做(YAGNI 戒律)

| 不做 | 理由 |
|---|---|
| lyra 建独立 `application/` 环 | `kernel/turn.Dispatcher` + `kernel.Engine` 就是应用层。加一层是 1:1 影子 |
| `kernel` 改名 `usecase` | `kernel` 更准确(描述微内核本质)。改名降准确度。GREENFIELD_ARCHITECTURE §5.3 已裁决 |
| 给 lyra 加 Domain Events 总线 | 单进程无异步 side-effect 需求。显式编排比事件总线清晰 10 倍。ARCHITECTURE_REVIEW §3.5 正确裁决 |
| `ProcessStore`/`SessionStore` 改名 `Repository` | 存的是快照字节不是领域对象,Store 名实相符 |
| 拆 `delivery/server` 的 protocol 方法(sessions.go/runs.go...) | 方法数 = 协议方法数,1:1 绑定是健康的。拆散才是低内聚 |
| 给 `adapter/toolset/build.go` 拆构造器 | 单一装配点是优点,不是缺点 |
| 给 domain 实体加 Aggregate Root 显式标记 | 单后端不需要。`Session.Fork()` 已经是聚合根行为 |

---

## 一句话收尾

**lyra 是应用 —— Clean Arch 同心环(delivery→kernel→domain→adapter→infra)+ 机器强制 DAG 是它的正确形状。当前设计继续向 greenfield 收敛,真正欠的债主要剩 `delivery/server` 藏了 use-case 编排(pump + rollback)。把那两块搬进 kernel,lyra 就到 A。跨模块接缝已经用 lyra 侧 `agentRuntime` 窄接口消费 agent,与 agent 的库层级形成干净的不对称契约。**
