// Kernel extension points — the typed handles for every contribution surface
// the kernel itself owns. Built-ins contribute via `host.extensions.contribute
// (POINT, …)` (or one of the few retained thin facades), third parties the same
// way, and the per-domain selectors read these — so kernel and third-party
// contributions use the exact same mechanism (the JetBrains "kernel is just
// another extension consumer" property).
//
// Adding a kernel point = one `defineExtensionPoint` block here + one selector.

import type {
  AgentSourceSpec,
  BeforeUnloadHandler,
  CommandSpec,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
  ContentBlockRenderer,
  StreamEventHandler,
  CustomEventHandler,
  DataProviderSpec,
  LayoutSlotSpec,
  LocaleSpec,
  LogSubscriber,
  CitationSource,
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
import type { ContentBlockKind } from "@/protocol/run/viewState";
import { defineExtensionPoint } from "./defineExtensionPoint";
import { LIFECYCLE_POINT_IDS } from "./pointIds";
import { normalizeCombo } from "./registry";

export const THEME = defineExtensionPoint<ThemeSpec>({
  id: "lyra.theme",
  capability: "theme",
  keying: "single",
});
export const ACCENT = defineExtensionPoint<ThemeAccentSpec>({
  id: "lyra.accent",
  capability: "theme",
  keying: "single",
});
export const LOCALE = defineExtensionPoint<LocaleSpec>({
  id: "lyra.locale",
  capability: "i18n",
  keying: "single",
});

export const ROUTE = defineExtensionPoint<RouteSpec>({
  id: "lyra.route",
  capability: "router",
  keying: "single",
});
export const AGENT_SOURCE = defineExtensionPoint<AgentSourceSpec>({
  id: "lyra.agent.source",
  capability: "agent",
  keying: "single",
});
export const DATA_PROVIDER = defineExtensionPoint<DataProviderSpec>({
  id: "lyra.data.provider",
  capability: "data",
  keying: "single",
  keyOf: (s) => s.key,
});
export const ERROR_FALLBACK = defineExtensionPoint<PluginErrorFallbackSpec>({
  id: "lyra.plugin.errorFallback",
  capability: "plugins",
  keying: "single",
});

export const COMPOSER_PLACEHOLDER = defineExtensionPoint<ComposerPlaceholderSpec>({
  id: "lyra.composer.placeholder",
  capability: "composer",
  keying: "single",
});
export const COMPOSER_STATUS = defineExtensionPoint<ComposerStatusSpec>({
  id: "lyra.composer.status",
  capability: "composer",
  keying: "single",
});
export const COMPOSER_ATTACHMENT_SOURCE = defineExtensionPoint<ComposerAttachmentSourceSpec>({
  id: "lyra.composer.attachmentSource",
  capability: "composer",
  keying: "single",
});
// Slash trigger lives in the map key, not on the spec — contributors pass it
// via `opts.key`. `normalizeKey` folds the leading "/" so callers can register
// "ping" or "/ping" and look it up either way.
export const SLASH_COMMAND = defineExtensionPoint<SlashCommandSpec>({
  id: "lyra.composer.slashCommand",
  capability: "composer",
  keying: "single",
  normalizeKey: (k) => (k.startsWith("/") ? k : `/${k}`),
});
// Key combos fold "Cmd+K" / "mod+k" to one canonical form on both contribute
// and lookup, so registrations and keydown lookups always agree.
export const COMPOSER_KEY_BINDING = defineExtensionPoint<ComposerKeyBindingSpec>({
  id: "lyra.composer.keyBinding",
  capability: "composer",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

export const SHORTCUT = defineExtensionPoint<ShortcutSpec>({
  id: "lyra.shortcut",
  capability: "shortcuts",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

// The "registered" half of the registered+declared-placeholder merge. The
// declared half (contributes.* placeholders awaiting activation) keeps its own
// named map; the selectors merge the two (registered wins on id collision).
export const COMMAND = defineExtensionPoint<CommandSpec>({
  id: "lyra.command",
  capability: "commands",
  keying: "single",
});
export const SETTINGS_PANE = defineExtensionPoint<SettingsPaneSpec>({
  id: "lyra.settingsPane",
  capability: "settings",
  keying: "single",
});
export const WORKSPACE_VIEW = defineExtensionPoint<WorkspaceViewSpec>({
  id: "lyra.workspaceView",
  capability: "workspace",
  keying: "single",
});

// ---- multi-handler surfaces (every contribution coexists, runs in order) --
export const RPC_BEFORE_REQUEST = defineExtensionPoint<RpcBeforeRequestHook>({
  id: "lyra.rpc.beforeRequest",
  capability: "rpc",
  keying: "multi",
});
export const RPC_AFTER_RESPONSE = defineExtensionPoint<RpcAfterResponseHook>({
  id: "lyra.rpc.afterResponse",
  capability: "rpc",
  keying: "multi",
});
export const LOG_SUBSCRIBER = defineExtensionPoint<LogSubscriber>({
  id: "lyra.log.subscriber",
  capability: "log",
  keying: "multi",
});

// Fired from inside the registry store (markAppReady / registerLoaded /
// unload), so their ids live in `pointIds.ts` — the store filters `extensions`
// by these while staying ignorant of the typed handles defined here.
export const READY_HANDLER = defineExtensionPoint<ReadyHandler>({
  id: LIFECYCLE_POINT_IDS.ready,
  capability: "lifecycle",
  keying: "multi",
});
export const BEFORE_UNLOAD_HANDLER = defineExtensionPoint<BeforeUnloadHandler>({
  id: LIFECYCLE_POINT_IDS.beforeUnload,
  capability: "lifecycle",
  keying: "multi",
});
export const PLUGIN_LOAD_LISTENER = defineExtensionPoint<(spec: PluginSpec) => void>({
  id: LIFECYCLE_POINT_IDS.pluginLoad,
  capability: "plugins",
  keying: "multi",
});
export const PLUGIN_UNLOAD_LISTENER = defineExtensionPoint<(name: string) => void>({
  id: LIFECYCLE_POINT_IDS.pluginUnload,
  capability: "plugins",
  keying: "multi",
});

// The item wraps its sub-key (name / eventType / slot) alongside the payload;
// the events + layout selectors build a cached secondary index over it (see
// `createPointSubIndex`). The reducer hits these per StreamEvent.
export const CUSTOM_EVENT_HANDLER = defineExtensionPoint<{
  name: string;
  handler: CustomEventHandler<unknown>;
}>({ id: "lyra.events.custom", capability: "events", keying: "multi" });
export const STREAM_EVENT_HANDLER = defineExtensionPoint<{
  eventType: string;
  handler: StreamEventHandler;
}>({ id: "lyra.events.stream", capability: "events", keying: "multi" });
export const LAYOUT_SLOT = defineExtensionPoint<{ slot: string; spec: LayoutSlotSpec }>({
  id: "lyra.layoutSlot",
  capability: "layout",
  keying: "multi",
});

export const SIDEBAR_SECTION = defineExtensionPoint<SidebarSectionSpec>({
  id: "lyra.sidebar.section",
  capability: "sidebar",
  keying: "single",
});
export const SIDEBAR_RAIL_ITEM = defineExtensionPoint<SidebarRailItemSpec>({
  id: "lyra.sidebar.railItem",
  capability: "sidebar",
  keying: "single",
});

export const MESSAGE_ROLE = defineExtensionPoint<MessageRoleSpec>({
  id: "lyra.message.role",
  capability: "message",
  keying: "single",
});
export const TOOL_ACTION = defineExtensionPoint<ToolActionSpec>({
  id: "lyra.tool.action",
  capability: "tool",
  keying: "single",
});
// Keyed by an explicit arg (tool fn name / block kind), not a field on the
// item — contributors pass `opts.key`. The item is the renderer/component
// itself (or, for icons, the icon name string).
export const TOOL_PREVIEW = defineExtensionPoint<ToolPreviewComponent>({
  id: "lyra.tool.preview",
  capability: "tool",
  keying: "single",
});
export const TOOL_ICON = defineExtensionPoint<string>({
  id: "lyra.tool.icon",
  capability: "tool",
  keying: "single",
});
export const CONTENT_BLOCK = defineExtensionPoint<ContentBlockRenderer<ContentBlockKind>>({
  id: "lyra.message.contentBlock",
  capability: "message",
  keying: "single",
});
// Per-message citation sources — each maps the message's blocks to the
// citations they imply (multi: every contribution's output is concatenated).
// Keeps the kernel ignorant of which block kind carries sources, so a
// citation-producing feature (e.g. the search block) stays fully removable.
export const MESSAGE_CITATION_SOURCE = defineExtensionPoint<CitationSource>({
  id: "lyra.message.citationSource",
  capability: "message",
  keying: "multi",
});
