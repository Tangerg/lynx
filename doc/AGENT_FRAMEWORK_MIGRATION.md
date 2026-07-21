# Agent Framework 开发期迁移指南

> 目标：一次性切换到当前框架语言与 API
> 策略：不保留兼容 alias、deprecated wrapper、双读、双写或旧 snapshot decoder

本文面向当前仓库消费者。它不是稳定版本的兼容承诺，而是开发期直接迁移清单。

## 1. 公共语言映射

| 旧语言 | 当前语言 | 语义变化 |
|---|---|---|
| Platform | Engine | 框架生命周期与资源 owner |
| AgentRef | DeploymentRef | 指向已编译部署，不指向可变定义 |
| AgentProcess | Process | 运行聚合，不重复 Agent 前缀 |
| PlanningSystem | planning.Domain | planner 可见的领域能力集合 |
| ConditionWorldState | planning.State | 条件世界状态的具体实现 |
| Determination | Truth | 三值逻辑：Unknown / False / True |
| IOBinding | Binding | Action 输入输出共用的类型化绑定 |
| Invocation | ModelCall / EmbeddingCall / ActionRun | 按真实边界命名，不用万能名词 |
| Services / ServiceProvider | Dependencies | typed、hierarchical、single-assignment scope |
| GoalExport | GoalTool | Goal 暴露为工具的元数据 |
| autonomy | routing | 自然语言到 Agent/Goal 的路由能力 |
| PromptRunner | ProcessContext.Prompt | 普通模型调用直接挂在 Action capability 上 |

迁移时直接删除旧封装，不要在应用层再造同名桥接类型。

## 2. Agent、Goal 与 Engine

定义使用普通 config literal，构造后只读：

```go
writer := agent.New(agent.AgentConfig{
    Name:        "writer",
    Version:     "1.2.3",
    Description: "write a post",
    Actions: []agent.Action{
        agent.NewAction("write", write, agent.ActionConfig{}),
    },
    Goals: []*agent.Goal{
        agent.NewOutputGoal[Post](agent.GoalConfig{Name: "post-ready"}),
    },
})

engine, err := agent.NewEngine(agent.EngineConfig{})
if err != nil {
    return err
}
deployment, err := engine.Deploy(ctx, writer)
if err != nil {
    return err
}
_ = deployment.Ref()
```

常用入口直接映射：

| 旧调用 | 当前调用 |
|---|---|
| `NewPlatform` / `MustNewPlatform` | `NewEngine` / `MustNewEngine` |
| `RunAgent` / `StartAgent` | `Engine.Run` / `Engine.Start` |
| 按名称回查定义 | `ActiveDeployment(name)` / `Deployment(ref)` |
| 同名部署隐式覆盖 | `Deploy` 幂等，`Replace` 显式切换 |
| 从 Agent 指针运行 child | 传同一 Engine 的 exact `*Deployment` |

所有会传播生命周期事件的 Engine 操作都把 `context.Context` 作为首参数；没有无 context
重载或 `FooContext` 兼容入口。`Start` 返回 `(*Process, <-chan error, error)`，
`ContinueAsync` 返回 `(<-chan error, error)`：构造/admission 错误同步返回，channel 只承载已经
启动的后台执行结果。

已运行 Process 永久绑定 `DeploymentRef{Name, Version, Digest}`。不要只持久化 Agent 名称，也不要接受其他 Engine 的 Deployment handle。
`AgentConfig.Version` 和 `DeploymentRef.Version` 的非空值必须使用规范
`MAJOR.MINOR.PATCH` SemVer（例如 `1.2.3`）；不要传 `v1.2.3`、`1.2` 或首尾带空白的值。
DeploymentRef 的 Name/Digest 同样拒绝首尾空白。runtime 会先冻结 Action/Condition metadata，
再让内建规则和 `AgentValidator` 校验该冻结快照，因此 validator 不应依赖 source Agent 指针身份。

## 3. Action 与 Binding

Action 统一返回 `(ActionStatus, error)`。status 表达 succeeded、waiting、paused、failed；error 表达失败细节、replan 或 suspension。

