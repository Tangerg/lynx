# 08. 架构对比：lynx/agent vs embabel-agent

> **对照对象**：embabel-agent v0.4.0-SNAPSHOT (HEAD `da1f1522`)，定位为 Kotlin/JVM、Spring Boot 生态的 GOAP-based agent 框架。
>
> 本文档不是「谁更好」的论战，而是把两边在**同一个架构问题上的不同选择**显式列出来。这是第二次深度刷新，重点更新了：
> - 之前盲点（`AgentValidationManager` / `SupervisorAgent` / `Aggregation` / `@State unrolling` / `DataDictionary` / Spring Observation）的对照
> - lynx 这边几次清理之后的最新状态（HITL 修复、`HashKey` 竞态修复、`ServiceProvider` 改成 key→any 注册表、JSON-event、subtree budget、A\* 后处理）

---

## 0. 一句话定位

| 维度 | embabel-agent | lynx/agent |
|---|---|---|
| 语言 / 平台 | Kotlin · JVM · Spring Boot | Go 1.25+ · 无 DI 容器 |
| 用户编程模型 | 注解扫描（`@Agent` / `@Action` / `@AchievesGoal`）+ Kotlin DSL（双轨） | 唯一入口：泛型 + 流式 DSL（已删反射注册路径） |
| 模块拆分 | 22 个 Maven 模块（按外部生态切） | 8 个 Go 包（按内部分层切） |
| 启动校验 | `AgentValidationManager` 多层 — 结构 + GOAP 可达性 | 无（运行时遇错） |
| 多 agent 编排 | `SupervisorAgent` — LLM 驱动的多 agent 选择 | 不内置（用户自行用 `Platform.CreateChildProcess` 组合） |
| 总代码量级 | 极大（核心 + 多 LLM provider + RAG + skills + a2a + shell） | 5,667 LOC（仅核心，零生态绑定） |
| 默认规划器 | A\* GOAP（可切 Utility / Supervisor） | A\* GOAP（接口可换） |

---

## 1. 模块结构

embabel **22 个 Maven 模块**：按外部生态切（每个 LLM provider / RAG backend / 集成协议一个 module）。

lynx **8 个 Go 包**：按内部分层切（DAG，无环）：

```
core/        ← 数据类型 + 接口（Agent, Action, Goal, Blackboard, WorldState, ...）
plan/        ← Plan, Planner 接口, PlanningSystem, ConditionWorldState
planner/goap ← A* 算法 + 后处理（独立子包，便于换 planner）
runtime/     ← Platform, AgentProcess, executeAction（执行引擎）
event/       ← 21 种生命周期事件 + Multicast + 原生 JSON 序列化
hitl/        ← Request[P,R] / ConfirmationRequest / FormBindingRequest
dsl/         ← Builder（用户唯一入口）
agent/       ← 顶层 re-export 包（type alias + 简短包装）
```

集成生态（MCP / chat / RAG / vectorstore / observability）**全部** 在外面 — `lynx/core/...`、`lynx/mcp/...`、`lynx/otelbridge/...` 各自独立 module。

**取舍：** embabel 是 "batteries-included"（带 OpenAI/Anthropic/Ollama starters / Lucene+Neo4j RAG / Docker skills），新人 onboarding 要先理解 22 个 module 的依赖图。lynx 是「最小内核」，core 永远不直接 import LLM SDK，演进自由度更大。

---

## 2. 用户编程模型

**embabel：** 注解 + Kotlin DSL 双轨。

```kotlin
@Agent(description = "Routes intents")
class IntentAgent(private val llm: ChatClient) {
    @Action
    fun classify(input: UserInput): Intent { ... }

    @Action
    @AchievesGoal(description = "Done", value = 1.0)
    fun resolve(intent: Intent): Resolution { ... }
}
```

`AgentMetadataReader.kt`（591 行）在 Spring 启动时反射扫所有 `@Agent` bean。同时也有 Kotlin DSL `AgentBuilder`。

**lynx：** 流式 DSL 是唯一入口。

```go
agent.New("IntentAgent").
    Description("Routes intents").
    Actions(agent.NewAction("classify",
        func(ctx context.Context, pc *core.ProcessContext, in UserInput) (Intent, error) {...},
        core.ActionConfig{},
    )).
    Goals(agent.GoalProducing[Resolution](core.Goal{
        Description: "Done", ValueStatic: 1.0,
    })).
    Build()
```

