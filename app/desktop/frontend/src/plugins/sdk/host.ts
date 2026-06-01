// Host bridges plugin code to the registry + shared services. Each
// plugin gets a Host bound to its name so registrations, errors, and
// conflict warnings can be attributed back when it unloads.

import type { ConfigValue } from "./config";
import type {
  AgentSourceSpec,
  BeforeUnloadHandler,
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
  ExtensionContributionOptions,
  ExtensionPoint,
  Host,
  HostCapability,
  LayoutSlotSpec,
  LoadedPlugin,
  LogLevel,
  LogSubscriber,
  MessageRoleSpec,
  PluginErrorFallbackSpec,
  PluginSpec,
  ReadyHandler,
  RouteSpec,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
  SettingsPaneSpec,
  ShortcutSpec,
  SidebarRailItemSpec,
  SidebarSectionSpec,
  SlashCommandSpec,
  ThemeAccentSpec,
  ThemeSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  WorkspaceViewSpec,
} from "./types";
import type { ContentBlockKind } from "@/protocol/agui/viewState";
import { api } from "@/lib/data/http";
import { addLocaleBundle } from "@/lib/i18n";
import { useSessionStore } from "@/state/sessionStore";
import { startTask } from "@/state/tasksStore";
import { getConfig, hasConfig, setConfig, useConfigStore } from "./config";
import { safeCall } from "./errors";
import {
  ACCENT,
  AGENT_SOURCE,
  COMPOSER_ATTACHMENT_SOURCE,
  COMPOSER_MODE,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
  CONTENT_BLOCK,
  DATA_PROVIDER,
  ERROR_FALLBACK,
  LOCALE,
  MESSAGE_ROLE,
  ROUTE,
  SIDEBAR_RAIL_ITEM,
  SIDEBAR_SECTION,
  THEME,
  TOOL_ACTION,
  TOOL_ICON,
  TOOL_PREVIEW,
} from "./kernelPoints";
import { useNotificationStore } from "./notifications";
import { usePluginStore } from "./registry";
import { getOrCreateSlice } from "./stateSlice";
import { createStorage } from "./storage";

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
interface PluginRuntime {
  load: (spec: PluginSpec) => Promise<void>;
  unload: (name: string) => void;
  reload: (name: string) => Promise<void>;
}
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

  // Shared write path for the open-extension-point substrate. Both the
  // public `host.extensions.contribute` and every migrated kernel facade
  // (`host.theme.registerTheme`, …) route through here, so built-in and
  // third-party contributions hit the exact same code. Keying policy lives
  // on the point: `single` dedupes by `keyOf(item)` (warns on cross-plugin
  // override); `multi` mints a per-(plugin,id) key so contributions coexist.
  const contribute = <T>(
    point: ExtensionPoint<T>,
    item: T,
    opts?: ExtensionContributionOptions,
  ): Disposable => {
    const keyOf = point.keyOf ?? ((i: T) => (i as unknown as { id: string }).id);
    let outerKey: string;
    let conflictKey: string;
    if (point.keying === "single") {
      const base = opts?.key ?? keyOf(item);
      const k = point.normalizeKey ? point.normalizeKey(base) : base;
      outerKey = `${point.id}#${k}`;
      conflictKey = k;
    } else {
      const id = opts?.id ?? mintId(point.id);
      outerKey = `${point.id}#${pluginName}|${id}`;
      conflictKey = id;
    }
    store().addContribution(
      pluginName,
      point.id,
      outerKey,
      { point: point.id, order: opts?.order, item },
      conflictKey,
    );
    return track({ dispose: () => store().removeContribution(pluginName, outerKey) });
  };

  const full = {
    tool: {
      registerPreview: (fn: string, component: ToolPreviewComponent): Disposable =>
        contribute(TOOL_PREVIEW, component, { key: fn }),
      registerAction: (spec: ToolActionSpec): Disposable => contribute(TOOL_ACTION, spec),
      registerIcon: (fn: string, icon: string): Disposable =>
        contribute(TOOL_ICON, icon, { key: fn }),
    },

    message: {
      registerContentBlock<K extends ContentBlockKind>(
        kind: K,
        renderer: ContentBlockRenderer<K>,
      ): Disposable {
        // The substrate holds renderers as the union root type; the public
        // method is typed per-kind for plugin author ergonomics.
        return contribute(CONTENT_BLOCK, renderer as ContentBlockRenderer<ContentBlockKind>, {
          key: kind,
        });
      },
      registerRole: (spec: MessageRoleSpec): Disposable => contribute(MESSAGE_ROLE, spec),
    },

    agui: {
      on<T = unknown>(name: string, handler: CustomEventHandler<T>): Disposable {
        // Composite-key registration so multiple plugins (or the same
        // plugin twice) can handle the same custom event name. The
        // reducer fans the event out through every match.
        const id = mintId(`custom:${name}`);
        store().addCustomEventHandler(pluginName, name, id, handler as CustomEventHandler<unknown>);
        return track({ dispose: () => store().removeCustomEventHandler(pluginName, id) });
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
      registerTheme: (spec: ThemeSpec): Disposable => contribute(THEME, spec),
      registerAccent: (spec: ThemeAccentSpec): Disposable => contribute(ACCENT, spec),
    },

    router: {
      register: (spec: RouteSpec): Disposable => contribute(ROUTE, spec),
    },

    composer: {
      registerCommand(cmd: string, spec: SlashCommandSpec): Disposable {
        // Normalize so callers can omit the leading slash.
        const key = cmd.startsWith("/") ? cmd : `/${cmd}`;
        store().addSlashCommand(pluginName, key, spec);
        return track({ dispose: () => store().removeSlashCommand(pluginName, key) });
      },
      registerStatus: (spec: ComposerStatusSpec): Disposable => contribute(COMPOSER_STATUS, spec),
      registerMode: (spec: ComposerModeSpec): Disposable => contribute(COMPOSER_MODE, spec),
      registerPlaceholder: (spec: ComposerPlaceholderSpec): Disposable =>
        contribute(COMPOSER_PLACEHOLDER, spec),
      registerAttachmentSource: (spec: ComposerAttachmentSourceSpec): Disposable =>
        contribute(COMPOSER_ATTACHMENT_SOURCE, spec),
      registerKeyBinding(spec: ComposerKeyBindingSpec): Disposable {
        store().addComposerKeyBinding(pluginName, spec);
        return track({ dispose: () => store().removeComposerKeyBinding(pluginName, spec.key) });
      },
    },

    sidebar: {
      registerSection: (spec: SidebarSectionSpec): Disposable => contribute(SIDEBAR_SECTION, spec),
      registerRailItem: (spec: SidebarRailItemSpec): Disposable =>
        contribute(SIDEBAR_RAIL_ITEM, spec),
    },

    shortcuts: {
      register(spec: ShortcutSpec): Disposable {
        store().addShortcut(pluginName, spec);
        return track({ dispose: () => store().removeShortcut(pluginName, spec.key) });
      },
    },

    agent: {
      registerSource: (spec: AgentSourceSpec): Disposable => contribute(AGENT_SOURCE, spec),
    },

    data: {
      registerProvider<T = unknown>(spec: DataProviderSpec<T>): Disposable {
        // Cast through unknown — the registry erases T, callers cast on
        // the way out via `lookupDataProvider<T>()`.
        return contribute(DATA_PROVIDER, spec as DataProviderSpec);
      },
    },

    commands: {
      register(spec: CommandSpec): Disposable {
        store().addCommand(pluginName, spec);
        return track({ dispose: () => store().removeCommand(pluginName, spec.id) });
      },
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
      onChange: (key, fn) => useConfigStore.getState().subscribe(key, fn),
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
      registerErrorFallback: (spec: PluginErrorFallbackSpec): Disposable =>
        contribute(ERROR_FALLBACK, spec),
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

    i18n: {
      addBundle(locale: string, dict: Record<string, string>): Disposable {
        addLocaleBundle(locale, dict);
        // i18next has no per-key removal; leaving bundles registered is
        // safe because t() only matters while plugin UI is mounted, and a
        // same-name reload overwrites cleanly.
        return track({ dispose: () => {} });
      },
      registerLocale: (spec): Disposable => contribute(LOCALE, spec),
    },

    tasks: {
      start(opts) {
        return startTask(pluginName, opts);
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
  return denied as unknown as Host;
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

// Method-name lookup beats the previous nested ternary — adding a
// level is one line. Stored as the method *name* (not a reference) so
// vitest's `vi.spyOn(console, "info")` after module load still binds.
const CONSOLE_METHOD: Record<LogLevel, "log" | "info" | "warn" | "error"> = {
  debug: "log",
  info: "info",
  warn: "warn",
  error: "error",
};

function logToConsole(plugin: string, level: LogLevel, args: unknown[]): void {
  console[CONSOLE_METHOD[level]](`[plugin:${plugin}]`, ...args);
}

// Module-scoped log emission: forwards to console with a `[plugin:<name>]`
// prefix (preserving the existing log shape) and fans the event out to
// every registered subscriber. Subscriber failures are isolated.
function emitLog(plugin: string, level: LogLevel, args: unknown[]): void {
  logToConsole(plugin, level, args);
  const event = { plugin, level, args, timestamp: Date.now() };
  for (const o of usePluginStore.getState().logSubscribers.values()) {
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
export interface PluginToastDetail {
  message: string;
  level: ToastLevel;
}

function dispatchToast(message: string, level: ToastLevel): void {
  if (typeof window === "undefined") return;
  window.dispatchEvent(
    new CustomEvent<PluginToastDetail>(PLUGIN_TOAST_EVENT, { detail: { message, level } }),
  );
}
