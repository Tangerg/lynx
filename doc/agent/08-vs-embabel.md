# 08. 架构对比：lynx/agent vs embabel-agent

> **对照对象**：embabel-agent v0.4.0-SNAPSHOT (HEAD `da1f1522`)，定位为 Kotlin/JVM、Spring Boot 生态的 GOAP-based agent 框架。
>
> 本文档不是「谁更好」的论战，而是把两边在**同一个架构问题上的不同选择**显式列出来 — 让 lynx/agent 的每个设计决策都能找到对应的 embabel 对照点，以及我们为什么做了不同的取舍。

---

## 0. 一句话定位

| 维度 | embabel-agent | lynx/agent |
|---|---|---|
| 语言 / 平台 | Kotlin · JVM · Spring Boot | Go 1.25+ · 无 DI 容器 |
| 用户编程模型 | 注解扫描（`@Agent` / `@Action` / `@AchievesGoal`）+ Kotlin DSL | **唯一入口：泛型 + 流式 DSL**（删掉了反射注册路径） |
| 模块拆分 | 22 个 Maven 模块（按集成生态切） | 8 个 Go 包（按内部分层切，集成留给上层模块） |
| 总代码量级 | 极大（含 Spring 适配 + 多 LLM provider + RAG 子生态） | 5,387 LOC（仅核心，零生态绑定） |
| 默认规划器 | A\* GOAP（可切 Utility / Supervisor） | A\* GOAP（接口可换，但不内置其它 planner） |

我们刻意走「更小的核心 + 更显式的用户契约」，把 LLM provider、RAG、shell、a2a 这些都放到框架之外。

---

## 1. 模块结构

### embabel：22 个 Maven 模块

按**外部集成生态**切：

```
embabel-agent-api          ← 核心：Agent / Action / Goal / Planner / AgentProcess
embabel-agent-domain       ← 数据模型与类型
embabel-agent-common       ← 共享工具 + AI 元信息 + observability hook
embabel-agent-autoconfigure ← Spring auto-config（OpenAI/Anthropic/Gemini/Ollama 各一份）
embabel-agent-observability ← OTel + Spring Observation
embabel-agent-mcp          ← MCP 客户端 + 服务端
embabel-agent-rag          ← Lucene / Neo4j / Tika
embabel-agent-skills       ← YAML 定义的可执行 skill + Docker/Process 引擎
embabel-agent-a2a          ← Agent-to-Agent HTTP 协议
embabel-agent-shell        ← REPL CLI
embabel-agent-openai       ← OpenAI 适配
embabel-agent-onnx         ← ONNX 本地 embedding
embabel-agent-code         ← 代码执行工具
embabel-agent-starters     ← Spring Boot starter POMs
embabel-agent-test-support ← Mockito 测试支持
embabel-agent-dependencies ← BOM
```

### lynx：8 个 Go 包

按**内部分层依赖**切（DAG，无环）：

```
core/        ← 数据类型 + 接口（Agent, Action, Goal, Blackboard, WorldState, ...）
plan/        ← Plan, Planner 接口, PlanningSystem, ConditionWorldState
planner/goap ← A* 算法（独立子包，便于换其它 planner）
runtime/     ← Platform, AgentProcess, executeAction（执行引擎）
event/       ← 21 种生命周期事件 + Multicast
hitl/        ← 人在回路：Request[P,R] / ConfirmationRequest / FormBindingRequest
dsl/         ← Builder（用户唯一入口）
agent/       ← 顶层 re-export 包（type alias + 简短包装）
```

集成生态（MCP / chat / RAG / vectorstore / observability）**全部** 留在外面 — `lynx/core/...`、`lynx/mcp/...`、`lynx/otelbridge/...` 各自独立 module。

### 取舍

embabel 的 22 模块设计反映 Spring 生态「starter-per-provider」的传统；切换 LLM 就是换 starter dep。代价是仓库本身重，新人 onboarding 要先理解模块图。

