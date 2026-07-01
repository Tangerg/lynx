import { t } from "@/lib/i18n";
import type { Interrupt, OpenInterrupt, ToolInvocation } from "@/rpc";
import type { AgentViewState, ContentBlock } from "@/plugins/sdk/types/agentView";
import { appendTimelineEntry } from "@/plugins/sdk";
import { approvalText, commandString, editableArgs, mapQuestion, toolLabel } from "./projections";
import { appendToTurn, patchBlock } from "./fold";

export function materializeInterrupt(
  state: AgentViewState,
  it: Interrupt,
  parentRunId: OpenInterrupt["parentRunId"],
): AgentViewState {
  if (it.type === "approval") {
    // Approval payloads are self-contained ToolInvocation envelopes. Upsert on
    // reconnect/replay so a re-seen interrupt re-affirms the same card.
    const tool = it.payload?.tool as ToolInvocation | undefined;
    if (
      state.messages.some((m) =>
        m.blocks.some((b) => b.kind === "approval" && b.itemId === it.itemId),
      )
    ) {
      return patchBlock(
        state,
        (b) => b.kind === "approval" && b.itemId === it.itemId,
        (b) => ({ ...b, status: "requires-action", parentRunId }),
      );
    }
    const block: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: it.itemId,
      parentRunId,
      text: tool ? approvalText(tool) : t("approval.fallbackText"),
      command: tool ? commandString(tool) : "",
      reason: it.payload?.reason ?? "",
      args: tool ? editableArgs(tool) : undefined,
      risk: it.payload?.risk,
    };
    const withBlock = appendToTurn(state, it.itemId, block);
    return appendTimelineEntry({
      kind: "approval-request",
      refId: it.itemId,
      summary: block.command || toolLabel(tool),
    })(withBlock);
  }
  if (it.type === "question") {
    // The question payload can materialize the card even if item.started was
    // missed while the process was down.
    const hasBlock = state.messages.some((m) =>
      m.blocks.some((b) => b.kind === "question" && b.itemId === it.itemId),
    );
    if (hasBlock) {
      return patchBlock(
        state,
        (b) => b.kind === "question" && b.itemId === it.itemId,
        (b) => ({ ...b, status: "requires-action", parentRunId }),
      );
    }
    return appendToTurn(state, it.itemId, {
      kind: "question",
      status: "requires-action",
      itemId: it.itemId,
      parentRunId,
      questions: mapQuestion(it.payload?.question),
    });
  }
  return state;
}
