// In-memory PushMetricExporter — handed to the SDK's
// PeriodicExportingMetricReader. Every flush drains otel's
// ResourceMetrics into the diagnostics Zustand store; the React view
// subscribes to that store and re-renders.
//
// CUMULATIVE temporality matches how the store overwrites on every
// export — no delta accumulation needed.

import type { AggregationTemporality, PushMetricExporter, ResourceMetrics } from "@opentelemetry/sdk-metrics";
import { useDiagnosticsStore } from "./store";

// Inlined enum values from @opentelemetry/sdk-metrics:
//   ExportResultCode.SUCCESS = 0  (from @opentelemetry/core)
//   AggregationTemporality.CUMULATIVE = 1
// Keeping them as literal constants means this file does NOT pull
// the SDK into the static graph — it stays lazy alongside the
// dynamic import in index.tsx.
const EXPORT_SUCCESS = 0;
const CUMULATIVE_TEMPORALITY = 1 as AggregationTemporality;

interface ExportResult { code: number }

export class DiagnosticsExporter implements PushMetricExporter {
  export(batch: ResourceMetrics, callback: (result: ExportResult) => void): void {
    useDiagnosticsStore.getState().ingest(batch);
    callback({ code: EXPORT_SUCCESS });
  }
  forceFlush(): Promise<void> {
    return Promise.resolve();
  }
  shutdown(): Promise<void> {
    useDiagnosticsStore.getState().clear();
    return Promise.resolve();
  }
  selectAggregationTemporality(): AggregationTemporality {
    return CUMULATIVE_TEMPORALITY;
  }
}
