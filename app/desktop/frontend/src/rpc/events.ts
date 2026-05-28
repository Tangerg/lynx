// Bridge from JSON-RPC server-side notifications to AG-UI BaseEvents.
// See docs/API.md §3 + §4: every run event is wrapped in a
// `notifications/run/event` notification whose `params` looks like:
//
//   { runId: string, eventId: string, event: AgUiEvent }
//
// This file gives the typed envelope + a helper to subscribe to a
// single run's events and yield AG-UI BaseEvents to the consumer
// (typically the agent store batcher). Filters notifications by runId
// (the resource id IS the stream id — no separate streamHandle layer).

import type { BaseEvent } from "@ag-ui/core";
import type { RpcClient, NotificationHandler } from "./client";

// ---------------------------------------------------------------------------
// Notification param shapes.
// ---------------------------------------------------------------------------

export interface RunEventParams {
  runId: string;
  eventId: string;
  event: BaseEvent;
}

export interface RunClosedParams {
  runId: string;
  reason?: string;
}

export interface TerminalOutputParams {
  runId: string;
  eventId: string;
  line: { kind: string; text: string };
}

export interface BackgroundUpdateParams {
  taskId: string;
  eventId: string;
  status: string;
  progress?: number;
  outputDelta?: string;
}

// ---------------------------------------------------------------------------
// Streaming helper — filter notifications by runId and yield the
// unwrapped event payload.
// ---------------------------------------------------------------------------

/**
 * Subscribe to AG-UI events for a single run. Returns an AsyncIterable
 * that yields `BaseEvent` until either:
 *   - `notifications/run/closed` arrives for this runId, OR
 *   - the AbortSignal fires, OR
 *   - the consumer calls `.return()` on the iterator
 *
 * The transport's recv() backs every notification through the client;
 * we layer per-run filtering on top.
 */
export function streamRunEvents(
  client: RpcClient,
  runId: string,
  signal?: AbortSignal,
): AsyncIterable<BaseEvent> {
  return {
    [Symbol.asyncIterator]() {
      const buffer: BaseEvent[] = [];
      let waiter: ((value: IteratorResult<BaseEvent>) => void) | null = null;
      let done = false;

      const settleDone = () => {
        if (done) return;
        done = true;
        if (waiter) {
          const w = waiter;
          waiter = null;
          w({ value: undefined, done: true });
        }
      };

      const onEvent: NotificationHandler = (msg) => {
        if (done) return;
        const params = msg.params as RunEventParams | undefined;
        if (!params || params.runId !== runId) return;
        if (waiter) {
          const w = waiter;
          waiter = null;
          w({ value: params.event, done: false });
        } else {
          buffer.push(params.event);
        }
      };

      const onClosed: NotificationHandler = (msg) => {
        const params = msg.params as RunClosedParams | undefined;
        if (params?.runId !== runId) return;
        settleDone();
      };

      const unsubEvent = client.subscribe("notifications/run/event", onEvent);
      const unsubClosed = client.subscribe("notifications/run/closed", onClosed);

      const onAbort = () => settleDone();
      if (signal) {
        if (signal.aborted) settleDone();
        else signal.addEventListener("abort", onAbort, { once: true });
      }

      return {
        async next(): Promise<IteratorResult<BaseEvent>> {
          if (buffer.length > 0) return { value: buffer.shift()!, done: false };
          if (done) return { value: undefined, done: true };
          return new Promise<IteratorResult<BaseEvent>>((resolve) => {
            waiter = resolve;
          });
        },
        async return(): Promise<IteratorResult<BaseEvent>> {
          settleDone();
          unsubEvent();
          unsubClosed();
          if (signal) signal.removeEventListener("abort", onAbort);
          return { value: undefined, done: true };
        },
      };
    },
  };
}
