# otel architecture

> Scope's official OpenTelemetry integration. It wraps Core and runtime protocol
> boundaries using the official OTel API directly, and gives traces, metrics, and
> logs one development sink in `log/slog`. It invents no observation abstraction
> of its own.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md). Symbols and
dependency versions follow the code; the usage entry point is
[`README.md`](README.md).

---

## 1. Position

- **An observation add-on, split by domain.** `otel` is a namespace with no root
  package. `otel/agent`, `otel/chat`, `otel/tool`, `otel/history`, `otel/rag`,
  `otel/vectorstore`, and the modality adapters each wrap their own domain;
  `otel/slog` owns only the development exporters. A domain module never imports
  this module or the official OTel API. A wrapper is an ordinary decorator and
  introduces no custom `Observation` or `Tracer` interface.
- **`a2a` and `mcp` are protocol integrations.** They use the official OTel API
  directly at their own protocol call boundary, and this module does not
  duplicate a synonymous wrapper for them.
- **All three signals land in slog.** Spans, metrics, and OTel log records each
  have one exporter that writes a slog record.
- **Why OTel rather than writing slog directly: replaceability.** In development
  slog is convenient to read; in production every exporter is swapped for OTLP
  and traces, metrics, and logs all go to a backend with zero change to domain
  code. That is what vendor-neutral buys.
- **Logs are a first-class OTel signal.** An application feeds slog into the
  LoggerProvider through contrib's `otelslog` bridge, and this module's log
  exporter lands it. This is not "bypass OTel and log directly".

## 2. Mental model

- **The official API is used directly.** A wrapper calls `otel.Tracer` and
  `otel.Meter` itself. Observation attributes such as provider identity are
  passed explicitly at construction and never pollute the wrapped domain's
  protocol.
- **All three exporters implement the OTel SDK's standard interfaces.** One per
  signal, mutually uncoupled, with no invented interface.
- **The log handler is not here.** Use contrib's `otelslog` (slog handler →
  LoggerProvider); this module provides only the log exporter downstream of it.
- **The composition root binds once.** At startup it sets the three global
  providers, replaces the default slog handler with the bridge, and installs the
  W3C propagator. After that `otel.Tracer` and `otel.Meter` are used directly,
  with no dependency injection.
- **Development-first trade-offs.** Export always returns nil, so a sink failure
  never pollutes the business path; flushing is synchronous rather than batched;
  an error span is promoted to error level.
- **Attributes pass through unchanged and keys carry no brand.** Use semconv
  where it exists, otherwise a bare domain name with no project prefix. The
  instrumentation scope name keeps the library path — that is a library
  identifier, not data.
- **The instrumentation scope follows its owner.** Each domain package uses its
  own full import path; different domains are never re-aggregated under a shared
  root scope.
- **`error.type` stays low cardinality.** Each adapter classifies failures into a
  small fixed vocabulary — canceled, deadline exceeded, invalid request, invalid
  response — because the attribute is a metric dimension and a provider message
  would create one time series per distinct string.
- **Content never enters telemetry.** Prompts, queries, documents, media, audio,
  transcripts, and evaluation subjects stay out. An adapter records identity,
  counts, latency, outcome, and a stable error classification.

## 3. Negative invariants

- Never make Core add a field or an interface for observation. The context and
  the protocol return value are already the boundary a wrapper needs.
- Never invent a tracer, meter, or registry abstraction. The OTel API *is* the
  vendor-neutral layer.
- Never implement an OTLP, Jaeger, or Zipkin exporter here. Those are production
  exporters from OTel contrib. This module exists so a span is one readable line
  on a developer's machine.
- Never add a Go package at the `otel` root or a cross-domain shared middleware
  helper. A wrapper is owned entirely by the domain subpackage it wraps.

## 4. Read before changing

Connecting a production backend does not touch this module — change the
exporters the composition root binds.
