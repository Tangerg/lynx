import type { ReactNode } from "react";
import { useMemo } from "react";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type {
  DelegatedRunNarrative,
  TranscriptRow,
  TurnFacts,
} from "@/plugins/builtin/agent/public/conversation";
import { cancelSessionRun } from "@/plugins/builtin/agent/public/run";
import { MessageContext } from "@/plugins/sdk/messageContext";
import { useCurrentMessageSessionId } from "@/plugins/sdk";
import { openTimelineView } from "@/plugins/builtin/workspace/public/deeplinks";
import { messageBlocksRenderInstant } from "../application/messageBlockModel";
import { DelegatedRunDisclosure } from "./DelegatedRunDisclosure";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { cn } from "@/lib/classNames";
import type { BlockCtx } from "./blockContext";

interface Props {
  narrative: DelegatedRunNarrative;
  ordinal: number;
  siblingCount: number;
  /** The spawning turn's facts. A delegated turn renders with them: the projection
   *  reaches through delegation when it slices a row, so a subagent's own tool calls
   *  and any run IT spawned are already in here. */
  facts: TurnFacts;
  ctx: BlockCtx;
  renderMessageBlocks: (row: Pick<TranscriptRow, "message" | "facts">, ctx: BlockCtx) => ReactNode;
}

export function DelegatedNarrative({
  narrative,
  ordinal,
  siblingCount,
  facts,
  ctx,
  renderMessageBlocks,
}: Props) {
  const hasMaterial = narrative.messages.some((message) => message.blocks.length > 0);

  return (
    <DelegatedRunDisclosure
      run={narrative.run}
      ordinal={ordinal}
      siblingCount={siblingCount}
      hasMaterial={hasMaterial}
      onCancel={() => {
        cancelSessionRun({ sessionId: narrative.run.sessionId, runId: narrative.run.id });
      }}
      onOpenAudit={openTimelineView}
    >
      <div className="grid gap-2">
        {narrative.messages.map((message) => (
          <DelegatedMessage
            key={message.id}
            message={message}
            facts={facts}
            ctx={ctx}
            renderMessageBlocks={renderMessageBlocks}
          />
        ))}
      </div>
    </DelegatedRunDisclosure>
  );
}

function DelegatedMessage({
  message,
  facts,
  ctx,
  renderMessageBlocks,
}: {
  message: Message;
  facts: TurnFacts;
  ctx: BlockCtx;
  renderMessageBlocks: Props["renderMessageBlocks"];
}) {
  const sessionId = useCurrentMessageSessionId();
  const blockCtx: BlockCtx = messageBlocksRenderInstant(message.role)
    ? { ...ctx, textReveal: "instant" }
    : ctx;
  const messageContext = useMemo(() => ({ sessionId, message }), [sessionId, message]);

  return (
    <MessageContext.Provider value={messageContext}>
      <div
        className={cn(
          MESSAGE_CONTENT_CLASS,
          "min-w-0 text-pretty text-prose leading-prose text-fg-soft",
          message.role === "user" && "rounded-md bg-sunken px-3 py-2 text-fg",
        )}
      >
        {renderMessageBlocks({ message, facts }, blockCtx)}
      </div>
    </MessageContext.Provider>
  );
}
