// Branded ID types for the Lyra Runtime Protocol. Each opaque string
// id carries a phantom tag so TypeScript stops you from passing a
// `RunId` where a `MessageId` is expected — both are strings at
// runtime, but the type checker treats them as distinct.
//
// Adopt these at boundaries that get IDs from the server (RPC method
// returns + notification params) and at any function that accepts
// multiple ID kinds (`fork(parentId, atMessageId)` is the canonical
// case). Internal-only string keys (e.g. `Map<string, X>` where the
// string never crosses a module boundary) can stay plain — the brand
// is overhead without payoff.
//
// At the wire boundary, plain strings arrive from JSON.parse. Use the
// `as<X>` helpers to assert intent at the parse site — TS doesn't
// validate the brand at runtime (it can't), so the cast is a marker
// for "I've checked this came from the right field". One-liner cast
// helpers (vs. type assertions inline) make every adoption explicit
// and searchable.

declare const sessionIdBrand: unique symbol;
declare const runIdBrand: unique symbol;
declare const messageIdBrand: unique symbol;
declare const toolCallIdBrand: unique symbol;
declare const taskIdBrand: unique symbol;
declare const attachmentIdBrand: unique symbol;
declare const approvalRequestIdBrand: unique symbol;

export type SessionId = string & { readonly [sessionIdBrand]: never };
export type RunId = string & { readonly [runIdBrand]: never };
export type MessageId = string & { readonly [messageIdBrand]: never };
export type ToolCallId = string & { readonly [toolCallIdBrand]: never };
export type TaskId = string & { readonly [taskIdBrand]: never };
export type AttachmentId = string & { readonly [attachmentIdBrand]: never };
export type ApprovalRequestId = string & { readonly [approvalRequestIdBrand]: never };

export const asSessionId = (s: string): SessionId => s as SessionId;
export const asRunId = (s: string): RunId => s as RunId;
export const asMessageId = (s: string): MessageId => s as MessageId;
export const asToolCallId = (s: string): ToolCallId => s as ToolCallId;
export const asTaskId = (s: string): TaskId => s as TaskId;
export const asAttachmentId = (s: string): AttachmentId => s as AttachmentId;
export const asApprovalRequestId = (s: string): ApprovalRequestId => s as ApprovalRequestId;
