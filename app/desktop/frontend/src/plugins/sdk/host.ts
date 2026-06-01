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
  AGENT_SOURCE,
  BEFORE_UNLOAD_HANDLER,
  COMMAND,
  COMPOSER_ATTACHMENT_SOURCE,
  COMPOSER_KEY_BINDING,
  COMPOSER_MODE,
  COMPOSER_PLACEHOLDER,
  COMPOSER_STATUS,
  CONTENT_BLOCK,
  CORE_EVENT_HANDLER,
  CUSTOM_EVENT_HANDLER,
  DATA_PROVIDER,
  ERROR_FALLBACK,
  LAYOUT_SLOT,
  LOCALE,
  LOG_SUBSCRIBER,
  MESSAGE_ROLE,
  PLUGIN_LOAD_LISTENER,
  PLUGIN_UNLOAD_LISTENER,
  READY_HANDLER,
  ROUTE,
  RPC_AFTER_RESPONSE,
  RPC_BEFORE_REQUEST,
  SETTINGS_PANE,
  SHORTCUT,
  SIDEBAR_RAIL_ITEM,
  SIDEBAR_SECTION,
  SLASH_COMMAND,
  TOOL_ACTION,
  TOOL_ICON,
  TOOL_PREVIEW,
  WORKSPACE_VIEW,
} from "./kernelPoints";
import { useNotificationStore } from "./notifications";
import { usePluginStore } from "./registry";
import { lookupExtensionByKey, lookupExtensionPoint } from "./selectors/extensions";
import { getOrCreateSlice } from "./stateSlice";
import { createStorage } from "./storage";

/**
 * Build a Host bound to a specific plugin. `register*` returns Disposables;
 * `setup`'s caller (loadPlugin) collects them so it can dispose on failure
 * or on unload.
 */
// Monotonic id minter for `multi` extension-point contributions that don't
// pass an explicit `opts.id` (custom/core event handlers, rpc + log hooks,
// lifecycle observers). Uniqueness only needs to hold within one point's
// keyspace; a global counter is simpler than per-point ones and the ids
// aren't exposed to plugin code.
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
      { point: point.id, key: conflictKey, order: opts?.order, item },
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
      // Both fan out to every matching handler; `multi` keying lets a plugin
      // register more than once for the same name/type (each contribution
      // coexists). The reducer chains StateUpdate returns across them.
      on: <T = unknown>(name: string, handler: CustomEventHandler<T>): Disposable =>
        contribute(CUSTOM_EVENT_HANDLER, {
          name,
          handler: handler as CustomEventHandler<unknown>,
        }),
      onCore: (eventType: string, handler: CoreEventHandler): Disposable =>
        contribute(CORE_EVENT_HANDLER, { eventType, handler }),
    },

    layout: {
      // Stable id `${slot}#${spec.id}` so re-registering the same slot entry
      // overwrites rather than stacking a duplicate (a dup would render the
      // same component twice under one React key in <Slot>).
      register: (slot: string, spec: LayoutSlotSpec): Disposable =>
        contribute(LAYOUT_SLOT, { slot, spec }, { id: `${slot}#${spec.id}` }),
    },

    workspace: {
      registerView: (spec: WorkspaceViewSpec): Disposable => contribute(WORKSPACE_VIEW, spec),
      openView(id: string): void {
        const view = lookupExtensionByKey(WORKSPACE_VIEW, id);
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

    router: {
      register: (spec: RouteSpec): Disposable => contribute(ROUTE, spec),
    },

    composer: {
      registerCommand(cmd: string, spec: SlashCommandSpec): Disposable {
        // Normalize so callers can omit the leading slash.
        const key = cmd.startsWith("/") ? cmd : `/${cmd}`;
        return contribute(SLASH_COMMAND, spec, { key });
      },
      registerStatus: (spec: ComposerStatusSpec): Disposable => contribute(COMPOSER_STATUS, spec),
      registerMode: (spec: ComposerModeSpec): Disposable => contribute(COMPOSER_MODE, spec),
      registerPlaceholder: (spec: ComposerPlaceholderSpec): Disposable =>
        contribute(COMPOSER_PLACEHOLDER, spec),
      registerAttachmentSource: (spec: ComposerAttachmentSourceSpec): Disposable =>
        contribute(COMPOSER_ATTACHMENT_SOURCE, spec),
      registerKeyBinding: (spec: ComposerKeyBindingSpec): Disposable =>
        contribute(COMPOSER_KEY_BINDING, spec),
    },

    sidebar: {
      registerSection: (spec: SidebarSectionSpec): Disposable => contribute(SIDEBAR_SECTION, spec),
      registerRailItem: (spec: SidebarRailItemSpec): Disposable =>
        contribute(SIDEBAR_RAIL_ITEM, spec),
    },

    shortcuts: {
      register: (spec: ShortcutSpec): Disposable => contribute(SHORTCUT, spec),
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
      register: (spec: CommandSpec): Disposable => contribute(COMMAND, spec),
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
        return contribute(READY_HANDLER, fn);
      },
      onBeforeUnload: (fn: BeforeUnloadHandler): Disposable =>
        contribute(BEFORE_UNLOAD_HANDLER, fn),
    },

    settings: {
      registerPane: (spec: SettingsPaneSpec): Disposable => contribute(SETTINGS_PANE, spec),
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
  for (const fn of lookupExtensionPoint(LOG_SUBSCRIBER)) {
    safeCall(() => fn(event), "[plugin] log subscriber threw:");
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
