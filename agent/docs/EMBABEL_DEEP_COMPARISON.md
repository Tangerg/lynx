# 深度对比：lynx/agent vs embabel-agent（2026-05 重审）

> **基线**
> - **lynx** HEAD `e480bc7`（branch `lyra`，2026-05-29）；`agent/` 子模块 **~11.9k Go LOC / 145 文件 / 12 个内部包**：`core/`（3.5k）、`runtime/`（4.1k）+`runtime/autonomy/`（0.6k）、`planning/`（0.4k）+`planning/planner/{goap 0.5k, htn 0.3k, reactive 0.2k, utility 0.2k}`、`event/`（0.6k）、`hitl/`（0.2k）、`toolpolicy/`（0.2k）、`workflow/`（1.0k）。仅依赖 `core`/`mcp`/`pkg` + `otel`/`uuid`/`go-sdk`。
> - **embabel** HEAD `9dc8a897`（main，2026-05-27）；~43 个 Maven module / ~240k LOC（Kotlin 主导），其中 `embabel-agent-api` 自身 **150,997 LOC**。
>
> **方法论**：本文由 7 个并行深读 agent 产出，每个 agent **同时通读两边源码**（不是读文档），对应 7 个维度：核心抽象 / 规划 / 运行时引擎 / Extension-SPI / Tool-ToolLoop / HITL-Autonomy-Workflow / 生态-事件-观测。所有 file:line 级断言均来自实际源码；其中 3 条最关键的"移植保真度"断言（NetValue 公式、retry 清 effect、condition And cost）已二次核对。
>
> 上一篇 `EMBABEL_COMPARISON.md`（基线 `c8797cf`，包布局还是 `plan/` + `service` 切片）已过时——本文按当前 `planning/` 布局重写，并修正了若干当时不准确的结论。

---

## 0. TL;DR

**心智模型完全一致**：GOAP planner + 黑板（blackboard）+ OODA tick 循环 + Action/Goal/Condition 三元组 + 三值 Determination。**工程形态截然相反**：

- embabel = **Spring Boot 应用 framework**：~43 module、autoconfigure + 21 starter、自带 Shell / A2A / MCP-server / REST 前端、Micrometer 观测栈、5-module RAG、Skills 引擎、ONNX。一引入就是可启动的 agent 主机。
- lynx/agent = **可嵌入的 Go 库**：~12k LOC，`Platform.New(...)` + `agent.New(...).Build()`，没有 DI 容器、没有前端、没有持久化后端；这些由 sibling 模块（`models`/`vectorstores`/`rag`/`chatmemory`）和 `lyra` 后端承担。

### 总评分卡（7 维度 × 两个轴：抽象整洁度 / 原始能力）

| 维度 | 抽象整洁度赢家 | 原始能力赢家 | 关键理由 |
|---|---|---|---|
| 核心抽象 | **lynx** | embabel | lynx：2 方法 `Action`、ISP 拆 Blackboard、Process HAS-a、`Determination` Unknown=0、context 替 ThreadLocal。embabel：多输入反射、`DataDictionary`/`DomainType` schema 层、注解模型 |
| 规划 | **lynx** | 平手偏 lynx | lynx 独有 HTN + 后向 STRIPS 剪枝 + reactive 0-progress 守卫；embabel 独有 unknown-condition 惰性分支求值 |
| 运行时引擎 | **lynx** | embabel | lynx：generic extension dispatch、ctx 传播、lock-free signals、真 goroutine 并行、deploy-time 可达性校验。embabel：OperationScheduler、强制持久化 repository |
| Extension/SPI | **lynx** | embabel | lynx：1 个 marker + `collectExtensions[T]` 收掉 embabel 8+ 个 Spring SPI。embabel：spi/loop 整棵树、OperationScheduler、validation manager |
| Tool/ToolLoop | embabel | **embabel（决定性）** | embabel ToolLoop 是可插拔 SPI（injection strategy / 错误策略 / 并行 / inspector）；lynx tool loop 极薄（但流式更强）。embabel 还有 artifact + 内置工具库 |
| HITL/Autonomy/Workflow | **lynx** | 各有所长 | lynx：单泛型 `TypedRequest[P,R]`、narrow platform 接口、`LLMPlanRanker`、Sequence/Parallel/Loop-over-subagent。embabel：`ux/form` 表单层、`SupervisorAgent`、best-of-N |
| 生态/事件/观测 | n/a | **embabel（决定性）** | embabel ~43 module + ~32 事件 + Micrometer + 前端全家桶；lynx 1 module + 16 事件 + 直连 OTel，靠 sibling 模块补广度 |

**一句话**：lynx 在**抽象卫生**上几乎全面领先（这是 CLAUDE.md 设计纪律的直接兑现），在**规划算法**上反超 embabel；embabel 在**原始能力广度**上领先，集中在它刻意做厚的 ToolLoop、表单、SupervisorAgent、观测框架、协议前端这些层——而 lynx 恰恰把这些刻意做薄或外移。

### lynx 真·反超 embabel 的点（本次核实）

1. **HTN planner 一等公民**（`planning/planner/htn/`，Library/Task/Method 递归分解，深度上限 64）—— embabel 全仓库 0 处 HTN（grep 确认）。
2. **GOAP 后向 STRIPS 相关性剪枝**（`planning/planner/goap/relevance.go`，真·不动点回归，A* 搜索**前**剪枝）—— embabel 只有 post-hoc 的 `OptimizingGoapPlanner.prune`（要先跑完整 A* 才能筛死动作），无搜索期剪枝。
3. **reactive planner 0-progress 守卫**（`reactive.go:116`：`progress==0 → continue`）—— embabel `UtilityPlanner` 无此守卫，NIRVANA goal + tick 循环会死循环（其自家 `HybridUtilityPlanner.kt:58` 注释承认）。
4. **`LLMPlanRanker`**（LLM 评估候选 plan）—— embabel 无任何 plan-ranking 抽象。
5. **`collectExtensions[T]` 统一 extension dispatch**：1 个 marker 接口收掉 embabel 8+ 个独立 Spring SPI，加新能力不改 dispatch loop（OCP）。
6. **ISP 拆 Blackboard**（Reader / Writer / Spawn）+ **Process HAS-a Blackboard**（~22 方法 vs embabel `AgentProcess` 50+ 方法的 god interface）。
7. **流式 tool loop 自驱**（`executeStreamRecursively`）—— embabel `LlmMessageStreamer` 文档明说流式下不能自驱 loop、只能观测。
8. **workflow 层覆盖 sub-agent 组合**：`Sequence`/`Parallel`/`Loop` 在 embabel 无对应（embabel 的 workflow 只组合 closure，不组合 sub-agent），且 lynx 用 `SpawnChildFresh` 做分支隔离。
9. **deploy-time 可达性校验**（`checkGoalsReachable`）—— embabel `deploy` 是裸 map put，不可达 goal 要到首次 tick STUCK 才暴露。
10. **更窄的耦合**（autonomy 包定义 2 方法 `platform` 接口 + 编译期 tripwire）—— embabel `Autonomy` 直接抱整个 `AgentPlatform`。

### lynx 真·gap（本次核实，按 ROI 排序）

| # | Gap | 严重度 | 是否 by-design |
|---|---|---|---|
| 1 | **持久化 SPI**（`AgentProcessRepository` / `ContextRepository` 可换后端 + eviction）| 高 | 部分（外移给 `chatmemory`/`lyra`），但 agent 内完全空白 |
| 2 | **ToolLoop 健壮性策略**（max-iteration cap / ToolNotFound 自纠 / EmptyResponse 重试）| 高 | 否——是真缺的安全网，弱模型场景会咬人 |
| 3 | **retry 不清 effect condition**（潜在正确性 bug，见 §9.2）| 中-高 | 否——疑似漏移植 |
| 4 | **`SupervisorAgent`**（LLM 编排多 agent / ReAct loop）| 中 | 是——lynx 刻意 planner-only |
| 5 | **`ux/form` 表单层**（Form + 11 控件 + 反射 binder）| 中 | 部分——UX 出 agent 范围，但服务端表单绑定真没有 |
| 6 | **NetValue 丢了 actionsValue 项**（保真度 gap，见 §9.1）| 中 | 否——注释自称"embabel 启发式"但公式不全 |
| 7 | **per-LLM-call / tool-call / embedding 事件**（agent 层无 LLM 事件）| 中 | 部分——在 `models` 层，但 agent 单独拿不到 tool 遥测 |
| 8 | **OperationScheduler**（delayed / scheduled 动作执行 + rate limit）| 低-中 | 是（YAGNI），但要调度就真没有 |
| 9 | **内置工具库**（file/math/process/blackboard/progress/communicate）| 低-中 | 部分——黑板工具与 planner 哲学冲突，但 file/math 无理由不加 |
| 10 | **AgentValidationManager 多层 + LLM 校验**（结构/可达性分型 + prompt 生成）| 低 | 否——但单 `AgentValidator` extension + 内建 reachability 已覆盖大半 |

