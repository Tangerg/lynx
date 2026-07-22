# Agent vs Embabel —— 移植对照与设计取舍

> lynx `agent` 模块的**规划内核思想**移植自 **Embabel**(Kotlin/JVM agent framework):GOAP planner 驱动、replan-every-tick、blackboard 中枢、typed-dataflow 推导 pre/postcondition、三值逻辑。本文逐块对照两者,并说明 lynx 在哪里**按 Go 第一性重新决策**、以及**为什么**。
>
> 对照基准:Embabel `main` @ `b1a923d1c`(2026-07-19,`v1.0.0-1-gb1a923d1c`,`1.0.1-SNAPSHOT`);lynx @ `cc4319c6d`,其中 `agent` 最近提交为 `d73188038`(2026-07-21)。基准于 2026-07-22 执行 `git pull --ff-only` 后确认。
>
> 结论先行:**规划思想高度收敛,工程形态几乎全盘重决策**。lynx 认同并移植了 Embabel 最核心的论点 ——「**planner-driven,不是 ReAct-loop**;LLM 只在 action *内部* 和 intent→goal 排序处出现,绝不驱动控制流」—— 但把 Embabel 完整 AgentPlatform 的「Spring 容器、注解扫描、bean 装配 model provider、SpEL 条件、反射驱动」形态,换成「**显式装配、可嵌入、无扫描/无注解/无全局 DI、类型分发 Extension 注册、Go 泛型构造器、`context.Context`**」的 Go 形态。Embabel 的 planning 与 ToolLoop SPI 中已有框架无关部分,但默认完整平台仍是 Spring-shaped。当前二者已经不是“上游与待同步移植版”的关系,而是**共享规划 DNA 的两套独立 runtime**。

---

## 当前状态(2026-07-22)

### 拉取与验证

- 桌面 Embabel 仓库已从 `73952024d` fast-forward 到 `b1a923d1c`,工作区干净。
- Embabel 已发布 `v1.0.0`;当前 HEAD 是发布后的 `1.0.1-SNAPSHOT`。旧基线之后只有 8 个提交,非 POM 源码变化集中在 JSON/Mermaid 转义、日志、streaming reasoning fence 清理和平台服务类型放宽,**没有再次改写核心架构**。
- lynx 当前分支为 `codex/next-iteration` @ `cc4319c6d`;分析开始时 `agent` 目录没有未提交修改,本文只更新本对比文档。
- 在 `lynx/agent` 执行 `go test ./...`,全量通过。本文没有在本地运行 Embabel Maven 测试,不据此评价其当前构建状态。

### 演进轨迹

- lynx 最初移植落在 `cc918d3c9`(2026-05-02,`feat(agent): port embabel-agent GOAP runtime to Go`):首版已包含 A* planner、内存 blackboard、OODA process、顺序/并发 tick、Action retry、事件、fluent DSL、反射注册与 Awaitable HITL。此后 `agent` 目录已经历数百个提交,很多首版设计已被主动替换,不能再用“翻译后的 Embabel”解释当前代码。
- Embabel 从 `v0.4.0`(2026-05-18)到 `v1.0.0`(2026-07-19)的主要演进包括 Hybrid planner、独立 ToolLoop、progressive tools、streaming/thinking、native structured output、多模态、观测迁移、MCP/A2A/provider 扩展、skills/RAG 强化以及 Spring MVC/REST 运行面。
- lynx 同期把重点放在 runtime contract:uniform-cost GOAP、不可变 deployment identity、版本化 snapshot、原子 process-tree persistence、精确 ToolLoop continuation、子进程隔离、session FIFO、extension ownership、宿主工具发现、结构化失败与 wire/arch tests。
- 因而今天的分歧并不是“哪边提交更多”,而是优化目标不同:Embabel 向完整 Spring AI 产品平台扩张;lynx 向确定性、可嵌入、可恢复的中立执行内核收敛。

### 现状判断

