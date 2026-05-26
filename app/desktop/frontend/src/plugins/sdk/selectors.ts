// Read side of the plugin registry — React hooks + imperative lookups
// + lazy-activation helpers (placeholder → real component / handler).

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
  LocaleSpec,
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
import type { ContentBlockKind } from "@/protocol/agui/viewState";
import { useMemo } from "react";
import { makeLazyActivator } from "../LazyActivator";
import { usePluginStore } from "./registry";

// ---------------------------------------------------------------------------
// Shared list-selector helper
// ---------------------------------------------------------------------------
//
// Most "list every X contributed by plugins" hooks share the same shape:
// pluck `value` off each owned entry and sort by ascending `order` (default
// 100 when unset). `useSortedList` factors that out so each registry-backed
// selector stays a one-liner. The Map identity gates re-derivation — see
// the useSlashCommands note below for the useMemo-on-Map discipline.

interface Owned<T> { value: T; pluginName: string }
interface Ordered { order?: number }

// Plain-function counterpart of useSortedList — pulls the value off
// each owned entry. Used by imperative lookups (`listRoutes`,
// `pick*`, RPC hook lists) that aren't allowed to call hooks.
function mapOwned<T>(map: Map<string, Owned<T>>): T[] {
  return Array.from(map.values()).map((o) => o.value);
}

// Lazily-built secondary index over an Owned<T> source map. The
// registry produces a fresh Map on every add/remove, so caching on
// the source Map reference auto-invalidates on mutation. Subsequent
// lookups against the same epoch are O(1).
//
// Used to flip three hot-path scans (lookupCoreEventHandlers,
// lookupCustomEventHandlers, useLayoutSlot) from O(n) per AG-UI
// event / Slot render into O(n) once per registry mutation.
function createIndex<S, V>(extract: (owned: Owned<S>) => { key: string; value: V }) {
  const cache = new WeakMap<Map<string, Owned<S>>, Map<string, V[]>>();
  return (source: Map<string, Owned<S>>): Map<string, V[]> => {
    let idx = cache.get(source);
    if (idx) return idx;
    idx = new Map();
    for (const o of source.values()) {
      const { key, value } = extract(o);
      const list = idx.get(key);
      if (list) list.push(value);
      else idx.set(key, [value]);
    }
    cache.set(source, idx);
    return idx;
  };
}

function useSortedList<T extends Ordered>(map: Map<string, Owned<T>>): T[] {
  return useMemo(
    () => mapOwned(map).sort((a, b) => (a.order ?? 100) - (b.order ?? 100)),
    [map],
  );
}

// Merge two ownership maps by id and sort by order. Used for the three
// surfaces that have both a "declared placeholder" (rendered until the
// owning plugin activates) and a "registered real" (the activated
// component). Registered entries win when ids collide.
function useDeclaredMerged<D extends { id: string }, R extends { id: string } & Ordered>(
  registered: Map<string, Owned<R>>,
  declared: Map<string, Owned<D>>,
  declaredToReal: (d: D, pluginName: string) => R,
): R[] {
  return useMemo(() => {
    const byId = new Map<string, R>();
    for (const o of declared.values()) byId.set(o.value.id, declaredToReal(o.value, o.pluginName));
    for (const o of registered.values()) byId.set(o.value.id, o.value);
    return Array.from(byId.values()).sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
  }, [registered, declared, declaredToReal]);
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
  return useDeclaredMerged(registered, declared, declaredToWorkspaceView);
}

