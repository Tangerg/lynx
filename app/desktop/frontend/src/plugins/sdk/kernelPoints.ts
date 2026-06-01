// Kernel extension points — the typed handles for every contribution
// surface the kernel itself owns. Each `host.X.register*` facade and its
// matching selector route through one of these onto the shared `extensions`
// substrate, so built-in contributions and third-party ones use the exact
// same mechanism (the JetBrains "kernel is just another extension consumer"
// property).
//
// Points are migrated domain-by-domain (L3); this file grows one block per
// migrated domain. `single` = one entry per `keyOf` (override + warn);
// `multi` = every contribution coexists.

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
  LayoutSlotSpec,
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
import { defineExtensionPoint } from "./defineExtensionPoint";
import { LIFECYCLE_POINT_IDS } from "./pointIds";
import { normalizeCombo } from "./registry";

// ---- theme domain --------------------------------------------------------
export const THEME = defineExtensionPoint<ThemeSpec>({ id: "lyra.theme", keying: "single" });
export const ACCENT = defineExtensionPoint<ThemeAccentSpec>({
  id: "lyra.accent",
  keying: "single",
});
export const LOCALE = defineExtensionPoint<LocaleSpec>({ id: "lyra.locale", keying: "single" });

// ---- runtime / data-layer domain -----------------------------------------
export const ROUTE = defineExtensionPoint<RouteSpec>({ id: "lyra.route", keying: "single" });
export const AGENT_SOURCE = defineExtensionPoint<AgentSourceSpec>({
  id: "lyra.agent.source",
  keying: "single",
});
export const DATA_PROVIDER = defineExtensionPoint<DataProviderSpec>({
  id: "lyra.data.provider",
  keying: "single",
  keyOf: (s) => s.key,
});
export const ERROR_FALLBACK = defineExtensionPoint<PluginErrorFallbackSpec>({
  id: "lyra.plugin.errorFallback",
  keying: "single",
});