```go
func (a *PublishAction) Execute(
    ctx context.Context,
    process *core.ProcessContext,
) (core.ActionStatus, error) {
    // ...
    return core.ActionSucceeded, nil
}
```

删除任何 `LastError`、`TakeError`、`ExecuteSafely` 或 Blackboard error key。Middleware 必须透传同一结果对。

普通 typed Action 默认使用名为 `it` 的输入和输出 Binding。自定义名称只通过：

```go
core.ActionConfig{
    Inputs:  []core.Binding{core.NewBinding[Topic]("topic")},
    Outputs: []core.Binding{core.NewBinding[Post]("post")},
}
```

不要恢复重复的 `InputBinding` / `OutputBinding` 快捷字段。

## 4. Retry、stuck 与并发

`RetryPolicy{}` 与 `DefaultRetryPolicy()` 都只执行一次。多次尝试必须显式声明副作用安全性：

```go
core.RetryPolicy{
    MaxAttempts: 3,
    BaseDelay:   100 * time.Millisecond,
    MaxDelay:    time.Second,
    Safety:      core.RetrySafetyIdempotent,
}
```

`StuckDecision` 的零值是 `StuckStop`；需要重新规划时返回 `StuckReplan`。不要依赖“未设置等于继续”的危险语义。
策略返回值可以用 `StuckDecision.Valid` 校验或用 `String` 记录。runtime 会拒绝未知值，
并要求 `StuckReplan` 在下一次观察前产生可见进展（例如修改 blackboard/conditions，或通过
清除 exclusion 使计划恢复）；相同 WorldState 再次无计划时会进入 `StatusStuck`，不会无限重试。
策略给出的 `StuckResult.Reason` 现在会进入 `event.ProcessStuck` 及其 JSON payload 的可选
`reason` 字段，事件消费者应按新增可选字段处理。

并发提交 `TerminateAction` 与 `TerminateAgent` 时，Agent scope 始终胜出；同 scope 的首个
原因被保留。调用方不应依赖 goroutine 调度决定最终终止边界。

所有 `ScoreFunc` 必须返回有限值；作为 Cost 使用时还必须非负。NaN、Inf、负 cost 或 panic
都会使 planning 明确失败，不再被 Utility 当作 0 或被排序器静默保留。Routing 的
`Choice.Confidence` 必须是 `[0,1]` 内的有限值；`ModelRanker` 对越界值、重复 ID 和未知 ID
返回错误，不再自动 clamp。自定义 Ranker 也受 Router 的同一结果合同约束。

删除 process-wide candidate-action 并发。Process tick 仍按计划稳定执行一个 Action；
需要业务级 fan-out 时使用：

- `workflow.Parallel`、`ScatterGather` 或 `Consensus`：同一 Process 内的结构化 fan-out；
- child Process：需要独立暂停、终止、持久化或生命周期时。

这不等于 ToolLoop 串行。一个模型响应中的多个 tool call 采用“并发执行、顺序提交”：

- 工具默认独占；只有实现 `toolloop.ConcurrentTool` 才允许重叠；
- `concurrent=true, key=""` 表示调用之间没有已知资源冲突；
- 相同的非空 resource key 串行，不同 key 可并发；
- `toolloop.Config.MaxConcurrentCalls` 或
  `interaction.Limits.MaxConcurrentToolCalls` 限制同时执行数，零值使用框架默认值；
- `ToolResult` event、continuation message、checkpoint `NextResult` 永远按模型原始
  tool-call 顺序推进，不能按 goroutine 完成顺序写入 history 或 cache。

例如按租户串行、跨租户并发：

```go
func (t *TenantTool) ConcurrencyKey(arguments string) (string, bool) {
    tenantID, err := decodeTenantID(arguments)
    if err != nil {
        return "", false
    }
    return "tenant:" + tenantID, true
}
```

### Tool policy names and scope

Tool decorators now use short names that remain clear after package
qualification. Development consumers must migrate directly; no compatibility
aliases remain:

