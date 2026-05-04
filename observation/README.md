# observation

> OpenTelemetry bridge utilities for Lynx (external module).
> Connects OTel spans/metrics to non-standard backends (slog, stdlib log,
> custom handlers) without dragging OTel SDK dependencies into `core/`.

---

## Why a Separate Module?

Instrumentation code in `core/` only imports the OTel **API package**
(`go.opentelemetry.io/otel/trace`) — a small, stable set of interfaces plus
noop defaults. The real weight comes from the OTel **SDK package**, which is
required to implement an exporter.

Keeping the SDK dependency isolated here means:

- **Users who never enable tracing**: zero new entries in `go.sum`
- **Users who use OTel SDK + official exporters (OTLP / stdout / Jaeger)**:
  no need to depend on this module at all
- **Users who want spans forwarded to their business log stream**:
  `go get github.com/Tangerg/lynx/observation/slog` (or `/log`)

This follows the same architectural rule as `models/` and `vectorstores/`:
**any adapter that pulls in third-party SDKs lives in an external module**,
never in `core/`.

---

## Sub-packages

| Package | Purpose | Pick this when… |
|---------|---------|-----------------|
| [`slog/`](./slog/) | `SpanExporter` that writes to `log/slog` | You use Go 1.21+ structured logging (recommended) |
| [`log/`](./log/)   | `SpanExporter` that writes to stdlib `*log.Logger` | Your codebase still uses `log.Print`/`log.SetOutput`, or you want logfmt-style single-line output |

Both sub-packages provide the same span fields (trace/span IDs, parent, name,
duration, attributes, events) — they only differ in how each record is rendered.

> **Naming note**: sub-packages use the short names `slog` and `log` to match
> the `observation/<backend>` convention. They collide with the stdlib packages
> of the same name, so callers that import both must alias one side, e.g.
> `stdslog "log/slog"` or `stdlog "log"`. See the examples below.

Potential future additions:
- `otelmetric/` — bridge OTel metrics to a custom registry
- other on-demand extensions

---

## Quick Examples

### Structured output via `log/slog`

```go
import (
    "context"

    stdslog "log/slog"

    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"

    "github.com/Tangerg/lynx/observation/slog"
)

func main() {
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(slog.NewExporter(stdslog.Default())),
    )
    otel.SetTracerProvider(tp)
    defer tp.Shutdown(context.Background())

    // Every Lynx span now appears in the slog stream.
}
```

### Plain line output via stdlib `log`

```go
import (
    "context"

    stdlog "log"

    "go.opentelemetry.io/otel"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"

    "github.com/Tangerg/lynx/observation/log"
)

func main() {
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(log.NewExporter(stdlog.Default())),
    )
    otel.SetTracerProvider(tp)
    defer tp.Shutdown(context.Background())

    // Each span prints one line like:
    //   2026/04/20 10:30:00 span trace_id=... name="gen_ai.chat" duration=523ms gen_ai.system=openai
}
```

---

## Relationship to Other Lynx Modules

```
core/             imports go.opentelemetry.io/otel (API only)
models/           same
vectorstores/     same
observation/       ← the only module that depends on go.opentelemetry.io/otel/sdk
    ├─ slog/
    └─ log/
```

Users who do not import any sub-package under `observation/` never see the
OTel SDK dependency.
