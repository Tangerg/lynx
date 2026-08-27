// Namespaced key-value storage handed to each plugin.
//
//   const storage = createStorage("scopeapp.builtin.my-plugin");
//   storage.set("threshold", 0.42);
//   const stored = storage.get("threshold");
//   const threshold = typeof stored === "number" ? stored : undefined;
//
// Keys live under `scopeapp.plugin.<plugin-name>.<key>` in localStorage so two
// plugins can never read each other's data and a stale plugin's keys are
// trivially purgeable.

const ROOT = "scopeapp.plugin";

export interface KeyValueStore {
  /** Persistent data is untrusted until the consuming boundary validates it. */
  get: (key: string) => unknown;
  set: (key: string, value: unknown) => void;
  remove: (key: string) => void;
  /** Clear *all* keys this plugin has stored. Used on unload by tests. */
  clear: () => void;
  /** List the plugin's keys (without the prefix). */
  keys: () => string[];
}

function readStorage<T>(fallback: T, read: (storage: Storage) => T): T {
  try {
    if (typeof window === "undefined") return fallback;
    return read(window.localStorage);
  } catch {
    return fallback;
  }
}

function writeStorage(operation: string, write: (storage: Storage) => void): void {
  try {
    if (typeof window === "undefined") return;
    write(window.localStorage);
  } catch (error) {
    console.warn(`[plugin] storage.${operation} failed:`, error);
  }
}

export function createStorage(pluginName: string): KeyValueStore {
  const prefix = `${ROOT}.${pluginName}.`;

  return {
    get(key: string): unknown {
      return readStorage<unknown>(undefined, (storage) => {
        const raw = storage.getItem(prefix + key);
        if (raw == null) return undefined;
        try {
          const parsed: unknown = JSON.parse(raw);
          return parsed;
        } catch {
          return raw;
        }
      });
    },

    set(key: string, value: unknown): void {
      writeStorage(`set("${key}")`, (storage) => {
        storage.setItem(prefix + key, JSON.stringify(value));
      });
    },

    remove(key: string): void {
      writeStorage(`remove("${key}")`, (storage) => {
        storage.removeItem(prefix + key);
      });
    },

    clear(): void {
      writeStorage("clear()", (storage) => {
        const doomed: string[] = [];
        for (let i = 0; i < storage.length; i++) {
          const key = storage.key(i);
          if (key?.startsWith(prefix)) doomed.push(key);
        }
        for (const key of doomed) storage.removeItem(key);
      });
    },

    keys(): string[] {
      return readStorage<string[]>([], (storage) => {
        const keys: string[] = [];
        for (let i = 0; i < storage.length; i++) {
          const key = storage.key(i);
          if (key?.startsWith(prefix)) keys.push(key.slice(prefix.length));
        }
        return keys;
      });
    },
  };
}
