# lynx/agent vs embabel-agent — 深度抽象对比

> **基线**
> - lynx HEAD `87d1421`（branch `main`，2026-05-21）；agent 子项目 ~10.2k Go LOC（不含 examples / tests），8 个内部包：`core` / `plan` / `runtime` / `event` / `hitl` / `toolpolicy` / `workflow` + 顶层 `agent.go` / `builder.go`
> - embabel-agent HEAD `24e98aca5`（main，2026-05-21）；~87.5k Kotlin LOC，71 个 Maven module，主 artifact `embabel-agent-api` ~50k LOC
>
> **与 lynx vs spring-ai 的对比关系**：lynx-agent 与 lynx-core 是 sibling go module；embabel-agent 依赖 spring-ai。所以本文聚焦 *agent 运行时层* 对比；core 层（chat / tool / vector store / RAG / MCP）对比见 [`SPRING_AI_COMPARISON.md`](./SPRING_AI_COMPARISON.md)。

---

## 0. TL;DR

**两侧使用高度相似的 GOAP（Goal-Oriented Action Planning）+ Blackboard 思想体系**。Action / Goal / Condition / 三值逻辑 / WorldState / Effect / Plan 在概念层一一对应——绝非巧合，GOAP 起源 FEAR 游戏 AI（2005），两侧明显共享同一理论基础。

**两侧的根本分歧不在 *做什么*，而在 *谁主导***：

| 分歧轴 | lynx-agent | embabel-agent |
|---|---|---|
| **宿主生态** | Go 标准库 + lynx-core | JVM（Kotlin / Java）+ Spring Boot 4 + spring-ai |
| **运行时容器** | 无 DI；显式 `runtime.Platform.Deploy(agent)` | Spring `ApplicationContext`；`@Agent` 类路径扫描 |
| **作者风格** | 单一 Builder DSL：`agent.New(...).Actions(...).Goals(...).Build()` | **双轨**：`@Agent` / `@Action` 注解式 **或** Kotlin `agent { flow {} transformation<I,O>{} }` DSL |
| **类型系统** | Go 泛型：`TypedAction[In, Out]` / `IOBinding[T]` 编译期安全 | Kotlin 反射 + `DomainType` 运行时系统（含 YAML / pack 动态类型）|
| **Planner 范式** | 3 种家族：GOAP A* / HTN / Reactive | 5 种实现：A*、OptimizingGoap、ConditionPlanner、UtilityPlanner、HybridUtilityPlanner |
| **LLM 调用入口** | `chat.Client`（lynx 自有）+ `core.PromptRunner` 包装器 | `PromptRunner`（包装 spring-ai `ChatModel`）|
| **Extension 模型** | 显式接口：`ActionInterceptor` / `ToolDecorator` / `AgentValidator` / `GoalApprover` | `AgenticEventListener` 事件总线 + Spring AOP |
| **HITL 形态** | 类型化 `Awaitable[T]` + `Confirmation` | `WAITING` 状态 + `AwaitableResponseException` 抛出 |
| **多 agent 协作** | `Platform` + child process + `runtime.AsChatTool` | `AgentPlatform` 聚合 scope + A2A REST gateway + MCP server |
| **持久化** | 无（内存 Process）| `AgentProcessRepository` SPI（含 EphemeralAgentProcess）|
| **可观测性** | OTel 原生 + slog bridge（agent runtime / planner / action 全量埋点）| Spring Boot Actuator + `AgenticEventListener` + Micrometer |

### lynx-agent 反超的方面

1. **HTN planner**（hierarchical task network——embabel 完全没有这一范式；embabel 的 5 个 planner 都是单层 action set 路径搜索 / 价值选择）
2. **类型安全工作流原语**（`TypedAction[In, Out]` 编译期检查；embabel 靠 Kotlin reflection + runtime DomainType）
3. **Workflow patterns 独立成 package**（`agent/workflow/`：sequence / parallel / scatter-gather / repeat-until / consensus / loop / feedback；embabel 散落在 DSL 函数中）
4. **HITL 类型化 `Awaitable[T]`**（编译期保证 resume 类型一致；embabel 走 exception + dynamic 类型）
5. **Extension 接口分层显式**（interceptor / decorator / validator / approver 四种角色明确；embabel 主要靠 event listener + Spring AOP）
6. **OTel-native 观测**（OTel 原生 + GenAI semconv 全量埋点；embabel 走 Spring Actuator + 自定义 event 系统）

### embabel 反超的方面

