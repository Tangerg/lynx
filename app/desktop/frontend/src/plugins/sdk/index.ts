// Public SDK surface — what plugin authors import.

export {
  definePlugin,
  loadPlugin,
  loadPlugins,
  unloadPlugin,
  reloadPlugin,
} from "./definePlugin";

// The Zustand store + write-side actions live in registry.ts.
export { normalizeCombo, usePluginStore } from "./registry";

// Read-side selectors / imperative lookups for every surface.
export {
  listRoutes,
  listRpcAfterHooks,
  listRpcBeforeHooks,
  lookupAccent,
  lookupCommand,
  lookupComposerKeyBinding,
  lookupCoreEventHandlers,
  lookupCustomEventHandler,
  lookupDataProvider,
  lookupShortcut,
  lookupSlashCommand,
  lookupTheme,
  lookupToolIcon,
  resolveScheme,
  pickAgentSource,
  pickComposerPlaceholder,
  pickPluginErrorFallback,
  useAccents,
  useCommands,
  useComposerAttachmentSources,
  useComposerModes,
  useComposerStatus,
  useContentBlockRenderer,
  useLayoutSlot,
  useMessageRole,
  useSettingsPanes,
  useSidebarRailItems,
  useSidebarSections,
  useSlashCommands,
  useThemes,
  useToolActions,
  useToolPreview,
  useWorkspaceViews,
} from "./selectors";

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

// Plugin error aggregation.
export {
  reportPluginError,
  usePluginErrorStore,
  type PluginError,
  type PluginErrorSource,
} from "./errors";

// Persistent notification feed.
export { useNotificationStore } from "./notifications";
export type { NotificationEntry, NotificationLevel } from "./types";

// Shared cross-plugin state slices.
export { getOrCreateSlice } from "./stateSlice";
export type { StateSlice } from "./stateSlice";

// Backend-driven shared state — AG-UI STATE_SNAPSHOT / STATE_DELTA.
export { useSharedState } from "./sharedState";

// App-wide config store.
export {
  getConfig,
  hasConfig,
  setConfig,
  useConfigStore,
} from "./config";
export type { ConfigValue } from "./config";

// Storage migrations (live in storage.ts since they're per-plugin).
export type { KeyValueStore, StorageMigration } from "./storage";

// `when` clause evaluator + context shape — exposed so plugin command
// consumers (palette, future menu providers) can filter declarative
// commands consistently.
export { evalWhen } from "./evalWhen";
export type { WhenContext } from "./evalWhen";

// Per-message context + slots. Re-export from the chat component so
// plugin authors only ever import from `@/plugins/sdk`.
export { useCurrentMessage } from "@/components/chat/MessageContext";
