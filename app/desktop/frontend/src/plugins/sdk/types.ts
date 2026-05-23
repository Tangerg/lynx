// Plugin SDK type surface.
//
// Every register-* extension point appears here as a method on Host.
// Adding new points = adding fields here; nothing reaches deeper into the
// host implementation.

import type { ComponentType } from "react";
import type { AbstractAgent } from "@ag-ui/client";
import type { BaseEvent } from "@ag-ui/core";
import type { ConfigValue } from "./config";
import type { StateSlice } from "./stateSlice";
import type {
  AgentViewState,
  ContentBlockKind,
  ContentBlockMap,
  ToolCall,
} from "@/protocol/agui/viewState";

// ---------------------------------------------------------------------------
// Common
// ---------------------------------------------------------------------------

/**
 * A reversible handle. `register*` returns this; the host calls `.dispose()`
 * during unload so plugin authors never write cleanup code themselves.
 */
export type Disposable = { dispose: () => void };

// ---------------------------------------------------------------------------
// Tool previews — plugin-contributed inline body for a tool call.
// ---------------------------------------------------------------------------

export type ToolPreviewProps = {
  tool: ToolCall;
  /** Promote the full tool view (terminal / diff / …) into a main-area tab. */
  onOpenView: () => void;
};
export type ToolPreviewComponent = ComponentType<ToolPreviewProps>;

// ---------------------------------------------------------------------------
// Tool actions — header buttons (Re-run, Copy, …) on a tool card.
// ---------------------------------------------------------------------------

/**
 * A button rendered on every ToolCard's header, before the expand button.
 * The optional `predicate` lets a plugin scope the action to a subset of
 * tool calls (e.g. only `bash` tools, only completed tools).
 *
 * Common use cases: copy-command, rerun, open-file, view-stderr.
 */
export type ToolActionSpec = {
  id: string;
  /** Icon name. */
  icon: string;
  /** Tooltip / aria label. */
  title: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** Optional gate — return false to hide the action for this tool. */
  predicate?: (tool: ToolCall) => boolean;
  /** Click handler. */
  run: (tool: ToolCall) => void | Promise<void>;
};

// ---------------------------------------------------------------------------
// Content blocks — renderable parts inside a message body.
// ---------------------------------------------------------------------------

/**
 * Renderer props for a specific content-block kind. The `block` prop is
 * typed to exactly the variant whose `kind` matches K, so plugins get
 * autocomplete on the block payload fields.
 */
export type ContentBlockRendererProps<K extends ContentBlockKind> = {
  block: ContentBlockMap[K];
};
export type ContentBlockRenderer<K extends ContentBlockKind> =
  ComponentType<ContentBlockRendererProps<K>>;

// ---------------------------------------------------------------------------
// AG-UI CUSTOM event handlers — fold protocol events into view state.
// ---------------------------------------------------------------------------

/**
 * Pure state update — takes the current view state, returns the next.
 *
 * Handlers compose updates from helpers exported by `@/plugins/sdk/state`
 * (e.g. `appendBlockToMessage`) so they don't have to know the state shape.
 */
export type StateUpdate = (state: AgentViewState) => AgentViewState;

/**
 * AG-UI CUSTOM event handler. Receives the event's `value` payload and
 * returns either a StateUpdate (to mutate the view state) or void (for
 * side-effect-only handlers like analytics).
 */
export type CustomEventHandler<T = unknown> = (value: T) => StateUpdate | void;

/**
 * Handler for an AG-UI *built-in* event type (RUN_STARTED, TEXT_MESSAGE_*,
 * TOOL_CALL_*, REASONING_*, etc.). Receives the full state + raw event and
 * returns the next state. Multiple plugins can register for the same event
 * type; they run in registration order, each seeing the previous output.
 *
 * The core protocol semantics live in the `lyra.builtin.core-reducer` plugin
 * — pluginifying these makes "everything is a plugin" literal: even the AG-UI
 * spec is just one (replaceable) plugin's contribution.
 */
export type CoreEventHandler = (state: AgentViewState, event: BaseEvent) => AgentViewState;

// ---------------------------------------------------------------------------
// Settings panes — rail entries inside the Settings workspace view.
// ---------------------------------------------------------------------------

export type SettingsPaneSpec = {
  /** Stable id used as the rail key + storage namespace if needed. */
  id: string;
  /** Sidebar label. */
  label: string;
  /** Optional icon name (any `IconName` the host exposes). */
  icon?: string;
  /** Sort hint — lower comes first. Built-ins use 0..99; plugins ≥ 100. */
  order?: number;
  /** The pane content. Receives no props in v1. */
  component: ComponentType;
};

// ---------------------------------------------------------------------------
// Themes — color schemes (dark/light) and accents.
// ---------------------------------------------------------------------------

