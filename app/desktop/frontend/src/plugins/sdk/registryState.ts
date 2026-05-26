// The registry's state shape + action signatures + the `freshState`
// factory. Pulled out of registry.ts so the Zustand store there is
// purely the action implementations; adding a new slot is a two-file
// edit (this file + registry.ts) instead of one file scrolled. Map
// mutation helpers live in `registryHelpers.ts`.

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
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  CoreEventHandler,
  CustomEventHandler,
  DataProviderSpec,
  LayoutSlotSpec,
  LoadedPlugin,
  LocaleSpec,
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

export interface Owned<T> {
  pluginName: string;
  value: T;
}

export interface PluginStoreState {
  loaded: Map<string, LoadedPlugin>;
  toolPreviews: Map<string, Owned<ToolPreviewComponent>>;
  toolActions: Map<string, Owned<ToolActionSpec>>;
  toolIcons: Map<string, Owned<string>>;
  contentBlocks: Map<string, Owned<ContentBlockRenderer<ContentBlockKind>>>;
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
  slashCommands: Map<string, Owned<SlashCommandSpec>>;
  settingsPanes: Map<string, Owned<SettingsPaneSpec>>;
  // Layout slot key is `${slot}|${pluginName}|${spec.id}` to allow the same
  // plugin to fill multiple slots and to keep insertion order deterministic.
  layoutSlots: Map<string, Owned<{ slot: string; spec: LayoutSlotSpec }>>;
  themes: Map<string, Owned<ThemeSpec>>;
  accents: Map<string, Owned<ThemeAccentSpec>>;
  locales: Map<string, Owned<LocaleSpec>>;
  routes: Map<string, Owned<RouteSpec>>;
  shortcuts: Map<string, Owned<ShortcutSpec>>;
  composerStatus: Map<string, Owned<ComposerStatusSpec>>;
  composerModes: Map<string, Owned<ComposerModeSpec>>;
  composerPlaceholders: Map<string, Owned<ComposerPlaceholderSpec>>;
  composerAttachmentSources: Map<string, Owned<ComposerAttachmentSourceSpec>>;
  composerKeyBindings: Map<string, Owned<ComposerKeyBindingSpec>>;
  sidebarSections: Map<string, Owned<SidebarSectionSpec>>;
  agentSources: Map<string, Owned<AgentSourceSpec>>;
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
  dataProviders: Map<string, Owned<DataProviderSpec>>;
  sidebarRailItems: Map<string, Owned<SidebarRailItemSpec>>;
  messageRoles: Map<string, Owned<MessageRoleSpec>>;
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
  pluginErrorFallbacks: Map<string, Owned<PluginErrorFallbackSpec>>;
  workspaceViews: Map<string, Owned<WorkspaceViewSpec>>;
  // Window title state — most-recent setter wins. Stored as the "base"
  // text + a badge count; document.title is derived: `[n] base`.
  windowTitle: string;
  windowBadge: number;
}

export interface PluginStoreActions {
  registerLoaded: (plugin: LoadedPlugin) => void;
  unload: (pluginName: string) => void;

  addToolPreview: (pluginName: string, fn: string, c: ToolPreviewComponent) => void;
  removeToolPreview: (pluginName: string, fn: string) => void;

  addToolAction: (pluginName: string, spec: ToolActionSpec) => void;
  removeToolAction: (pluginName: string, id: string) => void;

  addToolIcon: (pluginName: string, fn: string, icon: string) => void;
  removeToolIcon: (pluginName: string, fn: string) => void;

  addContentBlock: (
    pluginName: string,
    kind: string,
    r: ContentBlockRenderer<ContentBlockKind>,
  ) => void;
  removeContentBlock: (pluginName: string, kind: string) => void;

  addCustomEventHandler: (
    pluginName: string,
    name: string,
    id: string,
    h: CustomEventHandler<unknown>,
  ) => void;
  removeCustomEventHandler: (pluginName: string, id: string) => void;

  addSlashCommand: (pluginName: string, cmd: string, spec: SlashCommandSpec) => void;
  removeSlashCommand: (pluginName: string, cmd: string) => void;

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

