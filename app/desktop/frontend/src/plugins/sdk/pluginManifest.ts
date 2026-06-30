import type { PluginSpec } from "./types";
import type { LoadResult } from "./pluginLifecycle";
import { reportPluginError } from "./errors";
import { isEagerActivation, stageLazyPlugin } from "./pluginActivation";
import { loadPlugin } from "./pluginLifecycle";
import { orderPlugins } from "./pluginOrder";

/**
 * Load a list of plugins in topological order honouring `spec.requires`.
 *
 * Order semantics:
 *   - A plugin with no `requires` keeps its position in the input array.
 *   - When a plugin declares dependencies, it loads after all of them.
 *   - Manifest order acts as the tie-breaker between independent plugins.
 *
 * Activation:
 *   - Specs without `activationEvents`, or those that include "onStartup", run
 *     setup eagerly here.
 *   - Specs that only declare lazy events skip setup: their declarative
 *     contributions are staged as placeholders and activated on first use.
 */
export async function loadPlugins(specs: PluginSpec[]): Promise<LoadResult[]> {
  const { order, skipped } = orderPlugins(specs);
  const out: LoadResult[] = [];
  for (const s of skipped) {
    reportPluginError(s.name, "setup", new Error(s.reason));
    out.push({ kind: "skipped", name: s.name, reason: s.reason });
  }
  for (const spec of order) {
    if (isEagerActivation(spec)) {
      out.push(await loadPlugin(spec));
    } else {
      stageLazyPlugin(spec);
      out.push({ kind: "loaded", name: spec.name });
    }
  }
  return out;
}