/**
 * A theme — IDE/VS Code-style swappable palette.
 *
 * The theme owns its entire color palette via `tokens`: each entry is a
 * CSS custom property name (WITHOUT the leading `--`) mapped to its value.
 * When a theme becomes active, the host writes every token to
 * `:root.style` (inline) so it overrides whatever the stylesheets declared,
 * and toggles the `theme-{scheme}` class on <html> so structural rules
 * keyed on `.theme-light` / `.theme-dark` still apply.
 *
 * `scheme` is the binary kind (dark vs light) used for:
 *   - The `<html>` class for structural overrides
 *   - Picking the accent variant (light vs dark hex on each accent)
 *   - Asset selection (Shiki / Mermaid theme presets)
 *
 * Themes without `tokens` keep working as metadata-only registrations —
 * the existing stylesheets supply values. Pass `tokens` to take full
 * control of the palette.
 */
export type ThemeSpec = {
  /** Stable id. Persisted in localStorage as `useUIStore.theme`. */
  id: string;
  /** User-facing label. */
  label: string;
  /** Native scheme — drives `<html style="color-scheme">` + accent picker. */
  scheme: "dark" | "light";
  /** Icon name for the segmented control. */
  icon?: string;
  /** Sort hint — lower comes first. */
  order?: number;
  /**
   * CSS custom property values, keyed by name WITHOUT the leading `--`.
   * Applied to `:root.style` when this theme is active.
   * Example: `{ "color-bg": "#010102", "color-surface": "#181a1d" }`
   */
  tokens?: Record<string, string>;
};

/**
 * An accent — a named color with one value per theme scheme. The active
 * scheme's hex is written to `--color-accent`.
 *
 * Defaults: `light` falls back to `dark` when omitted, which is what
 * "monochrome-friendly" accents will want.
 */
export type ThemeAccentSpec = {
  id: string;
  label: string;
  dark: string;
  light?: string;
  /** Sort hint — lower comes first. */
  order?: number;
};

// ---------------------------------------------------------------------------
// Notifications — persistent feed surfaced by host.notify().
// ---------------------------------------------------------------------------

export type NotificationLevel = "info" | "warn" | "error";

/**
 * One entry in the persistent notification feed. Created every time a
 * plugin calls `host.notify(...)`. The transient toast is just the visual
 * surface; the feed is what plugins (e.g. workspace view, settings pane)
 * read.
 */
export type NotificationEntry = {
  /** Monotonic id assigned by the host. */
  id: number;
  /** Plugin that called `host.notify`. */
  plugin: string;
  level: NotificationLevel;
  message: string;
  /** Created-at timestamp (ms). */
  timestamp: number;
  /** Set when the user dismisses the toast / clears the feed entry. */
  dismissed?: boolean;
};

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

/**
 * Fires once, when PluginProvider has finished loading all built-in
 * plugins (sideloaded plugins may still be in-flight). Registering a hook
 * after the ready point fires it synchronously / on the next microtask —
 * "have I missed it" is never a concern.
 *
 * Common use: a plugin whose setup needs to read the full registry
 * (e.g. snapshot every accent, every command). Registering at setup time
 * is order-dependent; deferring to onReady is not.
 */
export type ReadyHandler = () => void;

/**
 * Fires on `window.beforeunload`. Synchronous — use it for "flush
 * something quickly" cleanup, not promise-y teardown.
 */
export type BeforeUnloadHandler = () => void;

// ---------------------------------------------------------------------------
// Logger — structured logger passed to a plugin in setup().
// ---------------------------------------------------------------------------

export type LogLevel = "debug" | "info" | "warn" | "error";

/**
 * One log event. `plugin` records who emitted it, so a UI that consumes
 * logs (notifications pane, dev panel) can group / filter by plugin.
 */
export type LogEvent = {
  plugin: string;
  level: LogLevel;
  args: unknown[];
  timestamp: number;
};

/** Subscriber for log events. Errors thrown inside are caught by the host. */
export type LogSubscriber = (event: LogEvent) => void;

// ---------------------------------------------------------------------------
// RPC hooks — runtime extension of the host.rpc namespace.
// ---------------------------------------------------------------------------

/**
 * A `beforeRequest` hook — runs immediately before the underlying fetch.
 * Can mutate the Request (set headers, replace URL, log) or return a
 * brand-new one to substitute. Awaited.
 *
 * Hooks run in registration order; the first registered runs first.
 */
export type RpcBeforeRequestHook = (request: Request) =>
  | void
  | Request
  | Promise<void | Request>;

/**
 * An `afterResponse` hook — runs once the underlying fetch resolves
 * (success or HTTP error). Can inspect / replace the Response (e.g.
 * shape error bodies, refresh expired tokens then retry).
 */