  addTheme: (pluginName: string, spec: ThemeSpec) => void;
  removeTheme: (pluginName: string, id: string) => void;

  addAccent: (pluginName: string, spec: ThemeAccentSpec) => void;
  removeAccent: (pluginName: string, id: string) => void;

  addLocale: (pluginName: string, spec: LocaleSpec) => void;
  removeLocale: (pluginName: string, id: string) => void;

  addRoute: (pluginName: string, spec: RouteSpec) => void;
  removeRoute: (pluginName: string, id: string) => void;

  addShortcut: (pluginName: string, spec: ShortcutSpec) => void;
  removeShortcut: (pluginName: string, key: string) => void;

  addComposerStatus: (pluginName: string, spec: ComposerStatusSpec) => void;
  removeComposerStatus: (pluginName: string, id: string) => void;

  addComposerMode: (pluginName: string, spec: ComposerModeSpec) => void;
  removeComposerMode: (pluginName: string, id: string) => void;

  addComposerPlaceholder: (pluginName: string, spec: ComposerPlaceholderSpec) => void;
  removeComposerPlaceholder: (pluginName: string, id: string) => void;

  addComposerAttachmentSource: (pluginName: string, spec: ComposerAttachmentSourceSpec) => void;
  removeComposerAttachmentSource: (pluginName: string, id: string) => void;

  addComposerKeyBinding: (pluginName: string, spec: ComposerKeyBindingSpec) => void;
  removeComposerKeyBinding: (pluginName: string, key: string) => void;

  addSidebarSection: (pluginName: string, spec: SidebarSectionSpec) => void;
  removeSidebarSection: (pluginName: string, id: string) => void;

  addAgentSource: (pluginName: string, spec: AgentSourceSpec) => void;
  removeAgentSource: (pluginName: string, id: string) => void;

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

  addDataProvider: (pluginName: string, spec: DataProviderSpec) => void;
  removeDataProvider: (pluginName: string, key: string) => void;

  addSidebarRailItem: (pluginName: string, spec: SidebarRailItemSpec) => void;
  removeSidebarRailItem: (pluginName: string, id: string) => void;

  addMessageRole: (pluginName: string, spec: MessageRoleSpec) => void;
  removeMessageRole: (pluginName: string, id: string) => void;

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

  addPluginErrorFallback: (pluginName: string, spec: PluginErrorFallbackSpec) => void;
  removePluginErrorFallback: (pluginName: string, id: string) => void;

  addWorkspaceView: (pluginName: string, spec: WorkspaceViewSpec) => void;
  removeWorkspaceView: (pluginName: string, id: string) => void;

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
    toolPreviews: new Map(),
    toolActions: new Map(),
    toolIcons: new Map(),
    contentBlocks: new Map(),
    customEventHandlers: new Map(),
    coreEventHandlers: new Map(),
    slashCommands: new Map(),
    settingsPanes: new Map(),
    layoutSlots: new Map(),
    themes: new Map(),
    accents: new Map(),
    locales: new Map(),
    routes: new Map(),
    shortcuts: new Map(),
    composerStatus: new Map(),
    composerModes: new Map(),
    composerPlaceholders: new Map(),
    composerAttachmentSources: new Map(),
    composerKeyBindings: new Map(),
    sidebarSections: new Map(),
    agentSources: new Map(),
    commands: new Map(),
    declaredCommands: new Map(),
    declaredViews: new Map(),
    declaredSettingsPanes: new Map(),
    pendingActivations: new Map(),
    dataProviders: new Map(),
    sidebarRailItems: new Map(),
    messageRoles: new Map(),
    rpcBeforeRequest: new Map(),
    rpcAfterResponse: new Map(),
    logSubscribers: new Map(),
    readyHandlers: new Map(),
    beforeUnloadHandlers: new Map(),
    pluginLoadListeners: new Map(),
    pluginUnloadListeners: new Map(),
    pluginErrorFallbacks: new Map(),
    workspaceViews: new Map(),
    appReady: false,
    windowTitle: "",
    windowBadge: 0,
  };
}
