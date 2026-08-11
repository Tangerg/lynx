// Ephemeral open state for the session search use case. The overlay, keyboard
// shortcut, and Work Index launcher are three adapters for the same action, so
// they meet here instead of importing one another.

import { create } from "zustand";

interface SessionSearchState {
  open: boolean;
  setOpen: (open: boolean) => void;
  show: () => void;
  toggle: () => void;
}

export const useSessionSearchStore = create<SessionSearchState>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
  show: () => set({ open: true }),
  toggle: () => set((state) => ({ open: !state.open })),
}));
