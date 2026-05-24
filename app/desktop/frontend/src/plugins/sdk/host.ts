// Host bridges plugin code to the registry + shared services. Each
// plugin gets a Host bound to its name so registrations, errors, and
// conflict warnings can be attributed back when it unloads.

import { api } from "@/lib/http";
import type { ContentBlockKind } from "@/protocol/agui/viewState";
import { useSessionStore } from "@/state/sessionStore";
import { getConfig, hasConfig, setConfig, useConfigStore, type ConfigValue } from "./config";
import { safeCall } from "./errors";
import { useNotificationStore } from "./notifications";
import { usePluginStore } from "./registry";
import { getOrCreateSlice } from "./stateSlice";
import { createStorage } from "./storage";
import type {
  AgentSourceSpec,
  CommandSpec,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
  ContentBlockRenderer,
  CoreEventHandler,
  CustomEventHandler,
  DataProviderSpec,
  Disposable,
  Host,
  HostCapability,
  LayoutSlotSpec,
  LoadedPlugin,
  MessageRoleSpec,
  PluginErrorFallbackSpec,
  PluginSpec,
  RouteSpec,
  WorkspaceViewSpec,
  SettingsPaneSpec,
  BeforeUnloadHandler,
  LogLevel,
  LogSubscriber,
  ReadyHandler,
  ShortcutSpec,
  SidebarRailItemSpec,
  SidebarSectionSpec,
  SlashCommandSpec,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
  ThemeAccentSpec,
  ThemeSpec,
  ToolActionSpec,
  ToolPreviewComponent,
} from "./types";

/**
 * Build a Host bound to a specific plugin. `register*` returns Disposables;
 * `setup`'s caller (loadPlugin) collects them so it can dispose on failure
 * or on unload.
 */
// Single monotonic id minter used by every composite-key register call
// (onCore, rpc.before/afterResponse, log.subscribe, lifecycle.onReady /
// onBeforeUnload, plugins.onLoad / onUnload, etc.). Uniqueness only needs
// to hold within one plugin's composite map; a global counter is simpler
// than per-scope ones and IDs aren't exposed to user code.
let nextCompositeKeyId = 0;
const mintId = (prefix: string) => `${prefix}#${++nextCompositeKeyId}`;

// Plugin runtime injection point — definePlugin.ts installs the real
// implementation at module load time. The Host's `plugins.{load,unload,
// reload}` methods dispatch through this seam so we don't introduce a
// circular import (host.ts → definePlugin.ts → host.ts).
//
// We hold the runtime inside a const wrapper so the binding itself is
// initialised at module-evaluation time. A bare `let pluginRuntime`
// would hit a TDZ if any code path called the setter before the let was
// reached (rare, but observable under Vitest's module loader).
type PluginRuntime = {
  load(spec: PluginSpec): Promise<void>;
  unload(name: string): void;
  reload(name: string): Promise<void>;
};
const runtimeSlot: { current: PluginRuntime | null } = { current: null };

export function setPluginRuntime(rt: PluginRuntime): void {
  runtimeSlot.current = rt;
}

