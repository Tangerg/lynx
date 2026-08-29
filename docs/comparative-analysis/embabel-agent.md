# embabel-agent —— 目标导向规划（GOAP）与 scope 的直系祖先

> 实测：Kotlin + Java，796 个 `src/main` `.kt` 文件，**101,701 行**主代码，20 个 Maven module。
> 作者：Rod Johnson（Spring 之父）。构建于 Spring Boot + Spring AI 之上。
> 结构：`embabel-agent-api`（核心）/ `-domain` / `-rag` / `-mcp` / `-a2a` / `-skills` / `-observability` / `-shell` / `-code` / `-onnx` / `-openai` / `-anthropic` / `-autoconfigure` / `-starters`。

---

## 0. 特殊地位

这是七个项目里**唯一一个与 scope 有直接血缘关系**的。scope `agent/doc/ARCHITECTURE.md` 开篇第一句：

> Scope 最早从 **Embabel Agent** 移植并发展出以 GOAP、Goal、Action、Condition、Blackboard、Planner 和 Process 为中心的框架。

而 `DESIGN_PHILOSOPHY.md` 的收尾把它列为两个外部权威印证之一：

> 这套哲学不是一家之言 —— 它经**两个外部权威独立印证**：与更老的 GOAP agent 框架 **embabel-agent** 形成 convergent design；又逐条命中 **MCP Go SDK 设计文档**。

所以这份文档要回答的问题与其他六份不同：**scope 从 embabel 继承了什么、后来推翻了什么、为什么推翻是对的（或可能是错的）。**

---

## 1. embabel 的核心命题：Agent = 规划器，不是循环

绝大多数 Agent 框架的核心是「模型 → 工具 → 模型 → …」的 ReAct 循环。embabel 的核心命题不同：

> **Plans are dynamically formulated by the system, not the programmer.**
> The system **replans after the completion of each action**, allowing it to adapt to new information.

四个一等概念：

| 概念 | 含义 |
|---|---|
| **Action** | Agent 能走的一步。有前置条件、有效果、有成本 |
| **Goal** | 想达成的世界状态 |
| **Condition** | 执行前评估、每步之后重新评估的谓词 |
| **Domain model** | 支撑流程并喂给 Action/Goal/Condition 的领域对象 |

规划器把 Action 的前置/效果/成本喂给 GOAP（`plan/goap/OptimizingGoapPlanner.kt` + `astar/`），A* 搜出到 Goal 的最优路径；每执行完一个 Action 就**重新规划**。另有 `plan/utility/`（效用规划）。

**这是 scope `agent/planning` 的直接来源**：

```
agent/planning/
  action.go binding.go condition.go definition.go dispatcher.go
  execution.go external.go goal.go name.go output.go
  goap/          ← GOAP 实现，只依赖 Planning contract
```

---

## 2. 继承下来的东西（且判断正确）

| embabel | scope | 状态 |
|---|---|---|
| GOAP 作为一种规划策略 | `agent/planning/goap` | ✅ 继承 |
| `Planner` 是可替换点（GOAP/HTN/Utility） | `planning.Planner` contract；`ARCHITECTURE.md §7.2` 明列 GOAP/HTN/Utility | ✅ 继承 |
| Goal / Condition / WorldState 归规划器独占 | 「Planning 独占 Goal、Condition、Truth、WorldState、PlannedAction metadata 和 replan/no-plan policy」 | ✅ 继承并**强化**（明确不进 Kernel） |
| `AgentProcess` 有 ID、状态、父子、usage、耗时 | `agent.Process` | ✅ 继承 |
| `AgentPlatform` 作为部署与目录 | `agent/platform` | ✅ 继承概念 |
| 子进程（`createChildProcess`） | `StartChild` / `WaitForChildren` | ✅ 继承并**大幅强化** |
| HITL 一等（`core/hitl`） | Signal / WaitID / Waiting 状态 | ✅ 继承 |
| `EarlyTerminationPolicy` / `Usage` / `LlmInvocation` 记账 | Budget / Limits / TreeLimits | ✅ 继承并强化 |
| 规划后重新评估条件（replan） | 「GOAP 真实可重规划」写进验收条件 | ✅ 继承 |
| Action 有 **诚实的预测语义**（前置/效果/成本） | `planning.Action` 只表达预测元数据，执行由 `ActionBinding` 绑定 | ✅ 继承并**收窄** |

最后一条值得展开。embabel 的 `Action` 是：

