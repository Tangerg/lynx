import type { ReactNode } from "react";
import { useMemo } from "react";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type { DelegatedRunNarrative } from "@/plugins/builtin/agent/public/conversation";
import { cancelActiveSessionRun } from "@/plugins/builtin/agent/public/run";
import { PlanBlock } from "./cards";
import { MessageContext } from "@/plugins/sdk/messageContext";
import { useCitationSources } from "@/plugins/sdk";
import { openTimelineView } from "@/plugins/builtin/workspace/public/deeplinks";
import { messageBlocksRenderInstant, messageCitations } from "../application/messageBlockModel";
import { CitationContext } from "./CitationContext";
import { DelegatedRunDisclosure } from "./DelegatedRunDisclosure";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { cn } from "@/lib/classNames";
import type { BlockCtx } from "./blockContext";

interface Props {
  narrative: DelegatedRunNarrative;
  ordinal: number;
  siblingCount: number;
  ctx: BlockCtx;
  renderMessageBlocks: (message: Message, ctx: BlockCtx) => ReactNode;
}

export function DelegatedNarrative({
  narrative,
  ordinal,
  siblingCount,
  ctx,
  renderMessageBlocks,
}: Props) {
  const childCtx: BlockCtx = { ...ctx, plan: narrative.plan };
  const hasPlanMarker = narrative.messages.some((message) =>
    message.blocks.some((block) => block.kind === "plan"),
  );
  const hasMaterial = narrative.messages.some((message) => message.blocks.length > 0);

  return (
    <DelegatedRunDisclosure
      run={narrative.run}
      ordinal={ordinal}
      siblingCount={siblingCount}
      hasMaterial={hasMaterial || narrative.plan.length > 0}
      onCancel={() => {
        cancelActiveSessionRun(narrative.run.id);
      }}
      onOpenAudit={openTimelineView}
    >
      {narrative.plan.length > 0 && !hasPlanMarker && <PlanBlock plan={narrative.plan} />}
      <div className="grid gap-2">
        {narrative.messages.map((message) => (
          <DelegatedMessage
            key={message.id}
            message={message}
            ctx={childCtx}
            renderMessageBlocks={renderMessageBlocks}
          />
        ))}
      </div>
    </DelegatedRunDisclosure>
  );
}

function DelegatedMessage({
  message,
  ctx,
  renderMessageBlocks,
}: {
  message: Message;
  ctx: BlockCtx;
  renderMessageBlocks: Props["renderMessageBlocks"];
}) {
  const sources = useCitationSources();
  const citations = useMemo(
    () => messageCitations(message.blocks, sources),
    [message.blocks, sources],
  );
  const blockCtx = messageBlocksRenderInstant(message.role) ? { ...ctx, instant: true } : ctx;

  return (
    <MessageContext.Provider value={message}>
      <CitationContext.Provider value={citations}>
        <div
          className={cn(
            MESSAGE_CONTENT_CLASS,
            "min-w-0 text-pretty text-ui-md leading-relaxed text-fg-soft",
            message.role === "user" && "rounded-md bg-sunken px-3 py-2 text-fg",
          )}
        >
          {renderMessageBlocks(message, blockCtx)}
        </div>
      </CitationContext.Provider>
    </MessageContext.Provider>
  );
}
