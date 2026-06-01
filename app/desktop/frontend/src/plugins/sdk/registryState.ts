// The registry's state shape + action signatures + the `freshState`
// factory. Pulled out of registry.ts so the Zustand store there is
// purely the action implementations; adding a new slot is a two-file
// edit (this file + registry.ts) instead of one file scrolled. Map
// mutation helpers live in `registryHelpers.ts`.

import type {
  BeforeUnloadHandler,
  CommandSpec,
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  CoreEventHandler,
  CustomEventHandler,
  LayoutSlotSpec,
  LoadedPlugin,
  LogSubscriber,
  PluginSpec,
  ReadyHandler,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
  SettingsPaneSpec,
  WorkspaceViewSpec,
} from "./types";

export interface Owned<T> {
  pluginName: string;
  value: T;
}

/**
 * One entry in the shared open-extension-point map. `point` is the point id
 * the entry belongs to; `key` is the dedupe key within that point (the
 * normalized single key, or the minted id for multi points) so consumers can
 * recover it without parsing the composite map key; `order` is the optional
 * sort hint passed at contribute time (the item may also carry its own
 * `order`); `item` is the contributed value. See `defineExtensionPoint` +
 * `selectors/extensions.ts`.
 */
export interface ContributionEntry {
  point: string;
  key: string;
  order?: number;
  item: unknown;
}

export interface PluginStoreState {
  loaded: Map<string, LoadedPlugin>;
  // Composite key `${pluginName}|${id}` so multiple handlers can be
  // registered for the same custom event name across (or within) plugins.
  // The reducer fans the event out through every match in registration
  // order, chaining StateUpdate returns.
  customEventHandlers: Map<string, Owned<{ name: string; handler: CustomEventHandler<unknown> }>>;
  // Built-in AG-UI events can have *multiple* handlers per type — they
  // chain. The key is `${eventType}|${pluginName}|${id}` to keep insertion
  // order stable + allow the same plugin to register more than one handler
  // for the same type (rare but legal).
  coreEventHandlers: Map<string, Owned<{ eventType: string; handler: CoreEventHandler }>>;
  settingsPanes: Map<string, Owned<SettingsPaneSpec>>;
  // Layout slot key is `${slot}|${pluginName}|${spec.id}` to allow the same
  // plugin to fill multiple slots and to keep insertion order deterministic.
  layoutSlots: Map<string, Owned<{ slot: string; spec: LayoutSlotSpec }>>;
  commands: Map<string, Owned<CommandSpec>>;
  /**
   * Commands declared in `PluginSpec.contributes.commands` but whose
   * owning plugin hasn't been activated yet. Displayed as palette
   * placeholders; running one activates the plugin first, then dispatches
   * to whatever `host.commands.register` set up during setup.
   */
  declaredCommands: Map<string, Owned<ContributedCommand>>;
  /** Same idea, for workspace views. Mounting renders a placeholder UI. */
  declaredViews: Map<string, Owned<ContributedView>>;
  /** Same idea, for settings panes. */
  declaredSettingsPanes: Map<string, Owned<ContributedSettingsPane>>;
  /**
   * Specs awaiting an activation event. Keyed by plugin name so we can
   * activate by id; the value carries the spec + the list of events it
   * registered for (used by the palette to map "onCommand:foo" back to a
   * plugin).
   */
  pendingActivations: Map<string, { spec: PluginSpec; events: string[] }>;
  // RPC hooks: composite key `${pluginName}|${id}` to allow multiple per plugin.
  rpcBeforeRequest: Map<string, Owned<RpcBeforeRequestHook>>;
  rpcAfterResponse: Map<string, Owned<RpcAfterResponseHook>>;
  // Log subscribers — composite key, same pattern.
  logSubscribers: Map<string, Owned<LogSubscriber>>;
  // Lifecycle hooks — also composite key.
  readyHandlers: Map<string, Owned<ReadyHandler>>;
  beforeUnloadHandlers: Map<string, Owned<BeforeUnloadHandler>>;
  /** Set true after PluginProvider has finished loading built-ins. */
  appReady: boolean;
  // Plugin-load / unload listeners — composite key per registration.
  pluginLoadListeners: Map<string, Owned<(spec: PluginSpec) => void>>;
  pluginUnloadListeners: Map<string, Owned<(name: string) => void>>;
  workspaceViews: Map<string, Owned<WorkspaceViewSpec>>;
  // Open extension points — the unified substrate. Plugin-defined points
  // (and, post-collapse, every kernel point) store their contributions here
  // keyed by `${point.id}#${dedupeKey}`. Read via the extensions selector.
  extensions: Map<string, Owned<ContributionEntry>>;
  // Window title state — most-recent setter wins. Stored as the "base"
  // text + a badge count; document.title is derived: `[n] base`.
  windowTitle: string;
  windowBadge: number;
}

