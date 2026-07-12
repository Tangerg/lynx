// Server-notification stream → typed AsyncIterable bridge (API.md §5 / §10,
// TRANSPORT.md §7-§9).
//
// v2 collapses run streaming onto ONE notification method:
// `notifications.run.event`, params = RunEvent. There is no separate
// "run closed" method — the terminal signal is a `run.finished`
// StreamEvent for the ROOT SEGMENT, delivered inside the same stream.
//
// A single stream is rooted on ONE segment (the segment `runs.start` /
// `runs.resume` / `runs.subscribe` opened, identified by SegmentId — a Run
// keeps a stable RunId across HITL resume, but each resume opens a fresh
// segment). That root segment stream carries the WHOLE run tree: the root
// segment's own events PLUS every descendant subagent run's events (§5.4).
// The root is keyed on segmentId; subagents are admitted by runId (they keep
// distinct RunIds) when a `run.started` carries a `spawnedByItemId` whose
// owning item we've already seen on this tree. The stream ends when the ROOT
// SEGMENT's `run.finished` arrives (a subagent's has a different segmentId).

import { z } from "zod";
import { createPushPullChannel, type PushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { RunEvent, StreamEvent, WorkspaceEvent } from "./shapes";
import { STREAM_DOWN_METHOD, WORKSPACE_SUBSCRIBE_METHOD, type StreamDownParams } from "./transport";

export const RUN_EVENT_METHOD = "notifications.run.event";
export const WORKSPACE_EVENT_METHOD = "notifications.workspace.event";

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
  segmentId: z.string(),
  eventId: z.string(),
  timestamp: z.string(),
  // No `durable` on the envelope — durability derives from event.type
  // (isDurableEvent), only `custom` carries its own (API.md §5.2 / TRANSPORT §6.4).
  event: z.looseObject({ type: z.string() }),
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

// Same envelope-only discipline as RunEvent: validate the wrapper + type
// discriminator, cast the typed payload (AUX_API §3.2).
const WorkspaceEventEnvelopeSchema = z.object({
  event: z.looseObject({ type: z.string() }),
});
const parseWorkspaceEvent = makeParser(WORKSPACE_EVENT_METHOD, WorkspaceEventEnvelopeSchema);

// ---------------------------------------------------------------------------
// Run-tree membership tracker
// ---------------------------------------------------------------------------
//
// Decides, for a given root segment stream, whether an inbound RunEvent
// belongs to this tree, and whether it's the terminal root-segment finish.

class RunTree {
  // Subagent runIds admitted onto this tree PLUS the root run's own runId
  // (learned from the root-segment run.started). The root's OWN events are
  // matched by segmentId, not by this set; the set exists so subagents can be
  // admitted by runId and so STREAM_DOWN (which reports runIds) can match.
  private readonly runs = new Set<string>();
  private readonly itemOwner = new Map<string, string>(); // itemId → owning runId
  // Event ids already delivered on this stream. §9.2 requires the client to
  // dedupe on replay/overlap (a residual live stream + a runs.subscribe
  // replay window would otherwise double-append every item.delta). The
  // contract only guarantees eventId is MONOTONIC, not lexicographically
  // comparable, so we track a per-stream seen-set (freed with the stream).
  private readonly seenEventIds = new Set<string>();

  constructor(private readonly rootSegmentId: string) {}

  /** True if this event id was already delivered on this stream (replay /
   *  overlapping-subscription duplicate). Marks it seen otherwise. */
  alreadySeen(eventId: string): boolean {
    if (this.seenEventIds.has(eventId)) return true;
    this.seenEventIds.add(eventId);
    return false;
  }

  /** True if the given run belongs to this stream's tree (root or subagent) —
   *  used by STREAM_DOWN, which is keyed on runId. The root runId is populated
   *  once its root-segment run.started is seen. */
  hasRun(runId: string): boolean {
    return this.runs.has(runId);
  }

  /** An event belongs to this tree if it's on the root segment or on an
   *  admitted subagent run. */
  private belongs(ev: RunEvent): boolean {
    return ev.segmentId === this.rootSegmentId || this.runs.has(ev.runId);
  }

  /** Update tree membership from an event; return true if it belongs here. */
  admit(ev: RunEvent): boolean {
    // The Zod envelope only guarantees `segmentId`/`runId` are strings and
    // `event.type` a string — the inner payload is cast, not validated (see
    // top-of-file note). So treat run/item as possibly-absent here: a malformed
    // event must update nothing and be dropped, never throw. This runs inside
    // the `client.subscribe` callback, which has no try/catch — an unguarded
    // deref would kill the whole run stream, not just drop one event.
    const e = ev.event as {
      type: string;
      run?: { id: string; spawnedByItemId?: string };
      item?: { id: string };
    };
    if (e.type === "run.started" && e.run) {
      if (ev.segmentId === this.rootSegmentId) {
        // Root-segment run.started — learn the root runId (for STREAM_DOWN).
        this.runs.add(e.run.id);
      } else {
        // A subagent run.started — admit it iff its spawning item is on the tree.
        const spawnedBy = e.run.spawnedByItemId;
        if (spawnedBy && this.itemOwner.has(spawnedBy)) this.runs.add(e.run.id);
      }
    } else if ((e.type === "item.started" || e.type === "item.completed") && e.item) {
      if (this.belongs(ev)) this.itemOwner.set(e.item.id, ev.runId);
    }
    return this.belongs(ev);
  }

  /** True once the ROOT SEGMENT has finished — ends the stream. A subagent's
   *  run.finished carries a different segmentId, so it never closes the tree. */
  isRootFinish(ev: RunEvent): boolean {
    return ev.segmentId === this.rootSegmentId && ev.event.type === "run.finished";
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
  // Membership FIRST, dedupe second: eventId is only monotonic/unique within
  // THIS root run stream — a foreign run's event may carry an equal id and
  // must not poison the seen-set (admit's bookkeeping is idempotent, so a
  // re-delivered duplicate passing through it is harmless).
  if (!tree.admit(ev)) return;
  if (tree.alreadySeen(ev.eventId)) return;
  channel.push(ev);
  if (tree.isRootFinish(ev)) channel.close();
}

/** Subscribe to the transport's stream-down synthetic: when the HTTP stream
 *  carrying this tree's events dies abnormally, close the channel so the
 *  consumer's for-await ends instead of hanging forever (see transport.ts). */
function subscribeStreamDown(
  client: RpcClient,
  channel: PushPullChannel<RunEvent>,
  treeOf: () => RunTree | null,
): () => void {
  return client.subscribe(STREAM_DOWN_METHOD, (msg) => {
    if (channel.closed) return;
    const tree = treeOf();
    const runIds = (msg.params as StreamDownParams | undefined)?.runIds ?? [];
    if (tree && runIds.some((id) => tree.hasRun(id))) channel.close();
  });
}

/** A run-event stream plus its teardown. `dispose` exists for the case where
 *  the stream's owning call FAILS before anyone iterates `events` — without
 *  it the subscription (and, for the deferred variant, its grow-forever
 *  pre-bind buffer) leaks, since iterableOf's cleanup only runs on iteration. */
export interface RunEventStream {
  events: AsyncIterable<RunEvent>;
  dispose: () => void;
}

/**
 * Subscribe to run events BEFORE the root segment id is known, then bind once
 * `runs.start` / `runs.resume` / `runs.subscribe` returns. Under streamable
 * HTTP the call's response and its event frames arrive on one ordered stream
 * (TRANSPORT.md §6.4), so the head events land right after the response —
 * subscribing only after the response resolves races and drops them. So we
 * subscribe immediately, buffer raw events until `bind(rootSegmentId)` supplies
 * the runtime-assigned root segment id, then replay the buffer through the tree
 * filter. (Every stream-opening method returns its root segmentId, so this is
 * the single run-event stream builder — a Run's runId is stable, but the
 * segment being streamed is only known from the response.)
 */
export function streamRunEvents(
  client: RpcClient,
  signal?: AbortSignal,
): RunEventStream & { bind: (rootSegmentId: string) => void } {
  const channel = createPushPullChannel<RunEvent>();
  const buffer: RunEvent[] = [];
  let tree: RunTree | null = null;

  const unsubEvents = client.subscribe(RUN_EVENT_METHOD, (msg) => {
    if (channel.closed) return;
    const ev = toRunEvent(msg.params);
    if (!ev) return;
    // Not bound yet — keep raw until bind() supplies our root segment id.
    if (tree === null) buffer.push(ev);
    else feedRunEvent(tree, channel, ev);
  });
  // Pre-bind, tree is null and the handler no-ops — correct: if the stream
  // died before our call's response arrived, the call itself rejects (the
  // transport synthesizes an error Response) and methods.ts disposes us.
  const unsubDown = subscribeStreamDown(client, channel, () => tree);

  const bind = (rootSegmentId: string): void => {
    if (tree !== null) return;
    tree = new RunTree(rootSegmentId);
    for (const ev of buffer) feedRunEvent(tree, channel, ev);
    buffer.length = 0;
  };

  const cleanup = bindLifecycle(
    channel,
    () => {
      unsubEvents();
      unsubDown();
    },
    signal,
  );
  return {
    events: iterableOf(channel, cleanup),
    bind,
    dispose: () => {
      channel.close();
      cleanup();
    },
  };
}

// ---------------------------------------------------------------------------
// Workspace event stream (workspace.subscribe, AUX_API §3)
// ---------------------------------------------------------------------------

/** The workspace notification stream plus its teardown (see RunEventStream).
 *  Connection-scoped and lossy: no terminal frame, no replay — the stream
 *  ends when its POST stream does, signalled via a method-attributed
 *  STREAM_DOWN. The consumer resubscribes and treats reconnect as `resync`. */
export interface WorkspaceEventStream {
  events: AsyncIterable<WorkspaceEvent>;
  dispose: () => void;
}

export function streamWorkspaceEvents(
  client: RpcClient,
  signal?: AbortSignal,
): WorkspaceEventStream {
  const channel = createPushPullChannel<WorkspaceEvent>();
  const unsubEvents = client.subscribe(WORKSPACE_EVENT_METHOD, (msg) => {
    if (channel.closed) return;
    const parsed = parseWorkspaceEvent(msg.params);
    if (parsed) channel.push(parsed.event as WorkspaceEvent);
  });
  const unsubDown = client.subscribe(STREAM_DOWN_METHOD, (msg) => {
    if (channel.closed) return;
    if ((msg.params as StreamDownParams | undefined)?.method === WORKSPACE_SUBSCRIBE_METHOD)
      channel.close();
  });
  const cleanup = bindLifecycle(
    channel,
    () => {
      unsubEvents();
      unsubDown();
    },
    signal,
  );
  return {
    events: iterableOf(channel, cleanup),
    dispose: () => {
      channel.close();
      cleanup();
    },
  };
}
