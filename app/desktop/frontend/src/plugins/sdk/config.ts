// App-wide configuration store.
//
// Distinct from `host.storage` (per-plugin namespaced) and `host.state.slice`
// (typed sharable state). Config is for *app-level* settings that any
// plugin can read or change — feature flags, debug toggles, HTTP base URL
// overrides, etc.
//
// In-memory only. Persistence is the plugin's responsibility: a plugin
// can subscribe to a key and mirror it to localStorage if needed.

import type { Disposable } from "./types/common";
import { create } from "zustand";
import { safeCall } from "./errors";

export type ConfigValue =
  | string
  | number
  | boolean
  | null
  | ConfigValue[]
  | { [key: string]: ConfigValue };

interface ConfigStoreState {
  values: Map<string, ConfigValue>;
  subscribers: Map<string, Set<(value: ConfigValue | undefined) => void>>;
}

interface ConfigStoreActions {
  set: (key: string, value: ConfigValue) => void;
  subscribe: (key: string, fn: (value: ConfigValue | undefined) => void) => Disposable;
}

export const useConfigStore = create<ConfigStoreState & ConfigStoreActions>((set, get) => ({
  values: new Map(),
  subscribers: new Map(),

  set(key, value) {
    const cur = get().values.get(key);
    if (Object.is(cur, value)) return;
    const next = new Map(get().values);
    next.set(key, value);
    set({ values: next });
    const subs = get().subscribers.get(key);
    if (!subs) return;
    for (const fn of [...subs]) {
      safeCall(() => fn(value), `[plugin] config subscriber for "${key}" threw:`);
    }
  },

  subscribe(key, fn) {
    const subs = get().subscribers;
    const next = new Map(subs);
    const set_ = new Set(next.get(key) ?? []);
    set_.add(fn);
    next.set(key, set_);
    set({ subscribers: next });
    return {
      dispose: () => {
        const cur = get().subscribers.get(key);
        if (!cur) return;
        const after = new Set(cur);
        after.delete(fn);
        const after2 = new Map(get().subscribers);
        if (after.size === 0) after2.delete(key);
        else after2.set(key, after);
        set({ subscribers: after2 });
      },
    };
  },
}));

/** Imperative read with optional fallback. */
export function getConfig<T = ConfigValue>(key: string, defaultValue?: T): T | undefined {
  const v = useConfigStore.getState().values.get(key);
  return v === undefined ? defaultValue : (v as unknown as T);
}

/** Imperative write. */
export function setConfig(key: string, value: ConfigValue): void {
  useConfigStore.getState().set(key, value);
}

export function hasConfig(key: string): boolean {
  return useConfigStore.getState().values.has(key);
}
