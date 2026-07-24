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
// Error codes (docs/protocol/API.md §8.2).
// ---------------------------------------------------------------------------
//
// IMPORTANT: codes are v2-fresh and NOT guaranteed to match any prior
// baseline. client + server judge errors by `error.data.type` (the
// symbolic name in `ProblemData.type`), NOT by the numeric code — the
// number is only a coarse classification. `errorType(err)` below reads
// the name; prefer it over comparing codes.

// JSON-RPC 2.0 standard codes.
export const RPC_PARSE_ERROR = -32700;
export const RPC_INVALID_REQUEST = -32600;
export const RPC_METHOD_NOT_FOUND = -32601;
export const RPC_INVALID_PARAMS = -32602;
export const RPC_INTERNAL_ERROR = -32603;

// Lyra business codes (§8.2).
export const RPC_PROVIDER_ERROR = -32001;
export const RPC_SESSION_NOT_FOUND = -32002;
export const RPC_RUN_NOT_FOUND = -32003;
export const RPC_ITEM_NOT_FOUND = -32004;
export const RPC_CWD_UNAVAILABLE = -32005;
export const RPC_CAPABILITY_NOT_NEGOTIATED = -32006;
// NOTE: there is no -32007 / run_not_running — a run is two-state
// (running|finished), so "not running" collapses into run_already_finished (§8.2).
export const RPC_RUN_ALREADY_FINISHED = -32008;
export const RPC_CHECKPOINT_UNAVAILABLE = -32009;
// -32010 (attachment_too_large) retired with the attachment upload domain
// (MULTIMODAL_IMAGE_INPUT, 2026-06-14) — left as a hole, never reused.
export const RPC_UNSUPPORTED_MIME = -32011; // image block mime: not an image type / unparseable
// -32012 (tool_denied) retired with the runtime's tool-denied error code (the
// backend no longer emits it; denial surfaces through the approval flow) — left
// as a hole, never reused.
export const RPC_PATH_OUTSIDE_ROOT = -32013;
export const RPC_INTERRUPT_NOT_OPEN = -32014;
export const RPC_IDEMPOTENCY_CONFLICT = -32015;
export const RPC_INVALID_PROTOCOL_VERSION = -32016;

// Read the stable symbolic error name from an RPCError.data.type (§8.2).
// This is the canonical way to branch on errors — never compare codes.
export function errorType(data: unknown): string | undefined {
  if (data && typeof data === "object" && "type" in data) {
    const t = (data as { type: unknown }).type;
    return typeof t === "string" ? t : undefined;
  }
  return undefined;
}

// Human-readable explanation from a ProblemData (§8.3): the per-occurrence
// `detail`, falling back to the symbolic `type`. For surfacing an error to a
// user inline (e.g. a failed providers.test) — branch logic still uses errorType.
export function errorDetail(data: unknown): string | undefined {
  if (data && typeof data === "object") {
    const d = (data as { detail?: unknown }).detail;
    if (typeof d === "string" && d) return d;
  }
  return errorType(data);
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