---

## 1. 哲学 / 定位

| 维度 | embabel-agent | lynx/agent |
|---|---|---|
| **角色** | Spring Boot **应用 framework** | 可嵌入的 Go **库** |
| **分发单元** | ~43 Maven module（core + 7 autoconfigure + 21 starter） | 1 个 Go 包，复用 sibling lynx 模块 |
| **DI / 组装** | Spring `@Bean` + `@Conditional*` + `@Primary` autowiring | `PlatformConfig.Extensions []Extension` slice + 显式构造 |
| **自带前端** | Shell REPL + A2A server + MCP server + REST（autoconfigure 触发） | 无；由 `lyra` 后端承担 RPC/HTTP |
| **持久化** | `AgentProcessRepository` + `ContextRepository` SPI（in-mem 默认 + Spring Data 换后端）| in-memory registry + 可选 `ProcessStore` 快照 sidecar |
| **并发模型** | kotlinx.coroutines + `CompletableFuture` | 原生 goroutine + `context.Context` + `sync.WaitGroup`/`errgroup` |
| **当前进程传播** | JVM `ThreadLocal<AgentProcess>` | `context.Context`（`core.WithProcess`/`ProcessFrom`）|
| **观测** | Spring Observation + Micrometer + `@Tracked` AOP + MDC | 直连 OTel API（`otel.Tracer("lynx/agent")`）|
| **LLM 抽象** | `LlmOperations`/`LlmService`/`ModelProvider` SPI（在 api/core 内）| 不在 agent 内抽——经 `ServiceProvider` 暴露 `chat.Client` |
| **核心 LOC** | api 自身 ~151k | agent ~12k |

embabel 押 "framework"——每个定制点是一个 Spring bean，autoconfigure 把一切接好。lynx 押 "library kit"——agent runtime 是个 `import` 的包，认证/传输/持久化/部署交给调用方。**不是优劣，是用例分布判断**：要"一台机器一个 agent 服务带 REPL / 多协议 / 完整观测"选 embabel；要"在 Go 微服务里嵌一段 planner-driven agent 逻辑、host 已有自己的观测和持久化栈"选 lynx。

---

## 2. 核心抽象建模

### 2.1 Action 形态：5 契约组合 vs 2 方法薄接口

**embabel** `Action` 是 5 个接口的交集（跨 3 个包层叠），一个具体 Action 要实现 ~12 个成员：

```kotlin
interface Action : DataFlowStep, ConditionAction, ActionRunner, DataDictionary, ToolGroupConsumer
// Step→value; Action(plan)→cost/netValue; ConditionStep→preconditions/isAchievable;
// AgentSystemStep→inputs; DataFlowStep→outputs; ConditionAction→effects;
// ActionRunner→execute/referencedInputProperties; DataDictionary→domainTypes/filter/relationships;
// ToolGroupConsumer→toolGroups
```

**lynx** `Action` 是 2 方法接口 + 元数据集中到一个 struct：

```go
type Action interface {
    Metadata() ActionMetadata
    Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}
// ActionMetadata: Name/Description/Inputs/Outputs/Preconditions/Effects/CanRerun/QoS/ToolGroups/Cost/Value/OutputBinding/ClearBlackboard
```

typed body 走泛型 `NewAction[In, Out]`，`computePreconditionsAndEffects`（action_typed.go:164）自动把每个 input 升为 True precondition、每个 output 升为 True effect，并切 `hasRun_<name>`。

- **In/Out 绑定来源**：embabel 用 JVM 反射读方法签名（注解模型）；lynx 用 Go 泛型 + `reflect.TypeFor[T]()`，编译期类型安全。
- **多输入**：embabel 反射可绑 N 个参数；**lynx 只自动绑 `inputs[0]` 这个泛型 `In`，多余的要手 `core.Get[T]` 抓**（action_typed.go:68）——Go 单类型参数的硬限制。
- **self-describing schema**：embabel `DataDictionary` mixin 让 Action 自描述 `DomainType`、`referencedInputProperties`（property 级数据流）；lynx 全砍。
- **类型键**：lynx `IOBinding{Name, Type}` 的 Type 用 `PkgPath + "." + Name`（包限定，抗同名冲突）；embabel 用裸 `Class.name`。

**裁决**：lynx 极致更干净（2 vs 12 成员 + 编译期泛型安全）；embabel 原始能力更强（多输入原生、Action 自带 schema 层）。**保真度差异**：lynx 砍掉 `DataDictionary`/`DomainType` 整个 schema 子系统、`readOnly` 标志、property 级数据流；新增 `ClearBlackboard` 标志。

### 2.2 Process IS Blackboard vs Process HAS Blackboard

embabel `AgentProcess : Blackboard, Timestamped, Timed, OperationStatus<...>, LlmInvocationHistory, EmbeddingInvocationHistory` —— 继承 ~25 个 Blackboard 成员 + lifecycle（tick/run/kill）+ cost 矩阵（own/subtree/LLM/total）= **50+ 方法的 god interface**，当前进程走 `ThreadLocal`。

lynx `Process` 是刻意收窄的 **读+控制面**（~22 方法）：`ID/ParentID/Status/Goal/Blackboard()/Options/Failure/LastWorldState` + 控制 `TerminateAgent/Action/ToolCall` + `AwaitInput` + `RecordUsage/Usage/LLMInvocations/EmbeddingInvocations`。**状态机（tick/run/kill）藏在 runtime 的 unexported `*AgentProcess` 上，不在接口里**；当前进程走 `context.Context`。

**裁决**：lynx 压倒性更干净——"HAS-a" + 把 lifecycle 藏到 unexported runtime 类型是教科书级 ISP/SRP，embabel 让 `AgentProcess` 同时是 Blackboard + lifecycle + cost。embabel cost 粒度更细（own/subtree/LLM/total 四象限），lynx 收敛成 subtree 聚合的 `Usage()` + 两个 typed invocation slice。lynx 新增 `TerminateToolCall`（tool-call 级取消）。

### 2.3 Blackboard：单巨接口 vs ISP 三拆

embabel `Blackboard : Bindable, MayHaveLastResult, HasInfoString`——单一树 ~25 成员，但**查找语义极强**：JVM 超类型匹配（`getValue("it","Animal")` 命中 `Dog`）、`DomainType` label 层级、tagged-map 匹配、`Aggregation`（"Megazord"）反射拼装。

lynx 三拆：`BlackboardReader`（8 方法）/ `BlackboardWriter`（8 方法）/ `Blackboard = Extension + Reader + Writer + Spawn() + Clear()`。`Bind(value)` 做 embabel-0.4 双绑（`"it"` + snake_case 派生键，`UserInput`→`user_input`）。**只做精确 type-name 匹配**，无超类型/label/Aggregation。

**裁决**：lynx 靠 Reader/Writer ISP 拆分实质更干净——`ConditionEnv` 拿 `BlackboardReader`，编译期保证"condition 在 OBSERVE 期不能写"，embabel 无此保证。embabel 查找能力远更强（超类型/label/Aggregation 自动拼装），lynx 全砍。lynx 把 Blackboard 做成 `Extension`（prototype 模式，注册的黑板按 `Spawn()` per-process 克隆，取代 embabel 的 `BlackboardProvider` factory）。

### 2.4 Determination 三值逻辑

| | embabel | lynx |
|---|---|---|
| 表示 | `enum {TRUE, FALSE, UNKNOWN}` | `int8`，**Unknown=0**（零值友好，未 init 的 map 项天然读作 Unknown，GOAP 依赖此） |
| 代数位置 | 内联在每个 Condition 类的 evaluate 里 | `Determination.And/Or/Not` 是**类型上的方法**，函数组合复用之 |
| And cost | `minOf(a,b)` | **`a+b`（求和）** |
| 评估面 | `OperationContext`（胖接口）| `ConditionEnv{Process, BlackboardReader}`（窄，ISP）|

