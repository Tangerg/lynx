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
import type { RunResult } from "../shapes";
import {
  JSONRPC_VERSION,
  isRequest,
  type RpcErrorPayload,
  type RpcId,
  type RpcMessage,
  type RpcRequest,
} from "../types";

// ---------------------------------------------------------------------------
// Outbound (client → server) — synchronisation helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Inbound (server → client) — message synthesis
// ---------------------------------------------------------------------------

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

/** Inject a `notifications/run/event` carrying an AG-UI event payload. */
export function injectRunEvent(
  t: MemoryTransport,
  runId: string,
  eventId: string,
  event: Record<string, unknown>,
): void {
  // `ts` is required by RunEventParamsSchema (§3.1 — every event carries a
  // server-authoritative timestamp). A fixed stamp keeps fixtures stable.
  injectNotification(t, "notifications/run/event", {
    runId,
    eventId,
    ts: "2025-01-01T00:00:00Z",
    event,
  });
}

/** Inject `notifications/run/closed` — terminates a run's event stream.
 *  Per §3.1 it carries a RunResult; the stream only needs `runId` to close,
 *  so `result` is optional for fixtures that don't assert on it. */
export function injectRunClosed(t: MemoryTransport, runId: string, result?: RunResult): void {
  injectNotification(t, "notifications/run/closed", result ? { runId, result } : { runId });
}
