import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { Theme } from "@/components/sidebar/types";
// Import the registry store directly rather than via the SDK barrel —
// the barrel pulls in definePlugin / host, and host.ts already imports
// this very file. Going through the barrel creates a real cycle that
// shows up as a TDZ at module-init time under the Vitest loader.
import { usePluginStore } from "@/plugins/sdk/registry";

// All cross-cutting UI state in one Zustand store.
//
// Why one store: every value here is kernel-level UI (theme, layout toggles,
// active tab, …) and components frequently combine them. One store, one
// subscription.
//
// Persistence policy (see `partialize` below):
//   - Theme / accent → persisted (user expects them to stick across launches)
//   - Sidebar rail → persisted (window state should survive)
//   - Active session + open tabs → persisted (continuity)
//   - Active file → ephemeral (references data that may not exist next boot)
//   - Tool selection / expansion → ephemeral (purely per-session UI)

type UIState = {
  theme: Theme;
  accent: string;

  sidebarRail: boolean;

  activeSessionId: string;
  tabIds: string[];

  activeFile: string;

  selectedToolId: string;
  expandedToolIds: Set<string>;

  /**
   * Heterogeneous chat-area tabs.
   *
   * Each entry is a workspace view the user "promoted" into the main
   * pane to read at full width. When `activeMainView` is set, the chat
   * panel renders that view's component instead of the message stream.
   * Selecting a chat session tab clears `activeMainView`.
   */
  mainViewTabs: { id: string; title: string; icon?: string }[];
  activeMainView: string | null;
};

type UIActions = {
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
  setAccent: (accent: string) => void;
  toggleSidebar: () => void;
  setActiveFile: (path: string) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  expandTool: (id: string) => void;

  selectTab: (id: string) => void;
  closeTab: (id: string) => void;
  openTab: (id: string) => void;

  /** Add (if absent) and focus a workspace view in the chat-area tab strip. */
  openMainView: (tab: { id: string; title: string; icon?: string }) => void;
  /** Remove a workspace view tab; falls back to chat if it was active. */
  closeMainView: (id: string) => void;
  /** Focus a workspace view tab without opening a new one. */
  selectMainView: (id: string) => void;
  /** Clear the workspace view focus so the chat session takes over again. */
  selectChat: () => void;
};

export const useUIStore = create<UIState & UIActions>()(
  persist(
    (set, get) => ({
      // ---- initial state ----
      theme: "dark",
      accent: "#1ed760",
      sidebarRail: true,
      activeSessionId: "s1",
      tabIds: ["s1", "s2", "s3"],
      activeFile: "src/api/auth.ts",
      selectedToolId: "",
      expandedToolIds: new Set<string>(),
      mainViewTabs: [],
      activeMainView: null,

      // ---- actions ----
      setTheme: (theme) => set({ theme }),
      // Flip to the opposite SCHEME (not just "dark"/"light" id) so custom
      // theme plugins still toggle sensibly. Pick the first registered
      // theme whose scheme is the opposite of the current one; if none
      // exists (e.g. registry only has dark themes), no-op.
      toggleTheme: () => {
        const cur = get().theme;
        const themes = usePluginStore.getState().themes;
        const curSpec = themes.get(cur)?.value;
        const curScheme = curSpec?.scheme ?? (cur === "light" ? "light" : "dark");
        const target = curScheme === "dark" ? "light" : "dark";
        // Sort by `order` so the toggle picks the "primary" theme of the
        // opposite scheme rather than whichever Map happens to enumerate
        // first. Matches the sort the appearance pane uses.
        const candidates = Array.from(themes.values())
          .map((o) => o.value)
          .filter((t) => t.scheme === target)
          .sort((a, b) => (a.order ?? 100) - (b.order ?? 100));
        if (candidates[0]) set({ theme: candidates[0].id });
      },
      setAccent: (accent) => set({ accent }),

      toggleSidebar: () => set((s) => ({ sidebarRail: !s.sidebarRail })),

      setActiveFile: (path) => set({ activeFile: path }),

      setSelectedToolId: (id) => set({ selectedToolId: id }),
      toggleExpandedTool: (id) => {
        const next = new Set(get().expandedToolIds);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        set({ expandedToolIds: next });
      },
      expandTool: (id) => {
        const next = new Set(get().expandedToolIds);
        next.add(id);
        set({ expandedToolIds: next, selectedToolId: id });
      },

      selectTab: (id) => {
        const { tabIds } = get();
        set({
          activeSessionId: id,
          tabIds: tabIds.includes(id) ? tabIds : [...tabIds, id],
        });
      },
      closeTab: (id) => {
        const { tabIds, activeSessionId } = get();
        const next = tabIds.filter((x) => x !== id);
        set({
          tabIds: next,
          activeSessionId: id === activeSessionId && next.length > 0 ? next[0] : activeSessionId,
        });
      },
      openTab: (id) => {
        const { tabIds } = get();
        if (!tabIds.includes(id)) set({ tabIds: [...tabIds, id] });
      },

      openMainView: (tab) => {
        const cur = get().mainViewTabs;
        const exists = cur.some((t) => t.id === tab.id);
        set({
          mainViewTabs: exists ? cur : [...cur, tab],
          activeMainView: tab.id,
        });
      },
      closeMainView: (id) => {
        const cur = get().mainViewTabs;
        const next = cur.filter((t) => t.id !== id);
        const activeMainView =
          get().activeMainView === id ? (next[next.length - 1]?.id ?? null) : get().activeMainView;
        set({ mainViewTabs: next, activeMainView });
      },
      selectMainView: (id) => set({ activeMainView: id }),
      selectChat: () => set({ activeMainView: null }),
    }),
    {
      name: "lyra.ui",
      storage: createJSONStorage(() => localStorage),
      // Whitelist what we persist. `partialize` projects the slice we want.
      // Things we deliberately leave out:
      //   - expandedToolIds (Set: hard to serialize + ephemeral by nature)
      //   - selectedToolId / activeFile (data-coupled to a running agent)
      partialize: (s) => ({
        theme: s.theme,
        accent: s.accent,
        sidebarRail: s.sidebarRail,
        activeSessionId: s.activeSessionId,
        tabIds: s.tabIds,
      }),
      // Bump on any breaking shape change — wipes stale stored data so a
      // dev who upgraded mid-session doesn't get hit by mismatched fields.
      // v4: sidebarRail default flipped to true (Linear/Cursor convention —
      // rail is the keyboard-driven default; full sidebar is on-demand).
      version: 4,
    },
  ),
);

