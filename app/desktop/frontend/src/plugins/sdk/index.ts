// Public SDK surface — what plugin authors import.

// App-wide config store.
export { getConfig, hasConfig, setConfig, useConfigStore } from "./config";

export type { ConfigValue } from "./config";

export { definePlugin, loadPlugin, loadPlugins, reloadPlugin, unloadPlugin } from "./definePlugin";

// Plugin error aggregation.
export {
  type PluginError,
  type PluginErrorSource,
  reportPluginError,
  usePluginErrorStore,
} from "./errors";

// `when` clause evaluator + context shape — exposed so plugin command
// consumers (palette, future menu providers) can filter declarative
// commands consistently.
export { evalWhen } from "./evalWhen";

export type { WhenContext } from "./evalWhen";

export { PLUGIN_TOAST_EVENT, type PluginToastDetail } from "./host";

// Persistent notification feed.
export { useNotificationStore } from "./notifications";
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
  lookupCustomEventHandlers,
  lookupDataProvider,
  lookupShortcut,
  lookupSlashCommand,
  lookupTheme,
  lookupToolIcon,
  pickAgentSource,
  pickComposerPlaceholder,
  pickPluginErrorFallback,
  resolveScheme,
  useAccents,
  useCommands,
  useComposerAttachmentSources,
  useComposerModes,
  useComposerStatus,
  useContentBlockRenderer,
  useLayoutSlot,
  useLocales,
  useMessageRole,
  useSettingsPanes,
  useShortcuts,
  useSidebarRailItems,
  useSidebarSections,
  useSlashCommands,
  useThemes,
  useToolActions,
  useToolPreview,
  useWorkspaceViews,
} from "./selectors";
// Backend-driven shared state — AG-UI STATE_SNAPSHOT / STATE_DELTA.
export { useSharedState } from "./sharedState";

export {
  appendBlockToLatestAssistant,
  appendBlockToMessage,
  appendTimelineEntry,
  compose,
  patchRun,
  setPlan,
} from "./state";

// Shared cross-plugin state slices.
export { getOrCreateSlice } from "./stateSlice";
export type { StateSlice } from "./stateSlice";

// Storage migrations (live in storage.ts since they're per-plugin).
export type { KeyValueStore, StorageMigration } from "./storage";

export type {
  AgentSourceSpec,
  BeforeUnloadHandler,
  CommandSpec,
  ComposerAttachment,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerKeyContext,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
  ContentBlockRenderer,
  ContentBlockRendererProps,
  CoreEventHandler,
  CustomEventHandler,
  DataProviderSpec,
  Disposable,
  DockLocation,
  Host,
  LayoutSlotSpec,
  LoadedPlugin,
  LogEvent,
  LogLevel,
  LogSubscriber,
  MessageRoleSpec,
  PluginContext,
  PluginErrorFallbackProps,
  PluginErrorFallbackSpec,
  PluginSpec,
  ReadyHandler,
  RouteSpec,
  RpcAfterResponseHook,
  RpcBeforeRequestHook,
  SettingsPaneSpec,
  ShortcutHandler,
  ShortcutSpec,
  SidebarRailItemSpec,
  SidebarSectionSpec,
  SlashCommandRunCtx,
  SlashCommandSpec,
  StateUpdate,
  ThemeAccentSpec,
  ThemeSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  ToolPreviewProps,
  WorkspaceViewSpec,
} from "./types";
export type { NotificationEntry, NotificationLevel } from "./types";

// Per-message context + slots. Re-export from the chat component so
// plugin authors only ever import from `@/plugins/sdk`.
export { useCurrentMessage } from "@/components/chat/MessageContext";
