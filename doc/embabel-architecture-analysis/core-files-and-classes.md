# 核心文件与类索引

> 0.4.0-SNAPSHOT
> 所有路径相对于仓库根 `/Users/tangerg/Desktop/embabel-agent/`。
> 星号表示重要性:★★★ 核心概念 · ★★ 常见扩展点 · ★ 配套类型

---

## 1. 文件地图(核心模块 `embabel-agent-api/src/main/kotlin/`)

```
com/embabel/agent/
├── core/                                        ← 核心运行时
│   ├── Agent.kt                                 ★★★ Agent 数据模型
│   ├── AgentPlatform.kt                         ★★★ 平台入口接口
│   ├── AgentProcess.kt                          ★★★ 执行实例接口
│   ├── AgentScope.kt                            ★★  能力范围抽象
│   ├── Action.kt                                ★★★ Action 接口
│   ├── Goal.kt                                  ★★★ Goal 数据类
│   ├── Condition.kt                             ★★  条件与三值逻辑运算符
│   ├── Blackboard.kt                            ★★★ 黑板接口
│   ├── IOBinding.kt                             ★★  输入 / 输出绑定
│   ├── ProcessContext.kt                        ★★  执行上下文
│   ├── ProcessOptions.kt                        ★   进程配置
│   ├── ActionQos.kt                             ★   重试 / 超时 / 退避
│   ├── ActionRunner.kt                          ★★  带计时与异常映射的 action 执行工具
│   ├── LlmInvocation.kt                         ★★  一次 LLM 调用的记录(含 cost)
│   ├── AgentProcessRepository.kt                ★★  进程持久化 SPI 基类
│   │
│   ├── support/
│   │   ├── DefaultAgentPlatform.kt              ★★★ 平台默认实现
│   │   ├── AbstractAgentProcess.kt              ★★★ OODA 主循环、成本聚合、终止
│   │   ├── SimpleAgentProcess.kt                ★★★ 串行 formulateAndExecutePlan
│   │   ├── ConcurrentAgentProcess.kt            ★★★ 并发 formulateAndExecutePlan
│   │   ├── AbstractAction.kt                    ★★  Action 基类
│   │   ├── BlackboardWorldStateDeterminer.kt    ★★★ 黑板 → WorldState 桥
│   │   ├── InMemoryBlackboard.kt                ★★  线程安全黑板
│   │   ├── ActionQosExtensions.kt               ★   withEffectiveQos 实现(#1572)
│   │   └── LlmInteraction.kt                    ★★  LLM 调用规格 / 提示词组合
│   │
│   ├── internal/                                ← 不保证 API 稳定
│   ├── deployment/                              ← 部署工具
│   └── hitl/                                    ← HITL 支持(AwaitableResponseException)
│
├── api/
│   ├── annotation/
│   │   ├── annotations.kt                       ★★★ @Agent @Action @Goal 等
│   │   ├── RunSubagent.kt                       ★★  抛 SubagentExecutionRequest 的静态工厂
│   │   └── support/
│   │       ├── AgentMetadataReader.kt           ★★★ 注解扫描与校验(OperationContext 构造器拒绝)
│   │       ├── ActionMethodManager.kt           ★★
│   │       ├── ActionMethodArgumentResolver.kt  ★★  参数解析器 SPI
│   │       └── SupervisorAgentFactory.kt        ★
│   │
│   ├── dsl/
│   │   ├── agent.kt                             ★★★ agent { ... } 入口函数
│   │   └── AgentBuilder.kt                      ★★★ DSL 构建器
│   │
│   ├── common/
│   │   ├── OperationContext.kt                  ★★★ Action 作用域上下文(方法注入!)
│   │   ├── ActionContext.kt                     ★★  带 asSubProcess 能力的 OperationContext
│   │   ├── Transformation.kt                    ★★  I → O 变换抽象
│   │   ├── TerminationSignal.kt                 ★★  TerminationScope 枚举(#1593)
│   │   ├── autonomy/
│   │   │   ├── Autonomy.kt                      ★★★ 三种执行模式入口
│   │   │   ├── AgentProcessExecution.kt         ★★  执行结果包装
│   │   │   └── GoalChoiceApprover.kt            ★
│   │   ├── workflow/
│   │   │   ├── ScatterGather.kt                 ★★
│   │   │   ├── RepeatUntil.kt                   ★★
│   │   │   ├── RepeatUntilAcceptable.kt         ★
│   │   │   └── Consensus.kt                     ★
│   │   ├── streaming/                           ★★  StreamingPromptRunner
│   │   └── thinking/                            ★★  ThinkingPromptRunnerOperations
│   │
│   ├── tool/
│   │   ├── Tool.kt                              ★★★ Tool 接口,Definition.metadata(#1603)
│   │   ├── Subagent.kt                          ★★★ LLM 可调用的子 Agent 工具
│   │   └── callback/
│   │       └── SimpleToolLoopCallbacks.kt       ★★  含 SlidingWindowTransformer(#1569)
│   │
│   ├── event/                                   ★★  AgenticEvent 层次
│   ├── invocation/                              ★★  AgentInvocation / TypedInvocation / SupervisorInvocation
│   ├── models/                                  ★   静态模型目录
│   ├── validation/
│   │   └── guardrails/
│   │       ├── GuardRail.kt                     ★★
│   │       ├── UserInputGuardRail.kt            ★★
│   │       └── AssistantMessageGuardRail.kt     ★★ 支持 ThinkingResponse<*>(#1582)
│   └── termination/                             ★   TerminationSignalPolicy / EarlyTerminationPolicy
│
├── spi/                                         ← **所有可替换扩展点**
│   ├── PlannerFactory.kt                        ★★★
│   ├── BlackboardProvider.kt                    ★★★
│   ├── OperationScheduler.kt                    ★★★
│   ├── ToolGroupResolver.kt                     ★★★
│   ├── ToolDecorator.kt                         ★★
│   ├── AgentProcessIdGenerator.kt               ★
│   ├── ContextRepository.kt                     ★★
│   ├── LlmService.kt                            ★★★ (generic fluent)
│   ├── AutoLlmSelectionCriteriaResolver.kt      ★
│   ├── loop/
│   │   ├── ToolLoop.kt                          ★★★
│   │   ├── ToolLoopFactory.kt                   ★★
│   │   ├── ToolInjectionStrategy.kt             ★★  MatryoshkaTool 支持
│   │   ├── ToolNotFoundPolicy.kt                ★★  AutoCorrection / ImmediateThrow
│   │   ├── LlmMessageSender.kt                  ★★★ 非流式 SPI
│   │   └── streaming/
│   │       └── LlmMessageStreamer.kt            ★★★ 流式 SPI(#1581)
│   ├── streaming/
│   │   └── StreamingLlmOperations.kt            ★★
│   ├── expression/spel/
│   │   └── SpelLogicalExpression.kt             ★★  缺失变量 → FALSE(#950b32ce)
│   ├── validation/
│   │   ├── AgentValidator.kt                    ★★
│   │   └── ValidationPromptGenerator.kt         ★
│   └── logging/
│       └── ColorPalette.kt                      ★
│
└── filter/, experimental/                       ← 非稳定 / 实验特性

com/embabel/plan/                                 ← 通用规划抽象
├── Planner.kt                                   ★★★ Planner<S, W, P>
├── Plan.kt                                      ★★
├── PlanningSystem.kt                            ★★
├── WorldState.kt                                ★★
├── common/condition/
│   ├── ConditionDetermination.kt                ★★★ 三值 enum
│   ├── ConditionWorldState.kt                   ★★★ 状态表示 + plus(action)
│   └── WorldStateDeterminer.kt                  ★★
└── goap/astar/
    └── AStarGoapPlanner.kt                      ★★★ A* 实现 + 前后向剪枝
```