1. **5 个 planner 实现** vs lynx 3 个（embabel UtilityPlanner / HybridUtilityPlanner / ConditionPlanner 家族是 lynx 没有的范式）
2. **持久化 process 仓库**（`AgentProcessRepository` SPI；近期新增 `EphemeralAgentProcess` 短生命周期变体；lynx 无对应抽象）
3. **A2A REST gateway**（独立 module `embabel-agent-a2a`；lynx 无对等）
4. **`ToolGroup` 角色 + 权限模型**（`HOST_ACCESS` / `INTERNET_ACCESS` 二值标签；lynx 走自由组合）
5. **Spring 生态深度整合**（Actuator / metrics / shell / scheduling / a2a / mcpserver-security 开箱即用，~10 个 starter）
6. **`@Cost` 动态代价方法引用**（注解 + 反射方法调用；lynx 走 `CostFunc func(WorldState) float64` 函数式——更轻但少了"按方法名查 documentation"的便利）
7. **Conversation / ChatSession 抽象**（embabel 把对话作为一等公民；lynx 无对应抽象，靠 chatmemory + 应用层组合）
8. **LlmInvocationHistory + EmbeddingInvocationHistory 一等接口**（embabel `AgentProcess` 实现这两个接口，per-call cost tracking 内建；最近还加了跨 child cost aggregation `totalCost()`。lynx 散落在 OTel span 里，无一等接口）
9. **Global Guardrails 配置**（最近新增；lynx 走 middleware 用户自拼）
10. **RAG 多 backend**（`embabel-agent-rag-lucene` / `-neo` / `-tika`；lynx 的 RAG 走 lynx-core 通用 vector store + documentreaders）

---

## 1. 哲学定位

| 维度 | lynx-agent | embabel-agent |
|---|---|---|
| **目标用户** | 库用户、需要嵌入 agent 能力到 Go 服务的开发者 | Spring Boot 应用、需要快速搭建 agentic 系统的企业开发者 |
| **运行时形态** | in-process library | Spring Boot application context |
| **LLM 后端绑定** | 一切走 `chat.Model`，lynx-core 内置 21 个 chat provider | 一切走 spring-ai `ChatModel`，依赖 spring-ai 主干 8 个 + community |
| **作者门槛** | 必须懂 Go 泛型 + Builder pattern | 注解式入门 5 分钟；DSL 进阶需要 Kotlin 知识 |
| **企业整合** | 自带 OTel + MCP，其它整合自己写 | actuator / shell / metrics / a2a REST / mcp / scheduling 模块化出货 |
| **代码规模** | ~10k Go LOC | ~87k Kotlin LOC（含 starter / autoconfigure 矩阵）|

**lynx-agent 是 *agent-as-library*，embabel-agent 是 *agent-as-platform***。这条立场定义两侧绝大多数具体设计取舍。

---

## 2. 核心抽象同构 — Action / Goal / Condition

### 2.1 Action

| 属性 | lynx (`agent/core/action.go`) | embabel (`embabel-agent-api/.../core/Action.kt`) |
|---|---|---|
| 元数据 | `ActionMetadata { Name, Description, Inputs/Outputs (IOBinding[]), Pre/Effects (EffectSpec), CanRerun, QoS, Cost/Value (CostFunc) }` | `ActionMetadata { name, description, inputs/outputs (Set<IoBinding>), preconditions/effects (EffectSpec), canRerun, readOnly, qos: ActionQos, cost/value }` |
| 执行 | `Execute(ctx, pc ProcessContext) ActionStatus` | `execute(processContext): ActionStatus` |
| 重试 QoS | `ActionQoS { MaxAttempts: 5, BaseDelay: 10s, MaxDelay: 60s }` | `ActionQos`（同名同语义）|
| 状态返回 | `Success / Failed / Waiting / Paused / Cancelled` | `SUCCESS / FAILED / WAITING / PAUSED / CANCELLED` |
| 类型化输入 | **`TypedAction[In, Out]`** 泛型包装 | 注解 + 反射：`@Action` 方法签名直接成为类型契约 |

**类型契约的表达方式差异**：

lynx 走 **泛型类型安全**：
```go
type TypedAction[In, Out any] struct {
    metadata ActionMetadata
    fn       func(ctx context.Context, pc *ProcessContext, in In) (Out, ActionStatus)
}
```
编译期就知道 In/Out 类型，IOBinding 由 reflect 自动生成。

embabel 走 **注解 + 反射**：
```kotlin
@Action(pre = ["hasInput"], post = ["analysisDone"])
fun analyzeContent(input: UserInput, blackboard: Blackboard): AnalysisResult { ... }
```
`AgentMetadataReader` 启动时扫描方法签名提取 `IoBinding`，运行时反射调用。

**取舍**：lynx 编译期类型安全胜出，但写起来 verbose（要手填 metadata）；embabel 简洁但运行时类型错误（参数不匹配、blackboard 缺值）启动时才暴露。

### 2.2 Goal

字段、Value（效用函数）、Export（→ MCP）、类型化 Goal 两侧形态几乎完全同构。差异仅在 `Value` 函数式 vs 方法引用：lynx 是函数指针 `CostFunc`，embabel 可以走 `@Cost` 注解的方法引用 + 反射。

