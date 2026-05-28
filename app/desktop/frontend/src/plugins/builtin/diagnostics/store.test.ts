import type {
  AggregationTemporality,
  MetricDescriptor,
  ResourceMetrics,
} from "@opentelemetry/sdk-metrics";
import { beforeEach, describe, expect, it } from "vitest";
import { useDiagnosticsStore } from "./store";

// Numeric enum values from @opentelemetry/sdk-metrics, inlined so
// the test file mirrors the production module's lazy-import stance.
const CUMULATIVE = 1 as AggregationTemporality;
const HISTOGRAM = 0;
const SUM = 3;

const RES = {} as ResourceMetrics["resource"];
const SCOPE = { name: "lyra", version: "1.0.0" };

// Build a minimal ResourceMetrics that exercises one instrument. The
// otel typings expect a few fields we don't care about for the store
// (e.g. resource, startTime); cast through unknown to dodge them
// while still keeping the meaningful shape exact.
function batch(metrics: MetricBuilder[]): ResourceMetrics {
  return {
    resource: RES,
    scopeMetrics: [{ scope: SCOPE, metrics: metrics.map((b) => b()) }],
  } as unknown as ResourceMetrics;
}

type MetricBuilder = () => ReturnType<typeof histogramMetric> | ReturnType<typeof counterMetric>;

function histogramMetric({
  name,
  unit = "ms",
  description = "",
  attrs = {},
  count,
  sum,
  boundaries = [10, 50, 100],
  counts,
}: {
  name: string;
  unit?: string;
  description?: string;
  attrs?: Record<string, string | number>;
  count: number;
  sum: number;
  boundaries?: number[];
  counts: number[];
}) {
  const descriptor: MetricDescriptor = { name, unit, description, valueType: 1 };
  return {
    descriptor,
    aggregationTemporality: CUMULATIVE,
    dataPointType: HISTOGRAM,
    dataPoints: [
      {
        attributes: attrs,
        startTime: [0, 0],
        endTime: [0, 0],
        value: { count, sum, buckets: { boundaries, counts } },
      },
    ],
  };
}

function counterMetric({
  name,
  unit = "",
  description = "",
  attrs = {},
  total,
}: {
  name: string;
  unit?: string;
  description?: string;
  attrs?: Record<string, string | number>;
  total: number;
}) {
  const descriptor: MetricDescriptor = { name, unit, description, valueType: 1 };
  return {
    descriptor,
    aggregationTemporality: CUMULATIVE,
    dataPointType: SUM,
    isMonotonic: true,
    dataPoints: [
      {
        attributes: attrs,
        startTime: [0, 0],
        endTime: [0, 0],
        value: total,
      },
    ],
  };
}

