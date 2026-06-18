// Frontend mirror of the backend's setupObservability
// (lynx lyra/cmd/lyra/observability.go): the ONE place all three OTel signals
// are wired onto the global providers. Everything else just calls the static
// otel accessors (trace.getTracer / metrics.getMeter / logs.getLogger) — no
// injection.
//
//   - Traces:  WebTracerProvider, AlwaysSample, local span sink (+ OTLP).
//   - Metrics: MeterProvider, PeriodicReader → local metric sink (+ OTLP).
//   - Logs:    LoggerProvider → local log sink (+ OTLP); host.log records flow
//              through it AS OTel log records (trace_id/span_id filled from the
//              active span — see ../logBridge).
//   - Context: W3C trace-context + baggage propagator, so the `traceparent`
//              this frontend injects on every RPC extends into the backend's
//              existing OTel trace (full-link tracing).
//
// Swappable exporter, exactly like the backend: the local in-memory sink is
// always on (dev visibility, ../stores); OTLP is added only when an endpoint
// is configured — the production swap to the real collector, zero call-site
// change. The whole module (incl. the ~SDK) is dynamic-imported by the
// bootstrap plugin, so none of it touches the first-paint path.

import { metrics } from "@opentelemetry/api";
import { logs } from "@opentelemetry/api-logs";
import {
  CompositePropagator,
  W3CBaggagePropagator,
  W3CTraceContextPropagator,
} from "@opentelemetry/core";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { bindMetricInstruments } from "@/lib/metrics";
import type { IMetricReader } from "@opentelemetry/sdk-metrics";
import { MeterProvider, PeriodicExportingMetricReader } from "@opentelemetry/sdk-metrics";
import type { LogRecordProcessor } from "@opentelemetry/sdk-logs";
import { BatchLogRecordProcessor, LoggerProvider } from "@opentelemetry/sdk-logs";
import type { SpanProcessor } from "@opentelemetry/sdk-trace-web";
import { BatchSpanProcessor, WebTracerProvider } from "@opentelemetry/sdk-trace-web";
import { LocalLogProcessor, LocalMetricExporter, LocalSpanProcessor } from "./sink";

export interface ObservabilityOptions {
  serviceName: string;
  serviceVersion: string;
  /** OTLP/HTTP base URL (e.g. the backend's collector ingress). When set,
   *  traces/metrics/logs are ALSO exported there — the production swap. */
  otlpEndpoint?: string;
}

const LOCAL_METRIC_INTERVAL_MS = 500;

let shutdownFn: (() => Promise<void>) | null = null;

export async function setupObservability(opts: ObservabilityOptions): Promise<void> {
  if (shutdownFn) return; // idempotent — one install per session

  const resource = resourceFromAttributes({
    "service.name": opts.serviceName,
    "service.version": opts.serviceVersion,
  });

  // OTLP exporters live behind a dynamic import so they never land in the
  // default chunk; only pulled when an endpoint is configured.
  const otlp = opts.otlpEndpoint ? await loadOtlp(opts.otlpEndpoint) : null;

  // ── Traces ──────────────────────────────────────────────────────────────
  const spanProcessors: SpanProcessor[] = [new LocalSpanProcessor()];
  if (otlp) spanProcessors.push(otlp.spanProcessor);
  const tracerProvider = new WebTracerProvider({ resource, spanProcessors });
  // register() installs the global TracerProvider + context manager + the
  // propagator that propagation.inject() will use on RPC headers.
  tracerProvider.register({
    propagator: new CompositePropagator({
      propagators: [new W3CTraceContextPropagator(), new W3CBaggagePropagator()],
    }),
  });

  // ── Metrics ─────────────────────────────────────────────────────────────
  const readers: IMetricReader[] = [
    new PeriodicExportingMetricReader({
      exporter: new LocalMetricExporter(),
      exportIntervalMillis: LOCAL_METRIC_INTERVAL_MS,
    }),
  ];
  if (otlp) readers.push(otlp.metricReader);
  const meterProvider = new MeterProvider({ resource, readers });
  metrics.setGlobalMeterProvider(meterProvider);
  // The metrics API has no proxy meter, so lib/metrics' instruments must be
  // created NOW (post-registration), not at its module load — otherwise every
  // measurement is a permanent no-op. See lib/metrics' header.
  bindMetricInstruments();

  // ── Logs ────────────────────────────────────────────────────────────────
  const logProcessors: LogRecordProcessor[] = [new LocalLogProcessor()];
  if (otlp) logProcessors.push(otlp.logProcessor);
  const loggerProvider = new LoggerProvider({ resource, processors: logProcessors });
  logs.setGlobalLoggerProvider(loggerProvider);

  shutdownFn = async () => {
    await Promise.allSettled([
      tracerProvider.shutdown(),
      meterProvider.shutdown(),
      loggerProvider.shutdown(),
    ]);
    shutdownFn = null;
  };
}

export async function teardownObservability(): Promise<void> {
  await shutdownFn?.();
}

interface OtlpBundle {
  spanProcessor: SpanProcessor;
  metricReader: IMetricReader;
  logProcessor: LogRecordProcessor;
}

// Construct the OTLP/HTTP exporters + their batching wrappers. Batch
// processors (not simple/sync) so high telemetry volume never turns into
// one network call per span/log.
async function loadOtlp(endpoint: string): Promise<OtlpBundle> {
  const base = endpoint.replace(/\/$/, "");
  // Only the OTLP exporter packages are dynamic — they're the part that never
  // loads unless an endpoint is configured. The Batch*Processor wrappers come
  // from sdk-trace-web / sdk-logs, already in this chunk via the static imports
  // above, so re-importing them dynamically would be a no-op chunk split.
  const [traceExp, metricExp, logExp] = await Promise.all([
    import("@opentelemetry/exporter-trace-otlp-http"),
    import("@opentelemetry/exporter-metrics-otlp-http"),
    import("@opentelemetry/exporter-logs-otlp-http"),
  ]);
  return {
    spanProcessor: new BatchSpanProcessor(
      new traceExp.OTLPTraceExporter({ url: `${base}/v1/traces` }),
    ),
    metricReader: new PeriodicExportingMetricReader({
      exporter: new metricExp.OTLPMetricExporter({ url: `${base}/v1/metrics` }),
      exportIntervalMillis: 10_000,
    }),
    logProcessor: new BatchLogRecordProcessor(
      new logExp.OTLPLogExporter({ url: `${base}/v1/logs` }),
    ),
  };
}