我们这边走「核心保持 5K LOC、所有外部集成都是用户的事」的路线 — 用户拿着 lynx/agent 不会被迫连带引入 11 个生态依赖。`runtime.PlatformConfig.Services` 是个 `map[string]any` 注册表，用户自己塞 LLM client / RAG pipeline / 任意领域服务，框架不参与生态选择。

---

## 2. 用户编程模型

### embabel：注解 + Kotlin DSL（双轨）

主推注解：

```kotlin
@Agent(description = "Routes customer intents")
class IntentReceptionAgent(private val llm: ChatClient) {

    @Action
    fun classifyIntent(ctx: OperationContext, input: UserInput): Intent { ... }

    @Action
    @AchievesGoal(description = "Department determined", value = 1.0)
    fun resolve(intent: BillingIntent): Resolution { ... }

    @Condition
    fun intentClassified(ctx: OperationContext): Boolean = ...
}
```

`AgentMetadataReader`（591 行）在 Spring 启动时扫描 `@Agent` bean，反射读 method 签名生成 Action / Goal / Condition 元信息。同时也提供 Kotlin DSL `AgentBuilder` 作 fallback。

### lynx：流式 DSL（单一入口）

```go
agent.New(core.AgentConfig{
    Name:        "IntentReceptionAgent",
    Description: "Routes customer intents",
}).
    Actions(agent.NewAction("classifyIntent",
        func(ctx context.Context, pc *core.ProcessContext, input UserInput) (Intent, error) {
            ...
        },
        core.ActionConfig{},
    )).
    Goals(agent.GoalProducing[Resolution](core.Goal{
        Description: "Department determined",
        ValueStatic: 1.0,
    })).
    Build()
```

我们曾经实现了 `agent/reflect/Register(instance any)`（450 LOC），允许 struct + 方法名约定的反射注册 — **但删掉了**（commit `53ea5a3`）。理由：

| 反射注册的代价（Go 视角） | 影响 |
|---|---|
| 错误推迟到运行时 | 方法签名错了编译能过，启动 `Register()` 才挂 |
| IDE 重构失效 | rename method 不会同步任何注册 |
| 命名约定成为隐藏契约 | `Achieves` 前缀、`camelCase → snake_case` 转换 — grep 不到 |
| 反射 `.Call()` 慢 ~10× | LLM 主导的场景里不显眼，但是隐性税 |

Go 社区共识是「显式优于隐式」（`net/http`、`gin`、`cobra` 全部显式注册）。所以**只保留泛型 DSL** 这一种入口。

### 共同点

`core.NewAction[In, Out]` 用 Go 1.18+ 泛型在编译期保留 In/Out 类型，对应 embabel 的 Kotlin reflection-based 类型推导 — 我们靠语言原生泛型零成本拿到等效效果。

---

## 3. Action 模型

### Action 接口

| 维度 | embabel.Action | lynx.Action |
|---|---|---|
| 接口形态 | 多接口组合：`DataFlowStep + ConditionAction + ActionRunner + DataDictionary + ToolGroupConsumer` | 单接口：`Metadata() ActionMetadata + Execute(ctx, pc) ActionStatus` |
| 元信息 | 散落在 5 个 mixin 接口 | 统一在 `ActionMetadata` 结构体 |
| 输入/输出 | `Set<IoBinding>` | `[]IoBinding`（同名同义） |
| Preconditions / Effects | `EffectSpec`（map[string]ConditionDetermination） | 同 `EffectSpec` |
| QoS | `ActionQos`（maxAttempts/backoffMillis/multiplier/maxInterval/idempotent） | `ActionQos`（MaxAttempts/BaseDelay/MaxDelay）— 退避数学交给 `pkg/retry` |
| Cost / Value | 静态数 + `cost(state)` 动态函数 | 同（`Cost / CostFn`、`Value / ValueFn`） |
| `OperationContext` | method 参数注入 | 第一参数固定 `pc *core.ProcessContext` |
| `@RequireNameMatch` | 注解显式覆盖默认 binding | `core.ActionConfig.Inputs` 字段 |

### Action 函数签名

