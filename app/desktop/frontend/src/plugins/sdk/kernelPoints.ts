// Kernel extension points — the typed handles for every contribution surface
// the kernel itself owns. Built-ins contribute via `host.extensions.contribute
// (POINT, …)` (or one of the few retained thin facades), third parties the same
// way, and the per-domain selectors read these — so kernel and third-party
// contributions use the exact same mechanism (the JetBrains "kernel is just
// another extension consumer" property).
//
// Adding a kernel point = one `defineExtensionPoint` block here + one selector.

import type {
  AgentRunOptionsProviderSpec,
  AgentSourceSpec,
  BeforeUnloadHandler,
  CommandSpec,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerSubmitModeSpec,
  ContextDockDestinationSpec,
  StreamEventHandler,
  DataProviderSpec,
  LayoutSlotSpec,
  LocaleSpec,
  LogSubscriber,
  CitationSource,
  MessageRoleSpec,
  PluginErrorFallbackSpec,
  ReadyHandler,
  RouteSpec,
  SettingsPaneSpec,
  ShortcutSpec,
  SlashCommandSpec,
  ColorThemeSpec,
  AccentSpec,
  VisualStyleSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  ToolViewOpenerSpec,
  WorkIndexItemSpec,
  WorkspaceViewSpec,
} from "./types";
import type { ReactNode } from "react";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import { defineExtensionPoint } from "./contracts";
import { LIFECYCLE_POINT_IDS } from "./pointIds";
import { normalizeCombo } from "./combo";

type RegisteredContentBlockRenderer = (block: ContentBlock) => ReactNode;

export const COLOR_THEME = defineExtensionPoint<ColorThemeSpec>({
  id: "lyra.colorTheme",
  capability: "theme",
  keying: "single",
});
export const ACCENT = defineExtensionPoint<AccentSpec>({
  id: "lyra.accent",
  capability: "theme",
  keying: "single",
});
export const VISUAL_STYLE = defineExtensionPoint<VisualStyleSpec>({
  id: "lyra.visualStyle",
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
export const AGENT_RUN_OPTIONS = defineExtensionPoint<AgentRunOptionsProviderSpec>({
  id: "lyra.agent.runOptions",
  capability: "agent",
  keying: "single",
  keyOf: (s) => s.id,
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
export const COMPOSER_SUBMIT_MODE = defineExtensionPoint<ComposerSubmitModeSpec>({
  id: "lyra.composer.submitMode",
  capability: "composer",
  keying: "single",
  keyOf: (mode) => mode.id,
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
export const CONTEXT_DOCK_DESTINATION = defineExtensionPoint<ContextDockDestinationSpec>({
  id: "lyra.contextDock.destination",
  capability: "workspace",
  keying: "single",
  keyOf: (destination) => destination.viewId,
});

// ---- multi-handler surfaces (every contribution coexists, runs in order) --
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
export const PLUGIN_LOAD_LISTENER = defineExtensionPoint<(name: string) => void>({
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
export const STREAM_EVENT_HANDLER = defineExtensionPoint<{
  eventType: string;
  handler: StreamEventHandler;
}>({ id: "lyra.events.stream", capability: "events", keying: "multi" });
export const LAYOUT_SLOT = defineExtensionPoint<{ slot: string; spec: LayoutSlotSpec }>({
  id: "lyra.layoutSlot",
  capability: "layout",
  keying: "multi",
});

export const WORK_INDEX_ITEM = defineExtensionPoint<WorkIndexItemSpec>({
  id: "lyra.workIndex.item",
  capability: "navigation",
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
export const TOOL_VIEW_OPENER = defineExtensionPoint<ToolViewOpenerSpec>({
  id: "lyra.tool.viewOpener",
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
// A tool whose whole outcome is already on screen somewhere that stays on screen.
// Keyed by tool name; the value names the surface, so the claim is answerable ("who
// says so?") rather than a bare flag. The narrative then does not repeat it — the
// plan was being drawn twice, in the active surface above the composer and
// again as a tool row inside it.
//
// Claim only what the surface shows in FULL. A tool that asks the person something
// is not presented by that surface however much of the plan it echoes: `exit_plan_mode`
// interrupts for approval of the plan, and hiding it would hide the question.
export const TOOL_STANDING_SURFACE = defineExtensionPoint<string>({
  id: "lyra.tool.standingSurface",
  capability: "tool",
  keying: "single",
});
export const CONTENT_BLOCK = defineExtensionPoint<RegisteredContentBlockRenderer>({
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