| 旧调用 | 当前调用 |
|---|---|
| `toolpolicy.OnceOnly` | `toolpolicy.Once` |
| `toolpolicy.Unlocked` | `toolpolicy.Gate` |
| `toolpolicy.UnlockCondition` | `toolpolicy.Condition` |
| `toolpolicy.LoopScope` | `toolpolicy.WithScope` |
| `toolpolicy.ErrToolAlreadyCalled` | `toolpolicy.ErrAlreadyCalled` |
| `toolpolicy.ErrToolLocked` | `toolpolicy.ErrLocked` |

需要每轮独立 allowance 时显式创建 scope：

```go
ctx = toolpolicy.WithScope(ctx)
once, err := toolpolicy.Once(search)
```

同一 scope 按工具名共享调用记录。没有 scope 时，allowance 属于返回的 decorator
实例：该实例生命周期内只有第一次调用成功；它不隐式猜测 Process 或 Session
生命周期。`Gate` 每次 `Call` 才求值 condition，调度器读取文件目标等元数据不会提前
消费 `Once` 状态，也不会执行 condition。两种有状态 decorator 均保持独占，不透传
内层工具的并发声明。

## 5. Chat、Prompt 与 ToolLoop

Engine 接受 provider-neutral chat capability：

```go
engine, err := agent.NewEngine(agent.EngineConfig{
    Chat: agent.ChatCapability{
        Model:    client,
        Streamer: client,
    },
})
```

`Streamer` 不能在没有 `Model` 时单独存在。每 Process 的模型选择通过 `core.ChatProvider` extension 完成。

Action 内普通调用：

```go
answer, err := process.Prompt(ctx, prompt, agent.PromptConfig{
    System: "Answer concisely.",
    Tools:  []tools.Tool{search},
})
```

结构化输出使用 `PromptJSON[T]`；需要完整 request、observer 或 limits 时使用 `Interact`。

`workflow.SupervisorConfig.Render` 现在返回 `(string, error)`。默认 renderer 只做 JSON 编码；
编码失败会直接结束 action，不再用 `fmt.Sprintf` 生成另一种、不可预测的 prompt：

```go
Render: func(input Request) (string, error) {
    return renderPrompt(input)
},
```

`RepeatUntilAcceptable` 的 Evaluator error 同样直接传播。需要容错时由业务 Evaluator 明确
返回合法 Feedback；框架不再把基础设施错误伪装成 `Score: 0` 并隐式执行下一轮。
`Loop`、`RepeatUntil` 和 `RepeatUntilAcceptable` 只把零值 MaxIterations 解释为默认值；负数
现在是配置错误。AcceptableScore 同样只以零值选择默认阈值，负数不再被默认为 0.7。

低层 `toolloop` 不再构造 `Invocation`：

```go
runner, err := toolloop.NewRunner(model, toolloop.Config{
    MaxRounds:          8,
    MaxConcurrentCalls: 4,
})
if err != nil {
    return err
}
for event, runErr := range runner.Run(ctx, request, resolver) {
    // ...
}
```

Host 不直接使用 ToolLoop 复制 Framework 的 usage、checkpoint 或 Process 状态机。

ToolLoop checkpoint 已升级到 schema v2。旧 `Results` / `NextCall` 被按模型调用位置
对齐的 `CallStates` / `NextResult` 取代，每个 call 明确处于 `queued`、`completed`
或 `paused`。这允许一个并发批次中多个工具同时完成或暂停，同时只向外暴露顺序上
最早的 pause。checkpoint 同时保存 `MaxConcurrentCalls`；恢复 Runner 必须使用完全
相同的宽度，避免重启后调度政策静默漂移。旧 schema v1 checkpoint 不兼容读取；
开发数据应与所属 waiting ProcessSnapshot 一并清除。

## 6. HITL

Typed interrupt 只有值与错误：

```go
approved, err := hitl.Interrupt[bool](ctx, "publish-approval", prompt)
if err != nil {
    return Output{}, err
}
```

首次调用返回统一 suspension error；恢复后同一调用点返回解码后的值。Host 分两步处理：

```go
if err := engine.Resume(process.ID(), suspension.ID, response); err != nil {
    return err
}
return engine.Continue(ctx, process.ID())
```

`Resume` 只记录响应，`Continue` 才重新进入执行。不要保存 continuation closure、handler、SDK client 或 runtime pointer。

Durable Host 不应加载或解释 `ProcessSnapshot` 来判断能否恢复。启动清理使用：