embabel：参数顺序自由，注解和 method-parameter 反射混合解析

```kotlin
@Action
fun handleBilling(@RequireNameMatch billing: BillingIntent, ctx: OperationContext): Resolution
```

lynx：签名固定为 `func(ctx, pc, In) (Out, error)`，`In/Out` 是泛型参数

```go
agent.NewAction("handleBilling",
    func(ctx context.Context, pc *core.ProcessContext, in BillingIntent) (Resolution, error) { ... },
    core.ActionConfig{},
)
```

### 取舍

embabel 灵活但靠反射；lynx 死板但编译期类型完整。**Go 没有方法范型**，所以多输入只能从 blackboard 手动 `core.Get[T](pc.Blackboard, name)` 取出 — 这是我们承担的 Go-语言-限制成本。

---

## 4. Planner

### A\* GOAP

两边算法基本一致：

| 步骤 | embabel | lynx |
|---|---|---|
| 早退 | 目标已满足 → 空 plan | 同 |
| 可达性预检 | `isGoalReachable()` 反向扫描 | （未实现 — 直接进 A\*） |
| 启发式 | 未满足条件个数 | 同 |
| 状态展开 | 逐 action 应用 effects 生成新 state | 同 |
| 闭集重开 | 找到更优路径时重新打开 closed 节点 | 同 |
| 后处理 | backward + forward planning optimization | （未实现） |

embabel 的 `OptimizingGoapPlanner`（A\* 父类）多两步 plan-level 优化：
- **Backward optimization**：反向从 goal 走，丢掉不为 goal 必要条件做贡献的 action
- **Forward optimization**：正向模拟，丢掉不实际推进 plan 的 action

我们目前没做这两步。**值得做** — 写在 TODO。

### 多种 planner

embabel：GOAP / Utility / Supervisor 三种，via `ProcessOptions.plannerType`

lynx：只有 GOAP；`runtime.PlatformConfig.PlannerFactory` 是 pluggable 接口，用户可以塞自己的 planner，但仓库不内置 utility-based 实现。

### WorldState

两边都用 `map[string]Determination` 不可变快照（`ConditionWorldState`）。`Apply(effects)` 返回新 state，planner 依赖不可变性做 closed-set dedup。

---

## 5. 运行时执行

### Tick 循环

| 阶段 | embabel | lynx |
|---|---|---|
| 入口 | `SimpleAgentProcess.tick()` 一次一动作；`run()` 循环到终止 | `runtime.AgentProcess.run(ctx)` 直接循环 |
| OBSERVE | `worldStateDeterminer` 扫黑板 | 同（接口私有化）|
| ORIENT | planner.bestValuePlan(start, system) | 同 |
| DECIDE | sequential：plan.actions[0]；concurrent：filter 可达 actions errgroup 并发 | 同 |
| ACT | `executeAction` + retry + panic recovery | 同 |
| 终止信号 | `TerminateActionException` / `TerminateAgentException` | `core.TerminationScopeSignal` 经 channel |
| Replan 信号 | `ReplanRequestedException` | `core.ReplanRequest`（实现 error 接口） |

embabel 用**异常控制流**表达 terminate / replan；我们用**普通 error 类型 + channel signal**。Go 不依赖异常做控制流，这是语言惯例差异。

### 重试

embabel：手写 backoff，`ActionQos` 有 5 个字段（maxAttempts / backoffMillis / multiplier / maxInterval / idempotent）。

lynx：`ActionQos` 缩到 3 字段（MaxAttempts / BaseDelay / MaxDelay），退避数学全部委托 `lynx/pkg/retry`（指数 + jitter + ctx 取消传播 + 溢出保护）。我们故意不内嵌退避算法 — 那是基础设施层的事。

### 并发执行

embabel 有 `ConcurrentAgentProcess` 但作者评论说「主要给 tool-call 并发用，action-level 并发不常见」。

lynx 有 `core.ProcessConcurrent` 模式 — 一个 tick 里 errgroup 并发执行所有当前可达 action，结果用 `mergeStatuses` 合并。`runtime/concurrent.go` 90 行实现完整。

