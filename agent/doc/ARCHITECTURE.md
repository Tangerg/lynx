# Agent Framework 绿色重构架构

> 状态：已接受的目标设计基线
> 建立日期：2026-08-06
> 最后更新：2026-08-30
> 实施范围：唯一的 `agent` Framework module

本文只定义新 Agent Framework 的定位、领域语言、边界、目标结构和不可变量，不记录阶段进度、提交或临时实施细节。

- 架构决策及取舍原因见 [`DECISIONS.md`](DECISIONS.md)。
- 工程实施和代码质量标准见 [`ENGINEERING_STANDARDS.md`](ENGINEERING_STANDARDS.md)。
- 上位约束见 [`../../CLAUDE.md`](../../CLAUDE.md)、[`../../DESIGN_PHILOSOPHY.md`](../../DESIGN_PHILOSOPHY.md) 和 [`../../REFACTORING.md`](../../REFACTORING.md)。

代码与本文冲突时不得静默迁就：如果实现有误，修改实现；如果设计被事实推翻，先更新决策文档，再修改本文和代码。归档中的历史阶段和迁移证据不授权当前实现范围。

---

## 1. 背景与重构边界

Scope 最早从 Embabel Agent 移植并发展出以 GOAP、Goal、Action、Condition、Blackboard、Planner 和 Process 为中心的框架。当前 `agent` 已完成绿色重写和消费者切换：共同 Process 不以 Planning 为中心，Interaction、Planning 与 Workflow 以可替换 Definition/Execution 策略共享同一个执行窄腰。

绿色重写期间保留的原框架实现已经整体删除。当前仓库只有一个 `agent` module、一套公共 API 和一个 Framework 生命周期 owner；迁移目录、双轨依赖和兼容入口都不属于目标架构。

### 1.1 原实现只保留为历史证据，不是兼容规范

原实现曾提供下列经过实践的证据，当前只能通过仓库历史查阅：

- GOAP、HTN、Utility 等算法及其正确性测试。
- Tool loop 的事件顺序、checkpoint/resume 和并发控制。
- Process tree、HITL、budget、usage、snapshot 和恢复中的已验证不变量。
- 原实现历次边界清洗留下的反例与 architecture tests。

历史证据不等于兼容：

- 原实现不决定当前 API、包结构或共同领域模型。
- 历史迁移裁决不构成当前合同，当前行为只由现行架构、ADR、公共基线与测试证明。
- 不从历史中恢复混合抽象、别名、旧 wire 或第二生命周期入口。
- architecture gate 永久禁止临时 module path 与 Host application 依赖回流。

---

## 2. 总体定位

> Agent 是一个可嵌入、可组合、拥有统一执行生命周期的 Go Framework；它允许 Interaction、Planning 以及未来被真实推进和恢复语义证明的新执行策略成为平等且可嵌套的 Agent Definition。

### 2.1 从直接能力到完整应用

框架必须允许使用者只支付当前需求所需的复杂度：

```text
直接 AI 能力
    chatclient / embeddingclient / tool
        ↓ 需要模型自主选择工具
本地 Agent 执行
    Engine + Interaction Definition
        ↓ 需要暂停恢复、预算和子任务
托管 Process 执行
    Engine + snapshot + child Process
        ↓ 需要多定义部署、路由和长期治理
完整 Agent 应用
    Platform + Host Application
```

“嵌入式”不是一种 Execution Mode，不建立 `EmbeddedMode`。可嵌入性来自显式依赖、无全局容器、可选持久化，以及同一 Engine 可以在普通 Go 进程内运行。

直接模型调用永远是一等路径。只需要一次或少量明确模型调用的程序应直接使用 `chatclient`；普通同步控制流可以直接使用独立的 `flow` 库，不应被迫创建 Agent 或 Process。

### 2.2 能力上限

新架构提高的是系统的表达、组合、恢复和治理上限，而不是模型本身的智力。其关键性质是组合闭包：

> 任意 Agent Definition 可以启动另一个 Agent Definition 对应的子 Process；子 Process 可以使用任意执行策略并继续组合，而所有实例仍服从同一个 Process 生命周期。

---

## 3. 核心设计原则

1. **最小正确抽象优先。** 不为完整感预建 package、接口、配置或扩展点。
2. **一个概念只有一个术语。** 不同时保留 plan/todo、run/execution、sub-agent/child-agent 等近义公共概念。
3. **抽象程度向下递增，具体度向上累积。** 共同 Kernel 不承载 GOAP、ReAct、任意编排实现或产品 Session 的专属状态。
4. **Execution Strategy 是主变化轴。** Interaction 和 Planning 不是 Extension；其他 Strategy 必须先证明独立推进与恢复语义。
5. **Extension 只表达横切能力。** 事件观察、策略检查、instrumentation 等可以扩展；主控制流不能伪装成扩展。
6. **生命周期只有一个所有者。** Engine 为每棵 root tree 建立唯一 `treeRuntime` 提交线，具体策略只在 owner 外推进一个有界步骤。
7. **组合优于包装。** orchestrator-worker 语义由已成立的 Strategy 和 child Process 组合；普通同步控制流留在 `flow`，需要独立 Process 生命周期的确定编排才进入 managed Workflow。
8. **状态归拥有者。** GOAP 状态归 Planning，消息和轮次归 Interaction，Stage/branch/fan-out 游标归 Workflow；未来新策略的状态同样留在其 owner，不提前进入 Kernel。
9. **框架状态与应用持久化分层。** Agent 捕获、验证和恢复执行快照；Host 决定何时、在哪里、以何种事务保存。
10. **默认安全且确定。** 不默认重试任意副作用，不默认无限递归，不允许并发完成顺序决定业务结果。
11. **透明胜过魔法。** 不做扫描、注解、全局 DI、隐式策略注册或隐藏模型调用。
12. **简单方案先行。** 只有评测证明需要时，才从直接调用或 `flow` 组合升级到 managed child Process、多 Agent 或自主 Agent。
13. **输入、意图和事实严格分离。** Signal 进入 Execution，Transition 表达意图，Event 记录事实；三者不得互相冒充。
14. **状态推进与外部效果分离。** Step 只产生候选状态和 Effect 意图；模型、Tool、Action 和其他 I/O 由策略拥有的 Effect dispatcher 执行。

这些原则与 Anthropic 在 [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents) 中强调的简单、透明、可组合以及重视工具接口设计的方向一致。

---

## 4. 统一领域语言

代码命名利用 package qualifier 避免口吃，例如优先 `agent.Definition`，不写 `agent.AgentDefinition`。

| 术语 | 唯一含义 | 明确不表示 |
|---|---|---|
| `Definition` | 不可变的 Agent 行为定义，可创建或恢复 Execution | 运行实例、部署记录 |
| `Descriptor` | Definition 的稳定名称、描述、输入输出契约等静态信息 | 可变配置袋 |
| `Deployment` | 已校验、冻结、可精确恢复的 Definition | Process |
| `DeploymentRef` | Deployment 的稳定身份和值引用 | 指针、可变注册项 |
| `Input` | 创建 Process 时按目标 Descriptor 校验的不可变 wire value | Host request DTO、共享可变对象 |
| `Output` | Process 完成时按目标 Descriptor 校验的最终语义结果 | Delta 拼接、UI 投影 |
| `Execution` | 某个 Process 内、由具体策略拥有的可推进执行状态 | Engine、goroutine |
| `Process` | Engine 管理的运行实例及共同生命周期事实 | GOAP Process 专属状态 |
| `Signal` | Engine 接受并按序交给 Execution 消费的输入 | 已发生事实、状态修改入口 |
| `SignalID` | 一次 Signal 投递的稳定去重身份 | 等待目标身份 |
| `WaitID` | Engine 铸造并暴露给外部回答者的等待目标身份 | Signal 投递身份、策略 payload |
| `Transition` | 一次有界 Step 产生的候选状态和下一生命周期意图 | 任意应用事件集合、已提交事实 |
| `Effect` | Transition 声明的、在 Step 之外由 Engine 或策略 dispatcher 执行的操作意图 | Host transaction、任意业务事件 |
| `Prepared Step` | Effect 执行前由 Engine 原子记录、尚未 finalize 的候选状态与固定意图 | 已提交 Execution state、Host transaction |
| `Event` | 已经发生的框架事实 | Signal、命令、Transition |
| `Delta` | 执行期间产生的临时流式增量 | 完成结果、可恢复状态、权威记录 |
| `Engine` | 驱动 Process、执行状态迁移和父子调度的唯一执行内核 | 产品 Session runtime、部署市场 |
| `Platform` | 可选的多 Definition 部署、目录、路由和治理容器 | Engine 的同义词 |
| `Strategy` | Interaction、Planning 及通过准入的新主执行语义 | Extension |
| `Planning Action` | Planning 中具有前置条件、预测效果和成本的不可变搜索操作 | 可执行函数、LLM Tool |
| `Tool` | 暴露给模型选择和调用的 JSON/Schema 能力 | 所有 Action |
| `Delegate` | 以模型可理解的 Tool 合同暴露一个 exact child Deployment 的 Interaction 组合值 | 通用 Action、Platform 路由、Dispatcher 私自启动 Process |
| `Child Process` | 由另一个 Process 启动的普通 Process | 独立的 `SubAgent` 类型 |
| `ProcessSnapshot` | 单 Process 的不可变诊断与 Strategy inspector value | 恢复单位、Store 或事务 |
| `TreeSnapshot` | 完整 root tree 的 canonical 恢复状态与可选 durable writer identity | Host Store、事务或产品记录 |
| `Waiting` | 正在等待已声明的外部条件 | 人工暂停 |
| `Paused` | 由操作者或策略明确停止调度、等待继续 | 子任务尚未完成 |