| 维度 | Embabel | lynx `agent` | 判断 |
|---|---|---|---|
| 产品定位 | Spring AI 平台与完整生态 | 可嵌入的 Agent Runtime,能力拆在 sibling modules | Embabel 更 turnkey;lynx 边界更中立 |
| Agent authoring | 注解、扫描、参数反射、丰富 PromptRunner | 泛型 Action/Goal、显式组装、root facade | Embabel happy path 更短;lynx 更透明、可测试 |
| 规划正确性 | A* + 未满足条件数启发式,动态 cost 仍在起始状态求值 | 确定性 uniform-cost,在每条边的当前状态求 cost | lynx 对公开的任意有限非负 cost 契约更严格 |
| ToolLoop | 独立 SPI、顺序/并行、动态注入、progressive/nested tools、回调与转换器 | 事件 Runner、精确 checkpoint、资源键并发、panic containment、`PromoteTools` | Embabel 强在策略/工具树;lynx 强在确定性和恢复协议 |
| HITL / 恢复 | `Awaitable` + repository SPI;仓库内置实现以 live object/in-memory 为主 | 版本化 snapshot、原子进程树变更、pending tool call 原位续跑 | lynx 的 durable continuation 更完整 |
| 子 Agent | 通常复制父 blackboard 并继承 options | full / ambient / isolated 三档,默认 ambient-only | lynx 的状态所有权更安全 |
| 并发 | 可并发执行共享黑板上的计划 Action;工具批次并行 | process tick 串行;工具显式声明并发安全并按资源键互斥 | Embabel 更激进;lynx 更可复现 |
| 流式/结构化输出 | PromptRunner 原生覆盖 streaming、thinking、模板、多模态、校验及 native structured output | reasoning 已是协议值,有 `interaction.StreamCall`,但普通 `Prompt` 仍不自动流式,`PromptJSON` 较薄 | 这是 lynx 当前最明确的作者体验差距 |
| 发布成熟度 | `v1.0.0`,更大的 Spring 生态和测试体量 | 开发候选版,公共 API/wire fixture/arch tests 已成体系 | Embabel 外部成熟度领先;lynx runtime 契约更强调可演进 wire |

### 应该借鉴什么

1. **把 streaming 接进标准 `Prompt` / managed interaction 路径**:宿主仍决定模型与策略,业务 Action 不必手工包 `interaction.StreamCall`。
2. **加强结构化输出的统一作者路径**:复用已有 parser/output abstraction 接入 managed ToolLoop,补校验、错误诊断和可选修复;不要复制一条平行 converter 链。
3. **完善 progressive-tool authoring facade**:保留 `PromoteTools` 这个小而确定的 runtime primitive,在上层补 catalog/group/nested tool 的易用封装。
4. **补 per-tool / batch timeout 的标准策略入口**:保持取消由 `context.Context` 传播,但减少每个宿主重复包装。
5. **多态 typed-dataflow 按真实需求推进**:Embabel 的父/子类型自动连边很强,也会扩大条件空间并引入反射隐式性;在出现稳定业务案例前不追求表面 parity。

### 不应该回迁什么

- Spring/注解扫描/全局 DI 与 `ThreadLocal` 当前进程模型。
- 在共享父 blackboard 上并发执行整段计划 Action。
- 框架级 Action 自动重试。lynx 现在明确只执行一次,重试应由理解副作用和幂等性的具体 operation 决定。
- Embabel 当前 A* 的“未满足条件数”启发式、在 `startState` 上计算所有动态边成本,以及搜索后裁剪 action 的组合。

### 对历史判断的修正

- Embabel 的 ToolLoop 现在是公开、框架无关的 SPI,不能再描述成完全埋在 `LlmOperations` 内部。
- lynx `utility.GoalFirst` 与 Embabel `HybridUtilityPlanner` 的核心语义已经对应:先检查业务目标是否完成,未完成时选择高效用 action。双方的 `Supervisor` 仍不是同一层概念:Embabel 是 planner 路径,lynx 是 workflow 编译器。
- lynx 已删除框架级 Action retry 及 `RetrySafety` 元数据;旧的“lynx retry safety 更强”应改为“lynx 不替未知副作用做重放”。
- lynx 已增加 `PromoteTools`、`interaction.StreamCall`、版本化 ToolLoop checkpoint、原子进程树持久化、root/child session ownership 与同 session FIFO;旧对比不能再把这些列为缺口。

---

## 0. 模块对应关系

