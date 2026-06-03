// Public SDK surface — the one door into the plugin system. Plugin authors AND
// the kernel's own built-ins import from here; there is no privileged back door.
// Contributions are written with `host.extensions.contribute(POINT, …)` (POINT
// from kernelPoints, re-exported below) and read with the generic substrate
// hooks (useExtensionPoint / lookupExtensionByKey / …). The named selectors are
// only the few reads that add real logic on top of a plain read.

// App-wide config store.
export { getConfig, hasConfig, setConfig, useConfigStore } from "./config";

export type { ConfigValue } from "./config";

export { definePlugin, loadPlugin, loadPlugins, reloadPlugin, unloadPlugin } from "./definePlugin";
export { definePluginPack } from "./definePluginPack";

// Open extension points — the JetBrains-style substrate: a plugin defines a
// typed point, any plugin contributes, any plugin consumes.
export { defineExtensionPoint } from "./defineExtensionPoint";
// Built-in kernel points (THEME / COMMAND / LAYOUT_SLOT / …). Re-exported so
// sideload bundles — which only see the SDK via `window.__LYRA__.SDK` — can
// contribute to kernel surfaces, the same way built-ins do via the deep path.
export * from "./kernelPoints";

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

// Persistent notification feed.
export { useNotificationStore } from "./notifications";
// The registry store — imperative observation of contributions (subscribe /
// getState). `normalizeCombo` + the toast-event contract stay internal to
// `./registry` / `./host` (plugins don't need them — points normalize combos
// on contribute, and toasts go through `host.notify`).
export { usePluginStore } from "./registry";

// Read side. Plain reads use the generic substrate (use/lookupExtensionPoint,
// use/lookupExtensionByKey); the rest are selectors with real logic.
export {
  listRpcAfterHooks,
  listRpcBeforeHooks,
  lookupCommandOwner,
  lookupStreamHandlers,
  lookupCustomHandlers,
  lookupDataProvider,
  lookupExtensionByKey,
  lookupExtensionPoint,
  lookupSlashCommandOwner,
  lookupToolActionOwner,
  pickAgentSource,
  pickComposerPlaceholder,
  pickPluginErrorFallback,
  resolveScheme,
  useCitationSources,
  useCommands,
  useExtensionByKey,
  useExtensionPoint,
  useLayoutSlot,
  useSettingsPanes,
  useSlashCommands,
  useWorkspaceViews,
} from "./selectors";
// Backend-driven shared state — AG-UI STATE_SNAPSHOT / STATE_DELTA.
export { useSharedState } from "./sharedState";

export {
  appendBlockToLatestAssistant,
  appendBlockToMessage,
  appendTimelineEntry,
  compose,
  patchBlocksWhere,
  patchRun,
  setPlan,
} from "./state";

// Shared cross-plugin state slices.
export { getOrCreateSlice } from "./stateSlice";
export type { StateSlice } from "./stateSlice";

// Storage migrations (live in storage.ts since they're per-plugin).
export type { KeyValueStore, StorageMigration } from "./storage";

export type {
  AgentDriver,
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
  Citation,
  CitationSource,
  ContentBlockRenderer,
  ContentBlockRendererProps,
  StreamEventHandler,
  CustomEventHandler,
  DataProviderSpec,
  Disposable,
  DockLocation,
  ExtensionContributionOptions,
  ExtensionKeying,
  ExtensionPoint,
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
  PluginPackSpec,
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

// Per-message context hook. The context + hook live in the SDK (the
// plugin-integration seam) so plugin authors only ever import from
// `@/plugins/sdk`; kernel UI (`MessageBlock`) imports the Provider from
// `./messageContext` directly.
export { useCurrentMessage } from "./messageContext";
