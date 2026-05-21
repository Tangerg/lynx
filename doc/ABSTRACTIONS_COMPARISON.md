# 抽象对比总览：lynx vs spring-ai / lynx-agent vs embabel-agent

> **基线**
> - lynx HEAD `4c762f8`（branch `feat/message-parts-impl`，2026-05-19）
> - spring-ai 主干（2026-05 范围）
> - embabel-agent 主干（2026-05 范围，Kotlin / Spring Boot）
>
> 本文是**两份对比的合订本**：
> - **Part I**：lynx 核心库 vs spring-ai（基础抽象层）——本节是高层综述，**完整版本见同目录 `SPRING_AI_COMPARISON.md`**
> - **Part II**：lynx/agent 子系统 vs embabel-agent（agentic 运行时层）——本节是**全新**深度对比
> - **Part III**：两条对比线交叉综合，给出 lynx 整体定位

---

## 0. 一图看懂三方关系

```
                   ┌─────────────────────────┐
   基础抽象层      │  Chat / Embedding / RAG  │
   (LLM 适配)      │  Tool / Memory / Doc     │
                   └────────────┬─────────────┘
                                │
                  ┌─────────────┼─────────────┐
                  │             │             │
                spring-ai       │           lynx-core
                  │             │             │
                  ▼             │             ▼
              （直接对比）       │       （直接对比）
                  │             │             │
                  └──┬──────────┘             │
                     │                        │
              ┌──────┴────────┐               │
              │ embabel-agent │ ←──────（构建于 spring-ai 之上）
              └──────┬────────┘               │
                     │                        │
                     │     ┌──────────────────┘
                     │     │
                     ▼     ▼
                  agentic 运行时层
              （Action / Goal / Condition / GOAP / Blackboard / Process）
                     │     │
                  embabel  lynx-agent
                  (Kotlin) (Go)
                     │     │
                     └─────┘
                    （直接对比）
```

**三组对比的不同性质：**
1. **lynx-core vs spring-ai**：同一层（LLM 抽象库），不同语言生态、不同打包哲学
2. **lynx-agent vs embabel-agent**：同一层（agentic runtime），**两侧使用了高度相似的 GOAP 思想**——这是本次对比最有趣的发现
3. **embabel-agent 与 spring-ai 是栈关系（embabel 依赖 spring-ai）**，但 lynx-agent 直接构建于 lynx-core（同仓库），所以对比 lynx-agent vs embabel-agent 时**底层 LLM 适配层的差异**已经在 Part I 中处理完毕

---

# Part I：lynx-core vs spring-ai（综述）

> 完整 10 节对比见 `doc/SPRING_AI_COMPARISON.md`（321 行，2026-05-17 重写）。本节仅做要点摘要。

## I.1 哲学定位

| 维度 | lynx | spring-ai |
|---|---|---|
| **角色** | thin library——给应用 / agent runtime 用 | application framework——给 Spring Boot 业务用 |
| **打包** | go.work 多 module，按需 import | Maven artifact 矩阵 + Spring Boot starter |
| **配置方式** | 构造函数 + Option struct | `application.yml` + `@ConfigurationProperties` + autoconfigure |
| **依赖体积** | 单 module 内可零外部依赖 | 即便最小用例也带 Spring Framework + Boot + Micrometer + Reactor |
| **加 vendor 的代价** | 新建 sibling dir，不影响 core | 新建 3 个 Maven module（vendor + autoconfigure + starter）|

## I.2 抽象同构（9 大概念两侧对得上）

ChatModel / Embedding / Image / Audio / Moderation / Document / VectorStore / RAG / MCP / Tool calling / Memory / Evaluation——**概念层面一一对应**，差异在"怎么做"和"广度/深度的取舍"。

| 抽象 | lynx 位置 | spring-ai 位置 |
|---|---|---|
| Chat Model | `core/model/chat/model.go` | `spring-ai-model:.../chat/model/ChatModel.java` |
| Chat Client | `core/model/chat/client.go`（ClientRequest 流式 builder）| `spring-ai-client-chat:.../ChatClient.java`（fluent + Advisor）|
| Message | sealed interface `chat.Message`（System/User/Assistant/Tool）| sealed class hierarchy（同名 4 类）|
| Tool | `chat.Tool` + `ToolRegistry` | `ToolCallback` + `ToolCallbackProvider` + `ToolCallingManager` |
| Embedding | `core/model/embedding/model.go` | `spring-ai-model:.../embedding/EmbeddingModel.java` |
| Memory | `core/model/chat/memory/Store` (Reader+Writer+Clearer) | `chat/memory/ChatMemory` + `ChatMemoryRepository` |
| Document | `core/document/Document` | `spring-ai-commons:.../document/Document.java` |
| VectorStore | `core/vectorstore/Store` (Creator+Retriever+Deleter) | `spring-ai-vector-store:.../VectorStore.java`（单巨型 interface）|
| RAG | `rag/` 扁平 pipeline | `spring-ai-rag/` 4 阶段（preretrieval/retrieval/postretrieval/generation）|
| MCP | `mcp/` 单 package | 12 个 Maven module |
| Evaluation | `core/evaluation/`（LLM + relevancy + fact-check + composite）| `spring-ai-commons:.../evaluation/` |
| Prompt template | Go `text/template` + media 附件 | StringTemplate + PromptTemplate / SystemPromptTemplate |
| Output parser | `chat.StructuredParser[T]` + List/Map/JSON | `StructuredOutputConverter` + `BeanOutputConverter` |

## I.3 lynx 反超的 7 条线

| # | 反超点 | 简述 |
|---|---|---|
| 1 | **Reasoning 一等公民** | `AssistantMessage.Parts` 含 `ReasoningPart{Text, Signature}` + `Usage.ReasoningTokens`；spring-ai 至今没有 |
| 2 | **chat 包零 provider 知识** | provider-specific metadata 下沉到 `models/<x>/`；spring-ai `MessageAggregator` 仍硬编码识别 Google `"isThought"` |
| 3 | **`iter.Seq2` 流式** | Go 1.23 内置迭代器；spring-ai 仍是 Reactor `Flux` ceremony |
| 4 | **ISP 拆接口** | `Store = Creator+Retriever+Deleter` / `memory.Store = Reader+Writer+Clearer`；spring-ai 单接口 |
| 5 | **Cache tokens 类型化** | `CacheReadInputTokens` / `CacheWriteInputTokens *int64` 三态语义清晰 |
| 6 | **Vector store 广度** | lynx 27 vs spring-ai 21（已绝对反超；多 8 个 spring-ai 没有的）|
| 7 | **多模态广度** | Image / TTS / STT 各 8 个 vendor；spring-ai 主干 image 仅 2 个、TTS/STT 仅 OpenAI |

## I.4 lynx 仍存在的真 gap

按 ROI 排序（已闭合项划线保留以便对照原列表）：

1. ~~**Retry `Transient` / `NonTransient` 分类**~~ —— 不做：SDK 内部已自带重试
2. ~~**Anthropic Extra 通道保护**~~ —— 已闭合（`models/anthropic/extra.go`）
3. ~~**持久化 Memory 后端**~~ —— **已闭合 + 反超**：顶层 `chatmemory/` 6 个 provider；spring-ai main 当前仅 5 个（v1.0 后 cosmosdb chat memory 模块已移除）
4. ~~**PDF / Markdown reader**~~ —— **已闭合**：顶层 `document-readers/` 出货 markdown / html / pdf 三个 reader（goldmark / goquery / ledongthuc/pdf）；Tika 因依赖 JVM 服务故意不做
5. ~~**Structured Output Converter**~~ —— 已闭合：`core/model/chat/parser.go`（JSONParser[T] / ListParser / MapParser / StructuredParser[T] / AnyParser）
6. ~~**SafeGuard / Logger middleware**~~ —— **已闭合**：`core/model/chat/middleware/` 新包，依赖倒置形态——`Logger` / `Matcher` 接口对外暴露，slog / `strings.Contains` 作为 stdlib 默认实现
7. ~~**DocumentJoiner / QueryRouter**~~ —— **不做**：spring-ai 的 `ConcatenationDocumentJoiner` 等价于 lynx 的 `DeduplicationRefiner` → `RankRefiner` refiner 链，无需额外抽象；RRF 要等到 lynx 加入非向量 retriever（BM25 / sparse）时再做，那时设计才有真实约束；QueryRouter 由用户在自定义 `DocumentRetriever` 内部决定即可，pipeline 不需要专门接口。详见 `rag/doc.go` 多 retriever 章节