```kotlin
interface Action : DataFlowStep, ConditionAction, ActionRunner, DataDictionary, ToolGroupConsumer
```

**五路接口继承**：它既是规划元数据（前置/效果），又是可执行体（`ActionRunner`），又是数据字典，又是工具组消费者。

scope 把这个混合体**切开了**（`ARCHITECTURE.md §8`）：

> `planning.Action` 只表达 Planning 搜索使用的稳定名称、准确描述、Preconditions、预测 Effects 和 Cost。**它没有 JSON 输入输出、执行函数或外部副作用语义**；`ActionBinding` 才把预测操作绑定到 dispatcher executor 或 exact child Deployment，`PlannedAction` 只是 Planner 输出的稳定引用。
> 因此 Framework **不再虚构一个与 `planning.Action` 同名的通用可执行 Action**，也不提供无法从预测元数据推出的通用 Action-to-Tool adapter。

这是一次教科书式的 SRP 拆分：**"规划器需要知道的"和"执行器需要做的"是两个变化理由**。embabel 把它们焊在一个接口里，结果是任何自定义 Action 都被迫同时实现五个关注点。

---

## 3. 推翻的东西（逐条，带理由）

### 3.1 Blackboard —— 最重要的一次推翻

```kotlin
interface AgentProcess : Blackboard, Timestamped, Timed, OperationStatus<AgentProcessStatusCode>,
                         LlmInvocationHistory, EmbeddingInvocationHistory {
	val blackboard: Blackboard
	...
}

interface Bindable {
	operator fun set(key: String, value: Any)
	fun bind(key: String, value: Any): Bindable
	fun bindProtected(key: String, value: Any): Bindable   // ← 能挺过 clear() 的绑定
	// 无名绑定：默认 key "it"
}
```

**`AgentProcess` 本身就是一个 Blackboard** —— 一个 `String → Any` 的共享可变键值空间，Action 往里写，Condition 从里读，规划器根据里面的内容判断前置条件是否满足。

还有两个补丁性概念：
- `bindProtected` —— 「受保护绑定能挺过 `Blackboard.clear()`（状态迁移时会发生）」
- 无名绑定绑到默认 key `"it"`，且"实现必须尊重加入顺序"

`bindProtected` 的存在本身就是信号：**共享可变袋子在状态迁移时会被清空，于是必须发明"不被清空"的第二类绑定。** 这是在给一个错误的所有权模型打补丁。

scope 的对应裁决遍布架构文档：

> Framework 不增加通用 Worker、Task、Result、Team、Supervisor 或**共享 Blackboard** 类型。
> attempt history、Score、Feedback、阈值范围和最终 report 都是 consumer-owned typed schema，不进入 Workflow/Kernel，**也不借共享 Blackboard 或 runtime type 查询传播**。
> 父上下文按任务投影，**不默认复制完整消息、Blackboard 或秘密**。
> **状态归拥有者。** GOAP 状态归 Planning，消息和轮次归 Interaction，Stage/branch/fan-out 游标归 Workflow。

替代方案是 `ExecutionState` 信封 + owner-owned typed payload：

```go
type ExecutionState struct {
	Kind          string
	SchemaVersion uint16
	Payload       json.RawMessage
}
```

**为什么推翻是对的**：Blackboard 让"这个 Process 的状态是什么"变成运行期问题。它无法可靠序列化（`Any` 值）、无法做能力隔离（子进程要么继承整个袋子要么什么都没有）、无法做 schema 演进（没有版本）、无法在恢复时校验。scope 需要的四条不变量 —— **可移植快照、父子投影、schema 版本、恢复校验** —— 没有一条能在 Blackboard 上成立。

**embabel 为什么用它**：GOAP 的 WorldState 天然就是一个事实集合，Blackboard 是它最直接的实现。且 embabel 的领域模型是 JVM 对象 + Jackson，动态绑定在 Spring 生态里是自然的。

### 3.2 `STUCK` 状态

```kotlin
enum class AgentProcessStatusCode {
	NOT_STARTED, RUNNING, COMPLETED, FAILED, TERMINATED, KILLED,
	/** The process cannot formulate a plan to progress.
	 *  This does not necessarily mean failure. Something might change **/
	STUCK,
	...
}
```

scope 的架构文档**逐字点名拒绝**：

> **`Stuck` 不是共同状态；GOAP 无可行计划属于 Planning 的结果和策略决定。**