命名反向约束：

- 不创建 `AgentManager`、`ExecutionService`、`RuntimeHelper`、`Common`、`Utils`、`Impl`。
- 不把 ReAct 命名为 `reactive`；Reactive Planner 和 ReAct 是不同概念。
- 不用 `Mode` 枚举承载本质不同的执行生命周期。
- 不为同一操作同时提供 `StartAgent`、`RunAgent`、`ExecuteAgent` 等含义重叠入口。
- 不创建 `SubAgent` 结构体；父子性存在于 Process relation。
- 不把暂时的 `agent` 写入领域类型名、事件名或 snapshot kind。
- 不用 `Correlation` 同时表示 SignalID、WaitID、EffectID 或策略内部逻辑 key。

---

## 5. 所有权边界

### 5.1 Agent Framework 拥有

- Definition 校验与 Deployment 冻结。
- Process 状态机和有界执行循环。
- Execution 的创建、推进、捕获和恢复。
- Process tree、子 Process 调度和父级唤醒。
- Signal 接受、排序、去重、等待寻址和成功消费确认。
- Effect 稳定身份、调度顺序和框架内结算事实。
- 通用 usage/budget 限制。
- Waiting、Paused、Completed、Failed、Canceled、TimedOut、Killed 生命周期。
- Framework event 和通用可观测事实。
- snapshot envelope、校验和恢复协议。
- 执行策略的显式装配。

### 5.2 基础模块拥有

- `core/chat`：provider-neutral 请求、单 `Output` 响应和流协议。
- `chatclient`：直接模型调用、middleware 和结构化输出。
- `embeddingclient`：直接 embedding 能力。
- `tool`/`tools`：工具协议、schema、调用和具体工具。
- `history`：独立 history 能力。
- `otel`：官方 OTel API 的组合与 adapter。

Agent 复用这些能力，不复制 Client、Model、Tool、Message、Embedding 或 OTel 抽象。

### 5.3 Host Application 拥有

- 用户、Workspace、Conversation、Session、Turn 等产品身份。
- HTTP/WebSocket/SSE/desktop/CLI 传输协议。
- Store、Repository、数据库 schema、事务和 CAS/lease 策略。
- 产品权限、订阅、计费、审计和 retention。
- UI 文案、展示状态和产品事件映射。
- provider/model 选择、价格表和产品默认预算。
- checkpoint 的提交时机与应用 write-set。
- 使应用自有事实失效的销毁、回滚、替换和恢复策略，以及关联 Process 的生命周期清理。

Host 最终只依赖 Agent 的中性生命周期合同，不解析 Planning、Interaction 或未来 Strategy 的内部 snapshot payload。

---

## 6. 目标架构与执行窄腰

```mermaid
flowchart TD
    Host["Host Application"] --> Platform["Platform（可选）"]
    Host --> Engine["Engine"]
    Platform --> Engine
    Engine --> Kernel["Process Kernel"]
    Kernel --> Definition["Definition / Execution"]
    Definition --> Interaction["Interaction"]
    Definition --> Planning["Planning"]
    Definition --> Workflow["Workflow"]
    Interaction --> ChatClient["chatclient"]
    Interaction --> Tool["tool"]
    Planning --> Planner["GOAP / HTN / Utility"]
    Interaction --> Child
    Planning --> Child
    Workflow --> Child
```

### 6.1 窄腰

所有 Execution Strategy 只在以下语义上相交：

```text
Definition
  ├─ 描述静态契约
  ├─ 创建新的 Execution
  └─ 从自身状态恢复 Execution

Execution
  ├─ 按序消费 Signal
  ├─ 推进一个有界且无外部副作用的 Step
  ├─ 产生候选状态、Transition 和 Effect
  └─ 捕获自身可移植状态

Engine
  ├─ 创建和拥有 Process
  ├─ 调用 Execution.Step
  ├─ 应用 Transition
  ├─ 为 Effect 建立稳定身份并调用策略 dispatcher
  ├─ 将 Effect 结果重新投递为 Signal
  ├─ 调度子 Process
  └─ 在满足等待条件时恢复父 Process
```

冻结后的执行窄腰如下；精确参数名、GoDoc 和完整公开面由 `API_BASELINE.md` 与自动 digest 守卫：

```go
type Definition interface {
	Descriptor() Descriptor
	Start(Input) (Execution, error)
	Restore(ExecutionState) (Execution, error)
}

type Execution interface {
	Step(context.Context, []Signal) (Transition, error)
	Snapshot() (ExecutionState, error)
}
```

公共窄腰使用可移植、类型擦除的 JSON value；泛型只用于边缘 typed adapter，不能把 `Definition[I, O]` 放进 Engine 必须同构保存的根合同。Descriptor 的输入输出 schema 是权威结构合同并进入 Deployment identity；Definition/typed adapter 仍负责 Go 类型和语义不变量。

Strategy Effect payload 和所有 Signal payload 对 Engine 不透明。每个 Strategy 在自己的 package 定义最小 dispatcher/codec，Deployment 冻结并绑定这些能力；Engine 只理解 Framework 自有 Effect、信封身份、路由、顺序、limit 和 settlement，不 import 或 type-switch 具体 Strategy。dispatcher 与 erased raw value 的精确公开合同已经由 Interaction、Planning、Workflow 和独立消费者共同冻结。

如果子 Process 能力需要注入 Execution，应由真实消费包定义最小接口，不能把完整 `*Engine` 或不断膨胀的 `ExecutionContext` 传给所有策略。子创建、等待、模型调用、Tool 和 Action 都通过 Transition 声明 Effect，使 Engine 继续拥有生命周期顺序，而 Strategy 继续拥有具体执行语义。

### 6.2 Step

`Step` 是一次可取消、可丢弃的纯候选归约，不等同于整个任务：

| Strategy | 一个 Step |
|---|---|
| Interaction | 消费模型/Tool/外部输入 Signal，决定下一 Effect、等待或完成 |
| GOAP | 消费观察/Action 结果，推进 observe → plan → act → reobserve 状态 |
| Workflow | 推进一个有序 Stage 的纯归约、child start/wait 或稳定聚合边界 |

任何 Strategy 的 Step 都不能执行模型、Tool、Action 或其他外部 I/O，不能隐藏无限循环或启动无所有者 goroutine。Runtime 从 `context.Background()` 构造 cancellation-only context；它不含 value、deadline 或 cause，只允许 tree owner 取消已经没有提交资格的计算。外部操作只能由 Effect 表达并由该 Strategy 的 dispatcher 执行。

每棵 root tree 的私有 `treeRuntime` 是唯一 Framework commit owner，但纯计算不占有 owner line：同一 Process 至多有一个 Step job，不同 sibling 可以并行。owner 将 last-stable ExecutionState 与有序 Signal 前缀交给 job；job 完成 Step、Transition 校验和 candidate snapshot 后返回 attempt identity。只有 owner 可以再次校验并采用结果；Kill/Pause/Cancel 或新 incarnation 使 attempt 过期时，结果与 error 整体丢弃，Execution 从 last-stable state 重建。

Dispatcher Effect 使用 `planned → pending → settled` 状态机：

1. owner 验证候选 state、signal consumption、budget、capability 与完整 batch identity；
2. 当前 Effect 进入 pending，并在 durable mode 先提交包含完整 tree 的 pending boundary；
3. pending 成功后才启动 owner 外的 Dispatcher job；
4. Dispatcher result 被规范化为 definite 或 Unknown Settlement；
5. settled boundary 成功后，owner 才安装 settlement、candidate state、mailbox 与 Process transition。

