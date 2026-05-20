# 多角度对比：lynx/agent vs embabel-agent

> **基线**
> - lynx HEAD `c8797cf`（branch `feat/core-spring-ai-comparison`，2026-05-14）；agent 子项目 ~15.4k Go LOC，9 个内部包（`core/` / `plan/` / `runtime/` / `event/` / `hitl/` / `toolpolicy/` / `workflow/` / `examples/` + 顶层 `agent.go` / `builder.go`），Go 1.26
> - embabel-agent HEAD `01a172340`（main，2026-05-11）；~244k LOC（Kotlin 主导），16 个核心 Maven module + 7 个 autoconfigure + 20+ 个 starter；其中 `embabel-agent-api` 自身 58.6k LOC
>
> 本文按 9 个角度组织：哲学 / 声明方式 / 核心抽象 / 规划 / Extension / Tool 系统 / HITL+Autonomy+Workflow / 生态广度 / 战略 gap。

---

## 0. TL;DR

**两者从同一份心智模型出发**：GOAP 规划器 + 黑板（blackboard）+ OODA 循环 + Action/Goal/Condition 三元组。**心智模型一致，工程化形态截然不同**：

- embabel 是 **Spring Boot 应用 framework**——`@EnableEmbabelAgent` 一引入，autoconfigure 把 Shell REPL / A2A server / MCP server / 多个 LLM provider / Observability / RAG / Skills 全部接好。开箱一个可运行的 agent 主机进程。
- lynx/agent 是 **可嵌入的 Go 库**——`agent.New(...).Build()` + `runtime.NewPlatform(...)`，没有 shell、没有 a2a、没有内建 server；运维 / 部署 / 持久化都交给 host app。

**lynx 当前在 8 条线上超过 embabel 主干**：

| # | 反超点 | 简述 |
|---|---|---|
| 1 | **HTN planner 一等公民** | `plan/planner/htn/` 完整实现 hierarchical task network；embabel 的 plan/ 下只有 GOAP + Utility，没有 HTN |
| 2 | **GOAP 后向相关性剪枝** | `plan/planner/goap/relevance.go` 用 STRIPS regression 反向追溯 action effects，A* 搜索前先剪枝；embabel `OptimizingGoapPlanner` 直接前向 A* |
| 3 | **Reactive planner 0-progress 守卫** | `plan/planner/reactive/` 显式拒绝不推进目标的 action；embabel `UtilityPlanner` 仅按 netValue 排序，**可能死循环** |
| 4 | **类型安全 `typedAction[In, Out]`** | 反射 + 泛型从 Go struct 静态推导 IOBinding，编译期类型检查；embabel `@Action` 注解走 reflection，类型擦除到运行时 |
| 5 | **ISP 拆 Blackboard** | `BlackboardReader` / `BlackboardWriter` 拆分；embabel `Blackboard: Bindable, MayHaveLastResult, HasInfoString` 单一巨接口 |
| 6 | **`iter.Seq2` 原生流式** | 流式 chat / tool loop 走 Go 1.23 标准库迭代器；embabel 自己设计 `LlmMessageStreamer` SPI 抽 Reactor Flux |
| 7 | **goroutine 原生并发** | `ConcurrentAgentProcess` 直接 goroutines + `context.Context` + `errgroup`；embabel 走 kotlinx.coroutines + `CompletableFuture` 混合 |
| 8 | **7 个 workflow primitives** | `Sequence / Parallel / Loop / RepeatUntil / RepeatUntilAcceptable / Consensus / ScatterGather` + `Feedback / History[T] / ResultList[E]`；embabel 没有专门的 workflow 抽象层，要靠多 action 拼或 SupervisorAgent |

**lynx 关键 gap**（按 ROI 顺序）：

| # | Gap | 简述 |
|---|---|---|
| 1 | **持久化 SPI** | embabel `ContextRepository` + `AgentProcessRepository` 两条 SPI，lynx 故意全内存，跨进程恢复需 host 自己拼 |
| 2 | **ToolLoop SPI 配套** | embabel `ToolLoop` + `ToolInjectionStrategy`（Full/Chaining/Unfolding/Single）+ `LoopMemo` + `OneShotPerLoopTool` + `EmptyResponsePolicy` + `ToolNotFoundPolicy` 全套；lynx 只有 `chat.ToolMiddleware` 隐式循环 + `toolpolicy.OnceOnly`/`Unlocked` 两个 decorator |
| 3 | **Goal.Export + MCP 暴露** | embabel 把 Achievable Goal 转 ToolGroup → MCP 工具一气呵成；lynx 当前 `AsMCPTool` 是 agent-level 而非 goal-level |
| 4 | **AgentValidationManager 多层校验** | embabel 启动时跑结构校验 + GOAP 可达性 + 命名冲突 + DSL 完整性多层检查；lynx 只有运行时报错 |
| 5 | **SupervisorAgent / 多 agent 编排** | embabel 一等公民（LLM 驱动多 agent 选择）；lynx 用 `Platform.CreateChildProcess` + workflow primitives 拼，但没有"自动选 agent"层 |
| 6 | **Skills 模块**（YAML + Docker/Process 引擎）| embabel `embabel-agent-skills/` 完整；lynx 无对应（按设计放 `agents/skills/` 外挂） |
| 7 | **A2A 协议 server** | embabel 内置；lynx 走 MCP 替代 |
| 8 | **Per-call cost tracking 细化**（每个 LLM/embedding 调用）| embabel 最近 commit 加了 `LlmInvocation` / `EmbeddingInvocation` per-call 历史；lynx 当前只追踪滚动总和 |

---

## 1. 哲学 / 范畴

**根本立场分歧** ——