```go
resumable, err := engine.Resumable(ctx, processID)
```

真正恢复使用：

```go
process, err := engine.RestoreResumable(ctx, processID, options)
if errors.Is(err, runtime.ErrResumableSnapshotLost) {
    // 将拥有该 process 的应用执行恢复为 lost。
}
```

`false, nil` 表示 snapshot 缺失、损坏、不是 waiting continuation，或 exact
deployment 不属于当前 Engine；持久化访问错误仍返回 error。删除 Host 自己对
snapshot status、suspension payload 或 deployment digest 的判断。

## 7. Agent 工具

当前工具 API：

- `runtime.NewAgentTool[In, Out]`：父 Process 内同步 child；
- `runtime.NewStandaloneAgentTool[In, Out]`：独立顶层执行；
- `runtime.NewAgentTaskTools[In, Out]`：后台 start/result 工具对；
- `engine.GoalToolsFor(...)`：指定部署的 Goal tools；
- `engine.GoalTools()` / `engine.StandaloneGoalTools()`：遍历活动部署。

工具在构造时解析并捕获 exact Deployment，不在每次调用时重新按名称选择版本。

`NewAgentTool` 实现并发工具能力：同一模型响应中的多个 AgentTool 调用各自拥有隔离
child Process，可以并发启动。Runtime 使用模型生成的 exact `ToolCall.ID` 关联 child，
即使工具名和 arguments 完全相同也不会混淆。若多个 child 同时 waiting，parent
checkpoint 保存按 tool-call 顺序排列的 child forest；Host 仍只对 parent Process ID
调用 `Resume` / `Continue`，Runtime 每次恢复当前最早未提交分支，其余 sibling 保持
parked。消费者不要自行排序 child，也不要用工具名或参数摘要充当并发调用身份。

本轮进一步把对象作为首参的 API 直接收回 receiver，不保留 wrapper：

| 旧调用 | 当前调用 |
|---|---|
| `core.EncodeBlackboard(agent, named, objects)` | `agent.EncodeBlackboard(named, objects)` |
| `core.DecodeBlackboard(named, objects, agent)` | `agent.DecodeBlackboard(named, objects)` |
| `planning.PlanGoals(..., domain, ...)` | `domain.Plans(...)` |
| `planning.BestPlan(..., domain, ...)` | `domain.BestPlan(...)` |
| `planning.Prune(..., domain, ...)` | `domain.Prune(...)` |
| `runtime.GoalToolsFor(engine, names...)` | `engine.GoalToolsFor(names...)` |
| `runtime.GoalTools(engine)` | `engine.GoalTools()` |
| `runtime.StandaloneGoalTools(engine)` | `engine.StandaloneGoalTools()` |
| `core.AllowsPermissions(allowed, required)` | `requirement.Allows(required)` |

## 8. Dependencies

动态领域依赖改为 typed scope：

```go
var SearchKey = core.MustDependencyKey[Search]("search")

if err := core.RegisterDependency(engine.Dependencies(), SearchKey, search); err != nil {
    return err
}

search, err := core.LookupDependency(process.Dependencies(), SearchKey)
```

静态依赖仍优先构造函数或字段注入。不要把 `Dependencies` 包装回全局 service locator。

## 9. Child 执行策略

Agent 默认仍只给 child 复制其声明的 blackboard 模式，并传播 process event
listener；不会隐式继承 provider、dependency、budget、guardrail 或其他 host
能力。需要让一个应用 Run 的策略覆盖完整委派树时，在 root
`ProcessOptions` 上显式安装 `ChildOptions`：

```go
options.ChildOptions = func(
    ctx context.Context,
    parent agent.ProcessView,
    child *agent.Agent,
) (agent.ProcessOptions, error) {
    dependencies := engine.Dependencies().Child()
    if err := core.RegisterDependency(dependencies, RunPolicyKey, policyFor(parent)); err != nil {
        return agent.ProcessOptions{}, err
    }
    return agent.ProcessOptions{
        Dependencies: dependencies,
        Extensions:   []agent.Extension{selectedChatProvider},
    }, nil
}
```

