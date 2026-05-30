// Server-side notification stream → typed AsyncIterable<T> bridge.
//
// Every Runtime Protocol streaming method (runs.start /
// workspace.terminal.subscribe / background.subscribe) follows the same
// shape: an immediate Response carrying a resource id (runId / taskId),
// followed by `notifications/<topic>` notifications keyed by that id.
//
// This module gives one `makeFilteredStream<T, P>()` helper + 3 typed
// wrappers (streamRunEvents / streamTerminalOutput / streamBackgroundUpdates)
// that the methods factory consumes. The underlying push→pull async
// channel comes from `channel.ts`. Per-notification-method Zod parsers
// (`parseRunEventParams` / etc.) give callers typed params in their
// `extract` callbacks — no `as unknown as Foo` double-casts.
//
// Stream close detection has two forms:
//   - via-method: a separate `closedMethod` notification means EOS
//                 (runs.start uses `notifications/run/closed`)
//   - via-predicate: inspect each payload for a terminal state
//                 (background updates carry `status: "succeeded" | ...`)

import type { BaseEvent } from "@ag-ui/core";
import { z } from "zod";
import { createPushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { BackgroundUpdate, TermLine } from "./shapes";

// ---------------------------------------------------------------------------
// Notification param schemas + typed parsers
// ---------------------------------------------------------------------------
//
// Per CLAUDE.md "边界校验用 Zod": the JSON-RPC notification payload is
// a trust boundary. We validate WRAPPER shape (`{ runId | taskId,
// eventId, ... }`) here. Inner `event` payload (for run events) stays
// `z.unknown()` — AG-UI CUSTOM event payloads have their own Zod
// schemas in `frontend/src/protocol/agui/schemas.ts` at the handler
// boundary.
//
// On validation failure: log warning, return null. makeFilteredStream
// drops the notification — one malformed event shouldn't kill an
// ongoing run.

const RunEventParamsSchema = z.object({
  runId: z.string(),
  eventId: z.string(),
  ts: z.string(), // §3.1: 服务端权威时间戳，每条必带
  parentToolUseId: z.string().optional(), // 子 agent 归属（缺省=主 agent）
  event: z.unknown(),
});
type RunEventParams = z.infer<typeof RunEventParamsSchema>;

// run/closed 现在携带 RunResult（§3.1 step 4）。流层只用 runId 判终止；
// result 的完整形状（stopReason/usage/cost）由消费侧按 shapes.RunResult 解读。
const RunClosedParamsSchema = z.object({
  runId: z.string(),
  result: z.unknown().optional(),
});
type RunClosedParams = z.infer<typeof RunClosedParamsSchema>;

const TerminalOutputParamsSchema = z.object({
  runId: z.string(),
  eventId: z.string(),
  line: z.object({
    kind: z.string(),
    text: z.string(),
  }),
});
type TerminalOutputParams = z.infer<typeof TerminalOutputParamsSchema>;

const BackgroundUpdateParamsSchema = z.object({
  taskId: z.string(),
  eventId: z.string(),
  status: z.enum(["running", "stopped", "succeeded", "failed"]),
  progress: z.number().optional(),
  outputDelta: z.string().optional(),
});
type BackgroundUpdateParams = z.infer<typeof BackgroundUpdateParamsSchema>;

// Generic factory: take a schema + method name, return a parser that
// validates + warns + returns typed result OR null. Avoids per-method
// duplicated try/catch boilerplate.
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

const parseRunEventParams = makeParser("notifications/run/event", RunEventParamsSchema);
const parseRunClosedParams = makeParser("notifications/run/closed", RunClosedParamsSchema);
const parseTerminalOutputParams = makeParser(
  "notifications/terminal/output",
  TerminalOutputParamsSchema,
);
const parseBackgroundUpdateParams = makeParser(
  "notifications/background/update",
  BackgroundUpdateParamsSchema,
);

// ---------------------------------------------------------------------------
// makeFilteredStream — generic, typed helper
// ---------------------------------------------------------------------------

export interface FilteredStreamSpec<T, P> {
  /** Field in parsed params to match against (e.g. "runId"). */
  idField: keyof P & string;
  /** Value to match. */
  idValue: string;
  /** Notification method that carries stream payloads. */
  notificationMethod: string;
  /**
   * Parse + validate notification.params. Returns typed `P` on success,
   * null on validation failure (caller drops the notification).
   */
  parseParams: (raw: unknown) => P | null;
  /** Project a typed param record to the downstream value type. */
  extract: (params: P) => T;
  /**
   * Close detection — exactly one of:
   *   - `closedMethod`: subscribe to a separate notification method;
   *     its arrival (with matching idField/idValue) closes the stream.
   *   - `isTerminal`: predicate applied to each event params; if true
   *     after pushing the value, close the stream.
   * `closedMethod` ships with its own parser (the close payload may
   * have a different shape).
   */
  closedMethod?: {
    method: string;
    parseParams: (raw: unknown) => Record<string, unknown> | null;
  };
  isTerminal?: (params: P) => boolean;
  /** Client-side AbortSignal — fires close on abort. */
  signal?: AbortSignal;
}

export function makeFilteredStream<T, P>(
  client: RpcClient,
  spec: FilteredStreamSpec<T, P>,
): AsyncIterable<T> {
  const channel = createPushPullChannel<T>();

  const unsubEvent = client.subscribe(spec.notificationMethod, (msg) => {
    if (channel.closed) return;
    const params = spec.parseParams(msg.params);
    if (!params) return;
    if ((params as Record<string, unknown>)[spec.idField] !== spec.idValue) return;
    channel.push(spec.extract(params));
    if (spec.isTerminal?.(params)) channel.close();
  });

  const unsubClosed = spec.closedMethod
    ? client.subscribe(spec.closedMethod.method, (msg) => {
        const params = spec.closedMethod!.parseParams(msg.params);
        if (!params) return;
        if (params[spec.idField] !== spec.idValue) return;
        channel.close();
      })
    : () => undefined;

  const onAbort = () => channel.close();
  if (spec.signal) {
    if (spec.signal.aborted) channel.close();
    else spec.signal.addEventListener("abort", onAbort, { once: true });
  }

  // Drop the client subscriptions + abort listener. Idempotent so it's
  // safe to call from both exit paths below.
  let cleanedUp = false;
  const cleanup = (): void => {
    if (cleanedUp) return;
    cleanedUp = true;
    unsubEvent();
    unsubClosed();
    if (spec.signal) spec.signal.removeEventListener("abort", onAbort);
  };

  // Wrap the channel iterator so cleanup runs on BOTH consumer exit paths:
  //   - early break / throw   → `for await` calls return()
  //   - natural completion     → channel closes (isTerminal / closedMethod /
  //                              abort), next() resolves done=true, and the
  //                              loop ends WITHOUT calling return().
  // Only handling return() leaked the run-event / run-closed subscribers in
  // the client's map for every run that finished normally (the common case).
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

// ---------------------------------------------------------------------------
// Typed wrappers — one per streaming method in the protocol
// ---------------------------------------------------------------------------

// Cast a typed parser to the generic "Record<string, unknown> | null"
// shape that `closedMethod.parseParams` expects. Safe because every
// Zod-parsed object IS a Record<string, unknown>.
const asGenericParser =
  <P extends object>(p: (raw: unknown) => P | null) =>
  (raw: unknown) =>
    p(raw) as Record<string, unknown> | null;

/**
 * Subscribe to run events BEFORE the runId is known, then bind to it once
 * `runs.start` returns. A fast runtime emits — and broadcasts — the WHOLE run
 * the instant it handles the POST; subscribing only after the response races
 * and drops everything. So we subscribe immediately, buffer events until
 * `bind(runId)` supplies the runtime-assigned id (a client-supplied runId may
 * be ignored by the runtime — §6.3), then flush the buffer + filter ongoing.
 *
 * Returns the event iterable plus `bind`; the caller invokes `bind(result.runId)`
 * with the runs.start response's runId.
 */
export function streamRunEventsDeferred(
  client: RpcClient,
  signal?: AbortSignal,
): { events: AsyncIterable<BaseEvent>; bind: (runId: string) => void } {
  const channel = createPushPullChannel<BaseEvent>();
  let runId: string | null = null;
  const pendingEvents: RunEventParams[] = [];
  const closedRunIds = new Set<string>();

  const unsubEvent = client.subscribe("notifications/run/event", (msg) => {
    if (channel.closed) return;
    const p = parseRunEventParams(msg.params);
    if (!p) return;
    if (runId === null)
      pendingEvents.push(p); // not bound yet — buffer until we learn our runId
    else if (p.runId === runId) channel.push(p.event as BaseEvent);
  });

  const unsubClosed = client.subscribe("notifications/run/closed", (msg) => {
    const p = parseRunClosedParams(msg.params);
    if (!p) return;
    if (runId === null) closedRunIds.add(p.runId);
    else if (p.runId === runId) channel.close();
  });

  const onAbort = () => channel.close();
  if (signal) {
    if (signal.aborted) channel.close();
    else signal.addEventListener("abort", onAbort, { once: true });
  }

  let cleanedUp = false;
  const cleanup = (): void => {
    if (cleanedUp) return;
    cleanedUp = true;
    unsubEvent();
    unsubClosed();
    if (signal) signal.removeEventListener("abort", onAbort);
  };

  const events: AsyncIterable<BaseEvent> = {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<BaseEvent>> => {
          const result = await inner.next();
          if (result.done) cleanup();
          return result;
        },
        return: async (): Promise<IteratorResult<BaseEvent>> => {
          channel.close();
          cleanup();
          return { value: undefined as never, done: true };
        },
      };
    },
  };

  const bind = (id: string): void => {
    if (runId !== null) return;
    runId = id;
    for (const p of pendingEvents) if (p.runId === id) channel.push(p.event as BaseEvent);
    pendingEvents.length = 0;
    if (closedRunIds.has(id)) channel.close();
  };

  return { events, bind };
}

