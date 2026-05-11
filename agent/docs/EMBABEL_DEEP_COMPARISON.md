# 深度对比：lynx/agent vs embabel-agent（2026Q2）

> 基线：embabel-agent HEAD `1042d5870`（2026-05-11，约 89.7k LOC Kotlin，
> 跨 16 个 Maven 子模块、723 个 main 源文件）vs lynx HEAD `3dfbe25`，
> 在 `feat/core-polish` 分支（2026-05-11，约 43.7k Go LOC，跨 9 个
> workspace 模块；`agent/` 自身 10.9k LOC）。
>
> 早一轮的 [`EMBABEL_GAP_ANALYSIS.md`](./EMBABEL_GAP_ANALYSIS.md) 是按
> 功能点逐项打分的"缺口清单"风格；本文按 **架构赌注（architectural
> bets）** 重新组织 —— 两个项目从完全不同的起点出发，各自做出一套
> 自洽的设计选择。这样写让差异变成"可决策的方向题"而不是"待修的缺口
> 列表"。

---

## 1. 架构层的不同赌注

| 维度 | embabel | lynx |
|---|---|---|
| **语言** | Kotlin 2 / JVM | Go 1.26 |
| **组合机制** | Spring DI + autoconfigure + 注解 | Go interface + 显式 `Extension` 注册表 |
| **Agent 声明方式** | `@Agent` / `@Action` / `@Condition` 反射驱动；**或** 程序化 DSL | 只有程序化 builder（`agent.New(name).Actions(...).Goals(...).Build()`） |
| **分发单元** | Spring Boot 应用（16 个 Maven 模块，按需选用） | Go 库（一个 go.work 里 9 个模块） |
| **自带 frontend** | Shell（带"人格"皮肤的 REPL）、MCP server、A2A server、REST | 没有 —— 作为库被嵌入 |
| **并发模型** | CompletableFuture / Java 线程 / 阻塞 | 原生 goroutine + `context.Context` |
| **持久化方案** | `BlackboardProvider` + `ContextRepository` + `AgentProcessRepository` SPI + Spring Data 插件 | 只有 `core.Blackboard` extension（prototype + `Spawn()`）；process 注册表故意是内存的 |
| **Tool-loop 原语** | 一等 `ToolLoop` SPI 配套策略（`ToolInjectionStrategy` / `LoopMemo` / `OneShotPerLoopTool` / `EmptyResponsePolicy`） | 隐式：`core/model/chat.ToolMiddleware` 把任何 chat 调用自动包成自驱动 tool 循环 |

embabel 押的是 "**framework**" —— 每个定制点都是一个 Spring bean；
autoconfigure 模块把 MCP / A2A / Shell / RAG 全部接好，给你一个可运行
的 Spring Boot 应用。lynx 押的是 "**library kit**" —— agent runtime 就是
一个可导入的包；认证、传输、持久化、部署都交给调用方。

这种取舍直接反映在源代码上：embabel 光是 `embabel-agent-api/` 就有
463 个 Kotlin 文件，覆盖从注解扫描到动态 agent 创建到 ranking 的全部
关注点。lynx 的 `agent/core/` 是 17 个 Go 文件；整个 runtime 能在 10.9k
LOC 里装下，是因为 "基础设施"（shell / server / autoconfigure）这些层
在 lynx 里根本不存在。

---

## 2. 核心抽象映射表

同样的概念在两边叫什么、长什么样：

