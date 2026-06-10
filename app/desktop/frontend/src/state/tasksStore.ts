// Long-running task tracker. `host.tasks.start(...)` registers an entry
// here; the kernel's status bar reads it to show "N running" + the latest
// label. Settled tasks linger briefly so the user sees the final state
// before they vanish.

import type { TaskHandle, TaskStartOptions } from "@/plugins/sdk/types/infra";
import { nanoid } from "nanoid";
import { create } from "zustand";

export type TaskStatus = "running" | "succeeded" | "failed";

export interface TaskEntry {
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
}

interface TasksState {
  tasks: Map<string, TaskEntry>;
}

interface TasksActions {
  add: (entry: TaskEntry) => void;
  patch: (id: string, next: Partial<TaskEntry>) => void;
  remove: (id: string) => void;
}

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
const TASK_LINGER_MS = 2400;

// Imperative entrypoint used by `host.tasks.start`. Kept here (not in
// host.ts) so the lifecycle — id minting, terminal-state guarding,
// auto-removal timer — can be tested without standing up a Host.
export function startTask(pluginName: string, opts: TaskStartOptions): TaskHandle {
  const store = useTasksStore.getState();
  const id = opts.id ?? `task:${pluginName}:${nanoid(8)}`;
  // Generation stamp: ids are a supported cross-call handle, so a restart can
  // reuse this id with a FRESH entry. The old handle (and its timers) must
  // only ever touch the generation it created — startedAt is the marker.
  const startedAt = Date.now();
  const isMine = (cur: TaskEntry | undefined): cur is TaskEntry =>
    cur !== undefined && cur.startedAt === startedAt;

  store.add({
    id,
    pluginName,
    label: opts.label,
    message: opts.message ?? null,
    progress: opts.progress ?? null,
    status: "running",
    startedAt,
  });

  // Mark settled + schedule removal. Guards against double-settle so a
  // late `succeed()` after `fail()` (or vice versa) is a silent no-op.
  const settle = (status: "succeeded" | "failed", patch: Partial<TaskEntry>): void => {
    const cur = useTasksStore.getState().tasks.get(id);
    if (!isMine(cur) || cur.status !== "running") return;
    const settledAt = Date.now();
    useTasksStore.getState().patch(id, { ...patch, status, settledAt });
    // The linger timer removes only THE settle it was armed for — a
    // restarted task reusing this id must not be deleted mid-flight by the
    // previous settle's stale timer.
    window.setTimeout(() => {
      const latest = useTasksStore.getState().tasks.get(id);
      if (isMine(latest) && latest.settledAt === settledAt) useTasksStore.getState().remove(id);
    }, TASK_LINGER_MS);
  };

  return {
    update(patch) {
      const cur = useTasksStore.getState().tasks.get(id);
      if (!isMine(cur) || cur.status !== "running") return;
      useTasksStore.getState().patch(id, {
        progress: patch.progress === undefined ? cur.progress : patch.progress,
        message: patch.message === undefined ? cur.message : patch.message,
      });
    },
    succeed(message) {
      settle("succeeded", { progress: 1, ...(message !== undefined ? { message } : {}) });
    },
    fail(err) {
      settle("failed", { error: err instanceof Error ? err.message : String(err) });
    },
  };
}