export type RpcAfterResponseHook = (
  request: Request,
  response: Response,
) => void | Response | Promise<void | Response>;

// ---------------------------------------------------------------------------
// Data providers — pluggable fetchers behind React Query hooks.
// ---------------------------------------------------------------------------

/**
 * A data fetcher registered against a key. TanStack-Query hooks in the app
 * resolve their `queryFn` by looking up the provider for their key. The
 * fetcher must return a fully-typed result, but the registry erases that
 * type so all providers fit in one map — call sites cast on their way out.
 *
 * Plugins can swap the underlying transport (HTTP, IPC, in-memory mock)
 * without callers having to know.
 */
export type DataProviderSpec<T = unknown> = {
  /** Query key — must match the consumer hook's expected key. */
  key: string;
  /** Async fetcher. Throw for failure; TanStack-Query handles the rest. */
  fetcher: () => Promise<T>;
};

// ---------------------------------------------------------------------------
// Agent sources — transports that drive the chat (HTTP, mock, IPC…).
// ---------------------------------------------------------------------------

/**
 * A provider for the AG-UI agent that drives the chat. The default ships an
 * HttpAgent against the local Go backend; alternative sources can implement
 * a WebSocket variant, mock streamer, etc.
 *
 * Only one source is active at a time — kernel-chat resolves to the
 * first spec sorted by `priority`. Higher priority wins; a user plugin
 * can override the built-in by registering at priority > 0.
 */
export type AgentSourceSpec = {
  id: string;
  label: string;
  /** Higher wins. Built-in defaults use 0. */
  priority?: number;
  /** Build a fresh agent for each session. */
  factory: () => AbstractAgent;
};

// ---------------------------------------------------------------------------
// Commands — entries shown in the Cmd+K palette.
// ---------------------------------------------------------------------------

/**
 * A palette-invokable action. Surfaced in the Cmd+K command palette and
 * (eventually) any context-menu / button that wants to invoke it by id.
 *
 * Distinct from slash commands (which run from the composer when the user
 * types `/<cmd>`). Both can coexist for the same action — register both
 * if you want it reachable from both UIs.
 */
export type CommandSpec = {
  /** Stable id. */
  id: string;
  /** Display label. */
  label: string;
  /** Short explanation shown below the label. */
  description?: string;
  /** Icon name. */
  icon?: string;
  /** Group header in the palette (e.g. "View", "Theme"). */
  group?: string;
  /** Extra search aliases — appears in the label match but isn't displayed. */
  keywords?: string[];
  /** Display-only hint of the keyboard shortcut; does NOT auto-register one. */
  shortcut?: string;
  /** Sort hint within the group. Lower comes first. */
  order?: number;
  /**
   * Optional `when` clause filtering when this command is visible in the
   * palette. See `evalWhen.ts` for the supported syntax. Identifiers come
   * from the runtime when-context (e.g. `mainViewActive`, `mainView`,
   * `theme`, `sidebarRail`). Missing/invalid → command hidden.
   */
  when?: string;
  /** What to do. */
  run: () => void | Promise<void>;
};

/**
 * Declarative command — same shape as CommandSpec minus the run handler.
 * Lives in `PluginSpec.contributes.commands` so the kernel can show the
 * palette entry *before* the plugin is activated. Picking the entry
 * triggers the plugin's activation; after setup runs, the real
 * `host.commands.register` call replaces this placeholder.
 */
export type ContributedCommand = Omit<CommandSpec, "run">;

// ---------------------------------------------------------------------------
// Keyboard shortcuts — global, app-wide key bindings.
// ---------------------------------------------------------------------------

/**
 * Handler invoked when the matching key combo is pressed. Receives the
 * raw event so handlers can decide whether to `preventDefault` (most do).
 *
 * Return value is ignored.
 */
export type ShortcutHandler = (event: KeyboardEvent) => void;

/**
 * A keyboard shortcut registration.
 *
 * `key` is a `KeyboardEvent.key` plus optional modifier prefixes joined by
 * `+`. Examples:
 *   - "Escape"
 *   - "Cmd+K"           (Mac ⌘)
 *   - "Ctrl+K"          (everywhere else)
 *   - "Mod+K"           (Cmd on Mac, Ctrl elsewhere — preferred)
 *   - "Shift+/"         (`?` on US keyboards)
 *   - "Mod+Shift+P"
 *
 * Matching is case-insensitive on the key name. If two plugins register
 * the same combo, the last one wins (with a warning) — same policy as the
 * other slots.
 */
