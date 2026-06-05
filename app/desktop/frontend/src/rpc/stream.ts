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
import { createPushPullChannel, type PushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { BackgroundTask, RunEvent, StreamEvent } from "./shapes";

export const RUN_EVENT_METHOD = "notifications.run.event";
export const BACKGROUND_UPDATE_METHOD = "notifications.background.update";

// ---------------------------------------------------------------------------
// Trust-boundary validation (CLAUDE.md: "validate at trust boundaries with Zod")
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
    // The Zod envelope only guarantees `event.type` is a string — the inner
    // payload is cast, not validated (see top-of-file note). So treat
    // run/item as possibly-absent here: a malformed event must update nothing
    // and be dropped, never throw. This runs inside the `client.subscribe`
    // callback, which has no try/catch — an unguarded deref would kill the
    // whole run stream, not just drop one event.
    const e = ev.event as {
      type: string;
      run?: { id: string; spawnedByItemId?: string };
      item?: { id: string };
    };
    if (e.type === "run.started" && e.run) {
      const spawnedBy = e.run.spawnedByItemId;
      if (spawnedBy && this.itemOwner.has(spawnedBy)) this.runs.add(e.run.id);
    } else if ((e.type === "item.started" || e.type === "item.completed") && e.item) {
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
// Channel → AsyncIterable plumbing (shared by every stream below)
// ---------------------------------------------------------------------------

/** Wrap a push-pull channel as a self-cleaning AsyncIterable: `cleanup` runs
 *  once when the iterator drains (done) or the consumer breaks early. */
function iterableOf<T>(channel: PushPullChannel<T>, cleanup: () => void): AsyncIterable<T> {
  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<T>> => {
          const result = await inner.next();
          if (result.done) cleanup();
          return result;
        },
        return: async (): Promise<IteratorResult<T>> => {
          channel.close();
          cleanup();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}

/** Tie a channel's lifetime to a subscription + an optional AbortSignal.
 *  Returns an idempotent cleanup that unsubscribes + detaches the listener. */
function bindLifecycle<T>(
  channel: PushPullChannel<T>,
  unsub: () => void,
  signal?: AbortSignal,
): () => void {
  const onAbort = () => channel.close();
  if (signal) {
    if (signal.aborted) channel.close();
    else signal.addEventListener("abort", onAbort, { once: true });
  }
  let cleaned = false;
  return () => {
    if (cleaned) return;
    cleaned = true;
    unsub();
    if (signal) signal.removeEventListener("abort", onAbort);
  };
}

// ---------------------------------------------------------------------------
// Run-event streams
// ---------------------------------------------------------------------------

/** Parse + cast a raw notification payload into a RunEvent (null if malformed). */
function toRunEvent(params: unknown): RunEvent | null {
  const parsed = parseRunEvent(params);
  if (!parsed) return null;
  return { ...parsed, event: parsed.event as StreamEvent } as RunEvent;
}

/** Push an event into the stream if it belongs to the tree; close on root finish. */
function feedRunEvent(tree: RunTree, channel: PushPullChannel<RunEvent>, ev: RunEvent): void {
  if (tree.admit(ev)) channel.push(ev);
  if (tree.isRootFinish(ev)) channel.close();
}

/** Subscribe to a known root run's event stream (runs.subscribe). */
export function streamRunEvents(
  client: RpcClient,
  rootRunId: string,
  signal?: AbortSignal,
): AsyncIterable<RunEvent> {
  const channel = createPushPullChannel<RunEvent>();
  const tree = new RunTree(rootRunId);
  const unsub = client.subscribe(RUN_EVENT_METHOD, (msg) => {
    if (channel.closed) return;
    const ev = toRunEvent(msg.params);
    if (ev) feedRunEvent(tree, channel, ev);
  });
  return iterableOf(channel, bindLifecycle(channel, unsub, signal));
}

/**
 * Subscribe to run events BEFORE the runId is known, then bind once
 * `runs.start` / `runs.resume` returns. Under streamable HTTP the call's
 * response and its event frames arrive on one ordered stream (TRANSPORT.md
 * §6.4), so the head events land right after the response — subscribing only
 * after the response resolves races and drops them. So we subscribe
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
    const ev = toRunEvent(msg.params);
    if (!ev) return;
    // Not bound yet — keep raw until bind() supplies our root run id.
    if (tree === null) buffer.push(ev);
    else feedRunEvent(tree, channel, ev);
  });

  const bind = (rootRunId: string): void => {
    if (tree !== null) return;
    tree = new RunTree(rootRunId);
    for (const ev of buffer) feedRunEvent(tree, channel, ev);
    buffer.length = 0;
  };

  return { events: iterableOf(channel, bindLifecycle(channel, unsub, signal)), bind };
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
  return iterableOf(channel, bindLifecycle(channel, unsub, signal));
}
