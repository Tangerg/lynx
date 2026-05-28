// MeterProvider install/teardown — split out of `./index.tsx` so the
// view component can call `ensureProvider` without creating a module
// cycle (index.tsx imports the view, the view imports the provider).
//
// The `@opentelemetry/sdk-metrics` module (~150 KB / ~40 KB gzip) is
// dynamic-imported here so Vite emits it as its own chunk and it never
// lands on the first-paint path.

import { metrics as otelApi } from "@opentelemetry/api";

// Roughly twice a second — fast enough to feel live in the table, slow
// enough to avoid React commit churn under heavy AG-UI traffic.
const METRIC_EXPORT_INTERVAL_MS = 500;

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

export async function teardownProvider(): Promise<void> {
  await teardown?.();
}
