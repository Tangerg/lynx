// Server-notification stream → typed AsyncIterable bridge (API.md §5 / §10,
// TRANSPORT.md §7-§9).
//
// v2 collapses run streaming onto ONE notification method:
// `notifications.run.event`, params = RunEvent. There is no separate
// "run closed" method — the terminal signal is a `run.finished`
// StreamEvent for the ROOT run, delivered inside the same stream.
//
// A single root run stream carries the WHOLE run tree (root + every
// descendant subagent run, §5.4). We track tree membership by runId:
// seed with the root, then admit a child run when its `run.started`
// carries a `spawnedByItemId` whose owning item we've already seen on
// this tree. The stream ends when the ROOT run's `run.finished` arrives.
//
// Background tasks stream on `notifications.background.update`, params =
// BackgroundTask, terminal when status leaves "running".

import { z } from "zod";
import { createPushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { BackgroundTask, RunEvent, StreamEvent } from "./shapes";

export const RUN_EVENT_METHOD = "notifications.run.event";
export const BACKGROUND_UPDATE_METHOD = "notifications.background.update";

// ---------------------------------------------------------------------------
// Trust-boundary validation (CLAUDE.md "边界校验用 Zod")
// ---------------------------------------------------------------------------
//
// We validate the RunEvent ENVELOPE shape + the StreamEvent discriminator
// here. The inner `event` payload (Item / ItemDelta / RunOutcome) is the
// Go runtime's codegen-typed output — we cast it to StreamEvent rather
// than re-deriving the full union in Zod (that would duplicate the whole
// §4 catalog at the boundary for no added safety the discriminator check
// doesn't already give). On wrapper-validation failure: warn + drop the
// one notification; a single malformed event must not kill a run.

const RunEventEnvelopeSchema = z.object({
  runId: z.string(),
  eventId: z.string(),
  timestamp: z.string(),
  durable: z.boolean(),
  event: z.looseObject({ type: z.string() }),
});

const BackgroundTaskSchema = z.object({
  id: z.string(),
  kind: z.string(),
  status: z.enum(["running", "completed", "failed", "canceled"]),
  createdAt: z.string(),
});

function makeParser<S extends z.ZodTypeAny>(method: string, schema: S) {
  return (raw: unknown): z.infer<S> | null => {
    const result = schema.safeParse(raw);
    if (!result.success) {
      console.warn(
        `[rpc/stream] dropping malformed ${method} payload:`,
        z.treeifyError(result.error),
      );
      return null;
    }
    return result.data as z.infer<S>;
  };
}

const parseRunEvent = makeParser(RUN_EVENT_METHOD, RunEventEnvelopeSchema);
const parseBackgroundTask = makeParser(BACKGROUND_UPDATE_METHOD, BackgroundTaskSchema);

// ---------------------------------------------------------------------------
// Run-tree membership tracker
// ---------------------------------------------------------------------------
//
// Decides, for a given root run stream, whether an inbound RunEvent
// belongs to this tree, and whether it's the terminal root finish.

class RunTree {
  private readonly runs: Set<string>;
  private readonly itemOwner = new Map<string, string>(); // itemId → owning runId

  constructor(private readonly rootRunId: string) {
    this.runs = new Set([rootRunId]);
  }

  /** Update tree membership from an event; return true if it belongs here. */
  admit(ev: RunEvent): boolean {
    const e = ev.event;
    if (e.type === "run.started") {
      const spawnedBy = e.run.spawnedByItemId;
      if (spawnedBy && this.itemOwner.has(spawnedBy)) this.runs.add(e.run.id);
    } else if (e.type === "item.started" || e.type === "item.completed") {
      if (this.runs.has(ev.runId)) this.itemOwner.set(e.item.id, ev.runId);
    }
    return this.runs.has(ev.runId);
  }

  /** True once the ROOT run has finished — ends the stream. */
  isRootFinish(ev: RunEvent): boolean {
    return ev.runId === this.rootRunId && ev.event.type === "run.finished";
  }
}

// ---------------------------------------------------------------------------
// Run-event streams
// ---------------------------------------------------------------------------

function pump(
  client: RpcClient,
  tree: RunTree,
  channel: ReturnType<typeof createPushPullChannel<RunEvent>>,
  preFiltered: RunEvent[],
): () => void {
  for (const ev of preFiltered) {
    if (tree.admit(ev)) channel.push(ev);
    if (tree.isRootFinish(ev)) channel.close();
  }
  return client.subscribe(RUN_EVENT_METHOD, (msg) => {
    if (channel.closed) return;
    const parsed = parseRunEvent(msg.params);
    if (!parsed) return;
    const ev = { ...parsed, event: parsed.event as StreamEvent } as RunEvent;
    if (tree.admit(ev)) channel.push(ev);
    if (tree.isRootFinish(ev)) channel.close();
  });
}

function makeIterable(
  channel: ReturnType<typeof createPushPullChannel<RunEvent>>,
  cleanup: () => void,
): AsyncIterable<RunEvent> {
  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<RunEvent>> => {
          const result = await inner.next();
          if (result.done) cleanup();
          return result;
        },
        return: async (): Promise<IteratorResult<RunEvent>> => {
          channel.close();
          cleanup();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}

/** Subscribe to a known root run's event stream (runs.subscribe). */
export function streamRunEvents(
  client: RpcClient,
  rootRunId: string,
  signal?: AbortSignal,
): AsyncIterable<RunEvent> {
  const channel = createPushPullChannel<RunEvent>();
  const unsub = pump(client, new RunTree(rootRunId), channel, []);
  const onAbort = () => channel.close();
  if (signal) {
    if (signal.aborted) channel.close();
    else signal.addEventListener("abort", onAbort, { once: true });
  }
  let cleaned = false;
  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    unsub();
    if (signal) signal.removeEventListener("abort", onAbort);
  };
  return makeIterable(channel, cleanup);
}

/**
 * Subscribe to run events BEFORE the runId is known, then bind once
 * `runs.start` / `runs.resume` returns. A fast runtime emits + broadcasts
 * the whole run the instant it handles the POST; subscribing only after
 * the response races and drops the head events. So we subscribe
 * immediately, buffer raw events until `bind(rootRunId)` supplies the
 * runtime-assigned id, then replay the buffer through the tree filter.
 */
export function streamRunEventsDeferred(
  client: RpcClient,
  signal?: AbortSignal,
): { events: AsyncIterable<RunEvent>; bind: (rootRunId: string) => void } {
  const channel = createPushPullChannel<RunEvent>();
  const buffer: RunEvent[] = [];
  let tree: RunTree | null = null;

  const unsub = client.subscribe(RUN_EVENT_METHOD, (msg) => {
    if (channel.closed) return;
    const parsed = parseRunEvent(msg.params);
    if (!parsed) return;
    const ev = { ...parsed, event: parsed.event as StreamEvent } as RunEvent;
    if (tree === null) {
      buffer.push(ev); // not bound yet — keep raw until we learn our root id
      return;
    }
    if (tree.admit(ev)) channel.push(ev);
    if (tree.isRootFinish(ev)) channel.close();
  });

  const onAbort = () => channel.close();
  if (signal) {
    if (signal.aborted) channel.close();
    else signal.addEventListener("abort", onAbort, { once: true });
  }

  let cleaned = false;
  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    unsub();
    if (signal) signal.removeEventListener("abort", onAbort);
  };

  const bind = (rootRunId: string): void => {
    if (tree !== null) return;
    tree = new RunTree(rootRunId);
    for (const ev of buffer) {
      if (tree.admit(ev)) channel.push(ev);
      if (tree.isRootFinish(ev)) channel.close();
    }
    buffer.length = 0;
  };

  return { events: makeIterable(channel, cleanup), bind };
}

/** Subscribe to a background task's updates (background.subscribe). */
export function streamBackgroundUpdates(
  client: RpcClient,
  taskId: string,
  signal?: AbortSignal,
): AsyncIterable<BackgroundTask> {
  const channel = createPushPullChannel<BackgroundTask>();

  const unsub = client.subscribe(BACKGROUND_UPDATE_METHOD, (msg) => {
    if (channel.closed) return;
    const task = parseBackgroundTask(msg.params);
    if (!task || task.id !== taskId) return;
    channel.push(task as BackgroundTask);
    if (task.status !== "running") channel.close();
  });

  const onAbort = () => channel.close();
  if (signal) {
    if (signal.aborted) channel.close();
    else signal.addEventListener("abort", onAbort, { once: true });
  }

  let cleaned = false;
  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    unsub();
    if (signal) signal.removeEventListener("abort", onAbort);
  };

  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<BackgroundTask>> => {
          const result = await inner.next();
          if (result.done) cleanup();
          return result;
        },
        return: async (): Promise<IteratorResult<BackgroundTask>> => {
          channel.close();
          cleanup();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}
