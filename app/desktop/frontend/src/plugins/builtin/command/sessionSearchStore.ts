// Ephemeral open state for the session search overlay.
//
// Outside the plugin that renders it, as its predecessor was: the shortcut
// contribution toggles it and the overlay reads it, and routing that through a
// shared store keeps plugin↔plugin imports out of the graph — the registry is the
// single seam between plugins.

import { create } from "zustand";

interface SessionSearchState {
  open: boolean;
  setOpen: (open: boolean) => void;
  toggle: () => void;
}

export const useSessionSearchStore = create<SessionSearchState>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
  toggle: () => set((state) => ({ open: !state.open })),
}));
