import { t } from "@/lib/i18n";
import type { Interrupt } from "@/rpc";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { AgentViewState } from "@/plugins/sdk/types/agentView";
import { appendTimelineEntry } from "@/plugins/sdk";
import { approvalText, commandString, editableArgs, mapQuestion, toolLabel } from "./projections";
import { appendToTurn, markToolRequiresAction, patchBlock } from "./fold";

export function materializeInterrupt(
  state: AgentViewState,
  it: Interrupt,
  runId: string,
): AgentViewState {
  const withToolStatus = markToolRequiresAction(state, it.itemId);
  if (it.type === "approval") {
    // Approval payloads are self-contained ToolInvocation envelopes. Upsert on
    // reconnect/replay so a re-seen interrupt re-affirms the same card.
    const tool = it.payload?.tool;
    if (
      withToolStatus.messages.some((m) =>
        m.blocks.some((b) => b.kind === "approval" && b.itemId === it.itemId),
      )
    ) {
      return patchBlock(
        withToolStatus,
        (b) => b.kind === "approval" && b.itemId === it.itemId,
        (b) => ({
          ...b,
          status: "requires-action",
          runId,
          rememberable: it.payload?.rememberable ?? false,
        }),
      );
    }
    const block: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: it.itemId,
      runId,
      text: tool ? approvalText(tool) : t("approval.fallbackText"),
      command: tool ? commandString(tool) : "",
      reason: it.payload?.reason ?? "",
      args: tool ? editableArgs(tool) : undefined,
      risk: it.payload?.risk,
      rememberable: it.payload?.rememberable ?? false,
    };
    const withBlock = appendToTurn(withToolStatus, it.itemId, block);
    return appendTimelineEntry({
      kind: "approval-request",
      refId: it.itemId,
      summary: block.command || toolLabel(tool),
    })(withBlock);
  }
  if (it.type === "question") {
    // The question payload can materialize the card even if item.started was
    // missed while the process was down.
    const hasBlock = withToolStatus.messages.some((m) =>
      m.blocks.some((b) => b.kind === "question" && b.itemId === it.itemId),
    );
    if (hasBlock) {
      return patchBlock(
        withToolStatus,
        (b) => b.kind === "question" && b.itemId === it.itemId,
        (b) => ({ ...b, status: "requires-action", runId }),
      );
    }
    return appendToTurn(withToolStatus, it.itemId, {
      kind: "question",
      status: "requires-action",
      itemId: it.itemId,
      runId,
      questions: mapQuestion(it.payload?.question),
    });
  }
  return withToolStatus;
}