我们曾经实现过 `agent/reflect/Register(instance any)`（450 LOC）做反射注册 — **但删掉了**（commit `53ea5a3`）。Go 社区共识「显式优于隐式」：反射注册让错误推迟到运行时、IDE 重构失效、命名约定成为隐藏契约。

---

## 3. Action 模型

| 维度 | embabel.Action | lynx.Action |
|---|---|---|
| 接口形态 | 多接口 mixin：`DataFlowStep + ConditionAction + ActionRunner + DataDictionary + ToolGroupConsumer` | 单接口：`Metadata() + Execute(ctx, pc) ActionStatus` |
| 元信息 | 散落在 5 个 mixin | 统一到 `ActionMetadata` 结构体 |
| Pre/Effects | `EffectSpec`（map[string]Determination） | 同 |
| QoS | 5 字段（maxAttempts/backoffMillis/multiplier/maxInterval/idempotent） | 3 字段（MaxAttempts/BaseDelay/MaxDelay）— 退避数学交给 `pkg/retry` |
| 函数签名 | `(ctx, ...任意参数顺序)` 反射解析 | 固定 `(ctx, pc, In) (Out, error)` 泛型 |
| 多输入 | **「Megazord」自动聚合**（见 §6） | 显式 — 用户写 `core.Get[T](pc.Blackboard, ...)` |

**embabel 的 PromptedTransformer 不存在** — 上次的对比文档说「DSL 有 transformation + promptedTransformer 双轨」是错的。实际 embabel 不区分「LLM action」和「非 LLM action」，所有 action 都是普通 `@Action` 方法，里面调不调 LLM 是实现细节。本次校正。

---

## 4. Planner

### A\* GOAP

| 步骤 | embabel | lynx |
|---|---|---|
| 早退 | 目标已满足 → 空 plan | 同 |
| 可达性预检 | `isGoalReachable()` 反向扫候选 actions | 同（`goalReachable`） |
| 启发式 | 未满足条件计数（admissible） | 同 |
| 状态展开 | 逐 action 应用 effects | 同 |
| 闭集重开 | 找到更优路径时重新打开 closed 节点 | 同 |
| 后处理 | backward + forward optimization | 同（`backwardOptimize` + `forwardOptimize`） |

planner trace span 现在记 `plan_length_raw`（A\* 出来的）和 `plan_length`（后处理后）两个字段，方便观察后处理裁掉了多少冗余 action。

### 多种 planner

embabel：GOAP / Utility / **Supervisor** 三种，via `ProcessOptions.plannerType`。Supervisor 是 LLM 驱动的多 agent 编排（详见 §13）。

lynx：只有 GOAP；`runtime.PlatformConfig.PlannerFactory` 是 pluggable 接口，用户可以塞自己的 planner 但仓库不内置 utility/supervisor。

---

## 5. 运行时执行

| 阶段 | embabel | lynx |
|---|---|---|
| 入口 | `SimpleAgentProcess.tick()` 一次一动作；`run()` 循环到终止 | `AgentProcess.run(ctx)` 内化循环 |
| OBSERVE | `BlackboardWorldStateDeterminer` | 同（接口私有化为 `worldStateDeterminer`） |
| ORIENT | planner.bestValuePlan(start, system) | 同 |
| DECIDE | sequential / concurrent | 同（`runtime/concurrent.go`） |
| ACT | `executeAction` + retry + panic recovery | 同 |
| 终止信号 | `TerminateActionException` / `TerminateAgentException` | `core.TerminationSignal` 经 channel |
| Replan 信号 | `ReplanRequestedException` | `core.ReplanRequest`（实现 error 接口） |
| Panic 隔离 | runtime 层 try-catch | `core.ProcessContext.ExecuteSafely(ctx, action)` 把 panic-guard 内化为方法 |

**两边对比：** embabel 用**异常控制流**表达 terminate / replan；我们用 **error 类型 + signal channel**。Go 不依赖异常做控制流，是语言惯例差异。

### 重试

embabel 手写 backoff + jitter。

