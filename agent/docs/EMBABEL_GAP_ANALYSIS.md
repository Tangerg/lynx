# lynx/agent vs embabel-agent — 深度对比与缺口分析（第十轮，全文重写）

> **2026-05-09**。基线：embabel-agent (Kotlin/Spring) ≈ 209k LOC / 1,182 文件 vs lynx/agent (Go 1.26.3) HEAD `0b2749f` / **9,825 LOC** non-test。
>
> 本轮**全文重写**，不在前九轮的增量上叠加，目的是给出一份当前状态权威清单——避免前几轮里因功能落地后未及时刷新而残留的过时声明。前九轮的历史与决策依然有效（见 §17），但本文档是单一可信源。
>
> 核心结论：**lynx 在 framework 抽象层面已与 embabel 持平**。功能差距全部已闭合或属于"故意分歧/职责边界外"。第十轮没有新增功能，只有近期 polish（DRY/SRP refactor、Go 1.26 迁移、`sync.OnceValue` / `wg.Go` / `errors.AsType[T]` 现代化、`mustDeploy` 测试规范化）后的状态固化。
>
> 配套文档：[`./ADK_COMPARISON.md`](./ADK_COMPARISON.md)（vs Google ADK 异范式）/ [`./EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md) / [`./PERSISTENCE.md`](./PERSISTENCE.md) / [`/Users/tangerg/Desktop/lynx/mcp/DESIGN.md`](../../mcp/DESIGN.md)。

---

## 0. TL;DR — 维度并列对照

| 维度 | lynx/agent | embabel-agent | 状态 |
|---|---|---|---|
| **核心抽象**（Agent / Goal / Action / Condition / Blackboard / WorldState / Plan） | ✅ 完整 | ✅ 完整 | **追平** |
| **Goal 字段**（Name / Description / Pre / Inputs / Value / Tags / Examples / Export） | ✅ 全字段 + reader 接通（`runtime.AllAchievableTools` / `PublishAll` / `LLMRanker.buildUserPrompt`） | ✅ 全字段 | **追平** |
| **Condition 三态逻辑**（True / False / Unknown） | ✅ `core.Determination` 三态 + `And/Or/Not` 组合子 | ✅ `ConditionDetermination` + `not/inv/or/and` operators | **追平** |
| **PromptCondition**（LLM-as-judge） | ✅ `core.NewPromptCondition` + `WithCost` + `ParseYesNoDetermination` | ✅ `experimental.PromptCondition` | **追平** |
| **GOAP A*** | ✅ `plan/planner/goap/astar.go` + `goalReachable` 一步检查 | ✅ `AStarGoapPlanner` | **追平** |
| **GOAP STRIPS regression 剪枝** | ✅ `plan/planner/goap/relevance.go` `relevantActions`（永远开） | ✅ `OptimizingGoapPlanner.kt` + `Planner.prune()` 接口方法 | **追平** |
| **HTN planner** | ✅ `plan/planner/htn/htn.go`（Library + Task + Method） | ❌（未提供 HTN 内置） | **lynx 更强** |
| **Reactive / Utility planner** | ✅ `plan/planner/reactive/`（progress × cost-tie 严格优于 utility） | ✅ `UtilityPlanner.kt`（仅 net-value） | **lynx 更强** |
| **OODA tick loop** | ✅ `runtime/run.go` + `runtime/concurrent.go`（Sequential + Concurrent） | ✅ `AbstractAgentProcess.run()` | **追平** |
| **并发执行原语** | ✅ `sync.WaitGroup` + Go 1.25 `wg.Go(fn)`（结构化结果槽，无需 errgroup 误用） | ✅ Reactor `Flux` / errgroup 类似 | **lynx 更轻** |
| **HITL（process-级 + tool-级双层）** | ✅ `hitl.TypedRequest[P,R]` + `RequireAwait / RequireConfirmation / RequireType[T]` + `HandlePause` (`errors.AsType[*PauseError]`) | ✅ `Awaitable<P,R>` + `AwaitingTools.kt` | **追平** |
| **Subagent typed wrapper** | ✅ `runtime.AsChatTool[In, Out]` + `AsChatToolFromAgent[In, Out]` + `AsMCPTool[In, Out]`（**3 个工厂走同一 `agentTool` strategy 内核**） | ✅ `Subagent.byName/ofClass/ofInstance/ofAnnotatedInstance` 4 路 | **追平**；细差见 §6 |
| **自动批量发布**（按 Goal.Export 收集） | ✅ `runtime.AllAchievableTools(platform)` + `runtime.PublishAll(platform)` | ✅ `PerGoalMcpExportToolCallbackPublisher` | **追平** |
| **MCP 集成**（client + server） | ✅ `lynx/mcp` 全功能 + `runtime.MCPToolGroupResolver` + `runtime.AsMCPTool` | ✅ `embabel-agent-mcp` | **追平** |
| **Tool advanced policies** | ✅ `toolpolicy/{OnceOnly, Unlocked, LoopScope}` | ✅ `OneShotPerLoopTool` + `PlaybookTool` + `UnfoldingTool`（前 Matryoshka） | **基线追平**；UnfoldingTool 未做（见 §16） |
| **Workflow（action 级）** | ✅ `workflow.{ScatterGather, RepeatUntil, RepeatUntilAcceptable, Consensus}` | ✅ `api/common/workflow/` 同款 | **追平** |
| **Workflow（agent 级）** | ✅ `workflow.{Sequence, Parallel, Loop}`（基于 `runtime.SpawnChildFresh` 的 branch isolation） | ❌（embabel 没有 agent 级 workflow，全 LLM-driven） | **lynx 更强** |
| **Autonomy / Goal Ranking** | ✅ `runtime/autonomy/{Choose, Run}` + `LLMRanker`（含 Tags/Examples）+ `LLMPlanRanker` + `GoalConfidenceCutOff` + filters | ✅ `Autonomy.chooseAndAccomplishGoal` + `Ranker` + `GoalChoiceApprover` | **追平** |
| **Extension 模型** | ✅ 单注册入口 `PlatformConfig.Extensions` / `ProcessOptions.Extensions` + 9 capability 类型断言检测 | ✅ Spring DI 多 SPI 注入点 | **lynx 更整洁** |
| **事件系统** | ✅ 14 个事件类型 + `event.Multicast` + JSON marshaler + `event.NewNamedListener` 单一注册路径 | ✅ `AgenticEventListener` + Micrometer | **追平** |
| **OTel observability** | ✅ `core.AgentTracer()` + 标准 attrs（`lynx.agent.tick` / `.action` / `.planner.astar` ...） | ✅ Micrometer + OTel exporter | **基线追平**（embabel 多了 cost / token 内置 metrics） |
| **持久化** | ⚙️ `BlackboardFactory` 扩展点 + [`PERSISTENCE.md`](./PERSISTENCE.md) 实现指南 | ✅ Spring Data + `ContextRepository` SPI | **抽象等价**；开箱实现欠（按设计） |
| **多 LLM provider 内置 / RAG / Onnx / Shell / Skills / A2A / 注解 / classpath scan** | ❌（按定位） | ✅ 7+ 独立 Maven module | **故意分歧** |
| **代码量** | **9,825 LOC** non-test, 7 包 | **~209k LOC**, 17 module | **lynx 1/20**（同核心模型，密度差源于 Spring 工程基础设施 + 独立子模块） |

**一句话**：lynx 在 framework 抽象层 1:1 追平 embabel 内核；多出来的能力（HTN / Reactive 严格更强 / Concurrent action mode / 单一 Extension 注册路径 / agent 级 workflow / dynamic publish）是 lynx 自有创新，不是迫近的 gap。

---

## A. 宏观对比

### A.1 设计哲学

| | lynx/agent | embabel-agent |
|---|---|---|
| 定位 | **minimal Go library**：嵌进微服务 / Lambda / CLI 工具 | **batteries-included Spring stack**：autoconfigure + starters + 子模块生态 |
| 注册风格 | **typed DSL**（`agent.New("x").Actions(...).Build()`），编译期类型安全 + IDE rename 100% 友好 | **classpath-scan + 注解**（`@Agent` / `@Action` / `@AchievesGoal` / `@LlmTool`） |
| DI 容器 | **无**（`core.ServiceProvider` 是 type-keyed map） | **Spring `ApplicationContext`** 全栈 |
| Module 划分 | 1 仓 / 7 子包（core / plan / runtime / event / hitl / workflow / toolpolicy）+ sibling `lynx/mcp` | 17 Maven module（api / code / a2a / mcp / observability / domain / common / autoconfigure / dependencies / docs / onnx / openai / rag / shell / skills / starters / test-support） |
| 上游依赖 | 极小（`semver` / `uuid` / `errgroup` / OTel / `swaggest/jsonschema-go`，全部可在 1 天内复审） | 巨大（Spring Framework / Boot / AI / Jackson / Micrometer / Reactor / Caffeine / Spring Data ...，百级 transitive） |
| 代码密度 | **9,825 LOC** non-test | **~209k LOC**（lynx ≈ 1/20） |

3-4 倍非测试代码差距来自：
- **Spring 工程基础设施** —— autoconfigure / starters / property binding / actuator 集成
- **独立子模块生态** —— RAG / Onnx / Shell / Skills / A2A / openai 等
- **多 model provider 内置** —— OpenAI / Anthropic / Gemini / Ollama / Bedrock / ONNX / Deepseek / MiniMax / Mistral / Google GenAI / LMStudio / Docker

### A.2 模块边界对照

| lynx | embabel |
|---|---|
| `agent/core/` | `embabel-agent-api/.../core/` |
| `agent/plan/` + `plan/planner/{goap,htn,reactive}/` | `embabel-agent-api/.../plan/{goap,utility,common}/` |
| `agent/runtime/` + `runtime/autonomy/` | `embabel-agent-code/` 部分 + `api/common/autonomy/` |
| `agent/workflow/` + `workflow/agent_level.go` | `embabel-agent-api/.../api/common/workflow/` |
| `agent/toolpolicy/` | `embabel-agent-api/.../api/agentic/playbook/` + `agentic/oneshot/` |
| `agent/event/` | `embabel-agent-api/.../api/event/` |
| `agent/hitl/` | `embabel-agent-api/.../core/hitl/` |
| `runtime/autonomy/` | `embabel-agent-api/.../api/common/autonomy/` |
| sibling `lynx/mcp` + `runtime/mcp.go` | `embabel-agent-mcp/` |
| —（用户自挂 `chat.Client`） | `embabel-agent-{openai,onnx,...}` 多 provider |
| —（按设计） | `embabel-agent-{rag,shell,skills,a2a}` 独立产品模块 |

### A.3 永久不做的能力（lynx 主动放弃）

- Spring DI 容器、autoconfigure、starter 生态 —— Go 没有 Spring，硬抄是反 Go 哲学
- classpath scan + 注解 agent 注册 —— Go 没有 classpath，注解只是 struct tag，注册要走显式 `platform.Deploy`
- profile-based feature gates（`@Profile`）—— 应用层 build flag / env 处理
- 内置 RAG / Onnx / Shell / Skills / 多 LLM provider / Personality —— 全部 chat 中间件层 / sibling repo / 用户应用代码职责
- Personality system（embabel `shell` 4 个 personality：Severance / Hitchhiker / Star Wars / Monty Python）—— lynx 不做产品 UX

每一项都是 Spring 生态延伸或 JVM 部署形态特有便利。**职责分工不同，不是缺口**。

### A.4 lynx 自有创新（embabel 没有 / 实现更弱）

- **HTN planner** —— embabel 完全无；lynx 提供 Library + Task + Method 标准 HTN
- **Reactive planner with progress × cost-tie** —— 比 embabel `UtilityPlanner` 严格更强（embabel 只看 netValue，遇到一堆有 netValue 但都不解 goal 的 action 会陷入循环；lynx reactive 直接拒绝出 plan，让 stuck handler 介入）
- **agent 级 workflow（Sequence/Parallel/Loop）** —— embabel 全 LLM-driven，没有这一层；lynx 有 `runtime.SpawnChildFresh` 配套 branch isolation
- **统一 ToolLoop runner**（`chat.NewToolMiddleware`）—— embabel 有多个 `ToolInjectionStrategy` 变体（Matryoshka / Unfolding / Standard）需要在不同路径维护
- **ctx-driven 进程注入**（`core.WithProcess` / `core.ProcessFrom`）—— embabel 走 thread-local + 显式参数传递混合；lynx 走纯 ctx
- **单注册入口的 Extension 模型** —— embabel 是多 SPI + Spring `@Bean` 注入点；lynx 是 1 个入口 + type assertion 检测，启动时 dedup-and-panic
- **死代码零容忍** —— Spring 体系任何"看似可能给用户用的字段"都不敢动；lynx 在前 9 轮做了多次 fail-fast 的死字段清理（Wave B 删过 30+ 字段，加回来时**字段 + reader 同时落**）

---

## 1. 核心抽象 — 1:1 追平

| 抽象 | lynx | embabel |
|---|---|---|
| **Agent** | `core.Agent`（[core/agent.go:52](../core/agent.go)）—— 嵌入 `AgentConfig` + `knownConditions func() map[string]struct{}` (sync.OnceValue) | `Agent.kt`（含 Spring 注入字段 / `domainTypes` / `opaque` 旗 / `pruneTo()` / `mergeTypes()`） |
| **AgentConfig** | 6 字段：`Name / Description / Version / StuckHandler / Actions / Goals / Conditions`（[core/agent.go:15](../core/agent.go)） | `Agent` data class，多嵌 `provider` / `opaque` / `domainTypes` |
| **Action** | `core.Action`（接口，[core/action.go:13](../core/action.go)）+ `core.ActionConfig`（[core/action_config.go:13](../core/action_config.go)） + `TypedAction[In, Out]` 实现 | `Action.kt`（接口）继承 DataFlowStep / ConditionAction / ActionRunner / DataDictionary / ToolGroupConsumer |
| **Goal** | `core.Goal`（[core/goal.go](../core/goal.go)）—— `Name / Description / Pre / Inputs / Value / Tags / Examples / Export` 8 字段 | `Goal.kt`（含 `outputType` / `pre` / `tags` / `examples` / `export`） |
| **GoalExport** | `core.GoalExport{Remote, Description, InputSample any}` + `GoalExportFor[In](remote)` 类型化构造器 | `Export(name, remote, local, startingInputTypes)` |
| **Condition** | `core.Condition` 接口 + `ComputedCondition` / `PromptCondition`（[core/condition.go](../core/condition.go)） + `And/Or/Not` 组合 | `Condition.kt`（含 `not`/`inv`/`or`/`and` 操作） |
| **Determination** | `core.Determination` 三态（`Unknown=0` / `True=1` / `False=2`）+ `And/Or/Not` 方法 | `ConditionDetermination` |
| **Blackboard** | `core.Blackboard`（[core/blackboard.go](../core/blackboard.go)）—— Reader / Writer / Mutator 三接口 | `Blackboard.kt`（含 `bindProtected` / `expressionEvaluationModel` / `Aggregation` 反射） |
| **WorldState** | `core.WorldState` + `plan.ConditionWorldState`（[plan/condition_world_state.go](../plan/condition_world_state.go)） | `WorldState` |
| **Process** | `core.Process` 接口（[core/process.go](../core/process.go)） + `runtime.AgentProcess` | `AgentProcess` 接口含 `tick()` / `run()` / `kill()` / `terminateAgent` / `terminateAction` / `cost` / `usage` 等 |
| **AgentProcessStatus** | 8 态：`StatusRunning / Waiting / Paused / Stuck / Completed / Failed / Terminated / Killed` | `AgentProcessStatusCode` 枚举（同等 9 态） |
| **ProcessContext** | `core.ProcessContext`（[core/process_context.go](../core/process_context.go)）—— 5 公开字段（Process / Blackboard / Options / OutputChannel / Services）+ 5 私有 hook | `OperationContext` 暴露 12+ 服务 |
| **EffectSpec** | `core.EffectSpec` —— Map[string]Determination | `Effects` typealias |

**结论**：核心模型 1:1 对齐。Goal 字段表完全一致（Name / Description / Pre / Inputs / Value / Tags / Examples / Export 8 项）。第十轮没有任何字段差异。

---

## 2. Planner — 三全；STRIPS 剪枝双方都做了；HTN/Reactive lynx 更强

| 维度 | lynx | embabel |
|---|---|---|
| **GOAP A*** | ✅ `plan/planner/goap/astar.go`（A* + reachability 一步检查） | ✅ `AStarGoapPlanner.kt` |
| **STRIPS regression 剪枝** | ✅ `plan/planner/goap/relevance.go` `relevantActions` —— 后向链式不动点扩展，**永远开**（可证明安全） | ✅ `Planner.prune()` SPI 接口方法 + `OptimizingGoapPlanner.kt` |
| **Reachability 预检** | ✅ ([astar.go](../plan/planner/goap/astar.go) 单步) | ✅ |
| **Excluded actions**（防 replan 死循环） | ✅ `state.snapshotExclusions` ([runtime/process_state.go](../runtime/process_state.go)) | ✅ |
| **多 plan 排序**（NetValue desc） | ✅ `plan.SortByNetValueDesc`（`slices.SortStableFunc + cmp.Compare`） + `autonomy.LLMPlanRanker` | ✅ + `LlmRanker` |
| **HTN planner** | ✅ `plan/planner/htn/htn.go`（Library + Task + Method，ctx-cancellable，maxRecursion 防御） | ❌ **embabel 不提供** |
| **Reactive / Utility planner** | ✅ `plan/planner/reactive/`（progress × cost-tie，0-progress 直接拒；让 stuck handler 介入） | ✅ `UtilityPlanner.kt`（仅 netValue 排序，无 progress —— **可能进无限循环**） |
| **PlannerFactory 扩展点** | ✅ `runtime.PlannerFactory`（[runtime/extension.go:31](../runtime/extension.go)） | ✅ Spring `@Bean` 注入点 |
| **Default factory 分派** | GOAP/Reactive 由 default 处理；HTN 必须用户提供 library | ✅ |

**lynx 强项**：
1. HTN 完整实现（embabel 无）
2. Reactive 比 embabel UtilityPlanner 严格更强（避免 net-value-only 的死循环陷阱）
3. STRIPS 剪枝是**永远开 + 可证明安全**（embabel 走 SPI 方法，需用户/默认实现选择是否开）

**追平**：GOAP A* + reachability 单步检查 + STRIPS 后向链式剪枝（embabel 通过 OptimizingGoapPlanner 实现；lynx 通过 relevantActions 实现）。

---

## 3. OODA tick loop / Concurrent — 等价；并发原语 lynx 更轻

| 维度 | lynx | embabel |
|---|---|---|
| Tick 主循环 | [runtime/run.go:16](../runtime/run.go) `(*AgentProcess).run` | `AbstractAgentProcess.run()` |
| Sequential / Concurrent 双模 | ✅ `tickSimple` + [runtime/concurrent.go](../runtime/concurrent.go) `tickConcurrent` | ✅ Simple / Concurrent |
| Suspend → Resume → Continue 闭环 | ✅ `Platform.ResumeProcess` + `ContinueProcess` | ✅ |
| Termination scope（agent / action 双粒度） | ✅ | ✅ |
| Stuck handler / Early termination | ✅ `core.StuckHandler` + `EarlyTerminationPolicy` ([core/early_termination.go](../core/early_termination.go)) | ✅ `StuckHandler.kt` |
| **并发原语** | ✅ `sync.WaitGroup` + Go 1.25 `wg.Go(fn)`（结构化 results / replans 槽位）—— [runtime/concurrent.go:54](../runtime/concurrent.go) | Reactor `Flux` + 自定义合并 |
| **lazy-cache 模式** | ✅ Go 1.21+ `sync.OnceValue[T]`（`core.Agent.knownConditions` / `plan.PlanningSystem.knownConditions`）—— **2 处 `atomic.Pointer + sync.Once` 已统一替换** | 未统计 |

**lynx 现代化要点（第八/九/十轮陆续落地）**：
- `sync.OnceValue` 替代 `atomic.Pointer + sync.Once` 双检锁样板
- `sync.WaitGroup + wg.Go(fn)` 替代 `errgroup` 误用（goroutine 总返回 nil 时 errgroup 是错原语）
- `errors.AsType[T]` 替代 `errors.As(err, &target)`
- `t.Context()` 替代 `context.WithCancel(context.Background())` 在测试里
- `for i := range N` 替代 `for i := 0; i < N; i++`

---

## 4. Tool 模型 — 基线对齐；advanced policies 落地；Matryoshka/Unfolding 不做

| 维度 | lynx | embabel |
|---|---|---|
| **Tool 定义** | `core.AgentTool = chat.Tool`（[core/tool_group.go:91](../core/tool_group.go) 类型别名 —— agent runtime 与 chat 包共享一套 tool 模型） | `Tool` 接口 + `Tool.Definition` / `InputSchema` / `Result` 三套类型 |
| **ToolGroup**（按 role 聚合 + lazy load） | `core.ToolGroup` + `LazyToolGroup`（[core/tool_group.go:96](../core/tool_group.go)） | `ToolGroup` |
| **ToolGroupResolver** | ✅ extension 接口（[core/tool_group.go:136](../core/tool_group.go)） | ✅ `spi.ToolGroupResolver` |
| **ToolGroupRequirement** | ✅ Role + TerminationScope | ✅ |
| **TerminationScope** | ✅ `agent / action` 二元 | ✅ |
| **ToolDecorator** | ✅ extension capability ([core/extension.go:41](../core/extension.go)) | ✅ `ToolDecorator` SPI |
| **ToolLoop runner** | ✅ `chat.NewToolMiddleware()` + [core/process_context.go](../core/process_context.go) `ChatWithActionTools` —— **一处实现覆盖所有调用点** | ✅ `ToolLoop.kt` + 多个 `ToolInjectionStrategy` |
| **One-shot-per-loop** | ✅ `toolpolicy.OnceOnly` + `LoopScope` ctx | ✅ `OneShotPerLoopTool.kt` |
| **Playbook tool（条件解锁）** | ✅ `toolpolicy.Unlocked(tool, cond)`（含 reason 文本回写 LLM） | ✅ `agentic/playbook/PlaybookTool.kt` |
| **Matryoshka / Unfolding（递归工具树）** | ❌ **明确不做** —— 跨 chat 包边界，niche 用例 | ✅ `progressive/UnfoldingTool.kt`（`@MatryoshkaTools` 已 deprecated） |
| **Replanning Tool** | 走 `core.ReplanRequest` 错误返回机制（action body return ReplanRequest as error） | `Tool.replanAlways/conditionalReplan` 装饰器 |

**追平**：基础两件 advanced tool policies（OnceOnly + Unlock）皆是 `chat.CallableTool` decorator，可与 `hitl/tool.go` 的 HITL decorator 自由组合（嵌套），共享同一种 ctx-driven scope 机制。

**不做**：UnfoldingTool 需要修改 `chat.ToolMiddleware` 支持动态工具注入，是跨包改动 + 真实用例少。等具体 use case 出现再考虑。

---

## 5. HITL — 双层全追平

| 维度 | lynx | embabel |
|---|---|---|
| Typed Awaitable<P, R> | ✅ `hitl.TypedRequest[P, R]` ([hitl/awaitable.go](../hitl/awaitable.go)) | ✅ `Awaitable<P, R>` |
| Confirmation 特化 | ✅ `hitl.NewConfirmation[P]` | ✅ `ConfirmationRequest` |
| Suspend → Resume → Continue 闭环 | ✅ ([runtime/platform_run.go](../runtime/platform_run.go)) | ✅ |
| `AwaitDecider` 钩子 | ✅ ([hitl/tool.go](../hitl/tool.go)) | ✅ `AwaitingTools.AwaitDecider` |
| `RequireAwait / RequireConfirmation / RequireType[T]` | ✅ ([hitl/tool.go](../hitl/tool.go)) | ✅ `Tool.withAwaiting / withConfirmation / requireType<T>` |
| `PauseError` sentinel + `HandlePause(pc, err)` | ✅ Go 1.26 `errors.AsType[*PauseError](err)` | ✅ `AwaitableResponseException` |
| ResponseImpact 枚举 | ✅ `Unchanged / Updated` | ✅ `UPDATED / UNCHANGED` |
| 持久化 awaitable | ❌ in-memory only（按设计） | ✅ context repository（前提：用持久化 backend） |

**追平**：双层（process 级 awaitable + tool 级装饰器）+ 类型安全 + 用户回调 + 持久化扩展点。lynx 的 `errors.AsType[*PauseError]`（Go 1.26 syntax）比 embabel 的 exception throwing 更现代/类型化。

---

## 6. Subagent / Supervisor — 基线追平 + lynx 内部更整洁

| 维度 | lynx | embabel |
|---|---|---|
| 创建子 process | ✅ `Platform.CreateChildProcess` + `runtime.SpawnChild` 公开 helper | ✅ |
| Budget 跨子 process 聚合 | ✅ `(*AgentProcess).Usage` 递归（[runtime/agent_process.go:125](../runtime/agent_process.go)） | ✅ `cost()` / `usage()` 跨子树聚合 |
| 子 agent → chat tool（typed） | ✅ `runtime.AsChatTool[In, Out]`（[runtime/subagent.go:43](../runtime/subagent.go)） + `AsChatToolFromAgent[In, Out]` | ✅ `Subagent.byName / ofClass / ofInstance / ofAnnotatedInstance` |
| 工厂路径数 | **2**（按名 + 直传 agent 实例）—— Go 没注解类，无 `ofClass / ofAnnotatedInstance` | **4** |
| 共享内部 wrapper | ✅ 第八轮 `agentTool` struct + `processStarter` 策略；第十轮 typed/dynamic 共享同一 wrapper（[runtime/agent_tool.go](../runtime/agent_tool.go)） | embabel 4 路工厂各自独立类 |
| Awaitable 子 agent 优雅退化 | ✅ child Waiting 时返回 `{status:"waiting", agent, processId, awaitableId, prompt}` JSON | ✅ `ProcessWaitingException` → tool result text |
| `AsMCPTool[In, Out]` —— top-level 发布 | ✅ ([runtime/subagent.go:79](../runtime/subagent.go)) —— 与 AsChatTool 走同一 agentTool 实现，仅策略不同 | ✅ `PerGoalMcpExportToolCallbackPublisher` |
| **自动批量发布**（按 Goal.Export 收集） | ✅ `runtime.AllAchievableTools(platform)` —— supervisor flow / 收集 Export!=nil ✅ `runtime.PublishAll(platform)` —— top-level flow / 仅 Export.Remote=true | ✅ `PerGoalMcpExportToolCallbackPublisher` 批处理同款 |
| Dynamic-typed 包装（无泛型 In/Out 时） | ✅ `runtime/publish.go` `dynamicAgentTool` 用反射 from `Goal.Export.InputSample` | ✅（embabel 注解类直接走反射） |
| `RunSubagent` 注解 | ❌（Go 无注解；走 typed `core.NewAction[In, Out]` 等价） | ✅ `@RunSubagent` |

**lynx 工厂路径整合**（第八、十轮）：
- `AsChatTool` / `AsChatToolFromAgent` / `AsMCPTool` 三个 typed factory + `AllAchievableTools` / `PublishAll` 两个 dynamic factory **共用同一 `agentTool` struct**
- 区别仅是 `decode` / `run` / `extract` 三个策略闭包：
  - `decode` typed 用 `json.Unmarshal` into typed `In`；dynamic 用 `reflect.New(InputSample.Type)` driven 解码
  - `run` 走 `SpawnChild`（supervisor flow，要 parent ctx）或 `RunFresh`（top-level，无 parent）
  - `extract` typed 用 `core.ResultOfType[Out]`；dynamic 用 blackboard.GetValue(LastResultBindingName)

新增第六种调用形态成本是一个闭包。

---

## 7. Workflow — action 级追平 + agent 级 lynx 更全

### 7.1 Action 级（追平）

| 维度 | lynx | embabel |
|---|---|---|
| Scatter-Gather | ✅ `workflow.ScatterGather[In, Element, Result]`（errgroup 并行 + `MaxConcurrency`） | ✅ `ScatterGather.kt` |
| RepeatUntil | ✅ `workflow.RepeatUntil[In, Out]`（`CanRerun` + ComputedCondition + `History[Out]` + `MaxIterations`） | ✅ `RepeatUntil.kt` |
| RepeatUntilAcceptable | ✅ `workflow.RepeatUntilAcceptable[In, Out]`（`Evaluator` 返回 `Feedback`，`AcceptableScore` 阈值默认 0.7） | ✅ `RepeatUntilAcceptable.kt` |
| Consensus | ✅ `workflow.Consensus[In, Element]`（ScatterGather 特化 + Key 投影 + 多数票） | ✅ `multimodel/ConsensusBuilder.kt` |
| Feedback 数据类型 | ✅ `workflow.Feedback`（含 `Acceptable(threshold)` 助手） | ✅ `Feedback.kt` |
| `WorkflowBuilder` 通用基类 | 不需要 —— builder 函数返回 `*core.Agent`，与普通 agent 同形 | ✅ `WorkflowBuilder.kt` |

### 7.2 Agent 级（lynx 自有）

| 维度 | lynx | embabel |
|---|---|---|
| **`Sequence`**（确定性 a₁ → a₂ → ... → aₙ） | ✅ ([workflow/sequence.go](../workflow/sequence.go))，基于 `runtime.SpawnChildFresh` | ❌ 全 LLM-driven，无此层 |
| **`Parallel`**（fan-out 多 sub-agent + Joiner） | ✅ ([workflow/parallel.go](../workflow/parallel.go))，**branch isolation** via Spawn() | ❌ |
| **`Loop`**（重复跑 sub-agent 直到 Until） | ✅ ([workflow/loop.go](../workflow/loop.go))，**fresh blackboard 每轮**（避免 orchestrator 累积 Out 短路 sub-agent goal） | ❌ |

**关键设计**：agent 级 workflow **必须** 用 `SpawnChildFresh`（每轮独立 blackboard）—— 用默认的 `Spawn()` 继承会让 orchestrator 累积的 typed Out 泄漏到 child blackboard，导致 sub-agent 的 "produce Out" goal 被判定为已满足，body 短路不执行。这是第 8/9 轮在实现 Loop 时发现并解决的设计要点。

---

## 8. Autonomy / Goal Ranking — 追平

完整 LLM-driven 选 goal/agent 路径在 [`runtime/autonomy`](../runtime/autonomy)。

| 维度 | lynx | embabel |
|---|---|---|
| GoalApprover（拒掉特定 goal） | ✅ `core.GoalApprover` extension（[core/extension.go:63](../core/extension.go)） | ✅ `GoalChoiceApprover.kt` |
| 多 goal 同 process 选优（按 NetValue） | ✅ planner 默认行为（`SortByNetValueDesc`） | ✅ `Autonomy` |
| LLM 选 goal/agent | ✅ `autonomy.Autonomy.Choose / Run`（[autonomy.go](../runtime/autonomy/autonomy.go)） + `Ranker` SPI + `LLMRanker` 实现 | ✅ `Autonomy.choose` + `Ranker` SPI |
| **LLMRanker prompt 包含 Goal.Tags + Examples** | ✅ ([llm_ranker.go](../runtime/autonomy/llm_ranker.go)) `buildUserPrompt` 在每 candidate 行下追加 `tags: ...` / `examples:` 块 | ✅ |
| Confidence cutoff | ✅ `Config.GoalConfidenceCutOff` → `ErrNoConfidentChoice` | ✅ `AutonomyProperties.goalConfidenceCutOff` |
| `approveWithScoreOver(threshold)` 工厂 | ✅ `GoalConfidenceCutOff` 同款 | ✅ |
| AgentFilter / GoalFilter | ✅ ([autonomy.go](../runtime/autonomy/autonomy.go)) | ✅ Spring profile / role |
| 一键执行 | ✅ `Autonomy.Run` —— 装一个 per-process `targetGoalApprover` 把 planner 锁定到选中 goal | ✅ `Autonomy.runWithChoice` |
| Plan-级 LLM 排序 | ✅ `autonomy.LLMPlanRanker` + `PlanRanker` 接口（[plan_ranker.go](../runtime/autonomy/plan_ranker.go)） | ✅ `LlmRanker` |
| Multi-goal 顺序模式 | ❌（极少用例；上层多次 `Run` 已覆盖） | ✅ `GoalSelectionOptions.multiGoal` |

---

## 9. MCP 集成 — 已闭合

| 子能力 | lynx | embabel |
|---|---|---|
| **客户端：消费外部 MCP server** | ✅ `lynx/mcp.NewProvider`（多源聚合 + 缓存 + 命名前缀 + list_changed 失效 + meta 透传 + sampling 反向 + 错误语义反转） | ✅ `SpringAiMcpToolFactory` |
| **服务端：暴露 lynx tool 为 MCP server** | ✅ `lynxmcp.RegisterTools(server, tools...)` | ✅ `McpToolExport.fromToolObject(...)` |
| **agent runtime 桥接** | ✅ `runtime.MCPToolGroupResolver`（[runtime/mcp.go](../runtime/mcp.go)） | ✅ McpToolFactory |
| **per-agent 自动暴露** | ✅ `runtime.AsMCPTool[In, Out](platform, agentName)` | ✅ `PerGoalMcpExportToolCallbackPublisher` |
| **多 goal 自动批量暴露** | ✅ `runtime.PublishAll(platform)`（按 `Goal.Export.Remote=true` 收集） | ✅ |

---

## 10. 事件 / 可观测 — 等价；event listener 单一注册路径 lynx 更整洁

| 维度 | lynx | embabel |
|---|---|---|
| Listener 接口 | `runtime.EventListener`（[runtime/extension.go](../runtime/extension.go)，runtime-tied 避免 core→event 循环） + `event.Listener` 简化版 | `AgenticEventListener` |
| 事件类型清单 | **14 个**：platform（`AgentDeployed / AgentUndeployed`）+ process（`ProcessCreated / ProcessCompleted / ProcessFailed / ProcessStuck / ProcessWaiting / ProcessKilled / ProcessTerminated`）+ planning（`ReadyToPlan / PlanFormulated / ReplanRequested`）+ execution（`ActionExecutionStart / ActionExecutionResult / GoalAchieved`） | 类似规模 |
| JSON 序列化 | ✅ 每事件 `json.Marshaler`，统一走 `emit(envelope)` 一行（[event/event.go](../event/event.go)） | Jackson 默认 |
| 多播 | ✅ `event.Multicast`（[event/multicast.go](../event/multicast.go)） + per-process 多播 | Spring 派发 |
| **Channel-style 消费 helper** | ✅ `event.NewNamedListener(name, fn)` —— 走单一 `Extensions` 注册路径，无双路径黑盒 | —（用户自实现） |
| OTel | ✅ `core.AgentTracer()` + 标准 attrs | ✅ Micrometer + OTel exporter |
| Cost / token / model 内置 metrics | ❌ 用户自挂（chat 中间件层职责） | ✅ embabel-agent-observability 模块内置 |

**event listener 单一注册路径** 是 lynx 的设计承诺：listener 只通过 `PlatformConfig.Extensions` / `ProcessOptions.Extensions` 注入，避免 "构造时显式 + 运行时偷偷注入" 双路径黑盒（这也是 ADK 比较时拒做 `Platform.RunAgentStream` 的核心论据，详见 [ADK_COMPARISON.md §6](./ADK_COMPARISON.md)）。

---

## 11. Extension 模型 — lynx 单入口更整洁

| 维度 | lynx | embabel |
|---|---|---|
| 注册入口 | **1 个**：`PlatformConfig.Extensions` + `ProcessOptions.Extensions` ([runtime/platform.go](../runtime/platform.go)) | 多个：每 SPI 一个 Spring `@Bean` 注入点 |
| 检测方式 | `core.Extension` + type assertion（`http.Pusher` 风格，[core/extension.go](../core/extension.go)） | Spring DI |
| **Core capability**（5 个） | `ActionInterceptor / ToolDecorator / AgentValidator / GoalApprover / BlackboardFactory` | 对应 5 个 SPI |
| **Runtime capability**（4 个） | `EventListener / PlannerFactory`（[runtime/extension.go](../runtime/extension.go)） + `IDGenerator`（[core/id_gen.go](../core/id_gen.go)）+ `ToolGroupResolver`（[core/tool_group.go](../core/tool_group.go)） | 对应 4 个 SPI |
| **总计** | **9** | **9 直接对应 + 4 LLM ops 专属**（LlmMessageSender / Streamer / ToolInjectionStrategy / EmbeddingService） |
| 重复检测 | ✅ 注册时按 Name dedup，panic ([runtime/dispatch.go](../runtime/dispatch.go)) | Spring `@Bean` 名级 |

**embabel 的 4 个 LLM ops 扩展点** lynx 都不在 agent 内核做——LlmMessageSender / Streamer 是 `core/model/chat` 的领域；ToolInjectionStrategy 是 ToolLoop 策略；EmbeddingService 是 RAG 子系统的事。**职责分工不同**，不是缺口。

---

## 12. 持久化 / 多进程 — 抽象等价、开箱仍弱（按设计）

| 维度 | lynx | embabel |
|---|---|---|
| Process registry | ✅ in-memory + `PruneTerminalProcesses` | ✅ + persistent backends |
| Blackboard | ✅ in-memory + Spawn + Clear + Protect | ✅ + Redis / DB |
| **`BlackboardFactory` 扩展点**（**唯一** blackboard SPI） | ✅ ([core/extension.go](../core/extension.go)) | ✅ `BlackboardProvider` |
| Context repository（跨 session） | ❌ —— 走 Blackboard 软持久化绕过（[`PERSISTENCE.md`](./PERSISTENCE.md)） | ✅ `ContextRepository` SPI |
| AgentProcessRepository | ❌（按设计不在 framework 内做） | ✅ `InMemoryAgentProcessRepository` |
| 实现指南 | ✅ [`PERSISTENCE.md`](./PERSISTENCE.md)（Redis / SQL / WAL 三策略） | ✅ Spring Data 集成 |

---

## 13. Modern Go 现代化（lynx 第八-十轮持续推进）

lynx 是**Go 1.26.3** 工作区，已用上的现代 Go 特性：

| Go 版本 | 特性 | lynx 用法 |
|---|---|---|
| Go 1.18 | `any` / generics | 全代码库 — `core.NewAction[In, Out]` 等所有 typed API |
| Go 1.21 | `sync.OnceValue[T]` | `core.Agent.knownConditions` / `plan.PlanningSystem.knownConditions` —— 替代 `atomic.Pointer + sync.Once` 双检锁样板 |
| Go 1.21 | `slices.SortStableFunc + cmp.Compare` | `plan.SortByNetValueDesc` / `autonomy.LLMPlanRanker` 排序 |
| Go 1.21 | `slices.Sorted(maps.Keys(...))` | `plan/condition_world_state.go` |
| Go 1.22 | `for i := range N` | `core/agent.go` 等 |
| Go 1.22 | 循环变量 per-iteration | 删除 `i, sub := i, sub` 类型样板 |
| Go 1.24 | `t.Context()` | 全测试代码 |
| Go 1.25 | `wg.Go(fn)` | `runtime/concurrent.go`（替代 errgroup 误用）+ `event/listener_test.go` |
| Go 1.26 | `errors.AsType[T]` | `runtime/execute_action.go` / `hitl/tool.go` —— 替代 `errors.As(err, &target)` 类型化指针舞蹈 |

**已主动放弃的旧代码模式**：
- `interface{}` → `any`
- `errors.As(err, &target)` → `errors.AsType[T](err)`
- `atomic.Pointer + sync.Once` → `sync.OnceValue`
- `errgroup` 用于纯 wait（不 fan-in error）→ `sync.WaitGroup + wg.Go`
- `_ = g.Wait()` → 弃用（重新选择正确原语）

---

## 14. 注解 / classpath scan vs DSL Builder — 永久分歧

lynx：`agent.Builder`（[builder.go](../builder.go)），编译期类型安全 + IDE rename 100% 友好 + 启动时静态可推。embabel：`@Agent / @Action / @AchievesGoal / @LlmTool / @PromptCondition / @RunSubagent / @MatryoshkaTools / @UnfoldingTools / @ToolGroup / @Cost / @Provided / @Semantics / @EmbabelComponent` 13+ 注解 + Spring scan。

Go 没 Spring AOT，反射注册不合 Go 哲学。**永久分歧**，请把它当哲学差异而非缺口。

---

## 15. 命名 / API ergonomics — lynx 顶层窄

| 维度 | lynx | embabel | 谁更好 |
|---|---|---|---|
| **Top-level surface** | **5 个 constructor** —— `agent.New / NewAction / NewCondition / GoalProducing / NewPlatform`（[agent.go](../agent.go)） + 几个 helper（`runtime.AsChatTool / AsChatToolFromAgent / AsMCPTool / AllAchievableTools / PublishAll / MCPToolGroupResolver`） | 100+ Spring beans + 13+ 注解 + DSL | **lynx** |
| Config 模式 | struct + `applyDefaults` | data class + sensible defaults | 等价 |
| 错误风格 | 统一 `verb-noun-prefix`（如 `"run agent %q: ..."` / `"subagent %q: parse input: %w"`），全用 `%w` 嵌套传播 | Kotlin exceptions | 各有所长 |
| 包数量（agent 框架） | **7**（`core / plan / runtime / event / hitl / workflow / toolpolicy`）+ 根包 `agent`（`Builder` + 5 forwarders）+ sibling `lynx/mcp` | 17+ Maven module | **lynx** |
| 类型层次 | 浅 | 深（Spring 抽象类堆叠） | **lynx** |
| 代码量 | **9,825 LOC** non-test（30+ 文件） | **~209k LOC**（1,182 个 .kt） | **lynx** |

---

## 16. 故意不要做的事（不是缺口）

**永久分歧 / 哲学差异**：
- **注解 / classpath scan agent 注册** —— Go 心智模型不支持 magic
- **Spring DI 容器** —— `core.ServiceProvider` 已够；不再加重 DI
- **Sync / Async 双套 API** —— Go 的 `context.Context` + goroutine 已经覆盖；embabel 二分是 Spring artifact
- **SpEL / LogicalExpressionParser** —— Spring Expression Language，与 Go 心智不符
- **Megazord 注解（多 agent 合体）** —— 反射特化，做了也没人用
- **Personality / Shell 装饰** —— lynx 没这文化
- **死字段 / 谓词糖** —— 接口面最小，靠 caller 写 `d == core.True` / `_, ok := provider.Get(key)` 这类直白表达

**职责边界外（chat 中间件层 / sibling repo / 用户应用代码）**：
- **多 LLM provider 内置整合 / `ModelProvider`** —— `chat.Model` 是开放接口，用户自己挂
- **`TemplateRenderer`** —— `core/model/chat` 已有 `PromptTemplate`
- **Session / Memory / Artifact 三件套** —— chat 中间件层 / `lynx/rag` sibling
- **TUI / SSE / HTTP / A2A server frontend** —— lynx 是库不是 framework
- **YAML agent 定义** —— typed DSL 是 lynx 核心价值
- **MCP server-side resources / prompts 暴露** —— SDK 直接用，不再包一层
- **RAG / Onnx / Skills / Shell 子模块** —— 独立 repo 职责

**用例驱动 / 暂不做**：
- **HTN 默认 task library** —— HTN 本来就需要领域知识；framework 不替用户假设
- **`OperationScheduler` / 异步调度** —— 现有原语（`AwaitInput` + 外部 `ResumeProcess` + `ActionQoS` retry）已能拼出所有真实用例
- **`MatryoshkaTools` / `UnfoldingTools` 渐进披露** —— 跨 chat 包边界，等真实用例
- **`DataDictionary` / `DomainType` / `DynamicType`** —— Go 强类型 + 反射 + IOBinding 已足；embabel 这套是 Spring KB / 多 agent schema 共享的特化
- **Multi-goal 顺序模式**（`GoalSelectionOptions.multiGoal`）—— 极少用例；上层多次 `Autonomy.Run` 已覆盖
- **Plan caching / Agent versioning / hot-reload** —— 双方都没做；不是竞争点

---

## 17. 重构与对比历史

第十轮之前的关键节点：

- **第七轮** (2026-05-08, `25fd46a`)：SOLID/DRY/KISS — split platform, slim Planner SPI, dedup helpers；Wave B/C/D/E/F 死代码 + 防御式 nil + 单实现接口去虚 + 现代 Go 标准库。LOC 9512 → 8853 (-7%)。
- **第八轮** (2026-05-09, `6a1e7a1`)：drop `dsl/` subpackage（Builder 升根 + workflow/toolpolicy 升 sibling）+ unify `subagentTool` / `mcpTool` 为 `agentTool` + `runProcessFunc`。LOC 8853 → 8715。
- **第九轮 P0** (2026-05-09, `17f4f62`)：`Goal.Export` + `GoalExportFor[In]` + `PromptCondition` + `ParseYesNoDetermination` + `AllAchievableTools` + `PublishAll` + `dynamicAgentTool`。LOC 8715 → 9319。
- **第九轮 P1+P2** (2026-05-09, `fa6f9a6`)：STRIPS regression 剪枝（`relevantActions`） + `Goal.Tags` + `Goal.Examples` + LLMRanker prompt 接通。LOC 9319 → 9888。
- **第九轮收口** (2026-05-09, `9145da7`)：fresh 对照 5/11 误报 + 1/11 真新 dimension（DataDictionary）经 critical review 是 YAGNI。
- **第十轮 polish** (2026-05-09, `ed23cbb` / `0b2749f` / `8c47475`)：DRY/SRP refactor → Go 1.26 现代化（`errors.AsType[T]` / `sync.OnceValue` / `wg.Go(fn)` / `t.Context()` / `for range N` / `mustDeploy` 测试规范化）+ 工作区升级到 Go 1.26.3。LOC 9888 → **9,825**。

每轮都按"用例驱动 + 死字段零容忍 + 字段+reader 同时落"原则推进。

---

## 18. 一句话总结

lynx 在 framework 抽象层 1:1 追平 embabel 内核，并在 HTN / Reactive / agent 级 workflow / 单注册 Extension / dynamic publish 等方向有自有创新。剩余差距全部是**故意分歧**或**职责边界外**（Spring DI / 注解 / 多 provider / RAG / Shell 等），不是 framework 抽象的缺口。

代码量 **9,825 LOC** vs embabel 的 **~209k LOC**，同样的领域模型、≈ 1/20 的密度差距全部来自 Spring 工程基础设施 + 内置 RAG/Onnx/Shell 等独立模块——这是分工不同，不是优劣。

**下一阶段不再靠 framework 内对照推动**：第十轮没有新增功能，只有近期 polish 的状态固化。framework 抽象层已收口；后续改动应该用例驱动——真有用户跑起来后发现具体痛点 → 针对性加。两个项目都正确，只是站在 Go 与 Kotlin/Spring 哲学的两端。
