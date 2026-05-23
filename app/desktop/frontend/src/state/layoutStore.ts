// Layout chrome state — currently just the sidebar rail/expanded toggle.
//
// Persisted so window state survives across launches (Linear/Cursor
// convention: rail = keyboard-driven default).

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import { migrateLegacyUIStore } from "./_legacyMigration";

migrateLegacyUIStore();

type LayoutState = {
  /** True = collapsed rail. False = expanded sidebar. */
  sidebarRail: boolean;
};

type LayoutActions = {
  toggleSidebar: () => void;
};

export const useLayoutStore = create<LayoutState & LayoutActions>()(
  persist(
    (set) => ({
      sidebarRail: true,
      toggleSidebar: () => set((s) => ({ sidebarRail: !s.sidebarRail })),
    }),
    {
      name: "lyra.layout",
      storage: createJSONStorage(() => localStorage),
      version: 1,
    },
  ),
);