### 2.3 Condition + 三值逻辑

两侧都用 **三值逻辑（true / false / unknown）** 处理条件不可决定的情况。lynx 是 `ConditionDetermination` enum + helper；embabel 是 `ConditionDetermination` enum + extension methods。形态对齐。

---

## 3. Planner 子系统 — 多范式 vs 多算法

### 3.1 实现矩阵

| Planner 范式 | lynx | embabel |
|---|---|---|
| **GOAP A* search** | ✅ `agent/plan/planner/goap/`（A* + 启发式 = 未满足 cond 数 + 状态去重 + relevance 后向剪枝）| ✅ `plan/goap/astar/AStarGoapPlanner`（A* + backward + forward 优化）|
| **GOAP optimized variant** | — | ✅ `plan/goap/OptimizingGoapPlanner`（避免 A* 完整搜索，启发式剪枝）|
| **HTN（hierarchical task network）** | ✅ `agent/plan/planner/htn/`（任务分解 / 方法选择 / 回溯）| ❌ |
| **Reactive（无搜索，每 tick 选最高价值可行 action）** | ✅ `agent/plan/planner/reactive/`（含 0-progress 守卫，拒绝不推进 goal 的 action）| ⚠️ ConditionPlanner 家族最接近，但保留 condition / world state 模型 |
| **Utility-based（按 netValue 排序）** | ⚠️ 由 reactive + value func 组合 | ✅ `plan/utility/UtilityPlanner`（pure value-based picking）|
| **Hybrid（utility + 目标终止）** | — | ✅ **`plan/utility/HybridUtilityPlanner`**（commit `24e98aca5`，2026-05）|
| **Condition-based**（DSL 直写 condition→action map）| — | ✅ `plan/common/condition/`（ConditionPlanner / ConditionPlanningSystem 家族）|

**总结**：两侧的 planner 哲学不同——

- **lynx 走范式多样性**：3 个完全不同的规划家族（搜索 / 分层 / 反应），HTN 是 embabel 完全没有的；每个家族解决不同问题（短链条用 reactive，长链条用 GOAP，分层任务用 HTN）
- **embabel 走 GOAP-derived 多样性**：5 个实现都是"single-layer action picking"系列（A* / optimized / utility / hybrid / condition），覆盖 GOAP 不同时间复杂度 + 不同终止策略

**没有谁绝对反超**——lynx 在范式广度上多一种（HTN），embabel 在 GOAP 家族内的优化变体多两种（OptimizingGoap + HybridUtility）。

### 3.2 Planner 接口

```go
// lynx: agent/plan/planner.go
type Planner interface {
    Extension
    PlanToGoal(ctx, start WorldState, system PlanningSystem, goal Goal, options PlanOptions) (*Plan, error)
}
```

```kotlin
// embabel: plan/Planner.kt
interface Planner<S : PlanningSystem, W : WorldState, P : Plan> {
    fun worldState(): W
    fun planToGoal(actions, goal): P?
    fun plansToGoals(system): List<P>
    fun bestValuePlanToAnyGoal(system, excludedActionNames): P?
    fun prune(planningSystem): S
}
```

embabel 接口是泛型化（state / system / plan 三参数），lynx 是单态（state / system 走参数）。embabel 形式更对称（每个 planner 自带 worldState() 访问），lynx 更简（一个方法走天下）。

### 3.3 Plan

| | lynx | embabel |
|---|---|---|
| 字段 | `Actions[], Goal` | `actions: List<Action>, goal: Goal` |
| 代价 / 价值 | `Cost(ws)` / `Value(ws)` / `NetValue(ws)` | `cost(state)` / `netValue(state)` |
| 完成判定 | `IsComplete()` | `isComplete()` |

**完全同构**。值对象、不可变、可序列化、可比较。

---

## 4. Blackboard 子系统

| 维度 | lynx | embabel |
|---|---|---|
| 接口分层 | **ISP**：Reader + Writer + Extension 三接口拼成 | 单接口 `Blackboard : Bindable, MayHaveLastResult, HasInfoString` |
| 按 key 存取 | `Set(key, value)` / `Get(key)` | `set(key, value)` / `get(name)` |
| 按类型存取 | `GetTyped[T](bb)` 泛型 | `last<T>(Class<T>)` / `objectsOfType<T>(Class<T>)` |
| 默认对象 | `AddObject(value)` | `addObject(value)` / `+= value` |
| Condition 直接存取 | `SetCondition / GetCondition` | `setCondition / getCondition` |
| 派生 / 子 board | `Spawn() → Blackboard` | `spawn(): Blackboard` |
| 受保护绑定 | — | **`bindProtected(key, value)`**（跨 state-machine reset 存活）|
| 默认变量名 | `"it"` | `"it"`（`IoBinding.DEFAULT_BINDING`）|
| 最近结果别名 | — | **`"lastResult"`**（`IoBinding.LAST_RESULT_BINDING`）|