**裁决**：lynx 代数更干净（方法可复用可测，Unknown=0 是 JVM enum 复制不了的 Go idiom）。**行为差异**：lynx `And` cost 用 `left+right`，embabel 用 `minOf`——短路下 embabel 更准，lynx 高估；lynx 砍掉 embabel 的 `inv()`/`UnknownCondition` 算子，新增结构化 `PromptCondition`（LLM-as-judge + 可插 `ConditionParser`）。

### 2.5 其它核心类型速览

- **Goal**：近乎对等，双方都从 `pre + inputs` 合成 True precondition。lynx `GoalExportFor[In]` 捕获 typed 零值样例驱动 JSON-schema + MCP-tool-call 期 typed unmarshal，**比 embabel `startingInputTypes: Set<Class>` 更实用**。lynx Goal 是 value struct（不是 planner 接口）。
- **Agent**：lynx 用 `sync.OnceValue` 缓存 condition（embabel 每次 recompute getter）；lynx 独有 `PlannerName`（embabel 硬连 condition planner）；lynx 有 `ValidateAgent` fail-fast；砍掉 embabel 的 `provider`/`opaque`/merged `domainTypes`/注解模型。
- **ProcessContext**：lynx 三区字段分区（per-process / platform-wired hooks 私有 / per-tick scratch），hooks 私有藏在 typed 方法后（`Chat()`/`ChatWithActionTools()`/`ExecuteSafely` panic 守卫）；embabel 用 Kotlin `by` 委托把 context 变成 `LlmOperations + AgenticEventListener`（更短但更漏）。
- **IOBinding/DomainType**：这是 embabel 远更强、lynx 远更简的维度——embabel `DomainType`/`DataDictionary`/`PropertyDefinition`/`Cardinality` 是完整的内存 schema/知识图谱层（relationship、cardinality、YAML 动态类型、classpath 后代扫描）；lynx 只留 planner 严格需要的类型键，`DomainType` 近乎 vestigial（`IsSealed`/`Parents` 字段存在但无 `DataDictionary` 消费）——**这是 lynx 刻意剪枝的证据**。

---

## 3. 规划子系统

### 3.1 Planner 接口

embabel `Planner<S,W,P>`——3 泛型参 + 5 方法（含 stateful `worldState()` 暴露内部 determiner + `prune()`）。lynx `Planner`——**1 方法 + stateless**：

```go
type Planner interface {
    core.Extension  // Name() string
    PlanToGoal(ctx, start core.WorldState, system *System, goal *core.Goal, options Options) (*Plan, error)
}
// PlansToGoals / BestValuePlan / Prune 是包级函数，不是接口方法
```

lynx 的 planner stateless（start 状态每次传入，并发安全 by construction），单方法 ISP 完胜；embabel 泛型买到编译期类型钉死但逼每个 planner 实现 5 方法（utility planner 的 `prune` 是 no-op override）。replan 黑名单：embabel `ReplanRequestedException` 控制流异常携 `BlackboardUpdater`；lynx 返回 `*core.ReplanRequest` 值，`errors.AsType` 检出——同语义不同传输。

### 3.2 GOAP A*

双方都是前向 A*、**同款可纳启发式**（未满足 goal precondition 计数）、**同款 10000 iter 上限**、cost 都对 start 状态采样、都有 backward+forward 两道优化。差异：

- **闭集重开**：embabel 发现更便宜路径会重开闭节点（`closedSet.remove`）；lynx 不重开（启发式 consistent 故首次弹出即最优，安全）。
- **forward 优化护栏**：embabel forward 优化若破坏可达性会**回退原 plan**；lynx `forwardOptimize` 无此护栏（但其规则更窄——只删不改 hashkey 的 no-op 动作，实践安全）。
- **🔑 embabel 独有：unknown-condition 惰性分支**：`OptimizingGoapPlanner.planToGoal`（OptimizingGoapPlanner.kt:41-56）若 start 恰有 1 个 Unknown condition，生成 True/False 两个 variant 各自规划，**若 plan 不同就真去 evaluate 那个 condition** 再 commit；`TODO` 留了 >1 unknown。**lynx 无对应**——直接把 world state 喂 A*，Unknown 当普通不匹配。

### 3.3 后向 STRIPS 相关性剪枝（lynx 独有，已核实）

lynx `relevance.go` 是真·不动点 STRIPS 回归：seed `needed` = goal preconditions（`needTuple{key,value}` 正确建模同一 key 在不同位置需不同值），循环"动作的 effect 命中 needed 则相关、加入后其 preconditions 扩充 needed"直到不动点；在 A* 入口**搜索前**剪掉无关动作（astar.go:88）。embabel **无任何对应**——它唯一的 `prune` 是 post-hoc 的（要先对每个 goal 跑完整 A*，再筛"出现在某 plan 里"的动作，是 deploy-time 死动作诊断，不缩小搜索前沿）。lynx 把 embabel 那个 post-hoc 行为单独移植为 `planning.Prune`，故 lynx 有**两套**机制，embabel 只有一套。

### 3.4 HTN（lynx 独有，已核实）

lynx `htn` 包：`Task`（primitive 包一个 Action XOR compound 含多 Method）、`Method`（preconditions + 有序 subtasks）、`Library`（具名任务注册表）、`decompose`（递归展开穿 world state，深度上限 64 防环，soft fail 回溯下一 sibling method，输出 flatten 后线性动作列表）。`grep -rli "htn\|hierarchical task network"` across embabel `.kt` = **0 命中**。embabel planner 家族仅 `AStarGoap`/`Utility`/`HybridUtility`/`SUPERVISOR`(映射到 GOAP)。

### 3.5 Utility / Reactive 与"死循环"断言（已核实，需精确表述）

embabel `UtilityPlanner.planToGoal` 本身**单步前瞻、不内部循环**（UtilityPlanner.kt:86：1 步能到就返回，否则 null）。"可能死循环"来自 **NIRVANA goal 路径**：对不可满足的 NIRVANA goal，它返回"net value 最高的可用动作，**不检查是否推进任何东西**"（UtilityPlanner.kt:52-64）。配合 host 每 tick 重调 planner，NIRVANA + 非自禁动作 = 永远开火。embabel 靠 planner **外部**缓解（`canRerun=false` 锁 + runtime replan 黑名单）。

lynx `reactive.Planner` 在 **planner 内**堵上这个洞（reactive.go:116：`progress==0 → continue`），按"effect 关闭多少个未满足 goal precondition"打分，0 进展直接拒绝、返回"卡住"而非机会主义 no-op。lynx `utility.Planner` 则**忠实移植** embabel 的可循环行为（含 NIRVANA 无守卫，utility.go:127）。故 lynx 两者都有：忠实版 `utility` + 安全版 `reactive`。

### 3.6 其它

- **Plan netValue**：embabel = `goal.value + actionsValue − cost`（3 项）；**lynx = `Value − Cost`（2 项，丢了 actionsValue）**——见 §9.1 保真度 gap。lynx `SortByNetValueDesc` memoize key 是 perf 改进。
- **WorldState**：embabel 有 `variants()`/`withOneChange()`/`unknownConditions()` 喂 unknown 分支；lynx 无（不做 unknown 分支），但 lynx eager-hash（并发读不竞争 lazy init）+ apply 时跳过 Unknown effect（"无信息不该覆盖确定值"）是 embabel 没有的语义。
- **planner 选择**：lynx string-keyed extension registry（`PlannerName` 匹配 `Name()`，默认只 `goap`/`reactive`，`htn` 需用户给 Library 故非默认）——更 OCP；embabel `PlannerType` enum + `DefaultPlannerFactory` when 分发——更封闭但 enum 携 `needsGoals` 校验元数据，lynx string 方案无处放。

---

## 4. 运行时 / 执行引擎

### 4.1 Platform 与 deploy 校验

lynx `Deploy` 做 **3 层**：`ValidateAgent`（结构）→ `checkGoalsReachable`（保守一步生产者扫描：goal 每个 True precondition 验证有动作 effect 产出或 input binding 形似）→ `runAgentValidators`（每个注册 validator，首错否决）。embabel `deploy` 是裸 `agents[name]=agent` + 事件，**三者皆无**——不可达 goal 要到首 tick 返回空 plan → STUCK 才暴露。**lynx 的 deploy-time 可达性校验是 embabel 没有的能力**。

