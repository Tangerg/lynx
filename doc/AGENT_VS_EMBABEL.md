# Agent vs Embabel —— 移植对照与设计取舍

> lynx `agent` 模块的**规划内核思想**移植自 **Embabel**(Kotlin/JVM agent framework):GOAP planner 驱动、replan-every-tick、blackboard 中枢、typed-dataflow 推导 pre/postcondition、三值逻辑。本文逐块对照两者,并说明 lynx 在哪里**按 Go 第一性重新决策**、以及**为什么**。
>
> 对照基准:Embabel `main` @ `73952024`(2026-07-15,Kotlin,Maven 多模块,焊死 Spring Boot);lynx `agent` 当前分支。
>
> 结论先行:**规划思想高度收敛,工程形态几乎全盘重决策**。lynx 认同并移植了 Embabel 最核心的论点 ——「**planner-driven,不是 ReAct-loop**;LLM 只在 action *内部* 和 intent→goal 排序处出现,绝不驱动控制流」—— 但把 Embabel「Spring 容器即框架、注解扫描发现 agent、bean 装配 model provider、SpEL 条件、反射驱动」这套**与 Spring 不可分割**的形态,换成「**显式装配、可嵌入、无扫描/无注解/无全局 DI、类型分发 Extension 注册、Go 泛型构造器、`context.Context`**」的 Go 形态。Embabel 没有框架中立的核;lynx 的核**本身就是中立的**(它是 lyra 后端一次 chat turn 工具循环的执行内核)。

---

## 0. 模块对应关系

| Embabel 模块 / 概念 | 对应 lynx | 说明 |
|---|---|---|
| `com.embabel.plan.*`(planner-agnostic 规划核) | `agent/planning`(`Planner`/`Domain`/`Plan`/`WorldState`)+ `agent/core`(`Truth`/`Binding`/`Goal`/`Condition`) | lynx 把「纯规划类型」与「planner 实现」分包 |
| `com.embabel.plan.goap`(A* GOAP) | `agent/planning/goap` | |
| `UtilityPlanner` | `agent/planning/utility` | Embabel `HybridUtilityPlanner` 无直接等价 planner;lynx 另有 `htn`、`reactive` |
| `com.embabel.agent.core`(Agent/Action/Blackboard/AgentProcess) | `agent/core` + `agent/runtime` | 定义(core)与运行时(runtime)分层 |
| `Ai` / `PromptRunner` / `LlmOperations` | `agent/toolloop` + `agent/core` ProcessContext chat 面 | 工具循环是显式 `toolloop.Runner` |
| `Autonomy`(Focused/Closed/Open + `Ranker`) | `agent/routing`(`Router`/`Ranker`/`GoalApprover`) | |
| `embabel-agent-rag`(pipeline + `ToolishRag`) | **独立 `rag` 模块**(不在 agent) | RAG 是 agent 消费的能力,不焊进 runtime |
| `embabel-agent-autoconfigure`(Spring Boot 自动装配) | **无对应 —— 显式装配** | Engine 由组合根显式构造,见 §8 |
| `embabel-agent-shell`(Spring Shell) | app 层(lyra runtime / desktop) | agent 只是执行内核,不含 shell |
| `AgentProcessRepository` / `ProcessOptions.contextId` | `agent/core` `ProcessStore`/`SessionStore` + snapshot codec | 见 §9 |
| `@Agent`/`@Action`/`@Condition`/`@AchievesGoal` 注解 | `NewAgent`/`NewAction[In,Out]`/`NewCondition`/`NewOutputGoal[T]` 构造器 + 泛型 | 见 §3 |

---

## 1. 核心论点 —— 高度收敛的地方

两边共享的规划范式(lynx 有意移植):

| 论点 | Embabel | lynx | 收敛度 |
|---|---|---|---|
| **planner-driven,非 ReAct** | GOAP 算 action 序列,LLM 不驱动控制流 | 同 —— 每 tick planner 看 world state + goal 出下一步 | ✅ 完全 |
| **replan-every-tick** | 每个 action 后从头重算 plan | observe→plan→act tick 循环,每 tick 重规划 | ✅ 完全 |
| **只执行 plan 首个(可执行的)action** | `SimpleAgentProcess` 执行 `plan.actions.first()`;另有 `ConcurrentAgentProcess` | 当前 process tick 只执行计划首步;显式并发放在 workflow / child process | ⚠️ 默认路径收敛,并发路径不同(见 §10) |
| **blackboard 中枢** | 有序 append-only 对象 + named binding + 条件 map | 同 —— action 之间不直传值,一律走黑板 | ✅ 完全 |
| **typed-dataflow 推导条件** | `@Action` 入参→precondition、返回类型→effect | `NewAction[In,Out]` 的 In→input binding、Out→output binding→effect | ✅ 收敛(推导方式不同,见 §3) |
| **三值逻辑** | `ConditionDetermination{TRUE,FALSE,UNKNOWN}` | `Truth{Unknown=0,True=1,False=2}`,`And/Or/Not` | ✅ 完全 |
| **binding = `name:Type`** | `@JvmInline value class IoBinding("name:Type")`,默认名 `it`/`lastResult` | `Binding{Name,Type}`,`String()`="name:Type",默认名 `it`/`last_result` | ✅ 完全 |
| **最小成本 GOAP 搜索** | A\*:g=Σcost,h=未满足 goal precondition 数 | 确定性 uniform-cost(Dijkstra / A\* with h=0) | ⚠️ 刻意分歧:lynx 用较保守的搜索换取严格的最小成本语义 |
| **cost/value 是 world-state 的函数** | `typealias CostComputation=(WorldState)->ZeroToOne` | `type ScoreFunc func(WorldState) float64`,`FixedScore` 提常量 | ✅ 完全 |
| **goal 可达性预检 + 迭代上限** | 反向可达检查 + `maxIterations=10000` | `hasGoalProducers()` 保守 producer 检查 + `MaxIterations` | ✅ 主体收敛 |
| **prune 掉无 plan 引用的 action** | `planner.prune(system)` | `Domain.Prune(...)` | ✅ 完全 |
| **intent→goal/agent 排序路由** | `Autonomy` + `LlmRanker` + 置信阈值 + `GoalChoiceApprover` | `routing.Router` + `Ranker`/`ModelRanker` + cutoff + `GoalApprover` | ✅ 收敛(见 §7) |

