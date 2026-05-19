import { satisfies } from "compare-versions";
import { HOST_API_VERSION } from "./apiVersion";
import { reportPluginError } from "./errors";
import { createHost } from "./host";
import { usePluginStore } from "./registry";
import type { Disposable, PluginSpec } from "./types";

/**
 * Identity function — `definePlugin({ ... })` is what plugins default-export.
 *
 * Why a function and not a plain object: keeps the door open for runtime
 * validation, schema checking, or wrapping the spec (without breaking the
 * call site).
 */
export function definePlugin(spec: PluginSpec): PluginSpec {
  return spec;
}

/** Result of `loadPlugin` — useful for callers that want to handle skips. */
export type LoadResult =
  | { kind: "loaded"; name: string }
  | { kind: "skipped"; name: string; reason: string }
  | { kind: "failed"; name: string; error: unknown };

/**
 * Load one plugin: validate apiVersion, build a host, run setup, record the
 * loaded plugin.
 *
 * Errors during setup don't propagate — we log, dispose anything that did
 * register before the crash, and return a `failed` result. The host stays up.
 */
export async function loadPlugin(spec: PluginSpec): Promise<LoadResult> {
  // 1. apiVersion gate. Plugins that don't declare a range are accepted —
  //    they implicitly trust whatever host is running them.
  if (spec.apiVersion) {
    try {
      if (!satisfies(HOST_API_VERSION, spec.apiVersion)) {
        const reason = `requires host apiVersion ${spec.apiVersion}, this host is ${HOST_API_VERSION}`;
        // eslint-disable-next-line no-console
        console.warn(`[plugin] ${spec.name} skipped: ${reason}`);
        reportPluginError(spec.name, "setup", new Error(reason));
        return { kind: "skipped", name: spec.name, reason };
      }
    } catch (err) {
      // Malformed range string. Treat as a load failure so the plugin
      // author sees it.
      const reason = `invalid apiVersion range "${spec.apiVersion}": ${err instanceof Error ? err.message : String(err)}`;
      reportPluginError(spec.name, "setup", new Error(reason));
      return { kind: "skipped", name: spec.name, reason };
    }
  }

  // 2. Setup.
  const disposables: Disposable[] = [];
  const host = createHost(spec.name, disposables);

  try {
    await spec.setup({ host });
    usePluginStore.getState().registerLoaded({ spec, disposables });
    return { kind: "loaded", name: spec.name };
  } catch (err) {
    // eslint-disable-next-line no-console
    console.error(`[plugin] ${spec.name} setup failed:`, err);
    reportPluginError(spec.name, "setup", err);
    for (const d of disposables) {
      try { d.dispose(); } catch { /* swallow */ }
    }
    return { kind: "failed", name: spec.name, error: err };
  }
}

/** Load a list of plugins in order. Failures are isolated per plugin. */
export async function loadPlugins(specs: PluginSpec[]): Promise<LoadResult[]> {
  const out: LoadResult[] = [];
  for (const spec of specs) {
    out.push(await loadPlugin(spec));
  }
  return out;
}
