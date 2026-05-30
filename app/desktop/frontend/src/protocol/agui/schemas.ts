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
 * `lyra.approval` payload (API.md §6.9 `ApprovalRequest`). The wire
 * contract makes `command` / `reason` / `risk` optional; the card +
 * handler degrade gracefully when absent. `risk` is a FREE string on the
 * wire — the handler narrows it to the known low/medium/high badge set
 * (anything else → no badge, same neutral fallback as unknown `scope`).
 *
 * `scope` / `target` / `reversible` are UI enrichments beyond the wire
 * contract — optional, richer backends may supply them. Unknown extra
 * keys (`args` / `expiresAt` / `onTimeout`, not consumed by the UI yet)
 * are stripped by Zod, so contract payloads carrying them still validate.
 */
export const ApprovalRequestSchema = z.object({
  // Pre-HITL events omit requestId — the resulting card is a decorative
  // preview with no buttons. Real requests have it.
  requestId: z.string().optional(),
  parentMessageId: z.string(),
  text: z.string(),
  command: z.string().optional(),
  reason: z.string().optional(),
  risk: z.string().optional(),
  scope: z.array(z.string()).optional(),
  /** Path / URL / resource the action will touch. */
  target: z.string().optional(),
  /** True if the action can be undone without side effects. Drives a
   *  "reversible" / "permanent" hint chip on the card. */
  reversible: z.boolean().optional(),
});

// `lyra.approval-result` payload (§6.9 `ApprovalResult`). decision is the
// imperative wire pair "approve" | "deny" (§4.3); the handler maps it to
// the view's past-tense vocabulary when stamping the block.
export const ApprovalResultSchema = z.object({
  requestId: z.string(),
  decision: z.enum(["approve", "deny"]),
});

// `lyra.question` payload (API.md §6.9 QuestionRequest). Clarifying
// questions the agent raises mid-run; the card renders each as a single/
// multi-select with an optional free-text field, and the answer goes back
// via the `runs.question.answer` method (not just a CUSTOM event).
const QuestionItemSchema = z.object({
  id: z.string(),
  question: z.string(),
  header: z.string(),
  options: z.array(
    z.object({ label: z.string(), description: z.string(), preview: z.string().optional() }),
  ),
  multiSelect: z.boolean(),
  allowFreeText: z.boolean().optional(),
});

export const QuestionRequestSchema = z.object({
  // Same pre-HITL convention as approval: a card without requestId is a
  // decorative preview with no submit button.
  requestId: z.string().optional(),
  parentMessageId: z.string(),
  questions: z.array(QuestionItemSchema),
});

// `lyra.question-result` payload (§6.9 QuestionResult) — answered receipt.
export const QuestionResultSchema = z.object({
  requestId: z.string(),
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