> 这一栏是「移植」的实质:lynx 把 Embabel 的规划论文级设计几乎照单接受。**下面所有章节讲的是分歧 —— 全在工程形态,不在规划思想。**

---

## 2. 规划核 —— 两套接口 vs 分包 + 更多 planner

**Embabel**:两层接口,`core.Action` **同时是** GOAP `ConditionAction`(一个类型既是「可执行代码」又是「规划节点」)。

```kotlin
interface Step : Named { val value: CostComputation }        // com.embabel.plan
interface Action : Step { val cost: CostComputation; fun netValue(s)=value(s)-cost(s) }
interface ConditionAction : Action { val preconditions: EffectSpec; val effects: EffectSpec }  // plan.common.condition
interface core.Action : DataFlowStep, ConditionAction, ActionRunner, ... { fun execute(ctx): ActionStatus }
internal class AStarGoapPlanner : OptimizingGoapPlanner  // A* over ConditionWorldState
```

公开的 `PlannerType` 目前有四种:`GOAP`(默认)、`UTILITY`、`HYBRID`、`SUPERVISOR`。`OptimizingGoapPlanner` 有个巧思 —— 遇到恰好一个 UNKNOWN 条件时,对 TRUE/FALSE 两个变体都规划,只有当两个 plan 不同才**惰性求值**该条件再重规划。

**lynx**:把「纯规划类型」与「planner 实现」**分包**,`Action` 是接口但 `WorldState` 也是接口(不可变快照)。

```go
// agent/planning —— planner-facing 类型
type Planner interface {
    core.Extension                                    // planner 本身是引擎 Extension
    PlanToGoal(ctx, state core.WorldState, domain *Domain, goal *core.Goal, opts Options) (*Plan, error)
}
// Domain 上是 template 方法:Plans / BestPlan(按 NetValue 降序)/ Prune —— 每个 planner 只写算法部分
// agent/core
type WorldState interface { Conditions() ConditionSet; Timestamp() time.Time; Key() string; Apply(effects) WorldState }
type Action interface { Metadata() ActionMetadata; Execute(ctx, *ProcessContext) (ActionStatus, error) }
```

**取舍与理由**:

- **planner 是 `Extension`,靠类型分发选中,不是 sealed `when`**。Embabel 用 `PlannerType` enum + factory dispatch 选 planner。lynx 让每个 planner 实现 `Planner` 接口并注册为 `core.Extension`,运行时按 `AgentConfig.PlannerName` 匹配 `Extension.Name`。组合根默认注册 `goap` / `reactive`;`htn` / `utility` 显式注册后可选。**加 planner = 新起一包实现接口,不改 runtime dispatch**(OCP 落点)。
- **两边现在都是四种规划能力,但集合不同**。lynx 是 `goap`、`htn`、`utility`、`reactive`;Embabel 是 `GOAP`、`UTILITY`、`HYBRID`、`SUPERVISOR`。lynx 独有 HTN / reactive planner;Embabel 独有 hybrid utility / supervisor planner。lynx 的 supervisor 是 workflow 组合器,不是 planner。
- **`WorldState` 是不可变快照接口 + `Key()` 去重**。Embabel 的 `ConditionWorldState` 是 data class + `plus(action)`。lynx 的 `WorldState` 显式要求 `Apply` 不得 mutate、`Key()` 给搜索的 settled-state 集合去重 —— 契约写进接口 doc,planner 依赖快照不可变。
- **`Truth` 是 `int8` 且 `Unknown=0`**(零值即 Unknown)。Embabel 用 enum。lynx 选零值语义:「一个没设过的条件天然读作 Unknown,无需初始化」—— GOAP 剪枝依赖这点。

> **里程碑修正**:lynx 已删除“未满足 goal 条件数一定 admissible”的错误假设,改为确定性的 uniform-cost search(`h=0`)。动态 action cost 在每条候选 transition 的源 `WorldState` 上求值,因此保留 state-dependent cost 能力;`ScoreFunc` 必须纯且确定,负数、NaN、Inf 会以 `goap.ErrInvalidActionCost` 明确失败。由于 cost 可以读取任意条件,lynx 同时删除 STRIPS relevance pruning 和搜索后 action removal:一个动作即使不直接贡献 goal/precondition,也可能通过改变状态降低后续成本,不能被静态闭包安全删除。只要实际边 cost 有限且非负,第一个出队的 goal state 及其原始 predecessor path 就是全局最小成本计划;多 effect action与小于 1 的 cost 不再破坏最优性。Embabel 仍保留原 A* 启发式,因此这是 lynx 为严格正确性做的主动分歧。