**vendor 生命周期视角**：spring-ai 自 v1.0.0 以来主仓库砍掉了一批模块（`spring-ai-azure-openai` / `spring-ai-vertex-ai` chat / `spring-ai-zhipuai` / `spring-ai-moonshot` / `spring-ai-qianfan` / `spring-ai-azure-cosmos-db-store` / `spring-ai-hanadb-store` / `spring-ai-infinispan-store` / cosmosdb chat memory）。lynx 这边对应的 `models/azureopenai` / `models/vertexai` / `models/zhipu` / `models/moonshot` / `chatmemory/cosmosdb` 都还在——**lynx 每个 vendor 是独立 go module，没有 spring-ai 的"autoconfigure + starter + BOM 三件套"维护税**，所以没有定期清理压力。扩张/收缩的成本曲线完全不同（详见 `SPRING_AI_COMPARISON.md` §1.1）。

> **完整 gap 路线图 + ROI 评级见 `SPRING_AI_COMPARISON.md` §9**。

## I.5 一句话定档（Part I）

**lynx 走 thin-library 路线扩 vendor 的边际成本远低于 spring-ai 的 framework 路线**——24 vector stores / 39 model providers / 6 chat memory provider / 3 document reader 已经验证；spring-ai 同期反而做了一轮主仓库收缩（azure-openai / vertex-ai chat / zhipuai / moonshot / cosmosdb / hanadb / infinispan 全部移出），是 framework 路线必须缴的"模块矩阵维护税"。**P0 + P1 + P2 三轮 gap 已全部闭合**——retry 不做、Anthropic Extra 已闭合、OTel 全量、持久化 Memory 反超、document readers、Structured Output、BaseVisitor、SafeGuard + Logger middleware 全部到位。剩下的是真正的长尾创新（多 agent 编排 / Bedrock Converse / Anthropic Skills 等）。

---

# Part II：lynx/agent vs embabel-agent（深度对比，全新）

## II.0 TL;DR

**两侧使用高度相似的 GOAP（Goal-Oriented Action Planning）+ Blackboard 思想体系**。核心抽象（Action / Goal / Condition / 三值逻辑 / WorldState / Effect / Plan）**在概念层一一对应**，**这绝非偶然——embabel 的设计明显借鉴或共享了同一理论基础**（GOAP 起源于 FEAR 游戏 AI，2005）。

**两侧的根本分歧**不在 *做什么*，而在 *谁主导*：

| 分歧轴 | lynx-agent | embabel-agent |
|---|---|---|
| **宿主生态** | Go 标准 + lynx-core | JVM（Kotlin/Java）+ Spring Boot + Spring AI |
| **运行时容器** | 无 DI；显式 `runtime.Platform.Deploy(agent)` | Spring `ApplicationContext`；`@Agent` 类路径扫描 |
| **作者风格** | 单一 Builder DSL：`agent.New(...).Actions(...).Goals(...).Build()` | **双轨**：`@Agent`/`@Action` 注解式 **或** Kotlin `agent { flow {} transformation<I,O>{} }` DSL |
| **类型系统** | Go 泛型：`TypedAction[In, Out]` / `IOBinding[T]` 编译期类型安全 | Kotlin 反射 + `DomainType` 运行时类型系统（支持动态类型） |
| **Planner 多样性** | **3 种**：GOAP（A*）、HTN、Reactive | **仅 GOAP（A*）** |
| **LLM 调用入口** | `chat.Client`（lynx 自有） | `PromptRunner`（包装 Spring AI `ChatModel`）|
| **Extension 模型** | 显式接口：`ActionInterceptor` / `ToolDecorator` / `AgentValidator` / `GoalApprover` | `AgenticEventListener` 事件总线 + Spring AOP |
| **HITL 形态** | 类型化 `Awaitable[T]` + `Confirmation` | `WAITING` 状态 + `AwaitableResponseException` 抛出 |
| **多 agent 协作** | `Platform` 内部组合 + MCP server 导出 | `AgentPlatform`（聚合 scope）+ A2A REST gateway + MCP server |
| **持久化** | 无（内存 Process）| `AgentProcessRepository` 接口（可持久化）|
| **可观测性** | OpenTelemetry 原生 + slog bridge | Spring Boot Actuator + AgenticEventListener + Micrometer |

**lynx-agent 反超的方面：**
1. **多 Planner 范式**（GOAP / HTN / Reactive 三选一；embabel 锁定 GOAP A*）
2. **类型安全工作流原语**（`TypedAction[In, Out]` 编译期检查；embabel 靠 Kotlin reflection + runtime DomainType）
3. **Workflow patterns 独立成 package**（`agent/workflow/`：sequence / parallel / scatter-gather / repeat-until / consensus；embabel 散落在 DSL 函数中）
4. **HITL 类型化 `Awaitable[T]`**（编译期保证 resume 类型一致；embabel 走 exception + dynamic 类型）
5. **Extension 接口分层显式**（interceptor / decorator / validator / approver 四种角色明确；embabel 主要靠 event listener + Spring AOP）

**embabel 反超的方面：**
1. **注解式 DSL**（`@Agent`/`@Action`/`@Cost` 三件套；lynx 因 Go 无 runtime annotation 走不通这条路）
2. **持久化 process 仓库**（`AgentProcessRepository` SPI；lynx 无对应抽象）
3. **A2A REST gateway**（独立 module；lynx 无对等模块）
4. **`ToolGroup` 角色 + 权限模型**（`HOST_ACCESS` / `INTERNET_ACCESS` 二值标签；lynx 走自由组合）
5. **Spring 生态整合**（actuator / metrics / shell / scheduling 开箱即用；lynx 全靠 OTel 自配）
6. **`@Cost` 动态代价计算**（运行时反射调用方法；lynx 走 `CostFunc func(WorldState) → float64` 函数式）
7. **Conversation / ChatSession 抽象**（embabel 把对话作为一等公民；lynx 无对应抽象，要靠 chat memory + 应用层组合）

---

## II.1 哲学定位

| 维度 | lynx-agent | embabel-agent |
|---|---|---|
| **目标用户** | 库用户、需要嵌入 agent 能力到 Go 服务的开发者 | Spring Boot 应用、需要快速搭建 agentic 系统的企业开发者 |
| **运行时形态** | in-process library | Spring Boot application context |
| **LLM 后端绑定** | 一切走 `chat.Model`，lynx-core 内置 39 个 provider | 一切走 Spring AI `ChatModel`，依赖 spring-ai 的 ~15 个 provider |
| **作者门槛** | 必须懂 Go 泛型 + Builder pattern | 注解式入门 5 分钟；DSL 进阶需要 Kotlin 知识 |
| **企业整合** | 自带 OTel + MCP，其它整合自己写 | actuator / shell / metrics / a2a REST / mcp / scheduling 模块化出货 |

**lynx-agent 是 *agent-as-library*，embabel-agent 是 *agent-as-platform***。这条立场差异定义了两侧绝大多数的具体设计取舍。