同一 Process 的 Effect 按声明顺序逐项跨越 pending/settled，尚未派发的 planned Effect不能成为 Unknown。ephemeral mode执行同一个状态机但不调用 Host durability port。Event/Delta 只记录 attempt/observation，不能替代 acknowledged `TreeSnapshot`。

### 6.3 确定性编排边界

确定性编排按生命周期强度分边界：普通 Go/AI 同进程控制流可以直接写 Go，也可以选择独立 `flow`；每项工作需要独立 ProcessID、DeploymentRef、snapshot、预算、能力、取消和 tree recovery 时使用 managed Workflow。`flow` 是可参考的既有实现而不是 Agent Framework 的必选依赖：Workflow 可以吸收其显式拓扑、typed composition、确定顺序和有界 fan-out 思想，但不强求复用、不建立强制 adapter，也不共享 runtime、Store、Journal 或恢复事实。

Workflow Definition 冻结一个有序 Stage 序列。一个 Stage 消费当前 immutable、schema-validated value 并产生下一 value；相邻 schema 在构造时精确衔接。首个封闭词汇只有 `Transform`、`Call`、`Switch`、`Fork`、`Map` 和 `Loop`：Sequence 是 Stage 声明顺序，Prompt Chaining 是连续 Call，Gate 由 Transform/Switch 表达，Vote 是 Fork 后的纯 reducer，evaluator-optimizer 是 Loop 组合。多阶段分支通过调用另一个 Workflow Deployment 组合，不在同一 Process 内嵌第二个 Execution。

Transform、selector、reducer 和 predicate 是一个 Step 内的有界确定纯函数。Call、被选择分支、Fork branch、Map item 和 Loop iteration 都启动 exact Deployment 的真实 child Process。Fork/Map 必须显式配置正数 `WindowSize`，每个固定窗口完整结算后才启动下一窗口，结果按声明顺序归位；该名称不承诺持续补位的滑动并发池。Loop 有显式正数迭代上限并准确区分 predicate satisfied 与 limit exhausted。ExecutionState 只保存 Stage/value/case/window/item/iteration 游标和 child/wait/result 身份，不保存函数、Deployment concrete value、Engine、goroutine、Store/Journal 或 Host 数据。

`flow.Node.Run`、Graph scheduler 和 Journal 仍不能进入 `Execution.Step`。Workflow 不产生 dispatcher Effect；它只通过 Framework child Effect 组合，包内 Dispatcher 仅拒绝意外 dispatcher Effect。这样 Engine 继续是唯一 Process loop、Effect 提交与 tree recovery owner。

### 6.4 Process 状态机

共同 Process 只使用以下状态：

```text
NotStarted → Running
Running    → Waiting | Paused | Completed | Failed | Canceled | TimedOut | Killed
Waiting    → Running | Failed | Canceled | TimedOut | Killed
Paused     → Running | Canceled | TimedOut | Killed
```

- `Continue`：仍可立即调度下一 Step。
- `Waiting`：等待工具结果、人类输入、时间、子 Process 或其他已声明条件。
- `Completed`：产生符合 Definition 输出契约的结果。
- 终态由 Engine 已记录的控制意图和 Step 结果共同决定，不能只按 error 文本或 `context.Canceled` 推断。
- Engine 显式 kill 映射为 `Killed`；父 Process 或 Host context 取消映射为 `Canceled`，并用 cause 区分发起方。
- Process 或被提升为 Process 终止原因的 Effect deadline 映射为 `TimedOut`；deadline 不压成普通 `Failed`。
- 普通 Step error、外部失败、panic 或合同违约映射为 `Failed`，并保留稳定错误分类。
- 已提交终态 first-terminal-wins；迟到取消或 deadline 不能覆盖它。
- `Stuck` 不是共同状态；GOAP 无可行计划属于 Planning 的结果和策略决定。

终态矩阵按以下优先级匹配，并由表驱动合同持续守卫：

| 已记录原因 | deadline 已到达 | Step/Effect 结果 | 终态 | cause |
|---|---:|---|---|---|
| Engine 显式 kill | 任意 | 任意 | `Killed` | Engine kill reason |
| Process/parent/Host deadline | 是 | 任意 | `TimedOut` | 准确 deadline owner |
| parent 取消 | 否 | 任意 | `Canceled` | parent cancellation |
| Host context 取消 | 否 | 任意 | `Canceled` | host cancellation |
| 无控制面取消 | 否 | 合同违约、外部失败或 panic | `Failed` | 稳定错误分类 |
| 无控制面取消 | 否 | 合法完成 | `Completed` | completion |

Effect 自己的取消或 deadline 先作为 settlement Signal 交给 Strategy；只有 Strategy/Engine 决定它终止整个 Process 时，才依据同一矩阵映射，不能把局部调用失败天然提升成 Process 终态。

共同 Process 可以拥有：

- `ProcessID`、`RootProcessID`、`ParentProcessID`
- `DeploymentRef`
- `StartedAt`、`FinishedAt`
- `Status`、准确 terminal cause/failure、当前等待条件
- 通用 usage/budget
- pending Signal 的不透明信封、到达序、投递状态和消费游标
- WaitID 与 Strategy logical wait key 的映射
- 尚未 finalize 的 prepared step envelope，其中唯一保存 pending Effect、稳定 EffectID 和逐项 settlement
- Execution state envelope

共同 Process 不拥有：

- Goal、WorldState、Plan
- WorkingContext、model-call/ToolCall cursor、Tool checkpoint
- 任意具体 Strategy 的私有 cursor、branch 或 join state
- 产品 Session、Conversation、Turn
- provider、model、USD 账本

### 6.5 Execution state envelope

共同 snapshot 只保存可判别的策略状态信封：

```go
type ExecutionState struct {
	Kind          string
	SchemaVersion uint16
	Payload       json.RawMessage
}
```

- Planning payload 拥有 WorldState、Planning pass、Action attempt/exclusion/confirmation 和 child wait 状态；Goal、Action binding 与 Planner 留在 exact Definition。
- Interaction payload 拥有 WorkingContext、model-call count、pending model response、ToolCall cursor/checkpoint、Delegate settlement 和 artifact provenance。
- Workflow payload 拥有 Stage index、current value、selected case、fan-out window/output、Loop iteration 和 child wait 状态。
- 未来 Strategy payload 只拥有自身经过准入的恢复状态；共同 envelope 不预留字段。
- Host 可以持久化 envelope，但不得依据 `Kind` 解析 payload 并参与策略控制流。
- 恢复必须通过精确 `DeploymentRef` 找回 Definition；禁止全局 `kind → factory` 巨型 switch。

共同 snapshot 只冻结 envelope，不递归解释 payload。每个 Strategy schema owner 在自己的 package 独立冻结 ExecutionState 与 Effect/Signal/Delta wire，并以覆盖守卫阻止新增私有 JSON shape 漏登记；Kernel 对这些 baseline 没有导入或解释权。

### 6.6 Signal、等待与安全消费

Signal 是唯一进入 Execution 的运行时输入。共同信封只包含稳定 SignalID、可选 WaitID 路由和不透明 JSON payload；Engine 另行记录自身分配的单调序号、投递状态和消费游标。Engine wall clock 不进入 Strategy input：业务时间必须由 Host 作为稳定 payload 明确提交，接收观察时间只属于 Event。Signal 的 kind/schema 若有需要也必须封装在 owner 自有 payload 内，不能成为共同 Process 可解释的类型。Engine 不依据 payload 决定策略控制流，也不把具体 Signal 类型放进共同 Process。

Signal 投递合同必须满足：

- 同一 SignalID 的重复提交只产生一次逻辑消费。
- Running Process 接受的 Signal 排队到 Strategy 声明的下一安全 Step 边界。
- Waiting Process 只接受当前 WaitID 允许的输入；过期、已结算和错目标输入确定失败。
- Signal 游标只有在候选状态和 Transition 成功提交时推进；失败 Step 不永久吞掉输入。
- pending queue、去重事实和游标均可 snapshot，并受单项/总量 limit。

WaitID 由 Engine 铸造，Execution 不能自行生成外部等待身份。Execution 先通过 Transition 声明 logical wait；Engine 在 Effect settlement/finalize 时原子保存 WaitID 与 logical wait 映射并入队包含 WaitID 的内部 Signal，Execution 在下一 Step 将它写入私有状态并显式进入 Waiting。映射建立后提前到达的回答可以排队，但只能在对应安全边界消费。该往返保持 Execution 单写者，不允许 Engine 回调或直接修改 Strategy state。

