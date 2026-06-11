// Typed exception thrown when a JSON-RPC Response carries an `error`.
// Wraps the raw payload so callers can switch on `code` (RPC_* constants
// from ./types) without parsing the message string.

import { errorType, type RpcErrorPayload } from "./types";

/** True when `error` is a JSON-RPC business error of the given ProblemData
 *  `type` (API.md §8: judge errors by type, never by code or message). The
 *  one idiom every "this failure is an expected state" branch needs —
 *  capability gating, vcs_unavailable, session_busy. */
export function isErrorType(error: unknown, type: string): boolean {
  return error instanceof RpcError && errorType(error.data) === type;
}

export class RpcError extends Error {
  readonly code: number;
  readonly data: unknown;

  constructor(payload: RpcErrorPayload) {
    super(payload.message);
    this.name = "RpcError";
    this.code = payload.code;
    this.data = payload.data;
  }

  toPayload(): RpcErrorPayload {
    return { code: this.code, message: this.message, data: this.data };
  }
}

// Lower-level transport failure — used when an HTTP request fails before
// we get a JSON-RPC response back (network error, 4xx/5xx that aren't
// JSON-RPC envelope, etc.). The HTTP status mapping in docs/protocol/API.md §7.3
// says 401/500/503 return flat JSON not envelope, so we surface those
// here without a JSON-RPC error code.
export class RpcTransportError extends Error {
  readonly status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = "RpcTransportError";
    this.status = status;
  }
}