export type ShortcutSpec = {
  /** Combo string, e.g. "Mod+K". */
  key: string;
  /** What to do. */
  handler: ShortcutHandler;
  /** Optional human-readable description for a future shortcuts cheat-sheet. */
  description?: string;
  /**
   * Whether to fire even when the active element is an `<input>`/`<textarea>`.
   * Defaults to false — most shortcuts shouldn't steal typing input.
   */
  allowInInputs?: boolean;
};

// ---------------------------------------------------------------------------
// Composer key bindings — keys handled by the chat composer textarea.
// ---------------------------------------------------------------------------

/**
 * Context passed to a composer key binding handler. The handler can read
 * the current value, replace it, or invoke `submit` to send the pending
 * text. Returning `true` (or invoking `preventDefault` indirectly via
 * `submit`) tells the host to stop the browser default.
 */
export type ComposerKeyContext = {
  value: string;
  onChange: (next: string) => void;
  submit: () => void;
  event: KeyboardEvent;
};

export type ComposerKeyBindingSpec = {
  /** Combo string — same format as `host.shortcuts.register`. */
  key: string;
  description?: string;
  /** Return `true` to call `preventDefault` on the keypress. */
  handler: (ctx: ComposerKeyContext) => boolean | void;
};

// ---------------------------------------------------------------------------
// Composer attachment sources — chips displayed above the composer.
// ---------------------------------------------------------------------------

/**
 * Shape of one chip rendered in the composer attachments row. Mirrors
 * `components/chat/Composer.tsx`'s `Attachment` type — declared here so
 * plugins don't have to import from `components/`.
 */
export type ComposerAttachment = {
  /** Display label, e.g. "src/api/auth.ts". */
  label: string;
  /** Optional icon glyph name. Defaults to "file" when omitted. */
  icon?: string;
};

/**
 * A plugin contribution that produces attachment chips. The kernel
 * merges the lists from every source (in `order`) ahead of any
 * user-added items stored in `useComposerStore.attachments`.
 *
 * `useAttachments` is a hook — plugins can derive the list from query
 * data ("recently edited files") or other stores.
 */
export type ComposerAttachmentSourceSpec = {
  id: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** Hook that returns the current attachments. */
  useAttachments: () => ComposerAttachment[];
};

// ---------------------------------------------------------------------------
// Composer placeholder pool — rotating textarea placeholders.
// ---------------------------------------------------------------------------

/**
 * One placeholder string for the composer textarea. Composer picks one at
 * mount via weighted random — `weight` defaults to 1, so a plugin can
 * register multiple to bias toward (or against) certain prompts.
 *
 * Useful for branding ("Ask Acme…") or seasonal nudges ("Try /lint on a
 * test file").
 */
export type ComposerPlaceholderSpec = {
  id: string;
  text: string;
  /** Selection weight — defaults to 1. Set to 0 to register but skip selection. */
  weight?: number;
};

// ---------------------------------------------------------------------------
// Composer modes — segmented toggle (agent / ask / plan / …).
// ---------------------------------------------------------------------------

/**
 * A composer mode toggle ("Agent" / "Ask" / "Plan" by default — plugins can
 * register more). The active mode is stored on `useComposerStore.mode` so
 * the conversation context (agent vs ask vs plan) can drive runtime
 * behaviour (e.g. a /plan command, or a stricter prompt prefix).
 *
 * Mode ids are free-form strings: built-ins use `agent`, `ask`, `plan`;
 * a third-party plugin could add `code`, `research`, etc.
 */
export type ComposerModeSpec = {
  id: string;
  label: string;
  icon?: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** Optional tooltip; defaults to "${label} mode". */
  title?: string;
};

// ---------------------------------------------------------------------------
// Composer status chips — informational chips above the composer.
// ---------------------------------------------------------------------------

/**
 * Plugin-contributed chip in the composer footer ("project · branch · mode").
 *
 * The component renders the chip body — typically a small `<button>` with
 * icon + label. The host provides no props; chips read state from stores
 * directly.
 */
export type ComposerStatusSpec = {
  id: string;
  /** Sort hint — lower comes first. Built-ins use 0..99. */
  order?: number;
  /** The chip body. Receives no props. */
  component: ComponentType;
};

// ---------------------------------------------------------------------------
// Sidebar sections — collapsible blocks inside the expanded sidebar.
// ---------------------------------------------------------------------------

/**
 * Plugin-contributed sidebar section, rendered between the search box and
 * the user-card footer. Each section owns its own header + body; the
 * sidebar kernel just orders them by `order`.
 */
export type SidebarSectionSpec = {
  id: string;
  /** Sort hint. */
  order?: number;
  /** Section body. Receives no props. */
  component: ComponentType;
};