不同 Strategy 必须声明自己的安全消费边界并用 contract tests 证明。Interaction 默认在已开始的模型调用和 Tool batch 结算后、下一模型 Effect 前消费 steer；其可观察生效延迟上界是当前不可中断 Effect 的剩余时长加下一 Step 的调度延迟，必须写入公开 GoDoc。通用 Engine 选择“等待结算”，不提供会遗弃不确定副作用的 interrupt-and-restart，也不假装拥有外部补偿语义。

### 6.7 Effect 与结算

Effect 是 Execution 请求 Step 之外操作的唯一方式。候选信封只区分 Framework 自有目标与 Deployment dispatcher 目标，并携带 owner-owned raw payload；Engine 依据 ProcessID、Step sequence 与 effect index 生成 EffectID 并冻结 payload。Engine 只解释 child/wait/timer 等封闭的 Framework Effect，Strategy Effect 整体交给绑定的 dispatcher 解释。dispatcher 不修改 Execution，只产生 Delta 和最终 settlement Signal；不得把模型、Tool、Action kind 提升为 Kernel union。

Engine-owned Effect 必须用 EffectID 保证重复调度不重复创建 child、wait 或其他 Framework 实体。模型、Tool、Action 等外部 Effect 的 dispatcher 必须把 EffectID 作为稳定请求身份并明确其 replay contract：只有已证明同一 EffectID 重放仍是同一逻辑操作时才允许自动重投。无法证明时，未知结算必须停留为可观察、待显式裁决的状态，不得静默重放或假装成功；业务事务、补偿和外部幂等实现仍属于具体 adapter 或 Host。

同一 Transition 的 Effect batch 首版按 declaration order 逐项推进；一个 Effect settled 后，下一个 planned Effect 才能进入 pending。EffectID 和 settlement 按 batch index 确定性归位；pending/Unknown 保留每项结算事实，只按各自 replay contract 恢复，不能重跑整个 batch 或按完成先后生成协议结果。并行 batch 只有在真实 benchmark/trace 证明必要且不改变 durable prefix 后才能另行设计。

### 6.8 输入、输出与 typed adapter

Engine 必须同构保存并运行异构 Definition，因此根窄腰不泛型化。Input、Output、Signal、Effect 和 ExecutionState 的 wire value 均使用被 owner 防御性复制且受大小限制的 JSON 表示，不用 `any` 或共享可变 `json.RawMessage` 绕过合同；严格解码和 payload 版本校验由拥有其 schema 的 Definition、Execution 或 dispatcher 完成。

Descriptor 携带权威 input/output schema；schema 及影响编码语义的配置进入 Deployment digest。Engine 在 start、complete 和 child settlement 边界执行结构校验，Definition 负责语义校验。常用 Go API 由边缘泛型 adapter 提供 `I ↔ raw input`、`raw output ↔ O` 的严格转换，但 Engine 和 catalog 永远只依赖非泛型 Definition。

---

## 7. Execution Strategy

### 7.1 Interaction

Interaction 是模型根据环境反馈自主选择工具的 ReAct 类执行策略，适用于编码、研究、聊天和开放式任务。

能力包括普通与流式模型调用、稳定 Tool 边界、多轮循环、checkpoint、精确恢复、有界工具并行、HITL、steer、usage 和完整生命周期事件。Interaction 的私有 WorkingContext 是当前 Execution 精确恢复所需的模型工作集，不等同于 Host 拥有的跨 Process 产品历史或 UI 记录。

不长期保留 `toolloop` 与 `interaction` 两套公共概念。工具循环是 Interaction 的实现机制；是否暴露更小的直接 Runner，必须由独立消费者证明，不能仅为了迁移旧代码保留。

### 7.2 Planning

Planning 表达已知 Action 模型上的目标导向选择。`Planner` 是 Planning 内部的真实可替换点：

```text
Planning Definition
    ├─ GOAP Planner
    ├─ HTN Planner
    ├─ Utility Planner
    └─ 后续有真实需求的新 Planner
```

Planning 独占 Goal、Condition、Truth、WorldState、PlannedAction metadata 和 replan/no-plan policy。

GOAP 适合目标可机器验证、存在多条路径、Action 前置条件/效果/成本可声明且环境会变化的场景。GOAP 不作为默认 Agent 语义，也不用于包装固定控制流或开放式 ReAct 循环。

### 7.3 确定性编排

`flow` 是可选的普通 in-process 组合库；Workflow 是只编排真实 child Process 的 managed Strategy。Workflow 使用有序 Stage 而不是任意 DAG/Registry，不依赖或复制 `flow` runtime，也不编译成 GOAP；后续设计可以选择性吸收已被 `flow` 证明的组合规律，而不以代码复用为目标。它的独立状态是当前 value、Stage 游标、branch/item/iteration 窗口和 child wait；这些状态已经由独立 public-API consumer 的完整 tree restore 证明。

### 7.4 Orchestrator-worker 组合

旧语境中所谓 Supervisor 当前统一称为 orchestrator-worker 组合：它由 Interaction、Workflow、managed Delegate、typed artifacts、completion validator 和 child Process 构成，不是独立 Strategy、独立 ExecutionState kind 或预建 package。只有未来实现证明它具有这些既有能力无法表达的独立推进与恢复语义，才通过新 ADR 重新申请 Strategy 准入。

orchestrator-worker 有两种正交且可组合的形态。模型需要直接选择少量已知 worker 时，Interaction 把 exact Deployment 暴露为 Delegate，模型的 ToolCall 直接创建对应 child Process。任务集合由模型按输入动态拆分、但调度顺序、窗口、数量上限和聚合必须确定时，decomposer Interaction 先输出 consumer-owned typed task list，Workflow Map 再为每项创建 exact worker child，最后由另一个 Interaction 综合有序 typed results。两种形态都只使用既有 Process 窄腰；Framework 不增加通用 Worker、Task、Result、Team、Supervisor 或共享 Blackboard 类型。

Planning 可以作为 exact worker Deployment，但仅适用于其目标能够由 WorldState/Goal 验证、Action 具有诚实预测语义的子任务。编排层只能提交该 Planning Definition 的 Input 并消费其 Output，不能检查计划、逐步遥控 Action、把 Planning 当作任意内容变换器，或把业务 task/result schema 下沉进 Planning/Workflow。开放式分析 worker 应使用 Interaction 或消费方自己的 Definition。

Interaction 的 typed artifact 只代表已成功完成、再次通过 exact Delegate Output schema 的 child 结果。它以模型轮次与 ToolCall 位置保持稳定顺序，在 ExecutionState 中保存 portable `agent.Output`，对 validator 则只暴露 immutable `Artifact`、exact Delegate name 与 erased/typed decode 边缘。普通 Tool 结果、参数违约、start failure、非 Completed child 和任意 `IsError` ToolResult 都不是 artifact；Framework 不按 Go type name 猜测、不过滤 `any`、不发布到共享 Blackboard，也不拥有产品 artifact store。若应用要长期保存或跨 Process 分享结果，必须在自己的 Definition Output 或 Host 聚合中显式建模。

completion validator 是 Interaction Definition 冻结的有界、确定、无副作用纯函数，只在模型最终响应或 direct-Tool 结果形成候选完成时读取独立复制的当前 WorkingContext、candidate Output 与 artifacts。WorkingContext 是该 Execution 的模型上下文，不是 Host conversation/transcript，并且尚未包含当前候选。validator 返回显式二选一：接受；或拒绝并给出非空、有界 feedback。拒绝时，候选上下文与 feedback 作为下一轮 user message 进入 WorkingContext，仍由正数 `MaxModelCalls` 限制；耗尽以稳定 execution failure 终止，不能把未接受候选伪装成完成。需要模型、Tool、网络或其他外部判断的 evaluator 必须是 managed child Process，不能藏进 validator callback。

### 7.5 Evaluator-optimizer 组合

Evaluator-optimizer 是 Workflow Loop 的派生组合，不是新 Strategy。Loop 的 typed value 由消费者定义，至少显式保存原目标、有序 attempt/feedback history、当前候选、best-so-far 与 accepted；每轮 body 是 exact child Workflow，先 Call 一个 exact optimizer child，再 Call 一个 exact evaluator child。optimizer 读取上一轮 evaluator feedback 产生新候选；evaluator 评分并给出下一轮 feedback；Loop 的 pure predicate 只读取已提交的 accepted 状态。