---

## 3. Agent / Action 编排 —— 注解反射 vs Go 泛型构造器

这是**工程形态上最大的分歧**。

**Embabel**:`@Action` 方法经反射变成规划元数据,且是 Spring bean。

```kotlin
@Agent(description="...", planner=GOAP)         // meta-@Component,被 classpath 扫描 + DI 注入
class HoroscopeAgent {
    @Action fun retrieveHoroscope(p: StarPerson, ai: Ai): Horoscope =   // 入参 StarPerson→precondition,返回 Horoscope→effect
        ai.withDefaultLlm().createObject("...")
    @AchievesGoal(description="...") @Action fun writeup(h: Horoscope): Writeup = ...  // 合成一个 Goal
}
```

`AgentMetadataReader`(`@Service`)反射 bean:`@Action` 方法参数经 `ActionMethodArgumentResolver` 链(`ProcessContext`/`OperationContext`/`Ai`/`@Provided`/最后 `BlackboardArgumentResolver`)→ 只有 domain-object 参数产出 `IoBinding` precondition;返回类型→output effect,**并把返回类型的所有子类型 + 父类型也灌进 effect 条件空间**(多态烘进条件)。**参数名有意义**(需 `-parameters` 编译标志)。`@AchievesGoal` 合成 `Goal`。

**lynx**:`NewAction[In,Out]` 泛型 + 显式 config struct,零注解、零 bean 扫描、不依赖参数名。

```go
// agent/core
type AgentConfig struct { Name, Description, Version string; Actions []Action; Goals []*Goal; Conditions []Condition; PlannerName string; StuckPolicy StuckPolicy }
func NewAgent(config AgentConfig) *Agent               // read-only 定义聚合,构造期防御性快照
type ActionMetadata struct { Name string; Inputs, Outputs []Binding; Preconditions, Effects ConditionSet; Retry RetryPolicy; Cost, Value ScoreFunc; ... }
// NewAction[In,Out](...) 捕获 In/Out 的 reflect.Type → 生成 input/output Binding
func NewBinding[T any](name string) Binding            // reflect.TypeFor[T](),指针归一,pkgpath 限定
func NewOutputGoal[T any](config GoalConfig) *Goal      // precondition = "类型 T 的产物在黑板上"
```

**取舍与理由**:

- **显式装配,不引入扫描/注解/全局 DI**(agent 模块定位硬约束)。Embabel 的 agent **必须**是 Spring bean —— 「你无法在不引入 Spring 组件模型的情况下声明一个 agent」。lynx 的 `Agent` 是 `NewAgent(AgentConfig)` 构造的只读值,组合根显式 append actions/goals。可嵌入、可单测、无容器依赖 —— 这正是它能当 lyra chat-turn 内核的前提。
- **泛型捕获类型,不靠反射发现方法**。lynx 只用 `reflect.TypeFor[T]()` **捕获 binding/schema 的类型名**,不用反射去 *发现* action/扫描注解/读参数名。`NewAction[In,Out]` 让 In→input binding、Out→output binding,类型信息端到端保留,但发现路径是编译期泛型,不是运行期反射 + Spring `ReflectionUtils.doWithMethods`。
- **多态能力并不对称(关键 tradeoff)**。Embabel 把返回类型的**子/父类型展开进 GOAP 条件键**,让 planner 能按类型层级链接。lynx 的 planner binding 与 `Blackboard.Lookup` 都按规范化后的**具体类型名精确匹配**;只有调用者已经拿到一个值以后,`Get[T]` / `Objects[T]` / `Last[T]` 的 Go 类型断言才可能匹配接口。因此“接口输入由具体实现自动满足”目前不是 planner/lookup 契约。好处是条件空间小且可诊断,代价是 Embabel 已有的多态 dataflow 自动连边能力目前缺失。
- **验证是显式方法 + `AgentValidator` SPI 两层**。`Agent.Validate()` 做结构检查(唯一命名、retry 安全、保守 goal 可达性);`AgentValidator` extension 加部署期自定义规则。Embabel 的 `AgentMetadataReader` **从不抛异常**(warn),坏注解不炸启动 —— 因为它在 Spring 启动期跑。lynx 在部署期显式 `Validate`,`errors.Join` 一次报全所有定义问题。

---

## 4. Blackboard —— 一个接口 + SpEL vs reader/writer 分离 + 纯 Go

**Embabel**:一个 `Blackboard` 接口,operator 重载,反射「megazord」聚合,SpEL 表达式条件。

```kotlin
interface Blackboard : Bindable, MayHaveLastResult {
    operator fun set(key, value); operator fun get(name); fun bind(v); fun bindProtected(k,v)  // 存活 clear()
    fun setCondition/getCondition; fun spawn(): Blackboard; fun expressionEvaluationModel()     // SpEL
    val objects: List<Any>   // append-only,只 hide 不删
}
// BlackboardWorldStateDeterminer:逻辑条件走 SpEL LogicalExpressionParser,name:Type 走类型绑定,hasRun_* 走标志
```