| 概念 | embabel | lynx | 备注 |
|---|---|---|---|
| Agent 定义 | `Agent(name, version, conditions, actions, goals, stuckHandler, opaque, ...)`（`core/Agent.kt`） | `AgentConfig{Name, Version, Description, Actions, Goals, Conditions, StuckHandler, PlannerName}`（`core/agent.go`） | lynx 多了 `PlannerName` 让 agent 自己选 planner（embabel 用 `ProcessOptions.plannerType`） |
| 离散执行步 | `Action` 接口：`Action: DataFlowStep, ConditionAction, ActionRunner, DataDictionary, ToolGroupConsumer` | `Action` 接口：`Metadata()` + `Execute(ctx, pc) ActionStatus` | embabel 的 Action 重得多，组合了 5 个契约；lynx 用 `ActionMetadata` 拆开 |
| Goal | `Goal`（data class） | `Goal` struct，含 `Tags`、`Examples`、`Preconditions()` | 形态一致 |
| 谓词 | `Condition` | `Condition` 接口（`Name()` / `Cost()` / `Evaluate()`）+ `ComputedCondition` + `PromptCondition` | lynx 开箱有 LLM-backed `PromptCondition` |
| 共享状态 | `Blackboard: Bindable, MayHaveLastResult, HasInfoString`（`core/Blackboard.kt`） | `Blackboard` 接口（嵌入 `Extension`）+ ISP 拆成 `BlackboardReader / Writer`（`core/blackboard.go`） | lynx：ISP 拆分；embabel：单一巨接口 |
| 每次运行的实例 | `AgentProcess: Blackboard, Timestamped, Timed, OperationStatus, ...` | `Process` 接口 + 实现 `runtime.AgentProcess` | embabel 让 AgentProcess **继承** Blackboard（process **就是** blackboard）；lynx 把两者分开 |
| 世界快照 | `WorldState`（`plan/WorldState.kt`） | `WorldState`（`core/planning.go`） | 一致；lynx 显式给了 `HashKey()` 用于 A* 去重 |
| 每次运行的配置 | `ProcessOptions`（data class，约 10 个字段：`verbosity`、`budget`、`processControl`、`prune`、`ephemeral`、`listeners`、`plannerType`、`toolCallContext`...） | `ProcessOptions`（5 个字段：`Blackboard, Budget, OutputChannel, ProcessType, Extensions`） | embabel 把所有事塞 ProcessOptions；lynx 把大多数关注点推到 `Extensions` |
| 每次调用上下文 | `ProcessContext` | `ProcessContext`（`core/process_context.go`） | 大致等价 |
| 成本追踪 | `LlmInvocation` + `EmbeddingInvocation` + `Usage` 聚合在 `AgentProcess` 上 | `RecordUsage(cost, tokens)` 在 `Process` 上 + 子树聚合通过 `Usage()` | embabel 追踪 per-invocation 细节（model / prompt / response —— commit `a31cef363` 刚刚改进过）；lynx 只追踪滚动总和 |

---

## 3. 规划子系统

| 方面 | embabel | lynx |
|---|---|---|
| Planner 接口 | `Planner<S: PlanningSystem, W: WorldState, P: Plan>` —— 3 个泛型 | `plan.Planner`（无泛型）+ 嵌入 `core.Extension` |
| 内置 planner | A* GOAP（`plan/goap/astar/OptimizingGoapPlanner.kt`，抽象类）、Utility（`plan/utility/UtilityPlanner.kt`） | goap（带后向相关性剪枝的 A*，`goap/AStarPlanner`）、htn（`htn/Planner` + Library）、reactive（`reactive/Planner` —— utility 风格） |
| HTN | ❌ API 里没有 | ✅ 完整的 hierarchical task network（`plan/planner/htn/`） |
| Planner 选择 | `ProcessOptions.plannerType: PlannerType` enum + `PlannerFactory` Spring bean | `AgentConfig.PlannerName: string` —— runtime 在 extension 里按 `Name()` 匹配 |
| Reactive / utility | `UtilityPlanner`（没有 progress check，可能无限循环） | `reactive.Planner`（拒 0-progress action 避免循环，让 `StuckHandler` 接手） |
| 后向相关性剪枝 | ❌ | ✅ `plan/planner/goap/relevance.go`（STRIPS regression 反向追溯 action effects） |
| `PlanningSystem` 缓存 | per-process | per-agent，在 `plan.FromAgent` 里建 |

**实质差别：** lynx 有三个 planner（含 HTN）；embabel 有两个但只有一个
robust。lynx 的 reactive 从设计上规避无限循环；embabel 的 `UtilityPlanner`
只按 netValue 排序，可能死循环。lynx 在 A* 搜索前做后向剪枝。

---

## 4. SPI / 可插拔性对照

embabel 的 SPI（在 `agent/spi/` 下）：

| 接口 | 文件 | 作用 |
|---|---|---|
| `BlackboardProvider` | `BlackboardProvider.kt` | 每个 process 造一个 Blackboard |
| `AgentProcessIdGenerator` | `AgentProcessIdGenerator.kt` | 生成 process ID |
| `PlannerFactory` | `PlannerFactory.kt` | 从 ProcessOptions 构造 planner |
| `ToolGroupResolver` | `ToolGroupResolver.kt` | 解析 tool group 要求 |
| `ToolDecorator` | `ToolDecorator.kt` | 注册时包裹 tool |
| `ContextRepository` | `ContextRepository.kt` | 跨 session 的 context 存储 |
| `AgentProcessRepository` | `core/AgentProcessRepository.kt` | 持久化 agent process |
| `OperationScheduler` | `OperationScheduler.kt` | 可插拔调度（Pronto / Delayed / Scheduled 变体） |
| `LlmService` | `LlmService.kt` | LLM 调用抽象 |
| `AutoLlmSelectionCriteriaResolver` | `AutoLlmSelectionCriteriaResolver.kt` | 按 criteria 选 LLM |
| `ToolLoop` + `ToolLoopFactory` | `spi/loop/` | 整个 tool 调用循环本身是 SPI |
| `ToolInjectionStrategy`（含 3 个实现） | `spi/loop/` | 决定 tool 怎么呈现给 LLM |
| `EmptyResponsePolicy`、`ToolNotFoundPolicy` | `spi/loop/` | tool 循环内部的策略 |