最大迭代数、评价规则和 acceptance threshold 必须显式配置并进入 Deployment identity，Framework 不提供默认“好坏”标准。best-so-far 只在严格更优时更新，同分保留最早结果；达到阈值时 `LoopResult.Satisfied=true`，耗尽时正常完成但必须是 `Satisfied=false`，并返回 best attempt 而不是最后 attempt。attempt history、Score、Feedback、阈值范围和最终 report 都是 consumer-owned typed schema，不进入 Workflow/Kernel，也不借共享 Blackboard 或 runtime type 查询传播。

optimizer/evaluator 的 exact child Deployment 必须满足该消费者声明的 typed state 合同；若底层能力来自 Interaction、Planning、普通 Tool 或其他 Definition，转换与状态合并必须由 consumer-owned adapter Deployment 显式完成，Workflow 不猜测也不改写其 Descriptor。需要模型、网络或其他外部判断的 evaluator 必须是 managed child Process，不能藏进 pure Loop predicate、Transform 或 Interaction completion validator。只有评价标准足够清晰、feedback 能被 optimizer 实际消费且评测证明循环优于单次生成时，才应使用该组合。

### 7.6 新策略准入

一个 Process 只有一个顶层 Execution 和一个顶层 ExecutionState envelope。Strategy 不得在同一 Process 内驱动另一个框架 Execution；组合跨越 Strategy 或需要独立暂停、恢复、预算、取消时必须创建 child Process。

只有当新模式拥有与现有 Strategy 本质不同的状态推进和恢复语义时，才增加新的 Definition/Execution。GOAP、HTN、Utility 共享 Planning 生命周期，应实现 Planner，而不是各自创建 Engine。

---

## 8. Planning Action、Tool 与 Delegate

`planning.Action` 只表达 Planning 搜索使用的稳定名称、准确描述、Preconditions、预测 Effects 和 Cost。它没有 JSON 输入输出、执行函数或外部副作用语义；`ActionBinding` 才把预测操作绑定到 dispatcher executor 或 exact child Deployment，`PlannedAction` 只是 Planner 输出的稳定引用。因此 Framework 不再虚构一个与 `planning.Action` 同名的通用可执行 Action，也不提供无法从预测元数据推出的通用 Action-to-Tool adapter。

Tool 是提供给模型的可调用协议，强调模型可理解的名称、描述、参数 schema 和文本结果。Tool 可以直接来自 MCP、Host 或普通 Go adapter，不一定参与 Planning；权限、sandbox、once-only、产品审批、事务和业务幂等属于具体装配边界。

当模型必须选择一个拥有独立生命周期的 worker 时，Interaction 使用 `Delegate`：它冻结模型友好的 Tool 名称/描述、一个 exact child Deployment、每次调用的 Budget 与衰减 Capabilities。模型参数只表达目标 child Descriptor 的业务 Input；ProcessID、DeploymentRef、递归深度、预算、权限、版本和父子关系都由 Definition 与 Engine 决定，不能让模型填写。Interaction Execution 识别 Delegate 调用并通过 Framework `StartChild`/`WaitForChildren` 推进；Dispatcher 和普通 Tool 不获得第二个 Process 创建入口。

一个模型 ToolCall batch 可以同时包含普通 Tool 与 Delegate。Interaction 只按原始顺序切分连续区段：普通 Tool 区段继续由 Dispatcher 执行并保留有界并发/HITL，Delegate 区段声明一批 child start 并 wait-all；全部结算后只向 WorkingContext 追加原 assistant message 和一个严格按原 ToolCall 顺序排列的 ToolResult message。无效 Delegate 参数、确定的 child start failure 和 child 非 Completed 终态是模型可重新决策的错误 ToolResult；错配的 Framework Signal、身份或成功 Output schema 是执行合同违约，不能伪装成普通 worker 失败。

Platform 只在 Process 启动前选择 active root Deployment，不把 Interaction 的 exact Delegate 偷换成字符串 registry lookup。若未来需要模型在一次 Interaction 中从动态 catalog 选择 worker，必须另行证明选择、版本和权限合同，不能绕过 exact child Deployment 与 Engine 准入。

---

## 9. 子 Process 与递归组合

Agent 自递归不是当前 Go 调用栈再次进入同一个函数，而是同一个 Definition 创建一个新的子 Process：

```text
Process A₀
  └─ Process A₁
       └─ Process A₂
```

每个 Process 具有唯一 ID、独立 Execution state、独立 snapshot 和受划拨的预算。Definition 可以相同，Process 永远不同。

只建立两个正交语义：

1. `StartChild`：启动一个或多个子 Process。
2. `WaitForChildren`：按 `all`、`any` 或 `quorum` 条件等待结果。

同步调用是 start + wait-all 的便利组合；并行、竞速和投票由相同原语表达。长时间等待不得阻塞 Step：父 Execution 通过 Effect 请求 child start，捕获等待状态并返回 Waiting，Engine 将 child settlement 作为 Signal 恢复它。

递归和动态委派必须由 Engine 强制约束：

- 最大深度、子 Process 总数、fan-out 和并行度。
- 最大 Step、模型调用、工具调用、token、cost 和 wall time。
- 子预算从父剩余预算中划拨，不能复制完整预算。
- 子能力只能是父能力的子集，不能递归提权。
- 父上下文按任务投影，不默认复制完整消息、Blackboard 或秘密。
- deadline 和 cancellation 向下传播。
- 父 Process 的任意终态都不能遗留活动孤儿；父 deadline 在后代保留准确来源，其他父终态作为 parent cancellation 逐层传播。
- 父终态只向当时仍 active 的 direct child 投递控制意图；已先行合法完成的 child 保持 Completion。若父级结果依赖 child，Strategy 必须显式 WaitForChildren，不能依赖两条并发 run loop 的调度先后。
- `Process.Await` 只在线性化该 Process 的 terminal Result 且 Engine 已完成这次终止直接触发的父完成通知、子终止投递和 wait 注销后返回；它不承诺整棵后代树已经全部终止。
- child start 使用稳定请求身份，恢复时不能重复创建。
- 子输出满足目标 Definition 的输出契约。
- 子 Process 不得等待祖先 Process。
- child failure 作为结果进入父 Execution；retry、fallback、部分成功、失败提升和聚合策略由编排显式定义，Engine 不自动猜测。
- 父子创建、等待、恢复和取消产生可观测事件。

Process tree 是执行、取消、预算和恢复的共同单位。如果未来需要多个父级共享结果，应通过显式 artifact/reference 建模，不能让 Process 同时拥有多个 parent。

---

## 10. Anthropic 编排模式覆盖

