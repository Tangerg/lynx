# otel

`otel` 是 Scope 的官方 OpenTelemetry integration module。它从外层为
Core 协议能力增加 traces/metrics，并提供把 OTel 三类信号写到
`log/slog` 的开发态 exporter；Core 本身不 import OTel。

## Chat instrumentation

`otel/chat` 提供不可变的 `Middleware`。provider identity 在组合根显式传入，
不要求 `core/chat.Model` 增加 `Metadata`、默认配置或观测方法：

```go
import (
	"github.com/Tangerg/scope/core/chat"
	otelchat "github.com/Tangerg/scope/otel/chat"
)

instrumentation, err := otelchat.NewMiddleware(otelchat.MiddlewareConfig{
	Provider: "openai",
})
if err != nil {
	return err
}

observedModel := chat.Wrap(providerModel, instrumentation.Call)
observedStream := chat.WrapStream(providerStreamer, instrumentation.Stream)
```

`Call` 和 `Stream` 保持为两个独立能力：call-only provider 不会因为观测
而获得一个伪造的流式接口。wrapper 的行为包括：

- 使用当前 OTel GenAI semconv 的 `gen_ai.provider.name`、operation、
  request/response model、finish reason 和 usage 属性。
- 发射 `gen_ai.client.operation.duration` 与
  `gen_ai.client.token.usage` histogram。
- stream 在真正迭代时才开始观测；第一个生成内容触发
  `first_token_received` event。
- provider error、部分 response 和 chunk 原样透传；观测聚合失败只记录
  event，不转换成业务错误。
- 调用方提前停止迭代时同步结束 span，并依靠底层 Streamer 同步释放资源。

`chat.MiddlewareConfig.TracerProvider` 和 `MeterProvider` 可用于显式注入；为 nil 时在
构造阶段取得官方全局 provider。没有安装 SDK provider 时，官方 provider
为 noop，但 wrapper 自身仍会执行计时、属性读取和流式聚合。

## Agent instrumentation

`otel/agent.Observer` 是 `agent.EventListener` 的官方反向集成：Agent Kernel
只发布自校验的 typed fact，不 import OTel；Observer 把一次 start 或 restore
到 terminal 的运行区间解释为一次 Process activation，并为其中的 Step 与 Effect
建立子 span。

```go
observer, err := agentotel.NewObserver(agentotel.ObserverConfig{
	TracerProvider: tracerProvider,
	MeterProvider:  meterProvider,
})
if err != nil {
	return err
}
defer observer.Close()

engine, err := agent.NewEngine(agent.EngineConfig{
	EventListeners: []agent.EventListener{observer},
})
```

Observer 发射以下稳定 instrument；所有 duration 都使用秒：

| Instrument | 类型 | 含义 |
|---|---|---|
| `agent.process.activations` | counter | started/restored runtime activation |
| `agent.process.exits` | counter | terminal Process outcome |
| `agent.process.activation.duration` | histogram | 当前 activation 到 terminal 的耗时 |
| `agent.process.committed_steps` | histogram | terminal Framework Usage 中的 committed Steps |
| `agent.process.prepared_effects` | histogram | terminal Framework Usage 中的 prepared Effects |
| `agent.process.accepted_signals` | histogram | terminal Framework Usage 中的 accepted Signals |
| `agent.step.duration` | histogram | Execution Step attempt 耗时 |
| `agent.effect.duration` | histogram | Framework/Dispatcher Effect attempt 耗时 |
| `agent.delta.dropped` | counter | 被有界 observation 路径丢弃的 Delta increment |

Process、Step 与 Effect span 携带 exact Process/tree、Deployment 和 activation
归因；durable execution 额外携带 `agent.tree.incarnation_id`。metric 只使用
Deployment name/version、activation、status/cause、Effect target/status 和稳定
failure kind/code 等受控维度，不写入 raw payload、Input、Output 或产品身份。

Observer 不保存 Event callback context，只保存仍在活动的 span。`Close` 先线性化
拒绝新 observation，等待已经进入的 callback 完成，再以错误状态结束残留 span；
因此可以安全地与 Engine shutdown 协调，也可以并发重复调用。

## Development sinks

`otel/slog` 提供三个官方 SDK 接口实现：

| Exporter | 输入 | 输出 |
|---|---|---|
| `NewSpanExporter` | completed span | one `slog.Record` per span |
| `NewMetricExporter` | metric batch | structured metric records |
| `NewLogExporter` | OTel log record | one `slog.Record` per log |

开发态 trace 示例：

```go
import (
	"context"
	stdslog "log/slog"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	otelslog "github.com/Tangerg/scope/otel/slog"
)

provider := sdktrace.NewTracerProvider(
	sdktrace.WithSyncer(otelslog.NewSpanExporter(stdslog.Default())),
)
otel.SetTracerProvider(provider)
defer provider.Shutdown(context.Background())
```

生产环境直接把 slog exporter 换成官方 OTLP exporter；业务 wrapper 和
Core 协议代码不变。

## Dependency direction

```text
agent             <-- otel/agent        --> OpenTelemetry API
core/chat         <-- otel/chat         --> OpenTelemetry API
history           <-- otel/history      --> OpenTelemetry API
core/vectorstore  <-- otel/vectorstore  --> OpenTelemetry API
                       otel/slog         --> OpenTelemetry SDK --> log/slog
```

- Core 用户不引入 `otel` 时，不承担任何 OTel 依赖。
- `otel` 目录只是命名空间，不存在根包；各领域 wrapper 只调用官方 API。同一 module 内的 `otel/slog` 和
  测试使用官方 SDK，因此该 module 的 `go.mod` 有 SDK requirement。
- 本模块不定义 tracer、meter、registry 或 observation 自有抽象。
- OTLP、Jaeger、Zipkin 等生产 exporter 使用官方实现，不在 Scope 中复制。
