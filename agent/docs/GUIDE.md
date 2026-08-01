# Lynx Agent Framework 使用指南

本文只描述当前代码。符号级契约以 GoDoc、API baseline 和 wire fixture 为准；架构目标、阶段进度与决策见
[`../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)。

## 1. 心智模型

Lynx Agent 是一个可嵌入的 Go framework，不是 DI 容器，也不是 provider SDK。它分为三层：

- `agent/core` 定义 Agent、Action、Goal、Condition、Blackboard、ProcessView 和扩展协议；
- `agent/runtime` 的 `Engine` 拥有部署、进程、执行循环、挂起恢复、事件，以及进程树执行状态的捕获与重建；存储与事务归 Host；
- 根 `agent` 包提供常用定义与生命周期的标准入口，高级能力留在具名子包。

一次标准执行是：

```text
Agent definition -> Engine.Deploy -> immutable Deployment
                 -> Engine.Run/Start -> Process
                 -> observe -> plan -> action -> observe ... -> terminal/waiting
```

Action 之间不直接传参。Action 的输入和产物进入 Blackboard，Planner 根据 Blackboard
投影出的 WorldState 选择下一步。运行中的 Process 永久绑定精确的 `DeploymentRef`，不会因
同名 Agent 后续替换而漂移。

## 2. 最小可运行 Agent

常用路径只需导入根包：

```go
package main

import (
    "context"
    "fmt"

    "github.com/Tangerg/lynx/agent"
)

type Topic struct{ Title string }
type Post struct{ Body string }

func main() {
    writer := agent.New(agent.AgentConfig{
        Name:        "writer",
        Description: "write a post from a topic",
        Actions: []agent.Action{
            agent.NewAction("write",
                func(_ context.Context, _ *agent.ProcessContext, topic Topic) (Post, error) {
                    return Post{Body: "About " + topic.Title}, nil
                },
                agent.ActionConfig{},
            ),
        },
        Goals: []*agent.Goal{
            agent.NewOutputGoal[Post](agent.GoalConfig{
                Name:        "post-ready",
                Description: "produce a post",
            }),
        },
    })

    engine, err := agent.NewEngine(agent.EngineConfig{})
    if err != nil {
        panic(err)
    }
    ctx := context.Background()
    deployment, err := engine.Deploy(ctx, writer)
    if err != nil {
        panic(err)
    }
    fmt.Println("deployed", deployment.Ref())

    process, err := engine.Run(
        ctx,
        writer,
        agent.Input(Topic{Title: "Go agents"}),
        agent.ProcessOptions{},
    )
    if err != nil {
        panic(err)
    }
    post, ok := agent.Result[Post](process)
    if !ok {
        panic("writer produced no Post")
    }
    fmt.Println(post.Body)
}
```

运行入口接收零值可用的 `core.Bindings`，不接收裸 `map[string]any`。单输入使用
`agent.Input(value)`；多个命名输入先声明一个 `agent.Bindings`，再调用 `Set`。Runtime 在创建
Process 时复制绑定容器，因此调用方后续增删绑定不会改写运行中的黑板。自定义 Blackboard 的
持久化能力通过 `runtime.BlackboardSnapshotter` / `runtime.BlackboardRestorer` 交换一个
`runtime.BlackboardState`，不再依赖多返回值的位置约定。

完整示例见 [`../examples/hello`](../examples/hello)、[`../examples/blog`](../examples/blog)
和 [`../examples/blogllm`](../examples/blogllm)。

## 3. Definition、Binding 与 Deployment

`AgentConfig.PlannerName` 使用 `planning.GOAPPlannerName`、`planning.HTNPlannerName`、
`planning.ReactivePlannerName`、`planning.UtilityPlannerName` 等规范标识。空值通过
`planning.EffectivePlannerName` 归一为 `planning.DefaultPlannerName`；Runtime 选择与
Deployment digest 共用同一归一规则，避免执行语义和缓存身份漂移。

`planning.Options.ExcludedActions` 使用零值可用的不可变 `planning.Exclusions`，通过
`planning.NewExclusions` 构造；条件状态统一使用 `core.ConditionSet`。`planning.NewDomain` 在构造时
校验条件来源冲突，并把每个条件编译为带 `ConditionKind` 的 `ConditionRef`；`KnownConditions`
以稳定序列暴露这些引用，因此运行时不解析条件名，也不依赖 Go map 的随机迭代顺序。
`Binding.Validate`、`ConditionSet.Validate` 与 `Truth.Valid` 共同封闭定义边界；非法值在 Deploy 前失败，
自定义 Action 或 Condition 在执行时返回未定义枚举值也会让 Process 以明确错误失败。

`AgentConfig` 是构造输入；`Agent` 是只读定义聚合。`Engine.Deploy` 会执行结构验证、扩展验证，
并编译出不可变 `Deployment`：

- 相同名称、版本和定义摘要的重复 Deploy 是幂等的；
- 同名但定义不同会返回 `ErrDeploymentConflict`；
- 明确切换活动版本使用 `Engine.Replace`；
- `Engine.Deployment(ref)` 可读取活动或历史部署；
- `Engine.Undeploy(ctx, name)` 只移除活动路由，不破坏历史定义。

`Deployment` 只通过 `Descriptor` 暴露不可执行的 `AgentDescriptor`。该描述符包含 Agent
身份、Action/Goal/Condition 声明与 snapshot state schema，但不含 `Action.Execute`、
`Condition.Evaluate`、score function 或 `StuckPolicy`。路由 Candidate 和 filter 同样只接收
Agent/Goal descriptor；只有 Engine 内部执行器、Planner 和 deploy-time `AgentValidator`
持有完整定义。

`NewAction[In, Out]` 默认生成一个名为 `it` 的输入和输出 Binding。需要自定义名字或多个
Binding 时，使用 `ActionConfig.Inputs`、`Outputs` 和 `core.NewBinding[T]`。`ActionConfig`
保持可用零值，常用可选项包括：

- `Preconditions`、`Effects`：显式业务条件；
- `Repeatable`：允许同一进程多次选择该 Action；
- `Cost`、`Value`：Planner 评分函数；
- `ToolGroups`：抽象工具角色字符串；Resolver 在执行时把 role 解析为具体工具；
- `ClearWorkingState`：成功后清理全部工作状态。

Framework 对每次调度的 Action 只调用一次，不提供 Action 级自动 retry。需要重试的 provider、
tool 或业务写入应由对应实现结合真实副作用语义处理；框架不会猜测幂等性、事务或补偿能力。

## 4. Engine 与 Process 生命周期

`Engine` 是 framework 级主对象，支持多实例，没有 package-global registry：

- `Run`：同步驱动到终态或 Waiting；
- `Start`：同步返回持有 Process 与不可变完成结果的 `Segment`，以及 admission error；
- `Continue`：同步继续已存在的非终态 Process；
- `ContinueAsync`：同步返回 admission error，成功后由 `Segment` 承载后台运行结果；
- `Resume`：校验并记录 Suspension 响应，不暗中启动执行；
- `ResumeAsync`：在同一个进程树临界区记录响应并取得 continuation 所有权，成功后返回唯一 `Segment`；
- `PendingSuspensions`：在同一稳定快照上列出整棵树中真正等待外部输入的边界，并标明直接发起它的 Process；
- `PlanWaitingSubtreeCancellation`：在稳定快照上计算 Waiting 子树取消后的状态，不保留锁或
  live ownership；
- `ApplyWaitingSubtreeCancellation`：仅当被规划的执行状态仍然有效时应用该框架状态变换；
- `Kill`：终止一棵进程子树；
- `Process`、`Processes`：读取当前 registry 快照；
- `Engine.SnapshotTree`：在稳定执行边界捕获完整根进程树；
- `ValidateRestoreTree`、`RestoreTree`：验证并重建调用方提供的完整根进程树；
- `RemoveTree`：整体释放已终止的根进程树。

Agent 不定义 Store，也不执行 I/O、事务、幂等、重试或保留策略。需要跨进程重启恢复的 Host
应在外层加载完整 `ProcessSnapshotTree`，调用 `ValidateRestoreTree` / `RestoreTree`，并在
成功释放内存树前自行完成存储删除。Restore 从不覆盖 registry 中已有的 Process ID；调用方
必须先显式 `RemoveTree` 旧 generation，再恢复新 generation。

`Undeploy` 只移除 active route；Host 确认外部 snapshot 不再引用某个历史定义后，可调用
`ForgetDeployment`。Framework 会拒绝 active deployment 和仍被已注册进程树引用的定义，
但不替 Host 选择保留期限。

`Continue`、`Resume`、`Kill` 或 `RemoveTree` 指向已不存在的 Process 时，错误可通过
`errors.Is(err, runtime.ErrProcessNotFound)` 稳定分类；调用方不得解析错误文本。

同一 Engine 内，同一 Process 同时只能有一个 active run 驱动执行。这是本机生命周期不变量，
不是分布式 owner 协议。

观察者只应依赖 `core.ProcessView`，其 `Goal` 返回不可执行的 `GoalDescriptor`。
`GoalApprover` 也只接收该描述符。Action 的可变能力集中在 `ProcessContext`：Blackboard、
Dependencies、Chat、Prompt、Suspend、Terminate 和 Usage 记录不会进入公开 ProcessView 或
ambient context。

## 5. Chat、Prompt 与工具

Engine 接受 provider-neutral 能力，而不是要求某个具体 client：

```go
client, err := chatclient.New(model)
if err != nil {
    return err
}
engine, err := agent.NewEngine(agent.EngineConfig{
    Chat: agent.ChatCapability{
        Model:    client,
        Streamer: client,
    },
})
```

`Chat.Model` 可为空；`Streamer` 只能与 `Model` 一起配置。每进程模型覆盖通过
`core.ChatProvider` extension 完成。Runtime 组合 `ChatMiddleware` 中显式提供的 middleware；
history 分区、产品上下文和 backend 都由 Host middleware 决定。middleware 可从调用 context 的
`core.ProcessViewFrom(ctx)` 取得当前进程标识，无需 Agent 定义产品 conversation/session 协议。
这些执行状态不会进入 provider Request/Response。

Action 内的常用调用入口是：

```go
answer, err := process.Prompt(ctx, prompt, agent.PromptConfig{
    System:  "Answer concisely.",
    Options: &chat.Options{Model: "provider-model-id"},
    Tools:   []tools.Tool{searchTool},
})
```

需要结构化结果时，沿用同一条 framework-managed interaction：

```go
type Answer struct {
    Summary string   `json:"summary"`
    Sources []string `json:"sources"`
}

answer, err := agent.Prompt(
    ctx,
    process,
    prompt,
    agent.PromptConfig{Tools: []tools.Tool{searchTool}},
    chatclient.JSON[Answer](),
)
```

`chatclient.Output[T]` 仍然唯一拥有格式 instructions 与 decoder；`agent.Prompt` 只把它接到
Agent 托管路径，先校验、再追加调用方拥有的 instructions，最后解码终态文本。Framework
不生成 schema、不复制 JSON parser，也不提供 `PromptJSON` 平行抽象。需要 schema 时传
`chatclient.JSONSchema[T]` 成功构造的 output；需要原始 response、自定义修复或逐事件控制时，使用
`ProcessContext.Interact`。Runtime 仍统一记录 model call、usage、事件、限制和可恢复 tool
checkpoint。

`agent/toolloop.Runner` 是可独立复用的叶子执行器，不是第二套 Agent runtime。直接使用它的
调用方自行负责 Process、usage、事件和持久化。

Agent 只提供一个显式的工具组合入口：

- `runtime.NewAgentTool[In, Out](engine, deployment)`：父 Process 内同步调用一个精确
  Deployment，构造后不随 active route 变化。

`Goal` 只描述 planner 要达到的目标，不携带发布协议、外部描述或输入 schema。MCP/HTTP
等顶层工具发布由 Host 自己适配：Host 明确选择 deployment、输入输出 wire、独立进程
生命周期、鉴权和等待响应协议。`workflow.Supervisor` 同样接收显式 `[]tools.Tool`，不会扫描
deployment 或 Goal 元数据。

同步 `NewAgentTool` 的 child 若进入 Waiting，Runtime 会把同一个 suspension 提升到
parent Process，并保存原 model round、已完成工具结果、pending tool call 和 exact child
relation。Host 始终对 parent process 调用 `Resume` / `Continue`；child terminal 后原工具调用
才提交结果。恢复只跳过已经进入稳定 checkpoint 的模型轮次和已结算工具结果；工具外部副作用
成功但结果尚未进入 checkpoint 的崩溃窗口，仍由工具或 Host 通过幂等、事务或补偿处理。

## 6. HITL 与统一 Suspension

Action 内使用线性的 typed API：

```go
approved, err := hitl.Interrupt[bool](ctx, "publish-approval", map[string]any{
    "message": "Publish this result?",
})
if err != nil {
    return Output{}, err
}
if !approved {
    return Output{}, errors.New("publish rejected")
}
```

首次执行返回统一的 suspended error，typed Action wrapper 自动把它转换为 Waiting。Host 读取
`Process.Suspension()`，随后调用：

```go
if err := engine.Resume(ctx, process.ID(), suspension.ID, response); err != nil {
    return err
}
if err := engine.Continue(ctx, process.ID()); err != nil {
    return err
}
```

当 suspension 来自同步 AgentTool 的任意深度 child 时，仍使用 root/parent Process 的 ID。
`Engine.Resume` 会沿已捕获的 child relation 把同一响应写到最深 waiting child，再由
`Engine.Continue` 逐层完成原 continuation。

`Resume` 只提交响应；`Continue` 才重新进入 Action。Human 输入与 Tool pause 使用同一
Suspension 协议，且 Suspension 不区分"谁来回答"：Framework 从不据此分支，等待方决策所需
的一切走 `Prompt`，需要自己的 interrupt 分类就在 Prompt 里表达。ToolLoop 和 nested child
checkpoint 只进入 `FrameworkState`，不使用私有 Blackboard key：只有 `FrameworkState` 里的
continuation 属于 Runtime。

## 7. Snapshot 与 Host 边界

`core.ProcessSnapshot` 使用严格 JSON 解码：未知 schema、未知字段、trailing value、无效 enum、
DeploymentRef 不匹配或 checkpoint correlation 错误都会 fail closed。普通 Blackboard 值默认
进入 portable snapshot；Blackboard 在写入时即取得 JSON 快照所有权，不可序列化的函数、
channel、client 等会直接返回错误。运行时 handle 和 client 是能力而非 planner state，应通过
`core.Dependencies` 或闭包注入，不进入 Blackboard。

当前 ProcessSnapshot schema 为 v15，Suspension schema 为 v3，ToolLoop checkpoint 为 v4，
Runtime 私有 suspension checkpoint envelope 为 v3。Waiting snapshot 同时允许
“尚未回答”和“已回答、尚未 Continue”两种可恢复阶段：前者恢复后调用 `Resume`，后者恢复
后直接 `Continue`。`OwnUsage` 只记录该 Process 的直接通用资源计数：
Host 定义单位的 opaque cost、tokens、model-call 与 action count。Action 的名称、耗时和状态
只通过 Event/OTel 输出，不再作为可恢复执行状态保存。Suspension 是否已回答只由 Response
是否存在决定，不再保存不参与恢复语义的响应时间。`OwnUsage` 不记录 provider/model 明细、
USD 账单、embedding 调用、时间线或审计数据；这些应用投影由 Host 从 managed-interaction
event 建立，并在自己的事务边界持久化。任意 live error 的 sentinel 身份与 unwrap
链不具备通用 wire 表达，因此恢复后的 `Process.Failure()` 是 message-only
`*core.ProcessFailure`；需要识别时使用 `errors.As`，不要假定跨进程 `errors.Is`。
Child 各自在 snapshot 中携带自己的 `OwnUsage`，Restore 通过父子关系重建聚合；读取完整
委派树用量时使用 `Process.Usage()`。被 framework tree transition detach 的 child subtree
不再拥有 portable snapshot，其历史消耗进入直接父进程的 `RetiredChildUsage`；它不会冒充
父进程直接消耗，但仍参与 tree usage、预算恢复与应用账本对账。

`ProcessSnapshotTree` 只是完整进程树的值协议。`Engine.SnapshotTree` 要求树内进程均处于
可捕获边界，并一次返回 root 与全部已注册 descendants；`Engine.ValidateRestoreTree` 校验树
结构、nested suspension relation 和当前 Deployment catalog；`Engine.RestoreTree` 在整棵树
均可重建后才原子注册。以上 API 不读取或写入任何 backend。

并发 tool call 或递归 AgentTool 会让一棵树同时存在多个真正需要回答的 Suspension。
`Engine.PendingSuspensions` 基于同一种稳定完整树捕获返回这些直接等待源：普通 managed tool
call 保持模型调用顺序，nested child 占据其父 tool call 的位置，除此之外的 sibling 按
Process ID 排序。返回值只含 Process 归属、Suspension ID、Prompt 与 ResumeSchema；私有
checkpoint 不跨出 Runtime。Host 应把整组响应作为一个产品事务接收，再按该顺序驱动
continuation，不能把父进程为传播控制流而持有的 Suspension 副本误当成第二个用户问题。

取消 Waiting delegated child 时，Host 不得先 `Kill` 再自行编辑 snapshot。
`Engine.PlanWaitingSubtreeCancellation(rootID, targetID)` 会基于一份稳定完整树：

- 精确移除 target subtree 的 live relation 与 portable snapshot；
- 将 canceled tool call 结算为确定性的 error result，并按 tool-call 顺序保留其发布位置；
- 把 target subtree 的历史总 usage 归入直接父进程 `RetiredChildUsage`，保证归因、完整树
  usage 与共享预算同时守恒；
- 将 active ancestor 标记为 framework-ready，而不是写入或伪造外部 Suspension Response；
- 返回 ownership-isolated 的 replacement tree、surviving Pending 与 exact canceled IDs。

Plan 不执行用户代码、不发布事件、不写存储、不改变 live state，也不在返回后持有任何
框架资源。Host 自行决定如何把 `ResultingTree` 纳入应用事务；事务成功后调用
`ApplyWaitingSubtreeCancellation`，Runtime 会先验证规划时观察到的执行状态仍然有效，再
detach canceled subtree 并发布 lifecycle event。框架不参与 Host 的 commit、abort、rollback
或幂等策略。

Apply 之后仍有 Pending 时进程树保持 Waiting；Host 需要驱动时先调用 `Continue` 消费
framework-ready 边界，待 Runtime 暴露下一个真实输入边界后再 `Resume`。若 Pending 为空，
直接 `Continue`。对 framework-ready boundary 调用 `Resume` 会以 stale 明确拒绝。

Host 自己定义消费侧存储接口，并决定：

- 何时捕获（例如只在可恢复的 Waiting 边界）；
- 完整树如何原子替换，以及写失败如何影响产品 Run；
- 删除、重试、幂等、事务、保留和跨节点 ownership；
- 产品 Session、subtask lineage、history 与 process snapshot 是否需要同事务提交。

`core.Budget{}` 的所有维度均为无限制。Agent 不选择货币、租户或产品层默认阈值；Host
需要限制时只在 root `ProcessOptions` 显式传入 cost/token/action/model-call limit，且同一次
执行中的 cost 单位必须一致。Runtime 为完整 Process tree 建立一个共享原子准入器：
Action 在执行前计数，模型调用在 I/O 前预留，因此并发 sibling 不会重复消费同一份
action/model-call 余量。`ChildOptions` 返回 Budget 会被拒绝（`runtime.ErrChildBudget`）：限额只在
root 有意义，接受一份 per-child 限额等于承诺一个没人执行的子树上限。Token 与 cost 只有响应后
才能确定，因此属于 continuation ceiling：当前已准入调用可以越过阈值，但不会再准入下一项工作。

托管 Interaction 不接受调用点传入的 raw Model、Cost 或 Observer。Runtime 每次从 Process
作用域解析 `ChatProvider` 并套用 ChatMiddleware；Host 定价实现
`InteractionCostProjector`，产品账本/UI/审计投影实现 `InteractionObserver`。流式调用只在
`Interaction.Stream` 暴露 delta，底层 Streamer、最终响应累积、usage 与 suspension 仍由
Runtime 统一管理。`Prompt` 和 `PromptCondition` 使用同一条托管路径。

Agent 不提供 conversation/session 标识或 context binder。Host 应通过 `ChatMiddleware` 安装普通
Call/Stream middleware：顶层进程映射到产品 conversation，子进程可按
`core.ProcessView.ID()` 建立独立历史分区。产品标识和分区规则因此不会进入 Process snapshot。

Agent 公共面没有存储端口、产品 Session、自动快照或持久化失败策略，也不提供它们的 alias 与
shim：Host 在自己的 adapter/application 层编排存储、产品上下文和事务，只把加载后的 snapshot
值交给 Agent。

## 8. Extension 与 Dependencies

所有行为扩展先实现：

```go
type Extension interface { Name() string }
```

Runtime 再按最小 capability interface 发现 `planning.Planner`、`ActionMiddleware`、
`ToolMiddleware`、`AgentValidator`、`GoalApprover`、`ToolGroupResolver`、`ChatProvider`、
`StopPolicy`、`IDGenerator`、`Blackboard`、`EventListener` 和 `SubtreeEventListener`。Engine scope 来自
`runtime.Config.Extensions`；Process scope 来自 `core.ProcessOptions.Extensions`，但只接受
执行期能力。`AgentValidator`、`IDGenerator`、Blackboard prototype 仅属于 Engine scope；
Process Blackboard 使用 `ProcessOptions.Blackboard`。扩展实例可能被不同 Process 并发调用，
实现自行保护可变状态；Runtime 不为扩展调用提供串行、重试或分布式协调。

动态领域依赖使用 typed `Dependencies`，而不是全局 service locator：

```go
var SearchKey = core.MustDependencyKey[Search](`search`)

if err := core.RegisterDependency(engine.Dependencies(), SearchKey, search); err != nil {
    return err
}

processDependencies := engine.Dependencies().Child()
if err := core.RegisterDependency(processDependencies, SearchKey, tenantSearch); err != nil {
    return err
}
options := core.ProcessOptions{Dependencies: processDependencies}
```

Action 内通过 `core.LookupDependency(process.Dependencies(), SearchKey)` 读取。查找顺序是
Action -> Process -> Engine；同名异型、重复注册、nil 值、缺失和冻结后写入都有独立 sentinel
error。静态 Action 仍优先使用构造函数、struct 字段或闭包注入。

## 9. Child Process、Workflow 与并发

Child API 的状态继承是明确契约：

| API | Blackboard | 使用场景 |
|---|---|---|
| `RunChildWithState` | 父 Blackboard 的完整副本 | 子任务确实需要父工作状态 |
| `RunChild` | 干净状态，仅绑定显式 input | 默认、安全的自包含委派及 workflow branch |

Child 使用精确 Deployment、整棵树共享的预算准入器，并仅继承父 Process 显式注册的
`SubtreeEventListener`；普通 `EventListener` 只观察注册它的 Process。其他 Process extension、
chat middleware、history partition 和 dependency override 都由 Host 的 `ChildOptions` 显式配置；
callback 只获得 child 的 `AgentDescriptor`，不能通过配置策略取得或执行 child Action。

`workflow.Sequence`、`Parallel`、`Loop`、`Team`、`RepeatUntil`、`RepeatUntilAcceptable`、
`ScatterGather`、`Consensus` 和 `Supervisor` 最终都编译回普通 Agent。需要在构造期部署
子 Agent 的 builder 接收 `context.Context`；`Team` 只合成定义，不拥有部署或执行。
`ScatterGather` / `Consensus` 的 generator 只接收 `context.Context` 和 typed input，从类型上
无法取得父 Process 的 Blackboard、生命周期控制或托管 Interaction；只有显式返回值按声明顺序
join。需要托管 Prompt/ToolLoop、暂停、终止、checkpoint 或进程级预算的并行单元必须使用
`Parallel` Child Process。`MaxConcurrency == 0` 表示同时启动全部已声明分支，负数配置会在
构造期失败。一个分支失败会取消共享 context，但 generator 必须协作退出；框架等待所有已启动
分支返回，且多个失败始终选择最低声明位置的非取消错误，不让完成时序改变 Process failure。

ToolLoop 的并发是另一层语义：工具默认独占，实现 `toolloop.ConcurrentTool` 后可按
resource key 有界并发。`toolloop.Config.MaxConcurrentCalls` 控制低层 Runner，
`interaction.Limits.MaxConcurrentToolCalls` 控制托管 Interaction。两个限额都不带 framework
默认值：未设并发即逐个执行，未设轮数上限即跑到模型不再请求工具为止。轮数上限未在
Interaction 上给出时继承进程的 `MaxToolRounds`，因此宿主有一处就能约束全部托管交互；跑多久、
容忍多少本地扇出属于产品决定，框架不替宿主选数字。限额也**不进 checkpoint**：它们是每次运行
重新提供的策略，不是被恢复的状态，所以宿主调整数字不会让已 park 的续跑失效——新限额直接对
恢复后的循环生效。执行完成顺序不影响可观察顺序：ToolResult、continuation 和 checkpoint 始终
按模型原始 tool-call 顺序提交。

工具需要实现幂等键、审计关联或下游 trace 关联时，可通过
`agent.ToolCallFromContext(ctx)` 读取当前模型请求的 `chat.ToolCall`。该访问器只读且按
Process 隔离；子进程不会继承父进程的调用身份。直接调用工具时返回 `ok=false`，调用方不应
自行伪造或重新绑定 ToolCall。

同步 `runtime.NewAgentTool` 的每次调用拥有独立 child Process，因此同一 model round 的
多个调用可以并发。Runtime 用 exact `ToolCall.ID` 关联并在 suspension checkpoint 中记录有序
child forest；多个
child 同时 waiting 时，parent 一次只暴露最早未提交的 suspension，恢复一个后再暴露下一个。

Host 若已经用自己的控制面确定性结束了一个 paused call（例如取消该调用拥有的 delegated
child），应使用 `Checkpoint.CompletePausedCall` 生成新 checkpoint。它只写入结果事实，
不执行工具也不推进可观察顺序：若结算的是当前边界，后续用 `Runner.Continue`；若结算的是
尚未轮到的 sibling，当前 `AwaitingInput` 保持不变，普通 `Runner.Resume` 会先消费当前回答，
再按模型顺序发布已结算 sibling。checkpoint v4 是唯一接受的 shape，不保留 v3 reader。
若 paused tool 自己的 durable dependency 已在 ToolLoop 外变为可继续状态，Runtime 使用
`Runner.ContinuePaused` 重新进入该 tool；该路径不携带外部输入，也不发布 `Resume` event。
Runner 只允许显式实现 `InputlessContinuationTool` 的 tool 走此路径；普通 HITL tool 会在
调用前被拒绝，通用 Host 不能借此绕过仍在等待用户输入的 tool。

## 10. API 与 wire 治理

Framework 使用两层自动门禁：

- `internal/arch/testdata/exported_api.txt` 锁定所有公共 package 的 exported API；
- wire fixture 只锁定可恢复执行所需的 ProcessSnapshot、Suspension 与 toolloop Checkpoint JSON shape。

Agent 事件的 discriminator 使用 `event.Kind` 与 `event.KindProcessCreated` 等规范常量；listener
不需要比较自由字符串。生命周期事件与模型/工具边界都是进程内强类型值，不承担外部 JSON 投影。
模型、工具、暂停和恢复边界的 discriminator 与 Resume payload 由 `interaction` 唯一拥有；
`toolloop` 只复用同一类型，不再维护一份可能漂移的平行协议。
Suspension、Pause、Checkpoint 与 Resume 的稳定 ID 统一通过 `interaction.ValidateID` 校验；
真正持久化的 Suspension 与 Checkpoint 严格拒绝未知字段和 trailing value，协议版本漂移不会被静默吞掉。
托管交互和 app/runtime 共享 `interaction.StopReason`，停止原因的值与 `Valid` 规则只有一个 owner。
ProcessSnapshot 不保存重复的 action audit history；`OwnUsage.Actions` 是恢复预算准入所需的
唯一执行计数，ActionStarted/ActionFinished 事件与 tracing 承担审计输出。恢复路径不会解析
自由字符串或为未知值猜测降级状态。
开发阶段允许破坏性调整，但每次都要把调用方、examples、GoDoc、API baseline、wire fixture 和
迁移文档一次性收口，不保留 alias/shim。存储 contract tests 属于定义该消费侧接口的 Host，
不回流 Agent 模块。

提交前至少运行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

更完整的门禁与阶段进度见架构执行计划。