// Side-effect: keep <html> class + inline CSS vars in sync with the
// active theme spec from the plugin registry.
//
// Theme model — IDE/VS Code style:
//   1. A plugin (default: `lyra.builtin.default-themes`) registers one or
//      more ThemeSpec entries. Each carries a `tokens` map: CSS variable
//      name → value.
//   2. When `theme` changes (or the registry's theme map mutates because
//      a plugin registered late), we look up the spec, toggle the
//      `theme-{scheme}` class on <html> so structural CSS still applies,
//      and write every token to `:root.style` as an inline override.
//   3. Until the plugin registers, the tokens declared in `tokens.css`
//      (`:root`) take effect as a first-paint fallback. The fallback
//      values match the dark theme — light users see a brief dark flash
//      before the plugin registers and inline tokens kick in.
//
// Accent works the same way: the accent picker stores a hex; we resolve
// to the light variant via the accent registry when the active theme's
// scheme is "light".

function lookupLightVariant(darkHex: string): string | undefined {
  const accents = usePluginStore.getState().accents;
  for (const o of accents.values()) {
    if (o.value.dark === darkHex) return o.value.light ?? darkHex;
  }
  return undefined;
}

function applyTheme(theme: Theme, accent: string) {
  const root = document.documentElement;
  const spec = usePluginStore.getState().themes.get(theme)?.value;

  // Scheme drives the structural class. If the spec isn't registered yet
  // we fall back to the id itself — for built-in ids ("dark"/"light")
  // that's still right; for custom ids it's the best we can do until the
  // plugin loads and we re-fire.
  const scheme = spec?.scheme ?? (theme === "light" ? "light" : "dark");
  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  // Write all of the theme's tokens as inline vars. Inline beats stylesheet
  // declarations, so this lets the plugin own the palette regardless of
  // what the fallback CSS in tokens.css says.
  if (spec?.tokens) {
    for (const [name, value] of Object.entries(spec.tokens)) {
      root.style.setProperty(`--${name}`, value);
    }
  }

  // Accent override last so the user's accent pick beats the theme's
  // default --color-accent token.
  const c = scheme === "light" ? (lookupLightVariant(accent) ?? accent) : accent;
  root.style.setProperty("--color-accent", c);
}

// Initial sync + subscription. The persist middleware rehydrates synchronously
// on store creation, so getState() here already reflects the persisted values.
applyTheme(useUIStore.getState().theme, useUIStore.getState().accent);
useUIStore.subscribe((state, prev) => {
  if (state.theme !== prev.theme || state.accent !== prev.accent) {
    applyTheme(state.theme, state.accent);
  }
});

// Re-apply when the plugin registry's theme or accent maps change — this
// is the path that handles built-in plugins registering at startup AFTER
// the persisted-state init above already fired applyTheme with an empty
// registry, AND the path for theme plugins loaded/unloaded at runtime.
usePluginStore.subscribe((state, prev) => {
  if (state.themes !== prev.themes || state.accents !== prev.accents) {
    const { theme, accent } = useUIStore.getState();
    applyTheme(theme, accent);
  }
});