| 维度 | embabel-agent | lynx/agent |
|---|---|---|
| **角色** | Spring Boot **应用 framework** | 可嵌入的 Go **库** |
| **分发单元** | Spring Boot 应用（16 个 Maven module，按需选用 starter）| Go 包（一个 `agent/` 子目录，9 个内部包）|
| **DI / 组装** | Spring DI + `@AutoConfiguration` + `@Conditional` + `@Bean` | 显式构造 + `core.Extension` 注册表 |
| **自带前端** | Shell REPL（`embabel-agent-shell`）+ A2A server（`embabel-agent-a2a`）+ MCP server（`embabel-agent-mcp`）+ REST endpoints（autoconfigure 触发）| 没有；作为库被 host 应用嵌入 |
| **持久化** | `BlackboardProvider` + `ContextRepository` + `AgentProcessRepository` SPI（in-memory 实现 + 留口给 Spring Data）| 只有 `core.Blackboard` extension（prototype + `Spawn()`）；process 注册表故意是内存的 |
| **并发模型** | kotlinx.coroutines + `CompletableFuture` 混合；JVM 线程池 | 原生 goroutine + `context.Context` + `errgroup` |
| **观测** | Spring Observation + Micrometer + `@Tracked` 注解 + MDC 传播 | 直接 OTel API（`otel.Tracer("lynx/agent")`），与 lynx core 一致 |
| **总代码量** | ~244k LOC | ~15.4k Go LOC |
| **embabel-agent-api 自身** | 58.6k LOC | — |

embabel 押的是 "**framework**"——每个定制点都是一个 Spring bean；autoconfigure 模块把 MCP / A2A / Shell / RAG / Observability 全部接好，给你一个**可启动的 Spring Boot 应用**。

lynx 押的是 "**library kit**"——agent runtime 就是一个 `import` 的 Go 包；认证、传输、持久化、部署都交给调用方。

**这种取舍直接反映在源代码上**：embabel 光是 `embabel-agent-api/` 就有 58k LOC，覆盖从注解扫描到动态 agent 创建到 ranking 到 ToolLoop SPI 的全部关注点。lynx `agent/core/` 30 个 Go 文件 ~2.5k LOC；整个 runtime 能在 15k LOC 装下，是因为 "基础设施"（shell / server / autoconfigure / 多 LLM provider starter / Spring Observation）这些层在 lynx 里根本不存在。

**不是优劣，是用例分布的判断**：
- 如果你要"一台机器跑一个 agent 服务，带 REPL 调试 / 多协议接入 / 完整观测"——embabel 帮你节省几周脚手架时间
- 如果你要"在 Go 微服务里嵌一段 agent 逻辑，跑在 Kubernetes pod 里，host 已经有自己的观测和持久化栈"——lynx 不强加 framework 包袱

---

## 2. Agent 声明方式

### 2.1 双轨 vs 单轨

**embabel：注解扫描 + Kotlin DSL 双轨并存**

```kotlin
// 注解风格（反射驱动）
@Agent(name = "ResearchAgent", version = "1.0")
class ResearchAgent {
    @Action
    fun gatherSources(input: Topic): Sources { ... }

    @Action
    @AchievesGoal(description = "blog post produced")
    fun writePost(sources: Sources): BlogPost { ... }

    @Condition
    fun isWellSourced(sources: Sources): Boolean = sources.count() >= 3
}

// Kotlin DSL 风格
val agent = agent("ResearchAgent") {
    action("gatherSources") { input: Topic -> Sources(...) }
    action("writePost", goal = "blog post produced") { s: Sources -> BlogPost(...) }
    condition("isWellSourced") { s: Sources -> s.count() >= 3 }
}
```

**lynx：单一程序化 builder API**

```go
type Topic struct{ Title string }
type Sources struct{ Items []string }
type BlogPost struct{ Body string }

a := agent.New("ResearchAgent").
    Version(semver.MustParse("1.0.0")).
    Actions(
        agent.NewAction("gatherSources",
            func(ctx context.Context, pc *core.ProcessContext, t Topic) (Sources, error) {
                return Sources{Items: gather(t.Title)}, nil
            },
            core.ActionConfig{},
        ),
        agent.NewAction("writePost",
            func(ctx context.Context, pc *core.ProcessContext, s Sources) (BlogPost, error) {
                return BlogPost{Body: compose(s)}, nil
            },
            core.ActionConfig{},
        ),
    ).
    Goals(agent.GoalProducing[BlogPost](core.Goal{Description: "blog post produced"})).
    Conditions(/* 自定义 core.Condition 实现 */).
    Build()
```

### 2.2 类型安全程度对比

| 检查项 | embabel @Annotation | embabel Kotlin DSL | lynx Go builder |
|---|---|---|---|
| Action In/Out 类型 | reflection（运行时）| Kotlin lambda 类型推导 + 反射 | 泛型 `NewAction[In, Out]` + `reflect.TypeFor[T]()` |
| IOBinding 名称 | 从参数名反射读 | 从 lambda 参数名读 | `"name:fqn"` 形式，从 Go type 推导 |
| 启动校验 | `AgentMetadataReader` + `AgentValidationManager` 多层 | DSL 时序检查 | `Platform.Deploy` 时跑 `AgentValidator` extension + goal 可达性 |
| GOAP 可达性 | embabel 启动时算 | embabel 启动时算 | lynx 也算（`platform_deploy.go` 里有 reachability check） |
| 命名冲突 | 启动校验 | 启动校验 | 运行时 deploy 时校验 |

**两边都把启动校验当一等公民**，差别在 embabel **多层 validator**（结构 / 可达性 / DSL 完整性 / 注解一致性都分开）而 lynx **一层 + 可扩展**（`AgentValidator` 接口让 host 加自己的检查）。

### 2.3 关键差异：`OperationContext` 注入

embabel v0.4 收紧了 `@Agent` 构造器校验：不再允许 `OperationContext` 注入到构造器（fail-fast）。

```kotlin
// 旧版可以
@Agent
class FooAgent(val ctx: OperationContext) { ... }   // ❌ 在 v0.4 fail-fast

// 必须改成 action 参数注入
@Action
fun doStuff(ctx: OperationContext, input: Foo): Bar { ... }
```

lynx **没这个问题**——builder 模式没有"构造器注入"概念，`ProcessContext` 永远是 action 第一参数。Go 静态类型 + 显式参数让这种约束在编译期自然成立。

### 2.4 第三种方式：codegen?

设计 notes 提过 `//go:generate lynx-agent-gen` 从 struct tag 生成注册代码——**没有落地**，也不打算落地。当前 lynx 就一条声明路径：`agent.New(name).Actions(...).Goals(...).Build()`。这是**故意的设计选择**——多种声明方式会让"agent 该长什么样"的心智模型不稳定。

---

## 3. 核心抽象建模

### 3.1 Action 接口形态

**embabel：5 个契约组合**

