// Thin tracing helpers for app-layer code (state / plugins / components).
// One-line span creation in the spirit of lib/metrics' measure* helpers.
//
// SPAN GRANULARITY IS DELIBERATELY COARSE — one span per agent run and one
// per RPC call, never per StreamEvent / item.delta / token. A run streams
// ~30 events/sec; a span per event would be thousands of spans per run. The
// per-event signal lives in metrics (lib/metrics' reducer histogram), not
// traces.
//
// The rpc/ layer is independent and can't import this module; the HTTP
// transport instruments its own CLIENT span directly against @opentelemetry/api
// (an external lib). This module is for everything above rpc/.

import { context, type Span, SpanKind, SpanStatusCode, trace } from "@opentelemetry/api";

const TRACER_NAME = "lyra-frontend";

/** Open a span for one agent run. Coarse: covers the whole run (start →
 *  finish), the parent the RPC CLIENT spans nest under. */
export function startRunSpan(attrs: Record<string, string | number | boolean>): Span {
  return trace.getTracer(TRACER_NAME).startSpan("agent.run", {
    kind: SpanKind.INTERNAL,
    attributes: attrs,
  });
}

/** Run `fn` with `span` as the active span, so anything it dispatches
 *  synchronously (the RPC call → transport.send) parents under it and
 *  inherits its trace context for `traceparent` injection. */
export function withSpan<T>(span: Span, fn: () => T): T {
  return context.with(trace.setSpan(context.active(), span), fn);
}

/** Settle + end a span. Pass the error to mark it failed. */
export function endSpan(span: Span, err?: unknown): void {
  if (err !== undefined && err !== null) {
    span.setStatus({
      code: SpanStatusCode.ERROR,
      message: err instanceof Error ? err.message : String(err),
    });
  } else {
    span.setStatus({ code: SpanStatusCode.OK });
  }
  span.end();
}