**类型解析**：lynx 用 Go 泛型 + reflect，embabel 用三级匹配（JVM class hierarchy → DomainType hierarchy → Tagged Map for pack types）。embabel 支持运行时定义的动态类型（来自 YAML / pack 文件），lynx 不需要这个（纯编译路线）。

**`bindProtected` 设计意图**：embabel 用于 *跨 state-machine reset 持久化*——当 action 用 `clearBlackboard=true` 清空 blackboard 时，protected binding 不会被清掉。典型用例：对话历史跨多个工作流阶段存活。lynx 无对应概念（action 设计倾向"effect 只增不减"，没有内建"清空 blackboard"机制）。

---

## 5. AgentProcess 生命周期

### 5.1 状态机

| 状态 | lynx | embabel |
|---|---|---|
| 初始 | （Platform 创建后立即 Running）| `NOT_STARTED` |
| 运行 | `Running` | `RUNNING` |
| 完成 | `Complete` | `COMPLETED` |
| 失败 | `Failed` | `FAILED` |
| 阻塞（无可行 action）| `Stuck` | `STUCK` |
| 等待外部输入 | `Paused`（HITL）| `WAITING` |
| 用户终止 | （由 Platform.Kill 触发）| `KILLED` |
| 策略终止 | budget / termination signal | `TERMINATED` |
| 调度暂停 | 同 Paused | `PAUSED` |

embabel 状态码 9 种、lynx 5 种 + signal 组合。embabel 显式区分 KILLED / TERMINATED / PAUSED / WAITING，lynx 用更少的状态码达到类似语义。

### 5.2 LLM / Embedding 调用历史

embabel `AgentProcess` 实现 `LlmInvocationHistory` + `EmbeddingInvocationHistory` 接口，**每次 LLM / embedding 调用作为一等公民记录**（最近 commit `a31cef363` 新增 per-call cost tracking）。lynx 的对应能力散落在 OTel span 里，没有一等接口——这是 embabel 的明确反超。

### 5.3 Budget / Cost

| | lynx (`runtime/process_budget.go`) | embabel (`ProcessOptions.Budget`) |
|---|---|---|
| 字段 | Cost (USD) / Tokens / Actions | cost ($) / actions / tokens |
| 跨 child 聚合 | — | `totalCost()` 自动求和整个 child tree |

embabel 的 budget 跨 child process 聚合 lynx 没做。

### 5.4 EphemeralAgentProcess（embabel 新增）

embabel `8247bc512` 引入 `EphemeralAgentProcess`——短生命周期 process，不进 repository，跑完即丢。lynx 因 in-memory only，所有 process 都"自然 ephemeral"。

---

## 6. ProcessContext / Operation context

| | lynx (`agent/core/process_context.go`) | embabel (`OperationContext`) |
|---|---|---|
| 注入位置 | 每个 action 的 `Execute(ctx, pc)` 第二参数 | `@Action` 方法可选注入 |
| 提供能力 | Blackboard + Logger + Platform 引用 + Chat client + Helpers | Blackboard + Process status + Process options + LLM service |
| LLM 入口 | `pc.Chat()` / `pc.ChatWithActionTools()` | `context.promptRunner()` |
| HITL 入口 | `pc.Await(awaitable Awaitable[T])` | `context.await(awaitable)` |
| 子 process | `pc.Platform().CreateChildProcess(...)` | `context.createChildProcess(...)` |

两侧 ProcessContext 形态高度同构——这是"agent runtime as system call interface"的共同心智模型。

---

## 7. LLM 调用入口 — PromptRunner 等价

| | lynx | embabel |
|---|---|---|
| 入口 | `core.PromptRunner` + `pc.Chat()` / `pc.ChatWithActionTools()` | `PromptRunner`（包装 spring-ai `ChatModel`）|
| Tool 注入 | `WithActionTools()` 自动注入 action 的 ToolGroup | `withTools(...)` 注入 |
| Blackboard 注入 | 显式 — 用户从 pc 读 | 显式 — `context.blackboard` 注入 |
| Process metadata | 显式 | 显式 |
| 链式 API | `chat.Client.Chat().With...()` | `promptRunner.transform(prompt).output(Type::class)` |

lynx `core.PromptRunner`（`agent/core/prompt_runner.go`）已落地，与 embabel `PromptRunner` 形态对齐。两侧 ergonomics 接近。

---

## 8. Tool / ToolGroup

