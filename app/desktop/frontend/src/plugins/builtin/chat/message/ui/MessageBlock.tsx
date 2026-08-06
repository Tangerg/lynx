import type { BlockCtx } from "./BlockRenderer";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { memo, useMemo } from "react";
import { useCitationSources } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import { messageBlocksRenderInstant, messageCitations } from "../application/messageBlockModel";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { Icon } from "@/ui";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { CitationContext } from "./CitationContext";
import { MessageContextMenu } from "./MessageContextMenu";
import { renderBlock, renderMessageBlocks } from "./BlockRenderer";

function MessageBlockInner({
  row,
  ctx,
  isLast,
  isRunning,
}: {
  row: TranscriptRow;
  ctx: BlockCtx;
  /** Last turn in the thread — its action bar stays pinned open. */
  isLast: boolean;
  /** A run is streaming — action bars stay hidden until it settles.
   *  Flips only at run boundaries, so it never churns this memo per token. */
  isRunning: boolean;
}) {
  const msg = row.message;
  const isUser = msg.role === "user";
  const t = useT();

  const sources = useCitationSources();
  const citations = useMemo(() => messageCitations(msg.blocks, sources), [msg.blocks, sources]);

  // System messages (e.g. a compaction boundary) are chrome-less full-width
  // notes — no avatar / name / time / outline / context-menu, just the block(s)
  // rendered inline (CompactionBlock draws its own divider). Placed after all
  // hooks so rules-of-hooks holds.
  if (msg.role === "system") {
    return (
      <MessageContext.Provider value={msg}>
        <div className={MESSAGE_CONTENT_CLASS}>
          {msg.blocks.map((block, index) => renderBlock(block, index, row.facts, ctx))}
        </div>
      </MessageContext.Provider>
    );
  }

  const blockCtx: BlockCtx = messageBlocksRenderInstant(msg.role) ? { ...ctx, instant: true } : ctx;

  const content = renderMessageBlocks(row, blockCtx);

  const actionsClass = cn(
    "flex shrink-0 transition-[opacity,visibility] duration-[var(--dur-fast)]",
    ACTIONS_VISIBILITY[messageActionsVisibility({ isRunning, isLast })],
  );

  const roleLabel = t(isUser ? "role.user" : "role.assistant");
  const stamp = formatClock(msg.createdAt);

  return (
    <MessageContext.Provider value={msg}>
      <CitationContext.Provider value={citations}>
        {/* A caption line over a full-width body, not an avatar gutter beside a
            narrowed one. Who is speaking is a two-word fact you read once per
            turn; the reading measure is the thing you spend the whole turn
            inside, and a 38px gutter was taking it from every code block, diff
            and table below. */}
        {/* A user turn hugs the trailing edge and takes only the width its words need;
            an assistant turn is the document and takes the whole measure. Both used to
            be full-width panels, so the transcript read as two kinds of document
            alternating instead of a document with asides in it. 77% is the reference's
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
          <div
            className={cn(
              actionsClass,
              isUser
                ? "-mr-[calc((var(--control-height-sm)-var(--icon-sm))/2)]"
                : "-ml-[calc((var(--control-height-sm)-var(--icon-sm))/2)]",
            )}
          >
            <Slot name="message.actions" />
          </div>
        </div>
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
// Both halves are load-bearing and the second one used to be false: `ctx` carried the
// session's tool-call map and its delegated-run narratives, and the narratives were
// rebuilt from scratch on every delta. That gave every message a new `ctx` on every
// token, so this memo never bailed once during a run — with 200 messages on screen,
// 199× redundant renders per token, each one re-planning its render units.
export const MessageBlock = memo(MessageBlockInner);

// How each action-bar visibility state looks. The state machine is the
// message-actions context's rule; what it looks like is this view's, and this is
// the only view that renders the bar. Hover reveal stays in CSS (`group-hover` /
// `focus-within`) rather than JS so a hovering pointer never triggers a render;
// the ancestor carrying `.group` is the message container below.
// The bar stays IN FLOW in every state, so the turn always reserves the row it
// may reveal. Hanging the transient states outside the box (`absolute top-full`)
// looks like it saves the gap and does not: every message carries
// `content-visibility: auto`, whose paint containment clips anything drawn past
// its edge, so the bar came out sliced. Reserved space is also the only version
// where pointing at a turn doesn't move the text under the pointer.
const ACTIONS_VISIBILITY: Record<MessageActionsVisibility, string> = {
  // `invisible`, not `pointer-events-none opacity-0`: transparency stops the pointer
  // but not the keyboard, so every message in a streaming run held two focusable
  // buttons nobody could see and nothing could reveal. `visibility` also keeps the
  // reserved row, which `display: none` would not.
  hidden: "invisible opacity-0",
  hover: "opacity-0 group-hover:opacity-100 focus-within:opacity-100",
  pinned: "opacity-100",
};