回调对每个 child 创建执行一次，并默认继续传给更深的后代；返回值中的非 nil
`ChildOptions` 可替换后续策略。返回 nil `Blackboard` 保留调用入口选择的 child
blackboard 模式。不要使用全局变量或私有 context key 偷渡 Run 策略。

`interaction.Limits.MaxSteps` 仍只限制当前一次 managed interaction；
`MaxModelCalls` 限制当前 Process 及其后代已经记录的累计模型调用数，适合由
Host 将一个应用级 step budget 覆盖到完整委派树。`MaxConcurrentToolCalls`
只限制一次 model round 中 conflict-free tool call 的执行宽度，不改变结果提交顺序。
所有 Limits 必须是有限非负值；`Limits.Validate` 会拒绝负数、NaN 和 Inf。

usage 记账现在显式返回错误：

```go
if err := process.RecordModelCall(ctx, call); err != nil {
    return Output{}, err
}
```

`ModelCall.Validate`、`EmbeddingCall.Validate` 和 `Budget.Validate` 是同一领域不变量的公开
入口。运行时会补齐零 Timestamp，再在写入账本前校验；非法值或累计 cost/token 溢出不会
产生事件，也不会部分修改账本。迁移时不要再忽略 `RecordUsage` / `RecordModelCall` /
`RecordEmbeddingCall` 的返回值。

`Engine.Kill` 现在会取消目标 Process 的活动 Run / Continue 上下文并递归终止
其存活后代；Action、provider 调用和同步 child 应始终监听传入的 context。
递归委派深度通过 `runtime.Config.MaxChildDepth` 明确配置，零值使用
`runtime.DefaultMaxChildDepth`；不再依赖包内隐藏常量。

同步 `runtime.NewAgentTool` 的 waiting 语义已改为真正的 nested suspension：
parent Process 进入 Waiting，Host 对 parent ID 调用 `Resume` / `Continue`，Runtime
恢复 exact child 与原 tool-call checkpoint。删除 parent action 中解析
`{"status":"waiting"}` 并自行寻找 child process 的逻辑。外部
`NewStandaloneAgentTool` 和 `NewAgentTaskTools` 继续使用结构化 waiting JSON。

## 10. SessionStore、ProcessStore 与 snapshot

Session 持久化现在只要求 Runtime 实际消费的读写能力：

```go
type SessionStore interface {
    Load(context.Context, string) (Session, error)
    Save(context.Context, Session) error
}
```

删除与列表不再强加给每个后端；需要管理能力时分别实现 `SessionDeleter` 与
`SessionLister`。旧实现无需删除方法，但依赖宽 `SessionStore` 接口调用 `Delete` / `List`
的消费者必须改为断言对应可选能力。

`runtime.Config` 把两个生命周期分开：

- `SessionStore` 只保存 `RunInSession` 的 root multi-turn Session；
- `ChildSessionStore` 只保存委派产生的 child Session；
- 一个后端确实拥有两类生命周期时，可以把同一实现显式配置到两个字段。

不要再把“只保存 subtask lineage”的产品 adapter 塞进 `SessionStore` 并伪造完整 CRUD。
`RunInSession` 现在按值接收 `Session`，入口通过 `Session.Clone` 取得 metadata 所有权，
因此并发 turn 不再共享调用方的可变 Session 指针。运行时派生的 AgentName 与 UpdatedAt
写入配置的 SessionStore，不会反向修改调用方值；需要最新状态时从 store 读取。
它会先调用 `Session.Validate`，用显式 Agent 时通过 `BindAgent` 固定 identity；
传 nil Agent 时按 `Session.AgentName` 查找 active Deployment。Agent 名称冲突在部署/运行前
以 `ErrInvalidSession` 失败。最终 Session save 使用保留 context value 但脱离 request
cancel 的 context，并用 `errors.Join` 同时保留运行错误和持久化错误。

默认 Session turn 协调器现在显式按到达顺序授权，同一 Session 串行、不同 Session 并行，
等待取消不会阻塞后续 turn。自定义 `SessionTurnSequencer` 也必须提供 FIFO 语义。该端口不
携带 fencing token，因此不能单独充当跨节点执行租约；分布式 Host 必须在外层拒绝陈旧 owner。

