// Host bridges plugin code to the registry + shared services. Each
// plugin gets a Host bound to its name so registrations, errors, and
// conflict warnings can be attributed back when it unloads.

import type { ConfigValue } from "./config";
import type {
  BeforeUnloadHandler,
  CommandSpec,
  ContentBlockRenderer,
  StreamEventHandler,
  CustomEventHandler,
  Disposable,
  Host,
  HostCapability,
  LayoutSlotSpec,
  LoadedPlugin,
  LogSubscriber,
  PluginSpec,
  ReadyHandler,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
} from "./types";
import type { ContentBlockKind } from "@/plugins/sdk/types/agentView";
import { api } from "@/lib/data/http";
import { addLocaleBundle } from "@/lib/i18n";
import { useWorkspaceNavigationStore } from "@/state/workspaceNavigationStore";
import { startTask } from "@/state/tasksStore";
import { getConfig, hasConfig, setConfig, useConfigStore } from "./config";
import { restrictHost } from "./capabilityGate";
import { safeCall } from "./errors";
import { createContribute, assertNamespaced } from "./hostContributions";
import { emitPluginLog, logToConsole } from "./hostLog";
import { getPluginRuntime } from "./hostRuntime";
import { dispatchToast } from "./hostToast";
import {
  BEFORE_UNLOAD_HANDLER,
  COMMAND,
  CONTENT_BLOCK,
  STREAM_EVENT_HANDLER,
  CUSTOM_EVENT_HANDLER,
  LAYOUT_SLOT,
  LOG_SUBSCRIBER,
  PLUGIN_LOAD_LISTENER,
  PLUGIN_UNLOAD_LISTENER,
  READY_HANDLER,
  RPC_AFTER_RESPONSE,
  RPC_BEFORE_REQUEST,
  WORKSPACE_VIEW,
} from "./kernelPoints";
import { useNotificationStore } from "./notifications";
import { pluginOrigin, setPluginOrigin } from "./pluginOrigin";
import { usePluginStore } from "./registry";
import { executeCommand } from "./selectors/commands";
import { lookupExtensionByKey } from "./selectors/extensions";
import { getOrCreateSlice } from "./stateSlice";
import { createStorage } from "./storage";

export { setPluginRuntime } from "./hostRuntime";
export { PLUGIN_TOAST_EVENT } from "./hostToast";
export type { PluginToastDetail } from "./hostToast";

/**
 * Build a Host bound to a specific plugin. `contribute` and the facades return
 * Disposables; `setup`'s caller (loadPlugin) collects them so it can dispose on
 * failure or on unload.
 */
