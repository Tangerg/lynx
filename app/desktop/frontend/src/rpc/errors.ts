// Typed exception thrown when a JSON-RPC Response carries an `error` — or when the
// SDK's capability preflight refuses a call the negotiation already ruled out, which
// is the same refusal with the round-trip removed. Wraps the raw payload so callers
// branch on the problem TYPE rather than parsing the message string.

import { errorType, type RpcErrorPayload } from "./types";
import type { WireViolation } from "./wireCheck";

/** Stable diagnostic text even when a dependency throws a non-Error value. */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/** True when `error` is a JSON-RPC business error of the given ProblemData
 *  `type` (API.md §8: judge errors by type, never by code or message). The
 *  one idiom every "this failure is an expected state" branch needs —
 *  capability gating, vcs_unavailable, session_busy. */
export function isErrorType(error: unknown, type: string): boolean {
  return error instanceof RpcError && errorType(error.data) === type;
}

export class RpcError extends Error {
  /** The wire's coarse numeric class, absent when the refusal never crossed the wire
   *  (the capability preflight below). Only the five standard JSON-RPC codes are
   *  named by this client — a business failure is identified by `data.type`, which is
   *  the stable name, and this number is a classification the protocol is free to
   *  renumber. */
  readonly code?: number;
  readonly data: unknown;

  constructor(payload: LocalRefusal | RpcErrorPayload) {
    super(payload.message);
    this.name = "RpcError";
    this.code = "code" in payload ? payload.code : undefined;
    this.data = payload.data;
  }
}

/** A refusal this client produced itself. It has no numeric code because no server
 *  answered it: the preflight already holds the negotiation that would say no. */
interface LocalRefusal {
  message: string;
  data?: unknown;
}

// Lower-level transport failure — used when an HTTP request fails before
// we get a JSON-RPC response back (network error, 4xx/5xx that aren't
// JSON-RPC envelope, etc.). The HTTP status mapping in docs/protocol/API.md §7.3
// says 401/500/503 return flat JSON not envelope, so we surface those
// here without a JSON-RPC error code.
export class RpcTransportError extends Error {
  readonly status?: number;
  readonly requestId?: string;
  readonly problemType?: string;

  constructor(message: string, status?: number, requestId?: string, problemType?: string) {
    super(message);
    this.name = "RpcTransportError";
    this.status = status;
    this.requestId = requestId;
    this.problemType = problemType;
  }
}

/** An inbound JSON-RPC frame contradicted the generated Runtime contract. */
export class RpcProtocolError extends Error {
  readonly violations: readonly WireViolation[];

  constructor(subject: string, violations: readonly WireViolation[]) {
    const detail = violations
      .map((violation) => `${violation.path} ${violation.detail}`)
      .join("; ");
    super(`invalid ${subject}: ${detail}`);
    this.name = "RpcProtocolError";
    this.violations = violations;
  }
}

interface TransportProblem {
  type?: string;
  detail?: string;
  requestId?: string;
}

/** Parse an RFC 9457-style transport problem without trusting its shape. */
export function parseTransportProblem(text: string): TransportProblem | undefined {
  try {
    const value: unknown = JSON.parse(text);
    if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
    const fields = value as Record<string, unknown>;
    return {
      type: typeof fields.type === "string" ? fields.type : undefined,
      detail: typeof fields.detail === "string" ? fields.detail : undefined,
      requestId: typeof fields.requestId === "string" ? fields.requestId : undefined,
    };
  } catch {
    return undefined;
  }
}