---

## 2. 必读的 20 个类(按层级)

### 2.1 核心模型层

| 类 | 文件 | 关键字段 / 方法 |
|----|------|----------------|
| **Agent** | `core/Agent.kt` | `actions: List<Action>` · `goals: Set<Goal>` · `conditions: Set<Condition>` · `planningSystem: PlanningSystem`(getter) |
| **Action** | `core/Action.kt` | `inputs/outputs: Set<IOBinding>` · `preconditions/effects: EffectSpec` · `qos: ActionQos` · `execute(ctx): ActionStatus` · `canRerun: Boolean` |
| **Goal** | `core/Goal.kt` | `pre: Set<String>` · `inputs` · `outputType` · `value: CostComputation` · `preconditions` 自动由 `pre + inputs` 合成 |
| **Condition** | `core/Condition.kt` | `evaluate(OperationContext): ConditionDetermination` · 重载 `not/or/and/inv` |
| **Blackboard** | `core/Blackboard.kt` | `objects: List<Any>` · `bind/bindProtected` · `getValue(var, type, dd)` · `setCondition/getCondition` · `spawn()` · `hide(obj)` |
| **AgentProcess** | `core/AgentProcess.kt` | `id` · `parentId?` · `status: AgentProcessStatusCode` · `blackboard` · `planner` · `history: List<ActionInvocation>` · `tick()/run()` · `cost()/usage()/modelsUsed()` |

