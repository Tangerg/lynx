// Ephemeral command-palette open state.
//
// Lives in the state layer (not inside the command-palette plugin) because it
// is app-wide UI state consumed beyond the palette itself: the global keymap
// reads `open` to decide whether Esc closes a workspace view or yields to the
// palette, and the `command.open` command triggers it sight-unseen. Hoisting
// it out of the plugin keeps plugin↔plugin direct imports out of the graph
// (the registry is the single seam between plugins) — the keymap reaches the
// palette's state through this shared store, not through the palette module.

import { create } from "zustand";

interface PaletteState {
  open: boolean;
  setOpen: (open: boolean) => void;
  toggle: () => void;
}

export const usePaletteStore = create<PaletteState>((set) => ({
  open: false,
  setOpen: (open) => set({ open }),
  toggle: () => set((s) => ({ open: !s.open })),
}));