---

## II.2 核心抽象同构 — Action / Goal / Condition

### II.2.1 Action

**两侧 Action 都是 GOAP 流派的标准定义：name + preconditions + effects + 可执行体**。

| 属性 | lynx (`agent/core/action.go`) | embabel (`embabel-agent-api/.../core/Action.kt`) |
|---|---|---|
| 元数据 | `ActionMetadata { Name, Description, Inputs/Outputs (IOBinding[]), Preconditions/Effects (EffectSpec), CanRerun, QoS, Cost/Value (CostFunc) }` | `ActionMetadata { name, description, inputs/outputs (Set<IoBinding>), preconditions/effects (EffectSpec), canRerun, readOnly, qos: ActionQos, cost/value }` |
| 执行 | `Execute(ctx, pc ProcessContext) ActionStatus` | `execute(processContext): ActionStatus` |
| 重试策略 | `ActionQoS { MaxAttempts: 5, BaseDelay: 10s, MaxDelay: 60s }` | `ActionQos`（同名同语义） |
| 状态返回 | `ActionStatus`: Success / Failed / Waiting / Paused / Cancelled | `ActionStatusCode`: SUCCESS / FAILED / WAITING / PAUSED / CANCELLED |
| 类型化输入 | **`TypedAction[In, Out]`** 泛型包装 func | 注解 + 反射：`@Action` 方法签名直接成为类型契约 |

**最大差异——类型契约的表达方式：**

lynx 走 **泛型类型安全**：
```go
type TypedAction[In, Out any] struct {
    metadata ActionMetadata
    fn       func(ctx context.Context, pc ProcessContext, in In) (Out, ActionStatus)
}
```
编译期就知道 In/Out 类型，IOBinding 由 reflect 自动生成。

embabel 走 **注解 + 反射**：
```kotlin
@Action(pre = ["hasInput"], post = ["analysisDone"])
fun analyzeContent(input: UserInput, blackboard: Blackboard): AnalysisResult { ... }
```
`AgentMetadataReader` 在启动时扫描方法签名提取 `IoBinding`，运行时反射调用。

**取舍**：lynx 编译期类型安全胜出，但 `TypedAction[In, Out]` 写起来 verbose（需要手填 metadata）；embabel 简洁但运行时类型错误（参数不匹配、blackboard 缺值等）在启动时才暴露。

### II.2.2 Goal

| 属性 | lynx (`agent/core/goal.go`) | embabel (`embabel-agent-api/.../core/Goal.kt`) |
|---|---|---|
| 字段 | `Name, Description, Pre[], Inputs[], Value (CostFunc), Tags[], Examples[], Export (GoalExport)` | `name, description, pre: Set<String>, inputs: Set<IoBinding>, outputType: DomainType?, value: CostComputation, tags, examples, export: Export` |
| Pre 表达 | precondition 条件名列表 | 同样是 condition 名 set |
| Value（效用函数） | `CostFunc func(WorldState) float64` | `CostComputation = (WorldState) -> Double` |
| 导出（→ 外部世界）| `GoalExport { Remote, Description, InputSample }`——决定是否暴露为 MCP tool | `Export { remote, description, inputSample }`——同语义 |
| 类型化 Goal | `GoalProducing[T]`——precondition 是"T exists on blackboard" | `Goal.createInstance(description, type=...)`——satisfiedBy 是 `KClass<*>` |

**两侧 Goal 在数据形态上**几乎完全同构。差异仅在 `Value` 函数式 vs 方法引用：lynx 是函数指针，embabel 可以走 `@Cost` 注解的方法引用 + 反射。

### II.2.3 Condition + 三值逻辑

**最惊人的相似点**：两侧都使用 **`Unknown` / `True` / `False` 三值逻辑**，并且都把它建模为 enum。

```go
// lynx: agent/core/determination.go
type Determination int
const (
    Unknown Determination = iota  // 0
    True                          // 1
    False                         // 2
)
```

```kotlin
// embabel: plan/common/condition/ConditionDetermination.kt
enum class ConditionDetermination {
    TRUE, FALSE, UNKNOWN
}
```

两侧都为三值逻辑提供 **AND / OR / NOT 组合子**，并且都在 `Condition.Evaluate` / `evaluate(OperationContext)` 中返回这个三值。

**Condition 的实现形态对比**：

| 形态 | lynx | embabel |
|---|---|---|
| 函数式 condition | `ComputedCondition { name, cost, fn }` | `ComputedBooleanCondition` |
| LLM-as-judge condition | **`PromptCondition`**（含 PromptBuilder + ConditionParser）| 无内置（需自己用 PromptRunner + 函数式 condition 组装） |
| 评估代价标注 | `Cost() float64`（0 = 便宜，1 = 昂贵）→ 规划器据此优化求值顺序 | `cost: ZeroToOne`（同语义） |

**lynx 独有：`PromptCondition`**——直接把"LLM 判断真假"做成一等 condition 类型，自动注入 chat.Model + parser。embabel 需要在 action 内部用 `PromptRunner.createObject<Boolean>(...)` 手工拼。

### II.2.4 WorldState + EffectSpec

| | lynx | embabel |
|---|---|---|
| WorldState | `State() map[ConditionName]Determination` + `Timestamp()` + `HashKey()` + `Apply(EffectSpec) → newState` | `WorldState` 接口（含 state map）|
| EffectSpec | `map[CondKey]Determination` | `EffectSpec = Map<String, ConditionDetermination>` |

两侧的"应用 effect → 新 state"算法完全一致（key 覆盖式合并）。**这一层抽象是两个框架最对齐的部分**。

---

## II.3 Planner 子系统 — A* vs 多范式

### II.3.1 Planner 接口

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

embabel `Planner` 是泛型化的（state / system / plan 三参数），lynx 是单态接口（只有 Plan 输出，state/system 走参数）。

### II.3.2 实现矩阵

| Planner 实现 | lynx | embabel |
|---|---|---|
| **GOAP A*** | `agent/plan/planner/goap/`（A* + 启发式 = 未满足 goal cond 数 + 状态去重）| `plan/goap/astar/AStarGoapPlanner`（同算法，含 backward + forward planning 优化）|
| **HTN** | `agent/plan/planner/htn/` | **无** |
| **Reactive**（无搜索，每 tick 选最高价值可行 action） | `agent/plan/planner/reactive/` | **无** |
| **Utility-based**（按效用排序）| 由 reactive + value func 组合实现 | 由 `bestValuePlanToAnyGoal` 体现 |

**lynx 的多 planner 设计**有两个好处：
1. 不同 agent 不同 planner（短链条 reactive、长链条 GOAP A*、分层任务 HTN）
2. `Planner` 是 `Extension`，可以热替换 / 拼装

**embabel 的单 planner 设计**则把 A* 优化做到极致（早期可达性检测、backward planning、forward planning 简化）。**深度 > 广度**。

### II.3.3 Plan

| | lynx (`agent/plan/plan.go`) | embabel (`plan/Plan.kt`) |
|---|---|---|
| 字段 | `Actions[], Goal` | `actions: List<Action>, goal: Goal` |
| 完成判定 | `IsComplete()`（动作执行完）| `isComplete()` |
| 代价 | `Cost(ws)` / `Value(ws)` / `NetValue(ws)` | `cost(state)` / `netValue(state)` |

**完全同构**。两侧 Plan 都是 *值对象*——不可变、可序列化、可比较。

---

## II.4 Blackboard 子系统

### II.4.1 接口形态