lynx 的 SPI（SPI flattening 之后，全部嵌入 `core.Extension`）：

| 接口 | 包 | 作用 | 派发模式 |
|---|---|---|---|
| `Blackboard` | `core` | per-process 共享状态 | Prototype + `Spawn()`，注册时 last-wins |
| `IDGenerator` | `core` | 生成 process ID | Last-wins |
| `Planner` | `plan` | 挑 action 序列 | 按 name（匹配 `AgentConfig.PlannerName`） |
| `ToolGroupResolver` | `core` | 解析 tool group | First-hit |
| `ToolDecorator` | `core` | 包裹解析出的 tool | 多实例 fan-out（装饰链） |
| `ActionInterceptor` | `core` | 包裹 action 执行 | 洋葱式 fan-out |
| `AgentValidator` | `core` | deploy 时验证 agent | 多实例 fan-in（任一报错则失败） |
| `GoalApprover` | `core` | 投票否决 goal 选择 | 多实例 unanimous |
| `EarlyTerminationPolicy` | `core` | 提前终止（budget / policy） | 多实例 OR-compose（Budget 始终隐式参与） |
| `EventListener` | `runtime` | 订阅 platform 事件 | Multicast |

**结构差异：**

embabel 需要约 13 个 SPI 接口，因为框架拥有更多层（调度 / LLM 派发 /
tool-loop 策略 / process repository / context repository）。lynx 有 10 个
是因为：

1. chat client 在 `core/model/chat`，**不是** agent 的关注点。模型 /
   策略的选择是调用方在 `chat.NewClient` 时的工作，不是平台 SPI。
2. 调度（embabel 的 `OperationScheduler`）没有抽象 —— action 在调用
   goroutine 上立即执行，`workflow.Parallel` 处理 fan-out。延迟 / 时序
   不是 framework 关注点。
3. ToolLoop 是隐式的（`chat.ToolMiddleware` 把任何 chat 调用包成自驱动
   循环）。要换可以在 middleware 层替换，不需要专门的 SPI。
4. Process / context repository 故意不在 framework 里（见
   [`PERSISTENCE.md`](./PERSISTENCE.md) §4）；跨 session 持久化是 host
   应用的责任，Blackboard extension 覆盖 in-process 状态足矣。

embabel 的 "更多 SPI" 对一个拥有 runtime container 的 Spring framework
来说是合适的。lynx 的 "更少 SPI" 对一个把运维关注点交给调用方的 library
来说是合适的。两边都没错 —— 给 lynx 加新功能时该问的是 "**这是 agent
的关注点，还是 host 应用的关注点？**"。

---

## 5. Tool / LLM 集成

这是**设计分歧最大**的区域。

### embabel：tool-loop 作为一等 SPI

`embabel-agent-api/src/main/kotlin/com/embabel/agent/spi/loop/` 定义了：

- `ToolLoop` —— loop 接口（start / iterate / done）
- `ToolLoopFactory` —— 可插拔构造
- `ToolLoopResult<O>` —— 类型化结果
- `ToolInjectionStrategy` —— tool 怎么呈现给 LLM：
  - `UnfoldingToolInjectionStrategy` —— 按需展开
  - `ChainedToolInjectionStrategy` —— 流水线
  - `MatryoshkaTools` —— 嵌套 tool 层级
- `LoopMemo` —— 循环内状态（新加，commit `d5d428456`）
- `OneShotPerLoopTool` —— 包一层让 tool 每轮 loop 最多触发一次
  （commit `01ebec8f6`）
- `EmptyResponsePolicy` —— LLM 返回空时怎么办
- `ToolNotFoundPolicy` —— LLM 挑了未知 tool 怎么办
- `RequiredToolGroupException` —— 类型化的未满足要求

所以 tool / loop 在 embabel 里非常正式；LLM 循环是一等的平台关注点，
旋钮很多。

### lynx：tool-loop 作为 middleware

`core/model/chat/tool_middleware.go` 整体约 160 行：

```go
type ToolMiddleware struct{}
func NewToolMiddleware() (CallMiddleware, StreamMiddleware) { ... }
```

