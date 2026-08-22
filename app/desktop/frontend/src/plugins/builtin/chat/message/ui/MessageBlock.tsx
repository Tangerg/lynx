import type { BlockCtx } from "./BlockRenderer";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { memo, useMemo, type ReactNode } from "react";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import {
  messageActionMaterialization,
  messageBlocksRenderInstant,
} from "../application/messageBlockModel";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { MessageContextMenu } from "./MessageContextMenu";
import { renderBlock, renderMessageBlocks } from "./BlockRenderer";
import {
  MessageVisibleMaterialOwner,
  MessageVisibleMaterialProvider,
  useVisibleActionMaterialization,
} from "./messageVisibleMaterial";

function MessageBlockInner({
  row,
  ctx,
  sessionId,
  isLast,
  isRunning,
  answerFollows = false,
  terminalFooter,
}: {
  row: TranscriptRow;
  ctx: BlockCtx;
  sessionId: string;
  /** Last turn in the thread — its action bar stays pinned open. */
  isLast: boolean;
  /** A run is streaming — action bars stay hidden until it settles.
   *  Flips only at run boundaries, so it never churns this memo per token. */
  isRunning: boolean;
  /** This work row is immediately followed by its exact Runtime-authored final answer. */
  answerFollows?: boolean;
  /** Session-level material that belongs after this exact turn settles visibly. */
  terminalFooter?: ReactNode;
}) {
  const msg = row.message;
  const isUser = msg.role === "user";
  const t = useT();
  const messageContext = useMemo(() => ({ sessionId, message: msg }), [sessionId, msg]);

  const visibleMaterialOwner = useMemo(
    () => new MessageVisibleMaterialOwner(sessionId, msg.id),
    [msg.id, sessionId],
  );
  const acceptedActionMaterialization = messageActionMaterialization(row);
  const visibleMaterialGeneration =
    acceptedActionMaterialization === "active" ? visibleMaterialOwner : row;
  const actionMaterialization = useVisibleActionMaterialization(
    visibleMaterialOwner,
    acceptedActionMaterialization,
    visibleMaterialGeneration,
  );
  const actionsVisibility =
    msg.phase === "commentary"
      ? "absent"
      : messageActionsVisibility({
          materialization: actionMaterialization,
          isRunning,
          isLast,
        });

  // System messages (e.g. a compaction boundary) are chrome-less full-width
  // notes — no avatar / name / time / outline / context-menu, just the block(s)
  // rendered inline (CompactionBlock draws its own divider). Placed after all
  // hooks so rules-of-hooks holds.
  if (msg.role === "system") {
    return (
      <MessageContext.Provider value={messageContext}>
        <div className={MESSAGE_CONTENT_CLASS}>
          {msg.blocks.map((block, index) => renderBlock(block, index, row.facts, ctx))}
        </div>
      </MessageContext.Provider>
    );
  }

  const blockCtx: BlockCtx = messageBlocksRenderInstant(msg.role)
    ? { ...ctx, textReveal: "instant" }
    : ctx;

  const content = renderMessageBlocks(row, blockCtx, answerFollows);

  const roleLabel = t(isUser ? "role.user" : "role.assistant");

  // A message whose only material is the pending Question is presented by the
  // composer request owner. Leaving its empty transcript wrapper behind would
  // preserve duplicate rhythm even after removing the duplicate card.
  if (content.length === 0) return null;

  const messageContent = (
    <div
      data-user-message-bubble={isUser ? "" : undefined}
      className={cn(
        MESSAGE_CONTENT_CLASS,
        "min-w-0 text-pretty leading-prose text-prose text-fg",
        // Match Codex's quoted-human geometry and neutral 5% ink wash.
        // A prompt owns a distinct material, but it is not a selected row
        // or semantic status and must not inherit the user's accent.
        isUser && "max-w-[77%] rounded-bubble bg-user-message px-3 py-2",
      )}
    >
      {content}
    </div>
  );

  return (
    <MessageContext.Provider value={messageContext}>
      <MessageVisibleMaterialProvider
        owner={visibleMaterialOwner}
        generation={visibleMaterialGeneration}
      >
        <div className={cn("group relative flex min-w-0 flex-col gap-2", isUser && "items-end")}>
          <h4 className="sr-only select-none">{roleLabel}</h4>
          {msg.phase === "commentary" ? (
            messageContent
          ) : (
            <MessageContextMenu msg={msg}>{messageContent}</MessageContextMenu>
          )}
          {/* Pulled outward by the button's own optical inset so the first glyph
              lands on the text's edge rather than its box doing so — and outward is a
              different side per role now that a user turn hugs the trailing edge. With
              the inset always on the left, the bar under a right-aligned bubble grew
              leftward and its last glyph sat ~5px inside the text it belongs to. */}
          {actionsVisibility !== "absent" && (
            <div
              className={cn(
                "flex shrink-0 transition-[opacity,visibility] duration-[var(--dur-fast)]",
                ACTIONS_VISIBILITY[actionsVisibility],
                isUser
                  ? "-mr-[calc((var(--control-height-sm)-var(--icon-sm))/2)]"
                  : "-ml-[calc((var(--control-height-sm)-var(--icon-sm))/2)]",
              )}
            >
              <Slot name="message.actions" />
            </div>
          )}
        </div>
        {actionMaterialization === "settled" && terminalFooter}
      </MessageVisibleMaterialProvider>
    </MessageContext.Provider>
  );
}

// React.memo with default shallow comparison, and both props are shaped to make it
// actually bail out:
//
//   * `row` comes from the transcript projection, which reuses a row whose message and
//     facts are all unchanged. The fold keeps untouched messages at the same reference,
//     so during pure text streaming only the tail row is new.
//   * `ctx` holds no session data at all, so nothing a run does can invalidate it.
//
// Both halves are load-bearing: each turn receives context narrowed to its own
// tool calls and delegated narratives, so unrelated deltas do not invalidate it.
export const MessageBlock = memo(MessageBlockInner);

// How each action-bar visibility state looks. The state machine is the
// message-actions context's rule; what it looks like is this view's, and this is
// the only view that renders the bar. Hover reveal stays in CSS (`group-hover` /
// `focus-within`) rather than JS so a hovering pointer never triggers a render;
// the ancestor carrying `.group` is the message container below.
// Once a turn settles, the bar stays IN FLOW in every visibility state, so the turn
// always reserves the row it may reveal. Active turns mount no bar: reserving terminal
// controls below material that is still growing makes that row chase the streaming
// tail, and a transient root-attention change can make it flash. Hanging the settled
// transient states outside the box (`absolute top-full`)
// looks like it saves the gap and does not: every message carries
// `content-visibility: auto`, whose paint containment clips anything drawn past
// its edge, so the bar came out sliced. Reserved space is also the only version
// where pointing at a turn doesn't move the text under the pointer.
const ACTIONS_VISIBILITY: Record<Exclude<MessageActionsVisibility, "absent">, string> = {
  // `invisible`, not `pointer-events-none opacity-0`: transparency stops the pointer
  // but not the keyboard, so every message in a streaming run held two focusable
  // buttons nobody could see and nothing could reveal. `visibility` also keeps the
  // reserved row, which `display: none` would not.
  hidden: "invisible opacity-0",
  hover: "opacity-0 group-hover:opacity-100 focus-within:opacity-100",
  pinned: "opacity-100",
};
