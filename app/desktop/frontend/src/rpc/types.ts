// JSON-RPC 2.0 envelope for the Lyra Runtime Protocol.
//
// See docs/protocol/API.md §1 for the full spec. Three message kinds share one
// shape (discriminated by which optional fields are populated):
//
//   Request:      { jsonrpc, id, method, params? }
//   Response:     { jsonrpc, id, result? | error? }    (mutually exclusive)
//   Notification: { jsonrpc, method, params? }         (no id)
//
// Lyra currently uses notifications only for runtime→client event delivery
// (`notifications.run.event` and `notifications.workspace.event`). Mutations
// such as `sessions.update` are ordinary requests with correlated responses.

import { z } from "zod";

export const JSONRPC_VERSION = "2.0" as const;

// JSON-RPC 2.0 spec allows string | number for id; we lock to string
// (docs/protocol/API.md §1.1). Type uniformity across the wire — every id in the
// protocol (sessionId / runId / requestId / envelope id) is a string, so
// dispatch + correlation never branch on id type. The client allocates
// monotonic integers but stringifies them before they hit the wire.
export type RpcId = string;

export interface RpcRequest<P = unknown> {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  method: string;
  params?: P;
}

export interface RpcResponseSuccess<R = unknown> {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  result: R;
}

export interface RpcResponseError {
  jsonrpc: typeof JSONRPC_VERSION;
  id: RpcId;
  error: RpcErrorPayload;
}

export type RpcResponse<R = unknown> = RpcResponseSuccess<R> | RpcResponseError;

export interface RpcNotification<P = unknown> {
  jsonrpc: typeof JSONRPC_VERSION;
  method: string;
  params?: P;
}

export type RpcMessage = RpcRequest | RpcResponse | RpcNotification;

export interface RpcErrorPayload {
  code: number;
  message: string;
  data?: unknown;
}

// ---------------------------------------------------------------------------
// Error codes (§9.2).
// ---------------------------------------------------------------------------
//
// The five JSON-RPC standard codes, and deliberately nothing else. A business
// failure is identified by `error.data.type` — the symbolic name — never by its
// number: the numeric space is the runtime's to assign, it has retired codes and
// left holes, and a mirror of it here is a second copy of a table that only one
// side edits. Use errorType(err.data) to branch; use lib/rpcErrors for words.
export const RPC_PARSE_ERROR = -32700;
export const RPC_INVALID_REQUEST = -32600;
export const RPC_METHOD_NOT_FOUND = -32601;
export const RPC_INVALID_PARAMS = -32602;
export const RPC_INTERNAL_ERROR = -32603;

// Read the stable symbolic error name from an RPCError.data.type (§8.2).
// This is the canonical way to branch on errors — never compare codes.
export function errorType(data: unknown): string | undefined {
  if (data && typeof data === "object" && "type" in data) {
    const t = (data as { type: unknown }).type;
    return typeof t === "string" ? t : undefined;
  }
  return undefined;
}

// The per-occurrence `detail` a ProblemData carried (§8.3), and nothing else.
// It used to fall back to the symbolic `type`, which meant every caller wanting
// words got "session_busy" whenever the runtime had no note to add — and, worse,
// filled the field that signals "the runtime said nothing", so the layers that
// own copy never got their turn. Callers that need words use lib/rpcErrors
// (describeProblem / rpcErrorText); branch logic uses errorType.
export function errorDetail(data: unknown): string | undefined {
  if (data && typeof data === "object") {
    const d = (data as { detail?: unknown }).detail;
    if (typeof d === "string" && d) return d;
  }
  return undefined;
}

/** Read a provider-requested retry delay without trusting RpcError.data. */
export function errorRetryAfterSeconds(data: unknown): number | undefined {
  if (data && typeof data === "object") {
    const retryAfterSeconds = (data as { retryAfterSeconds?: unknown }).retryAfterSeconds;
    if (
      typeof retryAfterSeconds === "number" &&
      Number.isInteger(retryAfterSeconds) &&
      retryAfterSeconds >= 0
    ) {
      return retryAfterSeconds;
    }
  }
  return undefined;
}

// Discriminators — used by transport layer to route inbound messages.
export function isRequest(msg: RpcMessage): msg is RpcRequest {
  return "id" in msg && msg.id !== undefined && "method" in msg;
}

export function isResponse(msg: RpcMessage): msg is RpcResponse {
  return "id" in msg && msg.id !== undefined && !("method" in msg);
}

export function isNotification(msg: RpcMessage): msg is RpcNotification {
  return !("id" in msg) || msg.id === undefined;
}

export function isErrorResponse(msg: RpcResponse): msg is RpcResponseError {
  return "error" in msg;
}

// ---------------------------------------------------------------------------
// Inbound envelope gate (trust boundary — CLAUDE.md §3).
// ---------------------------------------------------------------------------

// Validates JSON-RPC 2.0 envelope STRUCTURE only. `result` / `params` stay
// `unknown`: per-method payload schemas are deliberately not maintained (kept
// in sync by review — see API.md / the no-codegen note), and leaving them
// opaque keeps the check O(top-level keys), cheap enough for the per-event
// streaming path. `looseObject` so a forward-compatible envelope extension
// isn't rejected. Which kind a message is (request / response / notification)
// is still routed by the discriminators above, not by this schema.
const RpcEnvelopeSchema = z.looseObject({
  jsonrpc: z.literal(JSONRPC_VERSION),
  id: z.string().optional(),
  method: z.string().optional(),
  params: z.unknown().optional(),
  result: z.unknown().optional(),
  error: z.looseObject({ code: z.number(), message: z.string() }).optional(),
});

// Parse + envelope-validate one raw inbound wire message (the trust boundary
// where untrusted bytes become an RpcMessage). Returns the message on success,
// or null when the text isn't valid JSON or doesn't match the accepted
// JSON-RPC top-level envelope shape — the caller decides whether that means
// "skip this stream frame" or "fail this call". Rejecting garbage here means
// correlation and notification dispatch downstream never see a non-envelope.
export function parseRpcMessage(text: string): RpcMessage | null {
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch {
    return null;
  }
  return RpcEnvelopeSchema.safeParse(json).success ? (json as RpcMessage) : null;
}