循环是递归 middleware：调 LLM → 它要 tool 就执行 → 把结果塞回 messages
重新调 LLM → 重复。对 action body 是透明的 —— `pc.ChatWithActionTools(ctx)`
返回一个已经装好这个 middleware 的 `chat.ClientRequest`。任意 tool 可以
声明 `ReturnDirect=true` 短路出去。

策略（once-only / retry / empty-response）以**显式装饰器**形式活在
`toolpolicy/`，在注册时套上：

```go
oneShotTool, _ := toolpolicy.OnceOnly(myTool, ...)
unlockedTool, _ := toolpolicy.Unlocked(myTool, condition)
hitlTool, _ := hitl.RequireConfirmation(myTool, prompter, onResponse)
```

所以 **lynx 通过组合而不是专门的 SPI 达到 embabel 的 per-loop 策略**。
取舍：

- ✅ lynx：少仪式；tool callback 就是普通 `chat.CallableTool`
- ❌ lynx：没有 `LoopMemo` 等价物（让 tool 读循环内状态）；需要这种的
  tool 得自己读写黑板
- ❌ lynx：没有 `ToolInjectionStrategy` 插件 —— 暴露哪些 tool 由
  action 声明时的 `ActionConfig.ToolGroups` 决定

`LoopMemo` 和 `OneShotPerLoopTool` 是 embabel 较新的引入。如果 lynx
开始遇到"tool 需要知道这轮调过没"这种模式，值得参考。

---

## 6. 事件系统

embabel（`agent/api/event/`）：

```
AgenticEvent（sealed）
├── AgentPlatformEvent
│   ├── AgentDeploymentEvent
│   ├── DynamicAgentCreationEvent
│   ├── RankingEvent（Request / ChoiceMade / CouldNotBeMade）
├── AgentProcessEvent
│   ├── AgentProcessCreationEvent
│   ├── AgentProcessReadyToPlanEvent
│   ├── ReplanRequestedEvent
│   ├── AgentProcessPlanFormulatedEvent
│   ├── StateTransitionEvent
│   ├── GoalAchievedEvent
│   ├── ActionExecutionStartEvent / ResultEvent
│   ├── ToolLoopStartEvent / CompletedEvent
│   ├── ToolCallRequestEvent / ResponseEvent
│   └── AgentProcessFinishedEvent（Completed/Failed/Waiting/Paused/Stuck）
├── LlmInvocationEvent
└── EmbeddingInvocationEvent
```

约 30 种事件，包括 per-tool-call 的 request/response 和 per-LLM-call 的
invocation 事件。默认 dispatcher 是 `MulticastAgenticEventListener`。

lynx（`agent/event/`）：

```
event.Event（接口：Timestamp / ProcessID / EventName）
├── BaseEvent
├── ActionExecutionStart / ActionExecutionResult / GoalAchieved
├── ReadyToPlan / PlanFormulated / ReplanRequested
├── AgentDeployed / AgentUndeployed
└── ProcessCreated / Completed / Failed / Stuck / Waiting / Killed / Terminated
```

15 种事件。**没有** per-tool-call request/response 的对应物，也没有
per-LLM-invocation 事件。如果 listener 要做 per-tool-call 观测，得自己
写 `ToolDecorator` 包一层。

**值得标注的 gap：** embabel 的 `ToolCallRequestEvent` /
`ToolCallResponseEvent` 是一等事件；lynx 要写 `ToolDecorator` 才能发出
这些事件。类似地 `LlmInvocationEvent`（最近还加了 per-call 成本追踪 ——
commit `a31cef363`）比 lynx 当前暴露的更细。

---

## 7. HITL（human-in-the-loop）

| 能力 | embabel | lynx |
|---|---|---|
| Awaitable 基础 | `Awaitable.kt` | `Awaitable` 接口（`core/awaitable.go`） |
| 确认流 | `ConfirmationRequest.kt` | `hitl.Confirmation` + `RequireConfirmation` tool 装饰器 |
| 表单 / 类型化请求 | `FormBindingRequest.kt`、`TypeRequest.kt` | `hitl.TypedRequest[Req,Resp]` + `RequireType[T]` |
| Tool 级 await | `AwaitableTypedTool.kt`、`AwaitingTools.kt` | `hitl.RequireAwait`（任何 `chat.CallableTool` 上的装饰器） |
| Resume 机制 | `wait.kt` + `AgentProcessRepository` 持久化 | `Platform.ResumeProcess(id, response)` |
| 跨重启持久化 | ✅ 通过 `AgentProcessRepository` Spring Data | ⚙️ 通过 blackboard（[`PERSISTENCE.md`](./PERSISTENCE.md) §4.1 软持久化模式） |

