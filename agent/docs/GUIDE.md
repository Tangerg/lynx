# Lynx Agent 使用指南

本文只描述当前实现。符号级细节以 GoDoc 和源码为准；架构演进记录见
[`../../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md`](../../doc/CORE_ARCHITECTURE_EXECUTION_PLAN.md)。

## 1. 心智模型

一个 Agent 是不可变定义：

- `Action` 描述前置条件、效果、成本和执行函数；
- `Goal` 描述期望成立的条件；
- `Condition` 从 Blackboard 读取世界状态，返回 True/False/Unknown；
- `Blackboard` 保存 action 的输入、产物和显式条件；
- `Planner` 根据当前世界状态选择能推进 Goal 的 action 序列；
- `Platform` 负责部署、创建进程、执行 tick、事件、暂停/恢复和持久化。

Action 之间不直接传参。一个 action 的返回值由 runtime 写入 Blackboard，后续
action 再按名字和类型读取。这样 planner 才能看见“已经知道什么”和“还缺什么”。

## 2. 最小 Agent

```go
package main

import (
    "context"
    "fmt"

    "github.com/Tangerg/lynx/agent"
    "github.com/Tangerg/lynx/agent/core"
    "github.com/Tangerg/lynx/agent/runtime"
)

type Topic struct{ Title string }
type Post struct{ Body string }

func main() {
    writer := agent.New("writer").
        Actions(agent.NewAction("write",
            func(_ context.Context, _ *core.ProcessContext, topic Topic) (Post, error) {
                return Post{Body: "About " + topic.Title}, nil
            },
            core.ActionConfig{},
        )).
        Goals(agent.GoalProducing[Post](core.Goal{Description: "post produced"})).
        Build()

    platform := agent.NewPlatform(runtime.PlatformConfig{})
    if err := platform.Deploy(writer); err != nil {
        panic(err)
    }
    process, err := platform.RunAgent(context.Background(), writer, map[string]any{
        core.DefaultBindingName: Topic{Title: "Go agents"},
    }, core.ProcessOptions{})
    if err != nil {
        panic(err)
    }
    post, _ := core.ResultOfType[Post](process)
    fmt.Println(post.Body)
}
```

完整可运行版本见 [`../examples/hello`](../examples/hello) 和
[`../examples/blog`](../examples/blog)。

## 3. Planner 与 workflow

`agent.NewPlatform` 默认注册 GOAP 和 reactive planner。Agent 未指定
`PlannerName` 时使用 `goap`；HTN 和 utility planner 需要作为 extension 显式注册。
`planning.Planner` 只有 `PlanToGoal` 一个算法方法，`PlansToGoals`、
`BestValuePlan` 和 `Prune` 是包级组合函数。

`workflow` 中的 `Sequence`、`Parallel`、`Loop`、`RepeatUntil`、
`ScatterGather`、`Consensus` 和 `Supervisor` 都产生普通 `*core.Agent`。它们不创建
第二套 runtime，部署、事件和持久化仍遵循同一套 Platform 规则。

## 4. LLM 调用

Provider adapter 只实现 `core/chat.Model`，高层默认值、middleware 和调用便利面由
`chatclient.Client` 持有。把 client 注入 Platform 后，action 通过
`ProcessContext.Chat()` 或 `ProcessContext.PromptRunner()` 使用：

```go
answer, err := pc.PromptRunner().
    WithSystem("Answer concisely.").
    WithOptions(&chat.Options{Model: "provider-model-id"}).
    Generate(ctx, prompt)
```

`PromptRunner` 构造普通 `core/chat.Request`。请求中只有可序列化的消息、工具定义和
options；可执行的 `tools.Tool` 保存在邻接 `tools.Registry` 中。没有工具时
`Stream` 直接转发 `chatclient` 的 `iter.Seq2`；存在工具时由同步 Event Runner 驱动，
最终文本作为一个元素返回，不伪造流式 tool-loop。