### 4.2 OODA tick 循环

两边结构等价（lynx run.go:42 注释明说"mirror embabel's AbstractAgentProcess.run loop"）：makeRunning 闸 → goal 存在性检查 → 循环 {early-term → tick → 非 Running 则 break}。tick = OBSERVE（determineWorldState → set lastWorldState → emit ReadyToPlan）→ plan → act。差异：

- lynx 每轮 `ctx.Err()` 检查（Go 取消 idiom）；embabel 靠 `InterruptedException`。
- **replan 黑名单清除时机**：embabel 任何成功规划后清黑名单、plan-not-found-with-blacklist 时 fallback 无黑名单再规划——**清得更勤**；lynx exclusions 只在 `handleStuck` 的 StuckPolicy-replan 路径清——**更黏**（反复 replan 会累积 exclusions 直到完全卡住才清）。**值得标注的行为差异。**
- **lynx 独有分支**：planner *报错* → FAILED（区分"planner 炸了"vs"无 plan = STUCK"）；embabel `bestValuePlanToAnyGoal` 返 null 不抛，故无此区分。

### 4.3 状态机

9 个状态 1:1（NotStarted/Running/Completed/Failed/Terminated/Killed/Stuck/Waiting/Paused）。`makeRunning` 闸忠实移植。**差异**：embabel `else` 分支允许从 FAILED/STUCK/WAITING/PAUSED 重跑；lynx 拒绝 FAILED 和 STUCK 重跑（更严）。early-termination 双方都 OR 组合（lynx slice 迭代 + `BudgetPolicy` + `collectExtensions[EarlyTerminationPolicy]`；embabel `FirstOfEarlyTerminationPolicy`）。

### 4.4 并发模型

| | lynx | embabel |
|---|---|---|
| 后台启动 | `StartAgent` → goroutine，返 `(*AgentProcess, <-chan error)` | `start` → `CompletableFuture` via asyncer |
| 并行动作 | `tickConcurrent`：`wg.Go` 一动作一 goroutine、预分配结果槽无 mutex、**真并行** | `actions.map{asyncer.async{...}}.map{runBlocking{await()}}`——发起并发但**收集循环串行阻塞** |
| 多 replan | **应用全部** replan | 只处理**第一个**，其余黑板更新**丢弃** |
| 无可达动作 | **优雅降级回 `tickSimple`** | 无此 fallback |
| status 合并优先级 | Failed>Waiting>Paused>Running | AGENT_TERMINATED>FAILED>PAUSED>SUCCEEDED>TERMINATED>WAITING（Paused/Waiting 顺序与 lynx 相反）|

lynx 并行执行实质更并行、多 replan 处理更正确，且明确注释了为何不用 `errgroup`（per-action 失败结构化捕获、取消走 ctx）。lynx 独有 **tool-call 级取消**（`atomic.Pointer[context.CancelFunc]`，`TerminateToolCall` 触发）。

### 4.5 子进程 / 持久化 / 调度

- **child spawn**：双方都 `blackboard.Spawn()` 继承 + BindProtected 跨子传递。lynx 更丰富：`SpawnChild`（继承父黑板，supervisor 流）vs `SpawnChildFresh`（干净黑板，orchestration/loop/fan-out 流）vs `RunFresh`（顶层 root）——这个区分 embabel 不形式化。lynx 用 `context.Context` 传当前进程，embabel 用 `ThreadLocal`。
- **budget 子树聚合**：lynx 走**直接 child 指针**递归（锁图是树，无死锁）；embabel **每次查 repository** `findByParentId`（O(n) 扫描，但这是其持久化故事的支柱）。默认数字相同：`{cost 2.0, actions 50, tokens 1_000_000}`。
- **持久化（最深架构分歧）**：embabel `AgentProcessRepository` 是**强制 SPI**——create 时 save、每 tick update、`findByParentId` 撑子树 cost 和 kill 级联；另有 `ContextRepository` 存跨 run 工作记忆；默认 in-mem 实现带 LRU + hierarchy-aware eviction。lynx live registry 是**不可插拔的内存 map**，持久化是**可选 sidecar**（`core.ProcessStore` 快照 + 可选 `SessionStore`，故意 lossy）。**lynx 完全没有可换后端的进程 repository SPI。**
- **OperationScheduler（embabel 独有）**：embabel `executeAction` 咨询 `operationScheduler.scheduleAction` → Pronto/Delayed(`Thread.sleep`)/Scheduled(→PAUSED 延到未来)，支持 rate-limit 和延迟/定时执行。**lynx 无调度器**——动作永远立即跑。

### 4.6 `collectExtensions[T]` —— lynx 的招牌架构改进

lynx 一切可插拔皆 `core.Extension`（marker `Name()`），能力检测靠**泛型类型路由**：

```go
func collectExtensions[T any](extensions []core.Extension) []T { /* type-assert 过滤 */ }
func lastExtension[T any](extensions []core.Extension) T { /* last-wins 单例 */ }
```

一个 `[]Extension` 列表按能力过滤：`[AgentValidator]`（deploy）、`[GoalApprover]`（formulatePlan，AND 否决）、`[ActionMiddleware]`（onion）、`[ToolDecorator]`/`[ToolGroupResolver]`、`[EarlyTerminationPolicy]`、`EventListener`（多播）。planner 单独走 name 分发。**加新能力 = 实现接口 + 注册一处，dispatch loop 永不改**（OCP）。embabel 是 ~8 个异质 Spring-DI SPI（各自构造注入），无统一 marker。

---

## 5. Extension / SPI 模型

### 5.1 lynx 完整 capability sub-interface 集（已核实当前代码）

| # | 接口 | 嵌 Extension | 分发模式 |
|---|---|---|---|
| 1 | `ActionMiddleware` | ✓ | collect（onion，首=外） |
| 2 | `ToolDecorator` | ✓ | collect（wrap，首=内） |
| 3 | `AgentValidator` | ✓ | collect（fail-fast） |
| 4 | `GoalApprover` | ✓ | collect（AND/否决） |
| 5 | `ToolGroupResolver` | ✓ | collect（首非 nil 胜，process 优先） |
| 6 | `IDGenerator` | ✓ | last-wins 单例 |
| 7 | `EarlyTerminationPolicy` | ✓ | collect（OR） |
| 8 | `core.Blackboard` | ✗（按类型检测，prototype 用 `Spawn()`） | last-wins prototype |
| 9 | `planning.Planner` | ✗（自带 Name()，name 分发非 type-collect） | name-match |
| 10 | `runtime.EventListener` | ✓ | collect → 启动时 `Multicast.Add` |

**注**：lynx **没有** "Blackboard factory" 或 "Planner factory" 接口——刻意用 prototype 模式 + name 匹配替代 embabel 的 `BlackboardProvider`/`PlannerFactory`。另有非 Extension 的 `ServiceProvider`（string-keyed `map[string]any`，`ServiceOf[T]` 泛型查，故意不 typed）和 `Guardrails`（chat 中间件 slice）。

### 5.2 embabel SPI 清单（spi/ 下 112 文件）

- **spi/（顶层 9）**：`AgentProcessIdGenerator` / `AutoLlmSelectionCriteriaResolver` / `BlackboardProvider` / `ContextRepository` / `LlmService` / `OperationScheduler` / `PlannerFactory` / `ToolDecorator` / `ToolGroupResolver`
- **spi/loop/（12 + streaming + support）**：`ToolLoop` / `ToolLoopFactory` / `ToolInjectionStrategy`(+Chained/Unfolding/ToolChaining) / `LlmMessageSender` / `LlmMessageStreamer` / `EmptyResponsePolicy` / `ToolNotFoundPolicy` / `ToolLoopResult` / 异常类型 / `DefaultToolLoop`/`ParallelToolLoop`/`ToolLoopCallbackSupport`
- **spi/validation/（7）**：`AgentValidator`(+AgentStructure/PathToCompletion 标记子型) / `ValidationPromptGenerator` / `DefaultAgentValidationManager` / `DefaultAgentStructureValidator` / `GoapPathToCompletionValidator`
- **spi/config/spring/（12）**：纯 Spring 接线（35 个 `@Bean`、5 个 `@ConditionalOnMissingBean`、6 个 `@ConditionalOnMcpConnection`）
- **spi/support/（25+嵌套）**：各默认实现 + Spring-AI 适配层（17）
- **spi/logging/（15）**：`LoggingAgenticEventListener` + 6 套主题 personality
- **spi/expression/spel/（2）**：SpEL condition 解析
- **spi/common/（2）**：`Constants` / `RetryProperties`

