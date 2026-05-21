// Built-in plugins: AG-UI CUSTOM event → view-state handlers.
//
// One plugin per event so users can replace individual ones (e.g. swap
// our `lyra.telemetry` semantics for a custom telemetry shape) without
// touching the others. Co-located here because each handler is a single
// `host.agui.on(...)` call.

import { appendBlockToMessage, definePlugin, patchRun, setPlan } from "@/plugins/sdk";
import {
  CUSTOM,
  type ApprovalRequest,
  type ApprovalResult,
  type CodeProposalPayload,
  type PlanBlockAttachment,
  type PlanSnapshot,
  type SearchResultsPayload,
  type TelemetryPayload,
} from "@/protocol/agui/customEvents";

export const approvalHandler = definePlugin({
  name: "lyra.builtin.approval-handler",
  version: "1.0.0",
  setup({ host }) {
    // Card arrives. `requestId` is carried through so the UI can POST
    // the user's click back to /permission. Pre-HITL backends omit the
    // id; in that case the card renders as a decorative read-only one
    // (ApprovalCard hides its action buttons when requestId is absent).
    host.agui.on<ApprovalRequest>(CUSTOM.APPROVAL, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "approval",
        text: value.text,
        command: value.command,
        reason: value.reason,
        requestId: value.requestId,
      }),
    );

    // Decision follow-up — find the approval block by requestId and
    // stamp `decision` on it so the card switches to its post-decision
    // state. Walks all messages because we don't carry parentMessageId
    // on the result event.
    host.agui.on<ApprovalResult>(CUSTOM.APPROVAL_RESULT, (value) => (state) => ({
      ...state,
      messages: state.messages.map((m) => ({
        ...m,
        blocks: m.blocks.map((b) =>
          b.kind === "approval" && b.requestId === value.requestId
            ? { ...b, decision: value.decision }
            : b,
        ),
      })),
    }));
  },
});

export const codeProposalHandler = definePlugin({
  name: "lyra.builtin.code-proposal-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<CodeProposalPayload>(CUSTOM.CODE_PROPOSAL, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "code",
        lang: value.lang,
        file: value.file,
        text: value.text,
      }),
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
    host.agui.on<PlanSnapshot>(CUSTOM.PLAN, (value) => setPlan(value.items));

    host.agui.on<PlanBlockAttachment>(CUSTOM.PLAN_BLOCK, (value) =>
      appendBlockToMessage(value.messageId, { kind: "plan" }),
    );
  },
});

export const searchResultsHandler = definePlugin({
  name: "lyra.builtin.search-results-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<SearchResultsPayload>(CUSTOM.SEARCH_RESULTS, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "search",
        toolCallId: value.parentMessageId,
        results: value.results,
      }),
    );
  },
});

export const telemetryHandler = definePlugin({
  name: "lyra.builtin.telemetry-handler",
  version: "1.0.0",
  setup({ host }) {
    host.agui.on<TelemetryPayload>(CUSTOM.TELEMETRY, (value) =>
      patchRun({
        step: value.step,
        totalSteps: value.totalSteps,
        activity: value.activity,
        tokens: value.tokens,
        ctxPct: value.ctxPct,
        cost: value.cost,
      }),
    );
  },
});
