// Local Zustand store the diagnostics plugin uses to receive flattened
// metric snapshots from its custom MetricExporter. The View subscribes
// to this store and renders a table per measurement.
//
// Persistence intentionally skipped — diagnostics are dev-time and the
// data refreshes from otel every collect interval anyway.

import type { MetricDescriptor, ResourceMetrics } from "@opentelemetry/sdk-metrics";
import { DataPointType } from "@opentelemetry/sdk-metrics";
import { create } from "zustand";

type Attrs = Record<string, string | number | boolean>;
interface HistogramValue {
  count: number;
  sum?: number;
  buckets: { boundaries: number[]; counts: number[] };
}

export type InstrumentKind = "histogram" | "counter";

/** One row per (instrument name, attribute combo). */
export interface MetricRow {
  /** Stable id — `${name}|${sorted-attr-json}`. */
  id: string;
  name: string;
  unit: string;
  description: string;
  kind: InstrumentKind;
  attrs: Record<string, string | number | boolean>;
  /** Cumulative count of observations. */
  count: number;
  /** Cumulative sum (histogram + counter share the field — counters
   *  set both count and sum to the running total). */
  sum: number;
  /** Estimated percentile from histogram buckets. Undefined for counters. */
  p50?: number;
  p95?: number;
  /** Last seen value — most useful for counters. */
  last: number;
}

interface State {
  rows: Record<string, MetricRow>;
  /** Drain a ResourceMetrics export batch into the store. */
  ingest: (batch: ResourceMetrics) => void;
  /** Reset accumulators (button in the diagnostics view). */
  clear: () => void;
}

export const useDiagnosticsStore = create<State>((set) => ({
  rows: {},
  ingest: (batch) => {
    const next: Record<string, MetricRow> = {};
    for (const scope of batch.scopeMetrics) {
      for (const m of scope.metrics) {
        const isHist = m.dataPointType === DataPointType.HISTOGRAM;
        for (const dp of m.dataPoints) {
          const attrs = (dp.attributes ?? {}) as Attrs;
          const id = `${m.descriptor.name}|${stableKey(attrs)}`;
          next[id] = isHist
            ? histogramRow(id, m.descriptor, dp.value as HistogramValue, attrs)
            : counterRow(id, m.descriptor, dp.value as number, attrs);
        }
      }
    }
    // Replace wholesale — otel's CUMULATIVE temporality means every
    // export carries the running total, so merging would double-count.
    set({ rows: next });
  },
  clear: () => set({ rows: {} }),
}));

function histogramRow(id: string, desc: MetricDescriptor, v: HistogramValue, attrs: Attrs): MetricRow {
  const sum = v.sum ?? 0;
  const { p50, p95 } = estimatePercentiles(v.buckets);
  return {
    id,
    name: desc.name,
    unit: desc.unit,
    description: desc.description,
    kind: "histogram",
    attrs,
    count: v.count,
    sum,
    p50,
    p95,
    last: v.count > 0 ? sum / v.count : 0,
  };
}

function counterRow(id: string, desc: MetricDescriptor, total: number, attrs: Attrs): MetricRow {
  return {
    id,
    name: desc.name,
    unit: desc.unit,
    description: desc.description,
    kind: "counter",
    attrs,
    count: total,
    sum: total,
    last: total,
  };
}

// Deterministic attribute key — sort to keep "lang=ts,plugin=foo" and
// "plugin=foo,lang=ts" collapse to the same row id.
function stableKey(attrs: Attrs): string {
  const entries = Object.entries(attrs).sort(([a], [b]) => a.localeCompare(b));
  return entries.map(([k, v]) => `${k}=${String(v)}`).join(",");
}

// Estimate P50/P95 from explicit bucket boundaries + cumulative-by-
// bucket counts. Walk in order, find which bucket the percentile
// observation lands in, return the upper boundary as a conservative
// estimate (errs high — fine for "is this slow?" eyeballing).
function estimatePercentiles(buckets: HistogramValue["buckets"]): { p50: number; p95: number } {
  const total = buckets.counts.reduce((a, b) => a + b, 0);
  if (total === 0) return { p50: 0, p95: 0 };
  const target50 = total * 0.5;
  const target95 = total * 0.95;
  let running = 0;
  let p50 = 0;
  let p95 = 0;
  let p50Done = false;
  for (let i = 0; i < buckets.counts.length; i++) {
    running += buckets.counts[i];
    if (!p50Done && running >= target50) {
      p50 = buckets.boundaries[i] ?? buckets.boundaries[buckets.boundaries.length - 1] ?? 0;
      p50Done = true;
    }
    if (running >= target95) {
      p95 = buckets.boundaries[i] ?? buckets.boundaries[buckets.boundaries.length - 1] ?? 0;
      return { p50, p95 };
    }
  }
  // Fell off the end — last bucket is the open-ended "+Inf"
  const lastBoundary = buckets.boundaries[buckets.boundaries.length - 1] ?? 0;
  return { p50: p50 || lastBoundary, p95: lastBoundary };
}