| 维度 | lynx | embabel |
|---|---|---|
| Tool 注册 | `chat.Tool` 接口 + agent action 携带 `Tools []chat.Tool` | `@Tool` 注解 / `MethodToolCallback` / `ToolGroup` 注册 |
| ToolGroup | lynx 用普通切片 | `ToolGroup` 一等公民，带 `roles`（角色字符串 set）+ `permissions`（HOST_ACCESS / INTERNET_ACCESS）|
| Tool 装饰器 | `core.ToolDecorator` 接口 + `agent/toolpolicy/` 包（OnceOnly / Unlocked / RateLimit）| 走 Spring AOP（运行时代理）|
| ToolGroup 解析 | `core.ToolGroupResolver` extension | `ToolGroupResolverService` Bean |
| MCP 消费 | `mcp.Provider` + `runtime.AsMCPTool` | `embabel-agent-mcp-server`（独立 module）|

**embabel `ToolGroup.permissions` 是 lynx 没有的二值安全标签**，决定 tool 能否访问主机 / 互联网。lynx 走的是 `ToolPolicy` 装饰器路线——更灵活（任何策略都可以包装），但安全模型不是 first-class。

---

## 9. Extension / 扩展机制

| 角色 | lynx 接口 | embabel 等价物 |
|---|---|---|
| 事件监听 | `EventListener` extension | `AgenticEventListener` Bean |
| Action 围绕拦截 | `ActionInterceptor`（around） | Spring AOP `@Around` |
| Tool 装饰 | `ToolDecorator` | Spring AOP 装饰 |
| Agent 启动校验 | `AgentValidator` | `ContextRefreshedEvent` + AgentValidationManager |
| Goal 审批 | `GoalApprover` | 自行实现 |
| Tool group 解析 | `ToolGroupResolver` | `ToolGroupResolverService` |
| ID 生成 | `IDGenerator` | `IdGenerator` Bean |
| Blackboard 工厂 | `Blackboard` interface 自身可替换 | `BlackboardFactory` Bean |
| Planner 工厂 | `PlannerFactory` extension | 平台层 Bean 替换 |

lynx 的 `core.Extension` 显式角色分层 vs embabel 的 Spring Bean + AOP——lynx 更明确（看接口就知道能扩什么），embabel 更灵活（AOP 可以拦截任何方法但配置复杂）。

---

## 10. Workflow 模式

| 模式 | lynx (`agent/workflow/`) | embabel（DSL 函数）|
|---|---|---|
| Sequence | ✅ | DSL 自然顺序 |
| Parallel | ✅ | `parallel { }` |
| Loop | ✅ | `repeat { }` |
| RepeatUntil | ✅ | `repeatUntil { }` |
| RepeatUntilAcceptable | ✅ | — |
| Consensus | ✅ | — |
| ScatterGather | ✅ | — |
| Feedback | ✅ `Feedback[Result, Score]` | — |
| History | ✅ `History[T]` | — |
| ResultList | ✅ `ResultList[E]` | — |

**lynx 把 workflow patterns 抽成独立 package**（7 个具名模式 + 3 个辅助类型），embabel 把它们留在 DSL 函数层。lynx 路线更"代码作为接口"，embabel 路线更"代码作为脚本"。

---

## 11. HITL（Human-in-the-Loop）

| | lynx (`agent/hitl/`) | embabel (`core/hitl/`) |
|---|---|---|
| 触发机制 | action 返回 `ActionStatus.Waiting` + 在 ctx 上塞 `Awaitable[T]` | action 抛 `AwaitableResponseException(Awaitable<T>)` |
| 类型化 resume | **`Awaitable[T]`**（编译期类型一致） | `Awaitable<T>` + 运行时类型校验 |
| 内置类型 | `Confirmation` / `FormRequest` / 自定义 | 由 spring-ai 提供 |
| Resume API | `Platform.Resume(processID, T)` | `agentProcess.respondTo(awaitable, response)` |

lynx 走 exception-free 路线（return-value），embabel 走 exception-based（控制流跳出）。Go 没有 checked exception，return-value 是唯一 idiomatic 选择；Kotlin 有 throw-based 控制流传统，embabel 走它路线合理。

---

## 12. 多 agent 协作

| 维度 | lynx | embabel |
|---|---|---|
| 父→子 process 创建 | `Platform.CreateChildProcess(agentName, opts)` | `AgentProcess.createChildProcess(agent)` |
| LLM-orchestrated multi-agent | `runtime.AsChatTool(agent)` 把 agent 暴露成 `chat.Tool` 注入父 LLM tool loop | `Subagent` tool + `@Subagent` 注解 |
| 跨 agent 自动规划 | — | `AgentPlatform : AgentScope` 把所有 deployed agents 看作一个大 ActionSource 联合规划 |
| Sub-process listeners | （独立）| `ProcessOptions.listeners` 自动从父 process 继承（最近 commit `1042d5870` 修复）|
| 同步 / 异步执行 | 同步（goroutine + child process）| `Subagent` 同步 / `@Subagent` 注解可异步 |
| Cost 跨子聚合 | — | `totalCost()` 自动求和整个 child tree |

