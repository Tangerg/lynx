// Session-scoped UI state: open chat-session tabs, promoted workspace
// view tabs, current file the user is looking at, and per-session tool
// inspector state (selected / expanded ids).
//
// Persistence policy:
//   - Persisted: activeSessionId + tabIds (continuity across launches).
//   - Ephemeral: mainViewTabs, activeMainView, activeFile,
//     selectedToolId, expandedToolIds — all reference data that may not
//     exist or may have changed on next boot.

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

interface MainViewTab { id: string; title: string; icon?: string }

interface SessionState {
  activeSessionId: string;
  tabIds: string[];

  /**
   * Heterogeneous chat-area tabs.
   *
   * Each entry is a workspace view the user "promoted" into the main
   * pane to read at full width. When `activeMainView` is set, the chat
   * panel renders that view's component instead of the message stream.
   * Selecting a chat session tab clears `activeMainView`.
   */
  mainViewTabs: MainViewTab[];
  activeMainView: string | null;

  activeFile: string;
  selectedToolId: string;
  expandedToolIds: Set<string>;
}

interface SessionActions {
  selectTab: (id: string) => void;
  closeTab: (id: string) => void;
  openTab: (id: string) => void;

  /** Close every chat tab except `id`. */
  closeOtherTabs: (id: string) => void;
  /** Close every chat tab whose position precedes `id` in `tabIds`. */
  closeTabsLeftOf: (id: string) => void;
  /** Close every chat tab whose position follows `id` in `tabIds`. */
  closeTabsRightOf: (id: string) => void;
  /** Close every chat tab. */
  closeAllTabs: () => void;

  /** Add (if absent) and focus a workspace view in the chat-area tab strip. */
  openMainView: (tab: MainViewTab) => void;
  /** Remove a workspace view tab; falls back to chat if it was active. */
  closeMainView: (id: string) => void;
  /** Focus a workspace view tab without opening a new one. */
  selectMainView: (id: string) => void;
  /** Clear the workspace view focus so the chat session takes over again. */
  selectChat: () => void;

  /** Close every workspace-view tab except `id`. */
  closeOtherMainViews: (id: string) => void;
  /** Close every workspace-view tab whose position precedes `id`. */
  closeMainViewsLeftOf: (id: string) => void;
  /** Close every workspace-view tab whose position follows `id`. */
  closeMainViewsRightOf: (id: string) => void;
  /** Close every workspace-view tab. */
  closeAllMainViews: () => void;

  setActiveFile: (path: string) => void;
  setSelectedToolId: (id: string) => void;
  toggleExpandedTool: (id: string) => void;
  expandTool: (id: string) => void;
}

export const useSessionStore = create<SessionState & SessionActions>()(
  persist(
    (set, get) => ({
      activeSessionId: "s1",
      tabIds: ["s1", "s2", "s3"],
      mainViewTabs: [],
      activeMainView: null,
      activeFile: "src/api/auth.ts",
      selectedToolId: "",
      expandedToolIds: new Set<string>(),

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

      // Bulk close helpers — all preserve `activeSessionId` when the
      // active tab survives, otherwise fall back to the leftmost
      // remaining tab (or empty string when nothing is left, mirroring
      // the original closeTab semantics).
      closeOtherTabs: (id) => {
        const { tabIds } = get();
        if (!tabIds.includes(id)) return;
        set({ tabIds: [id], activeSessionId: id });
      },
      closeTabsLeftOf: (id) => {
        const { tabIds, activeSessionId } = get();
        const idx = tabIds.indexOf(id);
        if (idx <= 0) return;
        const next = tabIds.slice(idx);
        set({
          tabIds: next,
          activeSessionId: next.includes(activeSessionId) ? activeSessionId : id,
        });
      },
      closeTabsRightOf: (id) => {
        const { tabIds, activeSessionId } = get();
        const idx = tabIds.indexOf(id);
        if (idx === -1 || idx === tabIds.length - 1) return;
        const next = tabIds.slice(0, idx + 1);
        set({
          tabIds: next,
          activeSessionId: next.includes(activeSessionId) ? activeSessionId : id,
        });
      },
      closeAllTabs: () => {
        set({ tabIds: [], activeSessionId: "" });
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

      // Same shape as the chat-tab bulk close helpers, scoped to the
      // workspace-view strip.
      closeOtherMainViews: (id) => {
        const cur = get().mainViewTabs;
        const target = cur.find((t) => t.id === id);
        if (!target) return;
        set({ mainViewTabs: [target], activeMainView: id });
      },
      closeMainViewsLeftOf: (id) => {
        const { mainViewTabs, activeMainView } = get();
        const idx = mainViewTabs.findIndex((t) => t.id === id);
        if (idx <= 0) return;
        const next = mainViewTabs.slice(idx);
        set({
          mainViewTabs: next,
          activeMainView:
            activeMainView && next.some((t) => t.id === activeMainView) ? activeMainView : id,
        });
      },
      closeMainViewsRightOf: (id) => {
        const { mainViewTabs, activeMainView } = get();
        const idx = mainViewTabs.findIndex((t) => t.id === id);
        if (idx === -1 || idx === mainViewTabs.length - 1) return;
        const next = mainViewTabs.slice(0, idx + 1);
        set({
          mainViewTabs: next,
          activeMainView:
            activeMainView && next.some((t) => t.id === activeMainView) ? activeMainView : id,
        });
      },
      closeAllMainViews: () => {
        set({ mainViewTabs: [], activeMainView: null });
      },

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
    }),
    {
      name: "lyra.session",
      storage: createJSONStorage(() => localStorage),
      // Persist only the continuity fields. Tool inspector + main views
      // + activeFile are intentionally session-scoped (the underlying
      // data may not exist on next boot).
      partialize: (s) => ({
        activeSessionId: s.activeSessionId,
        tabIds: s.tabIds,
      }),
      version: 1,
    },
  ),
);