**lynx**:**reader / writer 结构化分离**,transient 变体,Blackboard 本身是 Extension(prototype),泛型顶层函数,**无 SpEL**。

```go
// agent/core
type BlackboardReader interface { ID() string; Load(key)(any,bool); Lookup(var,typeName)(any,bool); HasValue(...); Objects()[]any; Condition(key)(bool,bool); Inspect(verbose) string }
type BlackboardWriter interface { Store; StoreTransient; Add; AddTransient; Bind; BindTransient; StoreProtected; Hide; StoreCondition; ... }
type Blackboard interface { Extension; BlackboardReader; BlackboardWriter; Clone() Blackboard; ClearWorkingState() }
func Get[T any](bb BlackboardReader, name string) (T, bool)   // 顶层泛型(Go 方法不能带类型参数)
```

**取舍与理由**:

- **reader/writer 分离是结构性 ISP**。`ConditionEnv.Blackboard` 只给 `BlackboardReader` —— 编译器保证 condition 在 OBSERVE 阶段**不能 mutate** 状态。Embabel 的 condition 拿到的是完整 `Blackboard`,约束靠约定。
- **transient 变体(`StoreTransient`/`BindTransient`/`AddTransient`)**:参与实时 lookup 但**排除出持久 snapshot**——给 handle/client/channel 等运行时状态用。Embabel 无此区分(它靠 `AgentProcessRepository` 整体持久化)。这是 lynx durable resume 的基础(见 §9)。
- **无 SpEL / 无表达式语言**。Embabel 用 SpEL(`LogicalExpressionParser`)做逻辑条件求值。lynx 的 condition 是 **Go 函数**(`ConditionFunc`)或 `PromptCondition`(LLM 驱动)—— 不引入嵌入式表达式语言(KISS + 无隐藏求值面)。逻辑组合是 `And/Or/Not` composite condition,纯 Go。
- **Blackboard 是 Extension + prototype 模式**:注册一个,运行时用 `Clone()` 给每个进程一份隔离实例。Embabel 用 `spawn()`。语义近似,但 lynx 用 Extension 注册统一入口。
- **`StoreProtected` 存活 `ClearWorkingState()`**,对应 Embabel `bindProtected`——收敛。

---

## 5. Action 从内部访问 LLM —— `Ai` 网关 god-object vs 窄 ProcessContext + 显式 toolloop

**Embabel**:action 参数注入 `Ai`/`OperationContext`,fluent immutable `PromptRunner`,内部 `LlmOperations` 跑工具循环。

```kotlin
interface Ai { fun withLlm(LlmOptions); fun withDefaultLlm(); fun withLlmByRole(role); fun withAutoLlm(); ... }  // 全返回 PromptRunner
interface PromptRunner { fun <T> createObject(prompt, Class<T>): T; fun respond(...); fun evaluateCondition(...); withTool/withToolGroup/withGuardRails/... }
internal interface LlmOperations { createObject/generate/doTransform(messages, LlmInteraction, outputClass, agentProcess, action) }  // 解析 tool callback + 跑循环
```

**lynx**:`ProcessContext` 按面拆成多个窄文件,工具循环是**显式的 `toolloop.Runner`**,不是藏在 `LlmOperations` 里。

- `ProcessContext` 的能力按文件分区:`process_context_chat.go`(模型调用)/ `process_context_control.go` / `process_context_interaction.go`(HITL)/ `process_context_tools.go` / `process_context_usage.go` —— 不是一个 `Ai` god-object。
- 工具循环 = `toolloop.Runner`:消费 `chat.Model` + `Request` + `ToolResolver`,emit `iter.Seq2` 的 `Event`(model/tool/pause/resume)。**默认互斥、显式 opt-in 的有界资源键并发、无自动 retry、可 checkpoint/resume**。执行可并发,但 `ToolResult` event 与下一轮 model request 始终按模型原始 tool-call 顺序提交,因此 chat-history/cache 输入稳定。协议值(model request/response)与运行时状态(可执行工具、pause)**分离**,不往 provider `Response` 塞运行时状态。
- model 选择走 process-scope `core.ChatProvider` / `ChatCapability`;`routing.ModelRanker` 只负责给 `agent × goal` 路由候选打分。结构化输出走 core `chat.JSONParser[T]`。

**取舍与理由**:

- **工具循环是一等运行时对象,不是链里的隐式 link**。Embabel 把工具循环埋进 internal `LlmOperations`。lynx 把它显式成 `toolloop.Runner` 状态机 —— 因为 HITL pause/resume 要**精确定位 pending call**(resume 在挂起的那个 tool call 处续跑,跳过已完成的模型轮和工具),埋进黑盒就做不到([[project_hitl_resume_at_pending_call]])。
- **窄面替 god-object**。`Ai` 一个接口聚了 with-llm/with-tool/createObject/evaluateCondition 全部能力。lynx 按 ProcessContext 的职责切文件、切窄面 —— chat / control / interaction / tools / usage 各管一域(SRP)。
- **model 选择与 intent 路由是两条独立边界**。`routing.Ranker` / `ModelRanker` 选择 agent goal;`core.ChatProvider` 为某个 process 提供实际 `ChatCapability`,app 层再承载 BYOK、provider/model 名称与角色配置。这样不会让“选择做什么”和“选择用哪个模型做”共享一个含混抽象。

