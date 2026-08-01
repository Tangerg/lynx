// Shared test helpers for scenarios driven against MemoryTransport.
//
// Before extraction, three test files (client / methods / smoke) each
// rolled their own "wait for next outbound Request" helper with slightly
// different signatures. This module collects the most expressive
// version (smoke's `waitForRequest`, which filters by method + has a
// timeout) plus a small palette of inject helpers so scenario tests
// stay declarative.
//
// Only imported from `*.test.ts` files — production code never sees
// this module.

import type { MemoryTransport } from "./memory";
import type { TransportRequest } from "../transport";
import type { RunMetrics, SegmentOutcome, StreamEvent } from "../wire.generated";
import type { WireMethodName } from "../wire.methods.generated";
import { RUN_EVENT_METHOD } from "../stream";
import { JSONRPC_VERSION, type RpcId, type RpcMessage } from "../types";

// Outbound (client → server) — synchronisation helpers

/**
 * Wait until a Request with the given method name appears in the
 * transport's outbox, then return it. Polls microtask-by-microtask up
 * to ~50 ticks (more than enough — the client typically queues the
 * request before the next microtask cycle).
 *
 * Use to grab the id the client allocated so you can craft a matching
 * Response via {@link respondSuccess} / {@link respondError}.
 */
export async function waitForRequest<M extends WireMethodName>(
  t: MemoryTransport,
  method: M,
): Promise<TransportRequest & { method: M }> {
  for (let attempt = 0; attempt < 50; attempt++) {
    const found = t
      .outbox()
      .find((message): message is TransportRequest & { method: M } => message.method === method);
    if (found) return found;
    await new Promise((r) => setTimeout(r, 0));
  }
  throw new Error(`timeout waiting for outbound Request "${method}"`);
}

// Inbound (server → client) — message synthesis

/** Inject a JSON-RPC success Response matching a prior Request id. */
export function respondSuccess(t: MemoryTransport, id: RpcId, result: unknown): void {
  t.inject({ jsonrpc: JSONRPC_VERSION, id, result } as RpcMessage);
}

/** Inject a server-side Notification with arbitrary method + params. */
export function injectNotification(t: MemoryTransport, method: string, params: unknown): void {
  t.inject({ jsonrpc: JSONRPC_VERSION, method, params });
}

/** Inject a `notifications.run.event` carrying a v2 StreamEvent (§5). A
 *  fixed timestamp keeps fixtures stable. The
 *  envelope carries BOTH runId and segmentId — the stream tree keys on the
 *  segmentId (a resume opens a new segment of the same run). */
export function injectRunEvent(
  t: MemoryTransport,
  runId: string,
  segmentId: string,
  eventId: string,
  event: StreamEvent,
): void {
  injectNotification(t, RUN_EVENT_METHOD, {
    runId,
    segmentId,
    eventId,
    timestamp: "2026-06-03T00:00:00Z",
    event,
  });
}

/** Inject a `segment.finished` StreamEvent for the root segment — terminates the
 *  stream (v2 has no separate "closed" method, §5). */
export function injectRunFinished(
  t: MemoryTransport,
  runId: string,
  segmentId: string,
  eventId: string,
  outcome: SegmentOutcome = { type: "completed" },
  metrics: RunMetrics = { steps: 0, activeDurationMs: 0 },
): void {
  injectRunEvent(t, runId, segmentId, eventId, { type: "segment.finished", outcome, metrics });
}
