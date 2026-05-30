// Built-in plugins: AG-UI CUSTOM event → view-state handlers.
//
// One plugin per event so users can replace individual ones (e.g. swap
// our `lyra.telemetry` semantics for a custom telemetry shape) without
// touching the others. Co-located here because each handler is a single
// `host.agui.on(...)` call.

import {
  appendBlockToMessage,
  appendTimelineEntry,
  compose,
  definePlugin,
  patchRun,
  setPlan,
} from "@/plugins/sdk";
import { CUSTOM } from "@/protocol/agui/customEvents";
import {
  ApprovalRequestSchema,
  ApprovalResultSchema,
  CodeProposalPayloadSchema,
  PlanBlockAttachmentSchema,
  PlanSnapshotSchema,
  QuestionRequestSchema,
  QuestionResultSchema,
  SearchResultsPayloadSchema,
  TelemetryPayloadSchema,
  withSchema,
} from "@/protocol/agui/schemas";

// Wire ↔ view vocab. The protocol uses the imperative pair (§4.3); the
// view layer + ApprovalCard speak past-tense (mirrors useApprovalSubmit).
const VIEW_DECISION = { approve: "approved", deny: "declined" } as const;
const KNOWN_RISK = new Set(["low", "medium", "high"]);
const narrowRisk = (r: string | undefined): "low" | "medium" | "high" | undefined =>
  r !== undefined && KNOWN_RISK.has(r) ? (r as "low" | "medium" | "high") : undefined;

export const approvalHandler = definePlugin({
  name: "lyra.builtin.approval-handler",
  version: "1.0.0",
  setup({ host }) {
    // Card arrives. `requestId` is carried through so the UI can POST
    // the user's click back to /permission. Pre-HITL backends omit the
    // id; in that case the card renders as a decorative read-only one
    // (ApprovalCard hides its action buttons when requestId is absent).
    host.agui.on(
      CUSTOM.APPROVAL,
      withSchema(CUSTOM.APPROVAL, ApprovalRequestSchema, (value) =>
        compose(
          appendBlockToMessage(value.parentMessageId, {
            kind: "approval",
            status: "requires-action",
            text: value.text,
            // command / reason are optional on the wire (§6.9); block fields
            // are required strings, so default to empty.
            command: value.command ?? "",
            reason: value.reason ?? "",
            requestId: value.requestId,
            risk: narrowRisk(value.risk),
            scope: value.scope,
            target: value.target,
            reversible: value.reversible,
          }),
          appendTimelineEntry({
            kind: "approval-request",
            summary: value.command || value.text,
            refId: value.requestId,
          }),
        ),
      ),
    );

    // Decision follow-up — find the approval block by requestId and
    // stamp `decision` on it so the card switches to its post-decision
    // state. Walks all messages because we don't carry parentMessageId
    // on the result event.
    host.agui.on(
      CUSTOM.APPROVAL_RESULT,
      withSchema(CUSTOM.APPROVAL_RESULT, ApprovalResultSchema, (value) =>
        compose(
          (state) => ({
            ...state,
            messages: state.messages.map((m) => ({
              ...m,
              blocks: m.blocks.map((b) =>
                b.kind === "approval" && b.requestId === value.requestId
                  ? { ...b, status: "complete", decision: VIEW_DECISION[value.decision] }
                  : b,
              ),
            })),
          }),
          appendTimelineEntry({
            kind: "approval-result",
            refId: value.requestId,
            status: VIEW_DECISION[value.decision],
          }),
        ),
      ),
    );
  },
});

export const questionHandler = definePlugin({
  name: "lyra.builtin.question-handler",
  version: "1.0.0",
  setup({ host }) {
    // Clarifying question arrives → render a single/multi-select card.
    // Same pre-HITL convention as approval: no requestId ⇒ decorative.
    host.agui.on(
      CUSTOM.QUESTION,
      withSchema(CUSTOM.QUESTION, QuestionRequestSchema, (value) =>
        appendBlockToMessage(value.parentMessageId, {
          kind: "question",
          status: "requires-action",
          requestId: value.requestId,
          questions: value.questions,
        }),
      ),
    );

    // Answer landed (runs.question.answer accepted) → stamp the matching
    // block answered + collapse it. Walks all messages (no parentMessageId
    // on the result event), mirroring the approval-result handler.
    host.agui.on(
      CUSTOM.QUESTION_RESULT,
      withSchema(CUSTOM.QUESTION_RESULT, QuestionResultSchema, (value) => (state) => ({
        ...state,
        messages: state.messages.map((m) => ({
          ...m,
          blocks: m.blocks.map((b) =>
            b.kind === "question" && b.requestId === value.requestId
              ? { ...b, status: "complete", answered: true }
              : b,
          ),
        })),
      })),
    );
  },
});

export const codeProposalHandler = definePlugin({
  name: "lyra.builtin.code-proposal-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on(
      CUSTOM.CODE_PROPOSAL,
      withSchema(CUSTOM.CODE_PROPOSAL, CodeProposalPayloadSchema, (value) =>
        appendBlockToMessage(value.parentMessageId, {
          kind: "code",
          lang: value.lang,
          file: value.file,
          text: value.text,
        }),
      ),
    );
  },
});

// `lyra.plan` replaces the plan snapshot. `lyra.plan-block` appends a
// plan content block to a message — both ride on the same plugin since
// they describe the same domain.
export const planHandler = definePlugin({
  name: "lyra.builtin.plan-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on(
      CUSTOM.PLAN,
      withSchema(CUSTOM.PLAN, PlanSnapshotSchema, (value) => setPlan(value.items)),
    );

    host.agui.on(
      CUSTOM.PLAN_BLOCK,
      withSchema(CUSTOM.PLAN_BLOCK, PlanBlockAttachmentSchema, (value) =>
        appendBlockToMessage(value.messageId, { kind: "plan" }),
      ),
    );
  },
});

export const searchResultsHandler = definePlugin({
  name: "lyra.builtin.search-results-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on(
      CUSTOM.SEARCH_RESULTS,
      withSchema(CUSTOM.SEARCH_RESULTS, SearchResultsPayloadSchema, (value) =>
        appendBlockToMessage(value.parentMessageId, {
          kind: "search",
          results: value.results,
        }),
      ),
    );
  },
});

export const telemetryHandler = definePlugin({
  name: "lyra.builtin.telemetry-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on(
      CUSTOM.TELEMETRY,
      withSchema(CUSTOM.TELEMETRY, TelemetryPayloadSchema, (value) =>
        patchRun({
          step: value.step,
          totalSteps: value.totalSteps,
          activity: value.activity,
          tokens: value.tokens,
          ctxPct: value.ctxPct,
          cost: value.cost,
        }),
      ),
    );
  },
});
