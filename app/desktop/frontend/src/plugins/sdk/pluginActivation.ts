import type { PluginSpec } from "./types";
import type { LoadResult } from "./pluginLifecycle";
import { usePluginStore } from "./registry";

type PluginLoader = (spec: PluginSpec) => Promise<LoadResult>;

export function isEagerActivation(spec: PluginSpec): boolean {
  const events = spec.activationEvents;
  if (!events || events.length === 0) return true;
  return events.includes("onStartup");
}

export function stageLazyPlugin(spec: PluginSpec): void {
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

export function createLazyActivator(loadPlugin: PluginLoader) {
  return async (pluginName: string): Promise<void> => {
    const store = usePluginStore.getState();
    const pending = store.pendingActivations.get(pluginName);
    if (!pending) return;

    // Remove BEFORE the await so a concurrent second activation no-ops instead
    // of double-loading.
    store.removePendingActivation(pluginName);
    const result = await loadPlugin(pending.spec);
    if (result.kind !== "loaded") {
      // Setup failed: re-stage so the placeholders stay and the user can retry
      // (the error already surfaced via reportPluginError). Dropping them here
      // would permanently erase the plugin's palette commands/views after one
      // bad activation.
      store.addPendingActivation(pending.spec, pending.events);
      return;
    }

    // Real registrations are in place now; drop every placeholder owned by this
    // plugin so selectors emit the real specs from here on out.
    store.removeDeclaredCommandsBy(pluginName);
    store.removeDeclaredViewsBy(pluginName);
    store.removeDeclaredSettingsPanesBy(pluginName);
  };
}
