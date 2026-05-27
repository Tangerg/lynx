// JSON-RPC 2.0 envelope for the Lyra Runtime Protocol.
//
// See docs/API.md §1 for the full spec. Three message kinds share one
// shape (discriminated by which optional fields are populated):
//
//   Request:      { jsonrpc, id, method, params? }
//   Response:     { jsonrpc, id, result? | error? }    (mutually exclusive)
//   Notification: { jsonrpc, method, params? }         (no id)
//
// Notifications carry both client→runtime (notifications/cancelled,
// runtime.shutdown) and runtime→client (notifications/run/event, …) traffic.

export const JSONRPC_VERSION = "2.0" as const;

export type RpcId = string | number;

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
// Error codes (see docs/API.md §7.1 + §7.2).
// ---------------------------------------------------------------------------

// JSON-RPC 2.0 standard codes.
export const RPC_PARSE_ERROR = -32700;
export const RPC_INVALID_REQUEST = -32600;
export const RPC_METHOD_NOT_FOUND = -32601;
export const RPC_INVALID_PARAMS = -32602;
export const RPC_INTERNAL_ERROR = -32603;

// Lyra business codes in the -32000..-32099 range. Numbers are stable —
// adding a new code requires bumping protocolVersion.
export const RPC_PROVIDER_ERROR = -32001;
export const RPC_PROVIDER_RATE_LIMITED = -32002;
export const RPC_TOOL_FAILED = -32003;
export const RPC_APPROVAL_REQUIRED = -32004;
export const RPC_SESSION_NOT_FOUND = -32005;
export const RPC_MESSAGE_NOT_FOUND = -32006;
export const RPC_RUN_NOT_FOUND = -32007;
export const RPC_ATTACHMENT_TOO_LARGE = -32008;
export const RPC_CAPABILITY_NOT_NEGOTIATED = -32009;
export const RPC_INVALID_PROTOCOL_VERSION = -32010;
export const RPC_PROTOCOL_VIOLATION = -32011;

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