依据 [Building effective agents](https://www.anthropic.com/engineering/building-effective-agents) 的分类，覆盖与证据如下。模式名称是组合词汇，不是必须各建一个 Strategy/package/type 的清单。

| 模式 | 实现边界 | 路径由谁决定 | 行为证据 |
|---|---|---|---|
| Augmented LLM | `chatclient`/Interaction + Tools + WorkingContext；retrieval 是 Tool/provider，长期 memory 属于明确 owner | 单次模型调用或 Interaction 轮次 | `direct_vs_managed`、`autonomous`、Interaction Tool/WorkingContext tests |
| Prompt Chaining | `flow.Then` 或 Workflow 连续 Call | 代码 | `workflow_patterns` 两个 exact child 串行并传递 typed value |
| Routing | `flow.Switch`、Workflow Switch 或 Platform 路由 | 代码、分类器或模型 | `workflow_patterns` urgent/standard 双输入均只创建被选 exact child |
| Parallel Sectioning | `flow.Map`/`flowx.FanOut` 或 Workflow Fork/Map | 代码 | `workflow_patterns` facts/risks 稳定声明顺序；Workflow Fork/Map contract tests |
| Parallel Voting | `flow` 并行组合，或 Workflow Fork + consumer-owned typed reducer | 代码 | `workflow_patterns` 四 voter 两窗执行，2–2 同票稳定选择最早声明结果 |
| Orchestrator-workers | Interaction Delegate，或 decomposer Interaction + Workflow Map + synthesizer Interaction；worker 始终是 child Process | 模型拆分/选择，Workflow 只确定调度与聚合 | `orchestrator_workers` 动态任务与 exact Planning Delegate tests |
| Evaluator-optimizer | `flow.Loop` 或 Workflow Loop + exact optimizer/evaluator child | 代码控制循环，可由模型评估 | `evaluator_optimizer` feedback、提前接受、exhausted best-not-last 与稳定同分 tests |
| Autonomous Agent | Interaction + Tools + 环境反馈 + 显式停止/上限 | 模型 | `autonomous` model→Tool→model final 与 Interaction limit tests |
| Pattern Composition | `flow`、Workflow、Interaction 与 child Process 按生命周期边界组合 | 代码与模型按边界组合 | `composition`、`orchestrator_workers`、`evaluator_optimizer` 的 heterogeneous Process trees |

验收同时要求可运行示例和行为断言；只有 topology 类型或文档中的模式名不算实现。若普通 Go/`chatclient` 已足够，就不创建 Process；只有独立生命周期、恢复、预算、取消或治理确有价值时才升级为 managed composition。

复杂度选择遵循最小充分阶梯：

| 需求 | 首选边界 | 不应承担 |
|---|---|---|
| 一次或少量模型调用，不需要托管生命周期 | 直接 `chatclient` | Process、snapshot、child tree |
| 同进程确定控制流，节点无需独立身份/预算/恢复 | 普通 Go 或独立 `flow` | Agent Engine、每节点 Process |
| 模型/Tool 循环需要暂停、恢复、steer、limit 或事件 | 单个 Interaction Process | Workflow、worker catalog |
| 分支/迭代必须各自拥有身份、预算、取消和 tree recovery | Workflow + exact child Process | 第二 scheduler、共享 Store/Blackboard |
| 模型动态选择 worker | Interaction Delegate；需要确定任务调度时再组合 Workflow | Supervisor Strategy、字符串 registry |
| 多 Deployment 的版本选择和统一治理 | Platform | 反向扩张 Engine 或 Strategy state |

managed 复杂度按真实树线性显现：最小 Workflow 示例是四个 Process；orchestrator-worker 是六个；三轮 evaluator-optimizer 与完整 workflow-patterns 各十个。Process 数本身不是价值，只有这些身份真实承担独立恢复、资源、取消或观察边界时才合理。纯 Transform 在示例中作为 topology fixture 创建 child，不代表业务代码应把普通函数默认升级为 Process。

---

## 11. Engine 与 Platform

### 11.1 Engine

Engine 是最小托管执行边界：为每棵 root tree 建立唯一 `treeRuntime`，启动、推进、暂停、恢复和终止 Process；接受并投递 Signal；调度有界 Step/Dispatch job并只在 owner line 提交结果；管理 Process tree、等待和取消；捕获/恢复完整 tree；执行通用 limit/policy；发布中性 framework events 和临时 delta。

Engine 不拥有产品 Session、数据库事务、模型 catalog、价格表或 UI 协议。

EngineConfig 为每棵 root tree 冻结 Limits、TreeLimits 与最大 CapabilitySet；child 只能从父预算永久划拨并衰减能力。可选 `ProcessAdmitter` 是根与子 Process 共用的唯一启动准入合同：它读取 immutable `ProcessAdmission` 中的 ProcessRelation、exact DeploymentRef、Descriptor、Budget 与 CapabilitySet，并以启动方的 context 返回批准或拒绝。它不能修改分配、创建 Process 或取得 Engine/Process 控制权；Engine 的预算、能力子集和树限额始终在唯一 Kernel 状态中执行。产品身份、订阅、价格与 transaction 不进入该值。

准入发生在 Definition.Start 和 Process 发布之前；拒绝的 root 不启动，拒绝的 child 形成稳定 child-start failure。admitter 可以协调 caller-owned 外部准入，但必须尊重 context、保持有界和并发安全、不得重入 Engine/Process，并对同一 prospective ProcessID 的可能重放自行保证业务正确性；Framework 不因此定义 Store、transaction、charge、lease 或幂等 SPI。

准入成功只表示允许初始化，不代表 Process 已经发布。可选 `ProcessStartOutcomeAcknowledger` 对每个已接受 admission 接收且只接收一个中性结论：初始化与初始 snapshot 自证完成时为 `started`，任一初始化边界失败时为带稳定 `Failure` 的 `aborted`。tentative `StartedAt` 只在 accepted admission 之后生成，且只有 started outcome 成功后才成为生命周期事实；aborted outcome 没有 StartedAt。outcome 不携带产品 identity、持久化对象、应用状态或回调 capability。Engine 在内部保留 prospective start reservation：`started` 在 acknowledgment 返回 nil 后才无失败地发布；`aborted` 永不发布；acknowledgment 失败同样不发布。Event listener 没有 veto/error 通道，不能替代该同步正确性边界。

已经捕获的 Process 恢复其原 admission，不按当前 admitter 再判一次，也不重放 start outcome；撤销授权由调用方在恢复前决定或通过明确的 Process control 表达，不能让 snapshot 恢复结果依赖隐藏的实时政策。

### 11.2 Platform

Platform 是建立在 Engine 上的可选完整形态，拥有 Deployment catalog、版本/digest、Definition 路由、多 Agent 组合发现和面向 Host 的治理入口。

本地嵌入式使用可以只创建 Engine 并运行显式 Definition；完整应用可以使用 Platform。二者共享 Process 和 Execution 语义，不建立两个 runtime。

Platform 不包装、不创建也不代理 Engine。完整形态由 Host 把同一个 `platform.Platform` 作为 exact DeploymentResolver，与原有 ProcessAdmitter、EventListener 和 EngineConfig 显式装配；根 Deployment 仍由 Host 在 active Candidate snapshot 上完成选择后交给 `Engine.Start/Run`。因此 Platform 没有第二套 Start/Run、Process handle、scheduler 或 observation bus。

Platform 的 Catalog 是 exact Deployment binding 的不可变内存快照：零值为空，同一 name/version 可以保留多个不同 digest 的历史定义，重复 exact DeploymentRef 必须拒绝而不能覆盖。枚举顺序固定为 Definition name、语义版本、完整 Deployment digest；返回集合与内部 slice 隔离。Catalog 直接实现 Engine 消费的 DeploymentResolver，但不包含 active route、变更命令、远程发现、Process 引用计数或 Host persistence。

Deploy/Replace/Undeploy 只更新 Platform owner 持有的 catalog/route snapshot；Definition 路由只从一个已提交快照选择 exact DeploymentRef。二者不能把 Catalog 退化为 package-global mutable registry，也不能让 Engine 反向依赖 Platform。

活跃 Deployment 的槽位键固定为 `(Definition name, semantic version)`：不同版本可以同时 active；同槽位的不同 complete digest 是冲突，必须显式 Replace。Replace 只改变同一槽位且保留旧 exact binding；新 SemVer 必须 Deploy 到新槽位。Undeploy 必须提交当前 exact DeploymentRef，陈旧引用不能下线已被替换的新 binding。所有本地变化一次性发布完整 immutable state；它们不声明外部持久化事务、分布式 CAS 或请求幂等。

Definition discovery/selection 只暴露一次 active snapshot 的 `DeploymentCandidate{exact ref, Descriptor}`；Candidate 没有 Dispatcher、Engine 或 Process capability。调用方提供的 DeploymentSelector 拥有 request-specific input 与选择政策，可使用 context 执行模型或网络 I/O，返回一个 exact DeploymentRef。Platform 校验该 ref 必须属于同一次候选快照，并返回快照中原始 Deployment；并发 Replace/Undeploy 不能把已完成选择重定向到另一个 binding。historical Catalog 只用于 exact restore，不自动进入路由候选。

Platform 的零值是可用的空部署聚合；`New(deployments...)` 只为一次性构造初始 active set 并校验冲突。发现调用方只取得 non-executable DeploymentCandidates，不公开另一份 executable ActiveDeployments 枚举；需要精确恢复的代码使用 Catalog/Resolve，需要启动的代码使用 SelectDeployment 返回的 captured exact Deployment。不存在单字段 Config、重复 active executable view 或兼容入口。

`embedded_vs_platform` 用完全相同的 Workflow root 与 exact worker 分别经过 caller-owned resolver 和 Platform selector/resolver 运行，并比较 Output、Status、Usage、两 Process tree、两次 admission 以及稳定 Process/Step/Effect Event 投影。不同运行中 child 可能在父 wait 注册前或后完成，所以 `signal.accepted` 与中间 running/waiting 事实可以合法不同；Platform 等价合同不谎称跨运行的 wall-clock、ProcessID 或完整 Event sequence 逐项相等。

### 11.3 Deployment 恢复

- Deployment 冻结 Definition 及影响恢复语义的配置。
- Process snapshot 始终记录精确 DeploymentRef。
- Engine 只依赖由自身消费位置定义的最小 DeploymentResolver，或在 restore 时显式接收已经解析的 Deployment。该 resolver 是无 context、同步、有界、确定且无远程 I/O 的 exact binding lookup；同一精确引用不能随调用方或调用时机改变结果。
- 路由、租户/调用方选择和远程发布发现必须先产生精确 DeploymentRef，再进入 resolver；resolver 不承担这些职责。
- same-reference child 直接复用当前 Deployment，不调用 resolver；tree restore 对每个 distinct DeploymentRef 至多解析一次，并在全部 Deployment 与 snapshot 校验成功后才注册整棵树。
- Platform 是唯一的进程内 executable catalog、版本路由和治理实现，不在 Engine 内再建立第二份权威目录；Host 的 durable publication 负责在进程启动时重建它。
- Catalog 保存 executable Deployment value，不能冒充可跨进程序列化的发布仓库；Host/adapter 负责从持久发布事实构造本地不可变快照。
- 不使用 package-global registry。
- 不通过 execution kind 猜测 concrete factory。
- Go 函数地址不能作为可靠实现身份；需要发布或 Host 提供稳定版本身份。

---

## 12. Snapshot、恢复与持久化

Agent Framework 负责捕获一致的 Process/tree 执行状态、校验 schema/DeploymentRef/父子关系/状态机不变量，并从完整 `TreeSnapshot` 和精确 Deployment 恢复。不可序列化状态必须显式失败。

Host 负责 Store、transaction、CAS、lease、幂等、retention、产品身份关联、应用 write-set 原子提交，以及崩溃后的调度政策。Agent 不定义 Store/Repository 来假装拥有持久化，也不在 Transition 内回调 Host transaction。

恢复约束：

- 不序列化 goroutine、调用栈、closure、客户端连接或 context。
- Strategy 把恢复所需状态显式放入自己的 ExecutionState。
- Snapshot 只能在 last-stable 或 prepared-step 原子边界捕获；prepared snapshot 必须完整包含候选状态、拟消费 Signal 范围、EffectID、冻结 payload 和已有 settlement。两个边界之间的并发 capture 必须确定等待或拒绝，不能读半提交状态。
- Framework 定义一致 capture 点，Host 决定哪些 capture 被持久化；“已捕获”不等于“已持久化”。
- `ProcessSnapshot` 只用于诊断、Strategy inspector、Event/debug tooling 和测试，不是恢复输入；完整 `TreeSnapshot` 是唯一恢复单位，禁止把 child 当新 root 或只恢复父级。
- `TreeSnapshot` 严格保存每个 Process snapshot、Engine-owned 活动 direct-child wait、planned/pending/settled Effect phase 与完整 tree program counter，不保存 dispatcher、resolver 或 Host persistence 对象。每棵树的 owner line直接形成 canonical snapshot；in-flight Step/Dispatch job 不进入 snapshot，只保留 last-stable state与已提交 Effect phase。
- tree restore 先校验 root/parent/depth/ChildKey、预算总和、能力衰减、tree limits、活动 child wait 和每个精确 DeploymentRef，再原子注册完整树；任一校验或解析失败不得留下部分 Process。
- 等待子树取消是 Kernel 自有的一次性 prepared capability：`PrepareWaitingSubtreeCancellation` 在完整 tree quiescent cut 上冻结 source root tree，记录 acknowledged `SourceTreeDigest`，计算确定的 resulting TreeSnapshot、parent-before-child 的 canceled Process IDs 和需要显式继续的 paused parent IDs，并返回 `PreparedWaitingSubtreeCancellation`。该 capability 必须且只能以 `Apply` 或 `Discard` 结束；在结束前同一 root tree 不能越过冻结边界，也不能从第二份状态重算结果。
- prepared 结果保留被取消 Process 及永久 child budget allocation，以 host-canceled target、parent-canceled active descendants、已关闭等待和 Kernel-owned child-completion Signal 表达事实；直接父级在消费完成 Signal 前进入 Paused。所有可失败、可取消的 Process projection staging 都在 Prepare 返回 capability 前完成；失败会释放 source tree且 live state 不变。返回后的 contextless `Apply()` 只跨越单一 apply gate并完成既有 finalization，caller 不能用请求取消撤销已经形成的 durable decision；`Discard` 只释放 source tree。两者都保留既有 Process handle，不替换 controller，不解析或修改 opaque ExecutionState，也不引入 persistence、transaction、checkpoint、lease 或产品删除模型。
- Process admission 与其 conclusive start outcome 只属于首次 root/child start；restore 不重复调用 admitter/acknowledger，也不把 live policy 或 outcome 写入共同 snapshot。Host 若不允许恢复，必须在调用恢复前拒绝或显式终止已恢复 Process。
- durable mode 由闭合 `TreeDurability` port 驱动：root/child start outcome、Effect pending/settled/resolved、Parked/Terminal checkpoint 和 restore activation 都提交完整 prospective TreeSnapshot。Host callback 必须以 previous digest/current incarnation 做原子 CAS，但该合同不得演变为 Framework Store/transaction SPI。
- child admission 期间的预算只作为 treeRuntime job 拥有的 provisional reservation 参与资源门禁，不进入 snapshot 的 committed reserved budget；started outcome 在同一 prospective tree 中同时安装 child、topology、parent settlement 和 committed reservation，其他路径释放 provisional state。
- durable restore 在 activation 前先建立不可见的 whole-tree local reservation；CAS 成功后的动作只剩无失败 publication，避免 authoritative head 已换代后才发现同 Engine identity 冲突。旧 writer 迟到 completion 由 attempt/incarnation fence 丢弃。
- tree-scoped durability fault 先收集每个 Process 的既有 Unknown、当前 boundary 与 sibling in-flight 外部 EffectID，再以 canonical 两阶段决议终止全树；终止结果不得受 map 遍历或 parent propagation 时序影响。
- 外部副作用用幂等键、外部事实或显式 checkpoint 协议处理，不能只靠 snapshot 猜测。
- snapshot schema 在开发阶段直接 breaking，不保留长期 dual-read。

ephemeral snapshot 没有 incarnation；durable snapshot 以有效、crypto-random `TreeIncarnationID` 作为唯一 mode 表示。每次 restore 在发布任何 Process 前生成新 incarnation，并由 Host 对 previous `(digest, incarnation)` 完成 activation CAS。durable Engine 不开放释放 owner 后再保存的 `CaptureTree`；只有 Runtime durability callback 或仍冻结 source 的具体 prepared tree capability 可以推进 authoritative head。

Interaction 默认把精确恢复所需的 WorkingContext 自足地保存在私有 ExecutionState；Host 的产品历史不是恢复时可静默重建它的第二真相源。若某个 Strategy 真实依赖可变外部事实，只能在自己的 state 保存不透明 revision/digest，并由该 Strategy 的 provider 校验；外部事实变化时拒绝精确恢复，从新事实开始属于创建新 Process。

Host 对自身事实执行销毁、回滚、替换或恢复时，必须在自己的生命周期/write-set 中终止并清理失效关联的 Process、snapshot 和 continuation。Framework 只提供中性 lifecycle/capture 能力，不认识产品身份、历史水位、删除集合或数据库原子性，这些值不得进入共同 Process snapshot。

---

## 13. Extension、事件与可观测性

横切替换点按真实消费位置定义一个准确的小接口，不建立通用 Extension marker、capability registry 或按运行时类型分派的 god scope。`ProcessAdmitter` 只负责启动准入；`ProcessStartOutcomeAcknowledger` 只负责 ephemeral admission lifecycle 闭合；完整 durable Host 实现闭合 `TreeDurability`；`EventListener`/`DeltaListener` 只负责观察。它们语义不同，不合并成 Policy/Guard/Middleware 或万能 Commit 近义层。只有一个实现且没有外部替换需求的内部依赖直接使用 concrete type。

Event 描述已经发生的框架事实，不承担 Signal、命令、Transition 或产品协议。每个 Event 都携带 Process-local sequence、ProcessID、exact DeploymentRef、ProcessRelation、可选 Step/Effect identity、稳定名称、phase、OccurredAt 与独立 payload，因此 child、版本和恢复归因不依赖 Host 查询。Event 分为 attempt facts 与 committed facts：前者证明一次 Step 或 Effect 确实尝试过，后者证明 Process/Signal/Step 状态已由 Engine 提交。Framework Event 词汇是封闭的；构造与反序列化同时校验 name、phase、Step/Effect identity 和对应 payload，未知名称、错配身份、缺失必填字段与非法枚举不能成为一个 `Valid` Event。常用 payload 通过 immutable typed fact 读取，observer 不复制私有 JSON struct 或按 tag 猜协议。

当前 Framework 事实集合固定为：

- Process started/restored/paused/resumed/finished；Strategy Step 提交 Paused 时同样产生 process paused fact
- Signal accepted
- Step started/finished/prepared/committed
- Framework 或 Dispatcher Effect started/finished
- Delta dropped

Event 名称由根 package 常量统一，不能在发布点散落字符串。Step finished 与 Effect finished payload 都携带同一 owner 测得的非负 `duration_ms`；Effect lifecycle 同时携带准确 target 与 settlement status。模型、Tool、Planning Action 等 Strategy-specific lifecycle 不能由不理解 opaque Effect 的 Kernel 猜测；需要时由相应 dispatcher/adapter 使用官方 OTel API 或它自己的中性观察合同，不污染 Framework Event 名称。

EventListener 与 DeltaListener 都是无错误返回的观察接口：返回值既不会改变事实，也不应制造“可否决执行”的误解。实现必须有界、并发安全且不得重入被观察 Process；panic 被隔离，并由持有投递生命周期的 Engine 以单调饱和计数暴露。Interaction Dispatcher 同理拥有模型/Tool observer 的分类计数。两者都返回不可变 typed snapshot，不递归发布故障 Event、不污染业务 Usage/settlement，也不把领域模块绑定到日志或遥测实现。Event 在每个 Process 内同步保持顺序，不承诺不同 Process 的全局顺序；同一 tree 的初始 started/restored facts 仍按 parent-before-child 的 canonical 顺序发布，使父子 trace 归因不依赖 map 遍历。durable tree 的 process-paused 与 terminal fact 共用 tree owner 的 checkpoint publication aggregate，只有对应 checkpoint callback 成功返回后才发布；crash prefix 不会越过 Host commit boundary 泄露尚未确认的 lifecycle。Delta 继续通过独立有界队列异步投递。

Delta 是与 Event 不同的临时流输出。Delta 缓冲显式有界、按调用内 sequence 排序、恢复后不重放；慢消费者造成的丢弃必须通过 gap/dropped count 可观察。观察型 listener 失败不改变 Step 或 Process 结果，也不得产生无 owner goroutine。完成 Output 必须由最终 Effect settlement/Transition 独立导出，不能由 delta 拼接成为唯一真相。

事件时间字段具有准确语义；duration 从成对时间计算或由同一 owner 生成。Host 可以投影 UI、审计和账本，Agent 不反向依赖投影。

独立 `otel/agent` adapter 只消费 Framework Event 的 typed fact 并直接使用官方 OTel trace/metric API：每次 Process runtime activation 及其 Step/Effect 形成 span；activation/exit、activation/Step/Effect 秒制 duration、terminal Framework Usage 和 Delta drop 形成 metric。span 携带 exact Process/tree/Deployment attribution，durable span 额外携带 current TreeIncarnationID；metric 只使用低基数 Deployment、activation、status、cause、target 与稳定 failure 分类。Observer 不长期保存 callback context，只保存 Span；`Close` 先拒绝新观察、等待全部 in-flight callback 完成，再结束剩余 span。provider 由 ObserverConfig 显式注入，nil 时遵循 OTel global provider；typed nil 构造期拒绝。adapter 不把 raw payload、Input、Output 或产品身份写入 telemetry。Kernel architecture gate 禁止任何 OTel import，adapter production gate 禁止 OTel SDK、Strategy、原框架实现与 Host import；SDK 只用于行为测试。

---

## 14. 依赖与目标包结构

当前已验收的生产依赖方向如下；箭头表示 Go import：

```text
Host / examples ──> root, platform, otel, concrete Strategies

platform ─────────┐
otel ─────────────┤
interaction ──────┤
planning ─────────┼──> root Kernel
workflow ─────────┘
planning/goap ───────> planning

interaction ─────────> chatclient + tool + core/chat
root Kernel ──────X──> every agent subpackage
all production ───X──> retired framework / Host app / flow / logging backend
non-otel packages ─X─> OpenTelemetry
```

当前生产 package 集合精确为：

```text
agent/
├── root package files       公共窄腰、Engine、Process、常用入口
├── interaction/             模型/工具自主交互 Definition
├── planning/                Planning domain 与 Planner contract
│   └── goap/                GOAP 实现，只依赖 Planning contract
├── workflow/                managed child Process 的确定性有序编排
├── platform/                Deployment catalog、选择与治理
├── examples/                验证公共使用路径的可运行示例
└── doc/                     架构、决策与执行记录
```

OpenTelemetry adapter 位于独立 sibling module `otel/agent`，从集成层依赖这里的公开 Event 合同；它不是 Agent Framework 的生产 package，也不会把 OTel 依赖带回本 module。

约束：

- 不预建 `core/`、`runtime/`、`service/`、`manager/`、`common/`、`utils/` 等层次或泛名 package。
- 根 package 承载真正共同且不可再分的公共语义；策略专属类型留在策略 package。
- 新 package 只有在独立变化原因和真实消费者已被证明、ADR 已更新生产 package 集合与允许边后才能建立；`htn`、`utility`、`hitl`、`internal` 目前都不存在。
- 不为“整洁”机械拆包，以独立变化原因、依赖切断和真实消费者作为依据。
- 根 facade 不通过大量 alias 重导出所有高级类型。
- `Process` 的构造权只属于 Engine；公开只读/控制面不能绕过合法状态机创建或改写 Process。
- 全图 architecture test 扫描所有非测试生产 `.go` 文件（包括受 build tag 约束的文件），锁定 package 集合、允许的内部直连边和关键外部依赖归属；examples 作为组合公共 API 的消费者单独验收，不进入生产 DAG。

---

## 15. API、实现与验收纪律

### 15.1 Go API

- 接口定义在消费方并保持最小；accept interfaces，return concrete structs。
- config 使用 options struct，不使用 builder 链和大量 variadic `WithXxx`。
- 不用 `any` 掩盖尚未想清楚的领域类型。
- 公共 wire value 使用严格、版本化、owner-copy 的 JSON；泛型只放在类型安全的边缘 adapter，不泛型化 Engine 根合同。
- 构造时校验并取得 slice/map/config 所有权。
- error 是合同；包装保留 `%w` cause，不按字符串分支。
- `context.Context` 只传取消、deadline 和请求范围值，不进入 snapshot。
- 并发有清晰 owner、停止条件和确定提交语义。
- 不为未来可能存在的实现预造接口或注册表。

### 15.2 Definition 与 Execution

- Definition 创建后不可变，并发共享安全。
- 每次 Start 创建独立 Execution。
- Execution 不被多个 Process 并发推进。
- 一个 Process 只拥有一个顶层 Execution/ExecutionState；Strategy 组合通过 child Process。
- Step 只归约状态和声明 Effect，不执行外部 I/O。
- Step 的状态变更可在 snapshot 中完整表达。
- Restore 验证 state kind/schema 和 Definition identity。
- Strategy 不能修改共同 Process 私有状态，只能通过 Transition 表达意图。

### 15.3 Retry 与副作用

- 任意 Action 和 Tool 默认执行一次。
- provider SDK 自有 retry 不在 Agent 重复实现。
- 只有明确幂等或有补偿语义时才配置 retry。
- 子 Process 创建、HITL response 和 checkpoint settlement 必须可去重。
- Engine 为 Effect 提供稳定身份，但不能据此宣称外部业务副作用已具备事务或幂等语义。
- 不以“出错就再跑一次”掩盖状态所有权问题。

### 15.4 验收层次

- 单元测试：值对象、状态机、定义校验、策略算法。
- contract tests：Definition/Execution、Signal/Transition/Event、Effect settlement、Planner、snapshot codec、child composition。
- 集成测试：Engine 驱动真实 Strategy 的完整生命周期。
- 恢复测试：每个合法挂起边界 capture → restore → continue。
- property/fuzz：snapshot、状态转换、wire 解码和畸形 adapter 输出。
- race：Engine、事件、多子 Process 和显式并行路径。
- architecture tests：依赖 DAG、禁止原框架实现/Host application import、策略状态和产品外部事实不进入共同 Process、Process 只能由 Engine 构造。
- examples：每一种正式公开编排方式至少一个最小可运行示例。

最终完成必须满足：Interaction、Planning 与 Workflow 都是经过真实消费者验证的 Execution；`flow` 与 Workflow 按 in-process/managed-Process 生命周期各自只有一个准确边界，不共享或复制 runtime；GOAP 真实可重规划；Anthropic 编排模式有行为测试；递归 child Process 可恢复、取消和预算限制；Framework snapshot 与 Host persistence 无交叉；Host 只消费中性合同；原实现、临时 module path 和兼容路径全部删除；仓库只保留唯一 `agent` module。