**embabel 反超**：`AgentPlatform : AgentScope` 让平台层直接做"跨 agent 联合规划"——LLM 不参与，纯 GOAP 在所有 agent 的 union action set 上跑 A*。lynx 当前需要走 LLM tool loop（`AsChatTool`）让 LLM 来选 sub-agent。

---

## 13. 作者风格 — 注解 / DSL / Builder

| 风格 | lynx | embabel |
|---|---|---|
| 注解风格 | — | `@Agent`、`@Action`、`@Condition`、`@Cost`、`@Goal`、`@Subagent` 等 |
| Kotlin DSL | — | `agent("name") { action { } flow { } condition { } }` 嵌套 lambda |
| 程序化 Builder | `agent.New("name").Actions(...).Goals(...).Conditions(...).Build()` | — |
| 反射启动校验 | （所有 metadata 编译期可见）| `AgentMetadataReader` 反射扫描 |

注解 + DSL 双轨是 embabel 的核心 ergonomics——5 行 Kotlin 就能搭一个 agent。lynx 因 Go 无 runtime annotation，没有等价路线；Builder API 是唯一选择。

---

## 14. Conversation / ChatSession（embabel 独有）

embabel `embabel-agent-api/.../api/channel/` 里把 *对话* 作为一等公民：`ChatSession` / `Channel` / `MessageChannel`——agent 可以拿到对话上下文，多轮对话有形式化语义。

lynx 无对应抽象——靠 `chatmemory.Store` + 应用层组合实现对话语义。**这是 embabel 反超的明确点**——聊天机器人场景下 embabel ergonomics 优势明显。

---

## 15. 持久化

| 能力 | lynx | embabel |
|---|---|---|
| Process 持久化 | **无**（in-memory only）| `AgentProcessRepository` interface + 多 backend |
| Ephemeral process | n/a（全都是 ephemeral）| `EphemeralAgentProcess`（新加，短生命周期变体）|
| Blackboard 持久化 | — | 同 process |
| Memory 持久化（chat memory）| ✅ 顶层 `chatmemory/` 6 个 backend | `ChatMemoryRepository` 5 个 backend（spring-ai 层）|

**Process 持久化是 embabel 明确反超项**——任何要做"agent 长跑 + 中断恢复 + 跨节点迁移"的场景，lynx 当前需要应用层从头实现。讽刺的是：lynx 已经有 27 个 vectorstore 后端 + 6 个 chatmemory 后端，**改写一个成 `process.Store` 只需 ~150 LOC**——成本曲线已被前两轮消化。

---

## 16. Observability

| 维度 | lynx | embabel |
|---|---|---|
| 标准对接 | OTel 原生（GenAI semconv + DB semconv）| Spring Boot Actuator + Micrometer Observation + `AgenticEventListener` 事件总线 |
| Trace 粒度 | OTel span 全覆盖（chat / embedding / tool / RAG / MCP / **agent runtime**（含 HTN / Reactive / GOAP planner / tick / action）/ vectorstore / chatmemory）| event-driven：`LlmInvocationEvent` / `AgentProcessEvent` / `ActionInvocationEvent` |
| Cost 报告 | OTel metric（自己 aggregate）| `totalCostInfoString(verbose)` 一键人类可读报告 |
| Per-call history | 走 OTel span | **`LlmInvocationHistory` / `EmbeddingInvocationHistory` 一等接口**（embabel 反超） |
| 全局 Guardrails | （middleware 自拼）| **Global Guardrails 配置**（最近 commit `e12269e77` 新增）|
| 启动期校验观察 | 自定义 logger | Spring `ApplicationReadyEvent` |

**lynx 反超**：OTel 是 Go 世界事实标准 + 已全量覆盖。**embabel 反超**：LLM/Embedding 调用历史的一等接口形态 + cost 报告 ergonomics。

---

## 17. 完整能力矩阵