### 5.3 映射结论

**真 gap（lynx 全无对应）**：`OperationScheduler`、整棵 `spi/loop`、`AutoLlmSelectionCriteriaResolver`、可换 `AgentProcessRepository` + eviction、`ValidationPromptGenerator`/LLM 校验/validation manager、SpEL 表达式语言、主题日志 listener。

**有等价但形态不同**：ID 生成、黑板供给（prototype 非 factory）、planner 供给（name-match 非 factory）、tool 装饰、tool-group 解析（1 方法 ISP 收窄 + 权限升级拒绝）、validation（扁平 extension）、guardrails（chat 中间件）、事件监听、retry（per-action QoS）。

**按架构 out-of-scope（在其它 lynx 模块）**：`LlmOperations`/`LlmService`/`ModelProvider`（→ `core/model/chat`、`models`）、`Ranker`（→ `runtime/autonomy`）、RAG（→ `rag` 模块）——经 `ServiceProvider` 暴露给 action，刻意不固定 typed SPI 面。

**lynx 独有（embabel 无 SPI 对应）**：`ActionMiddleware`（通用 around-action onion）、`GoalApprover`（goal 否决）、`EarlyTerminationPolicy`/`BudgetPolicy`。

### 5.4 注册与作用域

lynx 双作用域单 slice 类型：platform-scope（`PlatformConfig.Extensions`，nil/空名/重名 **panic**——boot 期 fail-fast）+ process-scope（`ProcessOptions.Extensions`，process 名**可**与 platform 名碰撞——这是显式 override 机制，返 error 非 panic）。merge：onion 链 `platform++process`、resolver `process++platform`（process 先）、单例 `lastExtension`（process 后置故覆盖）。embabel：Spring `@Bean` + `@ConditionalOnMissingBean`（type-keyed 覆盖）。lynx 的 collect-vs-last 区分**在代码里显式**；embabel 用 `List<X>` vs 单 bean 注入表达多重性。

### 5.5 校验子系统

lynx：单 `AgentValidator` extension（fail-fast）+ deploy 内建 `ValidateAgent` + reachability 扫描（非 extension，硬编码）。embabel：manager + typed validator 层级（structure/path-to-completion 分型）+ `ValidationPromptGenerator`（生成 LLM 校验 prompt + Jakarta `ConstraintViolation` 报告）。**lynx 真缺** validation manager 抽象 + validator 分型 + LLM 驱动校验；但内建 reachability 已覆盖 `GoapPathToCompletionValidator` 的事（只是不是可换 SPI）。

---

## 6. Tool / Tool-Loop 系统

**架构立场对立**：embabel **自有 tool loop**（`spi/loop/ToolLoop` 是手写可插拔 while 循环 SPI，first-class 产品面）；lynx **agent 层不拥有 tool loop**，loop 下沉到 `core/model/chat` 的 `ToolMiddleware`（递归 CallHandler 装饰器），agent 层把"带工具问 LLM"当成一次 `pc.ChatWithActionTools(ctx)` 调用，**planner 重规划循环才是 agent 层的 loop**。

### 6.1 Tool 抽象

| | lynx `chat.Tool` | embabel `Tool : ToolInfo` |
|---|---|---|
| 返回 | `(string, error)` | sealed `Result { Text / WithArtifact(content, artifact:Any) / Error }` |
| schema | 裸 JSON-Schema 字符串 | 结构化 `InputSchema`（Parameter/ParameterType enum）或类型派生 |
| 调用上下文 | `context.Context`（Go idiom） | 显式 `ToolCallContext`（auth token / tenant / loopId）|
| artifact | ❌ 无（结果是字符串） | ✅ 带外 typed 对象，可 sink 到黑板 |

**artifact 通道是 tool 模型最大能力差**：embabel `WithArtifact` 携不给 LLM 看的 typed 域对象，可 `ArtifactSinkingTool` 落黑板或被 replan 决策检视；lynx 无对应（要落黑板得在 `Call` 里做副作用，loop 只见字符串）。控制流信号（HITL pause/replan/lock）lynx 走 sentinel error。

### 6.2 ToolGroup（近乎对等，lynx 忠实移植）

双方都 lazy group + 权限 enum + role 解析。lynx `ToolGroupResolver` 是 runtime Extension（`collectExtensions[T]` 发现，OCP），`PermissionsSatisfy` 子集检查 + 沙箱可拒。差异：embabel lazy MCP group **吞加载错误返空列表**（重可用性）；lynx `LazyToolGroup` **传播 loadErr**（让调用方决定）。lynx 略干净（resolver-as-extension、显式权限子集语义）。

### 6.3 ToolLoop SPI（最深 gap）

embabel `spi/loop/` 树：`DefaultToolLoop`（真 `while(iter<max)` 引擎，每轮 inspector/transformer → `LlmMessageSender.call` → empty-response 策略 → tool 调用 → injection 策略）+ `ParallelToolLoop`（asyncer + per-tool/batch 超时）。四正交扩展轴：

- **ToolInjectionStrategy**：Full（基线，每轮见全集）/ Chained（组合多策略求和增删）/ **Unfolding**（渐进披露/Matryoshka——LLM 先见一个 facade tool，调用后换成内部 tools，`exclusive` 模式清掉其余聚焦）/ ToolChaining（drain `DomainToolTracker`，工具返回的工具下轮可调）。
- **EmptyResponsePolicy**：`ExitOnEmpty`/`RetryWithFeedbackPolicy(maxRetries)`/`Throw`。
- **ToolNotFoundPolicy**：`AutoCorrectionPolicy`（模糊匹配建议 N 重试）/`ImmediateThrow`。
- **inspector/transformer**：只读观测 + history 改写（压缩/摘要），try/catch 包裹防破坏 loop。

lynx 对应是 `tool_middleware.go` 的**递归**（非计数 while）：

```go
func (m *ToolMiddleware) executeCallRecursively(...) {
    resp := next.Call(...)              // 一轮 LLM
    if !shouldInvoke { return resp }    // 无 tool call → 完
    result := InvokeToolCalls(...)
    if result.ShouldReturn() { return ... }  // returnDirect
    return m.executeCallRecursively(nextReq, ...) // 递归
}
```

特性对照：

| 能力 | embabel | lynx | 备注 |
|---|---|---|---|
| 自驱多轮 loop | ✅ | ✅ 递归 | 都有 |
| returnDirect 短路 | ✅ | ✅ | 对等 |
| **流式 tool loop** | 部分（框架拥有，只观测） | ✅ **自驱**（`executeStreamRecursively` 注入合成 ToolMessage） | **lynx 更强** |
| max-iteration cap | ✅ `MaxIterationsExceededException` | ❌ **无界递归**（靠模型停 + budget 兜底） | **gap** |
| ToolNotFound 策略 | ✅ `AutoCorrectionPolicy` | ❌ 硬错（整 call 中止） | **gap** |
| EmptyResponse 策略 | ✅ `RetryWithFeedback` | ❌ 无（空回复直接结束） | **gap**（弱模型） |
| 动态 tool 注入 | ✅ injection strategy | ❌ tool 集冻结 | **gap**（无 unfolding） |
| loop inspector/transformer | ✅ | ❌（仅 per-tool OTel span） | gap |
| 并行 tool 执行 | ✅ `ParallelToolLoop` | ❌ 串行 for-range | **gap** |
| LoopMemo / 单轮去重 | ✅ `LoopMemo`(loopId+LRU) | ✅ `toolpolicy.LoopScope`+`OnceOnly`(ctx scope) | **精神对等**，机制不同 |

lynx `toolpolicy` 全集只有**两个装饰器 + 一个 ctx helper**：`OnceOnly`（第 2 次调用返 `ErrToolAlreadyCalled`，≈ embabel `OneShotPerLoopTool` 但返 error 而非可定制 advice text）、`Unlocked`（条件门控，≈ `PlaybookTool`）、`LoopScope`。**无** `ReplanningTool`/`ArtifactSinkingTool`/`MatryoshkaTool`/`RenamedTool` 等。

