import type { BlockCtx } from "./BlockRenderer";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { memo, useMemo, type ReactNode } from "react";
import { useCitationSources } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import {
  messageActionMaterialization,
  messageBlocksRenderInstant,
  messageCitations,
} from "../application/messageBlockModel";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { Icon } from "@/ui";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { CitationContext } from "./CitationContext";
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
  /** Session-level material that belongs after this exact turn settles visibly. */
  terminalFooter?: ReactNode;
}) {
  const msg = row.message;
  const isUser = msg.role === "user";
  const t = useT();
  const messageContext = useMemo(() => ({ sessionId, message: msg }), [sessionId, msg]);

  const sources = useCitationSources();
  const citations = useMemo(() => messageCitations(msg.blocks, sources), [msg.blocks, sources]);
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
  const actionsVisibility = messageActionsVisibility({
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

  const content = renderMessageBlocks(row, blockCtx);

  const roleLabel = t(isUser ? "role.user" : "role.assistant");
  const stamp = formatClock(msg.createdAt);

  return (
    <MessageContext.Provider value={messageContext}>
      <CitationContext.Provider value={citations}>
        <MessageVisibleMaterialProvider
          owner={visibleMaterialOwner}
          generation={visibleMaterialGeneration}
        >
          {/* A caption line over a full-width body, not an avatar gutter beside a
            narrowed one. Who is speaking is a two-word fact you read once per
            turn; the reading measure is the thing you spend the whole turn
            inside, and a 38px gutter was taking it from every code block, diff
            and table below. */}
          {/* A user turn hugs the trailing edge and takes only the width its words need;
            an assistant turn is the document and takes the whole measure. 77% is the reference's
            cap and it matters: without one, a pasted paragraph becomes a full-width
            panel again and the distinction disappears exactly when the turn is long. */}
          <div className={cn("group relative flex min-w-0 flex-col gap-2", isUser && "items-end")}>
            <div className="flex min-h-5 min-w-0 items-center gap-2 text-ui-xs text-fg-faint">
              {/* The turn's mark, and the one place the accent gets to be solid rather
                than a wash. It is the only object that repeats at every turn, so it is
                what the eye uses to find where a turn begins while scrolling — at 18px
                of bare accent glyph it was the same weight as the words beside it and
                did no finding at all. The reference sets a filled block here for the
                same reason. A square (rounded) for the agent and a circle for the
                person: one is a system, the other is somebody. */}
              <span
                aria-hidden
                className={cn(
                  "grid size-5 shrink-0 place-items-center",
                  isUser
                    ? "rounded-full bg-surface-2 text-fg-muted"
                    : "rounded-[var(--shape-xs)] bg-cta text-cta-text",
                )}
              >
                <Icon name={isUser ? "user" : "sparkle"} size="xs" />
              </span>
              <span className="min-w-0 truncate">{roleLabel}</span>
              {stamp && (
                <>
                  <span aria-hidden>·</span>
                  <span className="shrink-0 font-mono tabular-nums">{stamp}</span>
                </>
              )}
            </div>
            <MessageContextMenu msg={msg}>
              <div
                className={cn(
                  MESSAGE_CONTENT_CLASS,
                  "min-w-0 text-pretty leading-prose text-prose text-fg",
                  // The human's words get their own material. `bg-card` made a user
                  // turn the same fill as a tool card, so the one thing on the page
                  // that is not the agent wore the agent's skin; the accent at wash
                  // strength says "you" once, quietly, and is the only tint in the
                  // reading column.
                  isUser && "max-w-[77%] rounded-bubble bg-accent-wash px-4 py-3",
                )}
              >
                {content}
              </div>
            </MessageContextMenu>
            {/* After the message, never in its caption. What you do WITH a turn
              belongs where the turn ends: in the caption the bar competed with
              the one line that says who is speaking, and it ran to the far edge
              of the column where nothing else in the transcript is.
              Pulled OUTWARD by the button's own optical inset so the first glyph
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
      </CitationContext.Provider>
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
