import { create } from "zustand";

// Which surface fills the content card: the chat (null) or one workspace view
// promoted to the whole card.
//
// There is no list of "open" views. A previous version kept one, and since
// nothing ever rendered it, closing a full view silently reopened whichever
// view had been promoted before it — an invisible tab stack driving navigation.
// Closing a view returns to the chat, which is the only other surface there is.
interface WorkspaceSurfaceState {
  activeMainView: string | null;
  settingsPane: string | null;
}

interface WorkspaceSurfaceActions {
  setSettingsPane: (pane: string | null) => void;
  openMainView: (id: string) => void;
  /** Close `id` if it is the surface on screen; a stale id is a no-op. */
  closeMainView: (id: string) => void;
  selectChat: () => void;
}

export const useWorkspaceSurfaceStore = create<WorkspaceSurfaceState & WorkspaceSurfaceActions>(
  (set, get) => ({
    activeMainView: null,
    settingsPane: null,

    setSettingsPane: (pane) => set({ settingsPane: pane }),
    openMainView: (id) => set({ activeMainView: id }),
    closeMainView: (id) => {
      if (get().activeMainView === id) set({ activeMainView: null });
    },
    selectChat: () => set({ activeMainView: null }),
  }),
);
