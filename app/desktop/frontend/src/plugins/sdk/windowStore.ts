// The document title, composed from three independent claims on it.
//
// A working dot, an unread count and a base title are each set by a different
// plugin. One composer owns the string; the three setters own one field each so
// no claim can erase another.
//
// Its own store rather than a plugin-registry slice: title composition is window
// state, not extension registration.

import { create } from "zustand";
import { PRODUCT_NAME } from "@/product";

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
  document.title = `${dot}${count}${base || PRODUCT_NAME}`;
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
