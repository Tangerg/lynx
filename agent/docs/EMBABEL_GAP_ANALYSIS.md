# lynx/agent vs embabel-agent — 深度对比与缺口分析

> **第八轮重写**（2026-05-09）。基线：embabel-agent (Kotlin/Spring) v0.4 / lynx/agent (Go) HEAD `6a1e7a1`。
>
> 本轮相对第七轮的关键变动（仅做了两件事 + 一次 fresh 对照）：
> - **Wave A**：拍平包布局 —— 删 `dsl/` 子包；`dsl/builder.go` → `agent/builder.go`（Builder 直接住在根包）；`dsl/workflow/` → `workflow/`；`dsl/toolpolicy/` → `toolpolicy/`。`workflow` 内部从原来"穿透 dsl.Builder 再 Build()"改为直接 `core.NewAgent(core.AgentConfig{...})`，少一层封装。
> - **Wave B**：合并 `runtime/subagent.go` 中的 `subagentTool` 与 `mcpTool` —— 两个只差"启动进程的策略"的 `[In, Out]` wrapper 合成一个 `agentTool[In, Out]` + `runProcessFunc[In]` 策略函数（`runAsChild` for `AsChatTool` / `runAsTopLevel` for `AsMCPTool`）。Definition / Metadata / Call / status-switch / output-extract 不再各写一遍。
> - **新一轮 fresh 对照**：见 §15.5 —— 以 P0/P1/P2 优先级列出 embabel 有而 lynx 仍缺的具体能力（`Goal.Export` 元数据 / `PromptCondition` / `AchievableGoalsToolGroupFactory` / `OperationScheduler` / `ModelProvider` / `OptimizingGoapPlanner` …）。
>
> 第七轮"减负"的成果保留；第八轮**只做结构整理**，没有删字段。**非测试代码 8715 LOC**（第七轮 8853 → 第八轮 8715，-138 LOC 来自 dsl 包消除 + subagent 合并），所有 examples + 测试包仍绿。
>
> 配套文档：[`./EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md) / [`./PERSISTENCE.md`](./PERSISTENCE.md) / [`/Users/tangerg/Desktop/lynx/mcp/DESIGN.md`](../../mcp/DESIGN.md)。

---

## 0. TL;DR

| 维度 | lynx/agent | embabel-agent | 差距 |
|---|---|---|---|
| 核心抽象（Agent / Goal / Action / Condition / Blackboard / WorldState） | ✅ 完整、本轮再瘦身（去 `Tags`/`Examples`/`Export`/`OutputType` 等） | ✅ 完整 | **lynx 更精简** |
| GOAP planner（A* + reachability） | ✅ `plan/planner/goap` | ✅ `DefaultPlannerFactory.kt` | **无差距** |
| HTN planner | ✅ `plan/planner/htn` | ✅ | **追平** |
| Reactive / Utility planner | ✅ `plan/planner/reactive`（progress × cost-tie，强于 embabel） | ✅ `UtilityPlanner.kt` | **lynx 更强** |
| Plan 后处理 / 多 plan 排序 | ✅ `plan.BestOf` + `plan.SortByNetValueDesc`（`slices.SortStableFunc`） + `autonomy.LLMPlanRanker` | ✅ + `LlmRanker` | **追平** |
| OODA tick loop（Sequential + Concurrent） | ✅ `runtime/run.go` + `runtime/concurrent.go` | ✅ | **无差距** |
| HITL: process + tool 双层 | ✅ `hitl.TypedRequest` + `hitl/tool.go`（WithAwaiting / WithConfirmation / RequireType） | ✅ `Awaitable<P,R>` + `AwaitingTools.kt` | **追平** |
| Extension 模型 | ✅ 1 注册入口 + **5** core capability + 4 runtime/extension capability + type-assertion 检测 | 多 SPI + Spring DI | **lynx 更整洁** |
| 事件 / 可观测 | ✅ **14** 个事件类型 + JSON marshaler + OTel tracer + `event.Multicast` | ✅ `AgenticEventListener` + Micrometer | **等价** |
| Tool 模型 / advanced policies | ✅ `core/tool_group.go`（与 `chat.Tool` 共用类型） + `toolpolicy/`（OnceOnly + Unlock） | ✅ Tool / ToolObject + `OneShotPerLoopTool` / `PlaybookTool` | **基线追平** |
| MCP 客户端 / 服务端 | ✅ `lynx/mcp` 全功能 + `runtime.MCPToolGroupResolver` + `runtime.AsMCPTool[In,Out]` | ✅ `SpringAiMcpToolFactory` + `McpToolExport` + `PerGoalMcpExportToolCallbackPublisher` | **闭合** |
| Supervisor / Subagent | ✅ `runtime.AsChatTool` + `AsChatToolFromAgent` + waiting graceful-degrade | ✅ `Subagent.kt`（4 工厂路径 + `RunSubagent` 注解） | **追平**；细差见 §10 |
| WorkflowBuilder | ✅ ScatterGather / RepeatUntil / RepeatUntilAcceptable / Consensus / Feedback 全套 | ✅ `api/common/workflow/` | **追平** |
| Autonomy / Goal Ranking | ✅ `runtime/autonomy`：`Choose`/`Run` + `LLMRanker` + `LLMPlanRanker` + cutoff + filter | ✅ `Autonomy` + `Ranker` | **追平** |
| 持久化 | ⚙️ `BlackboardFactory` 扩展点 + [`PERSISTENCE.md`](./PERSISTENCE.md)；开箱仍 in-memory | ✅ Spring Data + `InMemoryAgentProcessRepository` | **抽象等价**；开箱实现欠（按设计） |
| 多 LLM provider 内置 / 注解 / classpath scan / Shell / RAG / A2A / Skills / Onnx | ❌ | ✅ 7 独立模块 | **故意分歧** |

**一句话**：第七轮没有新增功能，纯做了一轮"减负"——把上一轮发布后两个月里收集到的死字段、防御式重复检查、单实现虚接口、过度文字化的 docstring 一口气拔掉。**lynx 的功能面没有缩水，体积反而下降 ~7%**。基线对齐已稳定一轮以上，下一阶段路线由用例驱动，不再靠 framework 内推。

---

## A. 宏观对比（macro）

### A.1 设计哲学坐标

| | lynx/agent | embabel-agent |
|---|---|---|
| 定位 | **minimal kernel + composable Go primitives** | **batteries-included Spring stack with annotation-driven discovery** |
| 非测试代码量 | **8853 LOC**（30+ Go 文件） | **~30k+ LOC**（200+ Kotlin 文件，700 个 .kt 含 ~87k 总行） |
| Maven/Module 划分 | 1 仓库 / 7 包（core / plan / runtime / event / dsl / hitl / agent）+ sibling `lynx/mcp` | **12+ Maven module**（api / code / a2a / mcp / rag / skills / shell / onnx / openai / common / domain / observability / starters / autoconfigure …） |
| 入口风格 | 显式 `dsl.Builder` + 结构体配置 | classpath scan + `@Agent` / `@Action` / `@AchievesGoal` |
| DI 容器 | 无（`core.ServiceProvider` 是个 type-keyed map） | Spring `ApplicationContext` 全栈 |

同一份领域模型（GOAP / Goal / Action / Condition / Blackboard / WorldState / Awaitable / ToolGroup），分布密度悬殊。

### A.2 层级映射（1:1）

| lynx | embabel |
|---|---|
| `agent/core/` | `embabel-agent-api/src/main/kotlin/com/embabel/agent/core/` |
| `agent/plan/` + `plan/planner/{goap,htn,reactive}/` | `embabel-agent-api/src/main/kotlin/com/embabel/plan/{goap,common,utility}/` |
| `agent/runtime/` + `runtime/autonomy/` | `embabel-agent-code/src/main/kotlin/com/embabel/agent/` + `agent/core/internal/` |
| `agent/builder.go`（根包 `Builder`，第八轮拍平）+ `agent/workflow/` + `agent/toolpolicy/` | `embabel-agent-api/src/main/kotlin/com/embabel/agent/api/{dsl,common/workflow}/` + `agent/agentic/` |
| `agent/event/` | `embabel-agent-api/src/main/kotlin/com/embabel/agent/event/` |
| `agent/hitl/` | `embabel-agent-api/src/main/kotlin/com/embabel/agent/core/hitl/` |
| `runtime/autonomy/` | `embabel-agent-api/.../spi/Ranker.kt` + `Autonomy.kt` |
| `(sibling) lynx/mcp` + `runtime/mcp.go` | `embabel-agent-mcp/` |

**重量差异**：embabel 的 `embabel-agent-code/` 比 lynx 的 `agent/runtime/` 重 **3-4 倍**（多了 Spring config / actuator / autoconfigure 胶水）；反过来 lynx 的 `core/` 比 embabel 的 `core/` 略瘦（embabel 多了 `DataDictionary` / `DomainType` / `DynamicType` / `expression/` SpEL 子树 / `internal/streaming/` 等内核分流）。**plan/ 体量近似**——GOAP/HTN/Reactive 三种 planner 都是相同粒度的算法实现。

### A.3 API discovery model

embabel：classpath scan + 注解。Spring Boot starter 装好后用户写 `@Agent class WriterAgent { @Action fun draft(...) }` 即出现在 platform。代价：反射 / classpath ordering 依赖 / Spring 应用生命周期 / IDE rename 在某些路径上不安全。

lynx：显式 `agent.New("writer").Actions(...).Goals(...).Build()` + `platform.Deploy(agent)`。代价：每个 agent 多写 5-10 行胶水。回报：编译时类型安全 / IDE rename 100% 安全 / 启动期可静态推理 agent 图。

**这是永久的哲学分歧**——把它当 trade-off 而非 gap。Go 没有 Spring AOT，把反射注册照搬过来就是反 Go 的；embabel 也不会为了 lynx 风格放弃 Spring 的工程基础设施。

### A.4 Dependency graph

| | lynx | embabel |
|---|---|---|
| 主依赖 | `golang.org/x/sync/errgroup` / `go.opentelemetry.io/otel` / `github.com/Masterminds/semver/v3` / `github.com/google/uuid` / `github.com/swaggest/jsonschema-go`（全部小而稳定） | Spring Framework / Spring Boot / Spring AI / Jackson / Micrometer / SLF4J / Reactor / Caffeine 等（百级 transitive） |
| 上游表面积 | 极小（每个第三方包都能在 1 天内复审完） | 庞大（Spring AI 一项即 6-7 位数 LOC） |

两者都能落地，**lynx 的小依赖面是设计选择**——便于嵌进 Go 微服务、Lambda、CLI 工具。

### A.5 embabel 有而 lynx 永远不会有

- Spring DI auto-wiring + `@Bean` config
- profile-based feature gates（`@Profile`）
- classpath-scan agent 注册
- Spring Boot starter 生态 / autoconfigure
- Shell + 4 个 personality（Severance / Hitchhiker / Star Wars / Monty Python）
- 内置 RAG 子系统（5 个 RAG module）
- 内置 ONNX embedding 本地推理
- 内置 OpenAI/兼容层

每一项都是 Spring 生态的延伸或 JVM 部署形态特有的便利。**这是分工分歧而非缺口**。

### A.6 lynx 有而 embabel 没有

- 统一的 ToolLoop runner —— `chat.NewToolMiddleware`，**一处实现覆盖所有调用点**。embabel 的 `ToolLoop.kt` 旁边还有多个 `ToolInjectionStrategy` 变体（Matryoshka / Unfolding / Standard）需要在不同路径上各自维护。
- 头等的 ctx-driven 进程注入 —— `core.WithProcess(ctx, p)` + `core.ProcessFrom(ctx)`，让每个 tool / 中间件 / decider 都能从 ctx 拿到当前进程，无需 thread-local 或方法签名穿透。embabel 走 `ProcessContext` 显式传参 + 部分 thread-local。
- 参数化的 subagent factory —— `runtime.AsChatTool[In, Out]` / `AsChatToolFromAgent[In, Out]` / `AsMCPTool[In, Out]` 三个 type-parameterised helper 顶替 embabel 的 `Subagent.byName()`/`ofClass()`/`ofInstance()`/`ofAnnotatedInstance()` 4 路 Builder + 注解。第八轮把这三个工厂背后的 wrapper 合并到 **一个** `agentTool[In, Out]` + `runProcessFunc[In]` 策略函数（[runtime/subagent.go:99](../runtime/subagent.go)），新增第四种调用形态只需写一个闭包。
- 显式的 PlannerType 枚举 —— `core.PlannerGOAP / PlannerHTN / PlannerReactive`，编译时穷举 `switch`。embabel 是 Spring bean 的字符串拼装。
- 顶层包扁平化 layout（第八轮拍平） —— 五个根包：`core`（primitives）+ `plan`（planner SPI + 三种实现）+ `runtime`（platform / process / subagent / autonomy）+ `event` + `hitl`，加两个旁路工具包 `workflow` / `toolpolicy`。原 `dsl/` 子包消除——`Builder` 升到根包（`agent.New(...)` 即原生构造器，非转发），`workflow` 不再走 `dsl.New` 中转，直接 `core.NewAgent(core.AgentConfig{...})`。
- 死代码的零容忍 —— 第七轮 Wave B 的删除清单（见 §17）embabel 不会做：Spring 体系下任何"看起来可能给用户用"的字段都不敢动。

---

## 1. 核心抽象 — 1:1 对齐，本轮再瘦身

| 抽象 | lynx | embabel |
|---|---|---|
| Agent | `core.Agent` ([core/agent.go:52](../core/agent.go)) | `core.Agent` |
| AgentConfig | 仅 6 字段：`Name / Description / Version / StuckHandler / Actions / Goals / Conditions`（[core/agent.go:15](../core/agent.go)） — Wave B 删去 `Provider / Opaque / DomainTypes / ToolGroupRequirements` 4 个未启用字段 | `Agent` data class（含 Spring 注入字段） |
| Action / metadata / retry | `core.Action` + `core.ActionConfig`（[core/action_config.go:13](../core/action_config.go)） — Wave B 删 `ReadOnly / Trigger / TriggerType` | `Action.kt` + `ActionRetryPolicy.kt` |
| Goal | `core.Goal`（[core/goal.go:9](../core/goal.go)） — Wave B 删 `Tags / Examples / Export / OutputType`，仅留 `Name / Description / Pre / Inputs / Value` | `Goal.kt`（含 `tags / examples / export` 字段） |
| Condition + And/Or/Not | `core.Condition` + `ComputedCondition` ([core/condition.go](../core/condition.go)) | `Condition.kt` |
| Blackboard | `core.Blackboard` ([core/blackboard.go](../core/blackboard.go)) | `Blackboard.kt` |
| WorldState | `core.WorldState` + `plan.ConditionWorldState`（[plan/condition_world_state.go](../plan/condition_world_state.go) — Wave D 改为 `slices.Sorted(maps.Keys(...))` 一行） | `WorldState` |
| EffectSpec | `core.EffectSpec` —— Wave B 删 `Clone / Merge / Keys / Set` 4 个 helper（YAGNI） | `Effects` typealias |
| Determination | `True / False / Unknown` 三态 —— Wave B 删 `IsTrue / IsFalse / IsUnknown` 谓词，让 caller 写 `d == core.True` | `Determination` enum |
| ServiceProvider | `core.ServiceProvider`（[core/service_provider.go](../core/service_provider.go)） — Wave B 删 `Has / Delete / Keys`（type-keyed registry，正常用法不需要枚举） | Spring `ApplicationContext` |

**结论**：核心模型对齐，**lynx 比上一轮更紧**。Goal 现在的字段表 `Name / Description / Pre / Inputs / Value` 跟 embabel `Goal.kt` 的核心字段完全 1:1；之前堆在 lynx Goal 上的 `Tags/Examples/Export/OutputType` 是仿 embabel 抄过来但 lynx 自己 runtime 从没 read 过的死字段，本轮全部砍。

---

## 2. Planner — 三全；公共 helper 已抽出；`Prune` 接口已废

| 维度 | lynx | embabel |
|---|---|---|
| A* GOAP | ✅ `plan/planner/goap/astar.go` | ✅ `plan/goap/astar/` |
| Reachability pre-check | ✅ ([runtime/platform.go](../runtime/platform.go)) | ✅ |
| Excluded actions（防 replan loop） | ✅ `state.snapshotExclusions` ([runtime/process_state.go:122](../runtime/process_state.go)) | ✅ |
| Plan 排序 | ✅ `plan.BestOf` + `plan.SortByNetValueDesc`（Wave D 用 `slices.SortStableFunc` + `cmp.Compare` 重写） | ✅ |
| HTN planner | ✅ `plan/planner/htn`（Library + Task + Method，ctx-cancellable） | ✅ |
| Reactive / Utility planner | ✅ `plan/planner/reactive`（progress × cost-tie，0-progress 直接拒） | ✅ `UtilityPlanner.kt`（仅 netValue 排序，无 progress） |
| PlannerFactory 扩展点 | ✅ `runtime.PlannerFactory`（[runtime/extension.go:31](../runtime/extension.go)） | ✅ `spi.PlannerFactory` |
| Default factory 分派 | GOAP / Reactive 由 default 处理；HTN 必须用户提供 library | ✅ |
| **`Planner.Prune` 接口方法** | ❌ Wave E 已删 —— GOAP 内部仍有 prune（A* 节点扩展时），但 SPI 上不再要求。原先的 3 个 no-op 实现一并删掉 | ✅ |

**Wave E 决定**：`Planner.Prune` 在三个实现里只有 GOAP 真做，HTN/Reactive 都是空方法存根。这种"接口里有但所有非主实现都 no-op"的形状是设计味道——抽掉接口方法、把 prune 做成 GOAP 内部细节，让 SPI 仅暴露 `PlanToGoal / PlansToGoals / BestValuePlan` 三件事。

**反向无差距**：lynx reactive planner 的 progress 评分 + cost tie-break 仍比 embabel `UtilityPlanner` 严格更强（embabel 的纯 netValue 排序遇到一堆有 netValue 但都不解 goal precondition 的 action 会无限循环；lynx 直接拒绝出 plan、让 stuck-handler 介入）。

---

## 3. OODA tick loop / Concurrent — 等价

| 维度 | lynx | embabel |
|---|---|---|
| Tick 主循环 | [runtime/run.go:16](../runtime/run.go) `(*AgentProcess).run` | `AbstractAgentProcess.run()` |
| Sequential / Concurrent 双模 | ✅ `tickSimple` + [runtime/concurrent.go](../runtime/concurrent.go) `tickConcurrent` | ✅ Simple / Concurrent |
| Suspend → Resume → Continue 闭环 | ✅ `Platform.ResumeProcess` + `ContinueProcess` | ✅ |
| Termination scope（agent / action 双粒度） | ✅ —— Wave B 删 `TerminationScopeToolCall`（运行时仍有 `TerminateToolCall` 走 ctx cancel，不是 scope 枚举值） | ✅ |
| Stuck handler / Early termination | ✅ `core.StuckHandler` + `EarlyTerminationPolicy` ([core/early_termination.go](../core/early_termination.go)) | ✅ `StuckHandler.kt` |
| Defensive nil guard 清理 | ✅ Wave C 删 `core/process_context.go` 全部 `if pc == nil` 检查 —— `ProcessContext` 永远从 runtime 构造，nil 是程序员错误，不该靠 lib 兜底 | — |

---

## 4. Tool 模型 — 基线对齐；advanced policies 落地；冗余抽象削减

| 维度 | lynx | embabel |
|---|---|---|
| Tool 定义 | `core.AgentTool = chat.Tool`（[core/tool_group.go:91](../core/tool_group.go) 类型别名 — agent runtime 与 chat 包共享一套 tool 模型） | `Tool.Definition` |
| ToolGroup（按 role 聚合 + lazy load） | `core.ToolGroup` + `LazyToolGroup`（[core/tool_group.go:96](../core/tool_group.go)） | `ToolObject` |
| ToolGroupResolver | ✅ extension 接口（[core/tool_group.go:136](../core/tool_group.go)） | ✅ `spi.ToolGroupResolver` |
| ToolGroupRequirement | ✅ Role + TerminationScope —— Wave B 删 `RequiredToolNames`（与 ToolGroup 已隐含的 role 概念重复） | ✅ |
| **`ToolGroupDescription` 接口** | ❌ Wave B 已删 —— 之前 ToolGroup 上挂的 `Description / ChildToolUsageNotes / Permissions` 接口在 lynx runtime 里没有任何 reader，纯仿 embabel 抄过来又没接通 | ✅ `ToolGroupDescription.kt` |
| TerminationScope | ✅ `agent / action` 二元 —— Wave B 删 `TerminationScopeToolCall`（与 `TerminateToolCall` 走 ctx cancel 的运行时形态重复） | ✅ |
| ToolDecorator | ✅ extension capability ([core/extension.go:41](../core/extension.go)) | ✅ `ToolDecorator` SPI |
| ToolLoop runner | ✅ `chat.NewToolMiddleware()` + [core/process_context.go](../core/process_context.go) `ChatWithActionTools` | ✅ `ToolLoop.kt` + 多个 `ToolInjectionStrategy` |
| **One-shot-per-loop** | ✅ `toolpolicy.WithOnceOnly` + `WithLoopScope` ctx | ✅ `OneShotPerLoopTool.kt` |
| **Playbook tool（条件解锁）** | ✅ `toolpolicy.WithUnlock(tool, cond)`（含 reason 文本回写 LLM） | ✅ `agentic/playbook/PlaybookTool.kt` |
| Matryoshka / StateMachine / Replanning tool | ❌（niche，P3 备查） | ✅ |
| `worldStateDeterminer` 接口 | ❌ Wave E 已删 —— 单实现 `blackboardDeterminer`（[runtime/world_state_determiner.go:23](../runtime/world_state_determiner.go)），SPI 化没必要 | (embabel 内部也只有一种实现，以 class 暴露) |

**追平**：基础两件 advanced tool policies（OnceOnly + Unlock）皆是 `chat.CallableTool` decorator，可与 `hitl/tool.go` 的 HITL decorator 自由组合（嵌套），共享同一种 ctx-driven scope 机制。

---

## 5. HITL — 双层全追平

| 维度 | lynx | embabel |
|---|---|---|
| Typed Awaitable<P, R> | ✅ `hitl.TypedRequest` ([hitl/awaitable.go](../hitl/awaitable.go)) | ✅ `Awaitable<P, R>` |
| Confirmation 特化 | ✅ `hitl.NewConfirmation` | ✅ `ConfirmationRequest` |
| Suspend → Resume → Continue 闭环 | ✅ ([runtime/platform.go](../runtime/platform.go)) | ✅ |
| `AwaitDecider` | ✅ ([hitl/tool.go:66](../hitl/tool.go)) | ✅ `AwaitingTools.AwaitDecider` |
| `WithAwaiting / WithConfirmation / RequireType[T]` | ✅ ([hitl/tool.go:72/113/138](../hitl/tool.go)) | ✅ `Tool.withAwaiting / withConfirmation / requireType<T>` |
| `PauseError` sentinel + `HandlePause(pc, err)` | ✅ ([hitl/tool.go:23/47](../hitl/tool.go)) | ✅ `AwaitableResponseException` |
| 持久化 awaitable | ❌ in-memory only | ✅ context repository（前提：用持久化 backend） |

唯一保留缺口：持久化 awaitable —— 前提是 BlackboardFactory + ProcessRepository 都有持久化 backend。lynx in-memory only，按设计不在 framework 内做（[`PERSISTENCE.md`](./PERSISTENCE.md)）。

---

## 6. Extension 模型 — lynx 单入口更整洁；本轮再削

| 维度 | lynx | embabel |
|---|---|---|
| 注册入口 | **1 个**：`PlatformConfig.Extensions` + `ProcessOptions.Extensions` ([runtime/platform.go:42](../runtime/platform.go)) | 多个：每 SPI 一个 Spring `@Bean` 注入点 |
| 检测方式 | `core.Extension` + type assertion（`http.Pusher` 风格，[core/extension.go:14](../core/extension.go)） | Spring DI |
| **Core capability**（5 个） | `ActionInterceptor / ToolDecorator / AgentValidator / GoalApprover / BlackboardFactory`（全部在 [core/extension.go](../core/extension.go)） | 对应 5 个 SPI |
| **Runtime capability**（4 个） | `EventListener / PlannerFactory`（[runtime/extension.go:21/31](../runtime/extension.go)） + `IDGenerator`（[core/id_gen.go](../core/id_gen.go)）+ `ToolGroupResolver`（[core/tool_group.go:136](../core/tool_group.go)） | 对应 4 个 SPI |
| 总计 | **9** | **9 直接对应 + 4 LLM ops 专属**（LlmMessageSender / Streamer / ToolInjectionStrategy / EmbeddingService） |
| 重复检测 | ✅ 注册时按 Name dedup，panic ([runtime/dispatch.go](../runtime/dispatch.go)) | Spring `@Bean` 名级 |
| **Wave F docstring 削减** | ✅ 5 个 capability 接口 + `ActionQoS` + `ProcessContext` 多方法的多段落描述全部精简到要点为止 | — |

**embabel 的 4 个 LLM ops 扩展点** lynx 都不在 agent 内核做——LlmMessageSender / Streamer 是 `core/model/chat` 的领域；ToolInjectionStrategy 是 ToolLoop 策略；EmbeddingService 是 RAG 子系统的事。**这是分工分歧而非缺口**。

---

## 7. 事件 / 可观测 — 等价；本轮收敛到 14 个事件

| 维度 | lynx | embabel |
|---|---|---|
| Listener 接口 | `runtime.EventListener`（[runtime/extension.go:21](../runtime/extension.go)，runtime-tied 避免 core→event 循环） + `event.Listener` 简化版 | `AgenticEventListener` |
| **事件类型清单**（14 个） | `BaseEvent` 加 platform: `AgentDeployed / AgentUndeployed`（[platform.go](../event/platform.go)）；process: `ProcessCreated / ProcessCompleted / ProcessFailed / ProcessStuck / ProcessWaiting / ProcessKilled / ProcessTerminated`（7 — Wave B 删 `ProcessPaused`）（[process.go](../event/process.go)）；planning: `ReadyToPlan / PlanFormulated / ReplanRequested`（3）；execution: `ActionExecutionStart / ActionExecutionResult / GoalAchieved`（3）。**llm.go 全文件 Wave B 删掉**（`LLMRequestEvent / LLMResponseEvent` 在 lynx runtime 没人发，是 chat 包应负责的事）；`ObjectBoundEvent` Wave B 删（blackboard bind 自带跟踪） | 类似规模 + Spring 派发 |
| JSON 序列化 | ✅ 每个事件 `json.Marshaler`，统一走 `emit(envelope)` 一行（[event/event.go:73](../event/event.go)） | Jackson 默认 |
| 多播 | ✅ `event.Multicast`（[event/multicast.go](../event/multicast.go)） + per-process 多播 | Spring 派发 |
| OTel | ✅ `core.AgentTracer()` 由 `agent_process.startTickSpan` 调（[runtime/agent_process.go:226](../runtime/agent_process.go)） | Micrometer |

**Wave B 事件清理思路**：保留通用生命周期事件、删掉那些 runtime 从来没 publish 过的事件类型。一个事件类型挂在 framework 上若没有 emit 点，就是"看起来支持但用户写监听器收不到"的死接口。14 个事件覆盖了完整的 process / planning / action / goal / agent 生命周期。

---

## 8. MCP 集成 — 已闭合（保留）

| 子能力 | lynx | embabel |
|---|---|---|
| **客户端：消费外部 MCP server** | ✅ `lynx/mcp.NewProvider`（多源聚合 + 缓存 + 命名前缀 + list_changed 失效 + meta 透传 + sampling 反向 + 错误语义反转） | ✅ `SpringAiMcpToolFactory` |
| **服务端：暴露 lynx tool 为 MCP server** | ✅ `lynxmcp.RegisterTools(server, tools...)` | ✅ `McpToolExport.fromToolObject(...)` |
| **agent runtime 桥接** | ✅ `runtime.MCPToolGroupResolver`（[runtime/mcp.go](../runtime/mcp.go)） | ✅ McpToolFactory |
| **per-agent 自动暴露** | ✅ `runtime.AsMCPTool[In, Out](platform, agentName)` | ✅ `PerGoalMcpExportToolCallbackPublisher` |
| 多 goal 自动批量暴露 | ❌（lynx 典型 1 agent 1 goal） | ✅ |

参见 [`agent/examples/mcp-agent/main.go`](../examples/mcp-agent/main.go)。

---

## 9. Supervisor / Subagent — 基线已追平

| 维度 | lynx | embabel |
|---|---|---|
| 创建子 process | ✅ `Platform.CreateChildProcess` | ✅ |
| Budget 跨子 process 聚合 | ✅ `(*AgentProcess).Usage` 递归（[runtime/agent_process.go:125](../runtime/agent_process.go)） | ✅ |
| 子 agent → chat tool | ✅ `runtime.AsChatTool[In, Out]`（[runtime/subagent.go:43](../runtime/subagent.go)） + `AsChatToolFromAgent[In, Out]`（[runtime/subagent.go:58](../runtime/subagent.go)） | ✅ `Subagent.byName / ofClass / ofInstance / ofAnnotatedInstance` |
| 工厂路径数 | **2**（按名 + 直传 agent 实例） | **4**（多两条来自注解类） |
| 内部 wrapper 收敛 | ✅ 第八轮 Wave B 把原先 `subagentTool` / `mcpTool` 两份近似 `[In, Out]` 结构合并为一份 `agentTool[In, Out]` + `runProcessFunc[In]` 策略（[runtime/subagent.go:99](../runtime/subagent.go)），三个工厂传不同闭包；新增第四种调用形态成本是一个闭包 | embabel 4 路均独立类 |
| Awaitable 子 agent 优雅退化 | ✅ child Waiting 时返回 `{status:"waiting", agent, processId, awaitableId, prompt}` JSON（[runtime/subagent.go:241](../runtime/subagent.go) 共用一份） | ✅ `ProcessWaitingException` → tool result text |
| `runtime.AsMCPTool[In, Out]` | ✅ ([runtime/subagent.go:79](../runtime/subagent.go)) —— 与 `AsChatTool` 走同一个 `agentTool` 实现，仅策略不同；独立 process（无父 ctx 要求），同样支持 waiting graceful-degrade | ✅ `PerGoalMcpExportToolCallbackPublisher` |
| `RunSubagent` 注解 | ❌（DSL 路线无注解） | ✅ |

**lynx 2 工厂 vs embabel 4 工厂**：因为 lynx 没有注解类，所以 `ofClass / ofAnnotatedInstance` 两条工厂自然不存在；2 工厂（按名 + 直传）已覆盖 lynx 风格的全部用例。第八轮把这 2 + 1（MCP）共 3 条工厂收敛到同一个内部 wrapper —— DRY 受益。

---

## 10. Autonomy / Goal Ranking — 已追平

完整 LLM-driven 选 goal/agent 路径在 [`runtime/autonomy`](../runtime/autonomy)。

| 维度 | lynx | embabel |
|---|---|---|
| GoalApprover（拒掉特定 goal） | ✅ `core.GoalApprover` extension（[core/extension.go:63](../core/extension.go)） | ✅ `GoalChoiceApprover.kt` |
| 多 goal 同 process 选优（按 NetValue） | ✅ planner 默认行为（`SortByNetValueDesc`） | ✅ `Autonomy` |
| LLM 选 goal/agent | ✅ `autonomy.Autonomy.Choose / Run`（[autonomy.go:140/174](../runtime/autonomy/autonomy.go)） + `Ranker` SPI + `LLMRanker` 实现 | ✅ `Autonomy.choose` + `Ranker` SPI |
| Confidence cutoff | ✅ `AutonomyConfig.GoalConfidenceCutOff` → `ErrNoConfidentChoice`（[autonomy.go:69](../runtime/autonomy/autonomy.go)） | ✅ `AutonomyProperties.goalConfidenceCutOff` |
| AgentFilter / GoalFilter | ✅ ([autonomy.go:82/87](../runtime/autonomy/autonomy.go)) | ✅ Spring profile / role |
| 一键执行 | ✅ `Autonomy.Run` —— 装一个 per-process `targetGoalApprover` 把 planner 锁定到选中 goal（[autonomy.go:185](../runtime/autonomy/autonomy.go)） | ✅ `Autonomy.runWithChoice` |
| Plan-级 LLM 排序 | ✅ `autonomy.LLMPlanRanker` + `PlanRanker` 接口（[plan_ranker.go](../runtime/autonomy/plan_ranker.go) — Wave D 用 `slices.SortStableFunc + cmp.Compare` 重写排序） | ✅ `LlmRanker` |
| Multi-goal 顺序模式 | ❌（极少用例；上层多次 `Run` 已覆盖） | ✅ `GoalSelectionOptions.multiGoal` |

---

## 11. WorkflowBuilder / 多步组合模式 — 全档追平

| 维度 | lynx | embabel |
|---|---|---|
| Scatter-Gather | ✅ `workflow.ScatterGatherAgent[In, Element, Result]`（errgroup 并行 + `MaxConcurrency`） | ✅ `ScatterGather.kt` |
| RepeatUntil | ✅ `workflow.RepeatUntilAgent[In, Out]`（`CanRerun` + ComputedCondition + `History[Out]` + `MaxIterations`） | ✅ `RepeatUntil.kt` |
| RepeatUntilAcceptable | ✅ `workflow.RepeatUntilAcceptableAgent[In, Out]`（`Evaluator` 返回 `Feedback`，`AcceptableScore` 阈值默认 0.7） | ✅ `RepeatUntilAcceptable.kt` |
| Consensus | ✅ `workflow.ConsensusAgent[In, Element]`（ScatterGather 特化 + Key 投影 + 多数票） | ✅ `multimodel/ConsensusBuilder.kt` |
| Feedback 数据类型 | ✅ `workflow.Feedback`（含 `Acceptable(threshold)` 助手） | ✅ `Feedback.kt` |
| `WorkflowBuilder` 通用基类 | 不需要 —— builder 函数返回 `*core.Agent`，与普通 agent 同形 | ✅ `WorkflowBuilder.kt` |

---

## 12. 持久化 / 多进程 — 抽象等价、开箱仍弱（按设计）

| 维度 | lynx | embabel |
|---|---|---|
| Process registry | ✅ in-memory + `PruneTerminalProcesses` | ✅ + persistent backends |
| Blackboard | ✅ in-memory + Spawn + Clear + Protect | ✅ + Redis / DB |
| **`BlackboardFactory` 扩展点**（**唯一** blackboard SPI） | ✅ ([core/extension.go:75](../core/extension.go)) | ✅ `BlackboardProvider` |
| Context repository（跨 session）| ❌ —— 走 Blackboard 软持久化绕过（[`PERSISTENCE.md`](./PERSISTENCE.md)） | ✅ `ContextRepository` SPI |
| AgentProcessRepository | ❌（按设计不在 framework 内做） | ✅ `InMemoryAgentProcessRepository` |
| 实现指南 | ✅ [`PERSISTENCE.md`](./PERSISTENCE.md)（Redis / SQL / WAL 三策略） | ✅ Spring Data 集成 |

---

## 13. 注解 / classpath scan vs DSL Builder — 永久分歧

lynx：`agent.Builder`（[builder.go](../builder.go)，第八轮从 `dsl/builder.go` 拍平到根包），编译期类型安全 + IDE rename 100% 友好 + 启动时静态可推。embabel：`@Agent / @Action / @AchievesGoal / @LlmTool` + Spring scan。Go 没有 Spring AOT，反射注册不合 Go 哲学。**永久分歧**，请把它当哲学差异而非缺口。

---

## 14. 命名 / API ergonomics — lynx 顶层再收窄

| 维度 | lynx | embabel | 谁更好 |
|---|---|---|---|
| **Top-level surface** | **5 个 constructor** —— `agent.New / NewAction / NewCondition / GoalProducing / NewPlatform`（[agent.go](../agent.go)） + 几个 helper（`runtime.AsChatTool / AsChatToolFromAgent / AsMCPTool / MCPToolGroupResolver`）；第八轮 `agent.New(name)` 从转发 `dsl.New(name)` 改为直接构造根包 `*Builder`（[builder.go](../builder.go)） | 100+ Spring beans + DSL | **lynx** |
| Config 模式 | struct + `applyDefaults` | data class + sensible defaults | 等价 |
| 错误风格 | Wave F 统一为 `verb-noun-prefix`（如 `"run agent %q: ..."` / `"subagent %q: parse input: %w"`） | Kotlin exceptions | 各有所长 |
| 包数量（agent 框架） | 7（`core / plan / runtime / event / hitl / workflow / toolpolicy`）+ 根包 `agent`（`Builder` + 5 forwarders） + sibling `lynx/mcp`；第八轮删去原 `dsl/` 子包，`workflow` / `toolpolicy` 升到根 sibling | 12+ Maven modules | **lynx** |
| 类型层次 | 浅 | 深（Spring 抽象类堆叠） | **lynx** |
| 代码量 | **8715 LOC** non-test（30+ 文件，第七轮 8853 → 第八轮 8715） | **~30k+ LOC**（200+ 文件） | **lynx** |

---

## 15. 路线图

| 优先级 | 项目 | 改动量 | 说明 |
|---|---|---|---|
| ~~已闭合~~ | **P0–P3 主体全部落地** —— ToolLoop / MCP client+server / HTN / Reactive / Tool-级 HITL / Supervisor / Subagent waiting graceful-degrade / per-agent MCP 自动暴露 / WorkflowBuilder DSL 全套 / 持久化接入指南 / Autonomy + LLMRanker / LLMPlanRanker / Tool advanced policies (OnceOnly + Unlock) / AsChatToolFromAgent / RepeatUntilAcceptable + Consensus | — | ✅ |
| 用例驱动 | A2A 协议 / RAG 子模块 / Skills / Shell / Onnx | 大（独立子模块） | embabel 各占独立 Maven module；lynx 走 sibling repo（`lynx/rag` 已作 chat 中间件存在）路线，按用例评估 |
| 用例驱动 | Matryoshka / StateMachine / Replanning Tool | 中 | niche LLM 优化技巧，等具体业务场景 |
| 用例驱动 | Multi-goal 顺序模式 | 中 | 极少用例；上层多次 `Autonomy.Run` 已覆盖 |
| 用例驱动 | `runtime.ProcessRepository` / `AwaitableRepository` 接口实现 | 中 | 接口无 runtime hook 是 wallpaper；走 Blackboard 软持久化即可 |

**全部为 P3-niche 或独立子模块路线**，无 P0–P2 缺口。

---

## 15.5. 第八轮 fresh 对照新发现的小缺口（2026-05-09）

第八轮做完结构整理后又跑了一遍 embabel-agent-api / code 模块的细粒度对照。下面这些**在 embabel 里有专门 SPI、lynx 当前需要用户手写绕过**——按 ROI 排序，全部是小改动级（≤120 LOC）：

### P0 — 用户即可感知的功能缺失（**全部已闭合**，2026-05-09）

1. ✅ **`Goal.Export` 元数据** —— [`core/goal.go`](../core/goal.go) 新增 `Export *GoalExport` 字段 + `GoalExportFor[In](remote bool) *GoalExport` 类型化构造器。Reader 在 [`runtime/publish.go`](../runtime/publish.go)：`runtime.PublishAll(platform)` 批量产出 MCP-publish 风格的 `[]chat.CallableTool`（仅 `Remote=true` 的 goal），`runtime.AllAchievableTools(platform)` 产出 supervisor 风格的工具集（所有 `Export!=nil` 的 goal）。**字段 + reader 同时落，不再做死字段**。

2. ✅ **`PromptCondition`（LLM 驱动条件）** —— [`core/condition.go`](../core/condition.go) 新增 `NewPromptCondition(name, client, prompt, parser)` + `WithCost(...)` + `ParseYesNoDetermination` 默认 parser。LLM 错误降级为 `Unknown`（planner 视为"不满足"），不会拖垮 tick。

3. ✅ **`AchievableGoalsToolGroupFactory` 等价物** —— [`runtime/publish.go`](../runtime/publish.go) `runtime.AllAchievableTools(platform) []chat.CallableTool` 自动遍历所有 deployed agent 的 `Export!=nil` goal，构造 supervisor-flow tool。父 agent 的 LLM 不再需要手动逐个 `runtime.AsChatTool[…]()`。

落地总改动：约 350 LOC（含三件功能 + 测试）；提交 `<TBD>`。

### P1 — SPI 缺口

4. ❌ **`OperationScheduler` / 异步调度** —— **明确不做**。embabel 暴露给 action body：`scheduleAt(t)`、`scheduleEvery(d)`、cancel。lynx 现有原语已经能拼出所有真实用例：
   - "等 5 分钟再继续" → `time.AfterFunc(d, func() { platform.ResumeProcess(id, ...) })` 用户应用代码 3 行
   - "每 5 分钟轮询" → 应用层外层循环调 `Platform.RunAgent`
   - "noon tomorrow 跑" → cron-style，是应用调度器的活
   - retry-with-backoff → 已在 `ActionQoS.MaxAttempts/BaseDelay`
   
   加进 framework 要么是 `time.AfterFunc` 的薄封装（trivial / 不值），要么是真做调度器（cron / persistence，超出库的定位）。**SKIP**.

5. ❌ **`ModelProvider` / 多模型** —— **明确不做**。多模型抽象是 chat 包职责（`core/model/chat.Client` 是开放接口）；agent framework 不预设 LLM provider 槽位。多模型用例靠用户挂多个 `*chat.Client` + `core.ServiceProvider.Set("model:gpt-4", clientA)` 走字符串约定即可。**SKIP**.

6. ✅ **`OptimizingGoapPlanner`（剪枝 GOAP）** —— [`plan/planner/goap/relevance.go`](../plan/planner/goap/relevance.go) 新增 `relevantActions(actions, goal)`：STRIPS 回归 / 后向链式推理。从 goal 出发，递归收集"effects 命中需要集"的 action（包括其 preconditions 加入需要集再扩展），不动点扩展直到稳定。**永远开**——可证明安全（被排除的 action 在目标的 transitive 需要图里没有作用）。在 [`plan/planner/goap/astar.go`](../plan/planner/goap/astar.go) `PlanToGoal` 里 `goalReachable` 检查之前先调用，A* 搜的就是裁剪后的 action 集。

### P2 — DX/便捷性

7. ❌ **`TemplateRenderer`** —— **明确不做**。`core/model/chat` 已有 `PromptTemplate`，agent framework 不再包一层。**SKIP**.

8. ✅ **`Goal.Tags` + `Goal.Examples`** —— [`core/goal.go`](../core/goal.go) 加 `Tags []string` + `Examples []string` 字段；[`runtime/autonomy/llm_ranker.go`](../runtime/autonomy/llm_ranker.go) `buildUserPrompt` 现在在每个 candidate 行下追加缩进的 `tags: ...` / `examples:` 块（仅在非空时）。**字段 + reader 同时落**。

9. ❌ **`MatryoshkaTools` / 工具递归展开** —— **明确不做**。要么改 `chat.ToolMiddleware` 加 dynamic-tool-injection 钩子（跨包改动，需要 chat 团队拍板），要么做"伪 Matryoshka"（result JSON 里提工具名，LLM 看不到真工具）。前者复杂度高 / 真实用户少；后者不是真 Matryoshka。**真有用例时再走改 chat 的路径**。

### 已明确不做

- **注解驱动的 `@Action / @Agentic`** —— 永久分歧，第 13 节。
- **`OperationContext` 巨型 service surface** —— embabel 把 12+ 服务挂到 `OperationContext`（llmOps / asyncer / templateRenderer / conversationFactoryProvider …）；lynx `ProcessContext` 5 字段 + 4 hook 已够 ISP，多出来的服务走 `core.ServiceProvider`。
- **三值逻辑 / Condition 组合子 / `BindProtected`** —— 之前以为缺，本轮 fresh 对照确认 lynx 都已对齐（`core.Determination.Unknown` ✓ / `core.Not/Or/And` ✓ / `Blackboard.BindProtected` ✓）。
- **`OperationScheduler`（P1 #4）** —— 见上。
- **`ModelProvider` / 多模型（P1 #5）** —— 见上；chat 包职责。
- **`TemplateRenderer`（P2 #7）** —— chat 包已有。
- **`MatryoshkaTools`（P2 #9）** —— 见上。

### 落地状态

**P0**（2026-05-09）✅ 全部闭合 —— `Goal.Export` + `PromptCondition` + `AllAchievableTools/PublishAll`。约 700 LOC（含 reflection-based dynamic agent tool wrapper + 三套测试）。

**P1**（2026-05-09）：
- ✅ #6 `OptimizingGoapPlanner` —— STRIPS 回归剪枝，永远开
- ❌ #4 `OperationScheduler` —— 已有原语足够
- ❌ #5 `ModelProvider` —— chat 包职责

**P2**（2026-05-09）：
- ✅ #8 `Goal.Tags` + `Goal.Examples` + `LLMRanker` prompt 接通
- ❌ #7 `TemplateRenderer` —— chat 包已有
- ❌ #9 `MatryoshkaTools` —— 跨 chat 包边界，等真实用例

**总落地量**：约 850 LOC（含 P0 + P1 #6 + P2 #8 + 测试）。

---

## 16. 故意不要做的事（不是缺口）

- **注解 / classpath scan agent 注册** —— Go 心智模型不支持 magic
- **Spring DI 容器** —— `ServiceProvider` 已够；不再加重 DI
- **Sync / Async 双套 API** —— Go 的 `context.Context` + goroutine 已经覆盖；embabel 二分是 Spring artifact
- **AOT 模型分类 / SpEL 表达式条件** —— 过度工程
- **Megazord 注解（多 agent 合体）** —— 反射特化，做了也没人用
- **Personality / Shell 装饰** —— lynx 没这文化
- **MCP server-side resources/prompts 暴露** —— SDK 直接用，不再包一层
- **多 LLM provider 内置整合** —— `chat.Model` 是开放接口，用户自己挂
- **HTN 默认 task library** —— HTN 本来就需要领域知识；framework 不替用户假设
- **死字段 / 谓词糖** —— Wave B 已删；保留接口面最小，靠 caller 写 `d == core.True` / `_, ok := provider.Get(key)` 这类直白表达

---

## 17. 重构历史

### 第八轮（2026-05-09）的具体改动

| Wave | 主题 | 内容 |
|---|---|---|
| **A** | 拍平 dsl/ 子包 | `dsl/builder.go` → `agent/builder.go`（`Builder` 升根包，`agent.New(name)` 直接构造，不再走 dsl 转发）；`dsl/workflow/` → `agent/workflow/`；`dsl/toolpolicy/` → `agent/toolpolicy/`；删 `dsl/` 目录。`workflow` 内部从原先"穿透 `dsl.New(...).Build()`"改为直接 `core.NewAgent(core.AgentConfig{...})`，少一层封装 |
| **B** | 合并 subagent / mcp tool wrapper | `runtime/subagent.go` 中 `subagentTool[In, Out]` / `mcpTool[In, Out]` 两份近似 wrapper 合成一个 `agentTool[In, Out]` + `runProcessFunc[In]` 策略函数（`runAsChild` for `AsChatTool` / `runAsTopLevel` for `AsMCPTool`）；Definition / Metadata / Call / status-switch / output-extract 一份代码 |

**净影响**：non-test 代码 8853 → **8715 LOC**（≈ -1.6%），所有 examples + 测试包仍绿。功能面无任何收缩——纯做了一轮结构整理。新增第四种 agent-as-tool 调用形态从此只需要一个 `runProcessFunc[In]` 闭包。

提交：`6a1e7a1` `refactor(agent): drop dsl/ subpackage, unify subagent/MCP tool wrappers`。

### 第七轮（2026-05-08）的具体删除

| Wave | 主题 | 删除清单 |
|---|---|---|
| **B** | 死代码 | `Goal.{Tags, Examples, Export, OutputType}` + `ExportConfig`；`AgentConfig.{Provider, Opaque, DomainTypes, ToolGroupRequirements}` + 配套 builder setter；`ActionConfig/Metadata.{ReadOnly, Trigger}` + `TriggerType`；`EffectSpec.{Clone, Merge, Keys, Set}`；`MaxActionsPolicy` + `CompositePolicy`；`WriterOutputChannel` + `ChannelOutputChannel`；`Count[T]`；`ServiceProvider.{Has, Delete, Keys}`；`DefaultBinding` / `LastResultBinding` 旧别名；`Determination.{IsTrue, IsFalse, IsUnknown}`；`StuckHandlerFunc`；`ToolGroupDescription` 接口 + `Description` / `ChildToolUsageNotes` / `Permissions`；`ToolGroupRequirement.RequiredToolNames`；`TerminationScopeToolCall`；**整个 `event/llm.go`**（`LLMRequestEvent` / `LLMResponseEvent`）；`ObjectBoundEvent`；`ProcessPausedEvent`；`(*AgentProcess).AgentDef()` |
| **C** | 过度防御 | `core/process_context.go` 全部 `if pc == nil` 检查 |
| **D** | 现代 Go 标准库 | 25 行手写 quoter → `strconv.Quote`；hand-written insertion sort（`autonomy/plan_ranker.go`）→ `slices.SortStableFunc + cmp.Compare`；`plan/condition_world_state.go` 排序 → `slices.Sorted(maps.Keys(...))` |
| **E** | 单实现接口去虚 | `Planner.Prune` + 3 no-op 实现；`worldStateDeterminer` 接口（仅 `blackboardDeterminer` 一实现） |
| **F** | docstring 与错误风格 | `core/extension.go` 5 个 capability 接口 + `ActionQoS` + `ProcessContext` 多方法的多段落描述精简到要点；错误信息统一 verb-noun-prefix 风格 |

**净影响**：non-test 代码从 ≈9512 LOC → **8853 LOC**（≈ -7%），所有 examples + 测试包仍绿。功能面无任何收缩——全部为内部清理。

---

## 18. 一句话总结

第八轮没有新增功能、也没有进一步删字段。**纯做结构整理**：消除 `dsl/` 子包（`Builder` 升根 + `workflow` / `toolpolicy` 升 sibling），合并 `subagentTool` / `mcpTool` 两份近似 wrapper 为一个 `agentTool[In, Out]` + 策略函数。non-test 代码 8853 → **8715 LOC**（-138）。

同时跑了一次 fresh 对照（§15.5），列出了 9 个 embabel 有而 lynx 仍缺的小级别能力（`Goal.Export` / `PromptCondition` / `AchievableGoalsToolGroupFactory` / `OperationScheduler` / `ModelProvider` / `OptimizingGoapPlanner` / `TemplateRenderer` / `Goal.Tags+Examples` / `MatryoshkaTools`），全部 ≤120 LOC 级，按 ROI 列了落地顺序——**没有 P0/P1/P2 的硬缺口**，全部为"用户可手写绕过 + 加上后体验更好"级别。

代码量 **8715 LOC** vs embabel 的 **~30k+ LOC**，同样的领域模型，3-4 倍的密度差距来自 Spring 工程基础设施 + 内置 RAG/Onnx/Shell 等独立模块。两个项目都正确——只是站在 Go 与 Kotlin/Spring 哲学的两端。