两边对 in-process HITL 的覆盖一致。embabel 对**跨重启** HITL 有更现成
的故事（因为 process 本身可持久化）。lynx 把这交给 host —— 黑板软持久化
能用，但你要写胶水。

---

## 8. embabel 有 / lynx 没有

这些是 lynx 故意不做的**部署 / 能力**模块（不是 framework 关注点）：

### 8.1 Shell frontend（`embabel-agent-shell/`）
Spring Shell REPL，带**人格化提示词**（Monty Python、Hitchhiker's
Guide、Severance、Colossus、Star Wars）。可爱且非平凡：提供完整的交互式
agent host。

### 8.2 A2A server（`embabel-agent-a2a/`）
Agent-to-agent 协议 server。11 个主文件：`AutonomyA2ARequestHandler`、
`EmbabelServerGoalsAgentCardHandler`、`A2AStreamingHandler`、
`TaskStateManager`、`AgentSkillFactory`、`AgentCardHandler`。实现了
Google 的 A2A 协议 —— 部署后的 embabel app 作为 peer agent 通过标准
协议被访问。

### 8.3 MCP server（`embabel-agent-mcp/`）
**Server 端** MCP —— embabel 把自己的 agent 导出成 MCP tool，让外部
host（Claude Desktop / Cursor 等）调用。另加 `embabel-agent-mcp-security`
做认证。lynx 有 MCP **client** 支持（`mcp/Provider`）—— 可以消费外部
MCP server —— 但没有自己把 agent 发布为 MCP tool（虽然 `runtime.AsMCPTool`
是构建这个能力的钩子）。

### 8.4 Autoconfigure 模块（`embabel-agent-autoconfigure/`）
7 个 Spring Boot autoconfigure 模块把一切端到端接好：
- `embabel-agent-platform-autoconfigure` —— 核心平台
- `embabel-agent-a2a-autoconfigure` —— A2A server
- `embabel-agent-mcpserver-autoconfigure` + `-security-autoconfigure`
- `embabel-agent-shell-autoconfigure`
- `embabel-agent-observability-autoconfigure`
- `embabel-agent-netty-client-autoconfigure`

lynx 没有 autoconfigure 对应物 —— 这是设计选择（它是 library）。

### 8.5 RAG（`embabel-agent-rag/`，99 个文件跨 5 个子模块）
- `rag-core` —— 摄取流水线（chunker / transformer / fetcher / 层级
  reader / RSS 处理）
- `rag-lucene` —— Lucene 后端
- `rag-neo` —— Neo4j 后端（graph RAG）
- `rag-tika` —— Apache Tika 内容提取
- `rag-pipeline` —— 流水线编排

lynx 的 `rag/` 模块要轻得多：query transformer（compression / rewrite /
translation）、query expander（multi）、augmenter（contextual）、refiner
（rank / deduplication）、retriever（vector-store 后端）、pipeline
middleware。**lynx 出货约 15 个 RAG 组件；embabel 出货约 80 个 + 四个
后端集成。** 实际 gap：lynx 没有开箱的 Neo4j / Lucene / Tika；用户在
retrieval 侧通过 `vectorstore.Store` 接口接入。

### 8.6 Skills（`embabel-agent-skills/`）
embabel 的 "skill" 概念：脚本化的 agent 能力，带 Docker / process script
执行引擎。大致对应 lynx 通过 `AsChatTool` 把 sub-agent 包成
`chat.CallableTool`，但有显式的脚本生命周期。被 A2A frontend 大量使用
（`FromGoalsAgentSkillFactory`）。

### 8.7 LLM provider 预设（`api/models/`）
9 个预设模块：Anthropic、DeepSeek、DockerLocal、GoogleGenAI、LmStudio、
MiniMax、MistralAi、Ollama、OpenAi —— 每个含精选的 model name + 默认
配置。lynx 出货 3 个（`models/anthropic`、`models/google`、`models/openai`）
且不做预设精选。

### 8.8 注解驱动的 agent 声明（`api/annotation/annotations.kt`）
`@Agent`、`@Action`、`@Condition`、`@Cost`、`@ToolGroup` 注解 + 反射
扫描器。用户写：

```kotlin
@Agent(name = "blog-writer", description = "...")
class BlogWriter {
    @Action fun draft(...) : Draft { ... }
    @Condition fun draftAcceptable(...) : Boolean { ... }
    @Action fun publish(...) : Published { ... }
}
```

embabel 在 boot 时发现并连线。lynx 没有注解等价物（Go 原生不支持那种
注解）—— agent 永远是通过 fluent builder 程序化构造。这是个**语言驱动**
的根本分歧；不是 bug，只是风格不同。