```kotlin
interface Action : DataFlowStep, ConditionAction, ActionRunner, DataDictionary, ToolGroupConsumer {
    val canRerun: Boolean
    val readOnly: Boolean
    val qos: ActionQos
    val domainTypes: Collection<DomainType>
    // from DataFlowStep: name, inputs, outputs, referencedInputProperties
    // from ConditionAction: preconditions, effects
    // from ActionRunner: run(OperationContext) → ActionResult
    // from DataDictionary: schema()
    // from ToolGroupConsumer: toolGroups
}
```

**lynx：薄接口 + 元数据分离**

```go
type Action interface {
    Metadata() ActionMetadata
    Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}

type ActionMetadata struct {
    Name           string
    Description    string
    Inputs, Outputs []IOBinding
    Preconditions, Effects EffectSpec
    CanRerun       bool
    QoS            ActionQoS
    ToolGroups     []ToolGroupRequirement
    Cost, Value    CostFunc
}
```

**关键差异**：embabel 把 Action 拆成 5 个**契约角色**（数据流步 / 条件动作 / 可运行物 / 数据字典 / 工具组消费者），所有 Action 类必须同时实现这 5 个角色。lynx 把元数据集中到一个 `ActionMetadata` struct，`Action` 接口只剩两个方法——元数据查询和执行。

**实质效果**：
- embabel 类型层级深，"实现 Action 接口" = 必须同时实现 ~15 个方法
- lynx 实现 Action 只需要 2 个方法，元数据自动从 `typedAction[In, Out]` + reflection 推出
- embabel 的多契约形态利于 Spring AOP / 拦截器（按角色切面）；lynx 走 `ActionInterceptor` 显式 onion 链

### 3.2 Process IS Blackboard vs Process HAS Blackboard

```kotlin
// embabel: AgentProcess 继承 Blackboard
interface AgentProcess : Blackboard, Timestamped, Timed, OperationStatus<AgentProcessStatusCode>,
                        LlmInvocationHistory, EmbeddingInvocationHistory {
    val id: String
    val blackboard: Blackboard   // 同时还有一个 blackboard 属性 ⚠️
    val parentId: String?
    val planner: Planner<*, *, *>
    // ...
}
```

```go
// lynx: Process 和 Blackboard 完全分开
type Process interface {
    ID() string
    ParentID() string
    Blackboard() Blackboard      // 通过组合关系暴露
    Goal() *Goal
    Status() ProcessStatus
    Options() *ProcessOptions
    RecordUsage(cost float64, tokens int64)
    Usage() Usage
    TerminateAgent(reason string)
    TerminateAction(reason string)
    TerminateToolCall(reason string)
    AwaitInput(Awaitable) (any, error)
    // ...
}
```

**embabel**："process **就是** blackboard"——可以直接 `process.set(key, value)`、`process.fetch<MyType>()`。代价：`AgentProcess` 接口有 ~30 个方法。
**lynx**："process **有** blackboard"——必须 `process.Blackboard().Set(...)`。代价：多一层访问；好处：职责清晰，process 只管生命周期与子树预算，blackboard 只管数据。

### 3.3 Blackboard 接口 ISP

```kotlin
// embabel: 单一巨接口
interface Blackboard : Bindable, MayHaveLastResult, HasInfoString {
    operator fun get(key: String): Any?
    inline fun <reified T> fetch(key: String): T?
    fun <T> findByType(type: KClass<T>): T?
    val objects: List<Any>
    val entries: Set<Map.Entry<String, Any>>
    val lastResult: Any?
    operator fun set(key: String, value: Any)
    fun bind(key: String, value: Any): Bindable
    fun bindProtected(key: String, value: Any): Bindable
    fun addObject(value: Any): Bindable
    // 共 ~15 个方法
}
```

```go
// lynx: ISP 拆三
type BlackboardReader interface {
    Get(key string) (any, bool)
    GetValue(variable, typeName string) (any, bool)
    HasValue(typeName string) bool
    Objects() []any
    GetCondition(key string) (Determination, bool)
}

type BlackboardWriter interface {
    Set(key string, value any)
    AddObject(value any)
    Bind(value any) string
    BindAll(m map[string]any)
    BindProtected(key string, value any)
    Hide(target any)
    SetCondition(key string, val bool)
}

type Blackboard interface {
    Extension
    BlackboardReader
    BlackboardWriter
    Spawn() Blackboard  // 子 process 拿到隔离的副本
}
```

**ISP 拆分的好处**：可以写"只读 view"（传 `BlackboardReader` 给某个 helper，编译期就保证它不能写）；mock 只需要实现需要的接口子集。embabel 走 Kotlin 接口默认实现，没有 ISP 倾向。

### 3.4 `Determination` 三值逻辑

```go
// lynx: Determination 三值
const (
    DeterminationUnknown Determination = 0  // 还没算 / 不可判定
    DeterminationTrue    Determination = 1
    DeterminationFalse   Determination = 2
)

// 支持运算
d.And(other)
d.Or(other)
d.Not()
```

embabel 用 `ConditionDetermination` 也是三值，但 lynx 把它放在更底层，`WorldState.State() map[string]Determination` 直接走三值——这让 "condition 还没评估" 与 "condition 评估为 false" 在 planner 里能区分（影响 A* 的目标判断）。

### 3.5 Goal 形态

| | embabel | lynx |
|---|---|---|
| 核心字段 | name, preconditions, inputs, outputType, value, tags, examples, export | Name, Description, Pre, Inputs, Value, Tags, Examples, Export |
| Examples | `examples: List<String>` | `Examples []string` —— 同款，用于 LLM-driven goal selection |
| Export | `Export(name, description)` —— MCP 暴露 | `GoalExport{Remote, Description, InputSample}` —— 同款，但有 `InputSample` 让 MCP schema 更准 |
| 形态 | data class | struct |

**lynx 多 `InputSample any`**：让 `AsMCPTool` 把 goal 转 MCP tool 时能给出更准确的 input schema 样例（不需要再从 IOBinding 反射推）。

### 3.6 Process 子树预算 / 成本追踪