lynx 委托 `lynx/pkg/retry` — 自动获得指数退避 + jitter + ctx 取消传播 + 溢出保护。`ActionQos` 字段从 5 个降到 3 个。

---

## 6. Blackboard

### 接口规模

两边都是 ~17 方法的接口：`Set/Get/GetValue/HasValue/Bind/BindAll/BindProtected/Hide/SetCondition/GetCondition/Spawn/Clear/InfoString/...`

### Megazord aggregation（embabel 独有）

embabel 的 `InMemoryBlackboard.getValue()` 在找不到直接绑定时，会启动一个**类型聚合**流程：

1. 在 `DataDictionary` 里搜实现 `Aggregation` 接口的类
2. 反射看构造函数参数（如 `Research(Topic, Outline)`）
3. 对每个参数从黑板找 last-of-type
4. 若全找到，调构造函数生成 `Research` 实例并缓存

整个过程在 `Blackboard.kt` line 339-393。这让多输入 action 写起来很爽：

```kotlin
@Action
fun write(topic: Topic, outline: Outline, research: Research): Article { ... }
// research 自动 Megazord 出来
```

**lynx 故意不做这件事。** 多输入 action 在 lynx 里走显式：

```go
agent.NewAction("write",
    func(ctx, pc, in Outline) (Article, error) {
        topic, _    := core.Get[Topic](pc.Blackboard, core.DefaultBindingName)
        research, _ := core.Get[Research](pc.Blackboard, core.DefaultBindingName)
        ...
    },
    core.ActionConfig{Pre: []string{"it:" + core.TypeFullNameOf[Research]()}},
)
```

**取舍：** embabel 的方便性带来的代价是参数顺序敏感、构造函数注入限制、调试时「Research 怎么突然冒出来的」。我们坚持显式 `Get[T]` — 多打几行字，所有数据流都可 grep。

### `expressionEvaluationModel()`

embabel 的 `Blackboard.expressionEvaluationModel(): Map<String, Any>` 把命名绑定快照出来给 SpEL 表达式引擎用（precondition / 提示词渲染）。

lynx 不用 SpEL —— 模板渲染交给用户的 prompt 库（`lynx/core/model/chat.PromptTemplate`），框架不参与字符串模板。

---

## 7. 工具 / ToolGroup

| 类型 | embabel | lynx |
|---|---|---|
| `ToolGroup` 接口 | `metadata + tools: List<Tool>` | 同 |
| `LazyToolGroup` | MCP 触发首次访问时拉 tools | 同 |
| `ToolGroupRequirement` | `role + requiredToolNames + terminationScope?` | `Role + TerminationScope` |
| `TerminationScope` | `AGENT / ACTION / TOOL_CALL` | 同（`Agent / Action / ToolCall`，最近添加） |
| 框架基础设施位置 | `ProcessContext.toolGroupResolver` | `Platform.tools`（独立字段，不进 Services 注册表） |

**ToolCall 取消语义**：我们刚加了 enum 值但**还没接 cancellation 管道**。embabel 那边在 `DefaultToolLoop` 里检查 `terminationRequest`，调 `future.cancel(true)` 中止 in-flight 工具调用。我们的版本目前是 placeholder — 待补。

---

## 8. 事件 / 可观测性

### 事件类型

| | embabel | lynx |
|---|---|---|
| 类型数 | ~15 个，sealed interface + Jackson `@JsonTypeInfo(SIMPLE_NAME)` | 21 个，原生 `MarshalJSON` |
| 派发 | `AgenticEventListener` 三个 callback：`onProcessEvent / onPlatformEvent / onLlmEvent` | 单 `OnEvent(Event)`，type switch 分流 |
| 多播 | `MulticastListener` 链式 | `Multicast` 切片 + panic recovery |
| LLM 事件 | `LlmInvocation` 收集进 `LlmInvocationHistory`，cost **按需算**（不发事件） | 有 `LLMRequestEvent` / `LLMResponseEvent`，但**框架不内置成本计算** — 用户在监听器里把 cost 上报回 `Process.RecordUsage` |
| JSON 序列化 | Jackson `@JsonTypeInfo(SIMPLE_NAME)` 自动处理多态 | 每个 event 类型实现 `json.Marshaler`；接口字段（Action / WorldState / Awaitable）退化成 wire-friendly 摘要 |