### 2.2 执行引擎层

| 类 | 文件 | 作用 |
|----|------|------|
| **AbstractAgentProcess** | `core/support/AbstractAgentProcess.kt` | 通用 OODA 循环 / 重试 / 事件 / 成本聚合 / 终止 |
| **SimpleAgentProcess** | `core/support/SimpleAgentProcess.kt` | 串行 `formulateAndExecutePlan`;处理 `ReplanRequestedException` |
| **ConcurrentAgentProcess** | `core/support/ConcurrentAgentProcess.kt` | 用 `Asyncer` 并行多个 achievable action;状态聚合优先级 |
| **BlackboardWorldStateDeterminer** | `core/support/BlackboardWorldStateDeterminer.kt` | 遍历 known conditions,按格式分别求值(SpEL/类型绑定/`hasRun`/对象条件) |
| **InMemoryBlackboard** | `core/support/InMemoryBlackboard.kt` | `ConcurrentHashMap` + `synchronizedList` + `protectedKeys` |

### 2.3 规划层

| 类 | 文件 | 作用 |
|----|------|------|
| **AStarGoapPlanner** | `plan/goap/astar/AStarGoapPlanner.kt` | A* 搜索 + heuristic + backward/forward plan optimization |
| **ConditionDetermination** | `plan/common/condition/ConditionDetermination.kt` | `TRUE / FALSE / UNKNOWN` 三值 + `invoke(Boolean?)` 构造 |
| **ConditionWorldState** | `plan/common/condition/ConditionWorldState.kt` | `Map<String, ConditionDetermination>` + `plus(action)` |

### 2.4 编程模型层