/** Subscribe to AG-UI events from a single `runs.start` invocation. */
export function streamRunEvents(
  client: RpcClient,
  runId: string,
  signal?: AbortSignal,
): AsyncIterable<BaseEvent> {
  return makeFilteredStream<BaseEvent, RunEventParams>(client, {
    idField: "runId",
    idValue: runId,
    notificationMethod: "notifications/run/event",
    parseParams: parseRunEventParams,
    extract: (p) => p.event as BaseEvent,
    closedMethod: {
      method: "notifications/run/closed",
      parseParams: asGenericParser<RunClosedParams>(parseRunClosedParams),
    },
    signal,
  });
}

/** Subscribe to pty output for a tool's terminal session. */
export function streamTerminalOutput(
  client: RpcClient,
  runId: string,
  signal?: AbortSignal,
): AsyncIterable<TermLine> {
  return makeFilteredStream<TermLine, TerminalOutputParams>(client, {
    idField: "runId",
    idValue: runId,
    notificationMethod: "notifications/terminal/output",
    parseParams: parseTerminalOutputParams,
    extract: (p) => p.line as TermLine,
    // Terminal streams close when the parent run closes.
    closedMethod: {
      method: "notifications/run/closed",
      parseParams: asGenericParser<RunClosedParams>(parseRunClosedParams),
    },
    signal,
  });
}

/** Subscribe to a long-running background task's updates. */
export function streamBackgroundUpdates(
  client: RpcClient,
  taskId: string,
  signal?: AbortSignal,
): AsyncIterable<BackgroundUpdate> {
  return makeFilteredStream<BackgroundUpdate, BackgroundUpdateParams>(client, {
    idField: "taskId",
    idValue: taskId,
    notificationMethod: "notifications/background/update",
    parseParams: parseBackgroundUpdateParams,
    // No separate close notification — terminal state is in `status`.
    isTerminal: (p) => p.status === "succeeded" || p.status === "failed" || p.status === "stopped",
    extract: (p) => p as unknown as BackgroundUpdate,
    signal,
  });
}