### Spring Observation vs 直接 OTel

embabel 用 Spring Observation API + micrometer：在 `EmbabelTracingObservationHandler` 里维护 `activeAgentSpans` / `activeActionSpans` / `activeLlmSpans` / `activeToolLoopSpans` 多个 active-span 注册表，自动 parent-resolution，标 `embabel.event.type` / `embabel.run_id` 等 baggage。Span 类型有 13 种（`AGENT_PROCESS / ACTION / GOAL / TOOL_CALL / LLM_CALL / TOOL_LOOP / RAG / RANKING / PLANNING / STATE_TRANSITION / LIFECYCLE / CUSTOM`）。

lynx 直接 `otel.Tracer("lynx/agent")`：planner 和 action 各自 `tracer.Start(ctx, ...)` 起 span。比 embabel 浅 — 但也少了 baggage propagation 和 micrometer metrics 的桥接。

**取舍：** embabel 更 enterprise-grade（多 active-span 注册表、自动 parenting、metrics handler 桥接）；lynx 更 minimal（一个 tracer、手动 instrumentation）。

---

## 9. DI / 配置

| | embabel | lynx |
|---|---|---|
| 配置入口 | Spring `@Bean` / `@Autowired` 构造注入 | `runtime.PlatformConfig` 结构体直接初始化 |
| 服务注册 | `ProcessContext` 持有 `LlmOperations` / `RagService` / `ToolGroupResolver` 等具体类型槽位 | `core.ServiceProvider` 是 `map[string]any`，用户用 `Set("chat", anything)` 塞，action 里用 `core.ServiceOf[T](pc.Services, "chat")` 取 |
| LLM 抽象 | 框架自带 `ChatClient` / `LlmOperations` 接口（绑 Spring AI 生态） | **不绑生态** — 框架不定义任何 LLM/RAG/VectorStore 接口 |

我们经历过：
- v0：`functional options`（`WithChatClient(c) PlatformOption`）
- v1：fixed-shape struct（`PlatformConfig{Chat, RAG, VectorStore, Tools}`）
- **v2：generic registry**（当前）

理由是不绑生态：embabel 选了 Spring AI 风格的 `ChatClient`；langchain 又是另一种；不同 RAG 框架更是五花八门。Lynx **不参与抽象大战**。

### 测试支持

embabel 提供 `EmbabelMockitoIntegrationTest`：Spring Boot test-slice + `@MockitoBean LlmOperations` + 谓词式 stubbing helpers（`whenCreateObject(matcher, T)`）。

lynx：没有。Mock 走 Go 标准做法（手写 stub 或 gomock）。`runtime.NewCounterIDGenerator(prefix)` 提供测试用的确定性 ID。

---

## 10. HITL（人在回路）

| | embabel | lynx |
|---|---|---|
| 根接口 | `Awaitable<P, R : AwaitableResponse>` | `core.Awaitable`（非泛型）+ `hitl.Request[P, R]`（泛型） |
| 具体类型 | `ConfirmationRequest` / `FormBindingRequest` / `UserInputRequest` | 前两个 |
| Suspend 机制 | 抛 `UserInputRequiredException` | action 返回 `core.ActionWaiting` + 把 awaitable 写进 atomic.Pointer |
| Resume 入口 | `Awaitable.onResponse(response, agentProcess): ResponseImpact` | `Platform.ResumeProcess(id, response) (ResponseImpact, error)` |
| Handler 触发 | embabel 在 resume 时同步调 typed `OnResponse` | lynx 通过 `core.Awaitable.OnResponseAny(any) (ResponseImpact, error)` 路由到 typed handler |

**最近修复**：lynx 的 `deliverResponse` 之前是个**死管道** —— 把 response 写进 `slot.respond` channel 但没人读，handler 永远不会跑（commit `445c23d` 修）。现在通过 `OnResponseAny` 在 resume 时同步执行 handler，再把 `ResponseImpact` 冒泡给 `Platform.ResumeProcess` 调用方，由调用方决定是否重启 run loop（保持 resume 接口 ctx-free + 同步）。

---

## 11. 启动校验

**embabel 三层 validator pipeline**（`AgentValidationManager`）：