每进程模型覆盖通过 `core.ChatClientProvider` extension 完成。它返回
`*chatclient.Client`，不让 provider SDK 或 client builder 泄漏进 Agent Core。

## 5. 工具与 Event Runner

唯一的工具执行契约是 `tools.Tool`，模型可见部分是
`core/chat.ToolDefinition`。`tools.Registry` 同时满足 `toolloop.ToolResolver`。

`agent/toolloop.Runner` 接收 `chat.Model` 和 `toolloop.Invocation`，以
`iter.Seq2[Event, error]` 依次发出：

```text
model_request -> model_response -> tool_call -> tool_result -> ... -> final
```

Runner 的策略是明确而保守的：工具串行执行、普通工具错误写成 error ToolResult、
不自动 retry、`MaxRounds` 限制模型轮次。`toolloop.Direct` 可在全 direct 的一轮后直接
结束。取消和首错通过序列错误返回。

工具需要人工输入时返回 `*toolloop.PauseError`。Pause 事件携带 JSON-safe
`Checkpoint`；`Runner.Resume` 从 pending tool 精确继续，不重调模型，也不重跑已完成
工具。完整示例见 [`../examples/toolloop`](../examples/toolloop)。

## 6. HITL、进程与持久化

Agent action 内的人机交互使用 `hitl.Interrupt[R]` 或 typed awaitable。进程进入
`StatusWaiting` 后，host 调用 `Platform.ResumeProcess` 提交响应，再调用
`ContinueProcess` 继续运行。

`core.ProcessStore` 保存 `ProcessSnapshot`；`PlatformConfig.AutoSnapshot` 可在每个
tick 与终态自动保存。`core.SessionStore` 保存跨 turn 的 Session。LLM 对话历史由独立
`chathistory` module 管理，并通过 context conversation ID 绑定，不塞入 Agent
Blackboard 或 provider response。

Tool-loop 的 `Checkpoint` 和 Agent 的 `ProcessSnapshot` 是两个不同层级：前者描述
一次模型/工具调用进行到哪里，后者描述整个 agent process。应用负责在自己的 turn
记录里组合持久化这两个值。

## 7. Extension

Platform-scope extension 放在 `runtime.PlatformConfig.Extensions`，process-scope
extension 放在 `core.ProcessOptions.Extensions`。所有 extension 先实现
`core.Extension.Name()`，runtime 再按具体能力接口发现行为。

常用能力包括：

- `planning.Planner`
- `core.ActionMiddleware`
- `core.ToolDecorator`
- `core.AgentValidator`
- `core.GoalApprover`
- `core.ToolGroupResolver`
- `core.ChatClientProvider`
- `core.EarlyTerminationPolicy`
- `core.IDGenerator`
- `core.Blackboard`
- `runtime.EventListener`

详细的合并、排序和所有权规则见 [`EXTENSION_DESIGN.md`](./EXTENSION_DESIGN.md)。

## 8. 并发与所有权

- `Platform` 管 agent/process registry；同一进程只允许一个 run loop 驱动。
- `Blackboard` 实现必须并发安全，因为 workflow 和并发 action 可能共享读取状态。
- `toolloop.Runner` 可复用，但它调用的 Model 和 ToolResolver 也必须满足相应并发条件。
- `chat.Request`、`Response`、`Checkpoint` 在跨边界前会验证；调用方不要在并发请求间
  复用并修改同一个 DTO。
- context cancellation 必须向 planner、model、tool 和持久化后端传播。

## 9. 选择扩展点的原则

先组合现有普通类型：action、workflow、middleware、tool decorator。只有当多个真实
实现需要由 runtime 在固定边界分发时，才增加新的 capability interface。协议值不携带
logger、registry、SDK client 或闭包；provider 特有 wire 字段使用 namespaced
`Extensions`，执行策略留在消费方。
