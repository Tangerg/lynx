// Thin wrapper over @opentelemetry/api. Kernel + plugin code calls
// the `measure*` helpers below; instruments are pre-created against
// the global meter at module load.
//
// Without a registered MeterProvider, otel returns no-op instances
// and every `record` / `add` call is essentially a function-call
// no-op. The diagnostics plugin (when loaded) installs a real
// MeterProvider and starts collecting; when it unloads, everything
// goes quiet again.
//
// This is the only file the rest of the codebase needs to import.
// Hide otel verbosity here — call sites should read as one line.

import { metrics } from "@opentelemetry/api";

const meter = metrics.getMeter("lyra");

const reducerHistogram = meter.createHistogram("lyra.reducer.duration", {
  description: "Time spent reducing one AG-UI event",
  unit: "ms",
});

const markdownHistogram = meter.createHistogram("lyra.markdown.repair.duration", {
  description: "Time spent in the markdown mid-stream repair step (remend) for one body",
  unit: "ms",
});

const shikiHistogram = meter.createHistogram("lyra.shiki.highlight.duration", {
  description: "Time spent highlighting one code block",
  unit: "ms",
});

const mermaidHistogram = meter.createHistogram("lyra.mermaid.render.duration", {
  description: "Time spent rendering one mermaid diagram",
  unit: "ms",
});

const pluginLoadHistogram = meter.createHistogram("lyra.plugin.load.duration", {
  description: "Time spent loading + running setup() for one plugin",
  unit: "ms",
});

const eventCounter = meter.createCounter("lyra.run.event.count", {
  description: "Number of run StreamEvents processed",
});

/**
 * Wrap one synchronous reducer call. Records duration + bumps the
 * StreamEvent counter, both tagged with `eventType`. Re-throws on
 * error so the reducer's existing error path keeps working.
 */
export function measureReduce<T>(eventType: string, fn: () => T): T {
  const start = performance.now();
  try {
    return fn();
  } finally {
    reducerHistogram.record(performance.now() - start, { eventType });
    eventCounter.add(1, { eventType });
  }
}

export function measureMarkdownRepair(ms: number, textLength: number, streaming: boolean): void {
  markdownHistogram.record(ms, { streaming, lengthBucket: bucketLength(textLength) });
}

export function measureShikiHighlight(ms: number, lang: string): void {
  shikiHistogram.record(ms, { lang });
}

export function measureMermaidRender(ms: number): void {
  mermaidHistogram.record(ms);
}

/** Outcome of one definePlugin loadPlugin() invocation. */
export type PluginLoadResult = "loaded" | "failed" | "skipped";

export function measurePluginLoad(ms: number, pluginName: string, result: PluginLoadResult): void {
  pluginLoadHistogram.record(ms, { plugin: pluginName, result });
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