理由是 `DESIGN_PHILOSOPHY §2.5`：只有一个消费方（Planning）需要的概念，不该塞进所有消费者共享的底层。Interaction 和 Workflow 永远不会 `STUCK`，但在 embabel 的模型里它们也得认识这个状态。

scope 的共同状态精确是 7 个：`NotStarted / Running / Waiting / Paused / Completed / Failed / Canceled / TimedOut / Killed`，且每一个都有跨策略的统一含义。

### 3.3 `TERMINATED` vs `KILLED` 的模糊

embabel 有两个"被停止"：`TERMINATED`（被 early termination policy 杀）和 `KILLED`（被用户/平台从外部杀）。

scope 建了一张**表驱动的终态矩阵**（`ARCHITECTURE.md §6.4`）：

| 已记录原因 | deadline 已到达 | Step/Effect 结果 | 终态 | cause |
|---|---:|---|---|---|
| Engine 显式 kill | 任意 | 任意 | `Killed` | Engine kill reason |
| Process/parent/Host deadline | 是 | 任意 | `TimedOut` | 准确 deadline owner |
| parent 取消 | 否 | 任意 | `Canceled` | parent cancellation |
| Host context 取消 | 否 | 任意 | `Canceled` | host cancellation |
| 无控制面取消 | 否 | 合同违约/外部失败/panic | `Failed` | 稳定错误分类 |
| 无控制面取消 | 否 | 合法完成 | `Completed` | completion |

并有一条硬约束：

> 终态由 Engine **已记录的控制意图**和 Step 结果共同决定，**不能只按 error 文本或 `context.Canceled` 推断**。
> 已提交终态 **first-terminal-wins**；迟到取消或 deadline 不能覆盖它。

这条「表驱动 + 由已记录意图决定 + first-terminal-wins」的组合，是 embabel 那两个枚举值完全没有的严谨度。它解决的是一个真实的并发正确性问题：**当 deadline 和用户 kill 同时发生时，终态应该是确定的。**

### 3.4 `AgentPlatform` 拥有 Process 生命周期

```kotlin
interface AgentPlatform : AgentScope {
	val platformServices: PlatformServices
	val toolGroupResolver: ToolGroupResolver
	fun getAgentProcess(id: String): AgentProcess?
	fun killAgentProcess(id: String): AgentProcess?
	fun agents(): List<Agent>
	fun deploy(agent: Agent): AgentPlatform
	fun runAgentFrom(...)
	fun createAgentProcess(...)
	fun createAgentProcessFrom(...)
	fun start(...)
	fun createChildProcess(...)
}
```

Platform 同时是：部署目录 + 进程注册表 + 进程创建者 + 进程杀手 + 工具组解析器 + 服务定位器（`platformServices`）。

scope 把这个**一刀切成两半**（`ARCHITECTURE.md §11.2`）：

> **Platform 不包装、不创建也不代理 Engine。** 完整形态由 Host 把同一个 `platform.Platform` 作为 exact DeploymentResolver，与原有 ProcessAdmitter、EventListener 和 EngineConfig 显式装配。
> 因此 **Platform 没有第二套 Start/Run、Process handle、scheduler 或 observation bus。**

即：**Engine 拥有生命周期，Platform 只拥有目录与路由**。Platform 的 Catalog 是不可变内存快照，`Deploy/Replace/Undeploy` 只更新快照，且：

> 活跃 Deployment 的槽位键固定为 `(Definition name, semantic version)`；同槽位的不同 complete digest 是冲突，**必须显式 Replace**。
> Definition discovery/selection 只暴露一次 active snapshot 的 `DeploymentCandidate{exact ref, Descriptor}`；**Candidate 没有 Dispatcher、Engine 或 Process capability**。

**为什么推翻是对的**：embabel 的 `AgentPlatform` 是一个 Spring bean —— 单例、有状态、被注入到处都是。任何拿到 `AgentPlatform` 的代码都能创建、查询、杀死任何进程。scope 的裁决把"能启动进程"（Engine）和"能查目录"（Platform）分成两种能力，`DeploymentCandidate` 明确"不可执行"。这是最小权限。

### 3.5 注解驱动 + Spring DI

```java
@Agent, @Action, @AchievesGoal, @Condition, @Export
```

+ classpath 扫描 + `embabel-agent-autoconfigure` + `embabel-agent-starters`。

scope 的对应立场（`ARCHITECTURE.md §3.11`）：

> **透明胜过魔法。** 不做扫描、注解、全局 DI、隐式策略注册或隐藏模型调用。

以及 §14 约束：