// ---- composer domain ------------------------------------------------------
export const COMPOSER_PLACEHOLDER = defineExtensionPoint<ComposerPlaceholderSpec>({
  id: "lyra.composer.placeholder",
  keying: "single",
});
export const COMPOSER_STATUS = defineExtensionPoint<ComposerStatusSpec>({
  id: "lyra.composer.status",
  keying: "single",
});
export const COMPOSER_MODE = defineExtensionPoint<ComposerModeSpec>({
  id: "lyra.composer.mode",
  keying: "single",
});
export const COMPOSER_ATTACHMENT_SOURCE = defineExtensionPoint<ComposerAttachmentSourceSpec>({
  id: "lyra.composer.attachmentSource",
  keying: "single",
});
// Slash trigger lives in the map key, not on the spec — contributors pass it
// via `opts.key`. `normalizeKey` folds the leading "/" so callers can register
// "ping" or "/ping" and look it up either way.
export const SLASH_COMMAND = defineExtensionPoint<SlashCommandSpec>({
  id: "lyra.composer.slashCommand",
  keying: "single",
  normalizeKey: (k) => (k.startsWith("/") ? k : `/${k}`),
});
// Key combos fold "Cmd+K" / "mod+k" to one canonical form on both contribute
// and lookup, so registrations and keydown lookups always agree.
export const COMPOSER_KEY_BINDING = defineExtensionPoint<ComposerKeyBindingSpec>({
  id: "lyra.composer.keyBinding",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

// ---- shortcuts domain -----------------------------------------------------
export const SHORTCUT = defineExtensionPoint<ShortcutSpec>({
  id: "lyra.shortcut",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

// ---- declared-merge surfaces ----------------------------------------------
// The "registered" half of the registered+declared-placeholder merge. The
// declared half (contributes.* placeholders awaiting activation) keeps its own
// named map; the selectors merge the two (registered wins on id collision).
export const COMMAND = defineExtensionPoint<CommandSpec>({ id: "lyra.command", keying: "single" });
export const SETTINGS_PANE = defineExtensionPoint<SettingsPaneSpec>({
  id: "lyra.settingsPane",
  keying: "single",
});
export const WORKSPACE_VIEW = defineExtensionPoint<WorkspaceViewSpec>({
  id: "lyra.workspaceView",
  keying: "single",
});

// ---- multi-handler surfaces (every contribution coexists, runs in order) --
export const RPC_BEFORE_REQUEST = defineExtensionPoint<RpcBeforeRequestHook>({
  id: "lyra.rpc.beforeRequest",
  keying: "multi",
});
export const RPC_AFTER_RESPONSE = defineExtensionPoint<RpcAfterResponseHook>({
  id: "lyra.rpc.afterResponse",
  keying: "multi",
});
export const LOG_SUBSCRIBER = defineExtensionPoint<LogSubscriber>({
  id: "lyra.log.subscriber",
  keying: "multi",
});

// ---- lifecycle hooks ------------------------------------------------------
// Fired from inside the registry store (markAppReady / registerLoaded /
// unload), so their ids live in `pointIds.ts` — the store filters `extensions`
// by these while staying ignorant of the typed handles defined here.
export const READY_HANDLER = defineExtensionPoint<ReadyHandler>({
  id: LIFECYCLE_POINT_IDS.ready,
  keying: "multi",
});
export const BEFORE_UNLOAD_HANDLER = defineExtensionPoint<BeforeUnloadHandler>({
  id: LIFECYCLE_POINT_IDS.beforeUnload,
  keying: "multi",
});
export const PLUGIN_LOAD_LISTENER = defineExtensionPoint<(spec: PluginSpec) => void>({
  id: LIFECYCLE_POINT_IDS.pluginLoad,
  keying: "multi",
});
export const PLUGIN_UNLOAD_LISTENER = defineExtensionPoint<(name: string) => void>({
  id: LIFECYCLE_POINT_IDS.pluginUnload,
  keying: "multi",
});

// ---- AG-UI events + layout (multi, sub-keyed by name / type / slot) -------
// The item wraps its sub-key (name / eventType / slot) alongside the payload;
// the events + layout selectors build a cached secondary index over it (see
// `createPointSubIndex`). The reducer hits these per AG-UI event.
export const CUSTOM_EVENT_HANDLER = defineExtensionPoint<{
  name: string;
  handler: CustomEventHandler<unknown>;
}>({ id: "lyra.agui.customEvent", keying: "multi" });
export const CORE_EVENT_HANDLER = defineExtensionPoint<{
  eventType: string;
  handler: CoreEventHandler;
}>({ id: "lyra.agui.coreEvent", keying: "multi" });
export const LAYOUT_SLOT = defineExtensionPoint<{ slot: string; spec: LayoutSlotSpec }>({
  id: "lyra.layoutSlot",
  keying: "multi",
});

// ---- sidebar domain -------------------------------------------------------
export const SIDEBAR_SECTION = defineExtensionPoint<SidebarSectionSpec>({
  id: "lyra.sidebar.section",
  keying: "single",
});
export const SIDEBAR_RAIL_ITEM = defineExtensionPoint<SidebarRailItemSpec>({
  id: "lyra.sidebar.railItem",
  keying: "single",
});

// ---- message / tool domain ------------------------------------------------
export const MESSAGE_ROLE = defineExtensionPoint<MessageRoleSpec>({
  id: "lyra.message.role",
  keying: "single",
});
export const TOOL_ACTION = defineExtensionPoint<ToolActionSpec>({
  id: "lyra.tool.action",
  keying: "single",
});
// Keyed by an explicit arg (tool fn name / block kind), not a field on the
// item — contributors pass `opts.key`. The item is the renderer/component
// itself (or, for icons, the icon name string).
export const TOOL_PREVIEW = defineExtensionPoint<ToolPreviewComponent>({
  id: "lyra.tool.preview",
  keying: "single",
});
export const TOOL_ICON = defineExtensionPoint<string>({
  id: "lyra.tool.icon",
  keying: "single",
});
export const CONTENT_BLOCK = defineExtensionPoint<ContentBlockRenderer<ContentBlockKind>>({
  id: "lyra.message.contentBlock",
  keying: "single",
});