/**
 * Plugin-contributed item in the collapsed (rail) sidebar. The kernel
 * renders all registered items vertically in `order`. Each item may be a
 * single button, a stack of buttons, a divider, or anything else — it
 * just has to fit in the rail's narrow column.
 *
 * Conventional order ranges:
 *   - 0..99: top-area items (brand, new session, search)
 *   - 100..899: middle area (recent sessions, custom stacks)
 *   - 900..999: bottom area (tools, settings, user) — these typically
 *     render with `margin-top: auto` or similar to stick to the bottom
 */
export type SidebarRailItemSpec = {
  id: string;
  /** Sort hint — see ranges above. */
  order?: number;
  /** Item body. Receives no props. */
  component: ComponentType;
};

// ---------------------------------------------------------------------------
// Message roles — display name + icon for each role in the transcript.
// ---------------------------------------------------------------------------

/**
 * A message role identity — display name + avatar icon. Used by
 * MessageBlock to render the message header consistently. Built-in roles
 * are `user`, `assistant`, `system`; a plugin can register more (e.g. a
 * `developer` role with a wrench icon).
 */
export type MessageRoleSpec = {
  /** Stable id — matches `Message.role`. */
  id: string;
  /** Header label shown next to the timestamp. */
  displayName: string;
  /** Icon name rendered inside the avatar bubble. */
  icon?: string;
  /**
   * Variant on the Avatar primitive — controls the bubble style. Two
   * built-ins exist today: `msg-user` and `msg-agent`. Plugins can stick
   * with one of those or rely on default styling.
   */
  avatarVariant?: "msg-user" | "msg-agent" | string;
};

// ---------------------------------------------------------------------------
// Routes — plugin-contributed URL paths and their renderers.
// ---------------------------------------------------------------------------

/**
 * A top-level route — registers a path → component pair. The router is
 * rebuilt from the registry at AppRouter mount time, so additions take
 * effect on next reload (or by calling `rebuildRouter()` from the host).
 */
export type RouteSpec = {
  /** Stable id — used as the TanStack route id. */
  id: string;
  /** URL path (TanStack syntax, e.g. "/", "/runs/$runId"). */
  path: string;
  /** Page component. */
  component: ComponentType;
  /** Sort hint — does not affect matching, only listing order. */
  order?: number;
};

// ---------------------------------------------------------------------------
// Plugin error fallback — UI shown when a plugin component throws.
// ---------------------------------------------------------------------------

/**
 * Props passed to the registered error-fallback renderer when a plugin
 * component throws inside `PluginBoundary`.
 */
export type PluginErrorFallbackProps = {
  /** Plugin name / context label, e.g. "view:diff" or "layout:app.main:chat". */
  plugin: string;
  /** Optional human-readable label that was passed to the boundary. */
  label?: string;
  /** The thrown Error. */
  error: Error;
};

export type PluginErrorFallbackSpec = {
  id: string;
  /** Sort hint — highest priority wins. Built-ins use 0; plugins ≥ 100. */
  priority?: number;
  component: ComponentType<PluginErrorFallbackProps>;
};

// ---------------------------------------------------------------------------
// Workspace views — plugin-contributed surfaces that open as main-area tabs.
// ---------------------------------------------------------------------------

/** First-launch docking hint. Plain regions only; floating is not exposed. */
export type DockLocation = "left" | "right" | "main" | "bottom";

/**
 * A plugin-contributed view that participates in the workspace layout.
 * Unlike `LayoutSlotSpec`, a workspace view doesn't pick a position — the
 * user does (open, close, switch tabs). The kernel only needs `id` +
 * the component; everything else is a hint.
 */
export type WorkspaceViewSpec = {
  /** Stable id — used as the layout persistence key. */
  id: string;
  /** Tab title shown in the panel header. */
  title: string;
  /** Icon name for the tab header. */
  icon?: string;
  /** First-launch docking hint. Ignored once the user has saved a layout. */
  defaultLocation?: DockLocation;
  /** First-launch open by default. Set false for "registered but hidden" views. */
  openByDefault?: boolean;
  /** Sort hint within the default location. Lower comes first. */
  order?: number;
  /** The body component. Receives no props. */
  component: ComponentType;
};

// ---------------------------------------------------------------------------
// Layout slots — named mount points the kernel exposes (sidebar.body, …).
// ---------------------------------------------------------------------------

/**
 * Plugin-contributed kernel region.
 *
 * The kernel renders `<Slot name="..."/>` for each region (sidebar, main,
 * statusbar, overlay). Plugins fill regions by registering a component +
 * sort hint. Most regions are conceptually singletons (sidebar / main)
 * but the registry allows multiple contributions so power users can
 * stack overlays without forking the kernel.
 *
 * The component receives no props — slot consumers read from app stores
 * (Zustand) and react-query hooks directly. That keeps the registration
 * descriptor flat and prevents the kernel from having to thread N props
 * down to N plugins.
 */
