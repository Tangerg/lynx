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
    host.agui.on<ApprovalRequest>(CUSTOM.APPROVAL, (value) =>
      appendBlockToMessage(value.parentMessageId, {
        kind: "approval",
        text: value.text,
        command: value.command,
        reason: value.reason,
      }),
    );
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