### Panic 恢复

两边都在每个 action 边界 `recover()`。lynx 的 `core.ProcessContext.ExecuteSafely(ctx, action)` 把 panic-guard 内化成方法（之前是 `runtime/runWithPanicRecovery` free function），让 runtime 端调用点缩到一行。

---

## 6. Blackboard

### 接口规模

| | embabel | lynx |
|---|---|---|
| 接口方法数 | ~17 | 17 |
| 命名 binding | `_map: ConcurrentHashMap` | `named map[string]any` |
| 顺序对象 | `_entries: SynchronizedList` | `objects []any` |
| 隐藏 | `hiddens: SynchronizedSet` | `hidden []any`（slice + DeepEqual，因为黑板上可能有不可哈希的 slice/map） |
| Protected key | `protectedKeys: SynchronizedSet<String>` | `protected map[string]struct{}` |
| 条件 | `setCondition / getCondition` 走 `_map` 但**不进 _entries** | 单独 `conditions map[string]bool` |
| Spawn | 复制 map + entries，保留 protected | 同 |
| 类型聚合 | 反射尝试构造 composite type（用于多输入 action） | （没做 — 我们的多输入要求用户从 blackboard 显式 `Get[T]`） |
| Dual-binding | `addObject(value)` 进 entries；`bind(key, value)` 进 map | `Bind(value)` 进 `objects` + 派生类型键（`UserInput → "userInput"`） |

### 取舍

我们把 **embabel 的反射式 composite-type 聚合** 故意省了。多输入 action 在 lynx 里走显式：

```go
// 不能用一个 In 同时承载 Topic + Outline + Research
// 用 Outline 作 In，剩下从 pc.Blackboard 显式取
agent.NewAction("write",
    func(ctx, pc, in Outline) (Article, error) {
        topic, _    := core.Get[Topic](pc.Blackboard, core.DefaultBinding)
        research, _ := core.Get[Research](pc.Blackboard, core.DefaultBinding)
        ...
    },
    core.ActionConfig{Pre: []string{"it:" + core.TypeFullNameOf[Research]()}},
)
```

embabel 那边 method 签名直接 `(ctx, topic: Topic, outline: Outline, research: Research)`，反射拼装。我们觉得显式取出来更可读、更可重构。

---

## 7. 工具 / ToolGroup

### 抽象层级

| 类型 | embabel | lynx |
|---|---|---|
| `ToolGroup` 接口 | `metadata + tools: List<Tool>` | 同（`Metadata() + Tools(ctx)`） |
| `ToolGroupRequirement` | `role + requiredToolNames + terminationScope?` | `Role + TerminationScope` |
| 解析失败 | 抛 `RequiredToolGroupException` 或 termination exception | resolver 返回 `(nil, err)`，由 action 决定怎么处理 |
| `LazyToolGroup` | MCP 触发首次访问时拉 tools | 同（`NewLazyToolGroup(meta, loadFn)`） |
| `TerminationScope` | `ACTION / AGENT / TOOLCALL` | `TerminationScopeAction / Agent`（没有 ToolCall — 我们认为单工具粒度 termination 是 noise） |

### 哪里挂

embabel：tools 是 `Action` 的 mixin（`ToolGroupConsumer`），每个 action 携带自己的 requirement。

lynx：`core.ActionConfig.ToolGroups` 是字段；agent-scoped requirements 在 `core.AgentConfig.ToolGroupRequirements`。两层都支持。

`runtime.PlatformConfig.Tools` 是单独字段（不进 ServiceProvider）— 因为 framework 自己要消费它（`pc.ResolveTools`），属于运行时基础设施而非业务服务。

---

## 8. 事件 / 可观测性

### 事件类型