| Embabel 模块 / 概念 | 对应 lynx | 说明 |
|---|---|---|
| `com.embabel.plan.*`(planner-agnostic 规划核) | `agent/planning`(`Planner`/`Domain`/`Plan`/`WorldState`)+ `agent/core`(`Truth`/`Binding`/`Goal`/`Condition`) | lynx 把「纯规划类型」与「planner 实现」分包 |
| `com.embabel.plan.goap`(A* GOAP) | `agent/planning/goap` | |
| `UtilityPlanner` / `HybridUtilityPlanner` | `agent/planning/utility` 的 `Planner` / `GoalFirst` | `GoalFirst` 已对应 Hybrid 的 goal-satisfaction-first 语义;lynx 另有 `htn`、`reactive` |
| `com.embabel.agent.core`(Agent/Action/Blackboard/AgentProcess) | `agent/core` + `agent/runtime` | 定义(core)与运行时(runtime)分层 |
| `Ai` / `PromptRunner` / `LlmOperations` / `ToolLoop` | `agent/toolloop` + `agent/core` ProcessContext chat 面 | 两边现在都有显式 ToolLoop;Embabel 策略/注入面更丰富,lynx checkpoint/恢复协议更强 |
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
- **planner 名称集合不同,但 Hybrid 不再是实质缺口**。lynx 是 `goap`、`htn`、`utility`、`goal-first-utility`、`reactive`;Embabel 是 `GOAP`、`UTILITY`、`HYBRID`、`SUPERVISOR`。lynx 的 `GoalFirst` 与 Embabel `HybridUtilityPlanner` 都在真实业务 goal 满足时立即终止,否则继续选高效用 action。lynx 独有 HTN / reactive planner;Embabel 的 supervisor 是 planner/factory 路径,lynx 的 supervisor 是 workflow 组合器,二者不是同一抽象层。
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
type ActionMetadata struct { Name string; Inputs, Outputs []Binding; Preconditions, Effects ConditionSet; Cost, Value ScoreFunc; ToolGroups []ToolGroupRequirement; ... }
// NewAction[In,Out](...) 捕获 In/Out 的 reflect.Type → 生成 input/output Binding
func NewBinding[T any](name string) Binding            // reflect.TypeFor[T](),指针归一,pkgpath 限定
func NewOutputGoal[T any](config GoalConfig) *Goal      // precondition = "类型 T 的产物在黑板上"
```

**取舍与理由**:

- **显式装配,不引入扫描/注解/全局 DI**(agent 模块定位硬约束)。Embabel 的主流注解 authoring path 把 agent 声明为 Spring component,默认平台也由 Spring 构造。lynx 的 `Agent` 是 `NewAgent(AgentConfig)` 构造的只读值,组合根显式 append actions/goals。可嵌入、可单测、无容器依赖 —— 这正是它能当 lyra chat-turn 内核的前提。
- **泛型捕获类型,不靠反射发现方法**。lynx 只用 `reflect.TypeFor[T]()` **捕获 binding/schema 的类型名**,不用反射去 *发现* action/扫描注解/读参数名。`NewAction[In,Out]` 让 In→input binding、Out→output binding,类型信息端到端保留,但发现路径是编译期泛型,不是运行期反射 + Spring `ReflectionUtils.doWithMethods`。
- **多态能力并不对称(关键 tradeoff)**。Embabel 把返回类型的**子/父类型展开进 GOAP 条件键**,让 planner 能按类型层级链接。lynx 的 planner binding 与 `Blackboard.Lookup` 都按规范化后的**具体类型名精确匹配**;只有调用者已经拿到一个值以后,`Get[T]` / `Objects[T]` / `Last[T]` 的 Go 类型断言才可能匹配接口。因此“接口输入由具体实现自动满足”目前不是 planner/lookup 契约。好处是条件空间小且可诊断,代价是 Embabel 已有的多态 dataflow 自动连边能力目前缺失。
- **验证是显式方法 + `AgentValidator` SPI 两层**。`Agent.Validate()` 做结构检查(唯一命名、score 合法性、保守 goal 可达性);`AgentValidator` extension 加部署期自定义规则。Embabel 的 `AgentMetadataReader` **从不抛异常**(warn),坏注解不炸启动 —— 因为它在 Spring 启动期跑。lynx 在部署期显式 `Validate`,`errors.Join` 一次报全所有定义问题。

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

## 5. Action 从内部访问 LLM —— 丰富 PromptRunner vs 窄 ProcessContext + durable toolloop

**Embabel**:action 参数注入 `Ai`/`OperationContext`,fluent immutable `PromptRunner`,内部 `LlmOperations` 跑工具循环。

```kotlin
interface Ai { fun withLlm(LlmOptions); fun withDefaultLlm(); fun withLlmByRole(role); fun withAutoLlm(); ... }  // 全返回 PromptRunner
interface PromptRunner { fun <T> createObject(prompt, Class<T>): T; fun respond(...); fun evaluateCondition(...); withTool/withToolGroup/withGuardRails/... }
internal interface LlmOperations { createObject/generate/doTransform(messages, LlmInteraction, outputClass, agentProcess, action) }  // 解析 tool callback + 跑循环
```

**lynx**:`ProcessContext` 按面拆成多个窄文件,工具循环是**显式的 `toolloop.Runner`**。Embabel 当前也已经把 `ToolLoop` 提升为公开、框架无关的 SPI,因此现状差异不再是“显式 vs 隐藏”,而是两边公开状态机的能力重心不同。

- `ProcessContext` 的能力按文件分区:`process_context_chat.go`(模型调用)/ `process_context_control.go` / `process_context_interaction.go`(HITL)/ `process_context_tools.go` / `process_context_usage.go` —— 不再用一个接口覆盖所有作者能力。
- 工具循环 = `toolloop.Runner`:消费 `chat.Model` + `Request` + `ToolResolver`,emit `iter.Seq2` 的 `Event`(model/tool/pause/resume)。**默认互斥、显式 opt-in 的有界资源键并发、无自动 retry、可 checkpoint/resume**。执行可并发,但 `ToolResult` event 与下一轮 model request 始终按模型原始 tool-call 顺序提交,因此 chat-history/cache 输入稳定。checkpoint v2 按 call 保存状态与 `NextResult`;`PromoteTools` 允许在 loop 中途公布此前已可解析但未广告的工具。协议值(model request/response)与运行时状态(可执行工具、pause)**分离**,不往 provider `Response` 塞运行时状态。
- model 选择走 process-scope `core.ChatProvider` / `ChatCapability`;`routing.ModelRanker` 只负责给 `agent × goal` 路由候选打分。reasoning 是 `chat.PartReasoning` 协议值;`interaction.StreamCall` 已能消费 `chat.Streamer` 并累积 delta,但普通 `ProcessContext.Prompt` 仍只调用 `chat.Model`。`PromptJSON[T]` 当前主要是 schema instruction + `json.Unmarshal`,作者体验明显薄于 Embabel 的 PromptRunner/native structured output。

**取舍与理由**:

- **两边工具循环都已是一等对象,但 durable 边界不同**。Embabel 的 `ToolLoop` 强在 sequential/parallel mode、动态 tool injection、progressive/nested tool、inspector/transformer 与超时/空响应策略。lynx 的 `toolloop.Runner` 强在 per-call checkpoint、严格 tool-call correlation、按模型顺序提交、资源键冲突控制和 pending call 原位 resume。这是能力侧重差异,不再是 API 可见性差异。
- **窄面替宽 fluent gateway**。`Ai` / `PromptRunner` 聚合 with-llm/with-tool/createObject/evaluateCondition/streaming/thinking 等能力,带来很好的 happy path,也形成较宽的作者 API。lynx 按 ProcessContext 的职责切文件、切窄面 —— chat / control / interaction / tools / usage 各管一域(SRP)。
- **model 选择与 intent 路由是两条独立边界**。`routing.Ranker` / `ModelRanker` 选择 agent goal;`core.ChatProvider` 为某个 process 提供实际 `ChatCapability`,app 层再承载 BYOK、provider/model 名称与角色配置。这样不会让“选择做什么”和“选择用哪个模型做”共享一个含混抽象。

---

## 6. 扩展机制 —— Spring bean 扫描 vs 类型分发 Extension 注册

**Embabel**:能力 = Spring bean;发现 = `BeanPostProcessor` + component scan;model provider = 命令式 `registerSingleton`。

- `@Agent`/`@EmbabelComponent` 是 meta-`@Component`;`ScanConfiguration` 对 8 个包 `@ComponentScan` + `@ConfigurationPropertiesScan`;`DelegatingAgentScanningBeanPostProcessor` 攒 bean 到 `ContextRefreshedEvent` 再 `AgentMetadataReader` 读进 `agentPlatform.deploy(...)`。
- model provider:每 provider 一个 `@AutoConfiguration`,`@Bean : ProviderInitialization` 读 `models/<provider>-models.yml` 后**命令式 `ConfigurableBeanFactory.registerSingleton(name, llm)`**(21 处);`modelProvider` 再 `getBeansOfType` 收回来。
- Spring 耦合普查(旧基线主源码):27 `@AutoConfiguration`、47 `@Bean`、24 `@Import`、22 `@ConfigurationProperties`、21 `registerSingleton`、4 `EnvironmentPostProcessor`……完整 AgentPlatform 的默认构造、发现和 provider 装配仍以 Spring DI 为内禀结构;这不否认 planning / ToolLoop 等局部 SPI 可以独立讨论。

**lynx**:能力 = `core.Extension`;发现 = **运行时按类型断言收集**;稳定构造依赖 = `Config` 显式字段。

```go
// agent/runtime —— 行为插件机制是 core.Extension 注册表
// 一个泛型 collector 在 dispatch 时按接口类型断言收集:event listener、action/tool middleware、
// AgentValidator、GoalApprover、tool-group resolver、id generator、blackboard prototype、planner …
// per-process Extension 与 engine-scope 合并;稳定依赖(chat/ProcessStore/root+child SessionStore/snapshot policy)是 Config 字段,不进注册表
```

**取舍与理由**(这是**根本结构差异**):

- **agent 是可嵌入 framework,不是需要容器的应用**。Embabel 的默认完整使用路径采用 Spring 容器、组件扫描、Boot 自动装配与 Environment 机制。lynx agent 由组合根显式 `New(Config)` 构造,能被 lyra 后端当库嵌进去跑一次 chat turn。要得到一个不依赖容器生命周期的完整 runtime,Go 移植必须重新设计而非逐类翻译 —— lynx 就重新设计了。
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

lynx 还把部署身份写进 durable contract:`DeploymentRef{Name,Version,Digest}` 随 snapshot 持久化,恢复时能校验精确部署版本。Embabel `DefaultAgentPlatform.deploy` 的核心仍是按 agent name 更新平台注册表,process 持有 Agent 对象,没有等价的不可变 digest/version pin。对 crash recovery、replay 和滚动升级而言,这是 lynx 运行时契约上的重要优势。

---

## 9. 进程生命周期 / HITL / 持久化 —— repository update vs type-tagged snapshot

**Embabel**:`AgentProcess` 状态机(`RUNNING/WAITING/PAUSED/STUCK/COMPLETED/...`),`AgentProcessRepository`(默认 in-memory,持久版 = 跨重启可恢复),每 tick `repository.update`;`ProcessOptions.contextId` 从 `ContextRepository` rehydrate 黑板;`ephemeral=true` 契约 = 从不持久、无 wait state、不能派生子进程;`ThreadLocal<AgentProcess>` 传当前进程;`Budget` 早终止(maxActions/maxTokens/cost)。HITL 靠 `ProcessWaitingException` + `Awaitable`(`ConfirmationRequest`/`FormBindingRequest`)。

**lynx**:`Engine` + `Deployment` + process registry;`StatusWaiting` 是一等状态;`Resume`/`Continue` 记录回复后**从精确挂起点重入**;durable 靠 `ProcessStore`、分离的 root/child `SessionStore` + **type-tagged snapshot codec**;当前 `ProcessSnapshot` schema 为 v5、ToolLoop checkpoint 为 v2;budget 树跨子进程;`stop_policy`/`stuck_policy`/`termination` 分文件。

**取舍与理由**:

- **durable resume 更精细**。Embabel 每 tick 整体 `repository.update`,持久 repo 即可跨重启续跑 —— 粒度是「整个进程状态」。lynx 用 **type-tagged snapshot**(chatInput round-trips、`LastWorld json:"-"` 派生、protected 在 re-run 时 re-bind、HITL park/interrupt 靠 atomic `Consume` 幂等 `DELETE...RETURNING`),且 **HITL resume 在 pending tool call 处续跑,跳过已完成的模型轮和工具**([[project_durable_resume_design]] + [[project_hitl_resume_at_pending_call]])。这是 lyra 生产级 chat turn 恢复的需求,比 Embabel 的整体 update 更外科手术。
- **transient 状态显式排除持久化**(§4)——handle/client/channel 不进 snapshot,对应 Embabel 的 `ephemeral` 但粒度到单个 binding。
- **Session identity 是领域合同而不是 `contextId` 字符串**。Session 校验 ID/parent/Agent/audit time；child adapter 必须 round-trip UserID、AgentName、时间与 metadata。同一 child ID 不能静默换 parent/user/Agent。Embabel 的 `ProcessOptions.contextId` 负责 rehydrate context，但没有这层 root/child persistence ownership 分离。
- **同一 root session 的 turn 显式 FIFO**。同 session 不会并行改写历史,不同 session 仍可并发;root session 与 delegated child session 的存储合同分离。Embabel 有 context/process identity,但没有等价明确的 session-turn serialization 与 child lifecycle ownership 边界。
- **嵌套 AgentTool 的暂停是父子树的一部分**。lynx 以原始 `tool_call_id` 关联 child,child 暂停会把 suspension 推进父进程的 managed interaction,恢复时继续原 child。Embabel Subagent 遇到等待状态时主要把等待信息返回为工具文本,另走 process/callback 机制继续,没有同样透明的父调用栈续跑。
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

- **HTN / reactive planner**;Embabel 的 Hybrid 已由 `GoalFirst` 覆盖核心语义,Supervisor 与 lynx workflow supervisor 位于不同抽象层。
- **workflow 组合器**(scatter-gather/repeat-until/consensus/supervisor)编译回普通 agent(§10);Embabel 靠 GOAP + `@State` 状态机表达。
- **reader/writer 分离黑板**(结构性 ISP,OBSERVE 不能 mutate)(§4)。
- **类型分发 Extension 注册**(无 Spring、可嵌入、无 string-key)(§6)。
- **外科手术式 durable resume**(type-tagged snapshot、HITL 在 pending tool call 处重入、park/interrupt 幂等)(§9)。
- **三档子进程委派梯度**(全继承 / 仅 ambient / 全空,默认仅 ambient 以免预满足子 agent 产出目标)([[agent_delegation_spawn_semantics]])。
- **框架中立的核**(无 DI 容器依赖)—— 可作为 lyra chat-turn 执行内核嵌入。
- **未知副作用不由框架自动重放**:lynx Action runtime 只执行一次,重试由理解具体副作用与幂等性的 operation 自己实现。Embabel 的 `ActionQos` 默认 `maxAttempts=5`、`idempotent=false`,而 `idempotent` 当前没有进入实际 retry 决策,因此 lynx 的默认边界更保守。
- **不可变部署身份**:`DeploymentRef` 的 name/version/digest 随 snapshot 固化,恢复不会静默切到同名新 Agent。
- **显式 session turn FIFO 与 root/child store ownership**:并发和持久化责任写进 runtime contract,不是宿主约定。
- **arch test + wire fixture**(与 core 同规格)冻结 agent 公共面 / snapshot wire。

---

## 一句话总结

> Embabel 证明了一条正确的路 ——「**GOAP planner 驱动、replan-every-tick、blackboard 中枢、typed-dataflow 推导条件、三值逻辑**」是比 ReAct-loop 更可解释、更可扩展的 agent 范式。lynx `agent` **接受了这套规划思想的主干**(§1),但把 Embabel 完整 AgentPlatform 的 Spring-shaped 工程形态 —— 注解扫描、bean 发现、`registerSingleton` model wiring、SpEL 条件、`ThreadLocal`、宽 Prompt gateway —— 换成「**显式装配、可嵌入、类型分发 Extension、Go 泛型构造器、`context.Context`、durable toolloop、原子进程树恢复**」的 Go 形态。今天二者已经不是移植追赶关系:lynx 更强在框架中立、严格最小成本 GOAP、不可变部署身份、确定性并发、session/child ownership 和精确 durable continuation;Embabel 更强在 Prompt authoring、多态 dataflow、动态工具树、Spring AI 集成与发布生态。后续应以 Embabel 为设计素材来源,而不是以表面 API parity 为目标。
