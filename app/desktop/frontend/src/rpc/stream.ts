// Server-side notification stream → typed AsyncIterable<T> bridge.
//
// Every Runtime Protocol streaming method (runs.start /
// workspace.terminal.subscribe / background.subscribe) follows the same
// shape: an immediate Response carrying a resource id (runId / taskId),
// followed by `notifications/<topic>` notifications keyed by that id.
//
// This module gives one `makeFilteredStream<T>()` helper + 3 typed
// wrappers (streamRunEvents / streamTerminalOutput / streamBackgroundUpdates)
// that the methods factory consumes. The underlying push→pull async
// channel comes from `channel.ts`.
//
// Stream close detection has two forms:
//   - via-method: a separate `closedMethod` notification means EOS
//                 (runs.start uses `notifications/run/closed`)
//   - via-predicate: inspect each payload for a terminal state
//                 (background updates carry `status: "succeeded" | ...`)
//
// The two-handler-on-same-method approach in the previous version had
// a bug — the close handler would close the stream on the FIRST matching
// update, because both handlers ran for every matching notification.
// Now we split via predicate vs separate method explicitly.

import type { BaseEvent } from "@ag-ui/core";
import { createPushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type {
  BackgroundUpdate,
  BackgroundUpdateParams,
  RunEventParams,
  TerminalOutputParams,
  TermLine,
} from "./shapes";

// ---------------------------------------------------------------------------
// makeFilteredStream — generic helper
// ---------------------------------------------------------------------------

export interface FilteredStreamSpec<T> {
  /** Field in notification.params to match against (e.g. "runId"). */
  idField: string;
  /** Value to match. */
  idValue: string;
  /** Notification method that carries stream payloads. */
  notificationMethod: string;
  /** Extract the typed value from a matched notification's params. */
  extract: (params: Record<string, unknown>) => T;
  /**
   * Close detection. Exactly one form must be provided:
   *  - `closedMethod`: subscribe to a separate notification method;
   *    its arrival (matching idField/idValue) closes the stream.
   *  - `isTerminal`: predicate applied to each payload; if true after
   *    pushing the value, close the stream.
   */
  closedMethod?: string;
  isTerminal?: (params: Record<string, unknown>) => boolean;
  /** Client-side AbortSignal — fires close on abort. */
  signal?: AbortSignal;
}

export function makeFilteredStream<T>(
  client: RpcClient,
  spec: FilteredStreamSpec<T>,
): AsyncIterable<T> {
  const channel = createPushPullChannel<T>();

  const unsubEvent = client.subscribe(spec.notificationMethod, (msg) => {
    if (channel.closed) return;
    const params = msg.params as Record<string, unknown> | undefined;
    if (!params || params[spec.idField] !== spec.idValue) return;
    channel.push(spec.extract(params));
    if (spec.isTerminal?.(params)) channel.close();
  });

  const unsubClosed = spec.closedMethod
    ? client.subscribe(spec.closedMethod, (msg) => {
        const params = msg.params as Record<string, unknown> | undefined;
        if (params?.[spec.idField] !== spec.idValue) return;
        channel.close();
      })
    : () => undefined;

  const onAbort = () => channel.close();
  if (spec.signal) {
    if (spec.signal.aborted) channel.close();
    else spec.signal.addEventListener("abort", onAbort, { once: true });
  }

  // Wrap the channel iterator so cleanup (unsubscribe, abort listener)
  // happens when the consumer exits via for-await break or .return().
  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: () => inner.next(),
        return: async (): Promise<IteratorResult<T>> => {
          channel.close();
          unsubEvent();
          unsubClosed();
          if (spec.signal) spec.signal.removeEventListener("abort", onAbort);
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}

// ---------------------------------------------------------------------------
// Typed wrappers — one per streaming method in the protocol
// ---------------------------------------------------------------------------

/** Subscribe to AG-UI events from a single `runs.start` invocation. */
export function streamRunEvents(
  client: RpcClient,
  runId: string,
  signal?: AbortSignal,
): AsyncIterable<BaseEvent> {
  return makeFilteredStream<BaseEvent>(client, {
    idField: "runId",
    idValue: runId,
    notificationMethod: "notifications/run/event",
    closedMethod: "notifications/run/closed",
    extract: (params) => (params as unknown as RunEventParams).event,
    signal,
  });
}

/** Subscribe to pty output for a tool's terminal session. */
export function streamTerminalOutput(
  client: RpcClient,
  runId: string,
  signal?: AbortSignal,
): AsyncIterable<TermLine> {
  return makeFilteredStream<TermLine>(client, {
    idField: "runId",
    idValue: runId,
    notificationMethod: "notifications/terminal/output",
    // Terminal streams close when the parent run closes.
    closedMethod: "notifications/run/closed",
    extract: (params) => (params as unknown as TerminalOutputParams).line,
    signal,
  });
}

/** Subscribe to a long-running background task's updates. */
export function streamBackgroundUpdates(
  client: RpcClient,
  taskId: string,
  signal?: AbortSignal,
): AsyncIterable<BackgroundUpdate> {
  return makeFilteredStream<BackgroundUpdate>(client, {
    idField: "taskId",
    idValue: taskId,
    notificationMethod: "notifications/background/update",
    // No separate close notification — terminal state is embedded
    // in the update's `status` field.
    isTerminal: (params) => {
      const status = (params as unknown as BackgroundUpdateParams).status;
      return status === "succeeded" || status === "failed" || status === "stopped";
    },
    extract: (params) => params as unknown as BackgroundUpdate,
    signal,
  });
}
