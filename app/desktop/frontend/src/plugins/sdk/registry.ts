// Central Zustand store of every plugin contribution — React
// components subscribe so registrations propagate live.

import { create } from "zustand";
import type { ContentBlockKind } from "@/protocol/agui/viewState";
import type {
  AgentSourceSpec,
  CommandSpec,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  PluginSpec,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
  ContentBlockRenderer,
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  PluginErrorFallbackSpec,
  WorkspaceViewSpec,
  CoreEventHandler,
  CustomEventHandler,
  DataProviderSpec,
  LayoutSlotSpec,
  LoadedPlugin,
  BeforeUnloadHandler,
  LogSubscriber,
  MessageRoleSpec,
  ReadyHandler,
  RouteSpec,
  SettingsPaneSpec,
  ShortcutSpec,
  SidebarRailItemSpec,
  SidebarSectionSpec,
  SlashCommandSpec,
  RpcBeforeRequestHook,
  RpcAfterResponseHook,
  ThemeAccentSpec,
  ThemeSpec,
  ToolActionSpec,
  ToolPreviewComponent,
} from "./types";
import { safeCall } from "./errors";

type Owned<T> = { pluginName: string; value: T };

type PluginStoreState = {
  loaded: Map<string, LoadedPlugin>;
  toolPreviews: Map<string, Owned<ToolPreviewComponent>>;
  toolActions: Map<string, Owned<ToolActionSpec>>;
  toolIcons: Map<string, Owned<string>>;
  contentBlocks: Map<string, Owned<ContentBlockRenderer<ContentBlockKind>>>;
  customEventHandlers: Map<string, Owned<CustomEventHandler<unknown>>>;
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
};