> 不预建 `core/`、`runtime/`、`service/`、`manager/`、`common/`、`utils/` 等层次或泛名 package。
> **不使用 package-global registry。**

**这条分歧与 [`spring-ai.md`](spring-ai.md) 里 auto-configuration 那条同源**：Java 应用的组合根散在容器里，注解是必需品；Go 应用的组合根就是 `main()`。**不是设计水平差异，是语言生态差异。**

### 3.6 `Action` 的五路接口继承 / `AgentProcess` 的六路

见 §2 末尾。scope 的对应红线：

> **ISP 切碎接口**：接口只放调用方真用的方法；胖接口拆成按需组合的子接口。
> 实现一个接口就要完整满足其语义与**行为契约**（不只是签名），不能某些方法/参数实现某些不实现。

---

## 4. scope 在 embabel 之上新建的东西（embabel 完全没有）

这些是 scope 独立发明的，值得列出来，因为它们定义了 scope 与整个赛道的差距：

| 机制 | 说明 | 解决的问题 |
|---|---|---|
| **Signal / Transition / Event 三分** | 输入进入 Execution、意图表达、事实记录，三者不得互相冒充 | embabel 里"用户输入"和"发生的事"都是 Blackboard 上的绑定 |
| **Step 是纯归约** | 「Step 必须是对相同 ExecutionState 与 Signal 序列产生相同候选语义的纯归约；不得读取 clock/random/global state」 | 使快照可重放、使 Step 可测 |
| **Effect + prepare/finalize 两阶段** | Step 只声明 Effect 意图，Engine 分配稳定 EffectID → dispatch → 原子 finalize | embabel 的 Action 直接执行副作用，崩溃后无法判断"发出去没有" |
| **EffectID replay contract** | 「只有已证明同一 EffectID 重放仍是同一逻辑操作时才允许自动重投。无法证明时，未知结算必须停留为可观察、待显式裁决的状态」 | 外部副作用的诚实处理 |
| **执行窄腰（4 个方法）** | `Definition{Descriptor,Start,Restore}` + `Execution{Step,Snapshot}` | 使 Interaction/Planning/Workflow 成为平等的可插拔策略 |
| **策略 payload 对 Kernel 不透明** | 「Engine 只理解 Framework 自有 Effect、信封身份、路由、顺序、limit 和 settlement，**不 import 或 type-switch 具体 Strategy**」 | 新策略不需要改 Kernel |
| **TreeSnapshot + quiescent cut** | 整棵进程树的一致切面，capture 时用 Engine 私有栅栏 | embabel 的 `AgentProcessRepository` 是单进程持久化 |
| **预算划拨 + 能力衰减** | 「子预算从父剩余预算中划拨，不能复制完整预算；子能力只能是父能力的子集，不能递归提权」 | 递归委派的资源与权限安全 |
| **`ProcessAdmitter` + `ProcessStartOutcomeAcknowledger`** | 启动准入与唯一 started/aborted 结论，语义分离不合并 | 与 Host 的准入协调，且不引入 Store/transaction SPI |
| **`PrepareWaitingSubtreeCancellation`** | 一次性 prepared capability，必须以 `Apply` 或 `Discard` 结束 | 等待子树取消的确定性 |
| **Deployment digest + 精确恢复** | 「Go 函数地址不能作为可靠实现身份；需要发布或 Host 提供稳定版本身份」 | 跨进程恢复找回正确的 Definition |

**这批机制的共同主题是：把 embabel 交给"运行期约定"的东西，全部搬到"类型与协议"上。**

---

## 5. embabel 有、scope 没有的东西（真实缺口）

要公允 —— embabel 也有 scope 缺的：

### 5.1 `ActionRetryPolicy` / `ActionQos` / `EarlyTerminationPolicy`

embabel 把"这一步失败了怎么办"做成了一等配置。scope 的立场是：

> 任意 Action 和 Tool **默认执行一次**。provider SDK 自有 retry 不在 Agent 重复实现。只有明确幂等或有补偿语义时才配置 retry。

这个立场是对的，但 scope **没有提供"明确幂等时怎么配置 retry"的机制** —— 只有一条"允许你配"的原则，没有落点。embabel 的 `ActionRetryPolicy` 至少给了一个位置。

**建议**：这属于 `agent/planning` 的 `ActionBinding` 和 `agent/interaction` 的 Dispatcher 各自的配置面，不是 Kernel 的事。**记账，等真实消费者。**

