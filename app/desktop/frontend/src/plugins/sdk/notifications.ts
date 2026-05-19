// Persistent notification feed — every `host.notify(...)` call appends here.
//
// Why separate from the transient toaster:
//   - the visual toast disappears after a few seconds, but users often want
//     to scroll back through what happened ("did anything fail?")
//   - inspector / settings tabs can read this without subscribing to DOM
//     events
//   - plugins can ingest the feed as a stream
//
// Capped at MAX_ENTRIES — oldest dropped first. Same store pattern as
// `usePluginErrorStore` for consistency.

import { create } from "zustand";
import type { NotificationEntry, NotificationLevel } from "./types";

const MAX_ENTRIES = 200;

type NotificationStoreState = {
  log: NotificationEntry[];
  nextId: number;
};

type NotificationStoreActions = {
  push(entry: { plugin: string; level: NotificationLevel; message: string }): NotificationEntry;
  dismiss(id: number): void;
  clearAll(): void;
};

export const useNotificationStore = create<NotificationStoreState & NotificationStoreActions>(
  (set, get) => ({
    log: [],
    nextId: 1,

    push({ plugin, level, message }) {
      const id = get().nextId;
      const entry: NotificationEntry = {
        id,
        plugin,
        level,
        message,
        timestamp: Date.now(),
      };
      const next = [...get().log, entry];
      // Cap from the front when we exceed the limit.
      const trimmed = next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
      set({ log: trimmed, nextId: id + 1 });
      return entry;
    },

    dismiss(id) {
      set({
        log: get().log.map((e) => (e.id === id ? { ...e, dismissed: true } : e)),
      });
    },

    clearAll() {
      set({ log: [] });
    },
  }),
);