type PluginStoreActions = {
  registerLoaded(plugin: LoadedPlugin): void;
  unload(pluginName: string): void;

  addToolPreview(pluginName: string, fn: string, c: ToolPreviewComponent): void;
  removeToolPreview(pluginName: string, fn: string): void;

  addToolAction(pluginName: string, spec: ToolActionSpec): void;
  removeToolAction(pluginName: string, id: string): void;

  addToolIcon(pluginName: string, fn: string, icon: string): void;
  removeToolIcon(pluginName: string, fn: string): void;

  addContentBlock(
    pluginName: string,
    kind: string,
    r: ContentBlockRenderer<ContentBlockKind>,
  ): void;
  removeContentBlock(pluginName: string, kind: string): void;

  addCustomEventHandler(pluginName: string, name: string, h: CustomEventHandler<unknown>): void;
  removeCustomEventHandler(pluginName: string, name: string): void;

  addSlashCommand(pluginName: string, cmd: string, spec: SlashCommandSpec): void;
  removeSlashCommand(pluginName: string, cmd: string): void;

  addSettingsPane(pluginName: string, spec: SettingsPaneSpec): void;
  removeSettingsPane(pluginName: string, id: string): void;

  addCoreEventHandler(
    pluginName: string,
    eventType: string,
    id: string,
    handler: CoreEventHandler,
  ): void;
  removeCoreEventHandler(pluginName: string, eventType: string, id: string): void;

  addLayoutSlot(pluginName: string, slot: string, spec: LayoutSlotSpec): void;
  removeLayoutSlot(pluginName: string, slot: string, id: string): void;

  addTheme(pluginName: string, spec: ThemeSpec): void;
  removeTheme(pluginName: string, id: string): void;

  addAccent(pluginName: string, spec: ThemeAccentSpec): void;
  removeAccent(pluginName: string, id: string): void;

  addRoute(pluginName: string, spec: RouteSpec): void;
  removeRoute(pluginName: string, id: string): void;

  addShortcut(pluginName: string, spec: ShortcutSpec): void;
  removeShortcut(pluginName: string, key: string): void;

  addComposerStatus(pluginName: string, spec: ComposerStatusSpec): void;
  removeComposerStatus(pluginName: string, id: string): void;

  addComposerMode(pluginName: string, spec: ComposerModeSpec): void;
  removeComposerMode(pluginName: string, id: string): void;

  addComposerPlaceholder(pluginName: string, spec: ComposerPlaceholderSpec): void;
  removeComposerPlaceholder(pluginName: string, id: string): void;

  addComposerAttachmentSource(pluginName: string, spec: ComposerAttachmentSourceSpec): void;
  removeComposerAttachmentSource(pluginName: string, id: string): void;

  addComposerKeyBinding(pluginName: string, spec: ComposerKeyBindingSpec): void;
  removeComposerKeyBinding(pluginName: string, key: string): void;

  addSidebarSection(pluginName: string, spec: SidebarSectionSpec): void;
  removeSidebarSection(pluginName: string, id: string): void;

  addAgentSource(pluginName: string, spec: AgentSourceSpec): void;
  removeAgentSource(pluginName: string, id: string): void;

  addCommand(pluginName: string, spec: CommandSpec): void;
  removeCommand(pluginName: string, id: string): void;

  addDeclaredCommand(pluginName: string, spec: ContributedCommand): void;
  removeDeclaredCommand(pluginName: string, id: string): void;
  removeDeclaredCommandsBy(pluginName: string): void;

  addDeclaredView(pluginName: string, spec: ContributedView): void;
  removeDeclaredViewsBy(pluginName: string): void;

  addDeclaredSettingsPane(pluginName: string, spec: ContributedSettingsPane): void;
  removeDeclaredSettingsPanesBy(pluginName: string): void;

  addPendingActivation(spec: PluginSpec, events: string[]): void;
  removePendingActivation(name: string): void;

  addDataProvider(pluginName: string, spec: DataProviderSpec): void;
  removeDataProvider(pluginName: string, key: string): void;

  addSidebarRailItem(pluginName: string, spec: SidebarRailItemSpec): void;
  removeSidebarRailItem(pluginName: string, id: string): void;

  addMessageRole(pluginName: string, spec: MessageRoleSpec): void;
  removeMessageRole(pluginName: string, id: string): void;

  addRpcBeforeRequest(pluginName: string, id: string, hook: RpcBeforeRequestHook): void;
  removeRpcBeforeRequest(pluginName: string, id: string): void;

  addRpcAfterResponse(pluginName: string, id: string, hook: RpcAfterResponseHook): void;
  removeRpcAfterResponse(pluginName: string, id: string): void;

  addLogSubscriber(pluginName: string, id: string, fn: LogSubscriber): void;
  removeLogSubscriber(pluginName: string, id: string): void;

  addReadyHandler(pluginName: string, id: string, fn: ReadyHandler): void;
  removeReadyHandler(pluginName: string, id: string): void;

  addBeforeUnloadHandler(pluginName: string, id: string, fn: BeforeUnloadHandler): void;
  removeBeforeUnloadHandler(pluginName: string, id: string): void;

  /** Mark the app as ready — fires all registered readyHandlers in order. */
  markAppReady(): void;

  addPluginLoadListener(pluginName: string, id: string, fn: (spec: PluginSpec) => void): void;
  removePluginLoadListener(pluginName: string, id: string): void;
  addPluginUnloadListener(pluginName: string, id: string, fn: (name: string) => void): void;
  removePluginUnloadListener(pluginName: string, id: string): void;

  addPluginErrorFallback(pluginName: string, spec: PluginErrorFallbackSpec): void;
  removePluginErrorFallback(pluginName: string, id: string): void;

  addWorkspaceView(pluginName: string, spec: WorkspaceViewSpec): void;
  removeWorkspaceView(pluginName: string, id: string): void;

  setWindowTitle(text: string): void;
  setWindowBadge(n: number): void;

  /**
   * Wipe the registry back to a fresh state. Only used by the test
   * harness — production code should never see this fire.
   */
  resetForTest(): void;
};

