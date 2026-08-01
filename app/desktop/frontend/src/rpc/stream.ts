// Server-notification stream → typed AsyncIterable bridge (API.md §5 / §10,
// TRANSPORT.md §7-§9).
//
// v2 collapses run streaming onto ONE notification method:
// `notifications.run.event`, params = RunEvent. There is no separate
// "run closed" method — the terminal signal is a `segment.finished`
// StreamEvent for the ROOT SEGMENT, delivered inside the same stream.
//
// A single stream is rooted on ONE segment (the segment `runs.start` /
// `runs.resume` / `runs.subscribe` opened, identified by SegmentId — a Run
// keeps a stable RunId across HITL resume, but each resume opens a fresh
// segment). That root segment stream carries the WHOLE run tree: the root
// segment's own events PLUS every descendant subagent run's events (§5.4).
// The root is keyed on segmentId; subagents are admitted by runId (they keep
// distinct RunIds) when a `segment.started` carries a `spawnedByItemId` whose
// owning item we've already seen on this tree. The stream ends when the ROOT
// SEGMENT's `segment.finished` arrives (a subagent's has a different segmentId).

import { createPushPullChannel, type PushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { RunEvent, RuntimeEvent } from "./wire.generated";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";
import { NOTIFICATIONS_RUN_EVENT, NOTIFICATIONS_RUNTIME_EVENT } from "./wire.generated";

export const RUN_EVENT_METHOD = NOTIFICATIONS_RUN_EVENT;
export const RUNTIME_EVENT_METHOD = NOTIFICATIONS_RUNTIME_EVENT;

// ---------------------------------------------------------------------------
// Run-tree membership tracker
// ---------------------------------------------------------------------------
//
// Decides, for a given root segment stream, whether an inbound RunEvent
// belongs to this tree, and whether it's the terminal root-segment finish.

class RunTree {
  // Subagent runIds admitted onto this tree PLUS the root run's own runId
  // (learned from the root-segment segment.started). The root's OWN events are
  // matched by segmentId, not by this set; the set exists so subagents can be
  // admitted by runId and so transport stream termination (which reports runIds)
  // can match.
  private readonly runs = new Set<string>();
  private readonly itemOwner = new Map<string, string>(); // itemId → owning runId
  // Event ids already delivered on this stream. §9.2 requires the client to
  // dedupe on replay/overlap (a residual live stream + a runs.subscribe
  // replay window would otherwise double-append every item.delta). The
  // contract only guarantees eventId is MONOTONIC, not lexicographically
  // comparable, so we track a per-stream seen-set (freed with the stream).
  private readonly seenEventIds = new Set<string>();

  constructor(
    rootRunId: string,
    private readonly rootSegmentId: string,
  ) {
    this.runs.add(rootRunId);
  }

  /** True if this event id was already delivered on this stream (replay /
   *  overlapping-subscription duplicate). Marks it seen otherwise. */
  alreadySeen(eventId: string): boolean {
    if (this.seenEventIds.has(eventId)) return true;
    this.seenEventIds.add(eventId);
    return false;
  }

  /** True if the given run belongs to this stream's tree (root or subagent) —
   *  used by transport stream termination, which is keyed on runId. The root run is known from
   *  the call response; descendant runs are learned from segment.started. */
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
    const event = ev.event;
    if (event.type === "segment.started") {
      if (ev.segmentId === this.rootSegmentId) {
        // Root-segment segment.started — learn the root runId for stream termination.
        this.runs.add(event.run.id);
      } else {
        // A subagent segment.started — admit it iff its spawning item is on the tree.
        const spawnedBy = event.run.spawnedByItemId;
        if (spawnedBy && this.itemOwner.has(spawnedBy)) this.runs.add(event.run.id);
      }
    } else if (event.type === "item.started" || event.type === "item.completed") {
      if (this.belongs(ev)) this.itemOwner.set(event.item.id, ev.runId);
    }
    return this.belongs(ev);
  }

  /** True once the ROOT SEGMENT has finished — ends the stream. A subagent's
   *  segment.finished carries a different segmentId, so it never closes the tree. */
  isRootFinish(ev: RunEvent): boolean {
    return ev.segmentId === this.rootSegmentId && ev.event.type === "segment.finished";
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
          try {
            const result = await inner.next();
            if (result.done) cleanup();
            return result;
          } catch (error) {
            cleanup();
            throw error;
          }
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
  lifetime: StreamLifetime,
): () => void {
  const signal = lifetime.signal;
  let cleaned = false;
  const cleanup = () => {
    if (cleaned) return;
    cleaned = true;
    unsub();
    signal.removeEventListener("abort", onAbort);
    lifetime.abort();
  };
  const onAbort = () => {
    channel.close();
    cleanup();
  };
  if (signal.aborted) onAbort();
  else signal.addEventListener("abort", onAbort, { once: true });
  return cleanup;
}

interface StreamLifetime {
  /** Combined caller + stream-owned signal passed to the transport request. */
  signal: AbortSignal;
  /** End only this stream without mutating the caller-owned signal. */
  abort(): void;
}

function createStreamLifetime(parent?: AbortSignal): StreamLifetime {
  const controller = new AbortController();
  return {
    signal: parent ? AbortSignal.any([parent, controller.signal]) : controller.signal,
    abort: () => controller.abort(),
  };
}

// ---------------------------------------------------------------------------
// Run-event streams
// ---------------------------------------------------------------------------

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

/** A run-event stream plus its teardown. `dispose` exists for the case where
 *  the stream's owning call FAILS before anyone iterates `events` — without
 *  it the subscription (and, for the deferred variant, its grow-forever
 *  pre-bind buffer) leaks, since iterableOf's cleanup only runs on iteration. */
export interface RunEventStream {
  events: AsyncIterable<RunEvent>;
  /** Signal owned by this stream and passed to its opening RPC request. */
  requestSignal: AbortSignal;
  dispose: () => void;
}

/**
 * Subscribe to run events BEFORE the root run / segment ids are known, then bind once
 * `runs.start` / `runs.resume` / `runs.subscribe` returns. Under streamable
 * HTTP the call's response and its event frames arrive on one ordered stream
 * (TRANSPORT.md §6.4), so the head events land right after the response —
 * subscribing only after the response resolves races and drops them. So we
 * subscribe immediately, buffer raw events until `bind(runId, segmentId)` supplies
 * the runtime-assigned root identity, then replay the buffer through the tree
 * filter. (Every stream-opening method returns its root segmentId, so this is
 * the single run-event stream builder — a Run's runId is stable, but the
 * segment being streamed is only known from the response.)
 */
export function streamRunEvents(
  client: RpcClient,
  signal?: AbortSignal,
): RunEventStream & { bind: (rootRunId: string, rootSegmentId: string) => void } {
  const lifetime = createStreamLifetime(signal);
  const channel = createPushPullChannel<RunEvent>();
  const buffer: RunEvent[] = [];
  const streamsEndedBeforeBind = new Map<string, Error | null>();
  let tree: RunTree | null = null;

  const unsubEvents = client.subscribe(RUN_EVENT_METHOD, {
    next(event) {
      if (channel.closed) return;
      if (tree === null) buffer.push(event);
      else feedRunEvent(tree, channel, event);
    },
    error: (error) => {
      channel.fail(error);
      lifetime.abort();
    },
  });
  const unsubDown = client.onStreamEnd((event) => {
    if (channel.closed) return;
    if (tree === null) {
      for (const runId of event.runIds) streamsEndedBeforeBind.set(runId, event.error ?? null);
      return;
    }
    const activeTree = tree;
    if (event.runIds.some((runId) => activeTree.hasRun(runId))) {
      if (event.error) channel.fail(event.error);
      else channel.close();
    }
  });

  const bind = (rootRunId: string, rootSegmentId: string): void => {
    if (tree !== null) return;
    tree = new RunTree(rootRunId, rootSegmentId);
    for (const ev of buffer) feedRunEvent(tree, channel, ev);
    buffer.length = 0;
    if (streamsEndedBeforeBind.has(rootRunId)) {
      const error = streamsEndedBeforeBind.get(rootRunId);
      if (error) channel.fail(error);
      else channel.close();
    }
    streamsEndedBeforeBind.clear();
  };

  const cleanup = bindLifecycle(
    channel,
    () => {
      unsubEvents();
      unsubDown();
    },
    lifetime,
  );
  return {
    events: iterableOf(channel, cleanup),
    requestSignal: lifetime.signal,
    bind,
    dispose: () => {
      channel.close();
      cleanup();
    },
  };
}

// ---------------------------------------------------------------------------
// Runtime event stream
// ---------------------------------------------------------------------------

/** The runtime notification stream plus its teardown (see RunEventStream).
 *  Connection-scoped and lossy: no terminal frame, no replay — the stream
 *  ends when its POST stream does, reported as a typed transport stream-end event.
 *  The consumer resubscribes and treats reconnect as `resync`. */
export interface RuntimeEventStream {
  events: AsyncIterable<RuntimeEvent>;
  /** Signal owned by this stream and passed to its opening RPC request. */
  requestSignal: AbortSignal;
  dispose: () => void;
}

export function streamRuntimeEvents(client: RpcClient, signal?: AbortSignal): RuntimeEventStream {
  const lifetime = createStreamLifetime(signal);
  const channel = createPushPullChannel<RuntimeEvent>();
  const unsubEvents = client.subscribe(RUNTIME_EVENT_METHOD, {
    next(params) {
      if (channel.closed) return;
      channel.push(params.event);
    },
    error: (error) => {
      channel.fail(error);
      lifetime.abort();
    },
  });
  const unsubDown = client.onStreamEnd((event) => {
    if (channel.closed) return;
    if (event.method !== RUNTIME_SUBSCRIBE_METHOD) return;
    if (event.error) channel.fail(event.error);
    else channel.close();
  });
  const cleanup = bindLifecycle(
    channel,
    () => {
      unsubEvents();
      unsubDown();
    },
    lifetime,
  );
  return {
    events: iterableOf(channel, cleanup),
    requestSignal: lifetime.signal,
    dispose: () => {
      channel.close();
      cleanup();
    },
  };
}