| | embabel | lynx |
|---|---|---|
| 类型数 | ~15+（sealed interface + Jackson `@JsonTypeInfo` 多态序列化） | 21 个 |
| 派发 | `AgenticEventListener` 三个 callback：onProcessEvent / onPlatformEvent / onLlmEvent | 单 `OnEvent(Event)`，事件用 type switch 分流 |
| Multicast | `MulticastListener` 链式包装 | `Multicast` 直接遍历 listeners 切片 + panic recovery |
| OTel 集成 | Spring Observation API + micrometer | 直接 `otel.Tracer("lynx/agent")`，无中间层 |
| 序列化 | `JsonTypeInfo(Id.SIMPLE_NAME)` 内置 polymorphic JSON | （没内置 — 用户自己用 reflect 或 type-assert） |

### 取舍

embabel 把 LLM 事件（`LlmInvocationEvent` 含 cost/tokens/model 等）作为内置类别，因为它对 LLM 整体生命周期负责。

lynx 不参与 LLM 调用细节 — 我们没有 `LlmEvent` 这种事件类别。如果用户想观测 LLM 调用，那是 chat client 自己 wire OTel 的事情，不该由 agent 框架重复一遍。

事件类型数不强求精简（现在 21 个）— Go 的 type-switch 监听器风格喜欢多个具体类型而非一个带 status 字段的大事件。

---

## 9. DI / 配置

### embabel：Spring DI

```kotlin
@Configuration
class MyAgentConfig {
    @Bean fun chatClient(): ChatClient { ... }
}

@Agent(description = "...")
class MyAgent(
    private val llm: ChatClient,        // 构造注入
    private val rag: RagService,         // 构造注入
)
```

`@OperationContext` 禁止构造注入（被 `rejectOperationContextConstructorInjection` 在 `AgentMetadataReader.kt` line 734 验证），必须 method 参数。

### lynx：functional config + ServiceProvider 注册表

```go
services := core.NewServiceProvider()
services.Set("chat",   lynxChatClient)         // 任意类型
services.Set("rag",    ragPipeline)
services.Set("oracle", domainService)

platform := agent.NewPlatform(runtime.PlatformConfig{
    Services:    services,
    Tools:       toolResolver,
    IDGenerator: runtime.NewCounterIDGenerator("test"),
})

// action 里类型化取出
client, _ := core.ServiceOf[*chat.Client](pc.Services, "chat")
```

我们经历过：
- v0：functional options（`WithChatClient(c) PlatformOption`）
- v1：fixed-shape struct（`PlatformConfig{Chat, RAG, VectorStore, Tools}`）
- **v2：generic registry**（`PlatformConfig{Services map[string]any}`）— 当前

理由是不绑生态：embabel 选了某种 LLM 抽象（`ChatClient` Spring AI 风格）；langchain 又是另一种；不同 RAG 框架更是五花八门。Lynx 选**不参与抽象大战** — `ServiceProvider` 是个 `map[string]any`，用户自己决定塞什么类型。

### 测试支持

embabel：`@MockitoIntegrationTest` Spring Boot 测试 slice。

lynx：没有 Spring，所以不需要专门的 test-support 模块。`runtime.NewCounterIDGenerator(prefix)` 提供确定性 ID 生成器；mock 走 Go 标准做法（手写 stub 或用 `gomock`）。

---

## 10. HITL（人在回路）

| | embabel | lynx |
|---|---|---|
| 根接口 | `Awaitable<P, R : AwaitableResponse>` | `core.Awaitable`（非泛型，给 runtime 用）+ `hitl.Request[P, R]`（泛型，给用户用） |
| 具体类型 | `ConfirmationRequest` / `FormBindingRequest` / `UserInputRequest` | 同（除最后一个） |
| Suspend 机制 | 抛 `UserInputRequiredException` | action 返回 `core.ActionWaiting` + 把 awaitable 写进 atomic.Pointer |
| Resume 入口 | `Awaitable.onResponse(response, agentProcess): ResponseImpact` | `Platform.ResumeProcess(id, response) error` |
| Response Impact | `UPDATED` 触发重 plan | 同（`core.ResponseImpactUpdated`） |

我们故意把根 Awaitable 拆成两层：`core.Awaitable` 非泛型，所以 `core.Process.AwaitInput(req)` 不需要带泛型参；`hitl.Request[P, R]` 是用户那一层的强类型契约。