### 8.9 其他模块
- `embabel-agent-onnx/` —— 本地推理的 ONNX runtime
- `embabel-agent-openai/` —— OpenAI Spring AI 集成
- `embabel-agent-starters/` —— Ollama / Google GenAI 等的 Spring Boot
  starter（配置包）
- `embabel-agent-code/` —— 写代码场景的 agent 工具（CI / Git / JVM /
  Bash）—— 16 文件；大致对应一个 coding assistant 包

---

## 9. lynx 有 / embabel 没有

### 9.1 Workflow 原语（`agent/workflow/`）
7 个可复用的 agent 形态，每个吃一个 `Spec` 返回 `*core.Agent`：

- `Loop` —— 重复 action 直到条件满足
- `Parallel` —— 在输入上 fan-out
- `Sequence` —— 用类型化 input/output 串多个 agent
- `ScatterGather` —— 并行 + 聚合
- `RepeatUntil` —— 用自定义谓词重复
- `RepeatUntilAcceptable` —— 重复直到 LLM-evaluated `Feedback` 通过
- `Consensus` —— N 个投票者，达阈一致

embabel 在这个层级没有 workflow 原语 —— 多步编排靠链式 agent 调用或
自定义 action 组合。lynx 的 workflow 包是小但很顶用的一笔。

### 9.2 GOAP 的后向相关性剪枝（`plan/planner/goap/relevance.go`）
embabel 的 `OptimizingGoapPlanner` 在完整 action 集上做 A*；lynx 在 A*
搜索前**预过滤** action 到 "effects 能反向追溯到 goal preconditions 的
传递闭包"。对很多无关 action 的领域，这收缩 search frontier 而不损
optimality。

### 9.3 带 progress 守护的 reactive planner
lynx 的 `reactive.Planner` 拒绝 effects 不闭合任何 goal precondition
（= 0 progress）的 action。embabel 的 `UtilityPlanner` 只按 net value
排序，如果每个 applicable action 都有正 value 但 0 progress 就可能死
循环。记在 commit `fa6f9a6`（"OptimizingGoap + reactive guard"）。

### 9.4 HTN planner（`plan/planner/htn/`）
Hierarchical Task Network planner，含 `Library` of `Task` + `Method`
分解。embabel 没有 HTN。小众但真有用：领域知识本身就是层级化的场景
（烹饪 / build pipeline / 多步教程）。

### 9.5 Autonomy ranker（`runtime/autonomy/`）
`Autonomy` 编排器：给定用户输入 + N 个 deployed agent，由 `LLMRanker`
（或自定义 `Ranker`）对 `(agent, goal)` 候选打分，挑头号，运行。
`LLMPlanRanker` 是同一思路用于对一个 agent 出的多个 plan 排序。让 host
能在平台上面搭 "选哪个 agent" 的路由层。

embabel 的 `RankingEvent` 家族说明它内部也有类似机制，但面向用户的
`Autonomy`-style API 没在公开 SPI 里露出。

### 9.6 Sub-agent 作为 chat tool（`runtime/AsChatTool*`）
把任何 deployed agent 包成 LLM tool loop 可挑的 `chat.CallableTool`。
配合 `ChatWithActionTools`，让一个 agent 通过 LLM 的 tool 选择 spawn
另一个 agent。类型化（`AsChatTool[In, Out]`）和动态（`PublishAll` /
`AllAchievableTools`）两种变体都有。embabel 的 `@RunSubagent` 注解概念
类似但停在注解驱动层；不像 callable-tool wrapper 这样易组合。

### 9.7 扁平 SPI 形态（重构后）
lynx 里每个可插拔 SPI 都直接嵌入 `core.Extension`：

```go
type Blackboard interface { Extension; BlackboardReader; BlackboardWriter; Spawn(); Clear() }
type Planner   interface { Extension; PlanToGoal(...) (*Plan, error) }
type EarlyTerminationPolicy interface { Extension; ShouldTerminate(p) (bool, string) }
// ...共 10 个，全统一
```

没有 factory 层，没有 `XxxProvider` 包装。embabel 仍有
`BlackboardProvider` / `PlannerFactory` factory 层 —— 这是 Spring 约定，
但每个概念多一层间接。lynx 的扁平形态显著更简单。

### 9.8 IoC 数据驱动 tool 示范（`tools/fakeweather/cities.go`）
115 个城市在一张 `cityProfile` 表里，含 `{Latitude, Longitude, Elevation,
Zone, Polluted}`。所有系统代码（气候逻辑、天气合成）都从这张表里读；
加新城市就是加一行，不动代码。这是 "**行为由数据参数化，而不是由代码
参数化**" 的一个漂亮参考样例 —— embabel 没有可类比的。

