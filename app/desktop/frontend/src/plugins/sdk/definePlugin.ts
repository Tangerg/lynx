import type { Disposable, PluginSpec } from "./types";
import { satisfies } from "compare-versions";
import { measurePluginLoad } from "@/lib/metrics";
import { HOST_API_VERSION } from "./apiVersion";
import { reportPluginError } from "./errors";
import { createHost, setPluginRuntime } from "./host";
import { usePluginStore } from "./registry";
import { setActivator } from "./selectors";

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
  const host = createHost(spec.name, disposables, spec.capabilities);

  try {
    const start = performance.now();
    const cleanup = await spec.setup({ host });
    measurePluginLoad(performance.now() - start, spec.name);
    if (typeof cleanup === "function") {
      // setup returned a cleanup fn — fold it into the disposable list so
      // unloadPlugin triggers it alongside every register* result.
      disposables.push({ dispose: cleanup });
    }
    usePluginStore.getState().registerLoaded({ spec, disposables });
    return { kind: "loaded", name: spec.name };
  } catch (err) {
     
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
 * Load a list of plugins in topological order honouring `spec.requires`.
 *
 * Order semantics:
 *   - A plugin with no `requires` keeps its position in the input array.
 *   - When a plugin declares dependencies, it loads after all of them.
 *   - Manifest order acts as the tie-breaker between independent plugins.
 *
 * Activation:
 *   - Specs without `activationEvents`, or those that include "onStartup",
 *     run setup eagerly here.
 *   - Specs that only declare lazy events (`onCommand:…`) skip setup —
 *     their `contributes.commands` are registered as placeholders and the
 *     spec is recorded for later activation. The first time a user runs
 *     one of the contributed commands, `activateLazy` runs setup, after
 *     which the real handler takes over.
 *
 * Errors are isolated per plugin:
 *   - Missing dependencies: dependent plugin is skipped with a clear reason.
 *   - Cycles: every plugin participating in the cycle is skipped.
 *   - Setup throws: handled by `loadPlugin` (already isolated).
 */
export async function loadPlugins(specs: PluginSpec[]): Promise<LoadResult[]> {
  const { order, skipped } = topoSort(specs);
  const out: LoadResult[] = [];
  for (const s of skipped) {
    reportPluginError(s.name, "setup", new Error(s.reason));
    out.push({ kind: "skipped", name: s.name, reason: s.reason });
  }
  for (const spec of order) {
    if (isEager(spec)) {
      out.push(await loadPlugin(spec));
    } else {
      stageLazyPlugin(spec);
      out.push({ kind: "loaded", name: spec.name });
    }
  }
  return out;
}

function isEager(spec: PluginSpec): boolean {
  const events = spec.activationEvents;
  if (!events || events.length === 0) return true;
  return events.includes("onStartup");
}

function stageLazyPlugin(spec: PluginSpec): void {
  const store = usePluginStore.getState();
  for (const c of spec.contributes?.commands ?? []) {
    store.addDeclaredCommand(spec.name, c);
  }
  for (const v of spec.contributes?.views ?? []) {
    store.addDeclaredView(spec.name, v);
  }
  for (const p of spec.contributes?.settingsPanes ?? []) {
    store.addDeclaredSettingsPane(spec.name, p);
  }
  store.addPendingActivation(spec, spec.activationEvents ?? []);
}

async function activateLazy(pluginName: string): Promise<void> {
  const store = usePluginStore.getState();
  const pending = store.pendingActivations.get(pluginName);
  if (!pending) return;
  store.removePendingActivation(pluginName);
  await loadPlugin(pending.spec);
  // Real registrations are in place now; drop every placeholder owned by
  // this plugin so selectors emit the real specs from here on out.
  store.removeDeclaredCommandsBy(pluginName);
  store.removeDeclaredViewsBy(pluginName);
  store.removeDeclaredSettingsPanesBy(pluginName);
}

// Install the activator hook so the placeholder command produced by
// `useCommands` can dispatch back here.
setActivator(activateLazy);

/**
 * Unload a previously-loaded plugin. Drops every disposable collected
 * during its setup, removes it from the loaded map, fires onUnload
 * listeners. No-op if the plugin isn't currently loaded.
 */
export function unloadPlugin(name: string): void {
  usePluginStore.getState().unload(name);
}

/**
 * Convenience: unload + load the same spec. Returns the load promise
 * so callers can `await` setup completion.
 *
 * If `name` isn't currently loaded, this is just a `loadPlugin` call
 * provided the spec is available (we look it up in the loaded map).
 */
export async function reloadPlugin(name: string): Promise<void> {
  const current = usePluginStore.getState().loaded.get(name);
  if (!current) return; // nothing to reload
  unloadPlugin(name);
  await loadPlugin(current.spec);
}

// Expose load/unload/reload to host.plugins via the runtime seam.
setPluginRuntime({
  load: async (spec) => {
    await loadPlugin(spec);
  },
  unload: unloadPlugin,
  reload: reloadPlugin,
});

interface Skipped { name: string; reason: string }

/**
 * Kahn-style topological sort. Picks the ready node with the lowest input
 * index so manifest order survives as a tie-breaker.
 */
function topoSort(specs: PluginSpec[]): { order: PluginSpec[]; skipped: Skipped[] } {
  const byName = new Map(specs.map((s) => [s.name, s]));
  const index = new Map(specs.map((s, i) => [s.name, i]));
  const skipped: Skipped[] = [];

  // Filter requires: missing deps mark the dependent as skipped right away.
  const requires = new Map<string, string[]>();
  for (const s of specs) {
    const deps = (s.requires ?? []).filter((dep) => {
      if (!byName.has(dep)) {
        skipped.push({
          name: s.name,
          reason: `requires "${dep}" which is not loaded`,
        });
        return false;
      }
      return true;
    });
    requires.set(s.name, deps);
  }
  const skippedNames = new Set(skipped.map((s) => s.name));

  const inDegree = new Map<string, number>();
  const dependents = new Map<string, string[]>();
  for (const s of specs) {
    if (skippedNames.has(s.name)) continue;
    inDegree.set(s.name, 0);
    dependents.set(s.name, []);
  }
  for (const s of specs) {
    if (skippedNames.has(s.name)) continue;
    for (const dep of requires.get(s.name)!) {
      if (skippedNames.has(dep)) continue;
      inDegree.set(s.name, (inDegree.get(s.name) ?? 0) + 1);
      dependents.get(dep)!.push(s.name);
    }
  }

  const ready = new Set<string>();
  for (const [name, deg] of inDegree) if (deg === 0) ready.add(name);

  const order: PluginSpec[] = [];
  while (ready.size > 0) {
    let pick: string | null = null;
    let pickIdx = Infinity;
    for (const name of ready) {
      const i = index.get(name)!;
      if (i < pickIdx) {
        pick = name;
        pickIdx = i;
      }
    }
    ready.delete(pick!);
    order.push(byName.get(pick!)!);
    for (const child of dependents.get(pick!)!) {
      const next = (inDegree.get(child) ?? 0) - 1;
      inDegree.set(child, next);
      if (next === 0) ready.add(child);
    }
  }

  // Anyone still with non-zero in-degree is in (or downstream of) a cycle.
  for (const [name, deg] of inDegree) {
    if (deg > 0) {
      skipped.push({ name, reason: "dependency cycle (skipped)" });
    }
  }

  return { order, skipped };
}
