// Cross-cutting primitives shared by every other types file.

/**
 * A reversible application-level handle. Dougong owns plugin setup resources;
 * this narrower shape remains for UI registrations outside its core contracts.
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
 * The application permission vocabulary used by extension points and sideload
 * manifests. A point's `capability` is checked before contribution, while the
 * dougong Platform authorizes the same vocabulary before loading third-party
 * code. Lives here so plugin and extension types share it without a cycle.
 */
export type HostCapability =
  | "tool"
  | "message"
  | "events"
  | "layout"
  | "workspace"
  | "theme"
  | "router"
  | "composer"
  | "navigation"
  | "shortcuts"
  | "agent"
  | "data"
  | "commands"
  | "extensions"
  | "lifecycle"
  | "config"
  | "settings"
  | "storage"
  | "notify"
  | "window"
  | "plugins"
  | "log"
  | "i18n"
  | "tasks";