1. **Structure validator**（`DefaultAgentStructureValidator`）— 检查至少 1 个 goal、action 名无重复、方法签名约束。
2. **GOAP path-to-completion validator**（`GoapPathToCompletionValidator`）— **真正调用 GOAP planner** 跑一遍，确保从初始状态能到每个 goal。识别「first action」（无外部依赖 / 显式 FALSE 依赖），然后对每个 goal 验证可达。
3. **结果聚合**（`DefaultAgentValidationManager.afterPropertiesSet`）— Spring `InitializingBean` lifecycle 期间跑，汇总错误、记日志、不阻塞启动（warning 级）。

**lynx 完全没有这层。** Agent 定义错（写错 effect key、precondition 永远不满足、no goal）的话，运行时第一次 tick 才发现。

**有意思的是 embabel 用自己的 planner 做 startup check** — 比静态分析能多发现一些非显然的 unreachable 情况。**值得借鉴。**

---

## 12. SupervisorAgent — LLM 驱动的多 agent 选择

embabel 的 `SupervisorAgentFactory` 把多个 action 包装成「LLM 决定下一步」的 supervisor agent：

1. 把所有非-goal action 暴露成 LLM tool
2. SupervisorAction 跑一个 agentic 循环（最多 10 轮）
3. 每轮：把 currently-applicable 的 action 当 tool 给 LLM；LLM 选一个工具；执行；检查 goal 是否达成
4. 直到 goal action 输入齐全，跑 goal action 收尾

**对比：** lynx 的 planner 出**线性 plan**；embabel 的 supervisor 让 **LLM 决定下一步**。前者有 GOAP 保证（最优、可达性、可解释），后者更灵活但失去 planner 的形式保证。

**lynx 故意不做这个。** 我们认为 GOAP 的形式保证是核心价值；supervisor 模式可以通过 user code 用 `Platform.CreateChildProcess` 自己组合（child agent 调 child agent），不需要框架内置另一种 planner。

---

## 13. @State 解构 — embabel 的状态机模式

```kotlin
@Agent(description = "Order")
sealed class OrderAgent {
    @State data class Pending(...) : OrderAgent() {
        @Action fun process(): InProgress { ... }
    }
    @State data class InProgress(...) : OrderAgent() {
        @Action fun complete(): Completed { ... }
    }
    @State data class Completed(...) : OrderAgent()
}
```

`AgentMetadataReader.unrollStateType()` 递归扫描 sealed 子类，把每个 `@State` 类的 `@Action` 方法**flatten 进根 agent 的 action 列表**。这让状态机式工作流写起来像 Kotlin sealed class。

**lynx 没有等价物。** Go 没有 sealed class 概念，要做类似事情得用大量 reflect 或代码生成。我们刻意走显式：用户自己用 `core.Goal.Pre` 字段串起状态依赖。

---

## 14. ProcessOptions / Budget

| 字段 | embabel | lynx |
|---|---|---|
| Budget | `Budget(cost, actions, tokens)` 默认 $2 / 50 / 1M | 同（默认值一致） |
| Verbosity | `Verbosity(showPrompts, showLlmResponses, debug, showPlanning)` | 同 |
| ProcessControl | `toolDelay + operationDelay + earlyTerminationPolicy` | 同 |
| EarlyTerminationPolicy | `firstOf(maxActions, maxTokens, hardBudgetLimit)` | `MaxActionsPolicy / BudgetPolicy / CompositePolicy` |
| Subtree 预算 | `cost() = ownCost + childProcesses.sum { it.cost }` 递归 | 同 — `AgentProcess.Usage()` 递归走 children；`MaxActionsPolicy` 和 `BudgetPolicy` 自动是子树作用域 |
| Cost 来源 | `LlmInvocation.cost()` 用 `pricingModel.costOf(usage)` 算 | 框架不算 cost — 用户监听 `LLMResponseEvent`，调 `pc.RecordUsage(cost, tokens)` 上报 |

子树预算汇总两边语义一致：parent agent 调 child agent 时，child 的 cost / tokens / actions 自动汇入 parent 上限。差别在 **cost 数值的来源**：embabel 内置 pricingModel（耦合具体 LLM provider 价格表），lynx 把 cost 计算推给集成层（更灵活，框架不参与定价）。

---

## 15. 整体取舍总结

### 我们刻意走窄的地方

