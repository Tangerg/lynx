# Lynx Agent Framework 使用指南

本文只描述当前代码。符号级契约以 GoDoc、API baseline 和 wire fixture 为准；架构目标、阶段进度与决策见
[`../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/AGENT_FRAMEWORK_ARCHITECTURE_EXECUTION_PLAN.md)。

## 1. 心智模型

Lynx Agent 是一个可嵌入的 Go framework，不是 DI 容器，也不是 provider SDK。它分为三层：

- `agent/core` 定义 Agent、Action、Goal、Condition、Blackboard、ProcessView 和扩展协议；
- `agent/runtime` 的 `Engine` 拥有部署、进程、执行循环、挂起恢复、事件和持久化协调；
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
    deployment, err := engine.Deploy(writer)
    if err != nil {
        panic(err)
    }
    fmt.Println("deployed", deployment.Ref())

    process, err := engine.Run(context.Background(), writer, map[string]any{
        agent.DefaultBindingName: Topic{Title: "Go agents"},
    }, agent.ProcessOptions{})
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

完整示例见 [`../examples/hello`](../examples/hello)、[`../examples/blog`](../examples/blog)
和 [`../examples/blogllm`](../examples/blogllm)。

## 3. Definition、Binding 与 Deployment

`AgentConfig` 是构造输入；`Agent` 是只读定义聚合。`Engine.Deploy` 会执行结构验证、扩展验证，
并编译出不可变 `Deployment`：

- 相同名称、版本和定义摘要的重复 Deploy 是幂等的；
- 同名但定义不同会返回 `ErrDeploymentConflict`；
- 明确切换活动版本使用 `Engine.Replace`；
- `Engine.Deployment(ref)` 可读取活动或历史部署；
- `Engine.Undeploy(name)` 只移除活动路由，不破坏历史定义。

`NewAction[In, Out]` 默认生成一个名为 `it` 的输入和输出 Binding。需要自定义名字或多个
Binding 时，使用 `ActionConfig.Inputs`、`Outputs` 和 `core.NewBinding[T]`。`ActionConfig`
保持可用零值，常用可选项包括：

- `Preconditions`、`Effects`：显式业务条件；
- `Repeatable`：允许同一进程多次选择该 Action；
- `Retry`：显式重试契约；
- `Cost`、`Value`：Planner 评分函数；
- `ToolGroups`：抽象工具能力及允许权限；
- `ClearWorkingState`：成功后清理普通工作状态，保留 protected ambient state。

Action 默认只执行一次。`RetryPolicy` 的零值与 `DefaultRetryPolicy()` 都表示一次尝试；
`MaxAttempts > 1` 必须声明 `RetrySafetyIdempotent` 或 `RetrySafetyCompensated`。Framework
不会猜测外部副作用是否安全。

## 4. Engine 与 Process 生命周期

`Engine` 是 framework 级主对象，支持多实例，没有 package-global registry：

- `Run`：同步驱动到终态或 Waiting；
- `Start`：返回 Process 与只发送一次结果的完成 channel；
- `Continue` / `ContinueAsync`：继续已存在的非终态 Process；
- `Resume`：校验并记录 Suspension 响应，不暗中启动执行；
- `Kill`、`Remove`、`Prune`：显式生命周期管理；
- `Process`、`Processes`：读取当前 registry 快照；
- `Save`、`Restore`、`RestoreSnapshot`：durable process 协调。

同一 Process 同时只能有一个 active owner 驱动执行。CAS 防止旧快照覆盖新快照，但不负责
跨节点选主；多节点 handoff 仍需 Host 提供 lease/fencing。

观察者只应依赖 `core.ProcessView`。Action 的可变能力集中在 `ProcessContext`：Blackboard、
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
`core.ChatProvider` extension 完成。Runtime 统一叠加 conversation ID、history 与
`ChatGuardrails`，这些执行状态不会进入 provider Request/Response。

Action 内的常用调用入口是：

```go
answer, err := process.Prompt(ctx, prompt, agent.PromptConfig{
    System:  "Answer concisely.",
    Options: &chat.Options{Model: "provider-model-id"},
    Tools:   []tools.Tool{searchTool},
})
```

需要结构化结果时使用 `agent.PromptJSON[T]`。需要完全控制 Request、observer 或 budget 时，
使用 `ProcessContext.Interact`。所有这些入口最终进入 framework-managed interaction，由
Runtime 记录 model call、usage、事件、限制和可恢复 tool checkpoint。

`agent/toolloop.Runner` 是可独立复用的叶子执行器，不是第二套 Agent runtime。直接使用它的
调用方自行负责 Process、usage、事件和持久化。

Agent 可通过以下 helper 暴露为工具：

- `runtime.NewAgentTool[In, Out]`：父 Process 内同步调用一个子 Agent；
- `runtime.NewStandaloneAgentTool[In, Out]`：无父 Process 的独立工具调用；
- `runtime.NewAgentTaskTools[In, Out]`：后台 start/result 工具对；
- `runtime.GoalToolsFor`：指定已部署 Agent 的 Goal 工具；
- `runtime.GoalTools` / `StandaloneGoalTools`：按 GoalTool 元数据批量生成。

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
if err := engine.Resume(process.ID(), suspension.ID, response); err != nil {
    return err
}
if err := engine.Continue(ctx, process.ID()); err != nil {
    return err
}
```

`Resume` 只提交响应；`Continue` 才重新进入 Action。Human 输入与 Tool pause 使用同一
Suspension 协议，tool checkpoint 保存在 Suspension payload 内，不使用私有 Blackboard key。

## 7. Snapshot、Store 与 Session

`core.ProcessSnapshot` 使用严格 JSON 解码：未知 schema、未知字段、trailing value、无效 enum、
DeploymentRef 不匹配或 checkpoint correlation 错误都会 fail closed。普通 Blackboard 值默认
durable；运行时 handle、函数、channel、client 等必须通过以下 API 显式标记为 transient：

- `StoreTransient`
- `BindTransient`
- `AddTransient`

Framework 自带 `MemoryProcessStore` 和 `MemorySessionStore` 作为 reference implementation。
外部 ProcessStore 应运行公开 contract suite：

```go
if err := storetest.TestProcessStore(t.Context(), store); err != nil {
    t.Fatal(err)
}
```

`ProcessOptions.Session` 与 `Engine.RunInSession` 管理多 turn identity；模型对话内容仍由
`chathistory` 维护，不写进 Agent Blackboard 或 provider Response。

## 8. Extension 与 Dependencies

所有行为扩展先实现：

```go
type Extension interface { Name() string }
```

Runtime 再按最小 capability interface 发现 `planning.Planner`、`ActionMiddleware`、
`ToolMiddleware`、`AgentValidator`、`GoalApprover`、`ToolGroupResolver`、`ChatProvider`、
`StopPolicy`、`IDGenerator`、`Blackboard` 和 `EventListener`。Engine scope 来自
`runtime.Config.Extensions`；Process scope 来自 `core.ProcessOptions.Extensions`。

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
| `RunChild` | 仅 protected ambient state | 默认、安全的自包含委派 |
| `RunChildIsolated` | 全新状态，仅绑定显式 input | loop、pipeline、parallel branch |
| `StartChild` | 与 `RunChild` 相同，后台执行 | 可稍后读取结果的任务 |

Child 使用精确 Deployment、独立 Session、父预算子树，并继承父 Process 的 EventListener；
其他 Process extension、guardrails 和 dependency override 不会被隐式复制。

`workflow.Sequence`、`Parallel`、`Loop`、`RepeatUntil`、`RepeatUntilAcceptable`、
`ScatterGather`、`Consensus` 和 `Supervisor` 最终都编译回普通 Agent。并行 branch 在启动
goroutine 前获得独立 Blackboard 和 Dependencies child；写入不合并，只有返回值按声明顺序
join。需要独立暂停/终止能力的并行单元应使用 Child Process。

## 10. API 与 wire 治理

Framework 使用两层自动门禁：

- `internal/arch/testdata/exported_api.txt` 锁定所有公共 package 的 exported API；
- wire fixture 锁定 ProcessSnapshot、Suspension、toolloop、event 等稳定 JSON shape。

开发阶段允许破坏性调整，但每次都要把调用方、examples、GoDoc、API baseline、wire fixture 和
迁移文档一次性收口，不保留 alias/shim。`storetest` 是故意公开的外部实现 contract package，
命名遵循标准库 `fstest`、`slogtest` 的惯例，不应移动到 `internal`。

提交前至少运行：

```bash
go test ./...
go test -race ./...
go vet ./...
```

更完整的门禁与阶段进度见架构执行计划。