| 类 | 文件 | 作用 |
|----|------|------|
| **AgentMetadataReader** | `api/annotation/support/AgentMetadataReader.kt` | 扫描 `@Agent` / `@EmbabelComponent`;校验构造器;unroll `@State` |
| **AgentBuilder** | `api/dsl/AgentBuilder.kt` | Kotlin DSL 构建器(`transformation` / `promptedTransformer` / `goal` / `condition`) |
| **Autonomy** | `api/common/autonomy/Autonomy.kt` | `runAgent` / `classifyAndRun` / `openMode` 入口;自治绑定(#306385b1) |
| **OperationContext** | `api/common/OperationContext.kt` | Blackboard + ToolGroupConsumer;暴露 `promptRunner()` / `ai()` / `parallelMap()` |

### 2.5 工具与守护层

| 类 | 文件 | 作用 |
|----|------|------|
| **Tool / Tool.Definition** | `api/tool/Tool.kt` | Definition 含 `metadata: Map<String,Any>`(#1603) |
| **Subagent** | `api/tool/Subagent.kt` | 通过 `AgentRef` 三种方式引用子 Agent;作为 LLM Tool |
| **SlidingWindowTransformer** | `api/tool/callback/SimpleToolLoopCallbacks.kt` | 历史裁剪 + 保留 tool call/result 配对(#1569) |
| **UserInputGuardRail / AssistantMessageGuardRail** | `api/validation/guardrails/` | 前后置守护;后者支持 `ThinkingResponse<*>`(#1582) |

---

## 3. 常用注解速查(`api/annotation/annotations.kt`)

```kotlin
@Agent(name, provider, description, version=Semver(), planner=PlannerType.GOAP, scan=true, opaque=false,
       actionRetryPolicy=..., actionRetryPolicyExpression="...")

@Action(description, pre=[], post=[], canRerun=false, readOnly=false, clearBlackboard=false,
        outputBinding=..., cost=0.0, value=0.0, costMethod="", valueMethod="",
        trigger=Any::class, actionRetryPolicy=..., actionRetryPolicyExpression="...")

@Condition(name="", cost=ZeroToOne.ZERO)

@Cost  // 可被 @Action.costMethod / valueMethod 引用,参数必须 nullable

@AchievesGoal(description, value=0.0, tags=[], examples=[], export=@Export())

@RequireNameMatch("myBinding")  // 参数上:强制绑定到指定名称而非 "it"

@EmbabelComponent  // 较轻:贡献 actions / conditions 但不是独立 Agent

@State  // 类上:其 action 会被递归 unroll 到宿主 Agent
```

---

## 4. 常用 SPI 速查(`api/spi/`)

```kotlin
// 规划器
fun interface PlannerFactory {
    fun createPlanner(opts: ProcessOptions, wsd: WorldStateDeterminer): Planner<*, *, *>
}

// 黑板
fun interface BlackboardProvider { fun createBlackboard(): Blackboard }

// 调度
interface OperationScheduler {
    fun scheduleAction(evt: ActionExecutionStartEvent): ActionExecutionSchedule
    fun scheduleToolCall(evt: ToolCallRequestEvent): ToolCallSchedule
}

// 工具组解析
interface ToolGroupResolver : HasInfoString {
    fun availableToolGroups(): List<ToolGroupMetadata>
    fun resolveToolGroup(req: ToolGroupRequirement): ToolGroupResolution
    fun findToolGroupForTool(name: String): ToolGroupResolution
}

// LLM
interface LlmService<THIS : LlmService<THIS>> : LlmMetadata, PromptContributorConsumer {
    fun createMessageSender(options: LlmOptions): LlmMessageSender
    fun withKnowledgeCutoffDate(date: LocalDate): THIS
    fun withPromptContributor(pc: PromptContributor): THIS
}

fun interface LlmMessageSender {                              // 非流
    fun call(messages: List<Message>, tools: List<Tool>): LlmMessageResponse
}

fun interface LlmMessageStreamer {                            // 流(#1581)
    fun stream(messages: List<Message>, tools: List<Tool>): Flux<String>
}

// Tool 定义(#1603)
interface Definition {
    val name: String; val description: String; val inputSchema: InputSchema
    val metadata: Map<String, Any> get() = emptyMap()
    fun withMetadata(k: String, v: Any): Definition
}

// 失败域(#1593)
enum class TerminationScope { AGENT, ACTION }

// 守护(#1582)
sealed interface GuardRail : ContentValidator<String> { val name; val description }
interface UserInputGuardRail : GuardRail { /* validate(messages, blackboard) */ }
interface AssistantMessageGuardRail : GuardRail {
    fun validate(resp: ThinkingResponse<*>, bb: Blackboard): ValidationResult
    fun validate(msg: AssistantMessage, bb: Blackboard): ValidationResult
}

// Token 估算(#1536)
@ApiStatus.Experimental
fun interface TokenCountEstimator<T> { fun estimate(content: T): Int }

// BYOK(#1609)
fun interface ByokFactory<out T> { fun buildValidated(): T }
```

---

## 5. 入门与调试指引

**我要追一个"agent 启动不运行"的问题** → 看 `AgentMetadataReader`(构造器注入错误?)→ `DefaultAgentPlatform.createAgentProcess`(PlannerFactory 返回 null?)→ `AStarGoapPlanner.planToGoalFrom`(无解?)

**我要追"成本为 0"的问题** → 0.4.0 前看 `AgentProcess.cost()` 是否递归;0.4.0 后看:是否被 `OperationContext` 构造器注入陷阱命中(用占位进程计费了,但占位进程不在父进程的子树里)

**我要追"工具调用死循环"** → `ToolNotFoundPolicy`(LLM 幻觉工具名?)· `SlidingWindowTransformer.maxMessages` · `ToolInjectionStrategy`(是否无限增加工具?)

**我要追"SpEL 条件为 UNKNOWN"** → 0.4.0 后缺失变量已改为 FALSE;如果仍是 UNKNOWN,检查 `Condition.evaluate()` 是否返回 null 或 non-Boolean

**我要加一个新 LLM 提供方** → 参考 `embabel-agent-autoconfigure/models/embabel-agent-anthropic-autoconfigure/`:写一个 `XyzModelLoader` + `XyzModelDefinition` + `XyzModelsConfig` + `xyz-models.yml`,然后建一个同名 starter POM。
