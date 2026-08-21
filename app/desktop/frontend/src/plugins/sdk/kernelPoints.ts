// Kernel extension points — the typed handles for every contribution surface
// the kernel itself owns. Built-ins contribute via `host.extensions.contribute
// (POINT, …)` (or one of the few retained thin facades), and the per-domain
// selectors read these.
//
// Adding a kernel point = one `defineExtensionPoint` block here + one selector.

import type {
  AgentRunOptionsProviderSpec,
  AgentSourceSpec,
  CommandSpec,
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
import { normalizeCombo } from "./combo";

type RegisteredContentBlockRenderer = (block: ContentBlock) => ReactNode;

export const COLOR_THEME = defineExtensionPoint<ColorThemeSpec>({
  id: "lyra.colorTheme",
  keying: "single",
});
export const ACCENT = defineExtensionPoint<AccentSpec>({
  id: "lyra.accent",
  keying: "single",
});
export const VISUAL_STYLE = defineExtensionPoint<VisualStyleSpec>({
  id: "lyra.visualStyle",
  keying: "single",
});
export const LOCALE = defineExtensionPoint<LocaleSpec>({
  id: "lyra.locale",
  keying: "single",
});

export const ROUTE = defineExtensionPoint<RouteSpec>({
  id: "lyra.route",
  keying: "single",
});
export const AGENT_SOURCE = defineExtensionPoint<AgentSourceSpec>({
  id: "lyra.agent.source",
  keying: "single",
});
export const AGENT_RUN_OPTIONS = defineExtensionPoint<AgentRunOptionsProviderSpec>({
  id: "lyra.agent.runOptions",
  keying: "single",
  keyOf: (s) => s.id,
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
export const COMPOSER_SUBMIT_MODE = defineExtensionPoint<ComposerSubmitModeSpec>({
  id: "lyra.composer.submitMode",
  keying: "single",
  keyOf: (mode) => mode.id,
});

export const SHORTCUT = defineExtensionPoint<ShortcutSpec>({
  id: "lyra.shortcut",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

export const COMMAND = defineExtensionPoint<CommandSpec>({
  id: "lyra.command",
  keying: "single",
});
export const SETTINGS_PANE = defineExtensionPoint<SettingsPaneSpec>({
  id: "lyra.settingsPane",
  keying: "single",
});
export const WORKSPACE_VIEW = defineExtensionPoint<WorkspaceViewSpec>({
  id: "lyra.workspaceView",
  keying: "single",
});
export const CONTEXT_DOCK_DESTINATION = defineExtensionPoint<ContextDockDestinationSpec>({
  id: "lyra.contextDock.destination",
  keying: "single",
  keyOf: (destination) => destination.viewId,
});

// ---- multi-handler surfaces (every contribution coexists, runs in order) --
export const LOG_SUBSCRIBER = defineExtensionPoint<LogSubscriber>({
  id: "lyra.log.subscriber",
  keying: "multi",
});

export const READY_HANDLER = defineExtensionPoint<ReadyHandler>({
  id: "lyra.lifecycle.ready",
  keying: "multi",
});

// The item wraps its sub-key (name / eventType / slot) alongside the payload;
// the events + layout selectors build a cached secondary index over it (see
// `createPointSubIndex`). The reducer hits these per StreamEvent.
export const STREAM_EVENT_HANDLER = defineExtensionPoint<{
  eventType: string;
  handler: StreamEventHandler;
}>({ id: "lyra.events.stream", keying: "multi" });
export const LAYOUT_SLOT = defineExtensionPoint<{ slot: string; spec: LayoutSlotSpec }>({
  id: "lyra.layoutSlot",
  keying: "multi",
});

export const WORK_INDEX_ITEM = defineExtensionPoint<WorkIndexItemSpec>({
  id: "lyra.workIndex.item",
  keying: "single",
});

export const MESSAGE_ROLE = defineExtensionPoint<MessageRoleSpec>({
  id: "lyra.message.role",
  keying: "single",
});
export const TOOL_ACTION = defineExtensionPoint<ToolActionSpec>({
  id: "lyra.tool.action",
  keying: "single",
});
export const TOOL_VIEW_OPENER = defineExtensionPoint<ToolViewOpenerSpec>({
  id: "lyra.tool.viewOpener",
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
  keying: "single",
});
export const CONTENT_BLOCK = defineExtensionPoint<RegisteredContentBlockRenderer>({
  id: "lyra.message.contentBlock",
  keying: "single",
});
// Per-message citation sources — each maps the message's blocks to the
// citations they imply (multi: every contribution's output is concatenated).
// Keeps the kernel ignorant of which block kind carries sources, so a
// citation-producing feature (e.g. the search block) stays fully removable.
export const MESSAGE_CITATION_SOURCE = defineExtensionPoint<CitationSource>({
  id: "lyra.message.citationSource",
  keying: "multi",
});
