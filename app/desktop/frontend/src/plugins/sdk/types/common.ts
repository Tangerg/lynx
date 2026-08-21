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
 * plugins. Registering a hook
 * after the ready point fires it synchronously / on the next microtask —
 * "have I missed it" is never a concern.
 *
 * Common use: a plugin whose setup needs to read the full registry
 * (e.g. snapshot every accent, every command). Registering at setup time
 * is order-dependent; deferring to onReady is not.
 */
export type ReadyHandler = () => void;
