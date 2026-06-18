# Greenfield 设计 —— 如果从零写 agent 模块

> **日期**：2026-06-18。**视角**：架构师命题作文 —— "假设从零开始写 agent,系统架构与文件组织怎么设计?"
> **状态**：**greenfield 重审(非新基准)**。与 [`ARCHITECTURE_REVIEW.md`](ARCHITECTURE_REVIEW.md)(现状体检)的关系:那份回答"现在离健康还差什么",本文回答"如果重来该怎么设计"。两份合起来 = 该长成什么样 + 现在差什么。落地动作只认本文 §6 + ARCHITECTURE_REVIEW 的 P0。
> **方法**:第一性推导 + 对照 [`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md)(§2.3 一个扩展机制 / §2.4 包大≠god package / §2.5 具体度向上流动)+ [`../../REFACTORING.md`](../../REFACTORING.md)(§5.1 充血不叠层 / §8 大但内聚不拆)+ [`../CLAUDE.md`](../CLAUDE.md)(库内部用具体类型,不做内部 ISP)。与 embabel 的组织哲学印证见 [`EMBABEL_ORGANIZING_PRINCIPLES.md`](EMBABEL_ORGANIZING_PRINCIPLES.md)。
>
> **结论先行**:
> 1. **agent 是库,不是应用 —— 教科书 Clean Arch 环(delivery/use-case/domain/infra)对它不直接适用。** 库的自然形状是「原语(core)+ 引擎(runtime)+ 策略插件(planning)+ SPI(Extension)」。给库强套应用环 = 得到空壳 + 强行分包的割裂。
> 2. **当前设计 ~90% 收敛于 greenfield。** 真正欠的债只有两处:`core/` 原语层被 `core/model/chat` 污染(6 文件)+ 无 `arch_test.go` 机器防腐。修这两处,agent 从 B+ → A。
> 3. **"向 DDD 方向"对库的真正含义是:保持原语层纯 + 保持 SPI 干净 + 充血模型 —— 而不是加 repository/use-case/aggregate-root 层。** `REFACTORING §5.1` 明确拒绝叠 DDD 层,对库更是过设计。agent 已有充血模型,不需再加层。

---

## 0. 怎么读这份文档

- **作者设计视角**:本文是"如果重来"的应然设计,用来检验现状每一处决策是否经得起第一性推敲。它**不引入新债、不要求改名**(与 ARCHITECTURE_REVIEW 同纪律)。
- **与既有文档的关系**:`ARCHITECTURE_REVIEW.md` 是现状体检,本文是 greenfield 应然;`EXTENSION_DESIGN.md` 是 SPI 怎么设计,本文是整体怎么组织;`EMBABEL_*` 是与业界对照。落地动作只认本文 §6 + ARCHITECTURE_REVIEW P0。
- **跨模块接缝**:agent 是被 lyra 消费的库。接缝的完整设计(lyra 定义 `AgentRuntime` 窄接口、tool loop 归属、事件桥、持久化分工)见 [`../../app/runtime/doc/GREENFIELD_DESIGN.md`](../../app/runtime/doc/GREENFIELD_DESIGN.md) §4 —— lyra 是消费方,接缝由它定义。本文 §3 给出 agent 侧的接缝契约。

---

## 1. 设计总纲:库的天然形状

### 1.1 库 ≠ 应用,形状不同

应用有真实的 delivery(HTTP/SSE)、真实的 infra(DB/git)、真实的 use-case(跑 turn/回滚)。Clean Arch 同心环**恰恰**是应用的正确形状。

库没有 delivery、没有 infra(实现由库外写)、没有 use-case(用例由调用方写)。给库强套应用环 = 得到空壳 + 强行分包的割裂。**库的真正分层是依赖层级,不是应用环:**

```
agent/  (library)
├── core/          原语层 — 纯领域模型 + 公开 SPI(不进/出具体实现)
├── runtime/       引擎层 — 状态机 + dispatch(依赖 core)
├── planning/      策略层 — Planner SPI + 算法族(依赖 core)
├── event/         事件层 — lifecycle 信号(依赖 core)
├── workflow/      组合层 — 高阶 builder(依赖 core + runtime,库内部用具体类型)
├── hitl/          HITL 原语(依赖 core)
├── toolpolicy/    Chat tool 装饰器(依赖 core)
└── internal/arch/ 机器防腐 test(不参与生产编译)
```

### 1.2 库内部依赖规则(不同于应用环)

| From | Can import | Cannot import |
|------|-----------|---------------|
| `core/` | 仅 stdlib + 自身定义的 interface | `runtime/`/`planning/`/`event/`/`workflow/`/`hitl/`/`toolpolicy/`/任何具体实现 |
| `planning/`/`event/` | `core/` | `runtime/` |
| `runtime/` | `core/` + `planning/` + `event/` + `workflow/` + `hitl/` + `toolpolicy/` | — (引擎是所有上层策略的消费方) |
| `workflow/`/`hitl/`/`toolpolicy/` | `core/` + `runtime/` 具体类型 | — (库内部用具体类型,不做内部 ISP) |

**关键点**:库内部 `workflow/`→`runtime/` 是具体类型依赖,不需要抽窄接口。这不是债,这是 `agent/CLAUDE.md` 明确规定的"库内部用具体类型"原则。抽一个 `SubprocessSpawner` interface(单实现、无第二消费方)= YAGNI 仪式。

### 1.3 接口放置规则(§2.5 的应用)

- **内部**:具体类型直接依赖(`workflow/` → `*runtime.Platform`)。不抽窄接口(YAGNI)。
- **公开 SPI**:`Planner`/`Extension` 子接口/`ProcessStore`/`ChatClientProvider` 等 —— 定义在 `core/`(或 `planning/`),由库外实现。
- **`ChatClient` interface**:★greenfield 新增,定义在 `core/`(普适需求,见 §2.1),`core/model/chat.Client` 隐式满足。

---

## 2. 层次与依赖规则

### 2.1 `core/` 纯洁性(最核心的架构问题)

**greenfield 采用方案 A:在 `core/` 定义 `ChatClient` interface。** 理由:

- chat 是普适需求:每个 action 体都要调 LLM,`PromptCondition` 需要 LLM 评估条件,`Guardrails` 是 chat middleware 包装。按 `DESIGN_PHILOSOPHY §2.5`:普适需求 → 沉核(作为 interface)。
- `core/model/chat` 是整个 lynx 共享的基础设施,不属于 agent。agent 的 `core/` 应只定义 interface,由库外装配具体实现。
- 不采用方案 B(把 chat 文件搬出 core/):`ProcessContext.Chat()` 是 action 体的唯一 LLM 入口,拆开会导致 action 体要 import 两个包(体验倒退)。

`core/chat.go` 定义的最小 interface 面:

```go
// core/chat.go

// ChatClient is the minimal LLM interface that core/ primitives depend on.
// It is satisfied implicitly by core/model/chat.Client.
type ChatClient interface {
    Call(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Stream(ctx context.Context, req ChatRequest) iter.Seq2[ChatStreamChunk, error]
}

type ChatRequest struct { /* Messages, Tools, Options — value types, not pointers */ }
type ChatResponse struct { /* Message, FinishReason, Usage */ }
type ChatStreamChunk struct { /* Delta, ToolCallDelta, FinishReason */ }
type ChatTool struct { /* Name, Description, Schema, Call */ }
type ChatMiddleware func(ChatClient) ChatClient
```

现状 6 个生产文件(`process_context.go`/`condition.go`/`prompt_runner.go`/`guardrails.go`/`extension.go`/`tool_group.go`)import `core/model/chat` 具体类型 → greenfield 全部改 import `core.ChatClient` interface。`core/` 变纯。

### 2.2 tool loop 归属

Tool loop 当前在 `core/model/chat/middleware/tool/`(12 文件,共享 lynx 基础设施)。**greenfield:留在原处,不搬进 agent。** 理由:

- tool loop 是通用 chat middleware(调模型 → 执行 tool call → 喂回 → 重复直到模型停止调 tool),不是 agent 专属概念。lyra 的 chat turn、未来非 agent 消费者都可能用它。
- agent 的 `core/` 通过 `ChatClient` interface 消费它(`ProcessContext.Chat()` 内部装配 tool middleware),不直接 import `core/model/chat/middleware/tool`。
- 放共享基础设施层是对的 —— 第三消费者出现时无需搬家。

### 2.3 `runtime/` 53 文件

**greenfield:保持一个包,不拆。** 理由(与 ARCHITECTURE_REVIEW §2.3 一致):

- `runtime/` 核心 ~15 个文件是**同一个状态机**的不同面(Platform 构造、AgentProcess 状态转移、tick 循环、action 执行、extension dispatch、并发/子进程 spawn)。
- 拆分会制造跨包子包依赖,增加 import 噪音,且违反"库内部用具体类型" —— 子包之间需要互相 import 具体类型。
- 53 文件在 Go 大包阈值内(`net/http` 远超此数)。`subagent.go`/`mcp.go`/`publish.go` 关注点不同但紧密依赖 `*Platform`/`*AgentProcess` 具体类型 —— 拆出去增加噪音多于收益。
- 触发条件(runtime/ 超 60 文件 + 出现第二个外部消费者)满足时才考虑拆。

### 2.4 其余包放置

| 包 | 当前放置 | greenfield 裁决 | 理由 |
|---|---|---|---|
| `planning/` | 策略层,Planner SPI + 4 算法子包 | 不变 | OCP 典范:加新 planner 放 `planning/planner/<name>/`,不动 runtime |
| `event/` | 事件层,lifecycle 类型 + Multicast | 不变 | 内聚的事件包;单向(runtime→observer),不形成循环依赖 |
| `hitl/` | HITL 原语,`Interrupt[R]` + `Awaitable[T]` | 不变 | 依赖 core 但不依赖 runtime,正确分层 |
| `toolpolicy/` | Chat tool 装饰器 | 不变 | 依赖 `core.AgentTool`(chat tool 概念在 core),正确 |
| `workflow/` | 高阶 builder,产出 `*core.Agent` | 不变,保持 import `runtime` 具体类型 | 库内部用具体类型。不抽 `SubprocessSpawner`(YAGNI) |
| `autonomy/` | Ranker-based goal 选择,在 `runtime/` 下 | 留在 `runtime/` 下 | 紧依赖 `*runtime.Platform`,库内部用具体类型合理 |

---

## 3. 跨模块接缝(agent 侧契约)

agent 是被 lyra 消费的库。接缝的完整设计由消费方(lyra)定义,见 [`../../app/runtime/doc/GREENFIELD_DESIGN.md`](../../app/runtime/doc/GREENFIELD_DESIGN.md) §4。agent 侧的契约:

| 契约 | agent 侧职责 |
|---|---|
| lyra 消费 agent | agent 暴露 `*runtime.Platform` 的公开 API;lyra 定义 `AgentRuntime` 窄接口(嵌入 `Deploy`/`RunAgent`/`StartAgent`/`ResumeProcess`/`KillProcess` 等),`*runtime.Platform` 隐式满足。agent 不需要为 lyra 改任何东西 |
| per-run model | agent 定义 `ChatClientProvider` extension SPI(`core/extension.go`),lyra 实现。装配路径:lyra 按 `runs.start{provider,model}` 建 client → 注入 `ProcessOptions.Extensions` → agent `ProcessContext.Chat()` 经 `ChatClientProvider` 拿到 per-run client |
| 事件桥 | agent 发布 `event.*` lifecycle 事件(`ProcessStarted`/`ActionExecutionEnd`/`GoalAchieved` 等),单向 lossy JSON。lyra 经 `EventListener` extension 订阅,翻译成 lyra `turn.Event`。agent 不感知 lyra |
| tool loop | tool loop 在共享 `core/model/chat/middleware/tool/`。agent `core/` 经 `ChatClient` interface 消费;lyra 直接装配。agent 不拥有 tool loop |
| 持久化 | agent 定义 `ProcessStore` SPI(`core/process_store.go`,`Save/Load/Delete/List`),存 `ProcessSnapshot`(含 `TaggedValue` 类型保持)。lyra 在 `infra/sqlite` 实现。agent 不依赖任何具体后端 |

---

## 4. 文件组织(greenfield 目标树)

```
agent/
├── core/                              原语层 — 纯领域模型 + 公开 SPI
│   ├── agent.go                       Agent + AgentConfig(配置聚合)
│   ├── action.go                      Action interface + ActionMetadata + ActionQoS
│   ├── action_typed.go                NewAction[In, Out] + TypedAction — 泛型 action 工厂
│   ├── goal.go                        Goal + GoalExport + GoalProducing
│   ├── condition.go                   Condition + ComputedCondition + And/Or/Not 组合子
│   ├── prompt_condition.go            PromptCondition — LLM-driven 条件(用 core.ChatClient)
│   ├── determination.go               Determination 三值逻辑(Unknown/True/False)
│   ├── blackboard.go                  Blackboard(Reader+Writer+Spawn+Clear) + helper
│   ├── process.go                     Process interface + Status enum
│   ├── process_context.go             ProcessContext — action 体的唯一入口
│   ├── process_options.go             ProcessOptions + Budget + Session
│   ├── chat.go                  ★NEW  ChatClient/ChatRequest/ChatTool/ChatMiddleware interfaces — core/ 纯度关键
│   ├── guardrails.go                  Guardrails — chat middleware wrapper(用 core.ChatMiddleware)
│   ├── prompt_runner.go               PromptRunner — action 体 LLM 调用门面(用 core.ChatClient)
│   ├── extension.go                   Extension marker + 12 capability 子接口
│   ├── tool_group.go                  ToolGroup + AgentTool(用 core.ChatTool)
│   ├── service_provider.go            ServiceProvider — 开放 service locator
│   ├── early_termination.go           EarlyTerminationPolicy + BudgetPolicy
│   ├── process_store.go               ProcessStore SPI + ProcessSnapshot + SnapshotCodec
│   ├── session.go                     SessionStore SPI + Session
│   ├── awaitable.go                   Awaitable[T] — HITL 原语
│   ├── invocation.go                  LLMInvocation / EmbeddingInvocation — 计费记录
│   ├── hooks.go / id_gen.go / output_channel.go / io_binding.go / enum.go / doc.go
│
├── runtime/                           引擎层 — 状态机 + dispatch(一个包,不拆)
│   ├── platform.go / platform_deploy.go / platform_run.go / platform_process.go
│   ├── agent_process.go               AgentProcess struct + state/budget/signals + 扩展访问器
│   ├── run.go                         run() tick loop + tickSimple + tickConcurrent
│   ├── execute_action.go              executeAction + retry + middleware chain + panic recovery
│   ├── process_snapshot.go / concurrent.go / child.go
│   ├── extension.go / dispatch.go     Extension 注册 + collectExtensions[T] 类型分发
│   ├── registries.go                  内部 registry helper
│   ├── subagent.go / subagent_background.go  子进程 spawn + agent-as-tool
│   ├── agent_tool.go                  AsChatTool / SubagentTools / AllAchievableTools
│   ├── mcp.go / publish.go / metrics.go
│   ├── autonomy/                      Ranker-based goal 选择
│   │   ├── router.go            ★RENAMED  Autonomy → Router(消 stutter)
│   │   ├── ranker.go                  LLMRanker / PlanRanker
│   │   └── types.go
│
├── planning/                          策略层 — Planner SPI + 算法族
│   ├── planner.go                     Planner interface + WorldState + System
│   ├── plan.go                        Plan + ActionStep + NetValue
│   └── planner/{goap,htn,reactive,utility}/
│
├── event/                             事件层 — lifecycle 信号
│   ├── event.go / process.go / execution.go / planning.go / invocation.go / platform.go
│   ├── listener.go / multicast.go / marshaling.go / summaries.go
│
├── workflow/                          组合层 — 产出 *core.Agent(import runtime 具体类型)
│   ├── types.go / sequence.go / loop.go / parallel.go / repeat_until.go
│   ├── repeat_until_acceptable.go / scatter_gather.go / consensus.go / supervisor.go
│
├── hitl/                              HITL 原语
│   ├── interrupt.go                   Interrupt[R] + InterruptError
│   ├── awaitable.go                   Awaitable[T] 类型化
│
├── toolpolicy/                        Chat tool 装饰器
│   └── once_only.go / unlocked.go / ...
│
├── internal/arch/               ★NEW  机器防腐(不参与生产编译)
│   └── arch_test.go            ★NEW  机器强制依赖规则
│
└── examples/                          Demo(非 production,refactor 时 ignore)
```

### 与当前目录树的差异

| 改动 | 类型 | 理由 |
|---|---|---|
| **`core/chat.go` — NEW** | 结构 | 定义 `ChatClient` interface,消除 6 文件对 `core/model/chat` 具体类型的 import。`core/` 变纯 |
| **`internal/arch/arch_test.go` — NEW** | 结构 | 机器强制:core ↛ runtime/planning/event/workflow/hitl/toolpolicy;planning/event ↛ runtime。零 API break |
| **`autonomy/autonomy.go` → `autonomy/router.go`** | 命名 | `type Autonomy` → `type Router`,消 `autonomy.Autonomy` stutter |
| **`core/` 6 文件改 import:`"core/model/chat"` → `core.ChatClient`** | 结构 | 原语层不再 import 具体 chat 实现 |
| **`runtime/` 不拆** | 不变 | 53 文件在内聚阈值内;拆了增加跨包具体类型 import |
| **`workflow/` 保持 import `runtime`** | 不变 | 库内部用具体类型;不抽 SubprocessSpawner(YAGNI) |
| **`planning/` + `event/` + `hitl/` + `toolpolicy/` 不变** | 不变 | 当前放置正确 |

---

## 5. 命名裁决

| 现状 | greenfield | 裁决理由 |
|---|---|---|
| `core` | **core** — 不变 | Go 社区通用,"原语层"语义清楚。`primitives` 太学究,`domain` 是 DDD 术语但 agent 含 SPI/Store 不全是 domain |
| `runtime` | **runtime** — 不变 | 描述"执行引擎"本质。`engine` 曾用名(已改),`usecase` 是应用术语对库不准 |
| `planning`/`event`/`hitl`/`toolpolicy`/`workflow` | **全不变** | 名实相符 |
| `autonomy.Autonomy` | `autonomy.Router` — **改** | 消 stutter;"路由器"准确描述本质(选哪个 agent 处理请求)。破坏性改名,需咨询用户 |
| `AgentProcess` vs `Process` | **不变** | HAS-a 关系,命名正确区分(Process 是 core 的 read surface,AgentProcess 是 runtime 的完整状态机) |
| `Determination`/`Blackboard`/`Goal`/`Action` | **不变** | 准确 |
| `collectExtensions[T]`/`Extension` marker | **不变** | `http.Pusher` 风格,OCP 干净 |
| `ProcessStore`/`SessionStore` | **不变**(不改 Repository) | 存的是快照字节,不是领域对象。Store 名实相符 |
| Java 味检查 | **全部通过** | 无 Impl/Service/Manager/Helper 后缀,无 builder 链,无 GetX/SetX |

---

## 6. 落地优先级

### P0 — 立即可做,低风险

| # | 改动 | scope | API break? |
|---|---|---|---|
| **1** | **加 `internal/arch/arch_test.go`** | ~60 行纯测试 | **零**。编码规则:core ↛ runtime/planning/event/workflow/hitl/toolpolicy;planning/event ↛ runtime。最大杠杆:立即防腐,防未来回归 |
| **2** | **`autonomy.Autonomy` → `Router`** | 1 文件重命名 + lyra import 点 | 破坏性(类型名变更),需咨询用户。改动小,风险低 |

### P1 — 结构性,需咨询用户

| # | 改动 | scope | 价值 |
|---|---|---|---|
| **3** | **`core/` 定义 `ChatClient` interface(方案 A)** | `core/chat.go` 新增 + 6 文件改 import + `core/model/chat.Client` 隐式满足 | **agent 从 B+ → A 的那一步**:原语层变纯,所有 chat 依赖走 interface。破坏性(`ProcessContext.Chat()` 返回类型变 interface),需咨询用户 |

### P2 — 等触发再做

| # | 改动 | 触发条件 |
|---|---|---|
| 4 | `runtime/` 拆 `subagent/` + `mcp/` 子包 | runtime/ 超 60 文件 且 有第二个外部消费者 |
| 5 | `domain/maintenance` 的 `kernel` DTO 移到独立 `kernel/types` | 有第二个 port 实现方需要这些 DTO |

### 明确不做(YAGNI 戒律)

| 不做 | 理由 |
|---|---|
| 给 agent 叠 DDD 层(AggregateRoot/Repository/ApplicationService) | 库不是应用,已有充血模型。`REFACTORING §5.1` 明确拒绝 |
| 把 agent `runtime/` 拆成 Clean Arch 环 | 库不是应用。拆了违反"库内部用具体类型" |
| `ProcessStore`/`SessionStore` 改名 `Repository` | 存的是快照字节不是领域对象,Store 名实相符 |
| 给 `workflow/` 抽 `SubprocessSpawner` interface | 库内部单实现,YAGNI 仪式 |
| `ServiceProvider` 加 typed Register | `map[string]any` 是存储层物理约束,typed Register 零新增安全,cosmetic |
| 拆 `process_context.go`(388 LOC) | `REFACTORING §8`:大但内聚不拆。3-zone 分区健康 |
| `AgentTracer()` 抽 interface 解耦 OTel | YAGNI:单 consumer,无痛感 |

---

## 一句话收尾

**agent 是库 —— 教科书 Clean Arch 环对它不直接适用,项目哲学的「库内部用具体类型、不叠 DDD 层」对它是对的。充血原语 + Extension 单分发 + planning 策略插件已是教科书级别好设计。真正欠的债是 `core/` 被 chat 污染 + 无机器防腐 —— 加 `arch_test.go` 立即防腐,定义 `ChatClient` interface 让原语层变纯,就到 A。**
