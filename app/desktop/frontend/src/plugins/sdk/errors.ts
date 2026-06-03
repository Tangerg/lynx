// Plugin error log — every catch site in the plugin pipeline pushes here.
//
// Why centralize:
//   - "your plugin is broken" should be findable in one place
//   - the settings UI can show a per-plugin error list
//   - automated tests can assert against the log
//
// Sources we feed in:
//   - PluginBoundary  (render errors)
//   - loadPlugin      (setup errors)
//   - reducer         (agui handler errors)
//   - Composer.submit (slash command run errors)

import { create } from "zustand";

export type PluginErrorSource = "setup" | "render" | "events" | "command" | "other";

export interface PluginError {
  id: number;
  timestamp: number;
  plugin: string;
  source: PluginErrorSource;
  message: string;
  /** Optional component stack / call site. */
  detail?: string;
}

interface ErrorStoreState {
  log: PluginError[];
  /** Monotonic counter for stable React keys. */
  nextId: number;
}

interface ErrorStoreActions {
  push: (e: Omit<PluginError, "id" | "timestamp">) => void;
  clearFor: (plugin: string) => void;
  clearAll: () => void;
}

// Cap matches useNotificationStore — oldest dropped first, FIFO. A
// pathologically buggy plugin that pushes on every event would otherwise
// grow the log + every consumer's render scope without bound.
const MAX_ENTRIES = 200;

export const usePluginErrorStore = create<ErrorStoreState & ErrorStoreActions>((set, get) => ({
  log: [],
  nextId: 1,

  push({ plugin, source, message, detail }) {
    const id = get().nextId;
    const next = [...get().log, { id, timestamp: Date.now(), plugin, source, message, detail }];
    const trimmed = next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
    set({ log: trimmed, nextId: id + 1 });
  },

  clearFor(plugin) {
    set({ log: get().log.filter((e) => e.plugin !== plugin) });
  },

  clearAll() {
    set({ log: [] });
  },
}));

// Convenience imperative helper for non-React callers (the reducer, the
// composer's command runner).
export function reportPluginError(
  plugin: string,
  source: PluginErrorSource,
  err: unknown,
  detail?: string,
): void {
  const message = err instanceof Error ? err.message : String(err);
  usePluginErrorStore.getState().push({ plugin, source, message, detail });
}

// Run `fn` in a try/catch and log to console with a tag on failure. Used
// throughout the plugin pipeline so a misbehaving subscriber / disposable
// / lifecycle hook can't crash the kernel.
export function safeCall(fn: () => void, tag: string): void {
  try {
    fn();
  } catch (err) {
    console.error(tag, err);
  }
}