---

## 6. 扩展机制 —— Spring bean 扫描 vs 类型分发 Extension 注册

**Embabel**:能力 = Spring bean;发现 = `BeanPostProcessor` + component scan;model provider = 命令式 `registerSingleton`。

- `@Agent`/`@EmbabelComponent` 是 meta-`@Component`;`ScanConfiguration` 对 8 个包 `@ComponentScan` + `@ConfigurationPropertiesScan`;`DelegatingAgentScanningBeanPostProcessor` 攒 bean 到 `ContextRefreshedEvent` 再 `AgentMetadataReader` 读进 `agentPlatform.deploy(...)`。
- model provider:每 provider 一个 `@AutoConfiguration`,`@Bean : ProviderInitialization` 读 `models/<provider>-models.yml` 后**命令式 `ConfigurableBeanFactory.registerSingleton(name, llm)`**(21 处);`modelProvider` 再 `getBeansOfType` 收回来。
- Spring 耦合普查(主源码):27 `@AutoConfiguration`、47 `@Bean`、24 `@Import`、22 `@ConfigurationProperties`、21 `registerSingleton`、4 `EnvironmentPostProcessor`……**DI 是内禀的,没有 Spring-free 核**。

**lynx**:能力 = `core.Extension`;发现 = **运行时按类型断言收集**;稳定构造依赖 = `Config` 显式字段。

```go
// agent/runtime —— 行为插件机制是 core.Extension 注册表
// 一个泛型 collector 在 dispatch 时按接口类型断言收集:event listener、action/tool middleware、
// AgentValidator、GoalApprover、tool-group resolver、id generator、blackboard prototype、planner …
// per-process Extension 与 engine-scope 合并;稳定依赖(chat/ProcessStore/root+child SessionStore/snapshot policy)是 Config 字段,不进注册表
```

**取舍与理由**(这是**根本结构差异**):

- **agent 是可嵌入 framework,不是需要容器的应用**。Embabel「采用它 = 全盘采用 Spring 容器、组件扫描、Boot 自动装配、Environment 机制」。lynx agent 由组合根显式 `New(Config)` 构造,能被 lyra 后端当库嵌进去跑一次 chat turn。**没有 Spring-free 核这件事,是 Go 移植必须重新设计而非翻译的部分** —— lynx 就重新设计了。
- **类型分发,不是 string-key 注册**(模块反向不变量:❌ 用 string-key 注册 Extension)。加能力 = 实现某个 Extension 子接口并注册,运行时按类型断言自动收集 —— 不改任何 dispatch loop(OCP)。Embabel 靠 Spring bean 类型 + `@ConditionalOnMissingBean`,lynx 靠 Go 接口类型断言 + 显式注册。
- **稳定依赖显式,不藏进注册表**:`chat`、`ProcessStore`、root/child `SessionStore`、snapshot policy 是 `Config` 的具名字段。root multi-turn 与 delegated child 是两个生命周期；同一后端可以显式承担两者，但只懂 subtask lineage 的 adapter 不能冒充 root store。Embabel 把一切(含 repository、ranker、eventListener)都走 `@Bean` + 构造注入。lynx 只把「可替换的跨切面」放 Extension,「稳定构造依赖」留 Config 字段 —— 区分「装配」与「扩展」。
- **model provider 无 `registerSingleton` 魔法**:provider 适配在 `models` 模块,组合根显式装配进来;不做 YAML 读表 + 命令式 bean 注册那套。

---

## 7. Intent 路由 —— Autonomy 三模式 vs Router + GoalApprover

**Embabel**:`Autonomy`(`@Service`)包平台,`LlmRanker` 打分 + 置信阈值 + `GoalChoiceApprover`。

- **Focused**:直接 `runAgentFrom` / `AgentInvocation<T>`,无排序。
- **Closed**:`chooseAndRunAgent(intent)` 排序 **agents**,选最优跑单个 agent(不跨 agent 混 action)。
- **Open**:`chooseAndAccomplishGoal(...)` 排序全平台 **goals**,过 `GoalChoiceApprover`,`createGoalAgent` 合成一个单目标 agent(从全平台 action 池,可 A* 剪枝)。

**lynx**:`routing.Router` + `Ranker`,枚举 agent×goal,置信 cutoff,`GoalApprover` 把 planner 锁定到选中 goal。

```go
// agent/routing
type Ranker interface { /* 给定 user input,对每个候选 goal 打 [0,1] */ }   // SPI:ModelRanker(LLM)/regex/hybrid
// Router:枚举引擎已部署 agents × 其 goals → Ranker → cutoff → Router.Run 启动获胜 agent
//        并带 per-process GoalApprover 把 planner 锁到选中 goal
```

**取舍与理由**:结构高度收敛(`Ranker`≈`LlmRanker`,`GoalApprover`≈`GoalChoiceApprover`,cutoff 概念一致)。差异:

- lynx 的 `Ranker` 是**消费方定义的窄 SPI**(model-driven / regex / hybrid 都能插),`Router` 是编排者 —— 两个正交类型,而非 Embabel 的 `Autonomy` 一个 `@Service` 聚合三模式。
- lynx 用 **per-process `GoalApprover`** 锁定 goal(planner 只认这一个),对应 Embabel Open 模式的 `createGoalAgent` + approver —— 但不合成新 agent,而是在既有 agent 上约束 planner 的 goal 选择。更轻。