### 9.9 现代 Go 习惯（Go 1.26）
- `errors.AsType[T]` 做类型化错误匹配
- 测试里用 `t.Context()`
- 启动 goroutine 用 `wg.Go()`
- benchmark 用 `b.Loop()`
- `slices` / `maps` / `cmp` / `iter` 包
- `sync.OnceValue` 做懒初始化
- `new(value)` 取字面量地址
- `omitzero` 处理 time.Time / Duration 零值

lynx 的 `agent/` 整个使用 Go 1.26 idiom。对 embabel 对比不重要，但作为
质量信号值得记一笔。

---

## 10. 成本 / observability

### embabel
- per-call 细节：`LlmInvocationEvent`、`EmbeddingInvocationEvent`、
  `AgentProcess.recordLlmInvocation()`、`recordEmbeddingInvocation()`
- `AgentProcess.totalCost()`、`totalUsage()`、`ownCost()`、`cost()` ——
  递归子树聚合含 embedding
- 每次调用成本独立追踪（commit `a31cef363`）
- `embabel-agent-observability` 模块（目前主要通过 Micrometer + Spring
  Actuator 脚手架化）

### lynx
- `Process.RecordUsage(cost, tokens)` + `Process.Usage() (cost, tokens,
  actions)` —— 只有聚合，没有 per-invocation 历史
- OTel 属性通过 `otel/log` / `otel/slog` exporter
- Span 覆盖：`lynx.agent.tick`、`lynx.agent.action`、
  `lynx.agent.planner.astar`、`lynx.agent.process_id` 等
- 没有 embedding 单独的成本，跟 chat 总成本混在一起

**Gap：** embabel 的 per-call invocation 追踪更细 —— 在调试 "哪一次
模型调用最贵" 时有用。lynx 只有聚合够 budget enforcement，但丢了归因。
lynx 加一个 `LLMInvocationEvent` 等价物能补这个 gap，代码量约 50 LOC +
在 chat middleware 里发 listener。

---

## 11. 近期动向

### embabel（近 ~20 commit，weeks）：
- `1042d5870` —— sub-agent ProcessOptions.listeners 传播修复
- `a31cef363` —— per-call 成本追踪
- `8247bc512` —— Ephemeral Agent Process（不持久化）
- `35b12148b` —— Action Retry 区分 retriable / non-retriable
- `0ee7356db` —— Anthropic 专属 caching
- `01ebec8f6` —— `OneShotPerLoopTool`
- `d5d428456` —— `LoopMemo`
- `3781d2317` —— Empty response policy

方向：**深挖 tool-loop 原语**、**更细的成本归因**、**持久化灵活性**
（ephemeral）、**更聪明的 retry 语义**。

### lynx（本分支近 ~20 commit）：
- `3dfbe25` —— 新一版 embabel 对比文档
- `c75068c` —— SPI flattening 之后的 doc 对齐
- `e4d2d75` —— user-facing 文档同步
- `e70c9fb` —— 现代 Go idiom
- `693dde2` —— gofmt -s 全仓
- `1485e11` —— EarlyTerminationPolicy → Extension（SPI flattening pt 3）
- `4a290b0` —— PlannerFactory + PlannerType enum 删除（SPI flattening pt 2）
- `70fe9d3` —— BlackboardFactory 删除（SPI flattening pt 1）
- `28a6eec` —— fakeweather city gazetteer + IoC 模式
- `140db5b` —— fakeweather 全部重写

方向：**结构性清理**（SPI flatten、重构后的 docs/code 对齐）、**现代 Go
现代化**。Agent 功能面已经稳定了几轮。

---

## 12. Gap 清单 —— lynx 可以考虑做什么

按我看到的 ROI 排序。都不紧急；这是路线图提示，不是任务清单。