// Every registry slot follows the same shape; factor add/remove to avoid
// duplicating the conflict-warning logic four times.
function addOwned<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  key: string,
  value: T,
  label: string,
): Map<string, Owned<T>> {
  const existing = map.get(key);
  if (existing && existing.pluginName !== pluginName) {
     
    console.warn(
      `[plugin] ${pluginName} overrides ${label} "${key}" ` +
        `previously registered by ${existing.pluginName}`,
    );
  }
  const next = new Map(map);
  next.set(key, { pluginName, value });
  return next;
}

function removeOwned<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  key: string,
): Map<string, Owned<T>> {
  const entry = map.get(key);
  if (!entry || entry.pluginName !== pluginName) return map;
  const next = new Map(map);
  next.delete(key);
  return next;
}

// ---------------------------------------------------------------------------
// Composite-key helpers (multi-registration slots).
//
// For slots that allow multiple registrations per (plugin, id) — RPC hooks,
// log subscribers, lifecycle hooks, plugin observers, core event handlers,
// layout slots — the key is `${pluginName}|${id}` (or a discriminated id
// like `${slot}#${spec.id}` baked in by the caller). No conflict warning
// is meaningful for these: multiple entries per plugin are intentional.
//
// All eight composite slots used to hand-roll the same five-line add/
// remove pair; these helpers DRY that up.
// ---------------------------------------------------------------------------

function compositeKey(pluginName: string, id: string): string {
  return `${pluginName}|${id}`;
}

function addOwnedMulti<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  id: string,
  value: T,
): Map<string, Owned<T>> {
  const next = new Map(map);
  next.set(compositeKey(pluginName, id), { pluginName, value });
  return next;
}

function removeOwnedMulti<T>(
  map: Map<string, Owned<T>>,
  pluginName: string,
  id: string,
): Map<string, Owned<T>> {
  const next = new Map(map);
  next.delete(compositeKey(pluginName, id));
  return next;
}

// "Drop every entry a given plugin owns" — used by the declared-* slots
// when a plugin's activation replaces its entire batch of placeholder
// contributions at once. Pure helper so the three callsites read the
// same way as the per-entry add/remove.
function clearByPlugin<T>(map: Map<string, Owned<T>>, pluginName: string): Map<string, Owned<T>> {
  const next = new Map(map);
  for (const [k, v] of next) if (v.pluginName === pluginName) next.delete(k);
  return next;
}