---

## 8. 装配 —— Spring Boot 自动装配 vs 组合根显式构造

**Embabel**(见 §6 普查):`.imports` + `@ComponentScan` 发现 agent,`DefaultAgentPlatform` 14 参构造注入,`AgentPlatformConfiguration` ~16 个 `@Bean`,4 个 `EnvironmentPostProcessor` 早期改环境(注日志主题、排除 Spring AI 的 MCP autoconfig、shell 强制 `web-application-type=none`)。

**lynx**:`runtime.Engine` 由**组合根 `New(Config)` 显式构造**;`Config` 具名字段带 chat/store/snapshot policy;Extension 显式注册;per-process Extension 在 `ProcessOptions.Extensions` 里合并。无 `.imports`、无 EPP、无 bean 图。

**取舍与理由**:这是 lynx 定位的直接结果 ——「Agent 是 framework,不是小库;但框架仍**显式装配、可嵌入**,不引入扫描、注解或全局 DI 容器」。好处:启动无反射扫描、无容器生命周期、可在任意组合根(含测试)一行构造、可作为库嵌入 lyra。代价:用户要显式列出 actions/goals/extensions —— 但这正是 lynx 想要的透明度(所有装配在代码里可见、可 diff、可 review),而不是散在注解 + classpath + YAML + EPP 里的隐式魔法。

---

## 9. 进程生命周期 / HITL / 持久化 —— repository update vs type-tagged snapshot

**Embabel**:`AgentProcess` 状态机(`RUNNING/WAITING/PAUSED/STUCK/COMPLETED/...`),`AgentProcessRepository`(默认 in-memory,持久版 = 跨重启可恢复),每 tick `repository.update`;`ProcessOptions.contextId` 从 `ContextRepository` rehydrate 黑板;`ephemeral=true` 契约 = 从不持久、无 wait state、不能派生子进程;`ThreadLocal<AgentProcess>` 传当前进程;`Budget` 早终止(maxActions/maxTokens/cost)。HITL 靠 `ProcessWaitingException` + `Awaitable`(`ConfirmationRequest`/`FormBindingRequest`)。

**lynx**:`Engine` + `Deployment` + process registry;`StatusWaiting` 是一等状态;`Resume`/`Continue` 记录回复后**从精确挂起点重入**;durable 靠 `ProcessStore`、分离的 root/child `SessionStore` + **type-tagged snapshot codec**;budget 树跨子进程;`stop_policy`/`stuck_policy`/`termination` 分文件。

**取舍与理由**:

- **durable resume 更精细**。Embabel 每 tick 整体 `repository.update`,持久 repo 即可跨重启续跑 —— 粒度是「整个进程状态」。lynx 用 **type-tagged snapshot**(chatInput round-trips、`LastWorld json:"-"` 派生、protected 在 re-run 时 re-bind、HITL park/interrupt 靠 atomic `Consume` 幂等 `DELETE...RETURNING`),且 **HITL resume 在 pending tool call 处续跑,跳过已完成的模型轮和工具**([[project_durable_resume_design]] + [[project_hitl_resume_at_pending_call]])。这是 lyra 生产级 chat turn 恢复的需求,比 Embabel 的整体 update 更外科手术。
- **transient 状态显式排除持久化**(§4)——handle/client/channel 不进 snapshot,对应 Embabel 的 `ephemeral` 但粒度到单个 binding。
- **Session identity 是领域合同而不是 `contextId` 字符串**。Session 校验 ID/parent/Agent/audit time；child adapter 必须 round-trip UserID、AgentName、时间与 metadata。同一 child ID 不能静默换 parent/user/Agent。Embabel 的 `ProcessOptions.contextId` 负责 rehydrate context，但没有这层 root/child persistence ownership 分离。
- **`context.Context` 传进程,不用 `ThreadLocal`**。Go 无 ThreadLocal;进程/取消/trace 靠 `ctx` 显式传递,脱钩后台 goroutine 用 `context.WithoutCancel` 保 span。
- **HITL 是显式 suspension + 幂等 Consume**,不是抛 `ProcessWaitingException`。lynx `hitl.Interrupt` 产出 suspension,`Engine.Resume` 记录 response 到精确 suspension,`Continue` 重入 —— park/interrupt 幂等。

---

## 10. 并发 —— ConcurrentAgentProcess vs workflow 组合器 + toolloop

**Embabel**:两种进程类型。`SimpleAgentProcess` 顺序执行 plan 首步;`ConcurrentAgentProcess`(`@ThreadSafe`)每 tick 找出 plan 里**所有当前可达的 action 并发跑**(`Asyncer.async{}` + 虚拟线程),replan 请求收集只应用第一个 —— 即「planner 决定并行化」。`CopyOnWriteArrayList`/`AtomicReference` 保线程安全。

**lynx 当前实现**:把并发放在不共享可变 planning state 的边界,并把顺序作为显式提交协议。