| # | 项目 | 工作量 | 论点 |
|---|---|---|---|
| 1 | `LLMInvocationEvent` + per-call 成本追踪 | 低（约 80 LOC） | 闭合唯一明显的 observability gap；与 embabel 近期方向一致；对成本归因有用 |
| 2 | `event/` 加 `ToolCallRequestEvent` / `ResponseEvent` | 低（约 60 LOC） | 今天观测单次 tool 调用只能写 `ToolDecorator`；做成一等事件更干净 |
| 3 | `toolpolicy/` 加 `OneShotPerLoopTool` / `LoopMemo` 类比 | 中 | 真实用例：tool 应该每个 LLM tool loop 触发一次，或 tool 需要记住 "这轮调过没"。今天用户得给每个 tool 写黑板胶水 |
| 4 | sub-agent 的 `ProcessOptions.Extensions` 从父继承 | 低 | embabel 刚为 listener 做了（commit `1042d5870`）；lynx 应该镜像，对任何应该传播的 process-scope extension（或者文档化为什么不传 —— 当前 `AsChatTool` **不**传 process-scope extension 给 child） |
| 5 | 按错误类型做 retriable / non-retriable 分类的 retry | 中 | embabel commit `35b12148b`。lynx 的 `ActionQoS.MaxAttempts` 存在但对所有错误一视同仁；分类后能做更聪明的 retry |
| 6 | RAG：文本文件摄取（Tika 类比） | 中 | lynx 出货 `document.JSONReader` / `TextReader`；不处理 PDF / HTML / DOCX。要达到对等需要 tika 风格的内容提取器 |
| 7 | "Skills" 抽象 | 中-高 | embabel 通过 `Skills` 把 agent 导出成 MCP tool。lynx 有零件（`AsMCPTool` + sub-agent 注册）；把它们打包成更高层的 "skill manifest" 也许能解锁同样的用例 |
| 8 | Ephemeral process 标记 | 低 | 如果 lynx 将来引入 Process repository SPI，镜像 embabel 的 `ephemeral: Boolean`，让 Spring-AI 注入的一次性 process 不被持久化 |
| 9 | 注解驱动的 agent 声明 | 高；推测性 | Go 没有 JVM 风格的注解。可以探索代码生成（`//go:generate`），但价值不明，现有 fluent builder 已经够简洁 |
| 10 | Shell frontend / A2A server | 非常高；不推荐 | 这是**部署**关注点；lynx-as-library 是刻意的范围选择。Host 应用可以在上面搭这些，成本比 embabel 的方式低得多 |

---

## 13. lynx 真领先的地方

不夸张，具体清单：

1. **SPI 扁平度**：10 个 capability interface 直接嵌入 `core.Extension`，
   没有 factory 层。embabel 有 13 个 SPI 接口，包装约定混杂
   （`*Provider`、`*Factory`、`*Resolver`、直接接口）。
2. **规划严谨度**：GOAP 后向剪枝 + reactive 0-progress 守护 + HTN
   planner。embabel 只有一个 robust planner；lynx 有三个。
3. **Workflow 原语**：`Loop`/`Parallel`/`Sequence`/`ScatterGather`
   /`RepeatUntil`/`Consensus` 一等公民。embabel 用手工组合同样形状。
4. **Autonomy ranker**：显式的 `Ranker` / `PlanRanker` SPI 用于"让 LLM
   挑哪个 agent / 哪个 plan"。embabel 内部存在该机制但没作为面向用户的
   工具暴露。
5. **Sub-agent supervisor 模式**：`AsChatTool[In, Out]` 端到端类型化。
   embabel 的 `@RunSubagent` 是注解驱动；动态组合更难。
6. **构建时清晰度**：runtime 全部 10.9k LOC，做完除部署外的一切。embabel
   光是 API 模块就 463 个文件；整仓 723 main 文件 + 约 26 个 autoconfigure
   文件。大部分是 framework 基础设施（Spring bean、autoconfigure、反射
   扫描器），在 Go 库场景下根本不存在。

---

## 14. 战略定位总结

- **embabel** 是 "JVM 企业应用的 agent framework" —— 一切都有 autoconfigure，
  多 frontend（shell / A2A / MCP server），精选的 LLM 预设，深度持久化
  方案。上 embabel 的 ROI：拿到完整的 agent runtime + 部署 shell，代价
  是承诺 Spring 的生命周期和 DI 约定。

- **lynx** 是 "作为 Go 库的 agent runtime" —— 小 SPI，谨慎的抽象，零
  framework 锁定。上 lynx 的 ROI：嵌入到现有 Go 服务，自己挑持久化 /
  传输 / 认证 / observability。runtime 本身保持小。

两个都是合理 bet。上面的 gap 清单短是因为**核心 runtime 抽象已经收敛**
—— agent、action、goal、condition、blackboard、planner、GOAP、tool
calling、HITL —— 两个项目都有，语义对齐。剩下的差别主要是 "**盒子里
带多少 framework**"。

如果 lynx 哪天想加 embabel 风格的 deployable starter，它会是一个独立
模块（`lynx-agent-shell/`、`lynx-agent-a2a/`）建在当前 agent 库之上 ——
而不是对核心 runtime 的根本性改变。扁平 SPI / 纯库的设计选择并不阻碍
增长；它只是把核心保持小。

---

*对比结束。双方 HEAD 截至 2026-05-11。*