export function createHost(
  pluginName: string,
  sink: Disposable[],
  capabilities?: HostCapability[],
): Host {
  const track = (d: Disposable): Disposable => {
    sink.push(d);
    return d;
  };

  const store = () => usePluginStore.getState();

  const contribute = createContribute(pluginName, capabilities, track);

  const full = {
    message: {
      registerContentBlock<K extends ContentBlockKind>(
        kind: K,
        renderer: ContentBlockRenderer<K>,
      ): Disposable {
        assertNamespaced(pluginName, "content-block kind", kind);
        // The substrate holds renderers as the union root type; the public
        // method is typed per-kind for plugin author ergonomics.
        return contribute(CONTENT_BLOCK, renderer as ContentBlockRenderer<ContentBlockKind>, {
          key: kind,
        });
      },
    },

    events: {
      // Both fan out to every matching handler; `multi` keying lets a plugin
      // register more than once for the same name/type (each contribution
      // coexists). The reducer chains StateUpdate returns across them.
      onCustom: <T = unknown>(name: string, handler: CustomEventHandler<T>): Disposable => {
        assertNamespaced(pluginName, "custom event name", name);
        return contribute(CUSTOM_EVENT_HANDLER, {
          name,
          handler: handler as CustomEventHandler<unknown>,
        });
      },
      onStream: (eventType: string, handler: StreamEventHandler): Disposable =>
        contribute(STREAM_EVENT_HANDLER, { eventType, handler }),
    },

    layout: {
      // Stable id `${slot}#${spec.id}` so re-registering the same slot entry
      // overwrites rather than stacking a duplicate (a dup would render the
      // same component twice under one React key in <Slot>).
      register: (slot: string, spec: LayoutSlotSpec): Disposable =>
        contribute(LAYOUT_SLOT, { slot, spec }, { id: `${slot}#${spec.id}` }),
    },

    workspace: {
      openView(id: string): void {
        const view = lookupExtensionByKey(WORKSPACE_VIEW, id);
        if (!view) {
          console.warn(`[plugin] workspace.openView("${id}"): no view registered`);
          return;
        }
        useWorkspaceNavigationStore
          .getState()
          .openMainView({ id, title: view.title, icon: view.icon });
      },
      closeView(id: string): void {
        useWorkspaceNavigationStore.getState().closeMainView(id);
      },
    },

    commands: {
      register: (spec: CommandSpec): Disposable => contribute(COMMAND, spec),
      // Run another plugin's command by id (VSCode-style executeCommand) —
      // activates it first if it's a lazy/declared command. Commands are the
      // lightweight cross-plugin RPC.
      execute: (id: string, ...args: unknown[]): Promise<void> => executeCommand(id, ...args),
    },

    extensions: { contribute },

    state: {
      slice<T>(name: string, initial: T) {
        return getOrCreateSlice<T>(name, initial);
      },
    },

    config: {
      get: <T = ConfigValue>(key: string, defaultValue?: T) => getConfig<T>(key, defaultValue),
      set: (key: string, value: ConfigValue) => setConfig(key, value),
      has: (key: string) => hasConfig(key),
      // track() — like every other registering facade — so unload disposes
      // the subscription. Untracked, each plugin reload (Plugins pane) would
      // leak the previous setup's subscriber, which keeps firing forever.
      onChange: (key, fn) => track(useConfigStore.getState().subscribe(key, fn)),
    },

    lifecycle: {
      onReady(fn: ReadyHandler): Disposable {
        // If we're already past the ready point, fire on the next microtask
        // — never synchronously inside register (which would surprise
        // setup-time callers).
        if (usePluginStore.getState().appReady) {
          queueMicrotask(() => safeCall(fn, `[plugin] ${pluginName} onReady threw:`));
          return track({
            dispose: () => {
              /* no-op: already fired */
            },
          });
        }
        return contribute(READY_HANDLER, fn);
      },
      onBeforeUnload: (fn: BeforeUnloadHandler): Disposable =>
        contribute(BEFORE_UNLOAD_HANDLER, fn),
    },

    storage: createStorage(pluginName),

    rpc: {
      get<T>(path: string, params?: Record<string, unknown>): Promise<T> {
        // ky's baseUrl resolution prefers paths without a leading slash.
        const p = path.startsWith("/") ? path.slice(1) : path;
        const opts = params
          ? { searchParams: params as Record<string, string | number | boolean> }
          : undefined;
        return api.get(p, opts).json<T>();
      },
      post<T>(path: string, body?: unknown): Promise<T> {
        const p = path.startsWith("/") ? path.slice(1) : path;
        return api.post(p, body !== undefined ? { json: body } : undefined).json<T>();
      },
      beforeRequest: (hook: RpcBeforeRequestHook): Disposable =>
        contribute(RPC_BEFORE_REQUEST, hook),
      afterResponse: (hook: RpcAfterResponseHook): Disposable =>
        contribute(RPC_AFTER_RESPONSE, hook),
    },

    notify(message: string, level: "info" | "warn" | "error" = "info"): void {
      logToConsole(pluginName, level, [message]);
      // Push to the persistent feed BEFORE dispatching the visual toast so
      // any listener that reacts to the toast can already cross-reference
      // an entry id (e.g. "Open in notifications panel").
      useNotificationStore.getState().push({ plugin: pluginName, level, message });
      dispatchToast(message, level);
    },

    window: {
      setTitle(text: string): void {
        store().setWindowTitle(text);
      },
      setBadge(n?: number): void {
        store().setWindowBadge(Math.max(0, n ?? 0));
      },
    },

    plugins: {
      list(): LoadedPlugin[] {
        return Array.from(usePluginStore.getState().loaded.values());
      },
      onLoad: (fn: (spec: PluginSpec) => void): Disposable => contribute(PLUGIN_LOAD_LISTENER, fn),
      onUnload: (fn: (name: string) => void): Disposable => contribute(PLUGIN_UNLOAD_LISTENER, fn),
      // Dynamic load / unload — the SDK indirection lets plugins ship admin
      // UI without importing definePlugin directly. The actual impl lives
      // in definePlugin.ts and is installed via setPluginRuntime() so we
      // don't introduce a circular import.
      load(spec: PluginSpec): Promise<void> {
        // Propagate the CALLER's origin: an unrecorded name defaults to
        // "builtin" (full trust), so a sideload that declared only
        // ["plugins"] could otherwise mint a fully-privileged child and
        // bypass deny-by-default entirely. definePluginPack already pins
        // child origins this way — same invariant for the raw facade.
        if (pluginOrigin(pluginName) === "sideload") {
          setPluginOrigin(spec.name, "sideload");
        }
        return getPluginRuntime().load(spec);
      },
      unload(name: string): void {
        getPluginRuntime().unload(name);
      },
      reload(name: string): Promise<void> {
        return getPluginRuntime().reload(name);
      },
    },

    log: {
      debug: (...args) => emitPluginLog(pluginName, "debug", args),
      info: (...args) => emitPluginLog(pluginName, "info", args),
      warn: (...args) => emitPluginLog(pluginName, "warn", args),
      error: (...args) => emitPluginLog(pluginName, "error", args),
      subscribe: (fn: LogSubscriber): Disposable => contribute(LOG_SUBSCRIBER, fn),
    },

    i18n: {
      addBundle(locale: string, dict: Record<string, string>): Disposable {
        addLocaleBundle(locale, dict);
        // i18next has no per-key removal; leaving bundles registered is
        // safe because t() only matters while plugin UI is mounted, and a
        // same-name reload overwrites cleanly.
        return track({ dispose: () => {} });
      },
    },

    tasks: {
      start(opts) {
        return startTask(pluginName, opts);
      },
    },
  } satisfies Host;

  return capabilities ? restrictHost(full, pluginName, capabilities) : full;
}