| | lynx (`agent/core/blackboard.go`) | embabel (`core/Blackboard.kt`) |
|---|---|---|
| 接口分层 | **ISP**：Reader + Writer + Extension 三接口拼成 Blackboard | 单接口 `Blackboard : Bindable, MayHaveLastResult, HasInfoString` |
| 按 key 存取 | `Set(key, value)` / `Get(key) → any` | `set(key, value)` / `get(name): Any?` |
| 按类型存取 | `Get[T](var) → T`（泛型）| `last<T>(Class<T>): T?` / `objectsOfType<T>(Class<T>): List<T>` |
| 默认对象 | `AddObject(value)`（追加无名对象列表）| `addObject(value)` / `+= value` |
| Condition 直接存取 | `SetCondition(key, val)` / `GetCondition(key)` | `setCondition(key, val): Blackboard` / `getCondition(key): Boolean?` |
| 派生 / 子 board | `Spawn() → Blackboard` | `spawn(): Blackboard` |
| 受保护绑定（state 切换后存活）| 无对应概念 | **`bindProtected(key, value)`**——独有 |

### II.4.2 类型解析的差异

**两侧都支持"按类型从 blackboard 取最近一个实例"**，但实现路径不同：

- **lynx**：Go 泛型 + reflect。`blackboard.GetTyped[*Result](bb)` 直接拿到强类型值；底层走 `reflect.TypeOf(target)` 匹配 + cast。
- **embabel**：三级匹配层级——
  1. JVM class hierarchy（simple-name 或 FQN）
  2. DomainType hierarchy（注解或 YAML 定义的领域类型）
  3. Tagged Map（TYPE_NAME_KEY / TYPE_LABELS_KEY for pack-authored types）

embabel 的 DomainType 系统支持 *运行时定义的动态类型*（来自 YAML / pack 文件），这是 lynx 没有的能力——但代价是类型解析复杂度高。

### II.4.3 `bindProtected` 的设计意图

embabel 的 `bindProtected` 是为了 *跨 state-machine reset 持久化*：当 action 用 `clearBlackboard=true` 清空 blackboard 时，protected binding 不会被清掉。典型用例：对话历史（conversation）需要跨多个工作流阶段存活。

**lynx 没有 `bindProtected`**——因为 lynx 的 action 设计倾向"effect 只增不减"，没有"清空 blackboard"的内建机制。哪种更好取决于使用场景：embabel 偏 state-machine（显式 reset），lynx 偏 incremental（一路追加）。

---

## II.5 IO Binding（类型化数据流契约）

### II.5.1 形态对比

```go
// lynx: agent/core/io_binding.go
type IOBinding[T any] struct {
    Variable string  // "it" / "userQuery" / ...
    TypeName string  // 反射得到
    Schema   *json.Schema  // 可选，给 LLM 看
}
```

```kotlin
// embabel: core/IoBinding.kt
@JvmInline
value class IoBinding(val value: String) {
    val type: String  // 从 "name:type" 字符串解析
    val name: String  // 同上
}
```

**根本差异**：
- **lynx**：泛型类型 + 反射，类型信息**在编译期保留**
- **embabel**：字符串 `"name:type"` 编码，类型信息**仅在运行时存在**

embabel 选择字符串编码的原因——为了支持 YAML / pack-defined agent（注解扫描 + 动态类型 + 字符串配置）。lynx 走纯 Go 编译路线，没有这个需求。

### II.5.2 默认绑定名

| | lynx | embabel |
|---|---|---|
| 默认 variable | `"it"` | `"it"`（`IoBinding.DEFAULT_BINDING`）|
| 最近结果别名 | 无对应（用 `BlackboardReader.LastResult()` 取） | **`"lastResult"`**（`IoBinding.LAST_RESULT_BINDING`）|

embabel 的 `"lastResult"` 特殊变量让动作可以直接 reference 上一动作的输出——这是 lynx 没有显式建模的便利。

---

## II.6 AgentProcess 生命周期

### II.6.1 状态机

| 状态 | lynx | embabel |
|---|---|---|
| 初始 | （无显式 NOT_STARTED，由 Platform 创建后立即转 Running） | `NOT_STARTED` |
| 运行中 | `Running` | `RUNNING` |
| 完成 | `Complete` | `COMPLETED` |
| 失败 | `Failed` | `FAILED` |
| 阻塞（无可行 action）| `Stuck` | `STUCK` |
| 等待外部输入 | `Paused`（HITL）| `WAITING` |
| 用户终止 | （由 Platform.Kill 触发）| `KILLED` |
| 策略终止 | 由 budget / termination signal 触发 | `TERMINATED` |
| 调度暂停 | 同 Paused | `PAUSED` |

embabel 状态划分更细（KILLED / TERMINATED / PAUSED / WAITING 四种"暂停态"区分语义）。lynx 用 signal + status 组合，达到类似效果但状态码更少。

### II.6.2 历史 / 追踪

两侧都有 **action invocation history**：
```go
// lynx
type ActionInvocation struct {
    ActionName string
    Timestamp  time.Time
    Duration   time.Duration
    Status     ActionStatus
    Attempts   int
}
```
```kotlin
// embabel
data class ActionInvocation(
    val actionName: String,
    override val timestamp: Instant,
    override val runningTime: Duration
)
```

embabel 的 `AgentProcess` 还额外实现了 `LlmInvocationHistory` + `EmbeddingInvocationHistory` 接口——把 LLM 调用记录作为一等公民。lynx 的对应能力散落在 OTel span 里，没有 first-class 接口。

### II.6.3 Budget / Cost 追踪

| | lynx (`runtime/process_budget.go`) | embabel (`ProcessOptions.Budget`) |
|---|---|---|
| 字段 | Cost (USD) / Tokens / Actions | cost ($) / actions / tokens |
| 强制执行 | `ProcessBudget` 检查违例时发送 termination signal | `EarlyTerminationPolicy` 在 budget 超限时切到 TERMINATED |
| Cost 报告 | 走 OTel metric | `totalCostInfoString(verbose)` 人类可读报告 |

embabel 多一个独有能力：**aggregate cost across child processes**——`totalCost()` = own + embeddings + 所有子 process 累加。lynx 的 ProcessBudget 仅追踪当前 process（子 process 各自独立）。

---

## II.7 ProcessContext / Operation context

两侧都为 action 执行体提供一个 *context object*，但承载内容不同：

```go
// lynx: agent/runtime/process_context.go
type ProcessContext struct {
    Process       Process           // 只读 process 视图
    Blackboard    Blackboard        // 读 + 写
    ActionTools   ToolResolver      // 当前可见的工具集
    Termination   <-chan struct{}   // 终止信号
}
```

```kotlin
// embabel: OperationContext
interface OperationContext {
    val agentProcess: AgentProcess
    val blackboard: Blackboard
    val verbosity: Verbosity
    val input: Any?  // 类型化输入（由 IoBinding 解析）
    fun promptRunner(...): PromptRunner
}
```

**最大差异**：embabel 的 `OperationContext` 携带 **`input`** 字段（已经按 IoBinding 解析好的当前 action 输入）和 **`promptRunner()` 工厂方法**（直接拿到一个配置好的 LLM 入口）。

lynx 的 ProcessContext 更接近 *infrastructure context*——只暴露基础设施，不预解析输入；action 自己从 blackboard 取。

**取舍**：
- embabel 风格便利（少几行 boilerplate）
- lynx 风格灵活（action 可以从 blackboard 取任意类型/数量的输入，不被 input 字段约束）

---

## II.8 LLM 操作入口

### II.8.1 lynx：`chat.Client` 直接复用

lynx-agent **不引入新的 LLM 抽象**——action 内部直接用 `chat.Client`（lynx-core 的标准 LLM 入口）：

```go
func myAction(ctx context.Context, pc ProcessContext, in MyInput) (MyOutput, ActionStatus) {
    resp, err := pc.ChatClient().
        WithMessages(...).
        WithTools(...).
        Call(ctx)
    // ...
}
```

