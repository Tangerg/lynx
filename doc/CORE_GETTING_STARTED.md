# Core API 最小上手

本文展示当前稳定方向的路径和调用面。直接调用与托管 Process 的完整对照见
[`agent/examples/direct_vs_managed`](../agent/examples/direct_vs_managed/)，模型自主调用 Tool 的完整示例见
[`agent/examples/autonomous`](../agent/examples/autonomous/)。

## 1. 先选最小依赖

| 需求 | 使用包 |
|---|---|
| 定义消息、请求、响应和最小 Model SPI | `core/chat` |
| 默认参数、middleware、同步/流式调用、模板和结构化输出 | `chatclient` |
| 最小工具契约、decorator 能力发现和 typed/schema 辅助 | `tool` |
| 实例工具集合与 Registry | `tools` |
| 把 typed function 变成可执行工具 | `tool` |
| 可恢复的模型/工具自主循环、暂停/恢复和子 Process | `agent`、`agent/interaction` |
| 聊天历史 | `chathistory` |
| OpenTelemetry | `otel/chat`、`otel/chathistory`、`otel/vectorstore`、`otel/slog` |

Provider 只需实现 `chat.Model`；流式能力独立实现 `chat.Streamer`。当前
OpenAI、Anthropic、Google 和 Ollama 的适配器分别由各 provider 包的
`NewChat(ChatConfig)` 构造，返回值可直接注入下述 API。

## 2. 最小同步调用

```go
func Ask(ctx context.Context, model chat.Model, question string) (string, error) {
    client, err := chatclient.New(model, chatclient.Config{
        Defaults: chat.Options{Model: "provider-model-name"},
    })
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
Call/Stream 是两个独立对象时写入 `chatclient.Config.Streamer`。

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

## 4. Typed Tool 与托管 Interaction

```go
type addInput struct {
    A int `json:"a"`
    B int `json:"b"`
}

add, err := tool.NewFunc(tool.FuncConfig{
    Name:        "add",
    Description: "add two integers",
}, func(_ context.Context, in addInput) (int, error) {
    return in.A + in.B, nil
})
if err != nil {
    return err
}
```

`tool.NewFunc` 只负责建立准确的 Tool schema 与调用边界。只需直接调用时，调用方可自行把 Tool definitions 放入 `chat.Request` 并执行返回的 ToolCall；需要框架托管的模型→Tool→模型循环时，使用 `agent/interaction`：

- `interaction.Definition` 冻结静态契约和模型调用上限；
- `interaction.Dispatcher` 持有 `chatclient.Client` 与可执行 Tools；
- `agent.Deployment` 冻结 Definition、Dispatcher 与精确 digest；
- `agent.Engine` 是 Process 生命周期、Signal、Effect、snapshot 和 child tree 的唯一 owner。

直接运行完整示例：

```bash
cd agent
go run ./examples/autonomous
```

Tool 的普通 error 由 Interaction 作为 `IsError` ToolResult 反馈给模型；请求取消和无法证明的副作用结果不会被自动重试。需要外部输入的 Tool 使用 `interaction.RequireToolInput` 产生 Strategy-owned 请求，由 Engine 铸造 WaitID；Host 只保存完整 `agent.TreeSnapshot` 并通过 `interaction.PendingToolInputFromSnapshot` 构造类型化响应。具体恢复流程以 `agent/interaction` GoDoc 和合同测试为准，不存在第二套 Runner/Resume 生命周期。

## 5. 模板与结构化输出

模板只负责渲染，不持有可变 per-call 状态：

```go
prompt, err := chatclient.ParseTemplate("Explain {{.Topic}} in one sentence.")
message, err := prompt.UserMessage(struct{ Topic string }{Topic: "Go interfaces"})
```

`OutputFormat` 同时拥有请求格式和结果 decoder。`Client.Output` 将它绑定为一个不可变的
短链，因此同步、流式只有一条累积与 decode 路径：

```go
type Answer struct {
    Value int `json:"value"`
}

format := chatclient.JSON[Answer]()

// 同步：完整响应是一种只有一个元素的流。
answer, err := client.Output(format).Call(ctx, request)

// 流式：复用完全相同的 decoder。
answer, err = client.Output(format).Stream(ctx, request)
```

流式短链在响应序列结束后返回完整的 `Answer`；需要逐块处理响应时直接使用
`Client.Stream`。

provider 优先映射为原生格式控制；只有协议不支持所请求格式时才注入等价 prompt。
decoder 接受完整 JSON、JSON markdown fence 和单个被说明文字包围的完整 JSON，拒绝
截断补全、多个 JSON 值、重复 key 和非法 UTF-8。repair/retry 策略仍由调用方显式决定。

## 7. 下一步

- `go doc github.com/Tangerg/lynx/core/chat`
- `go doc github.com/Tangerg/lynx/chatclient`
- `go doc github.com/Tangerg/lynx/tool`
- `go doc github.com/Tangerg/lynx/tools`
- `go doc github.com/Tangerg/lynx/agent`
- `go doc github.com/Tangerg/lynx/agent/interaction`
- 观测接入见 [`OBSERVABILITY.md`](./OBSERVABILITY.md)
- 模块维护规则与反向不变量见 [`../core/CLAUDE.md`](../core/CLAUDE.md)