- **为什么曾退化为串行已经定位到提交级根因**。`b2100d507` 最初加入 conflict-aware
  并发，`adeb82cb5` 曾专门修复 observer decorator 丢失 `ConcurrencyKey`；
  `f147ed7b2` 删除旧 chat runtime 时连同并发 invoker 一起移除，随后
  `ab30d8943` 新建的 Event Runner 只实现逐 call 串行；最后 `4f4fb5651` 迁移 App
  到新 Runner 时又删除了 observer 的能力透传。也就是说，底层工具上的
  `ConcurrencyKey` 方法仍在，但既没有 Runner 消费，也在 App 最外层包装处被遮蔽。
- **toolloop 已恢复 conflict-aware 并发**。工具默认互斥;实现 `toolloop.ConcurrentTool` 后可返回 `(resourceKey,true)`:空 key 表示无已知冲突,同一非空 key 串行。`MaxConcurrentCalls` 默认 8,阻止模型 fan-out 击穿 provider/本地资源。
- **执行顺序与提交顺序分离**。同一并发段内工具可乱序完成,但 `ToolResult` event、continuation tool message、下一轮 model/cache 输入始终按原 tool-call 顺序提交。checkpoint v2 为每个 call 保存 `queued/completed/paused` 独立状态、`NextResult` 与原 `MaxConcurrentCalls`,因此后完成的结果可以安全缓冲在前序 pause 后面,重启也不会静默换一套调度宽度。
- **插件失败被限制在工具边界**。工具 `Call` panic 被转换为当前位置的 recoverable error ToolResult,不会从并发 goroutine 击穿 Host;同批 sibling 的结果仍按调用顺序提交。
- **AgentTool 是并发安全的子进程能力**。每个调用拥有隔离 child process,通过精确 `tool_call_id` 关联;同名、同参数的多个调用不会混淆。多个 child 同时暂停时,parent 对外仍只暴露 call-order 中最早的 suspension,其余 suspension 已持久化但不越序可见。
- **workflow 组合器做显式 fan-out**:`scatter-gather`/`parallel`/`repeat-until-acceptable`/`loop`/`sequence`/`consensus`/`supervisor` —— 每个分支拿 `Clone()` 黑板,mutation 在确定性 join 前丢弃(不共享写)。这些组合器**都编译回普通 GOAP agent**,不是新 runtime 概念。
- `StartChild` 支持显式异步 child process;`workflow.Parallel` / `ScatterGather` 支持有界并发与按输入顺序稳定 join。普通模型一次返回多个 AgentTool call 时也会并发启动独立子 Agent。
- process tick 刻意只执行 plan 首步。旧的 process-wide action 并发已删除,因为多个 action 共享父黑板且完成顺序会改变 world state;这一删除与 tool-round 并发回归不是同一件事。

**判断**:保留串行 process tick 是正确的确定性取舍;tool-round / child-process 并发则已恢复到正确层级。此次修正不是简单搬回 goroutine,而是同步完成“有界资源键调度 + 按原 call 顺序稳定提交 + 多 pending call checkpoint + child forest save/restore + active-branch Resume”。因此并发提高吞吐,却不让共享黑板、cache key、durable replay 受完成时序支配。

---

## 11. RAG —— 焊进框架 vs 独立模块

**Embabel**:`embabel-agent-rag` 是框架的一部分,**两代并存**:
- 遗留 pipeline:`RagService`/`NavigableRagService`/`FacetedRagService` + 两级装饰增强器(HyDE→recall widening→dedup→chunk-merge→压缩→rerank→filter)+ RAGAS 质量度量。
- 当前 agentic:`ToolishRag` 反射 `SearchOperations` 装出能力域工具(vector/text/regex/find/expand),**让 LLM 自己迭代检索**,透明 metadata/entity filter 做多租户隔离。

**lynx**:RAG 是**独立 `rag` 模块**,不在 agent framework 内。agent **消费** RAG(作为工具/能力),但 runtime 不内建 RAG pipeline。core 定义 `vectorstore` 语义,`rag` 模块实现检索增强。

**取舍与理由**:高内聚低耦合。RAG 是一个 domain,不该焊进 agent runtime —— agent 只需能「调用一个检索工具」,至于工具背后是 pipeline 还是 agentic 迭代,是 `rag` 模块的事。Embabel 把两代 RAG(含正在弃用的 pipeline)都塞进 agent 模块,lynx 保持 agent runtime 只关心「规划 + 执行 + 工具循环」,RAG 通过工具边界接入。这也让 agent 模块不背 RAGAS/Lucene/Tika 这些重依赖。

---

## 12. 可观测性 & 「人格」

**Embabel**:所有日志派生自 `AgenticEventListener` 事件流(~25 种进程事件);`instrumentation` 默认 NoOp,`embabel-agent-observability` 模块贡献 Micrometer/OTel(「无模块 = 无 span」);**「人格」主题**(starwars/severance/colossus/...)一个属性同时换 JLine prompt provider + 日志监听器 + 配色。

**lynx**:`agent/event` 多播事件;OTel span 在 `lynx/agent` tracer(planner 用 `lynx/agent/planner`)按 action/replan/run 埋点,`otel` 模块装饰;无「人格」主题(那是 app 层 UI 的事,不是执行内核的事)。

**取舍与理由**:事件流驱动可观测这点收敛(两边都不在业务代码撒日志,而是发事件 / 开 span)。差异:lynx 把「人格 / 日志叙事风格」这类**表现层**关注留在 app 层,agent 执行内核只发结构化事件 + 开 span,保持内核与呈现分离。

