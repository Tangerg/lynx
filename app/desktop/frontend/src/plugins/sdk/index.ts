// Public SDK surface — the one door into the plugin system. Plugin authors AND
// the kernel's own built-ins import from here; there is no privileged back door.
// Contributions are written with `host.extensions.contribute(POINT, …)` (POINT
// from kernelPoints, re-exported below) and read with the generic substrate
// hooks (useExtensionPoint / lookupExtensionByKey / …). The named selectors are
// only the few reads that add real logic on top of a plain read.

// App-wide config store.
export { getConfig, hasConfig, setConfig, useConfigStore } from "./config";

export type { ConfigValue } from "./config";

export { definePlugin } from "./definePlugin";
export { createKernel, startKernel, stopKernel } from "./bootstrap";
export type { Contributor, PluginContext, PluginSpec } from "./definePlugin";
export { contributeLayout } from "./contributeHelpers";
export type { Contribution } from "./contracts";
export {
  contributionsTo,
  subscribeContributions,
  useContributions,
  useInstalledPlugins,
} from "./kernel";
export { COMMANDS, CONFIG, I18N, WINDOW, WORKSPACE } from "./services";
export type {
  AmbientShell,
  CommandsService,
  ConfigService,
  I18nService,
  WindowService,
  WorkspaceService,
} from "./services";

// Built-in kernel points (COLOR_THEME / COMMAND / LAYOUT_SLOT / …).
export * from "./kernelPoints";

// Plugin error aggregation.
export {
  type PluginError,
  type PluginErrorSource,
  reportPluginError,
  usePluginErrorStore,
} from "./errors";

// Persistent notification feed + the app-side notify pair that writes to it.
export { notifyError, notifyInfo, useNotificationStore } from "./notifications";
export type { NotifyOptions, NotifySource } from "./notifications";
// Cached read hooks over a contributed data provider.
export { createDataQuery, createParameterizedDataQuery } from "./dataQuery";
export type { ParameterizedQueryOptions } from "./dataQuery";
// The registry store — imperative observation of contributions (subscribe /
// getState). `normalizeCombo` + the toast-event contract stay internal to
// `./registry` / `./hostToast` (plugins don't need them — points normalize combos
// on contribute, and toasts go through `host.notify`).

// Read side. Plain reads use the generic substrate (use/lookupExtensionPoint,
// use/lookupExtensionByKey); the rest are selectors with real logic.
export {
  executeCommand,
  lookupStreamHandlers,
  lookupDataProvider,
  lookupExtensionByKey,
  lookupExtensionOwner,
  lookupExtensionPoint,
  lookupSlashCommandOwner,
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  pickAgentSource,
  pickPluginErrorFallback,
  resolveAgentRunStartOptions,
  useExtensionByKey,
  useExtensionPoint,
  useContextDockDestinations,
  useLayoutSlot,
  useWorkIndexItems,
  useSettingsPanes,
  useSlashCommands,
  useWorkspaceViews,
} from "./selectors";
export { appendTimelineEntry } from "./types/agentTimeline";

export type { KeyValueStore } from "./storage";
export type { AgentMessagePhase, AgentPlan, PlanStep } from "./types/agentSessionView";

export type {
  AgentDriver,
  AgentCancelResult,
  AgentEventEnvelope,
  AgentInterrupt,
  AgentItem,
  AgentItemDelta,
  AgentItemStatus,
  AgentMessagePart,
  AgentPendingInterruptSet,
  AgentQuestion,
  AgentQuestionField,
  AgentQuestionOption,
  AgentRunFact,
  AgentSegmentOutcome,
  AgentStreamEvent,
  AgentToolInvocation,
  AgentRunOptionsProviderSpec,
  AgentRunStartOptions,
  AgentSourceSpec,
  CommandSpec,
  ComposerKeyBindingSpec,
  ComposerKeyContext,
  ComposerSubmitModeContext,
  ComposerSubmitModeDraft,
  ComposerSubmitModeSpec,
  ContextDockDestinationScope,
  ContextDockDestinationSpec,
  ContentBlock,
  StreamEventHandler,
  DataProviderSpec,
  Disposable,
  ExtensionContributionOptions,
  ExtensionKeying,
  ExtensionPoint,
  LayoutSlotSpec,
  LogEvent,
  LogLevel,
  LogSubscriber,
  MessageRoleSpec,
  PluginErrorFallbackProps,
  PluginErrorFallbackSpec,
  ReadyHandler,
  RouteSpec,
  SettingsPaneSpec,
  ShortcutHandler,
  ShortcutSpec,
  SlashCommandRunCtx,
  SlashCommandSpec,
  ColorThemeSpec,
  NeutralStep,
  ThemeNeutralSteps,
  AccentSpec,
  VisualStyleSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  ToolPreviewProps,
  ToolViewOpenerSpec,
  WorkIndexItemScope,
  WorkIndexItemSpec,
  WorkIndexItemVariant,
  WorkspaceViewSpec,
} from "./types";
export type { NotificationEntry, NotificationLevel } from "./types";

// Per-message context hooks. The context + hooks live in the SDK (the
// plugin-integration seam) so plugin authors only ever import from
// `@/plugins/sdk`; kernel UI (`MessageBlock`) imports the Provider from
// `./messageContext` directly.
export { useCurrentMessage, useCurrentMessageSessionId } from "./messageContext";
