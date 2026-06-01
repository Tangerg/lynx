// The Host facade — every register* surface a plugin can touch.
//
// Each namespace groups by domain (matches the per-domain type files).
// Adding a new extension point: drop a new spec type in its domain file,
// surface it as a `register*` method here, wire it through registry.ts.

import type { ConfigValue } from "../config";
import type { StateSlice } from "../stateSlice";
import type { CoreEventHandler, CustomEventHandler } from "./agui";

import type { CommandSpec } from "./commands";
import type { ExtensionContributionOptions, ExtensionPoint } from "./extensions";
import type { BeforeUnloadHandler, Disposable, ReadyHandler } from "./common";
import type { LocaleSpec } from "./i18n";
import type {
  LogSubscriber,
  NotificationLevel,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
  TaskHandle,
  TaskStartOptions,
} from "./infra";
import type { ContentBlockRenderer } from "./message";
import type { LoadedPlugin, PluginSpec } from "./plugin";
import type { LayoutSlotSpec, WorkspaceViewSpec } from "./workspace";
import type { ContentBlockKind } from "@/protocol/agui/viewState";

export interface Host {
  message: {
    /** Register a renderer for a content-block kind. */
    registerContentBlock: <K extends ContentBlockKind>(
      kind: K,
      renderer: ContentBlockRenderer<K>,
    ) => Disposable;
  };
  agui: {
    /** Subscribe to an AG-UI CUSTOM event by name. */
    on: <T = unknown>(name: string, handler: CustomEventHandler<T>) => Disposable;
    /**
     * Subscribe to an AG-UI *built-in* event type (RUN_STARTED, etc.).
     *
     * Handlers chain: the reducer dispatches one event through every plugin
     * registered for its type, in registration order, threading state from
     * one to the next. Throwing isolates to the offending plugin and falls
     * back to the input state (same isolation policy as `on`).
     */
    onCore: (eventType: string, handler: CoreEventHandler) => Disposable;
  };
  layout: {
    /** Contribute a component to a named kernel region. */
    register: (slot: string, spec: LayoutSlotSpec) => Disposable;
  };
  workspace: {
    /** Contribute a dockable view that participates in the workspace layout. */
    registerView: (spec: WorkspaceViewSpec) => Disposable;
    /** Open (or focus) a registered view by id. Imperative trigger from a
     *  command palette entry / slash command / external link. */
    openView: (id: string) => void;
    /** Close a registered view by id. */
    closeView: (id: string) => void;
  };
  commands: {
    /** Contribute a command palette entry. */
    register: (spec: CommandSpec) => Disposable;
  };
  extensions: {
    /**
     * Contribute a typed item to a plugin-defined extension point. The point
     * is an `ExtensionPoint<T>` handle from `defineExtensionPoint` — no
     * pre-declaration needed, mirroring `agui.on`. Multiple plugins (or the
     * same plugin via distinct `opts.id` on a `multi` point) can contribute;
     * consumers read the sorted list via `useExtensionPoint` /
     * `lookupExtensionPoint`. This is the JetBrains-style "plugins open and
     * fill each other's points" substrate.
     */
    contribute: <T>(
      point: ExtensionPoint<T>,
      item: T,
      opts?: ExtensionContributionOptions,
    ) => Disposable;
  };
  lifecycle: {
    /** Fires once after the built-in plugin set finishes loading. */
    onReady: (fn: ReadyHandler) => Disposable;
    /** Fires synchronously on window.beforeunload. */
    onBeforeUnload: (fn: BeforeUnloadHandler) => Disposable;
  };
  state: {
    /**
     * Get (or create) the shared `StateSlice` for `name`. The first caller's
     * `initial` wins — subsequent calls receive the same slice and ignore
     * their `initial` argument.
     *
     * Use it to share ephemeral state between plugins without forming a
     * hard module import: producer + consumer agree on the slice name and
     * the type.
     */
    slice: <T>(name: string, initial: T) => StateSlice<T>;
  };
  config: {
    /** Read an app-wide config value (with optional fallback). */
    get: <T = ConfigValue>(key: string, defaultValue?: T) => T | undefined;
    /** Set an app-wide config value. Fires subscribers. */
    set: (key: string, value: ConfigValue) => void;
    /** Does the key have a value (regardless of falsiness)? */
    has: (key: string) => boolean;
    /** Subscribe to changes for one key. Receives the new value (or undefined). */
    onChange: (key: string, fn: (value: ConfigValue | undefined) => void) => Disposable;
  };
  /** Namespaced key-value storage, persisted to localStorage. */
  storage: {
    get: <T = unknown>(key: string) => T | undefined;
    set: <T = unknown>(key: string, value: T) => void;
    remove: (key: string) => void;
    keys: () => string[];
  };
  rpc: {
    /** GET against the Go backend (baseUrl is pre-configured). */
    get: <T>(path: string, params?: Record<string, unknown>) => Promise<T>;
    /** POST against the Go backend. */
    post: <T>(path: string, body?: unknown) => Promise<T>;
    /**
     * Register a request hook. Runs for every call made through the shared
     * `api` ky instance (which is what `host.rpc.get/post`, queries.ts, and
     * kernel-chat all use). Common uses: auth headers, request logging,
     * X-Request-Id injection.
     */
    beforeRequest: (hook: RpcBeforeRequestHook) => Disposable;
    /**
     * Register a response hook. Runs after every call via the shared `api`
     * instance. Common uses: response logging, automatic token refresh,
     * normalising error envelopes.
     */
    afterResponse: (hook: RpcAfterResponseHook) => Disposable;
  };
  i18n: {
    /**
     * Merge a translation dictionary into the kernel's i18n store for
     * `locale`. Plugin keys live alongside the kernel's; lookups via
     * `useT()` / `t()` resolve them normally. Last writer wins for any
     * collision.
     *
     * The returned disposable is a no-op (i18next has no per-key removal
     * in its public API), but a same-name plugin reload safely
     * re-overwrites the same keys.
     */
    addBundle: (locale: string, dict: Record<string, string>) => Disposable;
    /**
     * Register a language with the Settings → Language picker. The
     * `spec.id` should match the locale passed to `addBundle`. Sort by
     * `order` ascending; built-ins use 0..99. Returns a disposable that
     * removes the entry from the picker when the plugin unloads.
     *
     * Typical flow inside a locale plugin's setup:
     *
     *   host.i18n.addBundle("ja", japaneseDict);
     *   host.i18n.registerLocale({ id: "ja", label: "日本語", order: 30 });
     */
    registerLocale: (spec: LocaleSpec) => Disposable;
  };
  tasks: {
    /**
     * Register a long-running task. Returns a handle for updating progress
     * + marking the task complete or failed. Settled tasks linger in the
     * status bar briefly so the final state is visible.
     */
    start: (opts: TaskStartOptions) => TaskHandle;
  };
  /** Display a brief toast notification. */
  notify: (message: string, level?: NotificationLevel) => void;
  window: {
    /**
     * Set the document title. The host stores the requested title per
     * plugin internally so two plugins fighting over it produce a
     * deterministic outcome — the latest setter wins.
     */
    setTitle: (text: string) => void;
    /**
     * Prefix the current title with `[n]` when `n > 0`. Pass 0 / undefined
     * to clear. Useful for "(3) Lyra" notification counts.
     */
    setBadge: (n?: number) => void;
  };
  plugins: {
    /** Snapshot of currently-loaded plugins. */
    list: () => LoadedPlugin[];
    /** Fires every time a plugin is loaded (including subsequent loads). */
    onLoad: (fn: (spec: PluginSpec) => void) => Disposable;
    /** Fires every time a plugin is unloaded. */
    onUnload: (fn: (name: string) => void) => Disposable;
    /**
     * Load a plugin spec at runtime — useful for hot-loading or
     * conditional features. Goes through the same setup pipeline as
     * built-in / sideload loads, including disposable tracking + error
     * isolation.
     */
    load: (spec: PluginSpec) => Promise<void>;
    /**
     * Unload a previously-loaded plugin. Disposes every register* return
     * value collected during setup, removes the plugin from the loaded
     * map, and fires onUnload listeners. No-op if the plugin isn't
     * currently loaded.
     */
    unload: (name: string) => void;
    /**
     * Convenience: `unload(name)` then re-load the same spec. Returns the
     * load promise so callers can `await` for setup completion.
     */
    reload: (name: string) => Promise<void>;
  };
  /**
   * Structured logger. Calls always forward to `console.{method}` with a
   * `[plugin:<name>]` prefix; in addition, every registered subscriber
   * receives a `LogEvent` for its own ingestion (telemetry, devtools, etc).
   */
  log: {
    debug: (...args: unknown[]) => void;
    info: (...args: unknown[]) => void;
    warn: (...args: unknown[]) => void;
    error: (...args: unknown[]) => void;
    /** Listen to every log event emitted via `host.log.*`. */
    subscribe: (fn: LogSubscriber) => Disposable;
  };
}

export interface PluginContext {
  host: Host;
}