---

## 13. Kotlin/Spring-ism → Go 的系统性重决策

| Embabel 形态 | lynx agent 形态 | 理由 |
|---|---|---|
| `@Agent`/`@Action` 注解 + Spring bean 扫描 | `NewAgent(AgentConfig)` + `NewAction[In,Out]` 泛型 | 显式装配、可嵌入 |
| `AgentMetadataReader` 反射发现 + 参数名依赖 | 泛型捕获类型,无运行期发现 | 无反射扫描、不依赖 `-parameters` |
| Spring 容器 / Boot autoconfigure / EPP | 组合根 `New(Config)` 显式构造 | 无全局 DI 容器 |
| `@Bean` + `registerSingleton` model wiring | `models` 模块适配 + 组合根显式装配 | 无 bean 魔法 |
| interface + private data class + `companion invoke()` | 显式构造器返回值/指针 | Go 惯例 |
| operator 重载(`state+action`、`bb["k"]=v`) | 普通方法(`Apply`、`Store`) | 无 operator 重载 |
| `@JvmInline value class IoBinding` | `Binding` struct | 值语义 |
| SpEL `LogicalExpressionParser` 条件 | `ConditionFunc`(Go 函数)+ composite | 无嵌入式表达式语言 |
| `ThreadLocal<AgentProcess>` | `context.Context` 显式传 | Go 无 ThreadLocal;WithoutCancel 保后台 span |
| `PlannerType` enum + `when` 选 planner | planner 是 `Extension`,类型分发 | 加 planner 不改 dispatch(OCP) |
| Spring bean 类型即 Extension | `core.Extension` 接口类型断言收集 | 无 string-key、可嵌入 |
| `ProcessWaitingException` HITL | 显式 suspension + 幂等 Consume | 精确挂起点重入 |
| `AgentProcessRepository` 整体 update | type-tagged snapshot + transient 排除 | 外科手术式 resume |

---

## 14. Embabel 有、而 lynx agent 刻意没有

- **Spring Boot 自动装配 / 注解扫描 / bean 发现**(§6、§8)—— 显式装配替代。
- **`@Action`/`@Agent` 注解编排**(§3)—— Go 泛型构造器。
- **SpEL 逻辑条件**(§4)—— Go 函数条件。
- **返回类型子/父类型烘进条件空间**(§3)—— exact binding + 黑板读取处类型断言。
- **RAG 焊进框架**(§11)—— 独立 `rag` 模块。
- **「人格」日志主题**(§12)—— 表现层留 app。
- **Spring Shell 交互面**—— agent 只是内核,shell 在 app 层。
- **Utility「Nirvana」永不完成 goal(给 chat 用)**—— lynx 用 `reactive` planner + `toolloop` 跑 chat turn。

## 15. lynx agent 有、而 Embabel 没有(或更弱)

- **HTN / reactive planner**;Embabel 对应地有 lynx planner 集合里没有的 HYBRID / SUPERVISOR planner。
- **workflow 组合器**(scatter-gather/repeat-until/consensus/supervisor)编译回普通 agent(§10);Embabel 靠 GOAP + `@State` 状态机表达。
- **reader/writer 分离黑板**(结构性 ISP,OBSERVE 不能 mutate)(§4)。
- **类型分发 Extension 注册**(无 Spring、可嵌入、无 string-key)(§6)。
- **外科手术式 durable resume**(type-tagged snapshot、HITL 在 pending tool call 处重入、park/interrupt 幂等)(§9)。
- **三档子进程委派梯度**(全继承 / 仅 ambient / 全空,默认仅 ambient 以免预满足子 agent 产出目标)([[agent_delegation_spawn_semantics]])。
- **框架中立的核**(无 DI 容器依赖)—— 可作为 lyra chat-turn 执行内核嵌入。
- **`RetrySafety` 更强**:`idempotent` / `compensated` 才允许 action `MaxAttempts>1`;Embabel 的 `ActionQos` 也有 `idempotent` 声明,但没有 compensated 语义,且默认 `maxAttempts=5`、`idempotent=false` 的组合仍把最终安全约束留给外围策略。
- **arch test + wire fixture**(与 core 同规格)冻结 agent 公共面 / snapshot wire。

---

## 一句话总结

> Embabel 证明了一条正确的路 ——「**GOAP planner 驱动、replan-every-tick、blackboard 中枢、typed-dataflow 推导条件、三值逻辑**」是比 ReAct-loop 更可解释、更可扩展的 agent 范式。lynx `agent` **接受了这套规划思想的主干**(§1),但把 Embabel「**与 Spring 不可分割**」的工程形态 —— 注解扫描、bean 发现、`registerSingleton` model wiring、SpEL 条件、`ThreadLocal`、Autonomy god-service —— 换成「**显式装配、可嵌入、类型分发 Extension、Go 泛型构造器、`context.Context`、显式 toolloop、外科手术式 durable resume**」的 Go 形态。两边当前都是四种 planner 能力但集合不同;lynx 更强在框架中立、workflow、严格最小成本 GOAP、按调用顺序提交的 tool/子 Agent 并发、durable resume 与显式安全边界,Embabel 更强在多态 dataflow、Hybrid/Supervisor planner、共享计划 action 的并发进程与成熟 Spring 装配。
