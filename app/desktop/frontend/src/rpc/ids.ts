// Branded ID types for the Lyra Runtime Protocol (API.md §2.2). Each
// opaque string id carries a phantom tag so TypeScript stops you from
// passing a `RunId` where an `ItemId` is expected — both are strings at
// runtime, but the type checker treats them as distinct.
//
// Business resource ids are ALWAYS server-generated and carry a type
// prefix on the wire: ses_ / run_ / item_ / att_ / tsk_ / evt_. The
// client never mints business ids (§2.2) — only the JSON-RPC envelope id.
// (An X-Idempotency-Key for retry-safe run creation, §10, is the other
// client-minted non-business token, but isn't wired yet — see the
// integration report's transport-conformance gaps.)
//
// Adopt these at boundaries that get ids from the server (RPC method
// returns + notification params). At the wire boundary plain strings
// arrive from JSON.parse; use the `as<X>` helpers to assert intent at the
// parse site — TS can't validate the brand at runtime, so the cast marks
// "I've checked this came from the right field" and stays searchable.

declare const sessionIdBrand: unique symbol;
declare const runIdBrand: unique symbol;
declare const itemIdBrand: unique symbol;
declare const taskIdBrand: unique symbol;
declare const attachmentIdBrand: unique symbol;
declare const eventIdBrand: unique symbol;

export type SessionId = string & { readonly [sessionIdBrand]: never };
export type RunId = string & { readonly [runIdBrand]: never };
export type ItemId = string & { readonly [itemIdBrand]: never };
export type TaskId = string & { readonly [taskIdBrand]: never };
export type AttachmentId = string & { readonly [attachmentIdBrand]: never };
export type EventId = string & { readonly [eventIdBrand]: never };

export const asSessionId = (s: string): SessionId => s as SessionId;
export const asRunId = (s: string): RunId => s as RunId;
export const asItemId = (s: string): ItemId => s as ItemId;
export const asTaskId = (s: string): TaskId => s as TaskId;
export const asAttachmentId = (s: string): AttachmentId => s as AttachmentId;
export const asEventId = (s: string): EventId => s as EventId;
