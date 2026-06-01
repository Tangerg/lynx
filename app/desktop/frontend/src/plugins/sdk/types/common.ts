// Cross-cutting primitives shared by every other types file.

/**
 * A reversible handle. `register*` returns this; the host calls `.dispose()`
 * during unload so plugin authors never write cleanup code themselves.
 */
export interface Disposable {
  dispose: () => void;
}

/**
 * Fires once, when PluginProvider has finished loading all built-in
 * plugins (sideloaded plugins may still be in-flight). Registering a hook
 * after the ready point fires it synchronously / on the next microtask —
 * "have I missed it" is never a concern.
 *
 * Common use: a plugin whose setup needs to read the full registry
 * (e.g. snapshot every accent, every command). Registering at setup time
 * is order-dependent; deferring to onReady is not.
 */
export type ReadyHandler = () => void;

/**
 * Fires on `window.beforeunload`. Synchronous — use it for "flush
 * something quickly" cleanup, not promise-y teardown.
 */
export type BeforeUnloadHandler = () => void;

/**
 * The permission vocabulary. Each value names a top-level `Host` namespace and
 * doubles as the gate for contributing to a kernel extension point (a point's
 * `capability`). A plugin narrows what it can touch by listing only the ones it
 * needs in `PluginSpec.capabilities`; anything else becomes a throwing proxy /
 * a denied `contribute`. Lives in this leaf module so both `plugin` and
 * `extensions` can reference it without forming a type cycle.
 */
export type HostCapability =
  | "tool"
  | "message"
  | "agui"
  | "layout"
  | "workspace"
  | "theme"
  | "router"
  | "composer"
  | "sidebar"
  | "shortcuts"
  | "agent"
  | "data"
  | "commands"
  | "extensions"
  | "lifecycle"
  | "state"
  | "config"
  | "settings"
  | "storage"
  | "rpc"
  | "notify"
  | "window"
  | "plugins"
  | "log"
  | "i18n"
  | "tasks";