1. **删掉了反射注册路径**（embabel 的 `@Agent` / `@Action` 风格在 Go 里是反模式）
2. **删掉了 LLM 客户端抽象**（`core.ChatClient` interface 不存在 — 用户在 `Services` 里塞任意类型）
3. **不内置任何外部集成**（MCP / RAG / shell / a2a / skills 全是其它模块的事）
4. **不依赖 Spring**（functional config + struct 替代 DI）
5. **不用异常做控制流**（`error` + signal channel 替代 `TerminateActionException` 等）
6. **不做 Megazord aggregation**（多输入显式 `Get[T]`，不靠反射拼装）
7. **不做 SupervisorAgent**（LLM 决定下一步交给 user code，框架不参与）
8. **不做 @State unrolling**（Go 没 sealed class，搞这个要不就 reflect 要不就 codegen，得不偿失）

### 我们继承的设计

1. **GOAP A\* + ConditionWorldState** — 几乎逐行对应
2. **Blackboard dual-binding**（默认 `it` + 类型派生键）
3. **TerminationScope（Agent / Action / ToolCall）**
4. **Awaitable 三种模式**（Confirmation / FormBinding 已实现，UserInput 没做）
5. **Budget + Verbosity + ProcessControl** 默认值
6. **`hasRun_X` 条件键防 plan 内重跑**
7. **Subtree 预算汇总**

### 已从 embabel 借完的（曾经的 TODO）

~~1. **AgentValidationManager** — 启动时跑校验~~ ✅ `core.ValidateAgent`（结构）+ `Platform.Deploy` 内的 `checkGoalsReachable`（一步反向 producer 扫描）。我们用了比 embabel 更轻的版本 — embabel 从空状态跑完整 planner，会误拒 input-binding 驱动的合法 agent；我们只检查「每个 goal precondition 至少有一个 producing action」，更宽松但够用。

~~2. **ToolCall cancellation 管道**~~ ✅ `Process.TerminateToolCall(reason)` 触发；`ProcessContext.ToolCallContext(parent) (ctx, cancel)` 让 action body 派生一个能被取消的 ctx，传给 chat client / tool 调用；caller defer cancel 释放。Cancellation 经 ctx.Done() 自然向下传播。

~~3. **版本化 ToolGroup 元信息**~~ ✅ `AssetCoordinates.Version` 从 `string` 升到 `*semver.Version`（用 Masterminds/semver/v3，跟 `AgentConfig.Version` 一致）。多版本 MCP server 共存可以排序比较。

（保留作为历史记录，方便回看曾经的差距清单。）

### 我们做得更干净的地方

1. **更严格的可见性** — runtime 把 `worldStateDeterminer` / `blackboardDeterminer` / `inMemoryBlackboard` / `newAgentProcess` 全部小写化；用户拿到的 API surface 比 embabel 干净一截。
2. **更窄的 ServiceProvider** — embabel 把 Chat / RAG / VectorStore 钉死在 ProcessContext；我们用 `map[string]any` 让它跟生态解耦。
3. **重试委托给 pkg/retry** — embabel 手写 backoff；我们直接用通用库，自动获得 jitter / ctx 传播 / 溢出保护。
4. **ActionQos 字段更少** — 5 → 3 字段。
5. **PlatformConfig 风格更 Go** — 不用 functional options，全是 struct field 直接初始化。
6. **没有反射路径** — 错误全在编译期暴露，IDE refactor 全程友好。
7. **事件原生 JSON** — 21 个 event 全部 `json.Marshaler`，自带 `event` 字段做 discriminator。embabel 靠 Jackson `@JsonTypeInfo` 才有同等效果。
8. **HITL 真的能跑** — 修复 `deliverResponse` 死管道之后，handler 现在真的会被调用（之前就是个声明性的 stub）。
9. **WorldState 真的不可变** — `ConditionWorldState.HashKey` 改成构造时算好（之前是 lazy + race），现在并发 tick 安全。
10. **更小的核心** — 5,667 LOC vs embabel 几万行（含集成）。少一个量级的代码意味着少一个量级的维护负担。

---

## 16. 文件级对照表

