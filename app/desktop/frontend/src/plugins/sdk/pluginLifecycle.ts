import type { Disposable, PluginSpec } from "./types";
import { satisfies } from "compare-versions";
import { measurePluginLoad } from "@/lib/metrics";
import { HOST_API_VERSION } from "./apiVersion";
import { aggregateRisk } from "./capabilities";
import { reportPluginError } from "./errors";
import { createHost } from "./host";
import { pluginOrigin } from "./pluginOrigin";
import { usePluginStore } from "./registry";

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
  // registerLoaded would overwrite the existing entry, orphaning its
  // disposables and double-registering every multi-point handler. Replacing
  // goes through unload → load (reloadPlugin), never implicit overwrite.
  if (usePluginStore.getState().loaded.has(spec.name)) {
    const reason = "already loaded — unload first to replace";
    console.warn(`[plugin] ${spec.name} skipped: ${reason}`);
    return { kind: "skipped", name: spec.name, reason };
  }

  if (spec.apiVersion) {
    try {
      if (!satisfies(HOST_API_VERSION, spec.apiVersion)) {
        const reason = `requires host apiVersion ${spec.apiVersion}, this host is ${HOST_API_VERSION}`;

        console.warn(`[plugin] ${spec.name} skipped: ${reason}`);
        reportPluginError(spec.name, "setup", new Error(reason));
        return { kind: "skipped", name: spec.name, reason };
      }
    } catch (err) {
      const reason = `invalid apiVersion range "${spec.apiVersion}": ${
        err instanceof Error ? err.message : String(err)
      }`;
      reportPluginError(spec.name, "setup", new Error(reason));
      return { kind: "skipped", name: spec.name, reason };
    }
  }

  const disposables: Disposable[] = [];
  // Sideload default-deny: a third-party bundle that declares no capabilities
  // gets none, never full access. Built-ins keep full access when omitted.
  const origin = pluginOrigin(spec.name);
  const declared = origin === "sideload" ? (spec.capabilities ?? []) : spec.capabilities;
  if (origin === "sideload" && declared && declared.length > 0) {
    console.info(
      `[plugin] sideload "${spec.name}" capabilities [${declared.join(", ")}] — ` +
        `risk: ${aggregateRisk(declared)}`,
    );
  }
  const host = createHost(spec.name, disposables, declared);

  const start = performance.now();
  try {
    const cleanup = await spec.setup({ host });
    measurePluginLoad(performance.now() - start, spec.name, "loaded");
    if (typeof cleanup === "function") {
      disposables.push({ dispose: cleanup });
    }
    usePluginStore.getState().registerLoaded({ spec, disposables });
    return { kind: "loaded", name: spec.name };
  } catch (err) {
    measurePluginLoad(performance.now() - start, spec.name, "failed");
    console.error(`[plugin] ${spec.name} setup failed:`, err);
    reportPluginError(spec.name, "setup", err);
    for (const d of disposables) {
      try {
        d.dispose();
      } catch {
        /* swallow */
      }
    }
    return { kind: "failed", name: spec.name, error: err };
  }
}

/**
 * Unload a previously-loaded plugin. Drops every disposable collected during
 * its setup, removes it from the loaded map, fires onUnload listeners. No-op if
 * the plugin isn't currently loaded.
 */
export function unloadPlugin(name: string): void {
  usePluginStore.getState().unload(name);
}

/**
 * Convenience: unload + load the same spec. Returns the load promise so callers
 * can await setup completion.
 */
export async function reloadPlugin(name: string): Promise<void> {
  const current = usePluginStore.getState().loaded.get(name);
  if (!current) return;
  unloadPlugin(name);
  await loadPlugin(current.spec);
}