| | embabel | lynx |
|---|---|---|
| 接口形态 | `LlmInvocationHistory` + `EmbeddingInvocationHistory`（按 invocation 记历史）| `Process.RecordUsage(cost, tokens)` + `Process.Usage()` 滚动总和 |
| 每次调用细节 | 记录 model / prompt / response / cost / tokens 完整 invocation | 仅 cost 和 tokens 数字 |
| 子树聚合 | 父 process 自动累计子 process 的 invocations | 父 process `Usage()` 自动累计子树 |
| 预算执行 | `EarlyTerminationPolicy.HARD_BUDGET_LIMIT` enum + `ProcessControl.budget` | `BudgetPolicy` extension 隐式 + `ProcessOptions.Budget{CostLimit, ActionLimit, TokenLimit}` |

**embabel 最近改进**（commit `a31cef36`）：每个 LLM/embedding 调用都有 `LlmInvocation`/`EmbeddingInvocation` 记录（model + cost + tokens + prompt + response 全留），可以做 per-call 审计。lynx 当前只追踪滚动总和——**是个 gap**，但实现起来不难（在 `Usage` 上加 `Invocations []InvocationRecord` 字段）。

---

## 4. 规划子系统

### 4.1 内置 planner 对照

| | embabel | lynx |
|---|---|---|
| **GOAP A\*** | `AStarGoapPlanner`（plan/goap/）| `goap.AStarPlanner`（plan/planner/goap/） |
| **优化 GOAP** | `OptimizingGoapPlanner`（multi-goal 评估 + ranking）| 已合并入 default GOAP（多 goal 一起搜，每个 goal 出一个 plan）|
| **Utility 规划器** | `UtilityPlanner`（按 netValue 排序，**没有 0-progress 守卫**，可能死循环）| `reactive.Planner`（utility 风格，**显式 0-progress 守卫**——拒绝不推进 goal 的 action） |
| **HTN** | ❌ 没有 | ✅ `htn.Planner` + `htn.Library`（hierarchical task network 完整实现）|
| **Condition 规划基类** | `AbstractConditionPlanner` + `ConditionPlanningSystem` | 通过 `plan.PlanningSystem` + `core.WorldState` 隐式表达 |
| **后向相关性剪枝** | ❌ 没有 | ✅ `plan/planner/goap/relevance.go`（STRIPS regression）|
| **搜索后剪枝** | `backwardOptimize` 在 OptimizingGoapPlanner 内做 | `astar.go` 内 `backwardOptimize` 同款 |

### 4.2 Planner 接口签名

```kotlin
// embabel: 3 个泛型参数
interface Planner<S : PlanningSystem, W : WorldState, P : Plan> {
    fun bestValuePlanToAnyGoal(system: S, excludedActionNames: Set<String>): P?
    fun bestValuePlansToGoal(system: S, goal: Goal): List<P>
}
```

```go
// lynx: 无泛型，简洁接口
type Planner interface {
    Extension
    PlanToGoal(ctx context.Context, system *PlanningSystem, goal *core.Goal, opts PlanOptions) (*Plan, error)
    PlansToGoals(ctx context.Context, system *PlanningSystem, opts PlanOptions) ([]*Plan, error)
}

type PlanOptions struct {
    ExcludedActions []string  // 黑名单 replan
    MaxIterations   int
}
```

**差异**：embabel 用 3 个泛型参数把 system / world state / plan 的具体类型锁到 planner 实现；lynx 用接口（`*PlanningSystem` 类型固定，`*Plan` 类型固定）+ `ExcludedActions` slice 明确黑名单。embabel 的泛型用法很 Kotlin（compile-time variance），但 type-erasure 让 runtime 实际只看见 `Planner<*, *, *>`。

### 4.3 HTN（lynx 独有）

```go
// lynx/agent/plan/planner/htn/
package htn

type Planner struct { ... }
type Library struct {
    Methods []Method
    // method: name + preconditions + decomposition (subtasks)
}

// HTN 把 abstract task 递归分解成 primitive action 序列
// 比 GOAP 更适合"已知任务结构、不需要纯搜索"的场景
```

**embabel 完全没有 HTN**。lynx 加 HTN 不是为了"反超"，而是因为 HTN 在某些用例上明显比 GOAP 高效（比如"研究→草稿→编辑"这种结构清晰的工作流，HTN 直接展开比 GOAP 搜索更自然）。

### 4.4 Reactive planner 的 0-progress 守卫

```kotlin
// embabel UtilityPlanner（伪代码）
fun nextAction(world: WorldState, system: System): Action? {
    return system.actions
        .filter { it.preconditions.satisfiedBy(world) }
        .maxByOrNull { it.netValue(world) }
    // ⚠️ 没有检查 "这个 action 是否真的推进 goal" —— 可能死循环
}
```

```go
// lynx reactive.Planner（伪代码）
func (p *Planner) NextAction(world *WorldState, goal *Goal) Action {
    candidates := applicableActions(world)
    candidates = filter(candidates, func(a Action) bool {
        return progressesGoal(a, world, goal)  // ✅ 必须推进至少一个未满足 precondition
    })
    return bestByNetValue(candidates)
}
```

**这个差异在 README 里 embabel 自己也承认**——`UtilityPlanner` 注释里说 "no progress check, may loop infinitely"。lynx 从设计上规避这个陷阱。

### 4.5 Planner 选择机制

```kotlin
// embabel: ProcessOptions.plannerType + PlannerFactory SPI
processOptions {
    plannerType = PlannerType.A_STAR
    // 或 PlannerType.UTILITY / PlannerType.SUPERVISOR
}
// PlannerFactory Spring bean 按 type 造 planner
```

```go
// lynx: agent.AgentConfig.PlannerName + Extension Name() 匹配
a := agent.New("...").Build()
a.PlannerName = "goap"  // 或 "htn" / "reactive"
// runtime 在 extension 注册表里按 Planner.Name() 找
```

两边形态等价——一个用 enum + factory，一个用字符串 + extension registry。**lynx 多了在 agent 级别配置 planner**（同一 platform 上的不同 agent 可以用不同 planner）；embabel 把 planner 选择放在 `ProcessOptions`，每次 run 时指定。

---

## 5. Extension / SPI 模型

### 5.1 lynx Extension 模型（统一）

所有 SPI 都嵌入 `core.Extension` 接口（marker，只有 `Name() string`）：