### II.8.2 embabel：`PromptRunner` 专属抽象

embabel **专门设计了 `PromptRunner`** 作为 agent 内 LLM 调用入口（包装 Spring AI ChatClient）：

```kotlin
operationContext.promptRunner(
    llm = LlmOptions.auto(),
    toolGroups = setOf(ToolGroupRequirement(CoreToolGroups.WEB))
).createObject<FactualAssertions>("""
    Given the following content, identify factual assertions:
    ${context.input.content}
""")
```

**`PromptRunner` 独有功能**：
1. 自动注入当前 agent 上下文（process / blackboard / tool resolver）
2. 流式版本 `streaming()` 切换
3. 结构化输出 `createObject<T>` / `createObjectIfPossible<T>`（自动 JSON schema + 解析 + 重试）
4. 与 `ToolGroup` 角色系统集成（声明 `toolGroups = setOf(...)` 自动注入对应 tool）

**lynx 缺什么**：上述 1–4 在 lynx 都需要 *用户手工拼*。这是 lynx-agent **最显著的 ergonomics 短板**——P0 级别。

> **路线图建议**：lynx-agent 应该把这套 ergonomics 补上（可以叫 `agent.PromptRunner` 或 `agent.LLM`），把工作流里 `pc.ChatClient().With...` 这一长串收敛成 2–3 行高表达力 API。

---

## II.9 Tool / ToolGroup

### II.9.1 概念对齐

| | lynx | embabel |
|---|---|---|
| 单个工具 | `chat.Tool`（单一接口）| `Tool`（Spring AI ToolDefinition + ToolCallback）|
| 工具集合 | `agent.ToolGroupRequirement { role: string }` | `ToolGroup` interface + `ToolGroupRequirement { role, requiredToolNames }` |
| Tool 注册 | `chat.ToolRegistry` + 显式 `Register(tool)` | Spring `@Tool` 注解扫描 + `ToolCallbackProvider` |
| 权限 / 安全 | 无显式模型（靠 tool 自己实现）| **`ToolGroupPermission`**：HOST_ACCESS / INTERNET_ACCESS |
| Tool 装饰 | **`ToolDecorator` 扩展接口**——动作维度装饰每个 tool（rate-limit / once-only） | 走 Spring AOP + 事件 |
| Tool 策略 | **`agent/toolpolicy/`** package（once-only / unlock-on-event / rate-limit）| 无对等独立 package |

### II.9.2 ToolGroup 权限模型

embabel 把 *安全分类* 编码进 ToolGroup：

```kotlin
enum class ToolGroupPermission {
    HOST_ACCESS,      // 修改本机资源（文件、shell）
    INTERNET_ACCESS   // 访问互联网
}
```

这让 *平台级安全策略* 成为可能——比如"只允许某些 agent 使用 HOST_ACCESS 工具"。lynx **无对等抽象**——目前所有安全策略需要在 ToolDecorator 里手工实现。

> **路线图建议**：lynx 可以借鉴 ToolGroupPermission 引入 `ToolCapability` 标签（HostAccess / InternetAccess / DataMutation 等），让 platform 层做统一权限检查。低工作量、高架构价值。

### II.9.3 ToolPolicy（lynx 独有）

`agent/toolpolicy/` 是 lynx 独有的 **decorator pattern**：
- `OnceOnly`——同一 process 内某 tool 只能调用一次
- `Unlock`——某 tool 被锁定，直到某 event 触发后解锁
- `RateLimit`——令牌桶速率限制

embabel 没有这种"动作维度 / process 维度的 tool 行为修饰"的成型抽象。

---

## II.10 Extension / 扩展机制

### II.10.1 lynx 的 4 种 Extension 角色

```go
// agent/core/extension.go
type Extension interface { Name() string }

type ActionInterceptor interface {
    Extension
    InterceptAction(ctx, process, action, next) ActionStatus  // around-call
}
type ToolDecorator interface {
    Extension
    DecorateTool(process, action, tool) AgentTool             // wrap
}
type AgentValidator interface {
    Extension
    ValidateAgent(agent) error                                // post-deploy
}
type GoalApprover interface {
    Extension
    ApproveGoal(process, goal) bool                           // gate
}
```

**特点**：四种角色 *显式签名*，编译期 *role 类型清晰*；Platform 维护四个独立 registry。

### II.10.2 embabel 的 Event Listener 总线

```kotlin
interface AgenticEventListener {
    fun onAgentProcessEvent(event: AgentProcessEvent)
    fun onLlmInvocation(event: LlmInvocationEvent)
    // ...
}
```

**特点**：所有"嵌入式 hook"统一走 event。优势是 *uniform API*，劣势是 *弱类型 + 后置语义*——你不能在事件里拒绝一个 action 或修改它的输入。

**取舍对比**：

| 能力 | lynx | embabel |
|---|---|---|
| 拦截并改写 action 调用 | ActionInterceptor（around）| 不支持（事件是 after-the-fact）|
| 包装 tool 行为 | ToolDecorator | 走 Spring AOP（重） |
| 启动期 agent 校验 | AgentValidator | Spring `ApplicationListener<ContextRefreshedEvent>` |
| 拒绝某个 goal | GoalApprover | 事件无法拒绝（需写 custom Planner / Action） |
| 被动观察 | （走 OTel + event）| AgenticEventListener |

**lynx 的 Extension 设计在 *主动控制权* 上更强**，embabel 在 *被动观察 + 整合* 上更便利。

---

## II.11 Workflow 模式

### II.11.1 lynx：`agent/workflow/` 独立 package

lynx 把常见模式提炼为独立 package：

| 模式 | 文件 | 用途 |
|---|---|---|
| Sequence | `workflow/sequence.go` | 顺序执行多个 action |
| Parallel | `workflow/parallel.go` | 并行执行后合并 |
| Scatter-gather | `workflow/scatter_gather.go` | 1→N 扇出 + 聚合 |
| Repeat-until | `workflow/repeat_until.go` | 循环直到某 condition 满足 |
| Repeat-until-acceptable | `workflow/repeat_until_acceptable.go` | 带质量门的循环（LLM 自评估）|
| Consensus | `workflow/consensus.go` | 多路并行 + 投票合并 |
| Loop | `workflow/loop.go` | 通用循环原语 |

**实现路径**：每个 workflow 都*编译*成一组 GOAP-compatible 的 Action + Condition + Effect，由标准 planner 调度。**没有引入新的 runtime 概念**——这一点设计很优雅。

### II.11.2 embabel：Kotlin DSL 内的 flow / aggregate / transformation

embabel 把模式做成 **DSL 函数**：

```kotlin
agent("FactChecker", ...) {
    flow {
        aggregate<UserInput, FactualAssertions, RationalizedFactualAssertions>(
            transforms = llms.map { llm -> { context -> ... } },
            merge = { list, context -> ... }
        )
    }
    transformation<RationalizedFactualAssertions, FactCheck> { context ->
        // ...
    }
    goal(name = "factCheckingDone", satisfiedBy = FactCheck::class)
}
```

`aggregate` / `transformation` / `flow` 都是 DSL 中的 *higher-order function*——内部生成 Action + Condition 注入 agent。

**对比**：

| 维度 | lynx | embabel |
|---|---|---|
| 模式可复用性 | 每个 workflow 是 *public type*，可独立 import / 单测 | DSL 函数，agent 内部使用 |
| 类型安全 | 编译期泛型保证（`workflow.ScatterGather[In, Mid, Out]`） | DSL 内泛型 + reify |
| 模式数量 | 7 类成型 | 3–4 类（aggregate / parallel / transformation / flow）|
| 学习曲线 | 需要理解每个模式的 type signature | DSL block 看起来更"声明式" |