---

## 11. 外部集成

### embabel：内置 11+ 个集成模块

`mcp` / `rag-{lucene,neo,tika,pipeline}` / `skills` / `a2a` / `shell` / `openai` / `onnx` / `code` / `webmvc` 等。每个模块都跟 framework 核心紧耦合。

### lynx：零集成内置

| 集成 | embabel | lynx |
|---|---|---|
| MCP | `embabel-agent-mcp` 内置（client + server） | 复用 `lynx/mcp/...` 顶层模块；agent 不参与 |
| RAG | 4 个子模块（Lucene/Neo4j/Tika/pipeline） | 用户自己注册到 `Services["rag"]`（lynx 顶层有 `core/rag` 但 agent 不知道） |
| LLM | OpenAI / Ollama / Anthropic / Gemini / ONNX 各一个 | 用户自己注册到 `Services["chat"]`（用 `lynx/core/model/chat` 或任何第三方 SDK） |
| Shell REPL | `embabel-agent-shell` | （未实现） |
| A2A | `embabel-agent-a2a` HTTP 协议 | （未实现） |
| Skills | YAML schema + Docker/Process 引擎 | （未实现） |

embabel 是「电池齐全」（batteries-included），开箱即用 MCP / Anthropic / Lucene RAG。

我们是「最小内核」（single-responsibility）— 集成生态由用户和别的 lynx 模块解决。framework 不做生态绑定，意味着我们不会因为某个 SDK breaking change 而影响。

哪种好？取决于业务定位。embabel 适合 enterprise 一次性「装一套就能跑」；lynx 适合做底层的、需要长期演进的系统。

---

## 12. ProcessOptions / Budget

| 字段 | embabel | lynx |
|---|---|---|
| Budget | `Budget(cost, actions, tokens)` 默认 $2 / 50 / 1M | 同（默认值一致） |
| Verbosity | `Verbosity(showPrompts, showLlmResponses, debug, showPlanning)` | 同 |
| ProcessControl | `toolDelay + operationDelay + earlyTerminationPolicy` | 同 |
| EarlyTerminationPolicy | `firstOf(maxActions, maxTokens, hardBudgetLimit)` | `MaxActionsPolicy / BudgetPolicy / CompositePolicy` |
| Identities | `forUser + runAs` | 同 |
| Listeners | `List<AgenticEventListener>` 通过 ProcessOptions 传 | `event.Listener` 通过 `PlatformConfig.Listeners` 传 |
| Subtree 预算 | `cost() = ownCost + childProcesses.sum { it.cost }` 递归汇总 | （未实现 — 子进程独立计费） |

embabel 的子树预算汇总是个真亮点：parent agent 调用 child agent 时，child 的 cost 自动算进 parent 的预算上限。我们当前不做，因为 child process 用例还少。**值得抄。**

---

## 13. 整体取舍总结

### 我们刻意走窄的地方

1. **删掉了反射注册路径**（embabel 的 `@Agent` / `@Action` 风格在 Go 里是反模式）
2. **删掉了 LLM 客户端抽象**（`core.ChatClient` interface 不存在 — 用户在 `Services` 里塞任意类型）
3. **不内置任何外部集成**（MCP / RAG / shell / a2a 全是其它模块的事）
4. **不依赖 Spring**（functional options + struct config 替代 DI）
5. **不用异常做控制流**（`error` + signal channel 替代 `TerminateActionException` 等）

### 我们继承下来的设计

1. **GOAP A\* + ConditionWorldState** — 几乎逐行对应
2. **Blackboard dual-binding**（默认 `it` + 类型派生键）— 直接学 embabel 0.4
3. **TerminationScope（Action / Agent）**
4. **Awaitable / ConfirmationRequest / FormBindingRequest** 命名与语义
5. **Budget + Verbosity + ProcessControl** 默认值
6. **Action `canRerun` + `hasRun_X` 条件键**（避免一个 plan 内重跑）

### 还想从 embabel 借的（TODO）

