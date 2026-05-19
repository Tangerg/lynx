// Public SDK surface — what plugin authors import.

export { definePlugin, loadPlugin, loadPlugins } from "./definePlugin";

export {
  listRoutes,
  lookupAccent,
  lookupCommand,
  lookupDataProvider,
  lookupCoreEventHandlers,
  lookupCustomEventHandler,
  lookupShortcut,
  lookupSlashCommand,
  lookupTheme,
  normalizeCombo,
  pickAgentSource,
  pickComposerPlaceholder,
  pickPluginErrorFallback,
  useAccents,
  useCommands,
  useComposerAttachmentSources,
  useComposerModes,
  useComposerStatus,
  useContentBlockRenderer,
  listLogSubscribers,
  listRpcAfterHooks,
  listRpcBeforeHooks,
  lookupComposerKeyBinding,
  lookupToolIcon,
  useInspectorTabs,
  useLayoutSlot,
  useMessageRole,
  usePluginStore,
  useWorkspaceViews,
  listWorkspaceViews,
  useSettingsPanes,
  useSidebarRailItems,
  useSidebarSections,
  useSlashCommands,
  useThemes,
  useToolActions,
  useToolPreview,
} from "./registry";

export type {
  AgentSourceSpec,
  CommandSpec,
  ComposerAttachment,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerKeyContext,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
  DataProviderSpec,
  ContentBlockRenderer,
  ContentBlockRendererProps,
  CoreEventHandler,
  CustomEventHandler,
  Disposable,
  Host,
  InspectorTabSpec,
  BeforeUnloadHandler,
  LayoutSlotSpec,
  LoadedPlugin,
  LogEvent,
  LogLevel,
  LogSubscriber,
  MessageRoleSpec,
  PluginErrorFallbackProps,
  PluginErrorFallbackSpec,
  ReadyHandler,
  PluginContext,
  PluginSpec,
  RouteSpec,
  SettingsPaneSpec,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
  ShortcutHandler,
  ShortcutSpec,
  SidebarRailItemSpec,
  SidebarSectionSpec,
  SlashCommandRunCtx,
  SlashCommandSpec,
  WorkspaceViewSpec,
  DockLocation,
  StateUpdate,
  ThemeAccentSpec,
  ThemeSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  ToolPreviewProps,
} from "./types";

export {
  appendBlockToLatestAssistant,
  appendBlockToMessage,
  compose,
  patchRun,
  setPlan,
} from "./state";

export { PLUGIN_TOAST_EVENT, type PluginToastDetail } from "./host";

// Phase 3 — error aggregation.
export {
  reportPluginError,
  usePluginErrorStore,
  type PluginError,
  type PluginErrorSource,
} from "./errors";

// Phase 16 — persistent notification feed.
export { useNotificationStore } from "./notifications";
export type { NotificationEntry, NotificationLevel } from "./types";

// Phase 17 — shared cross-plugin state slices.
export { getOrCreateSlice } from "./stateSlice";
export type { StateSlice } from "./stateSlice";

// Phase 19 — app-wide config store.
export {
  getConfig,
  hasConfig,
  setConfig,
  useConfigStore,
} from "./config";
export type { ConfigValue } from "./config";

// Phase 21 — storage migrations (live in storage.ts since they're per-plugin).
export type { KeyValueStore, StorageMigration } from "./storage";

// Phase 22 — per-message context + slots. Re-export from the chat
// component so plugin authors only ever import from `@/plugins/sdk`.
export { useCurrentMessage } from "@/components/chat/MessageContext";