**裁决**：这是最深 gap。embabel tool loop 是四轴可配可观测可扩展 SPI + 并行；lynx 是紧凑正确的递归 loop（+ returnDirect + 更强流式），但**缺：max-iter cap / ToolNotFound 自纠 / EmptyResponse 重试 / 动态注入 / 并行 / inspector**。lynx 的补偿赌注是**planner 重规划**——不在 tool loop 内换工具，而是结束 action 让 planner 下 tick 选带不同 `ToolGroups` 的动作。这覆盖了 unfolding/replan 用例（更粗粒度），但**对 not-found/empty-response/max-iter 这些健壮性策略无能为力**——这是真缺的安全网，弱模型场景尤甚。自然补法：在 `core/model/chat` 的 `ToolMiddleware`/`ToolSupport` 加 max-iter 守卫 + tool-not-found 反馈 + 可选 empty-response 重试，**不需要**引入 embabel 的 injection 机器（planner 模型已大半 obviate 之）。

### 6.4 内置工具

embabel **自带标准工具库**：file（read/write/edit/glob/diff）、math（`UnfoldingTool`）、blackboard（list/get/last/count）、process（status/budget/cost/history）、osx（AppleScript）、`ProgressTool`、`CommunicateTool`、`Subagent`/`GoalTool`/`Handoffs`。**lynx 基本零内置**——唯一内部构造的 tool 是 agent-as-tool 包装。部分哲学（黑板工具与 planner-centric 模型冲突），部分只是没建（file/math 无哲学障碍）。

### 6.5 Agent-as-tool / MCP 暴露（lynx 略胜）

lynx 一个共享 `agentTool` struct 参数化三个策略闭包（decode/run/extract）+ 5 入口：`AsChatTool[In,Out]`（typed supervisor，SpawnChild）、`AsChatToolFromAgent`、`AsMCPTool[In,Out]`（typed 顶层，RunFresh）、`AllAchievableTools`（动态反射，所有 Export!=nil goal）、`PublishAll`（动态，仅 Remote goal，MCP fan-out）。HITL first-class：挂起子进程返 JSON `{"status":"waiting", process_id, awaitable_id, prompt}` 让父 LLM 重规划。embabel `Subagent`（sealed `AgentRef` + `consuming` builder）+ 独立的 `PerGoalToolFactory`/`GoalTool`。**近乎对等，lynx factoring 略干净**（单 `agentTool` + 三可换闭包 vs embabel `Subagent` + 独立 `PerGoalToolFactory`）；embabel 优势：携子进程 typed `execution.output` 作真 artifact，lynx JSON 编码（丢 typed 句柄）。

---

## 7. HITL / Autonomy / Workflow

### 7.1 HITL：单泛型 vs sealed 子型层级

embabel ship 真类型层级：`ConfirmationRequest<P>` / `FormBindingRequest<O>`（携 `Form` + `outputClass` + 校验）/ `TypeRequest<T>`，各配 `AwaitableResponse` 子型；`onResponse` 逻辑**写在子型里**（`agentProcess += payload`）。lynx 把三种形态收成**一个** struct：

```go
type TypedRequest[P, R any] struct { IDStr string; Payload P; Handler func(R) core.ResponseImpact }
func NewConfirmation[P any](payload P, handler func(approved bool) ResponseImpact) *TypedRequest[P, bool]
```

"响应后做什么"**由调用方注入闭包**。tool 装饰器 1:1 对应（`RequireAwait`/`RequireConfirmation`/`RequireType[T]` ≈ `withAwaiting`/`withConfirmation`/`requireType`）；lynx pause 返 `*PauseError`（embabel 抛 `AwaitableResponseException`），`HandlePause` 是 canonical `errors.As`+`AwaitInput` helper。

**决定性 gap：表单绑定**。embabel `ux/form/`：`Form` + 11 控件（Button/Dropdown/TextField/TextArea/Checkbox/RadioGroup/DatePicker/TimePicker/Slider/Toggle/FileUpload）+ `FormBinder<T>`（Kotlin/Java 双反射、`@FormField` 注解、类型强制、record/data-class 绑定）+ `SimpleFormGenerator`（从类 schema 生成 Form）。**lynx 无任何表单模型**——`RequireType[T]` 的 prompt 只是字符串，schema→可渲染表单→绑回的事全留给 UI 层。

**裁决**：lynx HITL **核心**更干净更类型安全（单 `TypedRequest[P,R]` + 注入 handler，编译期 `R` 安全）；embabel **更有能力**因为表单层。若 lynx 不打算服务端渲染表单则 by-design，否则这是最大缺件。

### 7.2 Autonomy：谁能 LLM-选什么

| 能力 | lynx | embabel |
|---|---|---|
| LLM 选 goal | ✅ `LLMRanker` 排 (agent,goal) Candidate | ✅ `Autonomy.chooseAndAccomplishGoal` via `Ranker.rank` |
| LLM 选 agent | ⚠️ 隐式（选 goal 即选其 agent），无独立排 agent 路径 | ✅ 显式 `chooseAndRunAgent`，直接排 `agents()` |
| **LLM 选 plan** | ✅ **`LLMPlanRanker`**（排 `[]*Plan`）| ❌ 无任何 plan-ranking |
| LLM 编排多 agent | ❌（lynx 严格 planner-driven） | ✅ **`SupervisorAgent`/`SupervisorAction`/`SupervisorInvocation`** ReAct loop |
| goal 否决 | ✅ `core.GoalApprover` extension | ✅ `GoalChoiceApprover` fun-interface |
| 置信阈 | ✅ `GoalConfidenceCutOff` | ✅ goal + **独立 agent** confidence cutoff |
| 多 goal 组合 / agent 剪枝 | ❌ | ✅ `multiGoal` 组合 + `Agent.prune` A* 剪枝 |

**Ranker SPI**：lynx `Rank(ctx, userInput, []Candidate) ([]Choice)`（Candidate 钉死 {Agent,Goal}）；embabel `rank<T: Named&Described>`——**同一 ranker 排 goal/agent/任意**，更通用。**narrow-interface 纪律（lynx 胜）**：autonomy 包定义自己的 2 方法 `platform` 接口 + 编译期 tripwire test；embabel `Autonomy` 直接抱整个 `AgentPlatform`——正是 CLAUDE.md 的 DIP/ISP 对比。

**SupervisorAgent（embabel 独有）**：`SupervisorAction.execute` 跑 LLM 编排 loop（max 10 iter），每轮把非 goal 动作暴露为 curried tools，问 LLM 调哪个、重查 `isGoalAchieved`、最后跑 goal action——LangGraph-supervisor/ReAct 风格。**与 lynx 设计对立**（CLAUDE.md 明说"Planner-driven 而非 ReAct-loop"），lynx 的 LLM-in-control-flow 预算花在 *ranking*（goal+plan）而非 *orchestration*。

### 7.3 Workflow primitives

lynx 7 个 primitive 全产出普通 `*core.Agent`（"无新运行时概念"）：

| Primitive | 编译成的 GOAP agent |
|---|---|
| `Sequence[In,Out]` | 1 action chain 子-**agent**（SpawnChildFresh），每步 last_result 喂下一步 |
| `Parallel[In,Element,Result]` | fanout（errgroup over 子**agents**）+ join，`MaxConcurrency` 上限 |
| `ScatterGather[In,Element,Result]` | scatter（errgroup over **闭包**生成器）+ gather |
| `Loop[In,Out]` | CanRerun action 跑 Body 子-agent，记 `History[Out]`，`{name}_done` 条件 |
| `RepeatUntil[In,Out]` | CanRerun action 跑**闭包** Task，`{name}_acceptable` 条件 |
| `RepeatUntilAcceptable[In,Out]` | 薄 shim → `RepeatUntil` + `Evaluator` 返 `Feedback`，阈 0.7 |
| `Consensus[In,Element]` | 薄 shim → `ScatterGather` + `pickConsensus` 计票 |

**embabel 确有 workflow 层**（`api/common/workflow/`，非只靠注解/Supervisor）：`SimpleAgentBuilder`/`ScatterGather`/`RepeatUntil`/`RepeatUntilAcceptable`/`ConsensusBuilder`，各返 `TypedAgentScopeBuilder`（可 deploy 或 `asSubProcess`）。但 **embabel 没有 `Sequence`、没有 over-sub-agent 的 `Parallel`/`Loop`**——它的 ScatterGather/RepeatUntil 都 over **闭包**，对应 lynx 的 `ScatterGather`/`Consensus`，不对应 lynx over-sub-agent 的 `Sequence`/`Parallel`/`Loop`。