function getRuntime(): PluginRuntime {
  if (!runtimeSlot.current) {
    throw new Error("plugin runtime not wired; call setPluginRuntime() before host is used");
  }
  return runtimeSlot.current;
}

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

  const full = {
    tool: {
      registerPreview(fn: string, component: ToolPreviewComponent): Disposable {
        store().addToolPreview(pluginName, fn, component);
        return track({ dispose: () => store().removeToolPreview(pluginName, fn) });
      },
      registerAction(spec: ToolActionSpec): Disposable {
        store().addToolAction(pluginName, spec);
        return track({ dispose: () => store().removeToolAction(pluginName, spec.id) });
      },
      registerIcon(fn: string, icon: string): Disposable {
        store().addToolIcon(pluginName, fn, icon);
        return track({ dispose: () => store().removeToolIcon(pluginName, fn) });
      },
    },

    message: {
      registerContentBlock<K extends ContentBlockKind>(
        kind: K,
        renderer: ContentBlockRenderer<K>,
      ): Disposable {
        // The store holds renderers as the union root type; the public method
        // is typed per-kind for plugin author ergonomics.
        store().addContentBlock(
          pluginName,
          kind,
          renderer as ContentBlockRenderer<ContentBlockKind>,
        );
        return track({ dispose: () => store().removeContentBlock(pluginName, kind) });
      },
      registerRole(spec: MessageRoleSpec): Disposable {
        store().addMessageRole(pluginName, spec);
        return track({ dispose: () => store().removeMessageRole(pluginName, spec.id) });
      },
    },

    agui: {
      on<T = unknown>(name: string, handler: CustomEventHandler<T>): Disposable {
        store().addCustomEventHandler(pluginName, name, handler as CustomEventHandler<unknown>);
        return track({ dispose: () => store().removeCustomEventHandler(pluginName, name) });
      },
      onCore(eventType: string, handler: CoreEventHandler): Disposable {
        // Composite-key registration in the registry allows multiple handlers
        // per type; we mint a stable id per call so the same plugin can register
        // more than once for the same type (each call gets its own disposable).
        const id = mintId(eventType);
        store().addCoreEventHandler(pluginName, eventType, id, handler);
        return track({ dispose: () => store().removeCoreEventHandler(pluginName, eventType, id) });
      },
    },

    layout: {
      register(slot: string, spec: LayoutSlotSpec): Disposable {
        store().addLayoutSlot(pluginName, slot, spec);
        return track({ dispose: () => store().removeLayoutSlot(pluginName, slot, spec.id) });
      },
    },

    workspace: {
      registerView(spec: WorkspaceViewSpec): Disposable {
        store().addWorkspaceView(pluginName, spec);
        return track({ dispose: () => store().removeWorkspaceView(pluginName, spec.id) });
      },
      openView(id: string): void {
        const view = usePluginStore.getState().workspaceViews.get(id)?.value;
        if (!view) {
           
          console.warn(`[plugin] workspace.openView("${id}"): no view registered`);
          return;
        }
        useSessionStore.getState().openMainView({ id, title: view.title, icon: view.icon });
      },
      closeView(id: string): void {
        useSessionStore.getState().closeMainView(id);
      },
    },

    theme: {
      registerTheme(spec: ThemeSpec): Disposable {
        store().addTheme(pluginName, spec);
        return track({ dispose: () => store().removeTheme(pluginName, spec.id) });
      },
      registerAccent(spec: ThemeAccentSpec): Disposable {
        store().addAccent(pluginName, spec);
        return track({ dispose: () => store().removeAccent(pluginName, spec.id) });
      },
    },

    router: {
      register(spec: RouteSpec): Disposable {
        store().addRoute(pluginName, spec);
        return track({ dispose: () => store().removeRoute(pluginName, spec.id) });
      },
    },

    composer: {
      registerCommand(cmd: string, spec: SlashCommandSpec): Disposable {
        // Normalize so callers can omit the leading slash.
        const key = cmd.startsWith("/") ? cmd : `/${cmd}`;
        store().addSlashCommand(pluginName, key, spec);
        return track({ dispose: () => store().removeSlashCommand(pluginName, key) });
      },
      registerStatus(spec: ComposerStatusSpec): Disposable {
        store().addComposerStatus(pluginName, spec);
        return track({ dispose: () => store().removeComposerStatus(pluginName, spec.id) });
      },
      registerMode(spec: ComposerModeSpec): Disposable {
        store().addComposerMode(pluginName, spec);
        return track({ dispose: () => store().removeComposerMode(pluginName, spec.id) });
      },
      registerPlaceholder(spec: ComposerPlaceholderSpec): Disposable {
        store().addComposerPlaceholder(pluginName, spec);
        return track({ dispose: () => store().removeComposerPlaceholder(pluginName, spec.id) });
      },
      registerAttachmentSource(spec: ComposerAttachmentSourceSpec): Disposable {
        store().addComposerAttachmentSource(pluginName, spec);
        return track({
          dispose: () => store().removeComposerAttachmentSource(pluginName, spec.id),
        });
      },
      registerKeyBinding(spec: ComposerKeyBindingSpec): Disposable {
        store().addComposerKeyBinding(pluginName, spec);
        return track({ dispose: () => store().removeComposerKeyBinding(pluginName, spec.key) });
      },
    },

    sidebar: {
      registerSection(spec: SidebarSectionSpec): Disposable {
        store().addSidebarSection(pluginName, spec);
        return track({ dispose: () => store().removeSidebarSection(pluginName, spec.id) });
      },
      registerRailItem(spec: SidebarRailItemSpec): Disposable {
        store().addSidebarRailItem(pluginName, spec);
        return track({ dispose: () => store().removeSidebarRailItem(pluginName, spec.id) });
      },
    },

    shortcuts: {
      register(spec: ShortcutSpec): Disposable {
        store().addShortcut(pluginName, spec);
        return track({ dispose: () => store().removeShortcut(pluginName, spec.key) });
      },
    },

    agent: {
      registerSource(spec: AgentSourceSpec): Disposable {
        store().addAgentSource(pluginName, spec);
        return track({ dispose: () => store().removeAgentSource(pluginName, spec.id) });
      },
    },

    data: {
      registerProvider<T = unknown>(spec: DataProviderSpec<T>): Disposable {
        // Cast through unknown — the registry erases T, callers cast on
        // the way out via `lookupDataProvider<T>()`.
        store().addDataProvider(pluginName, spec as DataProviderSpec);
        return track({ dispose: () => store().removeDataProvider(pluginName, spec.key) });
      },
    },

    commands: {
      register(spec: CommandSpec): Disposable {
        store().addCommand(pluginName, spec);
        return track({ dispose: () => store().removeCommand(pluginName, spec.id) });
      },
    },

    state: {
      slice<T>(name: string, initial: T) {
        return getOrCreateSlice<T>(name, initial);
      },
    },

    config: {
      get: <T = ConfigValue>(key: string, defaultValue?: T) => getConfig<T>(key, defaultValue),
      set: (key: string, value: ConfigValue) => setConfig(key, value),
      has: (key: string) => hasConfig(key),
      onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
    },

    lifecycle: {
      onReady(fn: ReadyHandler): Disposable {
        // If we're already past the ready point, fire on the next microtask
        // — never synchronously inside register (which would surprise
        // setup-time callers).
        if (usePluginStore.getState().appReady) {
          queueMicrotask(() =>
            safeCall(fn, `[plugin] ${pluginName} onReady threw:`),
          );
          return track({
            dispose: () => {
              /* no-op: already fired */
            },
          });
        }
        const id = mintId("ready");
        store().addReadyHandler(pluginName, id, fn);
        return track({ dispose: () => store().removeReadyHandler(pluginName, id) });
      },
      onBeforeUnload(fn: BeforeUnloadHandler): Disposable {
        const id = mintId("before-unload");
        store().addBeforeUnloadHandler(pluginName, id, fn);
        return track({ dispose: () => store().removeBeforeUnloadHandler(pluginName, id) });
      },
    },

    settings: {
      registerPane(spec: SettingsPaneSpec): Disposable {
        store().addSettingsPane(pluginName, spec);
        return track({ dispose: () => store().removeSettingsPane(pluginName, spec.id) });
      },
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
      beforeRequest(hook: RpcBeforeRequestHook): Disposable {
        const id = mintId("before");
        store().addRpcBeforeRequest(pluginName, id, hook);
        return track({ dispose: () => store().removeRpcBeforeRequest(pluginName, id) });
      },
      afterResponse(hook: RpcAfterResponseHook): Disposable {
        const id = mintId("after");
        store().addRpcAfterResponse(pluginName, id, hook);
        return track({ dispose: () => store().removeRpcAfterResponse(pluginName, id) });
      },
    },

    notify(message: string, level: "info" | "warn" | "error" = "info"): void {
      const tag = `[plugin:${pluginName}]`;
      if (level === "error") console.error(tag, message);
      else if (level === "warn") console.warn(tag, message);
      else console.info(tag, message);
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
      onLoad(fn: (spec: PluginSpec) => void): Disposable {
        const id = mintId("load");
        store().addPluginLoadListener(pluginName, id, fn);
        return track({ dispose: () => store().removePluginLoadListener(pluginName, id) });
      },
      onUnload(fn: (name: string) => void): Disposable {
        const id = mintId("unload");
        store().addPluginUnloadListener(pluginName, id, fn);
        return track({ dispose: () => store().removePluginUnloadListener(pluginName, id) });
      },
      // Dynamic load / unload — the SDK indirection lets plugins ship admin
      // UI without importing definePlugin directly. The actual impl lives
      // in definePlugin.ts and is installed via setPluginRuntime() so we
      // don't introduce a circular import.
      load(spec: PluginSpec): Promise<void> {
        return getRuntime().load(spec);
      },
      unload(name: string): void {
        getRuntime().unload(name);
      },
      reload(name: string): Promise<void> {
        return getRuntime().reload(name);
      },
      registerErrorFallback(spec: PluginErrorFallbackSpec): Disposable {
        store().addPluginErrorFallback(pluginName, spec);
        return track({ dispose: () => store().removePluginErrorFallback(pluginName, spec.id) });
      },
    },

    log: {
      debug: (...args) => emitLog(pluginName, "debug", args),
      info: (...args) => emitLog(pluginName, "info", args),
      warn: (...args) => emitLog(pluginName, "warn", args),
      error: (...args) => emitLog(pluginName, "error", args),
      subscribe(fn: LogSubscriber): Disposable {
        const id = mintId("log");
        store().addLogSubscriber(pluginName, id, fn);
        return track({ dispose: () => store().removeLogSubscriber(pluginName, id) });
      },
    },
  } satisfies Host;

  return capabilities ? restrictHost(full, pluginName, capabilities) : full;
}

