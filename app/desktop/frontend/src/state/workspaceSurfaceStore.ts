import { create } from "zustand";

export interface WorkspaceSurfaceTab {
  id: string;
  title: string;
  icon?: string;
}

interface WorkspaceSurfaceState {
  mainViewTabs: WorkspaceSurfaceTab[];
  activeMainView: string | null;
  settingsPane: string | null;
}

interface WorkspaceSurfaceActions {
  setSettingsPane: (pane: string | null) => void;
  ensureMainViewTab: (tab: WorkspaceSurfaceTab) => void;
  openMainView: (tab: WorkspaceSurfaceTab) => void;
  closeMainView: (id: string) => void;
  selectChat: () => void;
}

export const useWorkspaceSurfaceStore = create<WorkspaceSurfaceState & WorkspaceSurfaceActions>(
  (set, get) => ({
    mainViewTabs: [],
    activeMainView: null,
    settingsPane: null,

    setSettingsPane: (pane) => set({ settingsPane: pane }),
    ensureMainViewTab: (tab) => {
      const cur = get().mainViewTabs;
      if (cur.some((t) => t.id === tab.id)) return;
      set({ mainViewTabs: [...cur, tab] });
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
      set({
        mainViewTabs: next,
        activeMainView:
          get().activeMainView === id ? (next.at(-1)?.id ?? null) : get().activeMainView,
      });
    },
    selectChat: () => set({ activeMainView: null }),
  }),
);