**这一项 lynx 反超**——workflow 作为独立可复用类型 + 编译期类型安全 + 单测友好。

---

## II.12 HITL（Human-in-the-Loop）

### II.12.1 lynx：类型化 `Awaitable[T]` + `Confirmation`

```go
// agent/hitl/
type Awaitable[T any] interface {
    Wait(ctx context.Context) (T, error)
    Resume(value T) error
}

type Confirmation struct { Prompt string; ApprovedBy string }
```

agent 在 action 内部 `Wait()` 时进入 Paused，外部通过 `Resume(value)` 注入用户输入。**T 类型由调用方在编译期确定**——类型不匹配编译失败。

### II.12.2 embabel：`WAITING` 状态 + `AwaitableResponseException`

```kotlin
// 在 action 里抛出
throw AwaitableResponseException(prompt, expectedType)
// process 转入 WAITING 状态，等待外部 resume
```

类型信息走 `expectedType: Class<*>`——**运行时类型**。

**对比**：
- lynx 的 `Awaitable[T]` 编译期类型安全
- embabel 的 exception-based 路线对调用方代码更简洁（不需要先包成 Awaitable）

---

## II.13 多 Agent 协作

### II.13.1 lynx 的多 agent 形态

| 路径 | 实现 |
|---|---|
| Agent 作为 Tool | `runtime/agent_tool.go` — `AgentTool` 包装一个完整 agent process 为 `chat.Tool` |
| Goal 导出 | `GoalExport.Remote = true` → 自动作为 MCP tool 暴露 |
| Inner platform 嵌套 | `Platform` 可以在 action 内部启动 child process |
| MCP server | `mcp/server.go` — 把 agent goals 暴露为 MCP tools |

**总结**：lynx 是 *MCP-first* 的多 agent 路线——agent 通过 MCP 协议与外部世界（包括其它 agent）交互。

### II.13.2 embabel 的多 agent 形态

| 路径 | 实现 |
|---|---|
| AgentScope 聚合 | `AgentPlatform` 实现 `AgentScope`，自动聚合所有 deployed agent 的 actions/goals/conditions 到 *统一规划域* |
| Reference agent action | DSL `referencedAgentAction<I, O>(agentName)` |
| Local agent action | DSL `localAgentAction<I, O>(agent)` |
| A2A REST gateway | `embabel-agent-a2a` 独立 module，把 agent 暴露为 REST endpoint |
| MCP server | `embabel-agent-mcpserver` 独立 module |

**总结**：embabel 是 *platform-first* 多 agent 路线——平台层把所有 agent 当作 *一个大规划域* 看待，规划器可以跨 agent 选 action。

**最大差异**：embabel 的 `AgentPlatform : AgentScope` 设计让 **跨 agent planning 自动成为可能**——planner 在 platform 视图里看到所有 deployed agent 的 actions，可以编排出"agent A 的 step 1 → agent B 的 step 2 → agent A 的 step 3"这样的混合 plan。lynx 无对等机制——跨 agent 编排需要应用层手动 orchestrate（或通过 MCP 隔离调用）。

**取舍**：
- embabel 的全局规划威力大，但**所有 agent 共享 condition namespace**——大规模部署时容易冲突
- lynx 的隔离规划稳定性好，但**跨 agent 需要应用层胶水代码**

---

## II.14 作者风格：注解 vs DSL vs Builder

### II.14.1 三种 authoring 风格

| 风格 | lynx | embabel |
|---|---|---|
| **注解** | 不支持（Go 无 runtime annotation）| `@Agent` + `@Action` + `@Condition` + `@Cost` |
| **Kotlin DSL** | N/A | `agent { flow {} transformation<I,O>{} }` |
| **Builder** | `agent.New("name").Description(...).Actions(...).Goals(...).Build()` | `AgentBuilder`（DSL 的底层）|

embabel 的 **双轨 authoring**（注解 + DSL）是其最大的入门优势——5 分钟可以写出第一个 `@Action` 方法。lynx 因 Go 的语言限制走 Builder 路线，必须显式 wire 所有 metadata（IOBinding / preconditions / effects 都要手填），认知门槛更高。

### II.14.2 IOBinding 推导差异

embabel 注解版本 **自动从方法签名推导 IOBinding**：
```kotlin
@Action(pre = ["hasInput"], post = ["analysisDone"])
fun analyze(input: UserInput, blackboard: Blackboard): AnalysisResult { ... }
// 自动推导: inputs = [UserInput], outputs = [AnalysisResult]
```

lynx Builder 需要 **显式声明**：
```go
agent.NewAction("analyze").
    WithInput(NewIOBinding[UserInput]("input")).
    WithOutput(NewIOBinding[AnalysisResult]("it")).
    WithPreconditions("hasInput").
    WithEffects("analysisDone").
    WithFunc(func(ctx, pc, in UserInput) (AnalysisResult, ActionStatus) { ... })
```

**ergonomics gap 显著**——lynx 这部分 *至少 50% 的 boilerplate*。可考虑用代码生成（`go:generate` + AST）补救，但目前未做。

---

## II.15 Conversation / ChatSession（embabel 独有）

embabel 把 *对话* 作为一等公民：

```kotlin
interface ChatSession {
    val outputChannel: OutputChannel
    val user: User?
    val conversation: Conversation
    val processId: String?
    
    fun onUserMessage(userMessage: UserMessage)
    fun saveAndSend(message: AssistantMessage)
}
```

**用例**：聊天机器人 / Discord bot / Slack bot——session 与 agent process 解耦，session 可以驱动多次 agent 调用。

**lynx 无对等抽象**——chat memory（`core/model/chat/memory/`）是 LLM 层抽象，没有 agent 层的"会话"概念。如果做聊天机器人，需要应用层组合 chat memory + agent process。

> **路线图建议**：lynx-agent 可以引入 `Session` 抽象，把"对话生命周期"作为 agent 的可选维度。

---

## II.16 持久化

| 能力 | lynx | embabel |
|---|---|---|
| Process 持久化 | **无**（in-memory only）| `AgentProcessRepository` interface + 多种 backend |
| Blackboard 持久化 | 无 | 同上 |
| Memory（chat memory）持久化 | ✅ 顶层 `chatmemory/` module 提供 postgres / redis / mongodb / cassandra / neo4j / cosmosdb 共 6 个 provider | `ChatMemoryRepository` 多 backend |
| Agent 定义持久化 | N/A（agent 是 compile-time）| 同上（agent 是 runtime 注册的 bean）|

**Process 持久化仍是 embabel 反超项**——任何要做"agent process 长期运行 + 中断恢复 + 跨节点迁移"的场景，lynx 当前都需要应用层从头实现。chatmemory 已闭合后，剩下 Process / Blackboard 持久化可以复用同一批 driver（每个 ~150 LOC）。

> **路线图建议**：Process Store 沿用 chatmemory 已经验证的 6 backend 形态。

---

## II.17 Observability

| | lynx | embabel |
|---|---|---|
| 标准 | **OpenTelemetry 原生**（trace + metric + log）| **Spring Boot Actuator** + Micrometer + `AgenticEventListener` |
| 追踪粒度 | OTel span 自动覆盖（action / planner / LLM call）| event-driven：LlmInvocationEvent / AgentProcessEvent / ActionInvocationEvent |
| 日志 bridge | `otel/slog`——slog 自动注入 trace context | Spring `Logger` + LogBack |
| Cost 报告 | 走 OTel metric，自己 aggregate | `totalCostInfoString(verbose)` 一键报告 |
| Shell 集成 | 无 | `embabel-agent-shell` CLI module（运行 / 检查 / 调试 agent）|

