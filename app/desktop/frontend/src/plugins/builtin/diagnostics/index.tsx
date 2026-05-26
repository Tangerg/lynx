// Diagnostics plugin — installs a MeterProvider on top of
// @opentelemetry/api so the (until now no-op) measure calls in
// lib/metrics.ts start producing real data, then surfaces that data
// as a "Diagnostics" workspace view.
//
// Why an opt-in plugin rather than always-on instrumentation:
//   - No MeterProvider registered → all `measure*` calls are JS-level
//     no-ops (otel api returns proxy meters that swallow records).
//   - Loaded plugin → real ingestion + a UI to inspect it.
//   - Unloaded plugin → SDK shutdown clears the provider and frees
//     buffered data.
//
// SDK module is dynamic-imported so the ~150 KB / ~40 KB-gzip
// @opentelemetry/sdk-metrics bundle never lands in the main chunk —
// it only loads when this plugin's setup() runs.

import { metrics as otelApi } from "@opentelemetry/api";
import { definePlugin } from "@/plugins/sdk";
import { DiagnosticsView } from "./DiagnosticsView";

// Push the buffered metrics into the store roughly twice a second —
// fast enough to feel live in the table, slow enough to avoid React
// commit churn under heavy AG-UI traffic.
const EXPORT_INTERVAL_MS = 500;

export default definePlugin({
  name: "lyra.builtin.diagnostics",
  version: "1.0.0",
  setup({ host }) {
    let teardownProvider: (() => Promise<void>) | null = null;

    // Fire and forget — the workspace view is registered synchronously
    // below so the tab appears immediately; until the SDK resolves,
    // measure calls remain no-op and the table is empty.
    void (async () => {
      const [{ MeterProvider, PeriodicExportingMetricReader }, { DiagnosticsExporter }] =
        await Promise.all([import("@opentelemetry/sdk-metrics"), import("./exporter")]);
      const reader = new PeriodicExportingMetricReader({
        exporter: new DiagnosticsExporter(),
        exportIntervalMillis: EXPORT_INTERVAL_MS,
      });
      const provider = new MeterProvider({ readers: [reader] });
      otelApi.setGlobalMeterProvider(provider);
      teardownProvider = async () => {
        await provider.shutdown();
        otelApi.disable();
      };
    })();

    host.workspace.registerView({
      id: "diagnostics",
      title: "Diagnostics",
      icon: "spark",
      openByDefault: false,
      order: 90,
      component: DiagnosticsView,
    });

    // Cleanup runs on plugin unload — shuts down the SDK so further
    // measure calls revert to no-op and the buffered data is dropped.
    return async () => {
      await teardownProvider?.();
    };
  },
});
