import type { PluginSpec } from "./types";
import { setPluginRuntime } from "./hostRuntime";
import { createLazyActivator } from "./pluginActivation";
import { loadPlugin, reloadPlugin, unloadPlugin } from "./pluginLifecycle";
import { loadPlugins } from "./pluginManifest";
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

// Keep composition wiring at the SDK facade so lifecycle modules stay acyclic.
setActivator(createLazyActivator(loadPlugin));

setPluginRuntime({
  load: async (spec) => {
    await loadPlugin(spec);
  },
  unload: unloadPlugin,
  reload: reloadPlugin,
});

export type { LoadResult } from "./pluginLifecycle";
export { loadPlugin, loadPlugins, reloadPlugin, unloadPlugin };