**这是 *philosophy difference 的典型体现***：
- lynx 拥抱 *Go 生态标准*（OTel + slog）
- embabel 拥抱 *Spring 生态约定*（Actuator + Boot 自动配置）

**取舍**：lynx 路线 *面向云原生 / Kubernetes 生态* 更顺手；embabel 路线 *面向企业 Spring 应用栈* 更顺手。

---

## II.18 完整能力矩阵（embabel vs lynx）

| 能力 | lynx-agent | embabel-agent |
|---|---|---|
| GOAP / A* planner | ✅ | ✅ |
| HTN planner | ✅ | ❌ |
| Reactive planner | ✅ | ❌ |
| Three-valued logic (T/F/U) | ✅ | ✅ |
| WorldState + EffectSpec | ✅ | ✅ |
| Action.preconditions/effects | ✅ | ✅ |
| Goal.value（效用函数） | ✅ | ✅ |
| Goal.export（→ MCP） | ✅ | ✅ |
| Blackboard | ✅ | ✅ |
| Blackboard.bindProtected | ❌ | ✅ |
| Blackboard.byType 取值 | ✅（泛型） | ✅（反射） |
| Typed IO binding | ✅（编译期） | ❌（运行时字符串） |
| TypedAction[In, Out] | ✅ | 注解 + 反射 |
| LLM-as-Condition | ✅ (`PromptCondition`) | ❌（自己拼） |
| Dynamic cost/value（@Cost）| ✅ (`CostFunc`) | ✅（注解 + 反射）|
| Process 状态码 | 5 种 | 9 种 |
| Process budget（cost/tokens/actions）| ✅ | ✅ |
| Process 持久化 | ❌ | ✅ (`AgentProcessRepository`) |
| HITL Awaitable | ✅（类型化） | ✅（exception-based）|
| Workflow patterns 库 | ✅（独立 package，7 类） | ✅（DSL，3–4 类）|
| Tool registry | ✅ | ✅ |
| ToolGroup 角色 | ✅（role string） | ✅（role + 权限） |
| ToolGroupPermission 安全 | ❌ | ✅（HOST/INTERNET）|
| ToolDecorator（动作维度） | ✅ | ❌（走 Spring AOP）|
| ToolPolicy（rate-limit/once）| ✅（独立 package） | ❌ |
| ActionInterceptor（around） | ✅ | ❌ |
| GoalApprover | ✅ | ❌ |
| AgentValidator | ✅ | Spring `ContextRefreshedEvent` |
| Event listener | 走 OTel + extension | ✅（`AgenticEventListener`）|
| Annotation-based authoring | ❌（Go 限制）| ✅ (`@Agent`/`@Action`/`@Cost`) |
| DSL authoring | Builder | Kotlin DSL `agent { }` |
| ChatSession 抽象 | ❌ | ✅ |
| Multi-agent platform | `Platform` + child process | `AgentPlatform : AgentScope`（自动跨 agent planning） |
| A2A REST gateway | ❌ | ✅（独立 module） |
| MCP server | ✅ | ✅ |
| MCP tool consumption | ✅ | ✅ |
| Cost aggregation across child | ❌ | ✅ (`totalCost()`)
| Observability | OTel 原生 | Spring Actuator + Event |
| CLI shell | ❌ | ✅（独立 module） |
| Persistent agent registry | N/A | Spring ApplicationContext |

---

## II.19 lynx-agent 高 ROI 路线图（针对 embabel 差距）

按 ROI 重排，**仅列工作量 ≤ 中、价值 ≥ 中** 的项：

### P0 — 低工作量、高 ergonomics 价值

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 1 | **`agent.PromptRunner`（包装 chat.Client，自动注入 process/blackboard/tools）** | 低（~150 LOC） | action 内 LLM 调用 ergonomics 立刻翻倍；最接近 embabel `PromptRunner` |
| 2 | **`ToolCapability` 标签 / 权限模型**（借鉴 `ToolGroupPermission`） | 低（~50 LOC） | 平台级安全策略基础 |
| 3 | **`Blackboard.LastResult()` 别名常量** | 极低（~10 LOC + 文档） | 简化"取上一动作输出"的写法 |

### P1 — 生产硬刚需

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 4 | **`Process.Store` 持久化 SPI**（基于已有 vector store 后端复用 redis / postgres） | 中（~300 LOC + 1-2 个后端 ~150 LOC） | 长跑 / 中断恢复 / 跨节点 agent |
| 5 | **`Session` / 对话生命周期抽象**（聊天机器人场景必需） | 中（~200 LOC） | 对标 embabel `ChatSession` |
| 6 | **`Blackboard.BindProtected`**（跨 state-machine reset 存活） | 低（~30 LOC） | clearBlackboard 场景必需 |

### P2 — 闭合 embabel 已有的好设计

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 7 | **`LlmInvocationHistory` 一等接口**（不只走 OTel） | 中（~200 LOC） | cost 报告 / debugging / replay |
| 8 | **`totalCost()` 跨 child process 聚合** | 低（~50 LOC） | 多 agent 编排时的 cost 可见性 |
| 9 | **Action 注解推导**（基于 `go:generate` + AST 分析）| 高（~800 LOC + 测试） | 减少 Builder boilerplate ~50% |
| 10 | **AgentScope 聚合规划**（多 agent 联合规划域） | 高（~400 LOC + 测试） | 对标 embabel `AgentPlatform : AgentScope` |

### P3 — 故意不做（"为什么不抄"）

| # | embabel 有但 lynx 不该抄 | 原因 |
|---|---|---|
| A | `@Agent` / `@Action` 完整注解 DSL | Go 无 runtime annotation；强行模拟会引入复杂代码生成层 |
| B | Spring ApplicationContext / DI 容器 | lynx 是 library，不是 framework |
| C | Single-planner 路线（删 HTN / Reactive） | lynx 已投入了 HTN/Reactive 实现，多 planner 是反超点 |
| D | A2A REST gateway | MCP 已经能干这事；不应该再造一个 REST 协议 |
| E | Spring Actuator 集成 | OTel 是 Go 世界标准，重新对接 actuator 反而割裂 |
| F | Kotlin DSL 风格的"嵌套 lambda" | Go 不擅长嵌套 lambda；`workflow/` package 风格已足够 idiomatic |

---

# Part III：综合定位 — lynx 在两条对比线上的位置

## III.1 双轴坐标系

```
                                  framework
                                      ▲
                                      │
                                  spring-ai
                                      │
                       embabel-agent ─┼─── (深度整合 Spring 生态)
                                      │
                                      │
              thin-library ◄──────────┼──────────► thick-library
                                      │
                                      │
                                  lynx-core ──── lynx-agent
                                      │             (sibling)
                                      │
                                      ▼
                                    library
```

- **lynx-core** 与 **spring-ai** 是 *同维度（基础抽象层）* 的不同象限——library vs framework
- **lynx-agent** 与 **embabel-agent** 是 *同维度（agentic 运行时）* 的不同象限——同样 library vs framework
- **embabel-agent 与 spring-ai 是 *栈关系***（embabel 依赖 spring-ai）；**lynx-agent 与 lynx-core 也是 *栈关系***（在同一 repo）

**这意味着 lynx 整个 stack（core + agent）对应 embabel-agent + spring-ai 整个栈**——所以三组对比其实可以看作两组栈级对比。

## III.2 抽象同构度评估

