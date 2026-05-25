// Cross-cutting primitives shared by every other types file.

/**
 * A reversible handle. `register*` returns this; the host calls `.dispose()`
 * during unload so plugin authors never write cleanup code themselves.
 */
export interface Disposable { dispose: () => void }

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