```go
type Extension interface { Name() string }

// 10 个 extension 类型，全部统一注册到 Platform：
1. plan.Planner             // 规划器（goap / htn / reactive 默认注册）
2. core.Blackboard          // 黑板存储（prototype 模式，Spawn 派生 per-process）
3. core.IDGenerator         // process ID 生成
4. runtime.EventListener    // 事件多播消费者
5. core.ActionInterceptor   // onion-style 包 action.Execute
6. core.ToolDecorator       // 包每个 resolved tool
7. core.ToolGroupResolver   // 按 role 解析 ToolGroup（MCP / 自定义）
8. core.AgentValidator      // deploy 时 gate（最后一道）
9. core.GoalApprover        // 否决 goal（AND 语义）
10. core.EarlyTerminationPolicy  // OR-组合终止信号
```

注册方式：`runtime.PlatformConfig{Extensions: []core.Extension{...}}`，或 `ProcessOptions.Extensions` 单 process 临时加。

### 5.2 embabel SPI 模型（多层）

```
embabel-agent-api/spi/
├── PlannerFactory               # 从 ProcessOptions 造 planner
├── ToolGroupResolver            # 解析 tool group 要求
├── BlackboardProvider           # 每个 process 造 Blackboard
├── ContextRepository            # ✅ lynx 没有 —— 持久化 context
├── AgentProcessRepository       # ✅ lynx 没有 —— 持久化 process state
├── AgentProcessIdGenerator      # process ID 生成
├── ToolDecorator                # 包 tool 调用
├── OperationScheduler           # Pronto / Delayed / Scheduled 调度
├── LlmService                   # ✅ lynx 不抽（用 chat.Client 直接）—— LLM 调用抽象
└── AutoLlmSelectionCriteriaResolver  # ✅ lynx 没有 —— 按 criteria 选 LLM

embabel-agent-api/spi/loop/          # ToolLoop 单独一组 SPI（14 个文件）
├── ToolLoop                     # 工具调用循环本身是 SPI
├── ToolLoopFactory              # 工厂
├── ToolInjectionStrategy        # tool 怎么呈现给 LLM
│   ├── ChainedToolInjectionStrategy
│   ├── UnfoldingToolInjectionStrategy
│   └── （Full / Single 变体）
├── EmptyResponsePolicy          # LLM 返回空怎么处理
├── ToolNotFoundPolicy           # LLM 调不存在的 tool 怎么处理
├── LlmMessageSender             # 非流式 LLM 通信
└── LlmMessageStreamer           # 流式 LLM 通信（Flux<String>）

embabel-agent-api/spi/validation/    # 校验子系统（9 个文件）
└── AgentValidationManager + 各类 validator
```

### 5.3 SPI 数量与覆盖

| 维度 | embabel | lynx |
|---|---|---|
| 核心 SPI 数 | 10（spi/） + 14（spi/loop/） + 9（spi/validation/）= **33** | **10** Extension types |
| 注册方式 | Spring `@Bean` + autowiring | `PlatformConfig.Extensions` slice |
| 校验 | `AgentValidationManager` 多 validator 编排 | `AgentValidator` 单一接口 + 用户加 |
| 启动失败信号 | Spring `@PostConstruct` 抛 | `Platform.Deploy` 返回 error |

### 5.4 lynx 缺的 embabel SPI

按真实需求排序：

| embabel SPI | lynx 是否有等价 | 备注 |
|---|---|---|
| `BlackboardProvider` | ✅ `core.Blackboard` extension 自带 `Spawn()` | 形态不同但等价 |
| `PlannerFactory` | ✅ `plan.Planner` extension 直接注册 | SPI flattening 去掉了 factory 层 |
| `ToolGroupResolver` | ✅ `core.ToolGroupResolver` | 同款；lynx 走 MCP resolver 一等公民 |
| `ToolDecorator` | ✅ `core.ToolDecorator` | 同款 |
| `AgentProcessIdGenerator` | ✅ `core.IDGenerator` | 同款 |
| `ContextRepository` | ❌ | 持久化 SPI 故意不抽（in-memory 设计）|
| `AgentProcessRepository` | ❌ | 同上 |
| `OperationScheduler` | ❌ | `ProcessControl.toolDelay/operationDelay` 字段都没消费 |
| `LlmService` | ❌（用 `chat.Client` 直接）| lynx 不抽这层 |
| `AutoLlmSelectionCriteriaResolver` | ❌ | 多 LLM 路由按 criteria 选——lynx 让 host 自己挑 |
| `ToolLoop` 整个子系统 | 部分（`chat.ToolMiddleware` + `toolpolicy.*` decorator）| **真 gap**——见 §6.4 |
| `LlmMessageStreamer` | ✅ `chat.StreamHandler` + `iter.Seq2` 天然提供 | 不需要 SPI 抽象 |

---

## 6. Tool / Tool-loop 系统

### 6.1 Tool 抽象

```kotlin
// embabel Tool 接口
interface Tool : ToolInfo {
    fun call(input: String): Result
    fun call(input: String, ctx: ToolCallContext): Result
    val definition: Tool.Definition  // name, description, inputSchema, metadata
    val metadata: Tool.Metadata
}

sealed class Result {
    data class Text(val content: String) : Result()
    data class WithArtifact(val content: String, val mime: String, val artifact: Any) : Result()
    data class Error(val message: String, val code: String?) : Result()
}
```

```go
// lynx tool 走 lynx/core/model/chat:
type Tool interface {
    Definition() ToolDefinition
    Metadata() ToolMetadata
    Call(ctx context.Context, arguments string) (string, error)
}
```

**形态差异**：embabel 的 `Tool.Result` 是 sealed class（Text / WithArtifact / Error 三种），lynx tool 直接返回 `(string, error)`——简洁，但当 tool 需要返回二进制 artifact（图像、PDF）时 lynx 需要走 base64 或 metadata 通道，embabel 一等公民。

**真 gap**：lynx 如果要支持 artifact-bearing tool（生成图像 / 表格的工具），需要扩 `Tool.Call` 返回类型——但代价是 breaking change，目前权衡是"99% 场景用 string 够，artifact 走 metadata 通道"。

### 6.2 Tool 实现矩阵

