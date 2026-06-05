// Thin wrapper over @opentelemetry/api metrics. Kernel + plugin code calls
// the `measure*` helpers; the instruments they record into are created by
// `bindMetricInstruments()`.
//
// IMPORTANT — why instruments are bound late, not at module load:
//   The metrics API has NO proxy meter (unlike trace's ProxyTracer / logs'
//   ProxyLogger). `metrics.getMeter()` before a MeterProvider is registered
//   returns a Noop meter, and an instrument created from it is a
//   NoopInstrument *forever* — a later setGlobalMeterProvider does NOT
//   upgrade it. This module loads very early (the reducer imports it), long
//   before observability is installed, so creating instruments here at module
//   load would make every measurement a permanent no-op.
//   Instead `lib/observability/setup` calls bindMetricInstruments() right
//   after it registers the MeterProvider — that's when the real instruments
//   come into being. Until then `measure*` are cheap no-ops.

import type { Counter, Histogram } from "@opentelemetry/api";
import { metrics } from "@opentelemetry/api";

interface Instruments {
  reducer: Histogram;
  markdown: Histogram;
  shiki: Histogram;
  mermaid: Histogram;
  pluginLoad: Histogram;
  events: Counter;
}

let inst: Instruments | null = null;

/** Create the instruments against the (now-registered) global MeterProvider.
 *  Called once by lib/observability/setup after setGlobalMeterProvider. */
export function bindMetricInstruments(): void {
  const meter = metrics.getMeter("lyra");
  inst = {
    reducer: meter.createHistogram("lyra.reducer.duration", {
      description: "Time spent reducing one StreamEvent",
      unit: "ms",
    }),
    markdown: meter.createHistogram("lyra.markdown.repair.duration", {
      description: "Time spent in the markdown mid-stream repair step (remend) for one body",
      unit: "ms",
    }),
    shiki: meter.createHistogram("lyra.shiki.highlight.duration", {
      description: "Time spent highlighting one code block",
      unit: "ms",
    }),
    mermaid: meter.createHistogram("lyra.mermaid.render.duration", {
      description: "Time spent rendering one mermaid diagram",
      unit: "ms",
    }),
    pluginLoad: meter.createHistogram("lyra.plugin.load.duration", {
      description: "Time spent loading + running setup() for one plugin",
      unit: "ms",
    }),
    events: meter.createCounter("lyra.run.event.count", {
      description: "Number of run StreamEvents processed",
    }),
  };
}

/**
 * Wrap one synchronous reducer call. Records duration + bumps the StreamEvent
 * counter, both tagged with `eventType`. Re-throws on error so the reducer's
 * existing error path keeps working. No-op until instruments are bound.
 */
export function measureReduce<T>(eventType: string, fn: () => T): T {
  const start = performance.now();
  try {
    return fn();
  } finally {
    if (inst) {
      inst.reducer.record(performance.now() - start, { eventType });
      inst.events.add(1, { eventType });
    }
  }
}

export function measureMarkdownRepair(ms: number, textLength: number, streaming: boolean): void {
  inst?.markdown.record(ms, { streaming, lengthBucket: bucketLength(textLength) });
}

export function measureShikiHighlight(ms: number, lang: string): void {
  inst?.shiki.record(ms, { lang });
}

export function measureMermaidRender(ms: number): void {
  inst?.mermaid.record(ms);
}

/** Outcome of one definePlugin loadPlugin() invocation. */
export type PluginLoadResult = "loaded" | "failed" | "skipped";

export function measurePluginLoad(ms: number, pluginName: string, result: PluginLoadResult): void {
  inst?.pluginLoad.record(ms, { plugin: pluginName, result });
}

// Coarse buckets — keeps cardinality bounded without losing the
// "tiny vs huge body" signal that drives perf decisions.
function bucketLength(n: number): string {
  if (n < 100) return "<100";
  if (n < 500) return "<500";
  if (n < 2000) return "<2000";
  if (n < 10000) return "<10000";
  return ">=10000";
}
