import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import type { InspectorTab } from "@/components/inspector/types";
import type { Theme } from "@/components/sidebar/types";
import { usePluginStore } from "@/plugins/sdk";

// All cross-cutting UI state in one Zustand store.
//
// Why one store: every value here is shell-level UI (theme, layout toggles,
// active tab, inspector state) and components frequently combine them. One
// store, one subscription.
//
// Persistence policy (see `partialize` below):
//   - Theme / accent → persisted (user expects them to stick across launches)
//   - Sidebar rail + inspector open → persisted (window state should survive)
//   - Active session + open tabs → persisted (continuity)
//   - Inspector tab + active file → ephemeral (resets each launch — those
//     reference data that may not exist on next boot)
//   - Tool selection / expansion → ephemeral (purely per-session UI)

type UIState = {
  theme: Theme;
  accent: string;

  sidebarRail: boolean;
  inspectorOpen: boolean;

  activeSessionId: string;
  tabIds: string[];

  inspectorTab: InspectorTab;
  activeFile: string;

  selectedToolId: string;
  expandedToolIds: Set<string>;

  // Phase 3 — settings modal. Ephemeral: always starts closed.
  settingsModalOpen: boolean;

  /**
   * Heterogeneous chat-area tabs (Phase B).
   *
   * Each entry is an inspector (or other plugin-contributed) view that the
   * user "promoted" into the main pane to read it at full width. When
   * `activeMainView` is set, the chat panel renders that view's component
   * instead of the message stream. Selecting a chat session tab clears
   * `activeMainView`.
   */
  mainViewTabs: { id: string; title: string; icon?: string }[];
  activeMainView: string | null;
};

type UIActions = {
  setTheme: (theme: Theme) => void;
  toggleTheme: () => void;
  setAccent: (accent: string) => void;
  toggleSidebar: () => void;
  toggleInspector: () => void;
  setInspectorOpen: (open: boolean) => void;
  setInspectorTab: (tab: InspectorTab) => void;
  setActiveFile: (path: string) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  expandTool: (id: string) => void;

  selectTab: (id: string) => void;
  closeTab: (id: string) => void;
  openTab: (id: string) => void;

  openSettings: () => void;
  closeSettings: () => void;

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
      sidebarRail: false,
      inspectorOpen: false,
      activeSessionId: "s1",
      tabIds: ["s1", "s2", "s3"],
      inspectorTab: "diff",
      activeFile: "src/api/auth.ts",
      selectedToolId: "",
      expandedToolIds: new Set<string>(),
      settingsModalOpen: false,
      mainViewTabs: [],
      activeMainView: null,

      // ---- actions ----
      setTheme: (theme) => set({ theme }),
      toggleTheme: () => set((s) => ({ theme: s.theme === "dark" ? "light" : "dark" })),
      setAccent: (accent) => set({ accent }),

      toggleSidebar: () => set((s) => ({ sidebarRail: !s.sidebarRail })),
      toggleInspector: () => set((s) => ({ inspectorOpen: !s.inspectorOpen })),
      setInspectorOpen: (open) => set({ inspectorOpen: open }),

      setInspectorTab: (tab) => set({ inspectorTab: tab, inspectorOpen: true }),
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

      openSettings: () => set({ settingsModalOpen: true }),
      closeSettings: () => set({ settingsModalOpen: false }),

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
      //   - selectedToolId / activeFile / inspectorTab (data-coupled)
      partialize: (s) => ({
        theme: s.theme,
        accent: s.accent,
        sidebarRail: s.sidebarRail,
        inspectorOpen: s.inspectorOpen,
        activeSessionId: s.activeSessionId,
        tabIds: s.tabIds,
      }),
      // Bump on any breaking shape change — wipes stale stored data so a
      // dev who upgraded mid-session doesn't get hit by mismatched fields.
      version: 2,
    },
  ),
);

// Side-effect: keep <html> class and CSS var in sync with the store.
//
// Light-mode variants come from the plugin registry (`lyra.builtin.default-themes`
// registers them) — uiStore stays decoupled from the actual palette so a
// theme plugin can change which colors are available without touching this
// file. While the registry is still empty (very early in boot, before the
// plugin loads), we fall back to the dark hex unchanged. Once the plugin
// registers and the store re-fires applyTheme, the light variant kicks in.
function lookupLightVariant(darkHex: string): string | undefined {
  const accents = usePluginStore.getState().accents;
  for (const o of accents.values()) {
    if (o.value.dark === darkHex) return o.value.light ?? darkHex;
  }
  return undefined;
}

function applyTheme(theme: Theme, accent: string) {
  const root = document.documentElement;
  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${theme}`);
  const c = theme === "light" ? (lookupLightVariant(accent) ?? accent) : accent;
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

// Re-apply when the plugin registry's accent map changes — this is the path
// for "default-themes" registering at startup AFTER the persisted state has
// already triggered an initial applyTheme with an empty registry.
usePluginStore.subscribe((state, prev) => {
  if (state.accents !== prev.accents) {
    const { theme, accent } = useUIStore.getState();
    applyTheme(theme, accent);
  }
});