| 能力 | lynx-agent | embabel-agent |
|---|---|---|
| GOAP A* planner | ✅ | ✅ |
| GOAP optimized variant | — | ✅ `OptimizingGoapPlanner` |
| HTN planner | ✅ | — |
| Reactive planner | ✅ | ⚠️ ConditionPlanner 接近 |
| Utility planner | — | ✅ |
| Hybrid utility planner | — | ✅（commit `24e98aca5`，2026-05）|
| Condition-based planner | — | ✅ ConditionPlanner 家族 |
| Three-valued logic (T/F/U) | ✅ | ✅ |
| WorldState + EffectSpec | ✅ | ✅ |
| Action.preconditions/effects | ✅ | ✅ |
| Goal.value（效用函数）| ✅ | ✅ |
| Goal.export（→ MCP）| ✅ | ✅ |
| Blackboard | ✅ | ✅ |
| Blackboard.bindProtected | — | ✅ |
| Blackboard.byType 取值 | ✅ 泛型 | ✅ 反射 |
| Typed IO binding | ✅ 编译期 | — 运行时字符串 |
| TypedAction[In, Out] | ✅ | 注解 + 反射 |
| LLM-as-Condition | ✅ `PromptCondition` | — 自拼 |
| Dynamic cost/value | ✅ `CostFunc` | ✅ `@Cost` 注解 |
| Process 状态码 | 5 + signal | 9 |
| Process budget（cost/tokens/actions）| ✅ | ✅ |
| Process 跨子 cost 聚合 | — | ✅ `totalCost()` |
| Process 持久化 | — | ✅ `AgentProcessRepository` |
| EphemeralAgentProcess | n/a（全 ephemeral）| ✅ |
| HITL Awaitable | ✅ 类型化 | ✅ exception-based |
| Workflow patterns 库 | ✅ 7 类独立 package | ⚠️ DSL 3–4 类 |
| Tool registry | ✅ | ✅ |
| ToolGroup 角色 | ✅ string | ✅ role + permission |
| ToolGroup 权限模型 | — | ✅ HOST/INTERNET 二值 |
| ToolDecorator（动作维度）| ✅ | — Spring AOP |
| ToolPolicy（rate-limit/once）| ✅ 独立 package | — |
| ActionInterceptor（around）| ✅ | — Spring AOP |
| GoalApprover | ✅ | — |
| AgentValidator | ✅ | Spring `ContextRefreshedEvent` |
| Event listener | ✅（OTel + extension）| ✅ `AgenticEventListener` |
| Annotation-based authoring | — Go 限制 | ✅ `@Agent` / `@Action` / `@Cost` |
| DSL authoring | Builder | Kotlin DSL `agent { }` |
| ChatSession 抽象 | — | ✅ |
| Multi-agent platform | `Platform` + child process | `AgentPlatform : AgentScope`（跨 agent 自动规划）|
| A2A REST gateway | — | ✅ 独立 module |
| MCP server | ✅ | ✅ |
| MCP tool consumption | ✅ | ✅ |
| MCP Streamable HTTP transport | ✅ | ✅ |
| MCP reverse capabilities（sampling/elicit/progress/log/ping）| ✅ | ✅ |
| LlmInvocationHistory 一等接口 | — 走 OTel span | ✅ |
| EmbeddingInvocationHistory 一等接口 | — | ✅ |
| Global Guardrails 配置 | — middleware 自拼 | ✅ |
| Cost 跨 child aggregation | — | ✅ |
| Observability | OTel 原生 + GenAI/DB semconv | Spring Actuator + Event |
| CLI shell | — | ✅ 独立 module |
| Skills 模块（YAML + Docker）| — | ✅ `embabel-agent-skills/` |
| RAG 独立 backend（Lucene/Neo/Tika）| 走 lynx-core RAG | ✅ 4 个 RAG backend 独立 module |

---

## 18. 剩余 gap 路线图

P0 / P1 的硬刚需已大部闭合。剩余按 ROI：

### 🔴 P1 — 生产硬刚需

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 1 | **`Process.Store` 持久化 SPI**（复用 chatmemory 的 6 个 driver）| 中（~300 LOC + 1-2 个 backend ~150 LOC）| 长跑 / 中断恢复 / 跨节点 agent |
| 2 | **`Session` / 对话生命周期抽象** | 中（~200 LOC）| 对标 embabel `ChatSession`；聊天机器人场景必需 |
| 3 | **`Blackboard.BindProtected`** | 低（~30 LOC）| `clearBlackboard` 场景必需 |

### 🟡 P2 — 闭合 embabel 已有设计

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 4 | **`LlmInvocationHistory` / `EmbeddingInvocationHistory` 一等接口** | 中（~200 LOC）| cost 报告 / debugging / replay |
| 5 | **`totalCost()` 跨 child process 聚合** | 低（~50 LOC）| 多 agent 编排时的 cost 可见性 |
| 6 | **`AgentScope` 聚合规划**（多 agent 联合 GOAP）| 高（~400 LOC + 测试）| 对标 embabel `AgentPlatform : AgentScope` |
| 7 | **`ToolGroupPermission` 安全模型**（HOST/INTERNET 标签）| 低（~50 LOC）| 平台级安全策略基础 |
| 8 | **Global Guardrails 配置** | 低（~100 LOC，复用 `middleware.Safeguard`）| 全局兜底，避免每个 agent 重复配 |
| 9 | **UtilityPlanner / HybridUtilityPlanner 移植** | 中（~300 LOC）| 闭合 planner 范式 gap；某些 reducer-style 工作流需要 |

### 🟢 P3 — 长尾

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 10 | Action 注解推导（`go:generate` + AST 分析）| 高（~800 LOC + 测试）| 减少 Builder boilerplate ~50%；但 Go 路线下值得性存疑 |
| 11 | Skills 模块（YAML + Docker 执行）| 高 | 用例分布远端 |
| 12 | A2A REST gateway | 中 | MCP 已经覆盖 90% 用例 |

