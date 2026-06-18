import type {
  AggregationTemporality,
  MetricDescriptor,
  ResourceMetrics,
} from "@opentelemetry/sdk-metrics";
import { beforeEach, describe, expect, it } from "vitest";
import { useTelemetryStore } from "./stores";

// Numeric enum values from @opentelemetry/sdk-metrics, inlined so the test
// mirrors the production module's SDK-free stance.
const CUMULATIVE = 1 as AggregationTemporality;
const HISTOGRAM = 0;
const SUM = 3;

const RES = {} as ResourceMetrics["resource"];
const SCOPE = { name: "lyra", version: "1.0.0" };

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
    dataPoints: [{ attributes: attrs, startTime: [0, 0], endTime: [0, 0], value: total }],
  };
}

const metricRows = () => Object.values(useTelemetryStore.getState().metrics);

describe("telemetry store: ingestMetrics", () => {
  beforeEach(() => useTelemetryStore.getState().clear());

  it("ingests a histogram into one row keyed by name+attrs", () => {
    useTelemetryStore.getState().ingestMetrics(
      batch([
        () =>
          histogramMetric({
            name: "lyra.reducer.duration",
            attrs: { eventType: "item.delta" },
            count: 10,
            sum: 220,
            counts: [4, 4, 2, 0],
          }),
      ]),
    );
    const rows = metricRows();
    expect(rows).toHaveLength(1);
    const r = rows[0]!;
    expect(r.name).toBe("lyra.reducer.duration");
    expect(r.kind).toBe("histogram");
    expect(r.attrs).toEqual({ eventType: "item.delta" });
    expect(r.count).toBe(10);
    expect(r.sum).toBe(220);
    expect(r.avg).toBe(22);
  });

  it("estimates p50 / p95 from the bucket boundary the percentile lands in", () => {
    useTelemetryStore.getState().ingestMetrics(
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
    const r = metricRows()[0]!;
    expect(r.p50).toBe(50);
    expect(r.p95).toBe(100);
  });

  it("returns p50=0 / p95=0 when the histogram has no observations", () => {
    useTelemetryStore
      .getState()
      .ingestMetrics(
        batch([() => histogramMetric({ name: "h-empty", count: 0, sum: 0, counts: [0, 0, 0, 0] })]),
      );
    const r = metricRows()[0]!;
    expect(r.count).toBe(0);
    expect(r.avg).toBe(0);
    expect(r.p50).toBe(0);
    expect(r.p95).toBe(0);
  });

  it("ingests a counter into a row with count == sum == total and no histogram fields", () => {
    useTelemetryStore
      .getState()
      .ingestMetrics(batch([() => counterMetric({ name: "lyra.run.event.count", total: 42 })]));
    const r = metricRows()[0]!;
    expect(r.kind).toBe("counter");
    expect(r.count).toBe(42);
    expect(r.sum).toBe(42);
    expect(r.avg).toBeUndefined();
    expect(r.p50).toBeUndefined();
    expect(r.p95).toBeUndefined();
  });

  it("collapses attribute orderings into the same row id", () => {
    useTelemetryStore
      .getState()
      .ingestMetrics(
        batch([
          () => counterMetric({ name: "c", attrs: { a: "1", b: "2" }, total: 3 }),
          () => counterMetric({ name: "c", attrs: { b: "2", a: "1" }, total: 7 }),
        ]),
      );
    const rows = metricRows();
    expect(rows).toHaveLength(1);
    expect(rows[0]!.count).toBe(7);
  });

  it("replaces the whole metric snapshot on each ingest (no merge)", () => {
    useTelemetryStore
      .getState()
      .ingestMetrics(batch([() => counterMetric({ name: "a", total: 1 })]));
    useTelemetryStore
      .getState()
      .ingestMetrics(batch([() => counterMetric({ name: "b", total: 1 })]));
    expect(metricRows().map((r) => r.name)).toEqual(["b"]);
  });
});

describe("telemetry store: spans + logs ring buffers", () => {
  beforeEach(() => useTelemetryStore.getState().clear());

  it("appends spans and clamps to the newest when over cap", () => {
    const mk = (i: number) => ({
      id: `s${i}`,
      traceId: "t",
      name: "rpc",
      kind: "client",
      startMs: i,
      durationMs: 1,
      status: "ok" as const,
      attrs: {},
    });
    // Push more than the cap (500) and assert the oldest were dropped.
    useTelemetryStore.getState().ingestSpans(Array.from({ length: 600 }, (_, i) => mk(i)));
    const { spans } = useTelemetryStore.getState();
    expect(spans).toHaveLength(500);
    expect(spans[0]!.id).toBe("s100"); // first 100 dropped
    expect(spans.at(-1)!.id).toBe("s599");
  });

  it("clear empties all three signals", () => {
    useTelemetryStore
      .getState()
      .ingestLogs([{ id: "l1", timeMs: 0, severity: "INFO", body: "hi", attrs: {} }]);
    useTelemetryStore.getState().clear();
    const s = useTelemetryStore.getState();
    expect(s.logs).toHaveLength(0);
    expect(s.spans).toHaveLength(0);
    expect(Object.keys(s.metrics)).toHaveLength(0);
  });
});