1. **Backward + forward planning optimization**（A\* 后处理）
2. **子进程预算汇总**（递归累计 cost / tokens）
3. **goal reachability 预检**（在跑 A\* 前快速判 goal 是否可达，省掉 worst-case 搜索）

### 我们做得更好的地方

1. **更严格的可见性** — runtime 把 `WorldStateDeterminer` / `BlackboardDeterminer` / `InMemoryBlackboard` / `NewAgentProcess` 这些 executor 全部小写化；用户拿到的 API surface 比 embabel 干净一截。
2. **更窄的 ServiceProvider** — embabel 把 Chat / RAG / VectorStore / Tools 钉死在 ProcessContext；我们用 `map[string]any` 让它跟生态解耦。
3. **重试委托给 pkg/retry** — embabel 手写 backoff；我们直接用通用库，自动获得 jitter / ctx 传播 / 溢出保护。
4. **ActionQos 字段更少** — 5 字段 → 3 字段，把退避数学交给 `pkg/retry`。
5. **PlatformConfig 风格更 Go** — 不用 functional options，全是 struct field 直接初始化，defaults 在 `ApplyDefaults` 里集中。
6. **没有反射路径** — 错误全在编译期暴露，IDE refactor 全程友好。

---

## 14. 文件级对照表（速查）

| 关注点 | embabel 文件 | lynx 文件 |
|---|---|---|
| Agent 实体 | `core/Agent.kt` | `core/agent.go` |
| Action 接口 | `core/Action.kt`（5 mixin） | `core/action.go`（单接口） |
| 注解扫描 | `api/annotation/support/AgentMetadataReader.kt` | （已删除 — `agent/reflect/` 不存在） |
| DSL Builder | `api/dsl/AgentBuilder.kt` | `dsl/builder.go` |
| GOAP A\* | `plan/goap/astar/AStarGoapPlanner.kt` | `planner/goap/astar.go` |
| WorldState | `plan/goap/ConditionWorldState.kt` | `plan/condition_world_state.go` |
| Blackboard 接口 | `core/Blackboard.kt` | `core/blackboard.go` |
| 内存 Blackboard | `core/support/InMemoryBlackboard.kt` | `runtime/in_memory_blackboard.go` |
| AgentProcess 抽象 | `core/AbstractAgentProcess.kt` | `runtime/agent_process.go` |
| Tick + 顺序执行 | `core/support/SimpleAgentProcess.kt` | `runtime/run.go` |
| 并发执行 | `core/support/ConcurrentAgentProcess.kt` | `runtime/concurrent.go` |
| Action 执行 | `core/support/ActionExecutor.kt` | `runtime/execute_action.go` |
| ActionQos | `core/ActionQos.kt` | `core/action_qos.go` |
| HITL Awaitable | `core/hitl/Awaitable.kt` | `core/awaitable.go` + `hitl/awaitable.go` |
| ProcessOptions | `core/ProcessOptions.kt` | `core/process_options.go` |
| EarlyTermination | `core/EarlyTerminationPolicy.kt` | `core/early_termination.go` |
| ToolGroup | `core/ToolGroup.kt` | `core/tool_group.go` |
| 事件 | `api/event/AgenticEvent.kt` 及多文件 | `event/event.go`（21 类型集中） |
| Platform | `core/AgentPlatform.kt` | `runtime/platform.go` |

---

## 附：embabel HEAD 引用

本文档对照的 embabel 版本：`v0.4.0-SNAPSHOT`，commit `da1f1522d`（"Introduce NamedPropertyDefinition"）。

---

**结论**：lynx/agent 的设计图谱跟 embabel 走在同一条路上 — GOAP / Blackboard / OODA loop / dual-binding / TerminationScope 这些核心概念都借鉴自 embabel。**真正的差异在「框架边界划在哪里」**：embabel 把 LLM/RAG/MCP/shell/a2a 都纳入框架自己的责任；我们刻意把这些推到框架外，让 core 只关心**规划 + 执行 + 状态管理**这三件事。
