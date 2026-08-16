// The document title, composed from three independent claims on it.
//
// A working dot, an unread count and a base title are each set by a different
// plugin, and each used to write `document.title` directly — so whichever wrote
// last erased the other two. One composer owns the string; the three setters own
// a field each.
//
// Its own store rather than a slice of the plugin registry, which is where it
// used to live: the registry held it only because the registry was the one
// module every plugin could already reach.

import { create } from "zustand";

interface WindowState {
  title: string;
  badge: number;
  working: boolean;
  setTitle: (text: string) => void;
  setBadge: (n: number) => void;
  setWorking: (on: boolean) => void;
}

function compose(base: string, badge: number, working: boolean): void {
  if (typeof document === "undefined") return;
  const dot = working ? "● " : "";
  const count = badge > 0 ? `(${badge}) ` : "";
  document.title = `${dot}${count}${base || "Lyra"}`;
}

export const useWindowStore = create<WindowState>((set, get) => ({
  title: "",
  badge: 0,
  working: false,
  setTitle(text) {
    set({ title: text });
    compose(text, get().badge, get().working);
  },
  setBadge(n) {
    set({ badge: n });
    compose(get().title, n, get().working);
  },
  setWorking(on) {
    set({ working: on });
    compose(get().title, get().badge, on);
  },
}));
