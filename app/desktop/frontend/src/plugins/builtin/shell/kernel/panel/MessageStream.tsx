import { publishStreamFollow } from "./streamFollow";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { AnimatePresence, motion } from "motion/react";
import { Fragment, useEffect } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { useMotionOff } from "@/lib/appearance";
import { dayKey, formatDay } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { Loader } from "@/ui";
import { Slot } from "@/plugins/host/Slot";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import { MessageBlock, RootRunOutcome } from "@/plugins/builtin/chat/message/public/rendering";

// Chat scroll surface, backed by use-stick-to-bottom. `resetKey`
// re-keys the subtree on session switch so a new thread lands at the
// bottom. That landing is `initial="instant"` (a jump, not an animation):
// a smooth initial replays a visible top→bottom scroll through the whole
// history on every mount / session switch / remount — which reads as the
// chat "auto-scrolling on open" and flashes content-visibility gaps as it
// flies past unrendered messages. Only the resize catch-up below stays
// smooth. Follow state is published by ControlsRelay below.

interface Props {
  messages: Message[];
  ctx: BlockCtx;
  /** Re-key on change to reset scroll position + follow state. */
  resetKey: string;
}

// Publishes StickToBottom's follow state out of the provider, for the
// jump-to-bottom button — which has to be a sibling of the scroller to sit over
// the composer, so it cannot read the context itself.
//
// Publishing rather than calling a parent's setState: the context object is rebuilt
// on every scroll event, so reporting it upward re-rendered the component that owns
// the composer at scroll frequency (see streamFollow.ts).
function ControlsRelay() {
  const ctx = useStickToBottomContext();
  // In an effect, not during render: the publish notifies subscribers, and doing
  // that mid-render would be updating one component while another is rendering. No
  // dep array — this runs on each of this (null-rendering) component's renders, so
  // the click handler it hands out is never a stale closure.
  useEffect(() => {
    publishStreamFollow({
      atBottom: ctx.isAtBottom,
      scrollToBottom: () => void ctx.scrollToBottom(),
    });
  });
  return null;
}

// A date, once, where the date changes. The clock time lives in every turn's own
// caption now, so a separator that repeated the full stamp above each turn was
// saying the same thing twice and saying it loudest at the boundary that carried
// the least information.
function DaySeparator({ createdAt }: { createdAt?: string }) {
  // useT() keeps this reactive on locale toggle even though the
  // translation function itself isn't used for the timestamp label.
  useT();
  const label = formatDay(createdAt);
  if (!label) return null;
  return (
    <div className="flex items-center gap-3 py-1">
      <span className="text-ui-xs text-fg-faint">{label}</span>
      <span aria-hidden className="h-px flex-1 bg-[var(--color-divider)]" />
    </div>
  );
}

/** Index of every message that opens a new calendar day — the separator's rule,
 *  computed once per render of the list rather than per row, so a row never has
 *  to know what came before it. */
function dayBoundaries(messages: Message[]): ReadonlySet<number> {
  const boundaries = new Set<number>();
  let previous: string | null = null;
  messages.forEach((message, index) => {
    const day = dayKey(message.createdAt);
    if (!day) return;
    if (previous !== null && day !== previous) boundaries.add(index);
    previous = day;
  });
  return boundaries;
}

export function MessageStream({ messages, ctx, resetKey }: Props) {
  // While a run streams, content grows continuously; the default `resize`
  // spring (stiffness 0.05 / mass 1.25) is too sluggish to track it and the
  // tail scrolls out of view (D2). Hard-pin to the bottom during generation,
  // and keep the smooth catch-up only when idle (re-open / history load).
  // `running` flips only at run boundaries, so this never churns per token.
  const running = useIsCurrentRootRunning();
  const motionOff = useMotionOff();

  const boundaries = dayBoundaries(messages);

  if (messages.length === 0) {
    return (
      <StickToBottom
        key={resetKey}
        className="msg-scroll-frame"
        initial="instant"
        resize={motionOff ? "instant" : "smooth"}
      >
        <StickToBottom.Content
          scrollClassName="panel-scroll"
          className="relative mx-auto flex w-full max-w-[var(--content-max)] flex-col gap-7 px-[var(--density-column-gutter)] pt-8 pb-8 sm:px-[var(--density-column-gutter-wide)]"
        >
          <Slot name="chat.empty" />
        </StickToBottom.Content>
        <ControlsRelay />
      </StickToBottom>
    );
  }

  return (
    <StickToBottom
      key={resetKey}
      className="panel-scroll msg-scroll"
      initial="instant"
      // A transcript that eases itself into place is motion, and motion is a
      // preference. The scroll library has no idea the user turned it off, so
      // the one place that knows tells it — otherwise "reduce motion" leaves
      // the one surface that moves most still moving.
      resize={running || motionOff ? "instant" : "smooth"}
    >
      {/* `msg-scroll-viewport` names the element that actually scrolls. The
          library renders it itself, one level inside the class above, so anything
          outside the transcript that needs the scroll box — the narrative rails —
          would otherwise have to guess at that nesting. */}
      <StickToBottom.Content
        scrollClassName="panel-scroll msg-scroll-viewport"
        className="relative mx-auto flex w-full max-w-[var(--content-max)] flex-col gap-7 px-[var(--density-column-gutter)] pb-8 pt-8 sm:px-[var(--density-column-gutter-wide)]"
      >
        <AnimatePresence initial={false}>
          {messages.map((m, i) => (
            <Fragment key={m.id}>
              {boundaries.has(i) && <DaySeparator createdAt={m.createdAt} />}
              {/* No `layout` prop — Motion's layout animation re-tweens
                  the block on every text delta, making the whole bubble
                  (avatar included) bobble while streaming. enterUp is
                  enough: first paint slides in, then the block grows
                  naturally with the DOM.

                  `content-visibility:auto` lets the browser skip layout+paint for
                  off-screen messages (the long-conversation scaling cliff) while
                  keeping every node IN the DOM — so ⌘F's TreeWalker + CSS-highlight
                  search, copy-all, and stick-to-bottom's height all still work
                  (true virtualization would unmount nodes and break those). The
                  `auto` intrinsic-size remembers each message's real height after
                  its first render, so the scroll height stays accurate; the 220px
                  fallback only covers never-yet-rendered messages far below. */}
              {/* `data-turn-id` is the anchor the narrative rails navigate by.
                  An attribute rather than a registry: the rails need the
                  element's position in the scroller, which only the DOM has. */}
              <motion.div
                {...enterUp}
                data-turn-id={m.id}
                data-turn-role={m.role}
                className="[content-visibility:auto] [contain-intrinsic-size:auto_220px]"
              >
                <MessageBlock
                  msg={m}
                  ctx={ctx}
                  isLast={i === messages.length - 1}
                  isRunning={running}
                />
              </motion.div>
            </Fragment>
          ))}
        </AnimatePresence>
        {/* Waiting-for-response indicator — the run is live but the assistant
            hasn't opened its turn yet (last message is still the user's). Once
            the assistant message arrives it takes over, so this hides itself. */}
        {running && messages[messages.length - 1]?.role === "user" && (
          <div className="flex">
            <Loader variant="dots" />
          </div>
        )}
        {!running && <RootRunOutcome />}
      </StickToBottom.Content>
      <ControlsRelay />
    </StickToBottom>
  );
}
