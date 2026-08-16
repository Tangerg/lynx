// Persistent notification feed — every `host.notify(...)` call appends here.
//
// Why separate from the transient toaster:
//   - the visual toast disappears after a few seconds, but users often want
//     to scroll back through what happened ("did anything fail?")
//   - workspace views / settings panes can read this without subscribing to DOM
//     events
//   - plugins can ingest the feed as a stream
//
// Capped at MAX_ENTRIES — oldest dropped first. Same store pattern as
// `usePluginErrorStore` for consistency.

import type { NotificationEntry, NotificationLevel } from "./types";
import { dispatchToast } from "./hostToast";
import { toast } from "sonner";
import { create } from "zustand";

const MAX_ENTRIES = 200;

interface NotificationStoreState {
  log: NotificationEntry[];
  nextId: number;
}

interface NotificationStoreActions {
  push: (entry: { plugin: string; level: NotificationLevel; message: string }) => NotificationEntry;
  dismiss: (id: number) => void;
  clearAll: () => void;
}

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

// --------------------------------------------------------------------------
// App-side notification service — the non-plugin twin of host.notify
// (plugins/sdk/host.ts). Same contract: a durable entry in the
// notification feed (the Notifications workspace view) PLUS a transient
// toast. The feed exists exactly so users can scroll back through "did
// anything fail?" — an error that only toasts vanishes when dismissed.
//
// Success confirmations ("Copied", "Imported …") stay toast-only
// (toast.success directly): they're feedback on an action the user just
// watched succeed, not events worth re-reading.

/**
 * Who the feed credits an app-side notification to.
 *
 * A closed set, and an identifier rather than copy: it renders in the same column
 * as a plugin's name (`host.notify` credits the plugin that called it), so it
 * stays untranslated for the same reason plugin ids do — a mixed column reads as
 * neither. Closed because it was `string`, and the same word was spelled at up to
 * five callsites: one typo would have opened a second, silent attribution bucket
 * that looks like a new subsystem in the feed.
 */
export type NotifySource =
  | "composer"
  | "events"
  | "goal"
  | "import"
  | "mcp"
  | "knowledge"
  | "project"
  | "render"
  | "session"
  | "setup"
  | "skills";

export interface NotifyOptions {
  /** Secondary line on the toast; folded into the feed entry's message. */
  description?: string;
  /** Feed attribution (the Notifications view's "{source} · time" line).
   *  Defaults to "app". */
  source?: NotifySource;
}

// The app's own notify helpers speak two of the feed's three levels — there is no
// notifyWarn, so "warn" reaches the feed only from a plugin's host.notify. Stated
// as a narrowing of the feed's vocabulary rather than a second list of words.
type Level = Extract<NotificationLevel, "info" | "error">;

const TOAST_BY_LEVEL: Record<Level, typeof toast.info> = {
  info: toast.info,
  error: toast.error,
};

function notify(level: Level, message: string, opts?: NotifyOptions): void {
  useNotificationStore.getState().push({
    plugin: opts?.source ?? "app",
    level,
    message: opts?.description ? `${message} — ${opts.description}` : message,
  });
  TOAST_BY_LEVEL[level](message, opts?.description ? { description: opts.description } : undefined);
}

export function notifyInfo(message: string, opts?: NotifyOptions): void {
  notify("info", message, opts);
}
export function notifyError(message: string, opts?: NotifyOptions): void {
  notify("error", message, opts);
}

/**
 * A plugin's own notification, attributed to it in the feed. Reaches "warn",
 * which the app-side helpers above deliberately don't.
 *
 * The feed entry is written before the toast is dispatched, so anything reacting
 * to the toast can already cross-reference it. The toast goes out as an event
 * rather than a `toast()` call so a plugin's notification path pulls no React
 * portal machinery into the SDK.
 */
export function notifyFrom(plugin: string, message: string, level: NotificationLevel): void {
  useNotificationStore.getState().push({ plugin, level, message });
  dispatchToast(message, level);
}
