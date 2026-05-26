// Zod schemas for CUSTOM AG-UI event payloads.
//
// Customs cross the wire from the Go runtime, so the React side can't
// trust their shape from TypeScript alone. Each schema mirrors the
// corresponding interface in `customEvents.ts` and is applied at the
// handler boundary in `agui-handlers/*` — a malformed payload is
// rejected with a structured error instead of throwing later in render.
//
// Internal data (Zustand stores, plugin SDK types, React props) does
// NOT need schemas — TypeScript covers those. Validate at trust
// boundaries only.

import type { CustomEventHandler, StateUpdate } from "@/plugins/sdk";
import { z } from "zod";

/**
 * Wrap a typed handler with runtime validation. If the payload doesn't
 * match the schema, log the structured Zod issues and return void —
 * leaving state untouched is safer than letting a downstream `.text`
 * deref throw under a streaming run.
 *
 * Use at the CUSTOM-event boundary, not internally — TypeScript already
 * covers in-process data flow.
 */
export function withSchema<T>(
  name: string,
  schema: z.ZodType<T>,
  handler: (value: T) => StateUpdate | void,
): CustomEventHandler<unknown> {
  return (raw) => {
    const result = schema.safeParse(raw);
    if (!result.success) {
      console.error(`[agui] "${name}" payload rejected by schema:`, z.treeifyError(result.error));
      return;
    }
    return handler(result.data);
  };
}

export const PlanItemSchema = z.object({
  id: z.number(),
  pid: z.string(),
  status: z.enum(["done", "doing", "todo"]),
  text: z.string(),
});

export const PlanSnapshotSchema = z.object({
  items: z.array(PlanItemSchema),
});

export const PlanBlockAttachmentSchema = z.object({
  messageId: z.string(),
});

/**
 * Risk metadata on an approval request. All optional — backends that
 * haven't been updated yet keep working; richer backends supply these
 * so the card can render a risk badge, scope chips, target path, and
 * a "reversible" hint (UX review §2.3 P0.5 — Approval Risk Model).
 *
 * `scope` is open-ended on purpose. The card renders any string as a
 * chip; backends that want stricter typing can validate themselves.
 * Built-in scopes the UI knows how to colour-code: read / write /
 * network / shell / delete.
 */
const ApprovalRiskSchema = z.enum(["low", "medium", "high"]);

export const ApprovalRequestSchema = z.object({
  // Pre-HITL events omit requestId — the resulting card is a decorative
  // preview with no buttons. Real requests have it.
  requestId: z.string().optional(),
  parentMessageId: z.string(),
  text: z.string(),
  command: z.string(),
  reason: z.string(),
  risk: ApprovalRiskSchema.optional(),
  scope: z.array(z.string()).optional(),
  /** Path / URL / resource the action will touch. */
  target: z.string().optional(),
  /** True if the action can be undone without side effects. Drives a
   *  "reversible" / "permanent" hint chip on the card. */
  reversible: z.boolean().optional(),
});

export const ApprovalResultSchema = z.object({
  requestId: z.string(),
  decision: z.enum(["approved", "declined"]),
});

export const SearchResultItemSchema = z.object({
  domain: z.string(),
  title: z.string(),
  time: z.string(),
  snippet: z.string(),
});

export const SearchResultsPayloadSchema = z.object({
  parentMessageId: z.string(),
  results: z.array(SearchResultItemSchema),
});

export const CodeProposalPayloadSchema = z.object({
  parentMessageId: z.string(),
  lang: z.string(),
  file: z.string(),
  text: z.string(),
});

export const TelemetryPayloadSchema = z.object({
  step: z.number(),
  totalSteps: z.number(),
  activity: z.string(),
  tokens: z.object({
    used: z.string(),
    total: z.string(),
  }),
  ctxPct: z.number(),
  cost: z.string(),
});
