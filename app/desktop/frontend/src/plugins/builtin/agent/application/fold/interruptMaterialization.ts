import type { AgentInterrupt } from "@/plugins/sdk";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { appendTimelineEntry } from "@/plugins/sdk";
import { commandString, editableArgs, mapQuestion, toolLabel } from "./projections";
import { appendToTurn, markToolRequiresAction, patchRunBlock } from "./fold";
import type { AgentFoldSource } from "./source";
import { sourceTimestamp } from "./source";

export function materializeInterrupt(
  state: AgentSessionView,
  interrupt: AgentInterrupt,
  source: AgentFoldSource,
  resumeRunId: string = source.runId,
): AgentSessionView {
  const withToolStatus = markToolRequiresAction(state, source.runId, interrupt.itemId);
  if (interrupt.type === "approval") {
    // Approval payloads are self-contained ToolInvocation envelopes. Upsert on
    // reconnect/replay so a re-seen interrupt re-affirms the same card.
    if (
      withToolStatus.messages.some(
        (message) =>
          message.runId === source.runId &&
          message.blocks.some(
            (block) => block.kind === "approval" && block.itemId === interrupt.itemId,
          ),
      )
    ) {
      return patchRunBlock(
        withToolStatus,
        source.runId,
        (b) => b.kind === "approval" && b.itemId === interrupt.itemId,
        (b) => ({
          ...b,
          status: "requires-action",
          runId: resumeRunId,
          rememberable: interrupt.payload.rememberable ?? false,
        }),
      );
    }
    const tool = interrupt.payload.tool;
    const block: ContentBlock = {
      kind: "approval",
      status: "requires-action",
      itemId: interrupt.itemId,
      runId: resumeRunId,
      toolName: tool?.name,
      command: tool ? commandString(tool) : "",
      reason: interrupt.payload.reason ?? "",
      args: tool ? editableArgs(tool) : undefined,
      rememberable: interrupt.payload.rememberable ?? false,
    };
    const withBlock = appendToTurn(
      withToolStatus,
      source.runId,
      interrupt.itemId,
      block,
      source.timestamp,
    );
    return appendTimelineEntry({
      id: `timeline:${source.eventId}:approval-request:${interrupt.itemId}`,
      ts: sourceTimestamp(source),
      kind: "approval-request",
      runId: source.runId,
      refId: interrupt.itemId,
      summary: block.command || toolLabel(tool),
    })(withBlock);
  }
  if (interrupt.type === "question") {
    // The question payload can materialize the card even if item.started was
    // missed while the process was down.
    const hasBlock = withToolStatus.messages.some(
      (message) =>
        message.runId === source.runId &&
        message.blocks.some(
          (block) => block.kind === "question" && block.itemId === interrupt.itemId,
        ),
    );
    if (hasBlock) {
      return patchRunBlock(
        withToolStatus,
        source.runId,
        (b) => b.kind === "question" && b.itemId === interrupt.itemId,
        (b) => ({ ...b, status: "requires-action", runId: resumeRunId }),
      );
    }
    return appendToTurn(
      withToolStatus,
      source.runId,
      interrupt.itemId,
      {
        kind: "question",
        status: "requires-action",
        itemId: interrupt.itemId,
        runId: resumeRunId,
        questions: mapQuestion(interrupt.payload.question),
      },
      source.timestamp,
    );
  }
  return withToolStatus;
}