function declaredToWorkspaceView(d: ContributedView, pluginName: string): WorkspaceViewSpec {
  return {
    ...d,
    component: makeLazyActivator(d.title, () => {
      void runActivator(pluginName);
    }),
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
  const specs = mapOwned(usePluginStore.getState().pluginErrorFallbacks);
  if (specs.length === 0) return undefined;
  return specs.reduce((best, cur) => ((cur.priority ?? 0) >= (best.priority ?? 0) ? cur : best));
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
  return useDeclaredMerged(registered, declared, declaredToSettingsPane);
}

function declaredToSettingsPane(d: ContributedSettingsPane, pluginName: string): SettingsPaneSpec {
  return {
    ...d,
    component: makeLazyActivator(d.label, () => {
      void runActivator(pluginName);
    }),
  };
}

// ---------------------------------------------------------------------------
// Layout / theme / composer / sidebar
// ---------------------------------------------------------------------------

const layoutBySlot = createIndex<{ slot: string; spec: LayoutSlotSpec }, LayoutSlotSpec>(
  (o) => ({ key: o.value.slot, value: o.value.spec }),
);

export function useLayoutSlot(slot: string): LayoutSlotSpec[] {
  const map = usePluginStore((s) => s.layoutSlots);
  return useMemo(
    () =>
      [...(layoutBySlot(map).get(slot) ?? [])].sort(
        (a, b) => (a.order ?? 100) - (b.order ?? 100),
      ),
    [map, slot],
  );
}

export function useThemes(): ThemeSpec[] {
  return useSortedList(usePluginStore((s) => s.themes));
}

export function useAccents(): ThemeAccentSpec[] {
  return useSortedList(usePluginStore((s) => s.accents));
}

export function useLocales(): LocaleSpec[] {
  return useSortedList(usePluginStore((s) => s.locales));
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
  const specs = mapOwned(usePluginStore.getState().composerPlaceholders);
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
  return useDeclaredMerged(registered, declared, declaredToPlaceholder);
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
let pluginActivator: Activator | null = null;

export function setActivator(fn: Activator): void {
  pluginActivator = fn;
}

async function activateAndRun(pluginName: string, commandId: string): Promise<void> {
  await runActivator(pluginName);
  const real = lookupCommand(commandId);
  if (!real) {
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
  if (!pluginActivator) {
    console.error(`[plugin] activator not wired; cannot lazily activate ${pluginName}`);
    return;
  }
  await pluginActivator(pluginName);
}

// ---------------------------------------------------------------------------
// AG-UI event handlers — imperative lookups used by the reducer
// ---------------------------------------------------------------------------

const customByName = createIndex<
  { name: string; handler: CustomEventHandler<unknown> },
  { pluginName: string; handler: CustomEventHandler<unknown> }
>((o) => ({
  key: o.value.name,
  value: { pluginName: o.pluginName, handler: o.value.handler },
}));

const coreByType = createIndex<
  { eventType: string; handler: CoreEventHandler },
  { pluginName: string; handler: CoreEventHandler }
>((o) => ({
  key: o.value.eventType,
  value: { pluginName: o.pluginName, handler: o.value.handler },
}));

/**
 * Look up every CUSTOM-event handler registered for `name`, in registration
 * order. The reducer fans the event out through all of them, chaining each
 * handler's StateUpdate return through the state.
 */
export function lookupCustomEventHandlers(
  name: string,
): Array<{ pluginName: string; handler: CustomEventHandler<unknown> }> {
  return customByName(usePluginStore.getState().customEventHandlers).get(name) ?? [];
}

/**
 * Look up all *core* handlers registered for an AG-UI built-in event type.
 * Returned in insertion order; the reducer chains them through the state.
 */
export function lookupCoreEventHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: CoreEventHandler }> {
  return coreByType(usePluginStore.getState().coreEventHandlers).get(eventType) ?? [];
}

// ---------------------------------------------------------------------------
// Routes / shortcuts / agent sources / data providers / hooks
// ---------------------------------------------------------------------------

/** Snapshot of all registered routes, sorted by `order`. */
export function listRoutes(): RouteSpec[] {
  return mapOwned(usePluginStore.getState().routes).sort(
    (a, b) => (a.order ?? 100) - (b.order ?? 100),
  );
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
  const sources = mapOwned(usePluginStore.getState().agentSources);
  if (sources.length === 0) return undefined;
  return sources.reduce((best, cur) => ((cur.priority ?? 0) > (best.priority ?? 0) ? cur : best));
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
  return mapOwned(usePluginStore.getState().rpcBeforeRequest);
}

/** Snapshot of registered afterResponse hooks in insertion order. */
export function listRpcAfterHooks(): RpcAfterResponseHook[] {
  return mapOwned(usePluginStore.getState().rpcAfterResponse);
}
