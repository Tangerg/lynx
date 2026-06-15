// Local sink — the dev-visibility exporters for all three signals, feeding
// the bounded in-memory stores the Diagnostics view renders. Mirror of the
// backend's otel/slog exporters (one local stream per signal).
//
// Every exporter BATCHES before touching the store: a burst of spans/logs
// buffers for one flush window and lands as a single Zustand commit, so
// render cost is bounded regardless of telemetry volume (the perf concern
// that shapes this whole module). Metrics already arrive batched from the
// SDK's PeriodicExportingMetricReader.

import type { HrTime } from "@opentelemetry/api";
import type {
  AggregationTemporality,
  PushMetricExporter,
  ResourceMetrics,
} from "@opentelemetry/sdk-metrics";
import type { LogRecordProcessor, SdkLogRecord } from "@opentelemetry/sdk-logs";
import type { ReadableSpan, SpanProcessor } from "@opentelemetry/sdk-trace-web";
import type { LogRow, SpanRow } from "./stores";
import { useTelemetryStore } from "./stores";

// Inlined enum values — keeps this module off the SDK's static graph.
const EXPORT_SUCCESS = 0; // ExportResultCode.SUCCESS
const CUMULATIVE_TEMPORALITY = 1 as AggregationTemporality;
const STATUS_ERROR = 2; // SpanStatusCode.ERROR
const STATUS_OK = 1; // SpanStatusCode.OK
const STATUS_TONE: Record<number, SpanRow["status"]> = {
  [STATUS_ERROR]: "error",
  [STATUS_OK]: "ok",
};

// One flush window for span/log batches — coalesces a burst into a single
// store commit. Matches the metric reader's cadence so the view ticks once.
const FLUSH_MS = 500;

const hrToMs = (t: HrTime): number => t[0] * 1000 + t[1] / 1e6;

function flattenAttrs(
  attrs: Record<string, unknown> | undefined,
): Record<string, string | number | boolean> {
  const out: Record<string, string | number | boolean> = {};
  for (const [k, v] of Object.entries(attrs ?? {})) {
    if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") out[k] = v;
    else if (v != null) out[k] = String(v);
  }
  return out;
}

// Generic batcher: collect items, flush them through `drain` once per window.
class Batcher<T> {
  private buf: T[] = [];
  private timer: ReturnType<typeof setTimeout> | null = null;
  constructor(private readonly drain: (batch: T[]) => void) {}
  add(item: T): void {
    this.buf.push(item);
    this.timer ??= setTimeout(() => this.flush(), FLUSH_MS);
  }
  flush(): void {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    if (this.buf.length === 0) return;
    const batch = this.buf;
    this.buf = [];
    this.drain(batch);
  }
}

// ── Metrics ───────────────────────────────────────────────────────────────
export class LocalMetricExporter implements PushMetricExporter {
  export(batch: ResourceMetrics, callback: (result: { code: number }) => void): void {
    useTelemetryStore.getState().ingestMetrics(batch);
    callback({ code: EXPORT_SUCCESS });
  }
  forceFlush(): Promise<void> {
    return Promise.resolve();
  }
  shutdown(): Promise<void> {
    return Promise.resolve();
  }
  selectAggregationTemporality(): AggregationTemporality {
    return CUMULATIVE_TEMPORALITY;
  }
}

// ── Traces ────────────────────────────────────────────────────────────────
export class LocalSpanProcessor implements SpanProcessor {
  private readonly batcher = new Batcher<SpanRow>((rows) =>
    useTelemetryStore.getState().ingestSpans(rows),
  );
  onStart(): void {
    /* coarse spans only — nothing to do on start */
  }
  onEnd(span: ReadableSpan): void {
    const ctx = span.spanContext();
    const code = span.status.code;
    this.batcher.add({
      id: ctx.spanId,
      traceId: ctx.traceId,
      parentSpanId: span.parentSpanContext?.spanId,
      name: span.name,
      kind: SPAN_KIND[span.kind] ?? String(span.kind),
      startMs: hrToMs(span.startTime),
      durationMs: hrToMs(span.duration),
      status: STATUS_TONE[code] ?? "unset",
      // The failure message endSpan set via setStatus — the one bit of "why"
      // the bare status enum can't carry. Empty string → omit.
      statusMessage: span.status.message || undefined,
      attrs: flattenAttrs(span.attributes),
    });
  }
  forceFlush(): Promise<void> {
    this.batcher.flush();
    return Promise.resolve();
  }
  shutdown(): Promise<void> {
    this.batcher.flush();
    return Promise.resolve();
  }
}

// SpanKind enum → label (inlined; SERVER/CLIENT/INTERNAL/PRODUCER/CONSUMER).
const SPAN_KIND: Record<number, string> = {
  0: "internal",
  1: "server",
  2: "client",
  3: "producer",
  4: "consumer",
};

// ── Logs ──────────────────────────────────────────────────────────────────
let logSeq = 0;

export class LocalLogProcessor implements LogRecordProcessor {
  private readonly batcher = new Batcher<LogRow>((rows) =>
    useTelemetryStore.getState().ingestLogs(rows),
  );
  onEmit(record: SdkLogRecord): void {
    const ctx = record.spanContext;
    const body = record.body;
    this.batcher.add({
      id: `log-${++logSeq}`,
      timeMs: record.hrTime ? hrToMs(record.hrTime) : Date.now(),
      severity: record.severityText ?? "INFO",
      body: typeof body === "string" ? body : JSON.stringify(body ?? ""),
      traceId: ctx?.traceId,
      spanId: ctx?.spanId,
      attrs: flattenAttrs(record.attributes as Record<string, unknown> | undefined),
    });
  }
  forceFlush(): Promise<void> {
    this.batcher.flush();
    return Promise.resolve();
  }
  shutdown(): Promise<void> {
    this.batcher.flush();
    return Promise.resolve();
  }
}
