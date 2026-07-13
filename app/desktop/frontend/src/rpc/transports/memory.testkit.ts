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
import type { RunOutcome, StreamEvent } from "../shapes";
import { RUN_EVENT_METHOD } from "../stream";
import {
  JSONRPC_VERSION,
  isRequest,
  type RpcErrorPayload,
  type RpcId,
  type RpcMessage,
  type RpcRequest,
} from "../types";

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
export async function waitForRequest(t: MemoryTransport, method: string): Promise<RpcRequest> {
  for (let attempt = 0; attempt < 50; attempt++) {
    const found = t.outbox().find((m): m is RpcRequest => isRequest(m) && m.method === method);
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

/** Inject a JSON-RPC error Response matching a prior Request id. */
export function respondError(t: MemoryTransport, id: RpcId, error: RpcErrorPayload): void {
  t.inject({ jsonrpc: JSONRPC_VERSION, id, error } as RpcMessage);
}

/** Inject a server-side Notification with arbitrary method + params. */
export function injectNotification(t: MemoryTransport, method: string, params: unknown): void {
  t.inject({ jsonrpc: JSONRPC_VERSION, method, params });
}

/** Inject a `notifications.run.event` carrying a v2 StreamEvent (§5). A
 *  fixed timestamp keeps fixtures stable; `durable` defaults to true. The
 *  envelope carries BOTH runId and segmentId — the stream tree keys on the
 *  segmentId (a resume opens a new segment of the same run). */
export function injectRunEvent(
  t: MemoryTransport,
  runId: string,
  segmentId: string,
  eventId: string,
  event: StreamEvent,
  durable = true,
): void {
  injectNotification(t, RUN_EVENT_METHOD, {
    runId,
    segmentId,
    eventId,
    timestamp: "2026-06-03T00:00:00Z",
    durable,
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
  outcome: RunOutcome = { type: "completed", result: {} },
): void {
  injectRunEvent(t, runId, segmentId, eventId, { type: "segment.finished", outcome });
}