| | embabel | lynx |
|---|---|---|
| 反射 method → tool | `MethodTool` / `MethodToolFactory` + `@Tool` 注解 | `pkg/json` 从 Go struct 生成 JSON schema + 泛型 typed action |
| 注解化声明 | `@Tool` / `@ToolParam` | 无（Go 无 runtime annotation）|
| 嵌套 sub-agent 作工具 | `Subagent` tool | `runtime.AsChatTool(agent)` |
| 暴露成 MCP tool | `embabel-agent-mcp/SyncMcpToolCallbackProvider` 自动 | `runtime.AsMCPTool(agent)` + `mcp.RegisterTools(...)` |
| Progress 反馈给 LLM | `ProgressTool` 内置 | 无（用户自己写一个 tool）|
| LLM-to-human 通信 | `CommunicateTool` 内置 | 无（用户走 HITL `Awaitable`）|

### 6.3 ToolLoop SPI（embabel 一等公民 vs lynx 隐式）

**embabel 的 ToolLoop SPI** ——这是 embabel 把 "LLM 自驱动多轮 tool 调用" 显式抽出来的子系统：

```kotlin
interface ToolLoop {
    suspend fun <O> execute(
        messages: List<Message>,
        tools: List<Tool>,
        outputParser: (String) -> O,
    ): ToolLoopResult<O>
}

// 配套：
ToolLoopFactory                  // 工厂
ToolInjectionStrategy            // tool 怎么呈现给 LLM
├── FullToolInjectionStrategy        // 全部 tool 一次塞
├── ChainedToolInjectionStrategy     // 链式分组塞
├── UnfoldingToolInjectionStrategy   // 渐进展开（按需暴露）
└── SingleToolInjectionStrategy      // 一次只暴露一个
LoopMemo                         // 循环内记忆
OneShotPerLoopTool               // 单次约束
EmptyResponsePolicy              // 空响应处理
ToolNotFoundPolicy               // tool 未找到处理
ToolCallInspector                // 拦截观察 tool 调用
ToolLoopCallback                 // 高层循环钩子
ReplanningTools                  // 触发动态 replan
```

**lynx 的等价：隐式 + 装饰器**

```go
// lynx 把 tool loop 放在 chat 层（lynx/core/model/chat/tool_middleware.go）
// agent 层只用 chat.ToolMiddleware，不重新抽 ToolLoop SPI

// agent 层的装饰器（toolpolicy/）
toolpolicy.OnceOnly(tool)            // 拒绝第 2 次调用（类似 OneShotPerLoopTool）
toolpolicy.Unlocked(tool, condFn)    // 条件性 gate（类似 ToolInjectionStrategy 部分能力）

// LLM-driven multi-turn 走 ProcessContext.ChatWithActionTools(ctx)
// 取消走 TerminateToolCall(reason)
```

**关键差异**：embabel 把 tool loop 拆成**显式 SPI 树**（让用户能换 injection strategy / memoize / 拦截），lynx 把 tool loop 当成 chat client 的**默认行为**（`chat.ToolMiddleware` 在每个 `Call`/`Stream` 上自动跑）。

**lynx 这条线 gap**：
- `ToolInjectionStrategy` 等价物——当 tool 数量大时，全部塞给 LLM 会爆 context，embabel 的 Chaining/Unfolding 在工程上有用
- `LoopMemo` 等价物——同一 loop 内重复调相同 tool 时短路 cache
- `ToolNotFoundPolicy` / `EmptyResponsePolicy` 等价物——可配置的错误降级

**ROI 评估**：上述 gap 关键性中等——大部分用户的 tool 数量在 ~10 个内，不需要 injection strategy。但 `OneShotPerLoopTool` 等价物 lynx 已经有了（`toolpolicy.OnceOnly`），剩下的可以按需补。

### 6.4 ToolGroup 概念

| | embabel | lynx |
|---|---|---|
| `ToolGroupRequirement` | Action 上声明需要哪些 toolGroup（按 role）| `core.ToolGroupRequirement{Role, Permissions}` 同款 |
| `ToolGroupResolver` | 按 role 找 tools | 同款；MCP resolver 一等公民 |
| `ToolGroupPermission` | enum: HOST_ACCESS, INTERNET_ACCESS | enum: PermissionInternet, PermissionFile, PermissionExec |
| 懒加载 | embabel 0.4 引入 lazy `ToolGroup.tools` | lynx `MCPToolGroupResolver` 自带 lazy（`provider.Tools()` 首次访问时 fetch） |

两边概念几乎对齐——embabel 0.4 的 ToolGroup 设计被 lynx 完整借鉴过来。

---

## 7. HITL / Autonomy / Workflow

### 7.1 HITL：单一泛型 vs 多个子类

```kotlin
// embabel: 多个 HITL 子类
sealed class Awaitable {
    abstract val responseProvider: ResponseProvider<*>
}
data class ConfirmationRequest(val prompt: String, val default: Boolean?) : Awaitable()
data class FormBindingRequest(val type: KType, val prompt: String) : Awaitable()
data class TypeRequest<T>(val type: Class<T>) : Awaitable()
// + AwaitableTypedTool / AwaitingTools 适配器
```

```go
// lynx: 一个泛型接口，所有 HITL 形态走它
package hitl

type Request[P, R any] interface {
    core.Awaitable
    Prompt() P
    OnResponse(r R) core.ResponseImpact
}

type TypedRequest[P, R any] struct {
    IDStr   string
    Payload P
    Handler func(R) core.ResponseImpact
}

// 特化构造函数：
NewConfirmation[P](payload P, handler func(approved bool)) *TypedRequest[P, bool]
// 用户写自己的 form / approval / 任意 typed HITL 都通过 TypedRequest[P, R]
```

**lynx 反向超**：用一个泛型 `TypedRequest[P, R]` 覆盖所有 HITL 形态（confirmation = `TypedRequest[P, bool]`，form = `TypedRequest[FormSchema, FormData]`），embabel 需要为每种 HITL 形态写一个 sealed class 子类。代价：lynx 用户写自定义 HITL 需要熟悉 Go 泛型。

### 7.2 Autonomy：LLM-driven 决策

| | embabel | lynx |
|---|---|---|
| LLM 选 goal | `Autonomy` class + `GoalChoiceApprover` + LLM ranking | `autonomy.LLMRanker`（Extension + `GoalRanker` 接口）|
| LLM 选 agent | `SupervisorAgent`（一等公民，多 agent 之间 LLM 选）| 无独立 SupervisorAgent；用户用 `Platform.CreateChildProcess` + workflow 拼 |
| LLM 评估 plan | embabel 没显式抽 plan ranking | `autonomy.LLMPlanRanker`（Extension + `PlanRanker` 接口）|
| 置信度阈值 | `AutonomyProperties` 配 | 直接在 Ranker 实现里写 |