describe("diagnostics store: ingest", () => {
  beforeEach(() => useDiagnosticsStore.getState().clear());

  it("ingests a histogram into one row keyed by name+attrs", () => {
    useDiagnosticsStore.getState().ingest(
      batch([
        () =>
          histogramMetric({
            name: "lyra.reducer.duration",
            attrs: { eventType: "TEXT_MESSAGE_CONTENT" },
            count: 10,
            sum: 220,
            counts: [4, 4, 2, 0],
          }),
      ]),
    );
    const rows = Object.values(useDiagnosticsStore.getState().rows);
    expect(rows).toHaveLength(1);
    const r = rows[0]!;
    expect(r.name).toBe("lyra.reducer.duration");
    expect(r.kind).toBe("histogram");
    expect(r.attrs).toEqual({ eventType: "TEXT_MESSAGE_CONTENT" });
    expect(r.count).toBe(10);
    expect(r.sum).toBe(220);
    expect(r.avg).toBe(22);
  });

  it("estimates p50 / p95 from the bucket boundary the percentile lands in", () => {
    // 20 observations across boundaries [10, 50, 100]:
    //   bucket 0 (≤10):  8 obs   cumulative  8 / 20 = 40%
    //   bucket 1 (≤50): 10 obs   cumulative 18 / 20 = 90%
    //   bucket 2 (≤100): 2 obs   cumulative 20 / 20 = 100%
    // p50 (target 10) lands in bucket 1 → upper bound 50
    // p95 (target 19) lands in bucket 2 → upper bound 100
    useDiagnosticsStore.getState().ingest(
      batch([
        () =>
          histogramMetric({
            name: "h",
            count: 20,
            sum: 600,
            boundaries: [10, 50, 100],
            counts: [8, 10, 2, 0],
          }),
      ]),
    );
    const r = Object.values(useDiagnosticsStore.getState().rows)[0]!;
    expect(r.p50).toBe(50);
    expect(r.p95).toBe(100);
  });

  it("returns p50=0 / p95=0 when the histogram has no observations", () => {
    useDiagnosticsStore.getState().ingest(
      batch([
        () =>
          histogramMetric({
            name: "h-empty",
            count: 0,
            sum: 0,
            counts: [0, 0, 0, 0],
          }),
      ]),
    );
    const r = Object.values(useDiagnosticsStore.getState().rows)[0]!;
    expect(r.count).toBe(0);
    expect(r.avg).toBe(0);
    expect(r.p50).toBe(0);
    expect(r.p95).toBe(0);
  });

  it("ingests a counter into a row with count == sum == total and no histogram fields", () => {
    useDiagnosticsStore.getState().ingest(
      batch([
        () =>
          counterMetric({
            name: "lyra.agui.event.count",
            attrs: { eventType: "TEXT_MESSAGE_CONTENT" },
            total: 42,
          }),
      ]),
    );
    const r = Object.values(useDiagnosticsStore.getState().rows)[0]!;
    expect(r.kind).toBe("counter");
    expect(r.count).toBe(42);
    expect(r.sum).toBe(42);
    expect(r.avg).toBeUndefined();
    expect(r.p50).toBeUndefined();
    expect(r.p95).toBeUndefined();
  });

  it("collapses attribute orderings into the same row id", () => {
    useDiagnosticsStore.getState().ingest(
      batch([
        () =>
          counterMetric({
            name: "c",
            attrs: { a: "1", b: "2" },
            total: 3,
          }),
        () =>
          counterMetric({
            name: "c",
            attrs: { b: "2", a: "1" },
            total: 7,
          }),
      ]),
    );
    const rows = Object.values(useDiagnosticsStore.getState().rows);
    // Two data points sharing the same name + attribute key collapse;
    // CUMULATIVE temporality means the last one wins.
    expect(rows).toHaveLength(1);
    expect(rows[0]!.count).toBe(7);
  });

  it("keeps separate rows when attributes differ", () => {
    useDiagnosticsStore
      .getState()
      .ingest(
        batch([
          () => counterMetric({ name: "c", attrs: { lang: "ts" }, total: 1 }),
          () => counterMetric({ name: "c", attrs: { lang: "go" }, total: 2 }),
        ]),
      );
    expect(Object.values(useDiagnosticsStore.getState().rows)).toHaveLength(2);
  });

  it("replaces the whole snapshot on each ingest (no merge)", () => {
    useDiagnosticsStore.getState().ingest(batch([() => counterMetric({ name: "a", total: 1 })]));
    useDiagnosticsStore.getState().ingest(batch([() => counterMetric({ name: "b", total: 1 })]));
    const rows = Object.values(useDiagnosticsStore.getState().rows);
    expect(rows.map((r) => r.name)).toEqual(["b"]);
  });
});

describe("diagnostics store: clear", () => {
  it("empties the snapshot", () => {
    useDiagnosticsStore.getState().ingest(batch([() => counterMetric({ name: "c", total: 1 })]));
    expect(Object.values(useDiagnosticsStore.getState().rows)).toHaveLength(1);
    useDiagnosticsStore.getState().clear();
    expect(Object.values(useDiagnosticsStore.getState().rows)).toHaveLength(0);
  });
});
