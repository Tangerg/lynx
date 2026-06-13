// Namespaced key-value storage handed to each plugin.
//
//   const storage = createStorage("lyra.builtin.my-plugin");
//   storage.set("threshold", 0.42);
//   storage.get<number>("threshold")  // → 0.42
//
// Keys live under `lyra.plugin.<plugin-name>.<key>` in localStorage so two
// plugins can never read each other's data and a stale plugin's keys are
// trivially purgeable.

const ROOT = "lyra.plugin";

export interface KeyValueStore {
  get: <T = unknown>(key: string) => T | undefined;
  set: <T = unknown>(key: string, value: T) => void;
  remove: (key: string) => void;
  /** Clear *all* keys this plugin has stored. Used on unload by tests. */
  clear: () => void;
  /** List the plugin's keys (without the prefix). */
  keys: () => string[];
}

export function createStorage(pluginName: string): KeyValueStore {
  const prefix = `${ROOT}.${pluginName}.`;

  // localStorage may throw in private-mode or sandboxed contexts. Wrap every
  // op so a plugin author doesn't have to handle the cross-browser quirks.
  const safeStorage = (): Storage | null => {
    try {
      return typeof window !== "undefined" ? window.localStorage : null;
    } catch {
      return null;
    }
  };

  return {
    get<T = unknown>(key: string): T | undefined {
      const ls = safeStorage();
      if (!ls) return undefined;
      const raw = ls.getItem(prefix + key);
      if (raw == null) return undefined;
      try {
        return JSON.parse(raw) as T;
      } catch {
        // Stored as opaque string — return as-is.
        return raw as unknown as T;
      }
    },

    set<T = unknown>(key: string, value: T): void {
      const ls = safeStorage();
      if (!ls) return;
      try {
        ls.setItem(prefix + key, JSON.stringify(value));
      } catch (err) {
        // Quota exceeded, etc. Surface to console but don't throw — plugins
        // shouldn't crash because storage is full.

        console.warn(`[plugin] storage.set("${key}") failed:`, err);
      }
    },

    remove(key: string): void {
      const ls = safeStorage();
      if (!ls) return;
      ls.removeItem(prefix + key);
    },

    clear(): void {
      const ls = safeStorage();
      if (!ls) return;
      const doomed: string[] = [];
      for (let i = 0; i < ls.length; i++) {
        const k = ls.key(i);
        if (k && k.startsWith(prefix)) doomed.push(k);
      }
      for (const k of doomed) ls.removeItem(k);
    },

    keys(): string[] {
      const ls = safeStorage();
      if (!ls) return [];
      const out: string[] = [];
      for (let i = 0; i < ls.length; i++) {
        const k = ls.key(i);
        if (k && k.startsWith(prefix)) out.push(k.slice(prefix.length));
      }
      return out;
    },
  };
}
