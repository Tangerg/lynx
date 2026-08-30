# otel

`otel` is Scope's official OpenTelemetry integration. It adds traces and metrics
to Core's capabilities from the outside, and provides development exporters that
write all three OTel signals to `log/slog`. Core itself never imports OTel.

Adapters are split by the contract they wrap, so one generic config or magic map
never mixes different lifecycles:

| Package | Observed boundary | Composition entry |
|---|---|---|
| `otel/chat` | chat call / lazy stream | `Middleware.Call` / `Middleware.Stream` |
| `otel/embedding` | embedding call, input and token counts | `Middleware.Wrap` |
| `otel/image` | image generation call | `Middleware.Wrap` |
| `otel/moderation` | moderation call and input count | `Middleware.Wrap` |
| `otel/speech` | speech call / lazy stream | `Middleware.Wrap` / `Middleware.WrapStream` |
| `otel/transcription` | transcription call | `Middleware.Wrap` |
| `otel/eval` | generic evaluator result | `Middleware[T].Wrap` |
| `otel/rag` | retrieval | `Middleware.Wrap` |
| `otel/tool` | tool invocation | `Middleware.Wrap` |
| `otel/history` | history store and listing | `Middleware.Store` / `Middleware.Conversations` |
| `otel/vectorstore` | vector-store operations | capability-specific methods |
| `otel/agent` | managed Process activation, Step, and Effect | `Observer` |

Model content, queries, documents, audio, images, transcripts, and evaluation
subjects never enter telemetry. An adapter records identity, counts, latency,
outcome, and a stable error classification only.

## Chat instrumentation

`otel/chat` provides an immutable `Middleware`. Provider identity is passed
explicitly at the composition root, so `core/chat.Model` is never asked to grow
a `Metadata`, a default config, or an observation method:

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

`Call` and `Stream` stay two independent capabilities: a call-only provider does
not gain a fake streaming interface just by being observed. The wrapper:

- uses the current OTel GenAI semconv attributes — `gen_ai.provider.name`,
  operation, request and response model, finish reason, and usage;
- emits the `gen_ai.client.operation.duration` and `gen_ai.client.token.usage`
  histograms;
- begins observing a stream only when it is actually iterated, and raises a
  `first_token_received` event on the first generated content;
- passes provider errors, partial responses, and chunks through unchanged — an
  observation aggregation failure is recorded as an event, never converted into
  a business error;
- ends the span synchronously when the caller stops iterating early, relying on
  the underlying `Streamer` to release resources synchronously.

`MiddlewareConfig.TracerProvider` and `MeterProvider` allow explicit injection;
when nil, the official global providers are resolved at construction. Without an
installed SDK provider those are noop, but the wrapper still performs its
timing, attribute reads, and stream aggregation.

## Agent instrumentation

`otel/agent.Observer` is the official reverse integration for
`agent.EventListener`: the Agent kernel publishes self-validating typed facts and
never imports OTel, and the Observer interprets one start-or-restore-to-terminal
interval as a Process activation with child spans for its Steps and Effects.

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

The Observer emits these stable instruments; every duration is in seconds:

| Instrument | Type | Meaning |
|---|---|---|
| `agent.process.activations` | counter | started or restored runtime activation |
| `agent.process.exits` | counter | terminal Process outcome |
| `agent.process.activation.duration` | histogram | this activation until terminal |
| `agent.process.committed_steps` | histogram | committed Steps in the terminal Framework Usage |
| `agent.process.prepared_effects` | histogram | prepared Effects in the terminal Framework Usage |
| `agent.process.accepted_signals` | histogram | accepted Signals in the terminal Framework Usage |
| `agent.step.duration` | histogram | Execution Step attempt |
| `agent.effect.duration` | histogram | Framework or Dispatcher Effect attempt |
| `agent.delta.dropped` | counter | Delta increments dropped by the bounded observation path |

Process, Step, and Effect spans carry exact Process/tree, Deployment, and
activation attribution; durable execution additionally carries
`agent.tree.incarnation_id`. Metrics use only controlled dimensions —
Deployment name, activation, status and cause, Effect target and status, and a
stable failure kind and code — never a raw payload, input, output, or product
identity. A Process error outcome, a failed Step attempt, and a non-successful
Effect settlement each set the span error status and record the standard OTel
exception event, projected from the typed fact: no fabricated stack, and no
reintroduced failure message or opaque payload.

The Observer retains no event callback context, only spans that are still
active. `Close` first linearizes rejection of new observation, waits for
callbacks already in flight, and then ends any residual span with an error
status — so it coordinates safely with Engine shutdown and tolerates concurrent
repeated calls.

## Development sinks

`otel/slog` provides three implementations of the official SDK interfaces:

| Exporter | Input | Output |
|---|---|---|
| `NewSpanExporter` | completed span | one `slog.Record` per span |
| `NewMetricExporter` | metric batch | structured metric records |
| `NewLogExporter` | OTel log record | one `slog.Record` per log |

A development trace setup:

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

In production, swap the slog exporters for the official OTLP exporters. The
wrappers and the Core protocol code do not change — that replaceability is the
whole reason this goes through OTel instead of writing slog directly.

## Dependency direction

```text
agent             <-- otel/agent        --> OpenTelemetry API
core/chat         <-- otel/chat         --> OpenTelemetry API
history           <-- otel/history      --> OpenTelemetry API
core/vectorstore  <-- otel/vectorstore  --> OpenTelemetry API
                       otel/slog         --> OpenTelemetry SDK --> log/slog
```

- A Core user who does not import `otel` takes on no OTel dependency.
- The root package contains only its `doc.go` overview and exports no API. Each
  domain wrapper calls the official API; `otel/slog` and the tests in the same
  module use the official SDK, which is why this module's `go.mod` requires it.
- This module defines no tracer, meter, registry, or observation abstraction of
  its own.
- Production exporters — OTLP, Jaeger, Zipkin — come from the official
  implementations and are never duplicated in Scope.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the invariants behind these rules.