| 抽象 | lynx-core ↔ spring-ai | lynx-agent ↔ embabel-agent |
|---|---|---|
| ChatModel / ChatClient | **高**（9/10）| N/A（agent 层不重复）|
| Message / Tool | **高**（9/10）| N/A |
| Embedding | **高**（10/10）| N/A |
| RAG pipeline | **中**（7/10——阶段拆分不同）| N/A |
| VectorStore | **高**（9/10）| N/A |
| Action / Goal / Condition | N/A | **极高**（10/10——三值逻辑都对齐） |
| Blackboard | N/A | **高**（9/10——bindProtected 差异） |
| GOAP Planner | N/A | **极高**（10/10——A* 实现都一致） |
| ProcessLifecycle | N/A | **中**（7/10——状态码细粒度不同） |
| Tool / ToolGroup | N/A | **中**（7/10——权限模型差异）|
| HITL | N/A | **中**（7/10——typed vs exception）|

**结论**：两条对比线的"抽象同构度"都很高。lynx 与对手在 *做什么* 上几乎对齐，差异都在 *怎么做*。

## III.3 lynx 的整体定位

**lynx 是一个 *Go-native, OTel-first, MCP-first 的 thin-library AI 栈***——这个定位在两条对比线上一致：

| 维度 | 表达 |
|---|---|
| 语言 | Go 1.23+ 泛型 + `iter.Seq2` streaming |
| 打包 | go.work 多 module，按需 import |
| 配置 | 构造函数 + Option struct（无 DI 容器）|
| 标准对接 | OTel 原生 + MCP 一等 |
| Vendor 矩阵 | core 层 27 vector + 39 model provider（已反超 spring-ai 广度） |
| Agent 范式 | GOAP + HTN + Reactive 三 planner（embabel 仅 GOAP） |
| ergonomics | 高表达力（泛型 + Builder），但缺 ergonomic 入口（无对等 PromptRunner） |
| 持久化 | 弱（内存 only），是当前最大架构 gap |

## III.4 整体战略路线图（合并 Part I + Part II）

按 ROI 综合排序：

### 🔴 P0 — 立刻见效（共计 ~350 LOC）

1. ~~**Retry `Transient` / `NonTransient` 分类**~~ —— 不做：SDK 自带重试
2. ~~**`agent.PromptRunner`** ergonomics 包装~~ —— 已闭合
3. ~~**Anthropic Extra 通道保护**~~ —— 已闭合
4. ~~**OTel hot path 埋点补完**~~ —— 已闭合：chat / embedding / tool / RAG / MCP / agent / 24 vectorstore / 6 chatmemory 全量

### 🟡 P1 — 生产硬刚需

5. ~~**持久化 Memory 后端**~~ —— 已闭合：顶层 `chatmemory/` 提供 6 个 provider
6. **PDF + Markdown reader**（Part I）——RAG 必需
7. **`Session` / 对话抽象**（Part II）——聊天机器人场景
8. ~~**Structured Output Converter**~~ —— 已闭合：`core/model/chat/parser.go`
9. **`ToolCapability` 安全标签**（Part II）

### 🟢 P2 — 架构补完

10. **`SafeGuard` / `Logger` middleware**（Part I）
11. **`DocumentJoiner` + `QueryRouter`**（Part I）——RAG 多路检索
12. **`LlmInvocationHistory` 一等接口**（Part II）
13. **`Blackboard.BindProtected`**（Part II）
14. ~~**VectorStore `BaseVisitor` 抽象**~~ —— 已闭合：`vectorstores/internal/filterhelp` + 10 visitor 迁移
15. **Action 注解推导（go:generate）**（Part II）

### 🔵 P3 — 大依赖 / 长尾

参见 `SPRING_AI_COMPARISON.md` §9.4 + Part II §19 P3。

## III.5 一句话定档

**lynx 用 *thin library + Go 语言原生能力 + OTel/MCP 标准对接* 三条腿，已经在 *广度* 上完成对 spring-ai 的反超（39 model / 24 vector / 多模态全栈）；在 *agent 范式* 上比 embabel 多两个 planner（HTN + Reactive）+ 类型安全工作流原语 + ToolPolicy + Extension 显式四角色。当前剩余 ROI 集中在 ergonomics（Session 对话抽象）+ RAG 输入（PDF/Markdown reader）+ middleware 长尾（SafeGuard/Logger/DocumentJoiner/QueryRouter）—— P0 + P1 + P2 主线已大部闭合，thin library 哲学不动摇**。

---

## 附录 A — 关键文件索引

### lynx-core（详见 `SPRING_AI_COMPARISON.md`）

| 抽象 | 文件 |
|---|---|
| Chat Model | `core/model/chat/model.go` |
| Message | `core/model/chat/message.go` |
| Tool | `core/model/chat/tool.go` |
| Embedding | `core/model/embedding/model.go` |
| Memory | `core/model/chat/memory/memory.go` |
| Document | `core/document/interface.go` |
| VectorStore | `core/vectorstore/store.go` |
| RAG | `rag/interface.go` |
| Evaluation | `core/evaluation/evaluator.go` |
| MCP | `mcp/provider.go` |

### lynx-agent

| 抽象 | 文件 |
|---|---|
| Agent | `agent/core/agent.go` |
| Action | `agent/core/action.go`, `agent/core/action_typed.go` |
| Goal | `agent/core/goal.go` |
| Condition | `agent/core/condition.go` |
| Determination | `agent/core/determination.go` |
| Blackboard | `agent/core/blackboard.go` |
| IOBinding | `agent/core/io_binding.go` |
| Planner | `agent/plan/planner.go` |
| GOAP A* | `agent/plan/planner/goap/` |
| HTN | `agent/plan/planner/htn/` |
| Reactive | `agent/plan/planner/reactive/` |
| Platform | `agent/runtime/platform.go` |
| AgentProcess | `agent/runtime/agent_process.go` |
| ProcessContext | `agent/runtime/process_context.go` |
| Extension | `agent/core/extension.go` |
| Workflow | `agent/workflow/` |
| ToolPolicy | `agent/toolpolicy/` |
| HITL | `agent/hitl/` |
| Builder DSL | `agent/agent.go`, `agent/builder.go` |

### spring-ai（参考）

| 抽象 | 文件 |
|---|---|
| ChatModel | `spring-ai-model/.../chat/model/ChatModel.java` |
| ChatClient | `spring-ai-client-chat/.../client/ChatClient.java` |
| ToolCallback | `spring-ai-model/.../tool/ToolCallback.java` |
| Advisor | `spring-ai-client-chat/.../advisor/api/Advisor.java` |
| RAG Advisor | `spring-ai-rag/.../advisor/RetrievalAugmentationAdvisor.java` |

### embabel-agent（参考）

| 抽象 | 文件 |
|---|---|
| Agent | `embabel-agent-api/.../core/Agent.kt` |
| Action | `embabel-agent-api/.../core/Action.kt` |
| Goal | `embabel-agent-api/.../core/Goal.kt` |
| Condition | `embabel-agent-api/.../core/Condition.kt` |
| Blackboard | `embabel-agent-api/.../core/Blackboard.kt` |
| AgentProcess | `embabel-agent-api/.../core/AgentProcess.kt` |
| AgentPlatform | `embabel-agent-api/.../core/AgentPlatform.kt` |
| A* GOAP Planner | `embabel-agent-api/.../plan/goap/astar/AStarGoapPlanner.kt` |
| PromptRunner | `embabel-agent-api/.../api/common/PromptRunner.kt` |
| Annotations | `embabel-agent-api/.../api/annotation/annotations.kt` |
| Kotlin DSL | `embabel-agent-api/.../api/dsl/AgentBuilder.kt` |
| Spring AI Bridge | `embabel-agent-api/.../spi/support/springai/SpringAiLlmService.kt` |
| ToolGroup | `embabel-agent-api/.../core/ToolGroup.kt` |

---

*对比结束。lynx HEAD `4c762f8`，2026-05-19。*
*关联文档：`SPRING_AI_COMPARISON.md`（Part I 完整版）。*
