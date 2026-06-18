// Per-session message queue: messages typed while a run is streaming are
// queued (one run per session, §6.11) and auto-sent when the run settles
// cleanly — so a message composed mid-run is never lost (T2.1 of the UX polish
// backlog). In-memory + ephemeral (run-flow state, like the agent view).

import { nanoid } from "nanoid";
import { create } from "zustand";
import type { ContentBlock } from "@/rpc";
import { disposeOnHmr } from "@/lib/hmr";
import { useAgentStore } from "./agentStore";

export interface QueuedMessage {
  id: string;
  input: ContentBlock[];
  /** Plain-text preview for the chip (first text block; "" for image-only). */
  text: string;
}

interface QueueState {
  queued: Record<string, QueuedMessage[]>;
  /** Append a message to a session's queue (FIFO). */
  enqueue: (sid: string, input: ContentBlock[]) => void;
  /** Drop the head (after it's been sent). */
  dequeue: (sid: string) => void;
  /** Remove one queued message by id (the chip's ✕). */
  remove: (sid: string, id: string) => void;
}

function previewText(input: ContentBlock[]): string {
  const block = input.find((b) => b.type === "text");
  return block && "text" in block ? block.text : "";
}

export const useQueueStore = create<QueueState>((set) => ({
  queued: {},
  enqueue: (sid, input) =>
    set((s) => ({
      queued: {
        ...s.queued,
        [sid]: [...(s.queued[sid] ?? []), { id: nanoid(), input, text: previewText(input) }],
      },
    })),
  dequeue: (sid) =>
    set((s) => {
      const list = s.queued[sid];
      if (!list || list.length === 0) return s;
      return { queued: { ...s.queued, [sid]: list.slice(1) } };
    }),
  remove: (sid, id) =>
    set((s) => {
      const list = s.queued[sid];
      if (!list) return s;
      return { queued: { ...s.queued, [sid]: list.filter((m) => m.id !== id) } };
    }),
}));

// Auto-drain: when a run settles cleanly (running true→false with no open
// interrupt and no error), send the next queued message — starting a fresh run.
// The queue is HELD while the agent is waiting on the user (open interrupt) or
// after an error, so a queued message never jumps an unanswered approval or
// auto-sends into a failed turn. Module-level subscription (app-lifetime),
// disposeOnHmr-guarded against dev hot-reload stacking.
const lastRunning = new Map<string, boolean>();
const unsubDrain = useAgentStore.subscribe((state) => {
  const { sessions } = state;
  for (const id in sessions) {
    const entry = sessions[id]!;
    const running = entry.view.run.running;
    const was = lastRunning.get(id) ?? false;
    if (was === running) continue;
    lastRunning.set(id, running);
    if (!was || running) continue; // act only on the true→false (settle) edge
    if (entry.view.openInterrupts.length > 0 || entry.view.error) continue; // hold
    const next = useQueueStore.getState().queued[id]?.[0];
    if (!next || !entry.send) continue;
    const send = entry.send;
    useQueueStore.getState().dequeue(id);
    // Defer the send out of the subscriber: entry.send starts a new run (more
    // store mutations), which must not re-enter the set() that triggered us.
    queueMicrotask(() => send(next.input));
  }
  // Forget dropped sessions (tab closed). `queued` is usually empty, so the
  // for-in is zero-cost in the common case.
  for (const id of [...lastRunning.keys()]) if (!(id in sessions)) lastRunning.delete(id);
  const { queued, remove } = useQueueStore.getState();
  for (const id in queued) if (!(id in sessions)) for (const m of queued[id]!) remove(id, m.id);
});
disposeOnHmr(unsubDrain);
