// Single home for translating protocol error types (API.md §8.2) into
// user-facing copy — the same type must read the same everywhere it
// surfaces. Branch logic still uses isErrorType at the call site when an
// error type changes BEHAVIOR (retry, keep-input-open); this table only
// owns the words.

import { errorDetail, errorType, RPC_METHOD_NOT_FOUND, RpcError, RpcTransportError } from "@/rpc";

const ERROR_COPY: Record<string, string> = {
  session_busy: "Session is busy — wait for the current run to finish.",
  checkpoint_unavailable: "No file checkpoint for that turn — nothing was changed.",
  cwd_unavailable: "That path does not exist on the runtime's disk.",
  vcs_unavailable: "This folder isn't a git repository.",
  provider_error: "The model provider didn't respond — try again.",
  agent_stuck: "The agent stopped making progress — try rephrasing or narrowing the task.",
  // 613 — B7 code intel / B8 file read / B12 MCP auth (all expected, UI-inline).
  no_language_server:
    "No language server for this file type — code intelligence isn't available here.",
  is_a_directory: "That path is a directory — pick a file to read.",
  file_too_large: "File is too large to read whole — request a line range instead.",
  mcp_auth_failed: "Authentication was rejected — check the token and try again.",
};

/** Friendly copy for a mapped protocol error type; undefined otherwise.
 *  Callers append their own context-specific fallback. */
export function describeRpcError(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  const type = errorType(err.data);
  return type ? ERROR_COPY[type] : undefined;
}

/** Best human-readable text for any RPC error: mapped copy, then the
 *  server's per-occurrence detail, then the raw message. Undefined for
 *  non-RPC errors (transport failures, programming errors). */
export function rpcErrorText(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeRpcError(err) ?? errorDetail(err.data) ?? err.message;
}

/** True when a call failed because the connected runtime doesn't implement the
 *  method — an unregistered RPC, e.g. a 613 proposal-surface method (file-tree
 *  browse, remembered-approval management) the backend hasn't shipped yet. lynx
 *  answers an unknown method with HTTP 404 + a -32601 envelope, so the HTTP
 *  transport surfaces it as a RpcTransportError(status 404); an in-process
 *  transport would surface the JSON-RPC -32601 directly as an RpcError. Lets a
 *  panel for a not-yet-supported feature render a calm "unavailable on this
 *  runtime" state instead of a hard error. */
export function isUnsupportedMethod(err: unknown): boolean {
  return (
    (err instanceof RpcTransportError && err.status === 404) ||
    (err instanceof RpcError && err.code === RPC_METHOD_NOT_FOUND)
  );
}