/**
 * Wrap a host such that any access to a namespace the plugin didn't
 * declare in `capabilities` throws with a clear error message. The
 * allowed namespaces are returned as-is.
 */
function restrictHost(host: Host, pluginName: string, allowed: HostCapability[]): Host {
  const allowedSet = new Set(allowed);
  const denied: Record<string, unknown> = {};
  for (const key of Object.keys(host) as Array<keyof Host>) {
    if (allowedSet.has(key as HostCapability)) {
      denied[key as string] = host[key];
    } else {
      denied[key as string] = createDenyProxy(pluginName, key as string);
    }
  }
  return denied as Host;
}

function createDenyProxy(pluginName: string, namespace: string): unknown {
  const explain = (prop: string) =>
    new Error(
      `[plugin] ${pluginName}: host.${namespace}${prop ? `.${prop}` : ""} ` +
        `is not in this plugin's declared capabilities (add "${namespace}" to spec.capabilities)`,
    );
  // Trap both function-style (host.notify(...)) and property access.
  const denied = function denied() {
    throw explain("");
  };
  return new Proxy(denied, {
    get(_, prop) {
      throw explain(String(prop));
    },
    apply() {
      throw explain("");
    },
  });
}

// Module-scoped log emission: forwards to console with a `[plugin:<name>]`
// prefix (preserving the existing log shape) and fans the event out to
// every registered subscriber. Subscriber failures are isolated.
function emitLog(plugin: string, level: LogLevel, args: unknown[]): void {
  const tag = `[plugin:${plugin}]`;
  const consoleFn =
    level === "error"
      ? console.error
      : level === "warn"
        ? console.warn
        : level === "info"
          ? console.info
          : console.log;
   
  consoleFn(tag, ...args);

  const event = { plugin, level, args, timestamp: Date.now() };
  const subs = usePluginStore.getState().logSubscribers;
  for (const o of subs.values()) {
    safeCall(() => o.value(event), "[plugin] log subscriber threw:");
  }
}

// ---- toast plumbing -------------------------------------------------------
//
// A self-mounting listener (see PluginToaster.tsx) picks up these events and
// renders an animated toast. Keeping the dispatcher event-based means the
// SDK doesn't import React for its notification path.

type ToastLevel = "info" | "warn" | "error";
export const PLUGIN_TOAST_EVENT = "lyra:plugin-toast";
export type PluginToastDetail = { message: string; level: ToastLevel };

function dispatchToast(message: string, level: ToastLevel): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<PluginToastDetail>(PLUGIN_TOAST_EVENT, { detail: { message, level } }),
  );
}