**这里有非对称**：
- **embabel 多 SupervisorAgent**（LLM 驱动选哪个 agent 处理请求）——lynx 真 gap
- **lynx 多 LLMPlanRanker**（LLM 评估候选 plan 哪个更好）——embabel 没有显式对应

### 7.3 Workflow primitives：lynx 独有的一层

embabel 把"高阶 agent 形态"统一走 SupervisorAgent + child process 调度——**没有专门的 workflow primitive 层**。

lynx 在 `agent/workflow/` 出货 7 个 workflow primitive：

```go
// agent/workflow/
1. Sequence[In, Out]            // 串行链：In → action₁ → action₂ → Out
2. Parallel[In, Out]            // 并行 fan-out N actions
3. Loop[State]                  // 状态机循环（带终止条件）
4. RepeatUntil[In, Out]         // 重复直到 Accept(result) 返回 true
5. RepeatUntilAcceptable[In, Out]  // LLM 评分版本（Feedback{Score, Text}）
6. Consensus[In, Out]           // N 个 actor 投票
7. ScatterGather[Element, Join] // fan-out 生成器 → ResultList[Element] → join action

// 配套类型
Feedback        // Score (0-1) + Text + Acceptable(threshold)
History[T]      // Attempts []T + Last() + Count()
ResultList[E]   // Items []E
```

每个 primitive 都构建出 `*core.Agent`，不引入新的运行时概念——这是**用 GOAP runtime 表达常见高阶模式的语法糖层**。

**embabel 没有这层**，但 embabel 用注解模型表达这些场景：
- Sequence = 多个 action 依次声明 + GOAP 自然串
- Parallel = `ConcurrentAgentProcess` + 并发 action
- RepeatUntilAcceptable = 用户自己写 condition + replan loop

**取舍**：lynx 的 workflow primitive 是显式的"如果你只想要串行/并行/重试"的快速入口；embabel 是"GOAP 表达力够强了，写多个 action 就行"。**两条路线都成立**——lynx 7 个 primitive 加起来 ~970 LOC，是性价比高的封装。

---

## 8. 生态广度

### 8.1 模块数量

| | embabel | lynx/agent |
|---|---|---|
| 核心 module | 16（api / common / shell / a2a / mcp / mcp-security / code / observability / rag×5 / domain / onnx / openai / skills / test-support / dependencies）| 1（agent/）|
| Autoconfigure module | 7 | 0 |
| Starter module | 20+ | 0 |
| 总 module 数 | ~43 | 1 |

### 8.2 LLM provider 接入

| | embabel starters | lynx |
|---|---|---|
| OpenAI | ✅ | ✅（通过 `lynx/models/openai`） |
| Anthropic | ✅ | ✅（通过 `lynx/models/anthropic`） |
| Google GenAI / Gemini | ✅ | ✅（通过 `lynx/models/google`） |
| Ollama | ✅ | ❌ |
| DeepSeek | ✅ | ❌（可 OpenAI-compat 走 openai adapter）|
| Mistral AI | ✅ | ❌（OpenAI-compat）|
| Bedrock | ✅ | ❌ |
| LM Studio | ✅ | ❌（OpenAI-compat）|
| Docker Models | ✅ | ❌（OpenAI-compat）|
| ONNX 本地 | ✅（embabel-agent-onnx） | ❌ |

**lynx core 当前 3 个 provider**——agent 复用 `lynx/core/model/chat` 接口，不需要 agent-specific 的 LLM 集成层。

### 8.3 RAG / 向量检索

| | embabel | lynx |
|---|---|---|
| RAG 子系统 LOC | 30,103（embabel-agent-rag 5 个子 module）| ~（复用 lynx/rag，agent 层无独立 RAG） |
| 向量检索后端 | Lucene / Neo4j / Tika 文档解析 | 复用 `lynx/vectorstores/{chroma,milvus,pinecone,qdrant,weaviate,inmemory}` |
| Document reader | Tika 集成（PDF/Office 全套） | `lynx/core/document/{reader_text,reader_json}` 仅 2 个 |
| Tools | `SearchDocuments`、`RetrieveDocuments` 内置 agent 工具 | 用户走 `rag.Pipeline` 包成 chat middleware |

**embabel RAG 是一等公民的子系统**（30k LOC），lynx 走 lynx/core 提供 RAG，agent 层直接用——这是哲学一致（agent 是 library，不重复造 RAG）。

### 8.4 持久化 SPI

| | embabel | lynx |
|---|---|---|
| `ContextRepository` | InMemoryContextRepository 内置，留口给 Spring Data | 无 |
| `AgentProcessRepository` | InMemoryAgentProcessRepository 内置，留口给 Spring Data | 无 |
| Blackboard 持久化 | `BlackboardProvider` SPI | `core.Blackboard` extension prototype 模式；用户实现自己的 |
| Hierarchy-aware eviction | embabel 有 `HierarchyAwareEvictionPolicy` | 无 |

**真 gap**：lynx 故意全内存，但生产场景必然要持久化——可放 `agent/persistence/` 子包，类比 lynx/core 的 memory backend 路线。

### 8.5 前端 / 协议

| | embabel | lynx |
|---|---|---|
| Shell REPL | ✅ `embabel-agent-shell`（1376 LOC，markdown / personality / commands） | ❌ |
| A2A protocol server | ✅ `embabel-agent-a2a` | ❌ |
| MCP server | ✅ `embabel-agent-mcp`（6171 LOC，sync + async + security 三件套）| ✅ 复用 `lynx/mcp.RegisterTools`（agent 用 `AsMCPTool` 包） |
| REST endpoints | ✅（feature toggle 控制） | ❌ |
| Skills（YAML + Docker）| ✅ `embabel-agent-skills`（2967 LOC）| ❌ |

**lynx 设计上把这些放 `agents/` 外挂 module**（README 提过 `agents/a2a` / `agents/shell` / `agents/skills`），但都未落地。

### 8.6 观测

| | embabel | lynx |
|---|---|---|
| 抽象层 | Spring Observation + Micrometer | 直接 OTel API |
| 自动埋点 | `@Tracked` 注解 + AOP aspect | 手动 `otel.Tracer("lynx/agent").Start(...)` |
| Cost tracking 粒度 | per-call `LlmInvocation`/`EmbeddingInvocation` | 滚动总和 |
| MDC / context | `MdcPropagationEventListener` 自动 | `context.Context` 显式传 |
| 配置 | `ObservabilityProperties`（启用/禁用每项）| Go 构造函数 + `PlatformConfig` |

