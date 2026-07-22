# Core API 最小上手

本文展示当前稳定方向的路径和调用面。完整的可运行版本见
[`agent/examples/toolloop`](../agent/examples/toolloop/)。

## 1. 先选最小依赖

| 需求 | 使用包 |
|---|---|
| 定义消息、请求、响应和最小 Model SPI | `core/chat` |
| 默认参数、middleware、同步/流式调用、模板和结构化输出 | `chatclient` |
| 把 typed function 变成可执行工具并按实例注册 | `tools` |
| 多轮工具执行、普通错误反馈、direct return、暂停/恢复 | `agent/toolloop` |
| 聊天历史 | `chathistory` |
| OpenTelemetry | `otel` |

Provider 只需实现 `chat.Model`；流式能力独立实现 `chat.Streamer`。当前
OpenAI、Anthropic、Google 和 Ollama 的适配器分别由各 provider 包的
`NewChat(ChatConfig)` 构造，返回值可直接注入下述 API。

## 2. 最小同步调用

```go
func Ask(ctx context.Context, model chat.Model, question string) (string, error) {
    client, err := chatclient.New(model,
        chatclient.WithDefaults(chat.Options{Model: "provider-model-name"}),
    )
    if err != nil {
        return "", err
    }
    request, err := chat.NewRequest(
        chat.NewUserMessage(chat.NewTextPart(question)),
    )
    if err != nil {
        return "", err
    }
    response, err := client.Call(ctx, request)
    if err != nil {
        return "", err
    }
    return response.Text(), nil
}
```

`chat.Request` 是普通协议值。request-specific 配置写入
`request.Options`；client option 只放 construction-time default、独立
Streamer 和 middleware。不存在第二套 fluent request builder。

## 3. 流式调用

当同步 model 本身也实现 `chat.Streamer` 时，`chatclient.New` 会自动发现它；
Call/Stream 是两个独立对象时使用 `chatclient.WithStreamer(streamer)`。

```go
for response, err := range client.Stream(ctx, request) {
    if err != nil {
        return err
    }
    fmt.Print(response.Text())
}
```

调用方提前停止 range 时，provider 必须同步释放资源。框架不会用一次同步 Call
伪造 streaming。

## 4. Typed Tool 与工具循环

```go
type addInput struct {
    A int `json:"a"`
    B int `json:"b"`
}

add, err := tools.New(tools.Config{
    Name:        "add",
    Description: "add two integers",
}, func(_ context.Context, in addInput) (int, error) {
    return in.A + in.B, nil
})
if err != nil {
    return err
}
registry, err := tools.NewRegistry(add)
if err != nil {
    return err
}

request, err := chat.NewRequest(
    chat.NewUserMessage(chat.NewTextPart("What is 2 + 3?")),
)
if err != nil {
    return err
}
request.Tools = registry.Definitions()

invocation, err := toolloop.NewInvocation(request, registry)
if err != nil {
    return err
}
runner, err := toolloop.NewRunner(model, toolloop.RunnerConfig{})
if err != nil {
    return err
}

for event, err := range runner.Run(ctx, invocation) {
    if err != nil {
        return err
    }
    switch {
    case event.Kind == toolloop.EventToolResult:
        fmt.Println("tool:", event.ToolResult.Result)
    case event.Kind == toolloop.EventModelResponse && event.Final:
        fmt.Println("assistant:", event.Response.Text())
    }
}
```

这里有两条边界不能省略：

- `request.Tools` 只含可序列化 `ToolDefinition`；可执行对象在相邻的
  `Invocation.Tools`，`Invocation` 本身拒绝 JSON 序列化。
- 工具普通 error 会变成 `IsError` ToolResult 反馈给模型；context error 和
  `toolloop.AbortError` 终止运行。Runner 不自动重试。

直接运行完整示例：

```bash
cd agent
go run ./examples/toolloop
```

## 5. Pause / Resume

需要批准的工具必须在产生副作用前返回稳定 checkpoint ID：

```go
resume, resumed := toolloop.ResumeFromContext(ctx)
if !resumed {
    return "", &toolloop.PauseError{
        ID:     "approve-delete-42",
        Reason: "delete record 42",
    }
}
if resume.Input != "approved" {
    return "", errors.New("approval rejected")
}
// 从这里开始才执行副作用。
```

`EventPause` 携带 JSON-safe `Checkpoint`。持久化该 checkpoint 后，使用相同
resolver 恢复：

```go
resume := toolloop.Resume{ID: checkpoint.ID, Input: "approved"}
for event, err := range runner.Resume(ctx, checkpoint, registry, resume) {
    // 与 Run 使用同一套 Event 消费逻辑。
}
```

Resume 不重新调用产生该 tool-call round 的模型，也不重新执行 checkpoint 中已
完成的工具。待恢复工具可以再次返回 PauseError，形成新的 checkpoint。

## 6. 模板与结构化输出

模板只负责渲染，不持有可变 per-call 状态：

```go
prompt, err := chatclient.ParseTemplate("Explain {{.Topic}} in one sentence.")
message, err := prompt.UserMessage(struct{ Topic string }{Topic: "Go interfaces"})
```

结构化输出使用普通泛型值和 decoder：

```go
type Answer struct {
    Value int `json:"value"`
}

answer, response, err := chatclient.CallStructured(
    ctx, client, request, chatclient.JSON[Answer](),
)
```

decode 失败时仍返回原始 Response，repair/retry 策略由调用方显式决定。

## 7. 下一步

- `go doc github.com/Tangerg/lynx/core/chat`
- `go doc github.com/Tangerg/lynx/chatclient`
- `go doc github.com/Tangerg/lynx/tools`
- `go doc github.com/Tangerg/lynx/agent/toolloop`
- 观测接入见 [`OBSERVABILITY.md`](./OBSERVABILITY.md)
- 模块维护规则与反向不变量见 [`../core/CLAUDE.md`](../core/CLAUDE.md)
