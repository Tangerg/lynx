// Single home for translating protocol error types (API.md §8.2) into
// user-facing copy — the same type must read the same everywhere it
// surfaces. Branch logic still uses isErrorType at the call site when an
// error type changes BEHAVIOR (retry, keep-input-open); this table only
// owns which words.
//
// The words themselves live in the locale catalogs. They were English string
// literals here, which put user copy in a utility module and outside the eight
// dictionaries the locale guard checks — the app shipped fifteen sentences no
// translator could see. The key's leaf is the wire symbol verbatim so the table
// stays a one-to-one map against the protocol.

import { errorDetail, errorType, RPC_METHOD_NOT_FOUND, RpcError, RpcTransportError } from "@/rpc";
import { t } from "./i18n";

const MAPPED_TYPES: readonly string[] = [
  "session_busy",
  "checkpoint_unavailable",
  "cwd_unavailable",
  "vcs_unavailable",
  // Provider failures, one stable symbol per mode (API.md §8.4) so copy +
  // behavior branch on the symbol, never on free-text detail. provider_error
  // stays as the generic fallback for an uncategorized provider failure.
  "rate_limited",
  "invalid_api_key",
  "timeout",
  "provider_unavailable",
  "provider_rejected",
  "provider_error",
  "agent_stuck",
  // Run terminals that carry no per-occurrence detail: the runtime classified
  // the failure but has nothing case-specific to add (an internal error must not
  // put its internals on the wire), so the words are ours to supply.
  "internal_error",
  "run_lost",
  // 613 — B7 code intel / B8 file read / B12 MCP auth (all expected, UI-inline).
  "no_language_server",
  "is_a_directory",
  "file_too_large",
  "mcp_auth_failed",
];

/** Friendly copy for a mapped protocol error type; undefined for an unmapped
 *  one. Callers append their own context-specific fallback. */
export function describeErrorType(type: string | undefined): string | undefined {
  return type && MAPPED_TYPES.includes(type) ? t(`rpcError.${type}`) : undefined;
}

/** Friendly copy for a mapped protocol error type; undefined otherwise.
 *  Callers append their own context-specific fallback. */
export function describeRpcError(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeErrorType(errorType(err.data));
}

/** Best human-readable text for any RPC error: mapped copy, then the
 *  server's per-occurrence detail, then the raw message. Undefined for
 *  non-RPC errors (transport failures, programming errors). */
export function rpcErrorText(err: unknown): string | undefined {
  if (!(err instanceof RpcError)) return undefined;
  return describeRpcError(err) ?? errorDetail(err.data) ?? err.message;
}

/** True when a call failed because the connected runtime doesn't implement the
 *  method — for example an older or custom runtime missing an optional surface. lynx
 *  answers an unknown method with HTTP 404 + a -32601 envelope, so the HTTP
 *  transport surfaces it as a RpcTransportError(status 404); an in-process
 *  transport would surface the JSON-RPC -32601 directly as an RpcError. Lets a
 *  panel for an unavailable feature render a calm "unavailable on this
 *  runtime" state instead of a hard error. */
export function isUnsupportedMethod(err: unknown): boolean {
  return (
    (err instanceof RpcTransportError && err.status === 404) ||
    (err instanceof RpcError && err.code === RPC_METHOD_NOT_FOUND)
  );
}