export type LayoutSlotSpec = {
  /** Stable id — multiple registrations to the same slot use this to dedupe. */
  id: string;
  /** Sort hint — lower comes first. Built-ins use 0..99; plugins ≥ 100. */
  order?: number;
  /** Optional className applied to the wrapper div around `component`. */
  className?: string;
  /** Component that renders the region. Receives no props. */
  component: ComponentType;
};

// ---------------------------------------------------------------------------
// Composer slash commands — typed-in `/foo` shortcuts.
// ---------------------------------------------------------------------------

/**
 * Context passed to a slash command's `run` function.
 *
 * `send(text)` lets the command queue a real agent message after running
 * its local logic. Useful for commands like `/lint <file>` that first hit
 * a backend endpoint and then ask the agent to interpret the result.
 */
export type SlashCommandRunCtx = {
  args: string;
  send: (text: string) => void;
};

export type SlashCommandSpec = {
  /** Description shown in the autocomplete dropdown. */
  description: string;
  /**
   * Optional run handler. If absent, the command is a *hint only* — typing
   * it just shows the description; pressing Enter forwards the raw text as
   * a normal user message.
   */
  run?: (ctx: SlashCommandRunCtx) => void | Promise<void>;
};

// ---------------------------------------------------------------------------
// Host
// ---------------------------------------------------------------------------