### 5.2 `ToolGroup` / `ToolGroupResolver` / `ToolConsumer`

embabel 有工具**分组**的一等概念：Action 声明它需要哪些 ToolGroup（`ToolGroupConsumer`），Platform 解析（`ToolGroupResolver`）。`CoreToolGroups.kt` 定义了内置组。

scope 的 `core/tool.Registry` 只管理实例集合，`tools/CLAUDE.md`：「**手动注册，无全局 registry**：调用方显式把工具注册进自己的 toolset」。

scope 的 `CapabilitySet` 是能力衰减机制，但它是 Framework 层的**权限**概念，不是"这个 Action 需要哪一类工具"的**需求声明**。

**判断**：embabel 的 ToolGroup 在有 DI 容器时有意义（声明需求 → 容器解析）。Go 里装配是显式的，直接传 toolset 即可。**不是缺口。**

### 5.3 `ux/form` —— 结构化人机交互表单（**已核对：不是缺口**）

`com.embabel.ux.form` 把 HITL 请求建模成**表单**（字段、类型、校验），而不是自由文本。MAF 的 `RequestPort{Request reflect.Type, Response reflect.Type}` 是同一需求的另一个答案。

**三个项目都做了类型化的 HITL 契约，scope 的版本最强。** 实测 `agent/interaction/pending_input.go`：

```go
type PendingToolInput struct {
	waitID         agent.WaitID
	prompt         json.RawMessage
	responseSchema json.RawMessage      // ← 权威 JSON Schema
}

func (p PendingToolInput) ResponseSchema() json.RawMessage        // 防御性复制
func (p PendingToolInput) ResponseSignal(id agent.SignalID, response json.RawMessage) (agent.SignalRequest, error)
//   ↑ 本地按 ResponseSchema 校验后，才铸出 WaitID 寻址的 SignalRequest
```

注释明确划了边界：

> It deliberately **excludes** Tool continuation state and all application UI, persistence, approval, or actor concepts.

三者对比：

| | 契约载体 | 可跨进程 | 回答前校验 | 是否混入 UI/审批概念 |
|---|---|---|---|---|
| embabel `ux/form` | JVM 表单对象 | ❌ | 部分 | ✅ 混入（form 就是 UI 概念） |
| MAF `RequestPort` | `reflect.Type` | ❌（靠 `PortableValue` 补） | 类型级 | ❌ |
| **scope `PendingToolInput`** | **JSON Schema** | ✅ | ✅ 本地校验后才发 Signal | ❌ |

scope 用 JSON Schema 而非语言类型，使契约**可序列化、可跨进程、可给非 Go 的 Host**；且 `ResponseSignal` 强制"先校验后投递"，错误在本地暴露而不是打到 Engine。**这条 scope 领先。**

### 5.4 `embabel-agent-shell` —— 交互式开发 shell

跑 Agent、看 Blackboard、单步调试的 REPL。scope 有 `examples/` 但没有交互式调试入口。

这属于 Host/工具层，不属于框架。**不是框架缺口**，但对开发体验是真实差距。

### 5.5 GOAP 的成熟度

`OptimizingGoapPlanner` + `astar/` + `plan/utility/` + `plan/common/condition/`。scope 的 `agent/planning/goap` 是重写的，架构文档承认原实现的 GOAP 正确性测试「只能通过仓库历史查阅」。

**这不是缺口，是需要持续验证的地方**：`ARCHITECTURE.md` 的最终完成条件里明写「GOAP 真实可重规划」。

---

## 6. 一个必须诚实面对的问题：GOAP 到底值不值

embabel 把 GOAP 当作**默认的 Agent 语义**。scope 明确降级：

> GOAP 适合目标可机器验证、存在多条路径、Action 前置条件/效果/成本可声明且环境会变化的场景。
> **GOAP 不作为默认 Agent 语义**，也不用于包装固定控制流或开放式 ReAct 循环。

这是一次重要的降级。理由在整个赛道的证据里：

- **eino** 把 agent transfer 标为 "NOT RECOMMENDED: has not proven to be more effective empirically"
- **Anthropic 的 Building effective agents** 主张简单、透明、可组合，并把 orchestrator-worker 定位为组合而非新机制
- **trpc / adk / MAF** 全部没有 GOAP，编排以图/链为主

**但 scope 保留了 Planning 作为平等的一等 Strategy，而不是删掉它。** 这个判断的依据写在 §7.2：Planning 是「已知 Action 模型上的目标导向选择」，`Planner` 是真实可替换点（GOAP/HTN/Utility）。它适用的场景（目标可机器验证 + 多条路径 + 环境会变）确实存在，只是不是多数。