### ❌ 故意不做（"为什么不抄"）

| # | embabel 有但 lynx 不该抄 | 原因 |
|---|---|---|
| A | `@Agent` / `@Action` 完整注解 DSL | Go 无 runtime annotation；强行模拟引入复杂代码生成层 |
| B | Spring ApplicationContext / DI 容器 | lynx 是 library，不是 framework |
| C | 单 planner 路线（删 HTN / Reactive）| lynx 已投入 HTN/Reactive，多 planner 是反超点 |
| D | Spring Actuator 集成 | OTel 是 Go 世界标准，重新对接 actuator 反而割裂 |
| E | Kotlin DSL 风格的"嵌套 lambda" | Go 不擅长嵌套 lambda；`workflow/` package 风格已足够 idiomatic |
| F | Exception-based HITL | Go 无 checked exception；return-value 路线更 idiomatic |

---

## 19. 一句话定档

**lynx-agent 用 *Go-native, OTel-first, MCP-first 的 thin-library 路线*，在 *agent 范式* 上比 embabel-agent 多一个 HTN planner + 类型安全工作流原语 + ToolPolicy + Extension 四角色分层；embabel 在 *GOAP 家族广度*（A* / Optimizing / Utility / Hybrid / Condition 五个）+ 持久化 + ChatSession + LlmInvocationHistory + ToolGroup 权限 + Spring 生态深度整合上反超**。

剩余 ROI 集中在三块：**process 持久化**（P1，复用 chatmemory 后端低成本）+ **Session 对话抽象**（P1）+ **per-call cost tracking 一等接口**（P2）。这三件做完，lynx-agent 对 embabel 的硬差距实质闭合，同时保留 thin-library 哲学不动摇。

---

## 附录 — 关键文件索引

### lynx-agent

| 抽象 | 文件 |
|---|---|
| Agent | `agent/core/agent.go` |
| Action | `agent/core/action.go`, `agent/core/action_typed.go` |
| Goal | `agent/core/goal.go` |
| Condition | `agent/core/condition.go` |
| Blackboard | `agent/core/blackboard.go` |
| IOBinding | `agent/core/io_binding.go` |
| Planner | `agent/plan/planner.go` |
| GOAP A* | `agent/plan/planner/goap/` |
| HTN | `agent/plan/planner/htn/` |
| Reactive | `agent/plan/planner/reactive/` |
| Platform | `agent/runtime/platform.go` |
| AgentProcess | `agent/runtime/agent_process.go` |
| ProcessContext | `agent/runtime/process_context.go` |
| PromptRunner | `agent/core/prompt_runner.go` |
| Extension | `agent/core/extension.go` |
| Workflow | `agent/workflow/` |
| ToolPolicy | `agent/toolpolicy/` |
| HITL | `agent/hitl/` |
| Builder DSL | `agent/agent.go`, `agent/builder.go` |

### embabel-agent

| 抽象 | 文件（`embabel-agent-api/src/main/kotlin/com/embabel/...`） |
|---|---|
| Agent | `agent/core/Agent.kt` |
| Action | `agent/core/Action.kt` |
| Goal | `agent/core/Goal.kt` |
| Condition | `agent/core/Condition.kt` |
| Blackboard | `agent/core/Blackboard.kt` |
| AgentProcess | `agent/core/AgentProcess.kt` |
| EphemeralAgentProcess | `agent/core/internal/EphemeralAgentProcess.kt` |
| AgentPlatform | `agent/core/AgentPlatform.kt` |
| A* GOAP Planner | `plan/goap/astar/AStarGoapPlanner.kt` |
| Optimizing GOAP | `plan/goap/OptimizingGoapPlanner.kt` |
| Condition Planner | `plan/common/condition/ConditionPlanner.kt` |
| Utility Planner | `plan/utility/UtilityPlanner.kt` |
| **Hybrid Utility Planner** | `plan/utility/HybridUtilityPlanner.kt`（NEW） |
| PromptRunner | `agent/api/common/PromptRunner.kt` |
| LlmInvocationHistory | `agent/core/LlmInvocation.kt` |
| EmbeddingInvocationHistory | `agent/core/EmbeddingInvocation.kt` |
| Annotations | `agent/api/annotation/annotations.kt` |
| Kotlin DSL | `agent/api/dsl/AgentBuilder.kt` |
| ToolGroup | `agent/core/ToolGroup.kt` |
| HITL | `agent/core/hitl/` |
| Spring AI Bridge | `agent/spi/support/springai/SpringAiLlmService.kt` |

---

*对比结束。lynx HEAD `87d1421`，对照 embabel-agent main HEAD `24e98aca5`，2026-05-21。*
*配套文档：[`SPRING_AI_COMPARISON.md`](./SPRING_AI_COMPARISON.md)（lynx-core 对 spring-ai 的对比）。*