// Single source of truth for the "fresh registry" shape. New slots only
// need to be added here — the test setup, the reset action, and the
// store initializer all call this.
function freshState(): PluginStoreState {
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

export const usePluginStore = create<PluginStoreState & PluginStoreActions>((set, get) => ({
  ...freshState(),

  registerLoaded(plugin) {
    const next = new Map(get().loaded);
    next.set(plugin.spec.name, plugin);
    set({ loaded: next });
    // Fan out to onLoad listeners — isolated per subscriber.
    for (const o of get().pluginLoadListeners.values()) {
      safeCall(
        () => o.value(plugin.spec),
        `[plugin] ${o.pluginName} onLoad listener threw:`,
      );
    }
  },

  unload(pluginName) {
    const plugin = get().loaded.get(pluginName);
    if (!plugin) return;
    for (const d of plugin.disposables) {
      safeCall(() => d.dispose(), `[plugin] ${pluginName} dispose threw:`);
    }
    const next = new Map(get().loaded);
    next.delete(pluginName);
    set({ loaded: next });
    for (const o of get().pluginUnloadListeners.values()) {
      safeCall(
        () => o.value(pluginName),
        `[plugin] ${o.pluginName} onUnload listener threw:`,
      );
    }
  },

  addToolPreview(pluginName, fn, component) {
    set({ toolPreviews: addOwned(get().toolPreviews, pluginName, fn, component, "tool preview") });
  },
  removeToolPreview(pluginName, fn) {
    set({ toolPreviews: removeOwned(get().toolPreviews, pluginName, fn) });
  },

  addToolAction(pluginName, spec) {
    set({ toolActions: addOwned(get().toolActions, pluginName, spec.id, spec, "tool action") });
  },
  removeToolAction(pluginName, id) {
    set({ toolActions: removeOwned(get().toolActions, pluginName, id) });
  },

  addToolIcon(pluginName, fn, icon) {
    set({ toolIcons: addOwned(get().toolIcons, pluginName, fn, icon, "tool icon") });
  },
  removeToolIcon(pluginName, fn) {
    set({ toolIcons: removeOwned(get().toolIcons, pluginName, fn) });
  },

  addContentBlock(pluginName, kind, renderer) {
    set({
      contentBlocks: addOwned(get().contentBlocks, pluginName, kind, renderer, "content block"),
    });
  },
  removeContentBlock(pluginName, kind) {
    set({ contentBlocks: removeOwned(get().contentBlocks, pluginName, kind) });
  },

  addCustomEventHandler(pluginName, name, handler) {
    set({
      customEventHandlers: addOwned(
        get().customEventHandlers,
        pluginName,
        name,
        handler,
        "agui handler",
      ),
    });
  },
  removeCustomEventHandler(pluginName, name) {
    set({ customEventHandlers: removeOwned(get().customEventHandlers, pluginName, name) });
  },

  addSlashCommand(pluginName, cmd, spec) {
    set({ slashCommands: addOwned(get().slashCommands, pluginName, cmd, spec, "slash command") });
  },
  removeSlashCommand(pluginName, cmd) {
    set({ slashCommands: removeOwned(get().slashCommands, pluginName, cmd) });
  },

  addSettingsPane(pluginName, spec) {
    set({
      settingsPanes: addOwned(get().settingsPanes, pluginName, spec.id, spec, "settings pane"),
    });
  },
  removeSettingsPane(pluginName, id) {
    set({ settingsPanes: removeOwned(get().settingsPanes, pluginName, id) });
  },

  // Core handlers + layout slots both pre-baked their discriminator
  // (eventType / slot) into the composite key so the same plugin can
  // register more than one entry per discriminator. We still bake it in,
  // but via the shared helper now.
  addCoreEventHandler(pluginName, eventType, id, handler) {
    set({
      coreEventHandlers: addOwnedMulti(get().coreEventHandlers, pluginName, `${eventType}#${id}`, {
        eventType,
        handler,
      }),
    });
  },
  removeCoreEventHandler(pluginName, eventType, id) {
    set({
      coreEventHandlers: removeOwnedMulti(
        get().coreEventHandlers,
        pluginName,
        `${eventType}#${id}`,
      ),
    });
  },

  addLayoutSlot(pluginName, slot, spec) {
    set({
      layoutSlots: addOwnedMulti(get().layoutSlots, pluginName, `${slot}#${spec.id}`, {
        slot,
        spec,
      }),
    });
  },
  removeLayoutSlot(pluginName, slot, id) {
    set({ layoutSlots: removeOwnedMulti(get().layoutSlots, pluginName, `${slot}#${id}`) });
  },

  addTheme(pluginName, spec) {
    set({ themes: addOwned(get().themes, pluginName, spec.id, spec, "theme") });
  },
  removeTheme(pluginName, id) {
    set({ themes: removeOwned(get().themes, pluginName, id) });
  },

  addAccent(pluginName, spec) {
    set({ accents: addOwned(get().accents, pluginName, spec.id, spec, "accent") });
  },
  removeAccent(pluginName, id) {
    set({ accents: removeOwned(get().accents, pluginName, id) });
  },

  addRoute(pluginName, spec) {
    set({ routes: addOwned(get().routes, pluginName, spec.id, spec, "route") });
  },
  removeRoute(pluginName, id) {
    set({ routes: removeOwned(get().routes, pluginName, id) });
  },

  addShortcut(pluginName, spec) {
    // Normalize on the way in so registration and lookup match regardless of
    // case ("mod+k" vs "Mod+K"). We keep the original spec.key in the value
    // for display purposes.
    const key = normalizeCombo(spec.key);
    set({ shortcuts: addOwned(get().shortcuts, pluginName, key, spec, "shortcut") });
  },
  removeShortcut(pluginName, key) {
    set({ shortcuts: removeOwned(get().shortcuts, pluginName, normalizeCombo(key)) });
  },

  addComposerStatus(pluginName, spec) {
    set({
      composerStatus: addOwned(get().composerStatus, pluginName, spec.id, spec, "composer status"),
    });
  },
  removeComposerStatus(pluginName, id) {
    set({ composerStatus: removeOwned(get().composerStatus, pluginName, id) });
  },

  addComposerMode(pluginName, spec) {
    set({
      composerModes: addOwned(get().composerModes, pluginName, spec.id, spec, "composer mode"),
    });
  },
  removeComposerMode(pluginName, id) {
    set({ composerModes: removeOwned(get().composerModes, pluginName, id) });
  },

  addComposerPlaceholder(pluginName, spec) {
    set({
      composerPlaceholders: addOwned(
        get().composerPlaceholders,
        pluginName,
        spec.id,
        spec,
        "composer placeholder",
      ),
    });
  },
  removeComposerPlaceholder(pluginName, id) {
    set({ composerPlaceholders: removeOwned(get().composerPlaceholders, pluginName, id) });
  },

  addComposerAttachmentSource(pluginName, spec) {
    set({
      composerAttachmentSources: addOwned(
        get().composerAttachmentSources,
        pluginName,
        spec.id,
        spec,
        "composer attachment source",
      ),
    });
  },
  removeComposerAttachmentSource(pluginName, id) {
    set({
      composerAttachmentSources: removeOwned(get().composerAttachmentSources, pluginName, id),
    });
  },

  addComposerKeyBinding(pluginName, spec) {
    // Normalize the key on the way in so registrations and lookups match
    // regardless of case ("Enter" vs "enter", "Cmd+Enter" vs "mod+enter").
    const key = normalizeCombo(spec.key);
    set({
      composerKeyBindings: addOwned(
        get().composerKeyBindings,
        pluginName,
        key,
        spec,
        "composer key binding",
      ),
    });
  },
  removeComposerKeyBinding(pluginName, key) {
    set({
      composerKeyBindings: removeOwned(get().composerKeyBindings, pluginName, normalizeCombo(key)),
    });
  },

  addSidebarSection(pluginName, spec) {
    set({
      sidebarSections: addOwned(
        get().sidebarSections,
        pluginName,
        spec.id,
        spec,
        "sidebar section",
      ),
    });
  },
  removeSidebarSection(pluginName, id) {
    set({ sidebarSections: removeOwned(get().sidebarSections, pluginName, id) });
  },

  addAgentSource(pluginName, spec) {
    set({ agentSources: addOwned(get().agentSources, pluginName, spec.id, spec, "agent source") });
  },
  removeAgentSource(pluginName, id) {
    set({ agentSources: removeOwned(get().agentSources, pluginName, id) });
  },

  addCommand(pluginName, spec) {
    set({ commands: addOwned(get().commands, pluginName, spec.id, spec, "command") });
  },
  removeCommand(pluginName, id) {
    set({ commands: removeOwned(get().commands, pluginName, id) });
  },

  addDeclaredCommand(pluginName, spec) {
    set({
      declaredCommands: addOwned(
        get().declaredCommands,
        pluginName,
        spec.id,
        spec,
        "declared command",
      ),
    });
  },
  removeDeclaredCommand(pluginName, id) {
    set({ declaredCommands: removeOwned(get().declaredCommands, pluginName, id) });
  },
  removeDeclaredCommandsBy(pluginName) {
    set({ declaredCommands: clearByPlugin(get().declaredCommands, pluginName) });
  },

  addDeclaredView(pluginName, spec) {
    set({
      declaredViews: addOwned(get().declaredViews, pluginName, spec.id, spec, "declared view"),
    });
  },
  removeDeclaredViewsBy(pluginName) {
    set({ declaredViews: clearByPlugin(get().declaredViews, pluginName) });
  },

  addDeclaredSettingsPane(pluginName, spec) {
    set({
      declaredSettingsPanes: addOwned(
        get().declaredSettingsPanes,
        pluginName,
        spec.id,
        spec,
        "declared settings pane",
      ),
    });
  },
  removeDeclaredSettingsPanesBy(pluginName) {
    set({ declaredSettingsPanes: clearByPlugin(get().declaredSettingsPanes, pluginName) });
  },

  addPendingActivation(spec, events) {
    const next = new Map(get().pendingActivations);
    next.set(spec.name, { spec, events });
    set({ pendingActivations: next });
  },
  removePendingActivation(name) {
    const next = new Map(get().pendingActivations);
    next.delete(name);
    set({ pendingActivations: next });
  },

  addDataProvider(pluginName, spec) {
    set({
      dataProviders: addOwned(get().dataProviders, pluginName, spec.key, spec, "data provider"),
    });
  },
  removeDataProvider(pluginName, key) {
    set({ dataProviders: removeOwned(get().dataProviders, pluginName, key) });
  },

  addSidebarRailItem(pluginName, spec) {
    set({
      sidebarRailItems: addOwned(
        get().sidebarRailItems,
        pluginName,
        spec.id,
        spec,
        "sidebar rail item",
      ),
    });
  },
  removeSidebarRailItem(pluginName, id) {
    set({ sidebarRailItems: removeOwned(get().sidebarRailItems, pluginName, id) });
  },

  addMessageRole(pluginName, spec) {
    set({ messageRoles: addOwned(get().messageRoles, pluginName, spec.id, spec, "message role") });
  },
  removeMessageRole(pluginName, id) {
    set({ messageRoles: removeOwned(get().messageRoles, pluginName, id) });
  },

  // RPC hooks, log subscribers, lifecycle, plugin observers — every
  // composite-key slot below has the exact same add/remove shape now,
  // factored through `addOwnedMulti` / `removeOwnedMulti`.
  addRpcBeforeRequest(pluginName, id, hook) {
    set({ rpcBeforeRequest: addOwnedMulti(get().rpcBeforeRequest, pluginName, id, hook) });
  },
  removeRpcBeforeRequest(pluginName, id) {
    set({ rpcBeforeRequest: removeOwnedMulti(get().rpcBeforeRequest, pluginName, id) });
  },
  addRpcAfterResponse(pluginName, id, hook) {
    set({ rpcAfterResponse: addOwnedMulti(get().rpcAfterResponse, pluginName, id, hook) });
  },
  removeRpcAfterResponse(pluginName, id) {
    set({ rpcAfterResponse: removeOwnedMulti(get().rpcAfterResponse, pluginName, id) });
  },

  addLogSubscriber(pluginName, id, fn) {
    set({ logSubscribers: addOwnedMulti(get().logSubscribers, pluginName, id, fn) });
  },
  removeLogSubscriber(pluginName, id) {
    set({ logSubscribers: removeOwnedMulti(get().logSubscribers, pluginName, id) });
  },

  addReadyHandler(pluginName, id, fn) {
    set({ readyHandlers: addOwnedMulti(get().readyHandlers, pluginName, id, fn) });
  },
  removeReadyHandler(pluginName, id) {
    set({ readyHandlers: removeOwnedMulti(get().readyHandlers, pluginName, id) });
  },

  addBeforeUnloadHandler(pluginName, id, fn) {
    set({ beforeUnloadHandlers: addOwnedMulti(get().beforeUnloadHandlers, pluginName, id, fn) });
  },
  removeBeforeUnloadHandler(pluginName, id) {
    set({ beforeUnloadHandlers: removeOwnedMulti(get().beforeUnloadHandlers, pluginName, id) });
  },

  markAppReady() {
    if (get().appReady) return;
    set({ appReady: true });
    // Fire each handler — isolated; one throw must not skip the rest.
    for (const o of get().readyHandlers.values()) {
      safeCall(() => o.value(), `[plugin] ${o.pluginName} onReady threw:`);
    }
  },

  addPluginLoadListener(pluginName, id, fn) {
    set({ pluginLoadListeners: addOwnedMulti(get().pluginLoadListeners, pluginName, id, fn) });
  },
  removePluginLoadListener(pluginName, id) {
    set({ pluginLoadListeners: removeOwnedMulti(get().pluginLoadListeners, pluginName, id) });
  },
  addPluginUnloadListener(pluginName, id, fn) {
    set({ pluginUnloadListeners: addOwnedMulti(get().pluginUnloadListeners, pluginName, id, fn) });
  },
  removePluginUnloadListener(pluginName, id) {
    set({ pluginUnloadListeners: removeOwnedMulti(get().pluginUnloadListeners, pluginName, id) });
  },

  addPluginErrorFallback(pluginName, spec) {
    set({
      pluginErrorFallbacks: addOwned(
        get().pluginErrorFallbacks,
        pluginName,
        spec.id,
        spec,
        "plugin error fallback",
      ),
    });
  },
  removePluginErrorFallback(pluginName, id) {
    set({ pluginErrorFallbacks: removeOwned(get().pluginErrorFallbacks, pluginName, id) });
  },

  setWindowTitle(text) {
    set({ windowTitle: text });
    syncDocumentTitle(text, get().windowBadge);
  },
  setWindowBadge(n) {
    set({ windowBadge: n });
    syncDocumentTitle(get().windowTitle, n);
  },

  addWorkspaceView(pluginName, spec) {
    set({
      workspaceViews: addOwned(get().workspaceViews, pluginName, spec.id, spec, "workspace view"),
    });
  },
  removeWorkspaceView(pluginName, id) {
    set({ workspaceViews: removeOwned(get().workspaceViews, pluginName, id) });
  },

  resetForTest() {
    set(freshState());
  },
}));

// Side-effect: keep `document.title` in sync with the registry's title +
// badge. Run only in DOM environments — test runs without a document just
// skip the assignment.
function syncDocumentTitle(base: string, badge: number): void {
  if (typeof document === "undefined") return;
  const prefix = badge > 0 ? `(${badge}) ` : "";
  document.title = `${prefix}${base || "Lyra"}`;
}

// Normalize "Cmd+K" / "cmd+K" / "Mod+k" to a canonical "mod+k" form. Mod
// collapses Cmd (Mac) / Ctrl (others) so registrations are cross-platform
// by default. The leftmost key segment is the modifier; the last is the key.
function normalizeCombo(combo: string): string {
  const parts = combo.split("+").map((p) => p.trim().toLowerCase());
  const key = parts.pop() ?? "";
  const mods = new Set<string>();
  for (const p of parts) {
    if (p === "cmd" || p === "meta" || p === "mod") mods.add("mod");
    else if (p === "ctrl" || p === "control") mods.add("ctrl");
    else if (p === "shift") mods.add("shift");
    else if (p === "alt" || p === "option") mods.add("alt");
    else mods.add(p);
  }
  // Stable order: mod, ctrl, alt, shift — matches common docs convention.
  const order = ["mod", "ctrl", "alt", "shift"];
  const sortedMods = order.filter((m) => mods.has(m));
  return [...sortedMods, key].join("+");
}

export { normalizeCombo };