**scope 的处理是这个赛道里最成熟的**：既不像 embabel 那样把 GOAP 当默认，也不像其余五家那样完全不做，而是把它降为三种平等策略之一，并明确写出它的适用边界。

---

## 7. 结论

### 继承（并强化）

| 概念 | scope 的强化 |
|---|---|
| GOAP / Planner 可替换 | 降为三种平等 Strategy 之一，明确适用边界 |
| Action 的预测语义（前置/效果/成本） | 与执行体切开：`Action`（预测）vs `ActionBinding`（执行） |
| Process / 子进程 / HITL / usage 记账 | 加上预算划拨、能力衰减、TreeSnapshot、终态矩阵 |
| Platform 部署与目录 | 与 Engine 切开：Platform 只有目录与路由，无生命周期 |
| 每步之后重新规划 | 保留，并写进验收条件 |

### 推翻（逐条可追溯）

| embabel | scope 的裁决 | 理由 |
|---|---|---|
| **Blackboard（`String → Any` 共享可变袋）** | `ExecutionState{Kind, SchemaVersion, json.RawMessage}`，状态归 owner | 快照/投影/版本/校验四条不变量在 Blackboard 上都不成立 |
| `bindProtected`（挺过 clear 的绑定） | 不需要 —— 状态本来就归 owner，没有全局 clear | 补丁消失，因为被打补丁的东西消失了 |
| **`STUCK` 共同状态** | Planning 内部结果，不进 Kernel | 只有一个消费方需要的概念不下沉（§2.5） |
| `TERMINATED` / `KILLED` 二值 | 表驱动终态矩阵 + first-terminal-wins + 由已记录意图决定 | 并发下的终态确定性 |
| **`AgentPlatform` 同时是目录 + 进程注册表 + 创建者 + 杀手** | Engine 拥有生命周期，Platform 只有不可变 Catalog + 路由；`DeploymentCandidate` 不可执行 | 最小权限 |
| `Action : DataFlowStep, ConditionAction, ActionRunner, DataDictionary, ToolGroupConsumer` | 拆开 | ISP；五个变化理由不该在一个接口 |
| **注解驱动 + classpath 扫描 + Spring DI** | 显式组合根 | 「透明胜过魔法」；Go 无 DI 容器 |
| Action 直接执行副作用 | Step 纯归约 + Effect 意图 + prepare/finalize + EffectID replay contract | 崩溃后能判断副作用发出与否 |

### 真实缺口

| # | 能力 | 判断 |
|---|---|---|
| 1 | Action 级 retry / QoS 配置落点 | scope 有原则（「明确幂等时才配 retry」）无落点。归 `ActionBinding` / Dispatcher 的配置面，**记账等真实消费者** |
| 2 | 交互式开发 shell | Host/工具层，非框架缺口。开发体验差距真实 |
| 3 | 类型化 HITL 契约 | **已核对为非缺口** —— scope 的 `PendingToolInput` 用 JSON Schema，比 embabel 的表单和 MAF 的 `reflect.Type` 都更可移植（见 §5.3） |
| 4 | ToolGroup / Resolver | **非缺口** —— DI 容器的产物，Go 里显式传 toolset 即可 |

### 判词

> **embabel 提出了这个赛道最有野心的命题：Agent 是一个规划器，计划由系统搜索得出，而不是程序员写死。**
> scope 完整继承了这个命题，并做了一件 embabel 没做的事：**把它降级为三种平等策略之一。** 这个降级不是否定 GOAP，是承认「大多数任务不需要搜索，需要搜索的任务仍然值得有一等支持」。
> scope 从 embabel 学到的最大教训是 **Blackboard**：一个在 JVM + Spring 里自然、在"可移植快照 + 父子投影 + 预算划拨 + 跨进程恢复"面前完全站不住的模型。scope 的 `ExecutionState` 信封、`Signal/Transition/Event` 三分、`Effect` 两阶段提交，全部是从这个教训长出来的。
> 而 `DESIGN_PHILOSOPHY.md` 说 embabel 与 scope 形成 convergent design 是**准确的但需要限定**：收敛发生在**领域语言**层（Goal/Action/Condition/Process/Planner/Platform/HITL 这套词汇被两次独立选中），分歧发生在**状态所有权与执行契约**层，而 scope 在这一层是全面重写的。
