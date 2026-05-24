// Namespaced key-value storage handed to each plugin.
//
//   const storage = createStorage("lyra.builtin.demo");
//   storage.set("threshold", 0.42);
//   storage.get<number>("threshold")  // → 0.42
//
// Keys live under `lyra.plugin.<plugin-name>.<key>` in localStorage so two
// plugins can never read each other's data and a stale plugin's keys are
// trivially purgeable.

const ROOT = "lyra.plugin";
/** Reserved key that records the highest-applied migration version. */
const SCHEMA_VERSION_KEY = "__schema_version";

/**
 * One step in a plugin's storage upgrade path. `version` is the new schema
 * version after the migration runs. The host applies migrations in
 * ascending order; each step is idempotent because it's only ever run
 * once per version bump (the current version is tracked in
 * `__schema_version`).
 */
export type StorageMigration = {
  version: number;
  /** Synchronous mutation against the plugin's namespace. */
  migrate: (store: KeyValueStore) => void;
};

export type KeyValueStore = {
  get<T = unknown>(key: string): T | undefined;
  set<T = unknown>(key: string, value: T): void;
  remove(key: string): void;
  /** Clear *all* keys this plugin has stored. Used on unload by tests. */
  clear(): void;
  /** List the plugin's keys (without the prefix). */
  keys(): string[];
  /**
   * Run an ordered set of migrations once each. Idempotent — pass the full
   * list every boot; only the unapplied ones execute. Migration errors
   * abort the chain and surface to the console (no partial bumps).
   */
  migrate(migrations: StorageMigration[]): void;
};

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
        // eslint-disable-next-line no-console
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

    migrate(migrations: StorageMigration[]): void {
      const self = this;
      const current = self.get<number>(SCHEMA_VERSION_KEY) ?? 0;
      // Stable sort so a migration list authored in arbitrary order still
      // applies low-to-high. Filter out steps we've already executed.
      const pending = [...migrations]
        .sort((a, b) => a.version - b.version)
        .filter((m) => m.version > current);
      for (const step of pending) {
        try {
          step.migrate(self);
          self.set(SCHEMA_VERSION_KEY, step.version);
        } catch (err) {
          // eslint-disable-next-line no-console
          console.error(`[plugin] ${pluginName} migration to v${step.version} failed:`, err);
          return;
        }
      }
    },
  };
}