export type Host = {
  tool: {
    /** Register a tool-call preview component (last-write-wins). */
    registerPreview(fn: string, component: ToolPreviewComponent): Disposable;
    /** Register a header action button shown on every ToolCard. */
    registerAction(spec: ToolActionSpec): Disposable;
    /**
     * Register the icon glyph for a tool function name. The lookup is
     * checked before any hardcoded fallback so plugins can swap icons
     * (e.g. give `bash` a custom terminal glyph) without forking the kernel.
     */
    registerIcon(fn: string, icon: string): Disposable;
  };
  message: {
    /** Register a renderer for a content-block kind. */
    registerContentBlock<K extends ContentBlockKind>(
      kind: K,
      renderer: ContentBlockRenderer<K>,
    ): Disposable;
    /** Register a role identity (display name + avatar icon). */
    registerRole(spec: MessageRoleSpec): Disposable;
  };
  agui: {
    /** Subscribe to an AG-UI CUSTOM event by name. */
    on<T = unknown>(name: string, handler: CustomEventHandler<T>): Disposable;
    /**
     * Subscribe to an AG-UI *built-in* event type (RUN_STARTED, etc.).
     *
     * Handlers chain: the reducer dispatches one event through every plugin
     * registered for its type, in registration order, threading state from
     * one to the next. Throwing isolates to the offending plugin and falls
     * back to the input state (same isolation policy as `on`).
     */
    onCore(eventType: string, handler: CoreEventHandler): Disposable;
  };
  layout: {
    /** Contribute a component to a named kernel region. */
    register(slot: string, spec: LayoutSlotSpec): Disposable;
  };
  workspace: {
    /** Contribute a dockable view that participates in the workspace layout. */
    registerView(spec: WorkspaceViewSpec): Disposable;
    /** Open (or focus) a registered view by id. Imperative trigger from a
     *  command palette entry / slash command / external link. */
    openView(id: string): void;
    /** Close a registered view by id. */
    closeView(id: string): void;
  };
  theme: {
    /** Contribute a selectable theme. */
    registerTheme(spec: ThemeSpec): Disposable;
    /** Contribute a selectable accent. */
    registerAccent(spec: ThemeAccentSpec): Disposable;
  };
  router: {
    /** Contribute a top-level route to the router tree. */
    register(spec: RouteSpec): Disposable;
  };
  composer: {
    /** Register a slash command (`/<cmd>`). */
    registerCommand(cmd: string, spec: SlashCommandSpec): Disposable;
    /** Contribute a status chip in the composer footer. */
    registerStatus(spec: ComposerStatusSpec): Disposable;
    /** Contribute a mode toggle (agent / ask / plan / etc.). */
    registerMode(spec: ComposerModeSpec): Disposable;
    /** Contribute a placeholder for the textarea. Random weighted pick. */
    registerPlaceholder(spec: ComposerPlaceholderSpec): Disposable;
    /** Contribute a source of attachment chips. */
    registerAttachmentSource(spec: ComposerAttachmentSourceSpec): Disposable;
    /** Bind a key combo on the textarea (Enter, Mod+Enter, etc.). */
    registerKeyBinding(spec: ComposerKeyBindingSpec): Disposable;
  };
  sidebar: {
    /** Contribute a section in the expanded sidebar. */
    registerSection(spec: SidebarSectionSpec): Disposable;
    /** Contribute an item to the collapsed (rail) sidebar. */
    registerRailItem(spec: SidebarRailItemSpec): Disposable;
  };
  shortcuts: {
    /** Register a global keyboard shortcut (e.g. "Mod+K", "Escape"). */
    register(spec: ShortcutSpec): Disposable;
  };
  agent: {
    /** Register an AG-UI agent source. Highest-priority spec wins. */
    registerSource(spec: AgentSourceSpec): Disposable;
  };
  data: {
    /** Register a query-key fetcher consumed by `useQuery({queryKey: [key]})`. */
    registerProvider<T = unknown>(spec: DataProviderSpec<T>): Disposable;
  };
  commands: {
    /** Contribute a command palette entry. */
    register(spec: CommandSpec): Disposable;
  };
  lifecycle: {
    /** Fires once after the built-in plugin set finishes loading. */
    onReady(fn: ReadyHandler): Disposable;
    /** Fires synchronously on window.beforeunload. */
    onBeforeUnload(fn: BeforeUnloadHandler): Disposable;
  };
  state: {
    /**
     * Get (or create) the shared `StateSlice` for `name`. The first caller's
     * `initial` wins — subsequent calls receive the same slice and ignore
     * their `initial` argument.
     *
     * Use it to share ephemeral state between plugins without forming a
     * hard module import: producer + consumer agree on the slice name and
     * the type.
     */
    slice<T>(name: string, initial: T): StateSlice<T>;
  };
  config: {
    /** Read an app-wide config value (with optional fallback). */
    get<T = ConfigValue>(key: string, defaultValue?: T): T | undefined;
    /** Set an app-wide config value. Fires subscribers. */
    set(key: string, value: ConfigValue): void;
    /** Does the key have a value (regardless of falsiness)? */
    has(key: string): boolean;
    /** Subscribe to changes for one key. Receives the new value (or undefined). */
    onChange(
      key: string,
      fn: (value: ConfigValue | undefined) => void,
    ): Disposable;
  };
  settings: {
    /** Contribute a pane to the settings modal. */
    registerPane(spec: SettingsPaneSpec): Disposable;
  };
  /** Namespaced key-value storage, persisted to localStorage. */
  storage: {
    get<T = unknown>(key: string): T | undefined;
    set<T = unknown>(key: string, value: T): void;
    remove(key: string): void;
    keys(): string[];
  };
  rpc: {
    /** GET against the Go backend (baseUrl is pre-configured). */
    get<T>(path: string, params?: Record<string, unknown>): Promise<T>;
    /** POST against the Go backend. */
    post<T>(path: string, body?: unknown): Promise<T>;
    /**
     * Register a request hook. Runs for every call made through the shared
     * `api` ky instance (which is what `host.rpc.get/post`, queries.ts, and
     * kernel-chat all use). Common uses: auth headers, request logging,
     * X-Request-Id injection.
     */
    beforeRequest(hook: RpcBeforeRequestHook): Disposable;
    /**
     * Register a response hook. Runs after every call via the shared `api`
     * instance. Common uses: response logging, automatic token refresh,
     * normalising error envelopes.
     */
    afterResponse(hook: RpcAfterResponseHook): Disposable;
  };
  /** Display a brief toast notification. */
  notify(message: string, level?: "info" | "warn" | "error"): void;
  window: {
    /**
     * Set the document title. The host stores the requested title per
     * plugin internally so two plugins fighting over it produce a
     * deterministic outcome — the latest setter wins.
     */
    setTitle(text: string): void;
    /**
     * Prefix the current title with `[n]` when `n > 0`. Pass 0 / undefined
     * to clear. Useful for "(3) Lyra" notification counts.
     */
    setBadge(n?: number): void;
  };
  plugins: {
    /** Snapshot of currently-loaded plugins. */
    list(): LoadedPlugin[];
    /** Fires every time a plugin is loaded (including subsequent loads). */
    onLoad(fn: (spec: PluginSpec) => void): Disposable;
    /** Fires every time a plugin is unloaded. */
    onUnload(fn: (name: string) => void): Disposable;
    /**
     * Load a plugin spec at runtime — useful for hot-loading or
     * conditional features. Goes through the same setup pipeline as
     * built-in / sideload loads, including disposable tracking + error
     * isolation.
     */
    load(spec: PluginSpec): Promise<void>;
    /**
     * Unload a previously-loaded plugin. Disposes every register* return
     * value collected during setup, removes the plugin from the loaded
     * map, and fires onUnload listeners. No-op if the plugin isn't
     * currently loaded.
     */
    unload(name: string): void;
    /**
     * Convenience: `unload(name)` then re-load the same spec. Returns the
     * load promise so callers can `await` for setup completion.
     */
    reload(name: string): Promise<void>;
    /**
     * Contribute a custom error-fallback UI shown inside `PluginBoundary`
     * when a plugin-rendered component throws. Highest-priority registration
     * wins; if none, the built-in red banner is rendered.
     */
    registerErrorFallback(spec: PluginErrorFallbackSpec): Disposable;
  };
  /**
   * Structured logger. Calls always forward to `console.{method}` with a
   * `[plugin:<name>]` prefix; in addition, every registered subscriber
   * receives a `LogEvent` for its own ingestion (telemetry, devtools, etc).
   */
  log: {
    debug(...args: unknown[]): void;
    info(...args: unknown[]): void;
    warn(...args: unknown[]): void;
    error(...args: unknown[]): void;
    /** Listen to every log event emitted via `host.log.*`. */
    subscribe(fn: LogSubscriber): Disposable;
  };
};