**lynx 当前 OTel 埋点覆盖率**：core hot path 待补（见 `doc/OBSERVABILITY.md` §4 清单）；exporter（`otel/slog`、`otel/log`）已就位。

---

## 9. 战略 gap + ROI 路线图

按 ROI 排序。每项注明"为什么不抄"或"该不该抄"。

### 9.1 P0 — 该补，工作量小性价比高

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 1 | **per-call `LlmInvocation` 细化** | 低（~80 LOC：`Usage` 加 `Invocations []InvocationRecord{Model, Cost, Tokens, At}` 字段；vendor adapter 在 Call 后填）| LLM 成本审计 / debugging / 计费按调用对账 |
| 2 | **`Goal.Export` → `AsMCPTool` per-goal 暴露** | 低（~100 LOC：`runtime.AgentTool` 已有；改成 per-goal 输出多个 MCP tool）| 让 MCP 客户端能按 goal 选择，而不是按 agent 整体 |
| 3 | **核心 hot path OTel 埋点**（参考 `doc/OBSERVABILITY.md` §4 清单）| 低（每个 callsite ~10 LOC）| 接收侧已就绪，缺埋点 |

### 9.2 P1 — 生产硬刚需

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 4 | **持久化 SPI**（`ContextRepository` + `AgentProcessRepository` 等价）| 中（接口设计 + Redis/Postgres 一个实现作样板）| 跨进程恢复 / 故障 resume；当前完全空白 |
| 5 | **ToolLoop SPI 配套补齐**（`LoopMemo` + `EmptyResponsePolicy` + `ToolNotFoundPolicy` 三件）| 中（在 `chat.ToolMiddleware` 上加 hook 或抽 SPI）| 大型 tool 集场景的稳定性 |
| 6 | **AgentValidationManager 多层化** | 低-中（在 `AgentValidator` 接口基础上加几个内置 validator）| deploy 时一次报全部问题，而不是运行时逐个发现 |
| 7 | **SupervisorAgent 模式 / 多 agent 自动选择** | 中（复用 autonomy.LLMRanker + 抽 supervisor-style API）| 多 agent 场景的入口；当前用户要自己拼 |

### 9.3 P2 — 闭合架构差距

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 8 | **`Subagent` tool 包装等价物** | 低（已经有 `AsChatTool`，扩成可在 action body 内直接调）| sub-agent 作 tool 给 LLM 用 |
| 9 | **`ProgressTool` / `CommunicateTool` 等价物** | 低（~50 LOC 共）| LLM 主动汇报进度 / 给用户发消息 |
| 10 | **`ProcessControl.toolDelay/operationDelay` 消费** | 低（在 dispatch 处加 sleep） | 配额 / rate-limit 友好 |
| 11 | **`ToolInjectionStrategy` 变体**（Chaining / Unfolding）| 中（chat 层改造）| 大 tool 集场景（>20 tools）的 context 优化 |

### 9.4 P3 — 长尾 / 外挂 module

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 12 | **Shell REPL**（放 `agents/shell/`）| 中（cobra + 事件订阅）| 调试 / 演示 |
| 13 | **A2A 协议 server**（放 `agents/a2a/`）| 高（协议不成熟）| 多 agent 跨进程；当前 MCP 已部分替代 |
| 14 | **Skills 模块**（YAML + Docker engine，放 `agents/skills/`）| 高（Docker SDK + YAML parser + 隔离执行）| Skill 分发 / 第三方 agent 扩展 |
| 15 | **更多 LLM provider**（Ollama / Bedrock / DeepSeek 独立 adapter）| 中（每个 vendor ~500 LOC，复用 lynx/core/model）| 按真实需求扩 |

### 9.5 故意不做（"为什么不抄"）

| # | embabel 有但 lynx 不该抄 | 原因 |
|---|---|---|
| A | Spring autoconfigure / starter 矩阵 | lynx 是 library 不是 framework；Go 生态无"DI 容器自动装配"传统 |
| B | `LlmService` SPI 抽象 | lynx 直接用 `chat.Client`，再抽一层 SPI 反而画蛇添足 |
| C | `OperationScheduler`（Pronto / Delayed / Scheduled）| Go 用 `context.WithDeadline` + `time.Timer` 显式即可 |
| D | Spring Observation 抽象层 | OTel 已是 Go 事实标准，再套 Micrometer-like 抽象无意义 |
| E | `@Agent` / `@Action` 注解模型 | Go 无 runtime annotation；codegen 路线性价比低，builder 已足够清晰 |
| F | 多种 HITL 子类（Confirmation / FormBinding / TypeRequest）| 一个泛型 `TypedRequest[P, R]` 覆盖所有形态，反而更整洁 |
| G | sync / async 双轨（每组件两份）| Go goroutine + `iter.Seq2` 单一路径覆盖两种用法 |
| H | Action 多契约继承（5 个接口组合）| ISP 拆 + 元数据 struct 集中，类型层级更扁 |
| I | `AutoLlmSelectionCriteriaResolver` | lynx 让 host 自己挑 LLM，不抽 LLM 路由层 |

---

## 10. 一句话定档

对照 embabel 时**核心思想不妥协，工程化形态全 Go 化**——GOAP + 黑板 + OODA 三件套保留，但不照搬 Spring framework 哲学。这套打法在 HTN planner（embabel 没有）、reactive 0-progress 守卫（embabel UtilityPlanner 可能死循环）、类型安全 `typedAction[In, Out]`（embabel 走反射）、ISP Blackboard（embabel 单巨接口）、workflow primitives（embabel 没有 workflow 层）五条线上已经验证成立。

**下一阶段最高 ROI**：per-call cost tracking + `Goal.Export → MCP` + 持久化 SPI + AgentValidationManager 多层化 + SupervisorAgent。其中前 3 项是生产闭环关键，做完 lynx/agent 就能从"原型 framework"升到"可生产部署"——同时保留 library kit 哲学不动摇。

---

*对比结束。双方 HEAD 截至 2026-05-14（lynx）/ 2026-05-11（embabel）。*
