// Host implementation — the bridge between plugin code and the registry +
// shared services (HTTP, notifications).
//
// Each plugin receives a Host instance bound to its own name. The bound name
// lets the registry track who registered what so we can clean up cleanly on
// unload + attribute errors and conflict warnings correctly.

import { api } from "@/lib/http";
import type { ContentBlockKind } from "@/protocol/agui/viewState";
import {
  getConfig,
  hasConfig,
  setConfig,
  useConfigStore,
  type ConfigValue,
} from "./config";
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
  InspectorTabSpec,
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
// onBeforeUnload, plugins.onLoad / onUnload, etc.). The actual uniqueness
// only matters within one plugin's composite map, so a global counter is
// overkill — but it's simpler than per-scope ones, and the IDs aren't
// exposed to user code.
let idCounter = 0;
const mintId = (prefix: string) => `${prefix}#${++idCounter}`;

export function createHost(pluginName: string, sink: Disposable[]): Host {
  const track = (d: Disposable): Disposable => {
    sink.push(d);
    return d;
  };

  const store = () => usePluginStore.getState();

  return {
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
        store().addContentBlock(pluginName, kind, renderer as ContentBlockRenderer<ContentBlockKind>);
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
        // Resolve spec metadata. Inspector tabs are second-class workspace
        // views: they get auto-promoted when the user asks to open them in
        // the main pane, without anyone calling registerView for them.
        const fromWorkspace = usePluginStore.getState().workspaceViews.get(id)?.value;
        const fromInspector = usePluginStore.getState().inspectorTabs.get(id)?.value;
        if (!fromWorkspace && !fromInspector) {
          // eslint-disable-next-line no-console
          console.warn(`[plugin] workspace.openView("${id}"): no view registered`);
          return;
        }
        const tab = fromWorkspace
          ? { id, title: fromWorkspace.title, icon: fromWorkspace.icon }
          : { id, title: fromInspector!.label, icon: fromInspector!.icon };
        // Lazy import to avoid a circular dependency between sdk → state.
        import("@/state/uiStore").then(({ useUIStore }) => {
          useUIStore.getState().openMainView(tab);
        });
      },
      closeView(id: string): void {
        import("@/state/uiStore").then(({ useUIStore }) => {
          useUIStore.getState().closeMainView(id);
        });
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
        return track({ dispose: () => store().removeComposerAttachmentSource(pluginName, spec.id) });
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
      get: <T = ConfigValue>(key: string, defaultValue?: T) =>
        getConfig<T>(key, defaultValue),
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
          queueMicrotask(() => {
            try { fn(); } catch (err) {
              // eslint-disable-next-line no-console
              console.error(`[plugin] ${pluginName} onReady threw:`, err);
            }
          });
          return track({ dispose: () => { /* no-op: already fired */ } });
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

    inspector: {
      registerTab(spec: InspectorTabSpec): Disposable {
        store().addInspectorTab(pluginName, spec);
        return track({ dispose: () => store().removeInspectorTab(pluginName, spec.id) });
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
      registerErrorFallback(spec: PluginErrorFallbackSpec): Disposable {
        store().addPluginErrorFallback(pluginName, spec);
        return track({ dispose: () => store().removePluginErrorFallback(pluginName, spec.id) });
      },
    },

    log: {
      debug: (...args) => emitLog(pluginName, "debug", args),
      info:  (...args) => emitLog(pluginName, "info",  args),
      warn:  (...args) => emitLog(pluginName, "warn",  args),
      error: (...args) => emitLog(pluginName, "error", args),
      subscribe(fn: LogSubscriber): Disposable {
        const id = mintId("log");
        store().addLogSubscriber(pluginName, id, fn);
        return track({ dispose: () => store().removeLogSubscriber(pluginName, id) });
      },
    },
  };
}

// Module-scoped log emission: forwards to console with a `[plugin:<name>]`
// prefix (preserving the existing log shape) and fans the event out to
// every registered subscriber. Subscriber failures are isolated.
function emitLog(plugin: string, level: LogLevel, args: unknown[]): void {
  const tag = `[plugin:${plugin}]`;
  const consoleFn =
    level === "error" ? console.error :
    level === "warn"  ? console.warn  :
    level === "info"  ? console.info  :
    console.log;
  // eslint-disable-next-line no-console
  consoleFn(tag, ...args);

  const event = { plugin, level, args, timestamp: Date.now() };
  const subs = usePluginStore.getState().logSubscribers;
  for (const o of subs.values()) {
    try { o.value(event); } catch (err) {
      // Don't let a subscriber crash the logger — but DO surface the error
      // so a malformed subscriber isn't invisible.
      // eslint-disable-next-line no-console
      console.error("[plugin] log subscriber threw:", err);
    }
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
