// In-memory PushMetricExporter — handed to the SDK's
// PeriodicExportingMetricReader. Every flush drains otel's
// ResourceMetrics into the diagnostics Zustand store; the React view
// subscribes to that store and re-renders.
//
// CUMULATIVE temporality matches how the store overwrites on every
// export — no delta accumulation needed.

import type { PushMetricExporter, ResourceMetrics } from "@opentelemetry/sdk-metrics";
import { AggregationTemporality } from "@opentelemetry/sdk-metrics";
import { useDiagnosticsStore } from "./store";

// ExportResultCode.SUCCESS = 0 in @opentelemetry/core. We inline the
// shape rather than depend on `core` for one numeric constant.
interface ExportResult { code: number }

export class DiagnosticsExporter implements PushMetricExporter {
  export(batch: ResourceMetrics, callback: (result: ExportResult) => void): void {
    useDiagnosticsStore.getState().ingest(batch);
    callback({ code: 0 });
  }
  forceFlush(): Promise<void> {
    return Promise.resolve();
  }
  shutdown(): Promise<void> {
    useDiagnosticsStore.getState().clear();
    return Promise.resolve();
  }
  selectAggregationTemporality(): AggregationTemporality {
    return AggregationTemporality.CUMULATIVE;
  }
}