**行为差异**：(1) embabel `RepeatUntilAcceptable` 留 `AttemptHistory{result,feedback}` 且 consolidate **返 bestSoFar()(按分)**；**lynx 返最后一次**，只绑最新 `Feedback`，无 result+feedback 配对、无"返最佳"。(2) embabel 把评估建模成**自己的 TransformationAction**（LLM-judge 是可观测规划步）；lynx 把 evaluator 折进 `Accept` 条件回调（更简但更不可观测）。(3) lynx 分支隔离显式文档化（`SpawnChildFresh` 干净子黑板）；embabel ScatterGather 在**同一**黑板上 parallelMap。

**裁决**：lynx workflow 层**更广、隔离更好**（Sequence/Parallel/Loop over sub-agent 是真反超 + 分支隔离）；embabel 在 **evaluator-optimizer 更深**（best-of-N、feedback 配对 history、评估即 action）。

---

## 8. 生态广度 / 事件 / 观测

### 8.1 模块广度

| embabel module | LOC | lynx 对应 |
|---|---|---|
| embabel-agent-api | 150,997 | `agent/`(~12k) + 复用 `core` |
| embabel-agent-autoconfigure | 25,896 | **gap**（Go 无 DI 容器，显式 `Platform.New`） |
| embabel-agent-rag-core | 18,956 | 复用 `lynx/rag`(~1.6k) + `lynx/vectorstores`(~21k) |
| embabel-agent-skills | 7,068 | **gap**（无 YAML+Docker skill 引擎） |
| embabel-agent-mcpserver(+security) | 6,171 | 部分复用 `lynx/mcp`(~1.3k) |
| embabel-agent-observability | 5,892 | 部分：agent 内直连 OTel（无独立 module） |
| embabel-agent-rag-{lucene,pipeline,tika} | ~10.4k | 复用 `lynx/vectorstores`(27 后端) + `lynx/rag` + `lynx/documentreaders` |
| embabel-agent-code | 3,660 | **gap** |
| embabel-agent-test-support | 3,394 | **gap**（仅 per-pkg `_test.go`） |
| embabel-agent-a2a | 2,615 | **gap** |
| embabel-agent-shell | 1,681 | **gap**（lyra 是后端，不在 agent 内） |
| embabel-agent-{openai,onnx} | 1,532 | openai 复用 `lynx/models`；**onnx gap** |
| embabel-agent-starter-* ×21 | ~0（pom） | **gap by design**（无 Spring） |

**裁决**：embabel 作为 framework 全面更广（~43 module/~240k）；lynx agent 是 ~12k 库，广度靠组合 sibling 模块。**真 gap（lynx 生态全无）**：A2A server、skill-script 引擎、ONNX 本地 embedding、code-agent、`@Tracked` AOP + Micrometer metrics、agent 层 per-LLM/tool/embedding 事件。**by-design**：autoconfigure/starter、HTTP/REST/Shell（lyra 拥有）、持久化外移 chatmemory、lossy 单向事件 JSON、OTel-direct。

### 8.2 事件模型

lynx 16 个事件（type-erased `Event` + `BaseEvent` 嵌入，`Multicast` snapshot-deliver-outside-lock + per-listener `recover()`，**单向 lossy JSON**）：Platform(2)、Process lifecycle(7)、Planning(3)、Execution(3) + `NamedListener`。

embabel ~32 个事件（sealed `AgenticEvent` + Jackson 多态**可往返**，`AgenticEventListener` 双方法）：除生命周期外，**一等的 LLM/tool/embedding 层事件**——`LlmRequest/Response`、`ToolLoopStart/Completed`、`ToolCallRequest/Response`(带 correlationId)、`Embedding*`、`Ranking*`、`ObjectBound`、`ProgressUpdate`、`RagRequest/Response`。

**裁决**：embabel ~2× 粒度且 LLM/tool/embedding 层远更丰富、JSON 可往返；lynx 覆盖 planner/process/action 生命周期好（16 个干净映射到 embabel 子集）但**无 LLM/tool/embedding/ranking/progress 事件**（在下层 `models`/chat 中间件，lyra 从 agent 事件流合成 turn/tool 事件）。lynx lossy 单向 JSON 是刻意选择（往返 = 内存里 type-assert）。

### 8.3 观测

| | lynx agent | embabel |
|---|---|---|
| 机制 | 直连 OTel（`otel.Tracer("lynx/agent")`，5 处 callsite + 各 planner） | Micrometer + Spring Observation（vendor-neutral）|
| 注解埋点 | 无 | `@Tracked` + AspectJ 自动埋点 |
| metrics | agent 内无（靠 OTel SDK exporter） | `EmbabelMetricsEventListener`（Micrometer 计数/计时） |
| 日志关联 | 无 | `MdcPropagationEventListener`（MDC 跨线程） |
| per-LLM-call 追踪 | **无**（agent 无 LLM 事件） | `LlmInvocationEvent`/`EmbeddingInvocationEvent`（per-call token/延迟） |
| 日志策略 | **仅 OTel，无 slog/log**（仅 examples 用 log） | slf4j 全程 |

**裁决**：embabel 作生产观测框架远更全（独立 module + Micrometer metrics + 可换后端 + `@Tracked` AOP + MDC + per-call token 计账）；lynx 直连 OTel 在 5 个战略点（process context + 各 planner，规范 `RecordError`/`SetStatus`），严守 no-slog。lynx **agent 内无 metrics、无 per-LLM-call 追踪**（在 `models` 中间件 + lyra RPC tracing）——部分 by-design，但 agent 层无 metrics + per-call token 是真 gap。

### 8.4 LLM provider / RAG / 持久化

- **provider**：lynx 复用 `lynx/models` 的 **41 个 provider dir**（含 audio/image/TTS）**实质超过** embabel ~13 个 LLM starter；embabel 优势是 turnkey 打包（每 provider 一个 Spring Boot starter + autoconfigure）。
- **RAG**：embabel RAG core+pipeline（~22k 定制 RAG 逻辑：层级内容模型、entity-graph 抽取、chunk 合并/压缩、RSS ingestion）**RAG 机器远更深**；lynx 在**向量后端广度**决定性胜（27 后端 vs embabel 实质 1-2）。强调不同。
- **持久化**：embabel ship 全套 in-mem repo（process/blackboard/context/conversation/asset）+ Spring Data 换后端 hook；lynx agent 内 in-mem process store + 黑板，**无 DB 后端**，持久 chat history 是 sibling `lynx/chatmemory`(~1.6k SQLite+) 的事。多为 by-design 外移，但 embabel 统一 Spring Data repository 抽象确更便利。

---

## 9. 已核实的保真度 / 正确性差异（重点）

这一节是本次重审最有价值的产出——移植中的**实质性偏离**，已逐条对照源码。

### 9.1 Plan NetValue 丢了 `actionsValue` 项（保真度 gap，已核实，**✅ 已修复 2026-05-29**）

> **修复**：`plan.go` 新增 `ActionsValue` 方法，`NetValue = Value + ActionsValue − Cost`，对齐 embabel `Plan.kt:96`。因 action `Value` 默认 `Static(0)`，常见场景（动作不设 Value）行为不变。回归测试 `planning/plan_test.go:TestNetValueIncludesActionsValue`。


`planning/plan.go:55-58`：

```go
// NetValue is goal value minus plan cost — the embabel ranking heuristic.
func (p *Plan) NetValue(worldState core.WorldState) float64 {
    return p.Value(worldState) - p.Cost(worldState)
}
```

embabel `Plan.kt:96`：`netValue = goal.value(state) + actionsValue(state) − cost(state)`（**3 项**，`actionsValue` = 各动作 value 之和）。lynx **只 2 项**，注释却自称"embabel ranking heuristic"。后果：在多动作 plan 之间排序时，embabel 奖励"构成动作本身有价值"的 plan，lynx 纯按 goal value vs path cost 排。**影响多 plan 排序选择**，建议要么补 `actionsValue` 项，要么改注释别声称等同 embabel。

### 9.2 retry 不清 effect condition（潜在正确性 bug，已核实，**✅ 已修复 2026-05-29**）

