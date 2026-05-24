// Long-running task tracker. `host.tasks.start(...)` registers an entry
// here; the kernel's status bar reads it to show "N running" + the latest
// label. Settled tasks linger briefly so the user sees the final state
// before they vanish.

import { create } from "zustand";

export type TaskStatus = "running" | "succeeded" | "failed";

export type TaskEntry = {
  id: string;
  pluginName: string;
  label: string;
  /** 0..1 for determinate progress; null for indeterminate (spinner). */
  progress: number | null;
  /** Sub-line shown under the label (optional). */
  message: string | null;
  status: TaskStatus;
  /** Populated when `status === "failed"`. */
  error?: string;
  startedAt: number;
  /** Set on terminal transitions; the store removes the entry shortly after. */
  settledAt?: number;
};

type TasksState = {
  tasks: Map<string, TaskEntry>;
};

type TasksActions = {
  add(entry: TaskEntry): void;
  patch(id: string, next: Partial<TaskEntry>): void;
  remove(id: string): void;
};

export const useTasksStore = create<TasksState & TasksActions>((set) => ({
  tasks: new Map(),
  add: (entry) =>
    set((s) => {
      const next = new Map(s.tasks);
      next.set(entry.id, entry);
      return { tasks: next };
    }),
  patch: (id, partial) =>
    set((s) => {
      const prev = s.tasks.get(id);
      if (!prev) return s;
      const next = new Map(s.tasks);
      next.set(id, { ...prev, ...partial });
      return { tasks: next };
    }),
  remove: (id) =>
    set((s) => {
      if (!s.tasks.has(id)) return s;
      const next = new Map(s.tasks);
      next.delete(id);
      return { tasks: next };
    }),
}));

// How long settled tasks linger before auto-removal — long enough for the
// user to catch the success/error flash, short enough that the status bar
// doesn't pile up with old work.
export const TASK_LINGER_MS = 2400;