export type PluginContext = { host: Host };

/**
 * When this plugin should activate (i.e. when `setup` runs).
 *
 *   - `"onStartup"`        — load eagerly during the kernel boot sequence.
 *                            This is the default when `activationEvents`
 *                            is missing or empty.
 *   - `"onCommand:<id>"`   — activate the first time the user runs that
 *                            command (declared in `contributes.commands`).
 *
 * Future events: `"onView:<id>"`, `"onLanguage:<id>"`, etc. — add when
 * there's a real need.
 */
export type ActivationEvent = "onStartup" | `onCommand:${string}`;

/**
 * A workspace view declared ahead of activation. Same shape as
 * `WorkspaceViewSpec` minus the body component — the kernel renders a
 * lightweight "activating…" placeholder until the plugin's setup runs.
 */
export type ContributedView = Omit<WorkspaceViewSpec, "component">;

/**
 * A settings pane declared ahead of activation. Mirror of
 * `SettingsPaneSpec` minus the body component.
 */
export type ContributedSettingsPane = Omit<SettingsPaneSpec, "component">;

/**
 * Names of the top-level groupings on `Host`. A plugin can voluntarily
 * narrow what it can see by listing only the namespaces it actually uses
 * in `PluginSpec.capabilities`. Anything not listed becomes a throwing
 * proxy on the bound host — useful as a self-imposed contract today and
 * a hook for future permission enforcement / marketplace audits.
 */
export type HostCapability =
  | "tool" | "message" | "agui" | "layout" | "workspace" | "theme"
  | "router" | "composer" | "sidebar" | "shortcuts" | "agent" | "data"
  | "commands" | "lifecycle" | "state" | "config" | "settings"
  | "storage" | "rpc" | "notify" | "window" | "plugins" | "log";

/**
 * Declarative ahead-of-activation contributions. Anything listed here is
 * visible in the palette / settings rail / workspace tab strip before
 * the plugin has actually been activated; first interaction triggers the
 * activation and swaps the placeholder for the real component.
 */
export type PluginContributes = {
  commands?: ContributedCommand[];
  views?: ContributedView[];
  settingsPanes?: ContributedSettingsPane[];
};

export type PluginSpec = {
  /** Unique identifier. Built-ins use the `lyra.builtin.*` namespace. */
  name: string;
  /** Semver string. Surfaced in settings + error reports. */
  version: string;
  /** Optional host API range this plugin targets. Not enforced yet. */
  apiVersion?: string;
  /**
   * Names of plugins that must load before this one. The kernel does a
   * topological sort over the requested list, then loads in dependency
   * order. Missing requires + cycles surface as setup errors.
   */
  requires?: string[];
  /**
   * When the plugin should activate. Defaults to eager (`["onStartup"]`)
   * when omitted, preserving the historical behaviour.
   */
  activationEvents?: ActivationEvent[];
  /**
   * Declarative metadata visible before activation. Today: commands,
   * workspace views, settings panes. Future surfaces follow the same
   * pattern when there's a real lazy-load use case.
   */
  contributes?: PluginContributes;
  /**
   * Voluntary capability declaration. When present, the bound host only
   * exposes the listed namespaces — accessing any other throws a clear
   * error at runtime. Omit to keep full access (the existing behaviour).
   */
  capabilities?: HostCapability[];
  /**
   * Called once at load time. All `host.*.register*` calls go here.
   *
   * May return a cleanup function (sync or via Promise). If returned,
   * the kernel runs it when the plugin is unloaded — handy for `subscribe`
   * style side effects whose disposable isn't a `host.*.register*` result
   * (Zustand store subscriptions, window event listeners, etc.).
   */
  setup: (ctx: PluginContext) =>
    | void
    | (() => void)
    | Promise<void | (() => void)>;
};

export type LoadedPlugin = {
  spec: PluginSpec;
  disposables: Disposable[];
};