export interface PluginStoreActions {
  registerLoaded: (plugin: LoadedPlugin) => void;
  unload: (pluginName: string) => void;

  addCustomEventHandler: (
    pluginName: string,
    name: string,
    id: string,
    h: CustomEventHandler<unknown>,
  ) => void;
  removeCustomEventHandler: (pluginName: string, id: string) => void;

  addSettingsPane: (pluginName: string, spec: SettingsPaneSpec) => void;
  removeSettingsPane: (pluginName: string, id: string) => void;

  addCoreEventHandler: (
    pluginName: string,
    eventType: string,
    id: string,
    handler: CoreEventHandler,
  ) => void;
  removeCoreEventHandler: (pluginName: string, eventType: string, id: string) => void;

  addLayoutSlot: (pluginName: string, slot: string, spec: LayoutSlotSpec) => void;
  removeLayoutSlot: (pluginName: string, slot: string, id: string) => void;

  addCommand: (pluginName: string, spec: CommandSpec) => void;
  removeCommand: (pluginName: string, id: string) => void;

  addDeclaredCommand: (pluginName: string, spec: ContributedCommand) => void;
  removeDeclaredCommand: (pluginName: string, id: string) => void;
  removeDeclaredCommandsBy: (pluginName: string) => void;

  addDeclaredView: (pluginName: string, spec: ContributedView) => void;
  removeDeclaredViewsBy: (pluginName: string) => void;

  addDeclaredSettingsPane: (pluginName: string, spec: ContributedSettingsPane) => void;
  removeDeclaredSettingsPanesBy: (pluginName: string) => void;

  addPendingActivation: (spec: PluginSpec, events: string[]) => void;
  removePendingActivation: (name: string) => void;

  addRpcBeforeRequest: (pluginName: string, id: string, hook: RpcBeforeRequestHook) => void;
  removeRpcBeforeRequest: (pluginName: string, id: string) => void;

  addRpcAfterResponse: (pluginName: string, id: string, hook: RpcAfterResponseHook) => void;
  removeRpcAfterResponse: (pluginName: string, id: string) => void;

  addLogSubscriber: (pluginName: string, id: string, fn: LogSubscriber) => void;
  removeLogSubscriber: (pluginName: string, id: string) => void;

  addReadyHandler: (pluginName: string, id: string, fn: ReadyHandler) => void;
  removeReadyHandler: (pluginName: string, id: string) => void;

  addBeforeUnloadHandler: (pluginName: string, id: string, fn: BeforeUnloadHandler) => void;
  removeBeforeUnloadHandler: (pluginName: string, id: string) => void;

  /** Mark the app as ready — fires all registered readyHandlers in order. */
  markAppReady: () => void;

  addPluginLoadListener: (pluginName: string, id: string, fn: (spec: PluginSpec) => void) => void;
  removePluginLoadListener: (pluginName: string, id: string) => void;
  addPluginUnloadListener: (pluginName: string, id: string, fn: (name: string) => void) => void;
  removePluginUnloadListener: (pluginName: string, id: string) => void;

  addWorkspaceView: (pluginName: string, spec: WorkspaceViewSpec) => void;
  removeWorkspaceView: (pluginName: string, id: string) => void;

  // Open extension points (substrate). `outerKey` is the fully-qualified
  // `${point}#${dedupeKey}`; the host computes it (single vs multi keying)
  // and hands it back via the disposable so removal is exact.
  addContribution: (
    pluginName: string,
    point: string,
    outerKey: string,
    entry: ContributionEntry,
    conflictKey: string,
  ) => void;
  removeContribution: (pluginName: string, outerKey: string) => void;

  setWindowTitle: (text: string) => void;
  setWindowBadge: (n: number) => void;

  /**
   * Wipe the registry back to a fresh state. Only used by the test
   * harness — production code should never see this fire.
   */
  resetForTest: () => void;
}

// Single source of truth for the "fresh registry" shape. New slots only
// need to be added here — the test setup, the reset action, and the store
// initializer all call this.
export function freshState(): PluginStoreState {
  return {
    loaded: new Map(),
    customEventHandlers: new Map(),
    coreEventHandlers: new Map(),
    settingsPanes: new Map(),
    layoutSlots: new Map(),
    commands: new Map(),
    declaredCommands: new Map(),
    declaredViews: new Map(),
    declaredSettingsPanes: new Map(),
    pendingActivations: new Map(),
    rpcBeforeRequest: new Map(),
    rpcAfterResponse: new Map(),
    logSubscribers: new Map(),
    readyHandlers: new Map(),
    beforeUnloadHandlers: new Map(),
    pluginLoadListeners: new Map(),
    pluginUnloadListeners: new Map(),
    workspaceViews: new Map(),
    extensions: new Map(),
    appReady: false,
    windowTitle: "",
    windowBadge: 0,
  };
}