embabel `AbstractAgentProcess.kt:561`：retry 前**清动作 effect 条件**（`if (context.retryCount > 0) action.effects.forEach { bb.setCondition(it, false) }`），防半应用的 effect 污染下次尝试。lynx `runWithRetry`（execute_action.go）原来每次尝试 `ResetError()` 但**不清 effect 条件**。

> **修复**：`runWithRetry` 在 `attempts > 1` 时遍历 `meta.Effects` 并 `SetCondition(key, false)`，对齐 embabel。hasRun 键仅在 loop 后成功时 set，故 retry 期清它是 harmless no-op。回归测试 `runtime/retry_effect_test.go:TestRetryClearsEffectConditions`（验证失败重试后第二次尝试看到 effect 条件已清，而非 stale true）。

### 9.3 其它已核实差异

- **condition And cost**：lynx `left+right`（求和，§2.4）vs embabel `minOf`——短路下 lynx 高估评估成本（影响 planner cost 启发式微调，非正确性）。
- **多输入 action**：lynx 只自动绑 `inputs[0]`，多余手 `Get[T]`（§2.1）——embabel 反射绑全部参数。
- **replan exclusion 黏性**：lynx exclusions 累积到完全 stuck 才清；embabel 每次成功规划后清（§4.2）——lynx 更易"自我饿死"。
- **`hasRun` 设置**：embabel 仅在黑板未被清时 set hasRun（防 clear 型动作），且无论 status 都 set；lynx 仅成功时无条件 set（§runtime cross-cutting）。
- **max-iteration**：lynx tool loop 无界递归（§6.3）——靠 budget 兜底，无显式 cap。
- **命名漂移**：`lastResult`(embabel camelCase)→`last_result`(lynx snake)；`StuckHandler`→`StuckPolicy`；`ConditionDetermination`→`Determination`；类型键包限定。

---

## 10. 战略 gap + ROI 路线图

按 ROI 排序。每项注"该不该补 / 为什么"。

### 10.1 P0 — 该补，工作量小性价比高

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 1 | ~~**NetValue 补 actionsValue 项**~~ **✅ 已修复 2026-05-29** | 极低（§9.1） | 移植保真度 / 多 plan 排序正确性 |
| 2 | ~~**retry 清 effect condition**~~ **✅ 已修复 2026-05-29**（§9.2）| 低 | 潜在正确性 bug；非幂等 action 的 retry 安全 |
| 3 | **ToolMiddleware max-iteration cap**（§6.3）| 低（chat 层加计数 + 上限错误） | 防失控 tool loop 烧 budget；最小安全网 |
| 4 | **ToolNotFound feedback + EmptyResponse retry**（§6.3）| 低-中（`ToolSupport` 加可配策略） | 弱模型健壮性；当前硬错/静默结束 |
| 5 | **per-LLM-call invocation 细化 + agent 层 LLM/tool 事件**（§8.2）| 中（`Usage` 加 `Invocations` + 新事件类型） | 成本审计 / tool 遥测 / 计费对账 |

### 10.2 P1 — 生产硬刚需

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 6 | **持久化 SPI**（`ProcessRepository` 可换后端 + Redis/Postgres 样板 + eviction）| 中 | 跨进程恢复 / 故障 resume；当前 agent 内空白（§4.5） |
| 7 | **AgentValidationManager 多层化**（在 `AgentValidator` 基础加内建 structure/reachability validator）| 低-中 | deploy 时一次报全问题（§5.5） |
| 8 | **metrics**（OTel meter：tick 数 / action 延迟 / plan 耗时 / budget 用量）| 低-中 | embabel `@Tracked` 等价的生产监控（§8.3） |

### 10.3 P2 — 闭合架构差距

| # | 项 | 工作量 | 价值 |
|---|---|---|---|
| 9 | **SupervisorAgent 模式**（复用 `LLMRanker` 抽 supervisor-style API）| 中 | 多 agent 自动选择 / ReAct 选项（§7.2，注意与 planner-only 哲学的张力）|
| 10 | **RepeatUntilAcceptable 返 bestSoFar + feedback 配对 history**（§7.3）| 低 | evaluator-optimizer 选最优而非最后 |
| 11 | **artifact-bearing tool**（扩 `Tool.Call` 返回类型或加 metadata 通道）| 中（breaking）| 图像/表格/PDF 工具；当前走 base64/副作用（§6.1）|
| 12 | **内置 file/math 工具**（§6.4）| 低 | 开箱即用；黑板工具因哲学跳过 |

### 10.4 P3 — 长尾 / 外挂

| # | 项 | 价值 |
|---|---|---|
| 13 | **ux/form 表单层 + 反射 binder**（§7.1）| 服务端 HITL 表单；若 UX 出范围则跳过 |
| 14 | **OperationScheduler**（delayed/scheduled 动作）| rate-limit / 定时；YAGNI 直到真需要 |
| 15 | **Shell REPL / A2A server / Skills 引擎 / ONNX**（放 `agents/` 外挂或 lyra）| 调试 / 协议 / skill 分发 / 本地 embedding |
| 16 | **GOAP unknown-condition 惰性分支**（§3.2）| 仅当出现"评估某 condition 才知道选哪个 plan"的真实场景 |

### 10.5 故意不做（"为什么不抄"）

| # | embabel 有但 lynx 不该抄 | 原因 |
|---|---|---|
| A | Spring autoconfigure / starter 矩阵 | lynx 是库不是 framework；Go 无 DI 容器传统 |
| B | `LlmService`/`ModelProvider`/`AutoLlmSelectionCriteriaResolver` SPI | lynx 直接用 `chat.Client`，多抽一层画蛇添足 |
| C | Spring Observation 抽象层 | OTel 已是 Go 事实标准 |
| D | `@Agent`/`@Action` 注解模型 | Go 无 runtime annotation；builder 已够清晰 |
| E | 多种 HITL sealed 子类 | 单泛型 `TypedRequest[P,R]` 更整洁（§7.1）|
| F | sync/async 双轨 | goroutine + `iter.Seq2` 单路径覆盖 |
| G | Action 5 契约继承 | ISP 拆 + 元数据 struct 集中，类型层级更扁（§2.1）|
| H | `DataDictionary`/`DomainType` 知识图谱层 | 当前无消费者，YAGNI（§2.5）|
| I | SpEL condition 表达式语言 | Go 闭包 + `Determination` 更类型安全 |
| J | ToolInjectionStrategy（Unfolding/Matryoshka） | planner 重规划已 obviate 大半；仅大 tool 集（>20）才值 |

---

## 11. 一句话定档

**对照 embabel：核心思想不妥协，工程化形态全 Go 化。** GOAP + 黑板 + OODA + 三值 Determination 完整保留；不照搬 Spring framework 哲学。lynx 在 **HTN planner**、**后向 STRIPS 剪枝**、**reactive 0-progress 守卫**、**`collectExtensions[T]` 统一 dispatch**、**ISP 拆 Blackboard / Process HAS-a**、**workflow over sub-agent + 分支隔离**、**`LLMPlanRanker`**、**流式 tool loop 自驱**、**deploy-time 可达性校验**、**narrow platform 接口**这 10 条线上已验证反超或更整洁。

embabel 的真优势集中在它**刻意做厚而 lynx 刻意做薄/外移**的层：**ToolLoop SPI 全家桶**（injection strategy / 错误策略 / 并行 / inspector）、**`ux/form` 表单层**、**`SupervisorAgent`**、**观测框架 + per-call 追踪**、**协议前端**、**强制持久化 repository**、**RAG ingestion 深度**、以及 Blackboard/DomainType 的**知识图谱式 schema 层**。

**下一阶段最高 ROI（P0）**：修两个保真度/正确性 gap（NetValue 补项 §9.1、retry 清 effect §9.2）+ tool loop 三个最小安全网（max-iter / not-found / empty-response §6.3）+ per-call LLM 事件（§8.2）。做完这五项，lynx/agent 在"正确性 + 健壮性 + 可审计"上补齐生产闭环关键，同时 library kit 哲学不动摇。**P1** 再上持久化 SPI + validation 多层 + metrics，即可从"高保真原型"升到"可生产部署"。

---

*对比结束。双方 HEAD 截至 2026-05-29（lynx `e480bc7`）/ 2026-05-27（embabel `9dc8a897`）。本文由 7 个并行深读 agent 通读两边源码产出，关键保真度断言已二次核对。*