| 关注点 | embabel 文件 | lynx 文件 |
|---|---|---|
| Agent 实体 | `core/Agent.kt` | `core/agent.go` |
| Action 接口 | `core/Action.kt`（5 mixin） | `core/action.go`（单接口） |
| 注解扫描 | `api/annotation/support/AgentMetadataReader.kt` | （已删除） |
| DSL Builder | `api/dsl/AgentBuilder.kt` | `dsl/builder.go` |
| **启动校验** | `spi/validation/DefaultAgentValidationManager.kt` + `GoapPathToCompletionValidator.kt` | （未实现） |
| GOAP A\* | `plan/goap/astar/AStarGoapPlanner.kt` | `planner/goap/astar.go` |
| **Supervisor** | `api/annotation/support/SupervisorAgentFactory.kt` | （未实现） |
| WorldState | `plan/goap/ConditionWorldState.kt` | `plan/condition_world_state.go` |
| Blackboard 接口 | `core/Blackboard.kt` | `core/blackboard.go` |
| 内存 Blackboard | `core/support/InMemoryBlackboard.kt`（含 Megazord） | `runtime/in_memory_blackboard.go`（无 Megazord） |
| AgentProcess 抽象 | `core/AbstractAgentProcess.kt` | `runtime/agent_process.go` |
| Tick + 顺序执行 | `core/support/SimpleAgentProcess.kt` | `runtime/run.go` |
| 并发执行 | `core/support/ConcurrentAgentProcess.kt` | `runtime/concurrent.go` |
| Action 执行 | `core/support/ActionExecutor.kt` | `runtime/execute_action.go` |
| ActionQos | `core/ActionQos.kt`（5 字段） | `core/action_qos.go`（3 字段，retry 走 pkg/retry） |
| HITL Awaitable | `core/hitl/Awaitable.kt` | `core/awaitable.go` + `hitl/awaitable.go` |
| ProcessOptions | `core/ProcessOptions.kt` | `core/process_options.go` |
| EarlyTermination | `core/EarlyTerminationPolicy.kt` | `core/early_termination.go` |
| ToolGroup | `core/ToolGroup.kt` | `core/tool_group.go` |
| **DataDictionary** | `core/DataDictionary.kt` | （部分功能在 `core.DomainType`） |
| LLM 跟踪 | `core/LlmInvocation.kt`（含 pricingModel） | `event/event.go LLM*Event` + `Process.RecordUsage` |
| 事件 | `api/event/AgenticEvent.kt` 多文件 | `event/event.go`（21 类型集中 + JSON marshaler） |
| 多播 | `api/event/MulticastListener.kt` | `event/event.go Multicast` |
| Platform | `core/AgentPlatform.kt` | `runtime/platform.go` |
| 测试支持 | `embabel-agent-test-support/.../EmbabelMockitoIntegrationTest.java` | （无 — 用户自带 mock） |
| Observability | `embabel-agent-observability/.../EmbabelTracingObservationHandler.java` | 直接 `otel.Tracer("lynx/agent")` |

---

## 附：embabel HEAD 引用

本文档对照的 embabel 版本：`v0.4.0-SNAPSHOT`，commit `da1f1522d`（"Introduce NamedPropertyDefinition"）。

---

## 结论

lynx/agent 的设计图谱跟 embabel 走在同一条主路上 — GOAP / Blackboard / OODA loop / dual-binding / TerminationScope 都借鉴自 embabel。**真正的差异在「框架边界划在哪里」**：

- embabel 把 LLM / RAG / MCP / shell / a2a / skills / 反射注册 / Megazord 聚合 / Supervisor / @State unrolling 都纳入自己责任范围 — **batteries-included**。
- lynx 只管**规划 + 执行 + 状态管理**这三件事 — **minimal core**。其余推到外部（lynx 自己的其它 module 或用户代码）。

这次刷新主要更新了三类内容：
1. **校正了之前对 embabel 的误读**（PromptedTransformer 不存在）
2. **补充了之前盲点**（AgentValidationManager / SupervisorAgent / Megazord aggregation / @State unrolling / DataDictionary / Spring Observation）
3. **反映了 lynx 这边几次清理后的最新状态**（HITL 真的能跑、WorldState 真的不可变、更窄的 API surface）

三个借鉴 TODO 现在全部落地：startup validation、ToolCall cancellation 管道、版本化 ToolGroup 元信息。其它差异是设计取舍，不打算抹平。