自定义 Session 后端运行公共契约：

```go
if err := storetest.TestSessionStore(t.Context(), store); err != nil {
    t.Fatal(err)
}
```

契约覆盖 validation、replace、nested metadata ownership、并发 Save、not-found，以及实现
声明的可选 List/Delete 行为。

最小存储合同：

```go
type ProcessStore interface {
    Load(context.Context, string) (ProcessSnapshot, error)
    Save(context.Context, ProcessSnapshot) error
}
```

`core` 不再导出具体内存 Store。测试代码把 `core.NewMemoryProcessStore` 和
`core.NewMemorySessionStore` 分别替换为 `storetest.NewMemoryProcessStore` 和
`storetest.NewMemorySessionStore`；生产 Host 应继续注入自己的适配器。

`ProcessSnapshot.Revision` 是 CAS 的唯一 expected revision；新 Process 使用 0，成功提交
持久化为 `Revision+1`。包含嵌套子进程的原子树快照额外要求实现
`SaveBatch(context.Context, []ProcessSnapshot) error`。

自定义实现运行公共契约：

```go
if err := storetest.TestProcessStore(t.Context(), store); err != nil {
    t.Fatal(err)
}
```

`storetest` 保持公开；`providertest` 已移除。ChatProvider 与 ToolGroupResolver 在真实 Engine dispatch 测试中验证，不为测试对称性扩大公共 API。

ProcessSnapshot 已升级到 schema v2，并只持久化当前 Process 的直接 ledger：

| schema v1 | schema v2 |
|---|---|
| `Cost` / `cost` | `OwnCost` / `own_cost` |
| `Tokens` / `tokens` | `OwnTokens` / `own_tokens` |
| `ModelCalls` / `model_calls` | `OwnModelCalls` / `own_model_calls` |
| `EmbeddingCalls` / `embedding_calls` | `OwnEmbeddingCalls` / `own_embedding_calls` |

子进程用量保存在各自 snapshot 中，Restore 按 parent-child linkage 重建聚合；不再从
父级 aggregate 猜测并减去 child suffix。运行时需要聚合值时使用
`Process.Usage()`、`Process.ModelCalls()` 和 `Process.EmbeddingCalls()`，不要把
snapshot 的 `Own*` 字段当作进程树总量。

旧 snapshot 不做兼容读取。开发环境迁移时：备份数据、终止依赖旧 snapshot 的非终态运行、
清除 schema v1 ProcessSnapshot 及其 v1 ToolLoop checkpoint、保留可独立解释的 Session
与 terminal history，然后只写当前 schema。

## 11. Event JSON

直接 `json.Marshal(event.ProcessSnapshotFailed{...})` 的消费者需要切换到统一事件
envelope：

```json
{
  "kind": "process_snapshot_failed",
  "timestamp": "...",
  "process_id": "...",
  "payload": {
    "policy": "report_only",
    "error": "store unavailable"
  }
}
```

该类型此前遗漏了自定义 marshaler，Go 默认编码无法输出 opaque Header，也会丢失
`Err`。现在它满足 `event` package 对所有具体事件的既有契约。内存中的 concrete
event、listener type switch 与 durable ProcessSnapshot 均不受影响；不要为旧的默认
JSON shape 增加双写或兼容 wrapper。

## 12. 推荐执行顺序

1. 切换 Engine、DeploymentRef 与 Process 公共语言。
2. 迁移 Agent/Goal config 与 Action/Binding。
3. 迁移 ChatCapability、Prompt/Interact 与 ToolLoop 调用。
4. 迁移 HITL suspension 与 Resume/Continue。
5. 迁移 Dependencies、child、workflow 和 agent tools。
6. 升级 ProcessStore CAS 与 snapshot 数据。
7. 删除旧 wrapper、alias、旧文件和旧数据路径。
8. 运行 Agent 与 App 的 build、vet、test、race、lint、tidy、API/wire/architecture gate。

当前用法见 [`../agent/docs/GUIDE.md`](../agent/docs/GUIDE.md)，扩展规则见 [`../agent/docs/EXTENSION_DESIGN.md`](../agent/docs/EXTENSION_DESIGN.md)。
