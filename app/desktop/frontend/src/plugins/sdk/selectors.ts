// Read-side of the plugin registry — every React hook + imperative
// lookup callers reach for. Split out of `registry.ts` to keep that
// file focused on the Zustand store + write actions.
//
// All selectors here observe the same store created in `./registry`.
// Lazy-activation helpers (placeholder → real component / handler) also
// live here because they're invoked only from selectors and would push
// the cycle back into registry.ts otherwise.

import { useMemo } from "react";
import type { ContentBlockKind } from "@/protocol/agui/viewState";
import { makeLazyActivator } from "../LazyActivator";
import { usePluginStore } from "./registry";
import type {
  AgentSourceSpec,
  CommandSpec,
  ComposerAttachmentSourceSpec,
  ComposerKeyBindingSpec,
  ComposerModeSpec,
  ComposerPlaceholderSpec,
  ComposerStatusSpec,
  ContentBlockRenderer,
  ContributedCommand,
  ContributedSettingsPane,
  ContributedView,
  CoreEventHandler,
  CustomEventHandler,
  LayoutSlotSpec,
  MessageRoleSpec,
  PluginErrorFallbackSpec,
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

// ---------------------------------------------------------------------------
// Shared list-selector helper
// ---------------------------------------------------------------------------
//
// Most "list every X contributed by plugins" hooks share the same shape:
// pluck `value` off each owned entry and sort by ascending `order` (default
// 100 when unset). `useSortedList` factors that out so each registry-backed
// selector stays a one-liner. The Map identity gates re-derivation — see
// the useSlashCommands note below for the useMemo-on-Map discipline.

type Owned<T> = { value: T; pluginName: string };
type Ordered = { order?: number };

function useSortedList<T extends Ordered>(map: Map<string, Owned<T>>): T[] {
  return useMemo(
    () =>
      Array.from(map.values())
        .map((o) => o.value)
        .sort((a, b) => (a.order ?? 100) - (b.order ?? 100)),
    [map],
  );
}

// ---------------------------------------------------------------------------
// Tool surface
// ---------------------------------------------------------------------------

export function useToolPreview(fn: string): ToolPreviewComponent | undefined {
  return usePluginStore((s) => s.toolPreviews.get(fn)?.value);
}

export function useToolActions(): ToolActionSpec[] {
  return useSortedList(usePluginStore((s) => s.toolActions));
}

/** Look up the registered icon for a tool fn name. */
export function lookupToolIcon(fn: string): string | undefined {
  return usePluginStore.getState().toolIcons.get(fn)?.value;
}

// ---------------------------------------------------------------------------
// Workspace views — registered + declared (placeholder) merged
// ---------------------------------------------------------------------------

export function useWorkspaceViews(): WorkspaceViewSpec[] {
  const registered = usePluginStore((s) => s.workspaceViews);
  const declared = usePluginStore((s) => s.declaredViews);
  return useMemo(() => {
    const byId = new Map<string, WorkspaceViewSpec>();
    for (const o of declared.values()) {
      byId.set(o.value.id, declaredToWorkspaceView(o.value, o.pluginName));
    }
    for (const o of registered.values()) {
      byId.set(o.value.id, o.value);
    }
    return Array.from(byId.values())
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  }, [registered, declared]);
}

function declaredToWorkspaceView(d: ContributedView, pluginName: string): WorkspaceViewSpec {
  return {
    ...d,
    component: makeLazyActivator(d.title, () => { void runActivator(pluginName); }),
  };
}

// ---------------------------------------------------------------------------
// Plugin error fallback
// ---------------------------------------------------------------------------

/**
 * Pick the highest-priority registered error fallback. Tied priorities
 * resolve by insertion order (later wins). Returns undefined when nothing
 * is registered.
 */
export function pickPluginErrorFallback(): PluginErrorFallbackSpec | undefined {
  const specs = Array.from(usePluginStore.getState().pluginErrorFallbacks.values()).map((o) => o.value);
  if (specs.length === 0) return undefined;
  return specs.reduce((best, cur) =>
    (cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best,
  );
}

// ---------------------------------------------------------------------------
// Message surface
// ---------------------------------------------------------------------------

export function useContentBlockRenderer(
  kind: string,
): ContentBlockRenderer<ContentBlockKind> | undefined {
  return usePluginStore((s) => s.contentBlocks.get(kind)?.value);
}

export function useMessageRole(id: string): MessageRoleSpec | undefined {
  return usePluginStore((s) => s.messageRoles.get(id)?.value);
}

// ---------------------------------------------------------------------------
// Slash commands
// ---------------------------------------------------------------------------

// IMPORTANT — selector + useMemo split.
//
// Zustand's `useShallow` compares element-by-element with Object.is. Our
// selectors used to wrap each entry in a fresh `{ cmd, spec }` object every
// call, which never `Object.is`-equals the previous one — useShallow saw a
// "different" array on every render, useSyncExternalStore raised
// "result of getSnapshot should be cached", and we got "Maximum update
// depth exceeded".
//
// Pattern: the selector returns the raw Map (reference stable until a
// register/unregister mutates the registry). The component-side useMemo
// then derives whatever shape it needs, with the Map as a dep so it only
// recomputes when the underlying data actually changes.
export function useSlashCommands(): Array<{ cmd: string; spec: SlashCommandSpec }> {
  const map = usePluginStore((s) => s.slashCommands);
  return useMemo(
    () => Array.from(map.entries()).map(([cmd, owned]) => ({ cmd, spec: owned.value })),
    [map],
  );
}

/** Look up a slash command by exact key (including leading "/"). */
export function lookupSlashCommand(cmd: string): SlashCommandSpec | undefined {
  return usePluginStore.getState().slashCommands.get(cmd)?.value;
}

// ---------------------------------------------------------------------------
// Settings panes — registered + declared merged
// ---------------------------------------------------------------------------

export function useSettingsPanes(): SettingsPaneSpec[] {
  const registered = usePluginStore((s) => s.settingsPanes);
  const declared = usePluginStore((s) => s.declaredSettingsPanes);
  return useMemo(() => {
    const byId = new Map<string, SettingsPaneSpec>();
    for (const o of declared.values()) {
      byId.set(o.value.id, declaredToSettingsPane(o.value, o.pluginName));
    }
    for (const o of registered.values()) {
      byId.set(o.value.id, o.value);
    }
    return Array.from(byId.values())
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  }, [registered, declared]);
}

function declaredToSettingsPane(d: ContributedSettingsPane, pluginName: string): SettingsPaneSpec {
  return {
    ...d,
    component: makeLazyActivator(d.label, () => { void runActivator(pluginName); }),
  };
}

// ---------------------------------------------------------------------------
// Layout / theme / composer / sidebar
// ---------------------------------------------------------------------------

export function useLayoutSlot(slot: string): LayoutSlotSpec[] {
  const map = usePluginStore((s) => s.layoutSlots);
  return useMemo(
    () =>
      Array.from(map.values())
        .filter((o) => o.value.slot === slot)
        .map((o) => o.value.spec)
        .sort((a, b) => (a.order ?? 100) - (b.order ?? 100)),
    [map, slot],
  );
}

export function useThemes(): ThemeSpec[] {
  return useSortedList(usePluginStore((s) => s.themes));
}

export function useAccents(): ThemeAccentSpec[] {
  return useSortedList(usePluginStore((s) => s.accents));
}

/** Look up a theme spec by id. */
export function lookupTheme(id: string): ThemeSpec | undefined {
  return usePluginStore.getState().themes.get(id)?.value;
}

/** Look up an accent spec by id. */
export function lookupAccent(id: string): ThemeAccentSpec | undefined {
  return usePluginStore.getState().accents.get(id)?.value;
}

/**
 * Resolve a theme id to its scheme (`"dark"` / `"light"`).
 *
 * Defaults to `"dark"` when the id isn't registered (e.g. very early in
 * boot before built-in plugins finish, or the user has a saved id that no
 * longer exists). Callers wanting the binary "is this a light theme?"
 * distinction (Shiki preset, Mermaid theme, …) should read scheme via
 * this helper rather than comparing the id against `"light"` directly —
 * custom themes like `"solarized-dark"` would otherwise fall through.
 */
export function resolveScheme(themeId: string): "dark" | "light" {
  return lookupTheme(themeId)?.scheme ?? "dark";
}

export function useComposerStatus(): ComposerStatusSpec[] {
  return useSortedList(usePluginStore((s) => s.composerStatus));
}

export function useComposerModes(): ComposerModeSpec[] {
  return useSortedList(usePluginStore((s) => s.composerModes));
}

export function useComposerAttachmentSources(): ComposerAttachmentSourceSpec[] {
  return useSortedList(usePluginStore((s) => s.composerAttachmentSources));
}

/**
 * Pick one composer placeholder via weighted random. Returns undefined
 * when nothing's registered; callers should fall back to a sensible
 * default. Pure read — call once at component mount, not on every render.
 */
export function pickComposerPlaceholder(): ComposerPlaceholderSpec | undefined {
  const specs = Array.from(usePluginStore.getState().composerPlaceholders.values()).map((o) => o.value);
  if (specs.length === 0) return undefined;
  const total = specs.reduce((sum, s) => sum + (s.weight ?? 1), 0);
  if (total <= 0) return undefined;
  let r = Math.random() * total;
  for (const spec of specs) {
    r -= spec.weight ?? 1;
    if (r <= 0) return spec;
  }
  return specs[specs.length - 1];
}

/** Look up a composer key binding by canonical combo. */
export function lookupComposerKeyBinding(canonical: string): ComposerKeyBindingSpec | undefined {
  return usePluginStore.getState().composerKeyBindings.get(canonical)?.value;
}

export function useSidebarSections(): SidebarSectionSpec[] {
  return useSortedList(usePluginStore((s) => s.sidebarSections));
}

export function useSidebarRailItems(): SidebarRailItemSpec[] {
  return useSortedList(usePluginStore((s) => s.sidebarRailItems));
}

// ---------------------------------------------------------------------------
// Commands — registered + declared (placeholder) merged
// ---------------------------------------------------------------------------

/**
 * Palette command list — registered commands merged with declared
 * (placeholder) ones. Registered wins on id collision, so once a plugin
 * is activated its real CommandSpec replaces the contributes.commands
 * placeholder transparently.
 *
 * Declared placeholders carry a stub `run` that triggers activation; the
 * activation flow then re-dispatches to the (now-registered) real
 * handler. Callers don't need to distinguish between the two.
 */
export function useCommands(): CommandSpec[] {
  const registered = usePluginStore((s) => s.commands);
  const declared = usePluginStore((s) => s.declaredCommands);
  return useMemo(() => {
    const byId = new Map<string, CommandSpec>();
    for (const o of declared.values()) {
      byId.set(o.value.id, declaredToPlaceholder(o.value, o.pluginName));
    }
    for (const o of registered.values()) {
      byId.set(o.value.id, o.value);
    }
    return Array.from(byId.values())
      .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  }, [registered, declared]);
}

/** Look up a registered command by id. */
export function lookupCommand(id: string): CommandSpec | undefined {
  return usePluginStore.getState().commands.get(id)?.value;
}

function declaredToPlaceholder(c: ContributedCommand, pluginName: string): CommandSpec {
  return {
    ...c,
    run: () => activateAndRun(pluginName, c.id),
  };
}

// ---- activation indirection ----
//
// The actual activate-the-plugin implementation lives in
// `definePlugin.ts` (it needs to run setup) and installs itself at
// module-load time via `setActivator`. This keeps the selectors →
// definePlugin import direction clean (no cycle).

type Activator = (pluginName: string) => Promise<void>;
let activator: Activator | null = null;

export function setActivator(fn: Activator): void {
  activator = fn;
}

async function activateAndRun(pluginName: string, commandId: string): Promise<void> {
  await runActivator(pluginName);
  const real = lookupCommand(commandId);
  if (!real) {
    // eslint-disable-next-line no-console
    console.warn(`[plugin] ${pluginName} activated but did not register command ${commandId}`);
    return;
  }
  await real.run();
}

/**
 * Trigger the configured activator for `pluginName`. Used by lazy
 * placeholder components (workspace views / settings panes) whose only
 * job is to nudge the plugin into running setup; once setup completes,
 * the selector hooks re-emit a list where the real component replaces
 * the placeholder.
 */
async function runActivator(pluginName: string): Promise<void> {
  if (!activator) {
    // eslint-disable-next-line no-console
    console.error(`[plugin] activator not wired; cannot lazily activate ${pluginName}`);
    return;
  }
  await activator(pluginName);
}

// ---------------------------------------------------------------------------
// AG-UI event handlers — imperative lookups used by the reducer
// ---------------------------------------------------------------------------

/** Look up a CUSTOM-event handler. Used by the reducer at event time. */
export function lookupCustomEventHandler(name: string): CustomEventHandler<unknown> | undefined {
  return usePluginStore.getState().customEventHandlers.get(name)?.value;
}

/**
 * Look up all *core* handlers registered for an AG-UI built-in event type.
 * Returned in insertion order; the reducer chains them through the state.
 */
export function lookupCoreEventHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: CoreEventHandler }> {
  const out: Array<{ pluginName: string; handler: CoreEventHandler }> = [];
  for (const o of usePluginStore.getState().coreEventHandlers.values()) {
    if (o.value.eventType === eventType) out.push({ pluginName: o.pluginName, handler: o.value.handler });
  }
  return out;
}

// ---------------------------------------------------------------------------
// Routes / shortcuts / agent sources / data providers / hooks
// ---------------------------------------------------------------------------

/** Snapshot of all registered routes, sorted by `order`. */
export function listRoutes(): RouteSpec[] {
  return Array.from(usePluginStore.getState().routes.values())
    .map((o) => o.value)
    .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
}

/** Look up a registered shortcut by canonical combo (after `normalizeCombo`). */
export function lookupShortcut(canonical: string): ShortcutSpec | undefined {
  return usePluginStore.getState().shortcuts.get(canonical)?.value;
}

/**
 * Pick the active agent source — highest priority wins, ties broken by
 * insertion order. Returns undefined if none registered.
 */
export function pickAgentSource(): AgentSourceSpec | undefined {
  const sources = Array.from(usePluginStore.getState().agentSources.values()).map((o) => o.value);
  if (sources.length === 0) return undefined;
  return sources.reduce((best, cur) =>
    (cur.priority ?? 0) > (best.priority ?? 0) ? cur : best,
  );
}

/**
 * Look up the fetcher for a data-provider key. Type is erased — callers
 * cast to their expected return shape. Returns undefined when nothing
 * registered (consumer hooks should throw or fall back).
 */
export function lookupDataProvider<T = unknown>(key: string): (() => Promise<T>) | undefined {
  const entry = usePluginStore.getState().dataProviders.get(key);
  return entry ? (entry.value.fetcher as () => Promise<T>) : undefined;
}

/** Snapshot of registered beforeRequest hooks in insertion order. */
export function listRpcBeforeHooks(): RpcBeforeRequestHook[] {
  return Array.from(usePluginStore.getState().rpcBeforeRequest.values()).map((o) => o.value);
}

/** Snapshot of registered afterResponse hooks in insertion order. */
export function listRpcAfterHooks(): RpcAfterResponseHook[] {
  return Array.from(usePluginStore.getState().rpcAfterResponse.values()).map((o) => o.value);
}

