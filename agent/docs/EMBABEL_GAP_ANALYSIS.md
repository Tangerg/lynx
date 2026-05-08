# lynx/agent vs embabel-agent — 深度对比与缺口分析

> **第六轮重写**（2026-05-08）。基线：embabel-agent (Kotlin/Spring) v0.4 / lynx/agent (Go) HEAD（含 HTN/Reactive 双 planner、tool-级 HITL、Supervisor `AsChatTool`、planner 公共 helper（`plan.BestOf` / `SortByNetValueDesc`）、`(*core.Goal).IsSatisfiedBy` 提升）。
>
> 本轮相对第五轮的关键变动：**HTN planner**（`plan/planner/htn`）、**Reactive planner**（`plan/planner/reactive`）、**Tool-级 HITL**（`hitl/tool.go`）、**Supervisor 模式**（`runtime/subagent.go` 的 `AsChatTool[In, Out]`）四件全部落地，把第五轮列为 P0–P1 的四条全部闭合，并完成一轮代码优化（goal 自满足判定、planner 公共排序/挑选 helper、HTN ctx 取消传播、subtask slice 预分配）。本文从代码现状重新逐行推演，不再袭用旧表。
>
> 配套文档：[`./EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md) / [`./REFACTOR_PLAN.md`](./REFACTOR_PLAN.md) / [`/Users/tangerg/Desktop/lynx/mcp/DESIGN.md`](../../mcp/DESIGN.md)。

---

## 0. TL;DR

| 维度 | lynx/agent | embabel-agent | 差距 |
|---|---|---|---|
| 核心抽象（Agent / Goal / Action / Condition / Blackboard / WorldState） | ✅ 完整（`(*Goal).IsSatisfiedBy` 已抽出） | ✅ 完整 | **无差距** |
| GOAP planner（A* + reachability） | ✅ `plan/planner/goap` | ✅ DefaultPlannerFactory | **无差距** |
| HTN planner | ✅ `plan/planner/htn`（`Library` + `Task` + `Method`，ctx-cancellable，8 单测） | ✅ | **追平** |
| Reactive / Utility planner | ✅ `plan/planner/reactive`（greedy 1-step，progress × cost-tie，7 单测） | ✅ `UtilityPlanner.kt` | **追平** |
| Plan 后处理 / 多 plan 排序 | ✅ `plan.BestOf` + `plan.SortByNetValueDesc` 公共 helper | ✅ + `LlmRanker` | 小差距：LLM 排序 |
| OODA tick loop（Sequential + Concurrent） | ✅ `runtime/run.go` + `runtime/concurrent.go` | ✅ | **无差距** |
| Retry / QoS（per-action） | ✅ 委托 `pkg/retry` | ✅ ActionRetryPolicy | **等价** |
| HITL: process 级 Awaitable<P, R> | ✅ `hitl.TypedRequest` | ✅ `Awaitable<P, R>` | **追平** |
| **HITL: tool 级 AwaitDecider / Confirming / TypeRequesting** | ✅ `hitl/tool.go`（`WithAwaiting` / `WithConfirmation` / `RequireType[T]`，11 单测） | ✅ `AwaitingTools.kt` | **追平** |
| Extension 模型 | ✅ 1 注册入口 + 9 capability + type assertion 检测 | ✅ 多 SPI + Spring DI | **lynx 更整洁**（设计分歧） |
| 事件 / 可观测 | ✅ `event.Multicast` + 16+ 事件类型 + JSON marshaler + OTel tracer | ✅ `AgenticEventListener` + Micrometer | **等价** |
| Tool 模型（Tool / Group / Resolver / Decorator / TerminationScope） | ✅ `core/tool_group.go`（与 `chat.Tool` 共用类型） | ✅ Tool / ToolObject | **追平** |
| ToolLoop runner | ✅ `chat.NewToolMiddleware` + `pc.ChatWithActionTools(ctx)` | ✅ `ToolLoop` + `ToolInjectionStrategy` | **追平**（路径不同） |
| Tool advanced policies（Matryoshka / OneShotPerLoop / Playbook / StateMachine） | ❌ | ✅ `MatryoshkaTool.kt` / `OneShotPerLoopTool.kt` / `agentic/playbook` / `agentic/state` | 真小缺口（P3）|
| **MCP 客户端 / 服务端** | ✅ `lynx/mcp` 全功能 + `runtime.MCPToolGroupResolver` | ✅ `SpringAiMcpToolFactory` + `McpToolExport` | **已闭合** |
| MCP per-agent 自动暴露（agent → MCP tool） | ✅ `runtime.AsMCPTool[In, Out]` | ✅ `PerGoalMcpExportToolCallbackPublisher` | **追平基线**（多 goal 自动切分仍欠，P3） |
| **Supervisor 模式（子 agent → chat tool）** | ✅ `runtime.AsChatTool[In, Out]`（4 集成测 + supervisor 示例） | ✅ `Subagent.kt`（4 工厂路径 + `RunSubagent` 注解） | **追平基线**；4 工厂细差见 §10 |
| WorkflowBuilder（ScatterGather / RepeatUntil / Feedback） | ✅ `dsl/workflow`（`ScatterGatherAgent` 并行 fan-out + `RepeatUntilAgent` 循环到 Accept；`Feedback` 数据类型；7 单测）| ✅ `api/common/workflow/` 完整 DSL | **基线追平**；Consensus 多模型投票 + RepeatUntilAcceptable 内置 LLM 评判仍欠 |
| Autonomy / Goal Ranker（LLM 选 goal） | 部分（`GoalApprover` capability） | ✅ `Autonomy` + `Ranker` + `GoalChoiceApprover` | 中差距（P2） |
| A2A / RAG / Skills / Shell | ❌ | ✅ 4 独立模块 | 真缺口（P3，独立子模块路线） |
| 持久化（Blackboard / process / context） | ⚙️ `BlackboardFactory` 扩展点已开放，开箱仍 in-memory；用户接 Redis/SQL/WAL 见 [`PERSISTENCE.md`](./PERSISTENCE.md) | ✅ Spring Data + 现成 in-memory 仓 | **抽象等价**；开箱实现仍欠（按设计不在 framework 内做） |
| 多 LLM provider 内置 | ❌（BYO `chat.Model`） | ✅ Spring AI 全套 + ONNX | **故意分歧** |
| 注解 / classpath scan | ❌（DSL Builder） | ✅ `@Agent` / `@Action` / `@LlmTool` | **故意分歧** |

**一句话**：第五轮列出的 4 条 P0–P1（HTN / Reactive / tool-级 HITL / Supervisor）全部闭合。剩下的真缺口收敛到 **WorkflowBuilder 模式 DSL（P2）、持久化 backend（P2）、Autonomy/LLM-Ranker（P2）、Tool advanced policies（P3）、A2A/RAG/Skills/Shell（P3 独立子模块）**。设计哲学不变：lynx 是 "minimal kernel + BYO integrations"，embabel 是 "batteries-included Spring stack"。

---

## 1. 核心抽象 — 1:1 对齐（含一处提升）

| 抽象 | lynx | embabel |
|---|---|---|
| Agent | `core.Agent` ([core/agent.go](../core/agent.go)) | `core.Agent` |
| Action / metadata / retry | `core.Action` + `core.ActionConfig` ([core/action.go](../core/action.go), [core/action_config.go](../core/action_config.go)) | `Action.kt` + `ActionRetryPolicy.kt` |
| Goal（前置 + cost/value，**`IsSatisfiedBy`**） | `core.Goal` + `GoalProducing[T]` ([core/goal.go:59](../core/goal.go)) | `Goal.kt` |
| Condition + And/Or/Not | `core.Condition` + `ComputedCondition` ([core/condition.go](../core/condition.go)) | `Condition.kt` |
| Blackboard（dual-binding "it" + protected） | `core.Blackboard` ([core/blackboard.go](../core/blackboard.go)) | `Blackboard.kt` |
| WorldState | `core.WorldState` + `plan.ConditionWorldState` ([plan/condition_world_state.go](../plan/condition_world_state.go)) | `WorldState` |
| EffectSpec / Effects | `core.EffectSpec` ([core/determination.go](../core/determination.go)) | `Effects` typealias |
| IOBinding | `core.IOBinding` ([core/io_binding.go](../core/io_binding.go)) | `IoBinding.kt` |
| Determination（True/False/Unknown） | `core.Determination` ([core/determination.go](../core/determination.go)) | `Determination` |
| ActionStatus / ProcessStatus | `core.ActionStatus` + `AgentProcessStatus` ([core/enum.go](../core/enum.go)) | `ActionStatus` + `AgentProcessStatusCode` |
| ProcessContext | `core.ProcessContext` ([core/process_context.go](../core/process_context.go)) | `ProcessContext.kt` |
| ServiceProvider | `core.ServiceProvider` ([core/service_provider.go](../core/service_provider.go)) | Spring `ApplicationContext`（更重） |

**结论**：核心模型对齐。`(*Goal).IsSatisfiedBy(ws)` 这次从三个 planner 的内部 helper 提到 Goal 上（[core/goal.go:59](../core/goal.go)），让 GOAP / HTN / Reactive 都不再各自写一份 "current state vs preconditions" 比较——和 embabel 的 `Goal.isAchievable(currentState)` 对齐。

---

## 2. Planner — GOAP / HTN / Reactive 三全；公共 helper 已抽出

| 维度 | lynx | embabel |
|---|---|---|
| A* GOAP | ✅ `plan/planner/goap/astar.go`（≈510 LOC） | ✅ DefaultPlannerFactory.kt |
| Reachability pre-check | ✅ [runtime/platform.go:183](../runtime/platform.go) `checkGoalsReachable` | ✅ |
| Backward + forward optimisation | ✅ | ✅ |
| Excluded actions（防 replan loop） | ✅ `state.snapshotExclusions` ([runtime/run.go](../runtime/run.go)) | ✅ |
| Plan 后处理 / 多 plan 排序 | ✅ `plan.BestOf(plans, err)` + `plan.SortByNetValueDesc(plans, ws)` ([plan/plan.go:69](../plan/plan.go), [plan/plan.go:104](../plan/plan.go)) | ✅ |
| Goal LLM 排序 | ❌ | ✅ `LlmRanker` / `Ranker` |
| **HTN planner** | ✅ `plan/planner/htn/htn.go`（`Library` + `Task` 兼容 primitive/compound + `Method`；ctx-cancellable 递归 decompose；method 失败 backtrack；未知 subtask = 结构错误；snapshot 复用避免逐 method 拷贝；subtask actions slice 预分配；8 单测） | ✅ |
| **Reactive / Utility planner** | ✅ `plan/planner/reactive/reactive.go`（progress×cost tie-break，0-progress 直接拒，7 单测） | ✅ `UtilityPlanner.kt`（"1 步够 → 立即；0 步 → 选第一可用 action 取下一步"） |
| PlannerFactory 扩展点 | ✅ `runtime.PlannerFactory` ([runtime/extension.go:31](../runtime/extension.go)) | ✅ `spi.PlannerFactory` |
| Default factory 分派 | ✅ `defaultPlannerFactory` 处理 GOAP / Reactive；HTN 返回 nil（要 user-supplied library）— 报错时附带 hint：[runtime/platform.go:380](../runtime/platform.go) | ✅ |
| Planner.Prune | ✅ goap 真做；htn / reactive 设计上无操作 | ✅ |

**新发现**：lynx reactive planner 跟 embabel `UtilityPlanner` 比，**多了 progress 评分 + cost tie-break**。embabel 的 `UtilityPlanner` 只是按 `netValue(currentState)` 一刀切排序、取第一个可用 action（[`UtilityPlanner.kt:42`](#)），不会衡量"这一步真的把 goal 推近多少"——遇到一堆都有 netValue 但都不解 goal precondition 的 action 会一直循环。lynx 的实现严格更强（无 progress = 拒绝出 plan，让 stuck 处理介入）。**反向无差距**。

**剩余小缺口**：embabel 的 `LlmRanker`（让 LLM 给候选 plan 打分）lynx 没做——这是 LLM-driven planning 路线的一块；优先级 P2。

---

## 3. OODA tick loop / Concurrent — 等价

| 维度 | lynx | embabel |
|---|---|---|
| Tick 主循环 | [runtime/run.go](../runtime/run.go) `(*AgentProcess).run` | `AbstractAgentProcess.run()` |
| Sequential / Concurrent 双模 | ✅ `tickSimple` + [runtime/concurrent.go](../runtime/concurrent.go) `tickConcurrent` | ✅ Simple / Concurrent |
| Suspend → Resume → Continue 闭环 | ✅ `Platform.ResumeProcess` + `ContinueProcess` ([runtime/platform.go:263](../runtime/platform.go)) | ✅ |
| Termination scope（agent / action / tool_call 三粒度） | ✅ ([core/tool_group.go](../core/tool_group.go)) | ✅ |
| Stuck handler / Early termination | ✅ `core.StuckHandler` + `EarlyTerminationPolicy` ([core/early_termination.go](../core/early_termination.go)) | ✅ `StuckHandler.kt` |

---

## 4. Tool 模型 — 基线追平；advanced policies 仍空

| 维度 | lynx | embabel |
|---|---|---|
| Tool 定义 | `core.AgentTool = chat.Tool`（[core/tool_group.go](../core/tool_group.go) 类型别名） | `Tool.Definition` |
| ToolGroup（按 role 聚合 + lazy load） | `core.ToolGroup` + `LazyToolGroup` | `ToolObject` |
| ToolGroupResolver（role → group） | ✅ extension 接口 | ✅ `spi.ToolGroupResolver` |
| ToolGroupRequirement（声明依赖） | ✅ 含 `TerminationScope` | ✅ |
| TerminationScope（agent / action / tool_call） | ✅ | ✅ |
| ToolDecorator（包装 tool） | ✅ extension capability ([core/extension.go:59](../core/extension.go)) | ✅ `ToolDecorator` SPI |
| ToolLoop runner | ✅ `chat.NewToolMiddleware()` + [core/process_context.go](../core/process_context.go) `ChatWithActionTools` | ✅ `ToolLoop.kt` + 多策略 |
| 动态 tool injection / Matryoshka | ❌ | ✅ `MatryoshkaTool.kt` / `UnfoldingToolInjectionStrategy` |
| One-shot-per-loop（避免 LLM 重复调用同 tool） | ❌ | ✅ `OneShotPerLoopTool.kt` |
| Playbook tool（条件解锁） | ❌ | ✅ `agentic/playbook/PlaybookTool.kt` + `UnlockCondition.kt` |
| StateMachine tool | ❌ | ✅ `agentic/state/StateMachineTool.kt` |
| Replanning tool | ❌ | ✅ `ReplanningToolFactory.kt` |
| EmptyResponsePolicy / ToolNotFoundPolicy / RequiredToolGroupException | 等价语义在 chat 包外侧（middleware） | ✅ `spi/loop/*Policy.kt` |

**新发现（第五轮没单列）**：embabel 在 `agentic/` 下放了一整套 LLM-tool-loop 高级模式——Playbook（按条件解锁工具）、StateMachine（工具状态机驱动多步对话）、Replanning（一次工具调用后用 LLM 重排 plan）。lynx 把这些都视为 **应用层模式**（用户在 action body 里组合 ToolDecorator + blackboard 写出来），不在 framework 层做。**P3 小缺口，存档备查**。

---

## 5. HITL — 双层全追平

| 维度 | lynx | embabel |
|---|---|---|
| Typed Awaitable<P, R> | ✅ `hitl.TypedRequest` ([hitl/awaitable.go](../hitl/awaitable.go)) | ✅ `Awaitable<P, R>` |
| 类型化 response 路由 | ✅ `OnResponseAny` 类型断言 | ✅ |
| Confirmation 特化 | ✅ `hitl.NewConfirmation` | ✅ `ConfirmationRequest` |
| Suspend → Resume → Continue 闭环 | ✅ ([runtime/platform.go:263](../runtime/platform.go)) | ✅ |
| **`AwaitDecider`（tool 入口决定要不要 HITL）** | ✅ `hitl.AwaitDecider` ([hitl/tool.go:66](../hitl/tool.go)) | ✅ `AwaitingTools.AwaitDecider` |
| **`WithAwaiting(tool, decider)`（条件性 HITL 包装）** | ✅ ([hitl/tool.go:72](../hitl/tool.go)) | ✅ `Tool.withAwaiting(decider)` |
| **`WithConfirmation(tool, prompter, onResponse)`（强制确认）** | ✅ ([hitl/tool.go:113](../hitl/tool.go)) | ✅ `Tool.withConfirmation { … }` |
| **`RequireType[T](tool, prompter, onResponse)`（强制类型化输入）** | ✅ ([hitl/tool.go:138](../hitl/tool.go)) | ✅ `Tool.requireType<T>()` |
| `PauseError` sentinel + `HandlePause(pc, err)` 一键路由 | ✅ ([hitl/tool.go:23](../hitl/tool.go), [hitl/tool.go:47](../hitl/tool.go)) | ✅ `AwaitableResponseException` |
| 持久化 awaitable | ❌ in-memory | ✅ context repository（前提：用持久化 backend） |

**怎么落地的**：tool 级 HITL 走的是 `chat.NewToolMiddleware` 抛错路径——decider 决定 pause 时返回 `*PauseError{Request: awaitable}`；middleware 捕获该 error 终止 LLM 调用、向上传给 action body；action body 用 `hitl.HandlePause(pc, err)` 一行做 `errors.As` + `pc.AwaitInput`，把 awaitable 注册到 process。embabel 的 `AwaitableResponseException` 走 JVM 异常通道，是相同模式。

**唯一保留缺口**：持久化 awaitable——前提是 BlackboardFactory + ProcessRepository 都有持久化 backend。lynx in-memory only，仍是 P2。

---

## 6. Extension 模型 — lynx 单入口更整洁

| 维度 | lynx | embabel |
|---|---|---|
| 注册入口 | **1 个**：`PlatformConfig.Extensions` + `ProcessOptions.Extensions` ([runtime/platform.go:66](../runtime/platform.go)) | 多个：每 SPI 一个 Spring `@Bean` 注入点 |
| 检测方式 | `core.Extension` + type assertion（`http.Pusher` 风格，[core/extension.go:20](../core/extension.go)） | Spring DI |
| Capability 数 | **9**（ActionInterceptor / ToolDecorator / AgentValidator / GoalApprover / EventListener / IDGenerator / PlannerFactory / BlackboardFactory / ToolGroupResolver） | **9 直接对应 + 4 LLM ops 专属**（LlmMessageSender / LlmMessageStreamer / ToolInjectionStrategy / EmbeddingService …） |
| Per-process 扩展 | ✅ `ProcessOptions.Extensions` | ✅ `ProcessOptions.listeners`（更窄） |
| 重复检测 | ✅ 注册时按 Name dedup，panic ([runtime/dispatch.go:28](../runtime/dispatch.go)) | Spring `@Bean` 名级 |

**embabel 的 4 个 LLM ops 扩展点** lynx 都不在 agent 内核做——LlmMessageSender / Streamer 是 `core/model/chat` 的领域；ToolInjectionStrategy 是 ToolLoop 策略；EmbeddingService 是 RAG 子系统的事。**这是分工分歧而非缺口**。

---

## 7. 事件 / 可观测 — 等价

| 维度 | lynx | embabel |
|---|---|---|
| Listener 接口 | `runtime.EventListener` ([runtime/extension.go:21](../runtime/extension.go))（runtime-tied，避免 core→event 循环） | `AgenticEventListener` |
| 事件类型数 / 多播 | 16+ 类型（platform / process / planning / execution / llm 五类）+ `event.Multicast` 含 per-process 多播 | 类似 + Spring 派发 |
| JSON 序列化 | ✅ 每个事件类型实现 `json.Marshaler` ([event/event.go](../event/event.go)) | 各事件 data class，Jackson 默认 |
| OTel / Metrics | ✅ `agentTracer = otel.Tracer("lynx/agent")`（[core/process_context.go:17](../core/process_context.go)），tick / action 自动 span | ✅ Spring AI 内置 + Micrometer |
| LLM 事件 + 用户自定 publish | ✅ `LLMRequestEvent` / `LLMResponseEvent` + `pc.Publish(any)` | 同等 |

---

## 8. MCP 集成 — 已闭合（保留）

| 子能力 | lynx | embabel |
|---|---|---|
| **客户端：消费外部 MCP server** | ✅ `lynx/mcp.NewProvider`（多源聚合 + 缓存 + 命名前缀 + list_changed 失效 + meta 透传 + sampling 反向 + 错误语义反转） | ✅ `SpringAiMcpToolFactory` |
| **服务端：暴露 lynx tool 为 MCP server** | ✅ `lynxmcp.RegisterTools(server, tools...)`（错误自动回写 IsError） | ✅ `McpToolExport.fromToolObject(...)` |
| **agent runtime 桥接** | ✅ `runtime.MCPToolGroupResolver` ([runtime/mcp.go](../runtime/mcp.go)) | ✅ McpToolFactory |
| **per-agent 自动暴露**（agent → MCP tool） | ✅ `runtime.AsMCPTool[In, Out](platform, name)` 配 `lynxmcp.RegisterTools` 一行 | ✅ `PerGoalMcpExportToolCallbackPublisher`（按 goal 切分）|
| 多 goal 自动批量暴露（同 agent 多 goal → 多 tool） | ❌ | ✅ |
| Async / Reactive 双套 server | ❌（Go 无此二分） | ✅（Spring artifact） |

参见 [`agent/examples/mcp-agent/main.go`](../examples/mcp-agent/main.go) / [`agent/examples/mcp-bridge/main.go`](../examples/mcp-bridge/main.go)。

---

## 9. Supervisor / Subagent — 基线已追平；4 工厂细差

第五轮 P0：现已闭合。`runtime.AsChatTool[In, Out]`（[runtime/subagent.go:43](../runtime/subagent.go)）把已部署的子 agent 包成 `chat.CallableTool`，由 `chat.NewToolMiddleware` 驱动 LLM-tool 循环；子 process 通过 `Platform.CreateChildProcess` 继承 parent blackboard，`parent.budget.addChild` 自动聚合开销，子 agent 输出经 `core.ResultOfType[Out]` 抽出后 JSON-encode 给 LLM。

| 维度 | lynx | embabel |
|---|---|---|
| 创建子 process | ✅ `Platform.CreateChildProcess` ([runtime/platform.go:511](../runtime/platform.go)) | ✅ |
| Budget 跨子 process 聚合 | ✅ `(*AgentProcess).Usage` 递归 ([runtime/agent_process.go](../runtime/agent_process.go)) | ✅ Hierarchy-aware |
| **子 agent → chat tool**（LLM 自由编排） | ✅ `runtime.AsChatTool[In, Out](platform, agentName)` | ✅ `Subagent.byName(...).consuming<I>()` |
| 工厂路径数 | **1**：`byName` 风格（`agentName` + 泛型 In/Out） | **4**：`ofClass` / `byName` / `ofInstance` / `ofAnnotatedInstance` |
| Awaitable 子 agent（child 暂停时优雅回报 LLM） | ✅ child Waiting 时返回 JSON `{status:"waiting", agent, processId, awaitableId, prompt}`（[runtime/subagent.go:114](../runtime/subagent.go) + `waitingResultText`） | ✅ `ProcessWaitingException` → `textCommunicator.communicateAwaitable` 回报 tool result |
| Megazord（多 agent 反射合体 helper） | ❌（不打算做） | ✅ |
| `RunSubagent` 注解 | ❌（DSL 路线无注解） | ✅ |
| 集成测 + 示例 | ✅ 4 测 + [`examples/supervisor`](../examples/supervisor) | ✅ |

**一点小差**：
1. **4 工厂路径**：embabel 因为支持注解类，需要 `ofClass` / `ofAnnotatedInstance` 两条额外构建路径；lynx DSL 路线下 agent 都是 `*core.Agent` 实例，统一走 `agentName`，**单一工厂就够**——分歧符合各自语言哲学。如果未来 lynx 想暴露"传 `*core.Agent` 实例直接构造，跳过 platform 注册"的便利构造，可以追加 `runtime.AsChatToolFromAgent(agent)`，但当前 ROI 低。
2. ~~**Awaitable 子 agent 优雅退化**~~ —— ✅ **第六轮 P1-1 已闭合**：child Waiting 时返回结构化 JSON（`status / agent / processId / awaitableId / prompt`，[runtime/subagent.go:114](../runtime/subagent.go) + `waitingResultText`），父 LLM 可基于此换 plan；host 仍可用 `Platform.ResumeProcess + ContinueProcess` 续跑该 process。

**新增**：`runtime.AsMCPTool[In, Out](platform, agentName)` 是 `AsChatTool` 的顶层 MCP-host 版本——独立 process（无父 ctx 要求），同样支持 waiting graceful-degrade，搭配 `lynxmcp.RegisterTools` 一行把 agent 暴露给 MCP host：
```go
mcp.RegisterTools(server, runtime.AsMCPTool[Topic, Brief](platform, "BriefingAgent"))
```
对应 embabel `PerGoalMcpExportToolCallbackPublisher` 的 per-agent 形态。lynx 典型"一 agent 一 goal"下足够；多 goal 自动分拆见路线图 P3。

---

## 10. Autonomy / Goal Ranking — embabel 走得更远

| 维度 | lynx | embabel |
|---|---|---|
| GoalApprover（拒掉特定 goal） | ✅ `core.GoalApprover` extension ([core/extension.go:94](../core/extension.go)) | ✅ `GoalChoiceApprover.kt` |
| 多 goal 同 process 选优（按 NetValue） | ✅ planner 默认行为（`SortByNetValueDesc`） | ✅ `Autonomy` |
| **基于用户输入 LLM 选 goal/agent** | ❌ | ✅ `Autonomy.choose(goalConfidenceCutOff, ranker)` + `Ranker` SPI（[Autonomy.kt:71](#)） |
| **Confidence cutoff**（"LLM 信心 < 0.7 就别跑这个 agent"） | ❌ | ✅ `AutonomyProperties.goalConfidenceCutOff` / `agentConfidenceCutOff` |
| **Multi-goal 模式**（同 process 顺序追求多个 goal） | ❌ | ✅ `GoalSelectionOptions.multiGoal` |
| `ProcessWaitingException` → 回写到 tool 结果 | ❌ | ✅ |

**新发现**：embabel 的 `Autonomy` 服务对 lynx 是 **整块缺口**——它把"用户给一段自然语言 → 选哪个 goal/agent → 用什么 confidence 决定"形式化成了一组 SPI 加 `Ranker`。lynx 的 `GoalApprover` capability 只能做硬规则过滤（"租户没买这个 goal"），做不到 LLM 排序 + 信心阈值的软选择。**P2 中等缺口**——可以做成 `runtime.AutonomyExtension`，复用 `*chat.Client` 投票。

---

## 11. WorkflowBuilder / 多步组合模式 — 基线追平

第六轮 P2 落地：[`agent/dsl/workflow`](../dsl/workflow) 提供两条通用模式 + `Feedback` 数据类型，每个都生成普通的 `*core.Agent` 走标准 GOAP 路径；用户可 `platform.Deploy(workflowAgent)` 直接跑，或用 `runtime.AsChatTool` / `AsMCPTool` 嵌套到上层 LLM 编排。

| 维度 | lynx | embabel |
|---|---|---|
| Scatter-Gather（多分支并发 → 汇总） | ✅ `workflow.ScatterGatherAgent[In, Element, Result]`（`errgroup` 并行 + `MaxConcurrency`，3 单测）| ✅ `ScatterGather.kt` |
| RepeatUntil（条件循环至成立） | ✅ `workflow.RepeatUntilAgent[In, Out]`（`CanRerun=true` + `ComputedCondition` + `History[Out]` + `MaxIterations` 兜底，4 单测）| ✅ `RepeatUntil.kt` |
| Feedback 数据类型（Score + Text） | ✅ `workflow.Feedback`（含 `Acceptable(threshold)` 助手）| ✅ `Feedback.kt` |
| RepeatUntilAcceptable（内置 LLM 评判循环） | ❌ —— 用户可在 `RepeatUntilAgent.Accept` 里手写 LLM 调用 + `Feedback`；framework 不预制 | ✅ `RepeatUntilAcceptable.kt` |
| Consensus（多 LLM 多模型投票汇合） | ❌ —— 是 ScatterGather 的特化，用户在 `Joiner` 里实现 majority/score 投票 | ✅ `multimodel/ConsensusBuilder.kt` |
| SimpleAgentBuilder（单 action 一行 agent） | 已够简（`agent.New(...).Actions(...).Build()`）| ✅ `SimpleAgentBuilder.kt` |
| `WorkflowBuilder` 通用基类 | 不需要 —— builder 函数返回 `*core.Agent`，与普通 agent 同形 | ✅ `WorkflowBuilder.kt`（含 `asSubProcess`） |

**API 速查**：

```go
// 并发 fan-out + 汇总
agent := workflow.ScatterGatherAgent(workflow.ScatterGatherSpec[Topic, Brief, FinalReport]{
    Name:           "research-fanout",
    MaxConcurrency: 3,
    Generators:     []func(ctx, pc, in Topic)(Brief, error){gen1, gen2, gen3},
    Joiner:         func(ctx, pc, briefs)(FinalReport, error) {...},
})

// 循环到 Accept
agent := workflow.RepeatUntilAgent(workflow.RepeatUntilSpec[Topic, Draft]{
    Name:          "iterate-draft",
    MaxIterations: 5,
    Task:          func(ctx, pc, in Topic, history)(Draft, error) {...},
    Accept:        func(ctx, in, last, history) bool {
        return llmFeedback(last).Acceptable(0.8)
    },
})
```

**故意不做的两件**（同 §17 哲学）：
- **RepeatUntilAcceptable / Consensus 一等公民 builder**：embabel 因为用 Spring DI 把"评判员"作为 bean 拿到，所以要专门 builder。lynx 用户在 `Accept` 闭包里直接调 `pc.Chat()` 或自己的评判函数即可，加专项 builder 反而是抽象税。
- **`asSubProcess` 显式同步嵌套**：lynx 用 `runtime.AsChatTool[In, Out](platform, agentName)` 已覆盖（让上层 LLM 编排子 workflow），不再开新通道。

---

## 12. 持久化 / 多进程 — 抽象等价、开箱仍弱（按设计）

| 维度 | lynx | embabel |
|---|---|---|
| Process registry | ✅ in-memory + `PruneTerminalProcesses` ([runtime/platform.go](../runtime/platform.go)) | ✅ + persistent backends |
| Blackboard | ✅ in-memory + Spawn + Clear + Protect | ✅ + Redis / DB 后端 |
| BlackboardFactory 扩展点 | ✅ ([core/extension.go:110](../core/extension.go)) | ✅ `BlackboardProvider` |
| Context repository（跨 session）| ❌ —— 走 Blackboard 软持久化绕过（见 [`PERSISTENCE.md` §4.1](./PERSISTENCE.md)）| ✅ `ContextRepository` SPI / `InMemoryContextRepository` |
| AgentProcessRepository | ❌ —— P3 等用例 | ✅ `InMemoryAgentProcessRepository` |
| 实现指南 / 接入文档 | ✅ [`PERSISTENCE.md`](./PERSISTENCE.md)（Redis / SQL / WAL 三策略）| ✅ Spring Data 集成 |

**lynx 设计取舍**：所有持久化关注点都抽成扩展点，框架本身保持 in-memory（contributor 不必维护多个 backend）；用户按部署形态接入自己的 backend 实现。第六轮交付了 [`PERSISTENCE.md`](./PERSISTENCE.md) 作为接入指南。`AgentProcessRepository` 抽象仍未做（仅在等真用例时落 P3）。

---

## 13. 注解 / classpath scan — 故意分歧

lynx：DSL Builder ([dsl/builder.go](../dsl/builder.go))，编译期类型安全 + IDE rename 友好。embabel：`@Agent` / `@Action` / `@AchievesGoal` / `@LlmTool` 注解 + Spring scan。Go 无 Spring AOT，反射注册不合语言哲学。**永久分歧**。

---

## 14. 生态集成（A2A / RAG / Skills / Shell） — lynx 全空

| 模块 | lynx | embabel |
|---|---|---|
| **A2A**（agent-to-agent JSON-RPC server） | ❌ | ✅ `embabel-agent-a2a/`（独立模块） |
| **RAG** | ❌ | ✅ 5 子模块（core / pipeline / lucene / neo / tika），含 `LlmHyDEQueryGenerator` |
| **Skills**（YAML/Markdown 加载脚本工具，含 Docker 沙箱） | ❌ | ✅ `embabel-agent-skills/`（含 `DockerSkillScriptExecutionEngine`） |
| **Shell / CLI** | ❌ | ✅ `embabel-agent-shell/` + 4 个 personality（Severance / Hitchhiker / Star Wars / Monty Python） |
| Embedding service | ❌ | ✅ `embabel-agent-onnx/` 本地 + Spring AI 远端 |
| LLM provider 整合 | BYO `chat.Model` | Spring AI 全套 + `embabel-agent-openai/` 兼容层 |

**lynx 选择**：这些都走应用层 / 独立子模块，非 agent 内核。

---

## 15. 命名 / API ergonomics — lynx 更精简

| 维度 | lynx | embabel | 谁更好 |
|---|---|---|---|
| Top-level surface | 5 个 constructor（New / NewAction / NewCondition / GoalProducing / NewPlatform） + 1 个 MCP resolver + 1 个 Supervisor helper（`runtime.AsChatTool`） | 100+ Spring beans + DSL | **lynx** |
| Config 模式 | struct + `ApplyDefaults` | data class + sensible defaults | 等价 |
| 错误风格 | `package.Func: %w` 短句 | Kotlin exceptions | 各有所长 |
| 包数量（agent 框架） | 7（core / plan / runtime / event / dsl / hitl / agent）+ sibling `lynx/mcp` | 12+ Maven modules | **lynx** |
| 类型层次 | 浅 | 深（Spring 抽象类堆叠） | **lynx** |
| 学习曲线 | Go 用户低 | Spring 用户低；其他高 | 各有优势 |

---

## 16. 路线图（按 ROI 重排，2026-05-08 起）

| 优先级 | 项目 | 改动量 | 说明 |
|---|---|---|---|
| ~~已闭合~~ | ~~ToolLoop / MCP client+server / HTN / Reactive / Tool-级 HITL / Supervisor / Subagent waiting graceful-degrade / per-agent MCP 自动暴露 / WorkflowBuilder DSL（ScatterGather + RepeatUntil + Feedback）/ 持久化接入指南~~ | — | ✅ 全部落地 |
| **P2-4** | **Autonomy + LLM Goal Ranker**：自然语言 → 选 goal/agent + confidence cutoff | 中（新 `runtime.AutonomyExtension`） | 多 agent 部署后绕不开的问题 |
| **P2-6** | **LlmRanker（plan 排序）** | 小（`plan` 包内一个新 helper） | 让 plan 排序可被 LLM 干预 |
| **P3-7** | A2A 协议 | 大（独立子模块） | agent 间互联，等用例 |
| **P3-8** | RAG 子模块（独立 `lynx/rag` 仓） | 大 | 不挤进 agent 框架 |
| **P3-9** | Tool advanced policies：Matryoshka / OneShotPerLoop / Playbook / StateMachine | 中（`hitl/tool.go` 同形态特化） | 应用层模式特化 |
| **P3-10** | `runtime.AsChatToolFromAgent(agent *core.Agent)` 便利工厂 | 极小 | 配合用户尚未注册到 platform 的子 agent；目前可以等需求驱动 |
| **P3-11** | `runtime.ProcessRepository` + `AwaitableRepository` 抽象（跨重启 HITL） | 中 | 有用例后再做；当前可走 Blackboard 软持久化 |
| **P3-12** | RepeatUntilAcceptable / Consensus 一等公民 builder | 小（`workflow` 包加两个 helper） | 用户在 RepeatUntilAgent.Accept 闭包里手写 LLM 评判已足够 |

**P2 阶段已落地 WorkflowBuilder DSL + 持久化接入指南**。下一阶段聚焦 **P2-4 Autonomy + LLM Goal Ranker** —— 多 agent 部署后选 goal/agent 是绕不开的问题。

---

## 17. 故意不要做的事（不是缺口）

- **注解 / classpath scan agent 注册**——Go 心智模型不支持 magic
- **Spring DI 容器**——`ServiceProvider` 已够；不再加重 DI
- **Sync / Async 双套 API**——Go 的 `context.Context` + goroutine 已经覆盖；embabel 二分是 Spring artifact
- **AOT 模型分类 / SpEL 表达式条件**——过度工程
- **Megazord 注解（多 agent 合体）**——反射特化，做了也没人用
- **Personality / Shell 装饰**——lynx 没这文化
- **MCP server-side resources/prompts 暴露**——SDK 直接用，不再包一层（[`lynx/mcp/DESIGN.md` §8](../../mcp/DESIGN.md)）
- **多 LLM provider 内置整合**——`chat.Model` 是开放接口，用户自己挂
- **HTN 默认 task library**——HTN 本来就需要领域知识；framework 不替用户假设

---

## 18. 一句话总结

第五轮 4 条 P0–P1（HTN / Reactive / tool-级 HITL / Supervisor）+ 第六轮 2 条 P1（Subagent waiting graceful-degrade / per-agent MCP 自动暴露）+ 第六轮 P2 三件（WorkflowBuilder DSL / 持久化接入指南 / 配套优化）全部闭合。lynx 当前的 agent 内核 + 工具生态接通能力 + 多步组合模式跟 embabel **基线全档对齐**，剩下真缺口收敛到 **Autonomy + LLM Goal Ranker（P2）+ Tool advanced policies / A2A / RAG / 持久化参考实现（P3）**——都是 P2/P3 中等量级或独立子模块路线。下一阶段聚焦 **P2-4 Autonomy + LLM Goal Ranker**——多 agent 部署后"自然语言 → 选哪个 goal/agent + confidence 阈值"是绕不开的工程问题。
