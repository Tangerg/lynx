import type { PluginSpec } from "./types";

// Plugin runtime injection point — definePlugin.ts installs the real
// implementation at module load time. Host plugin-management methods dispatch
// through this seam so we don't introduce a circular import.
interface PluginRuntime {
  load: (spec: PluginSpec) => Promise<void>;
  unload: (name: string) => void;
  reload: (name: string) => Promise<void>;
}

// A const wrapper keeps the binding initialized during module evaluation. A
// bare `let pluginRuntime` can hit a TDZ under Vitest's module loader if the
// setter is reached before the declaration.
const runtimeSlot: { current: PluginRuntime | null } = { current: null };

export function setPluginRuntime(rt: PluginRuntime): void {
  runtimeSlot.current = rt;
}

export function getPluginRuntime(): PluginRuntime {
  if (!runtimeSlot.current) {
    throw new Error("plugin runtime not wired; call setPluginRuntime() before host is used");
  }
  return runtimeSlot.current;
}
