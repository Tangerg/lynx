// Diagnostics plugin — registers the "Diagnostics" workspace view and
// the lazy MeterProvider installer that view uses on first open.
//
// Why opt-in by view rather than always-on instrumentation:
//   - No MeterProvider registered (plugin loaded but view never
//     opened) → every `measure*` call in lib/metrics is a JS-level
//     no-op (otel-api returns proxy meters that swallow records).
//   - View first mounted → ensureProvider() runs once; subsequent
//     mounts hit the cached promise. Provider stays installed for
//     the rest of the session so history persists across tab
//     close/reopen.
//   - Plugin unloaded → teardown() shuts the SDK down and otel-api
//     falls back to no-op proxies again.
//
// The `@opentelemetry/sdk-metrics` module (~150 KB / ~40 KB gzip) is
// dynamic-imported inside ensureProvider, so Vite emits it as its
// own chunk and it never lands on the first-paint path.

import { metrics as otelApi } from "@opentelemetry/api";
import { definePlugin } from "@/plugins/sdk";
import { DiagnosticsView } from "./DiagnosticsView";

// Roughly twice a second — fast enough to feel live in the table,
// slow enough to avoid React commit churn under heavy AG-UI traffic.
const METRIC_EXPORT_INTERVAL_MS = 500;

// Module-scoped install state. `ensureProvider` is called by
// DiagnosticsView; `teardown` is called by the plugin's cleanup.
let installPromise: Promise<void> | null = null;
let teardown: (() => Promise<void>) | null = null;

export async function ensureProvider(): Promise<void> {
  if (installPromise) return installPromise;
  installPromise = (async () => {
    const [{ MeterProvider, PeriodicExportingMetricReader }, { DiagnosticsExporter }] =
      await Promise.all([import("@opentelemetry/sdk-metrics"), import("./exporter")]);
    const reader = new PeriodicExportingMetricReader({
      exporter: new DiagnosticsExporter(),
      exportIntervalMillis: METRIC_EXPORT_INTERVAL_MS,
    });
    const provider = new MeterProvider({ readers: [reader] });
    otelApi.setGlobalMeterProvider(provider);
    teardown = async () => {
      await provider.shutdown();
      otelApi.disable();
      installPromise = null;
      teardown = null;
    };
  })();
  return installPromise;
}

export default definePlugin({
  name: "lyra.builtin.diagnostics",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "diagnostics",
      title: "Diagnostics",
      icon: "spark",
      openByDefault: false,
      order: 90,
      component: DiagnosticsView,
    });
    return async () => {
      await teardown?.();
    };
  },
});
